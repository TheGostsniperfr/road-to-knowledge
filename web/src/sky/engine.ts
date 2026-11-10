import type { UniverseView } from "../api";
import { SkyGL, type Camera } from "./gl";
import { buildLayout, type CNode, type GNode, type Layout, type PNode, type SkyNode, type SNode } from "./layout";
import { synthesize, type Slot } from "./particles";
import { TAU, clamp, ease, mkRng, ramp } from "./rand";

export type ScaleName = "Univers" | "Galaxie" | "Amas" | "Système stellaire";

export interface SkyEvents {
  onSelect(node: SkyNode | null): void;
  onScale?(name: ScaleName): void;
  onFps?(fps: number): void;
}

interface Hit { n: SkyNode; x: number; y: number; r: number; d: number }
interface Flight { f: (t: number) => [number, number, number]; t0: number; dur: number; end: Camera }

const reduceMotion = () => matchMedia("(prefers-reduced-motion: reduce)").matches;
const clampK = (k: number) => clamp(k, 0.02, 160);

// van Wijk & Nuij optimal zoom-and-pan path, the one d3-interpolate-zoom uses.
function vanWijk(p0: [number, number, number], p1: [number, number, number]) {
  const rho = 1.42, r2 = rho * rho, r4 = r2 * r2;
  const [ux0, uy0, w0] = p0, [ux1, uy1, w1] = p1, dx = ux1 - ux0, dy = uy1 - uy0, d2 = dx * dx + dy * dy;
  let f: (t: number) => [number, number, number], S: number;
  if (d2 < 1e-6) {
    S = Math.log(w1 / w0) / rho;
    f = (t) => [ux0 + t * dx, uy0 + t * dy, w0 * Math.exp(rho * t * S)];
  } else {
    const d1 = Math.sqrt(d2);
    const b0 = (w1 * w1 - w0 * w0 + r4 * d2) / (2 * w0 * r2 * d1), b1 = (w1 * w1 - w0 * w0 - r4 * d2) / (2 * w1 * r2 * d1);
    const q0 = Math.log(Math.sqrt(b0 * b0 + 1) - b0), q1 = Math.log(Math.sqrt(b1 * b1 + 1) - b1);
    S = (q1 - q0) / rho;
    f = (t) => {
      const s = t * S, c0 = Math.cosh(q0), u = (w0 / (r2 * d1)) * (c0 * Math.tanh(rho * s + q0) - Math.sinh(q0));
      return [ux0 + u * dx, uy0 + u * dy, (w0 * c0) / Math.cosh(rho * s + q0)];
    };
  }
  return { f, dur: clamp(Math.abs(S) * 620, 500, 3200) };
}

export class Sky {
  private gl: SkyGL | null;
  private ctx: CanvasRenderingContext2D;
  private layout: Layout = { galaxies: [], clusters: [], stars: [], byId: new Map() };
  private slots: Slot[] = [];
  private cam: Camera = { x: 0, y: 0, k: 0.05 };
  private target: Camera = { x: 0, y: 0, k: 0.05 };
  private flight: Flight | null = null;
  private W = 1;
  private H = 1;
  private dpr = 1;
  private hits: Hit[] = [];
  private selected: SkyNode | null = null;
  private hovered: SkyNode | null = null;
  private running = false;
  private dirty = true;
  private visible = true;
  private paused = false;
  private pointer: [number, number] | null = null;
  private reduce = reduceMotion();
  private T0 = performance.now();
  private born = performance.now();
  private frames = 0;
  private pass = 0;
  private fpsT = performance.now();
  private lastScale = "";
  private glowCache = new Map<number, HTMLCanvasElement>();
  private ptrs = new Map<number, [number, number]>();
  private down: { x: number; y: number; cx: number; cy: number; moved: boolean } | null = null;
  private pinch: { d: number; k: number } | null = null;
  private cleanup: (() => void)[] = [];
  readonly webgl: boolean;

  constructor(private glCanvas: HTMLCanvasElement, private canvas: HTMLCanvasElement, private ev: SkyEvents) {
    this.gl = SkyGL.create(glCanvas);
    this.webgl = !!this.gl;
    this.ctx = canvas.getContext("2d")!;
    this.bind();
  }

  get nodes() {
    return this.layout;
  }

  setUniverse(u: UniverseView) {
    const first = this.layout.galaxies.length === 0;
    const selId = this.selected?.id;
    this.layout = buildLayout(u);
    const { particles, slots } = synthesize(this.layout);
    this.slots = slots;
    this.gl?.setParticles(particles, slots.length);
    this.selected = selId ? this.layout.byId.get(selId) ?? null : null;
    this.hovered = null;
    if (first) {
      const [x, y, k] = this.universe();
      Object.assign(this.cam, { x, y, k: k * 0.25 });
      Object.assign(this.target, this.cam);
      this.born = performance.now();
      setTimeout(() => this.flyTo(x, y, k), this.reduce ? 0 : 250);
    }
    this.kick();
  }

  select(id: string | null, fly = true) {
    const n = id ? this.layout.byId.get(id) ?? null : null;
    this.selected = n;
    if (fly) {
      const [x, y, k] = n ? this.fit(n) : this.universe();
      this.flyTo(x, y, k);
    }
    this.ev.onSelect(n);
    this.kick();
  }

  zoomBy(f: number) {
    this.flight = null;
    this.target.k = clampK(this.target.k * f);
    this.kick();
  }

  // Study mode covers the map: the loop stops instead of drawing frames nobody sees.
  setPaused(on: boolean) {
    this.paused = on;
    if (!on) this.kick();
  }

  destroy() {
    this.cleanup.forEach((f) => f());
    this.visible = false;
    this.gl?.release();
  }

  // --- camera ---

  private universe(): [number, number, number] {
    let a = 1e9, b = 1e9, c = -1e9, d = -1e9;
    for (const g of this.layout.galaxies) {
      a = Math.min(a, g.x - g.R); b = Math.min(b, g.y - g.R); c = Math.max(c, g.x + g.R); d = Math.max(d, g.y + g.R);
    }
    if (a > c) return [0, 0, 0.1];
    return [(a + c) / 2, (b + d) / 2, Math.min(this.W / (c - a), this.H / (d - b)) * 0.96];
  }

  private fit(n: SkyNode): [number, number, number] {
    const m = Math.min(this.W, this.H);
    switch (n.kind) {
      case "galaxy": return [n.x, n.y, m / (n.R * 2.25)];
      case "cluster": return [n.x, n.y, m / (n.r * 2.5)];
      case "star": return n.planets.length ? [n.x, n.y, m / (n.u * (3 + 2.2 * n.planets.length) * 2.6)] : [n.x, n.y, m / (n.c.r * 0.9)];
      case "planet": return [n.star.x, n.star.y, m / (n.star.u * (3 + 2.2 * n.star.planets.length) * 2.2)];
    }
  }

  private flyTo(x: number, y: number, k: number) {
    k = clampK(k);
    if (this.reduce) {
      Object.assign(this.cam, { x, y, k });
      Object.assign(this.target, this.cam);
      this.kick();
      return;
    }
    const { f, dur } = vanWijk([this.cam.x, this.cam.y, this.W / this.cam.k], [x, y, this.W / k]);
    this.flight = { f, t0: performance.now(), dur, end: { x, y, k } };
    this.kick();
  }

  private scaleName(): ScaleName {
    const k = this.cam.k;
    return k < 0.28 ? "Univers" : k < 2 ? "Galaxie" : k < 12 ? "Amas" : "Système stellaire";
  }

  private toS(x: number, y: number): [number, number] {
    return [(x - this.cam.x) * this.cam.k + this.W / 2, (y - this.cam.y) * this.cam.k + this.H / 2];
  }

  private toW(sx: number, sy: number): [number, number] {
    return [(sx - this.W / 2) / this.cam.k + this.cam.x, (sy - this.H / 2) / this.cam.k + this.cam.y];
  }

  // --- loop ---

  private kick() {
    this.dirty = true;
    if (this.running || !this.visible || this.paused) return;
    this.running = true;
    requestAnimationFrame((t) => this.frame(t));
  }

  private frame(now: number) {
    this.running = false;
    if (!this.visible || this.paused || document.hidden) return;
    const { cam, target } = this;
    if (this.flight) {
      const fl = this.flight, t = Math.min(1, (now - fl.t0) / fl.dur), [x, y, w] = fl.f(ease(t));
      cam.x = x; cam.y = y; cam.k = this.W / w;
      if (t >= 1) {
        Object.assign(cam, fl.end);
        this.flight = null;
      }
      Object.assign(target, cam);
    } else {
      const e = this.reduce ? 1 : 0.18;
      let moved = false;
      for (const key of ["x", "y"] as const) {
        const d = target[key] - cam[key];
        if (Math.abs(d * cam.k) > 0.05) { cam[key] += d * e; moved = true; } else cam[key] = target[key];
      }
      const r = target.k / cam.k;
      if (Math.abs(r - 1) > 0.001) { cam.k *= Math.pow(r, e); moved = true; } else cam.k = target.k;
      if (moved) this.dirty = true;
    }
    const t = (now - this.T0) / 1000;
    this.writeSlots(now);
    this.gl?.draw(cam, this.W, this.H, this.dpr, t, !this.reduce);
    this.draw(t);
    const sc = this.scaleName();
    if (sc !== this.lastScale) {
      this.lastScale = sc;
      this.ev.onScale?.(sc);
    }
    this.frames++;
    if (now - this.fpsT > 1000) {
      this.ev.onFps?.(this.frames);
      this.frames = 0;
      this.fpsT = now;
    }
    const animating = !this.reduce || this.flight || this.dirty || now - this.born < 3000;
    this.dirty = false;
    if (animating) {
      this.running = true;
      requestAnimationFrame((tt) => this.frame(tt));
    }
  }

  private writeSlots(now: number) {
    if (!this.gl) return;
    const d = this.gl.slots, k = this.cam.k;
    this.slots.forEach((s, i) => {
      const o = i * 12;
      if (s.kind === "g") {
        const g = s.g, born = this.reduce ? 1 : ease(ramp(now - this.born - 300 - s.i * 170, 0, 1400)), sc = 0.82 + 0.18 * born;
        d.set([g.x, g.y, g.R * sc, born, g.ct, g.st, g.tilt, g.m, g.cp, g.sp, 0, g.zt], o);
      } else {
        const c = s.c, born = this.reduce ? 1 : ramp(now - this.born - 900 - s.gi * 170, 0, 1400);
        d.set([c.x, c.y, c.r, born * ramp(c.r * k, 1.2, 5), 1, 0, 0.82, 1, 1, 0, 0, 0.57], o);
      }
    });
  }

  // --- overlay: knowledge stars, planets, wormholes, labels ---

  private glow(h: number) {
    h = Math.round(h / 6) * 6;
    let cv = this.glowCache.get(h);
    if (cv) return cv;
    cv = document.createElement("canvas");
    cv.width = cv.height = 128;
    const c = cv.getContext("2d")!, gr = c.createRadialGradient(64, 64, 0, 64, 64, 64);
    gr.addColorStop(0, "rgba(255,255,255,1)");
    gr.addColorStop(0.07, `hsla(${h},100%,93%,.95)`);
    gr.addColorStop(0.2, `hsla(${h},90%,72%,.4)`);
    gr.addColorStop(0.5, `hsla(${h},90%,60%,.08)`);
    gr.addColorStop(1, `hsla(${h},90%,55%,0)`);
    c.fillStyle = gr;
    c.fillRect(0, 0, 128, 128);
    this.glowCache.set(h, cv);
    return cv;
  }

  private label(text: string, x: number, y: number, size: number, weight: string, alpha: number, color: string, spacing = "0px", display = false) {
    if (alpha < 0.02) return;
    const ctx = this.ctx;
    ctx.font = `${weight} ${size}px ${display ? '"Chakra Petch"' : '"Montserrat"'}, system-ui, sans-serif`;
    if ("letterSpacing" in ctx) ctx.letterSpacing = spacing;
    ctx.textAlign = "center";
    ctx.globalAlpha = alpha;
    ctx.lineWidth = 3.5;
    ctx.strokeStyle = "rgba(2,3,8,.85)";
    ctx.lineJoin = "round";
    ctx.strokeText(text, x, y);
    ctx.fillStyle = color;
    ctx.fillText(text, x, y);
    ctx.globalAlpha = 1;
    if ("letterSpacing" in ctx) ctx.letterSpacing = "0px";
  }

  private draw(t: number) {
    const { ctx, cam, W, H } = this, k = cam.k;
    this.hits = [];
    this.pass++;
    ctx.setTransform(this.dpr, 0, 0, this.dpr, 0, 0);
    ctx.clearRect(0, 0, W, H);
    if (!this.gl) {
      ctx.fillStyle = "#04060E";
      ctx.fillRect(0, 0, W, H);
    }
    for (const g of this.layout.galaxies) {
      const gs = g.R * k, [sx, sy] = this.toS(g.x, g.y);
      if (gs > 8 && sx > -gs && sx < W + gs && sy > -gs && sy < H + gs) this.hits.push({ n: g, x: sx, y: sy, r: Math.max(24, gs * 0.6), d: 0 });
    }
    for (const c of this.layout.clusters) {
      const cs = c.r * k, [sx, sy] = this.toS(c.x, c.y);
      if (sx < -cs * 2 || sy < -cs * 2 || sx > W + cs * 2 || sy > H + cs * 2) continue;
      if (cs > 4) this.hits.push({ n: c, x: sx, y: sy, r: Math.max(16, cs * 0.9), d: 1 });
      this.clusterMark(c, sx, sy, cs, t);
      if (cs > 34) this.drawStars(c, cs, t);
    }
    this.drawWorms(t);
    const n = this.selected;
    if (n && (n.kind === "star" || n.kind === "planet")) {
      const [sx, sy] = n.kind === "planet" ? [n.sx, n.sy] : this.toS(n.x, n.y);
      const p = this.reduce ? 0 : Math.sin(t * 3) * 2, rr = n.rs + 10 + p;
      ctx.strokeStyle = "rgba(255,115,0,.9)";
      ctx.lineWidth = 1.4;
      ctx.beginPath();
      for (let i = 0; i < 4; i++) {
        const a0 = (i * TAU) / 4 + t * 0.6;
        ctx.moveTo(sx + Math.cos(a0) * rr, sy + Math.sin(a0) * rr);
        ctx.arc(sx, sy, rr, a0, a0 + 0.9);
      }
      ctx.stroke();
    }
    this.drawLabels();
  }

  private clusterMark(c: CNode, sx: number, sy: number, cs: number, t: number) {
    const ctx = this.ctx, sel = this.selected === c, hv = this.hovered === c;
    const am = ramp(cs, 10, 20) * (1 - ramp(cs, 300, 700));
    if (am < 0.02) return;
    const rr = Math.max(cs * 0.95, 10);
    ctx.globalAlpha = am * (sel ? 0.95 : hv ? 0.85 : 0.6);
    ctx.strokeStyle = sel ? "#FF7300" : `hsl(${c.hue},70%,${hv ? 86 : 76}%)`;
    ctx.lineWidth = sel ? 1.6 : hv ? 1.3 : 1.1;
    ctx.setLineDash([3, 5]);
    ctx.lineDashOffset = this.reduce ? 0 : -t * 6;
    ctx.beginPath();
    ctx.arc(sx, sy, rr, 0, TAU);
    ctx.stroke();
    ctx.setLineDash([]);
    ctx.lineDashOffset = 0;
    ctx.globalAlpha = 1;
  }

  private fog(s: SNode, sx: number, sy: number, size: number, alpha: number) {
    const ctx = this.ctx, r = mkRng(Math.floor(s.seed * 1e4));
    for (let i = 0; i < 7; i++) {
      const x = sx + (r() - 0.5) * size * 0.7, y = sy + (r() - 0.5) * size * 0.55, rr = size * (0.22 + r() * 0.3);
      const gr = ctx.createRadialGradient(x, y, 0, x, y, rr);
      gr.addColorStop(0, `rgba(3,4,10,${0.5 * alpha})`);
      gr.addColorStop(1, "rgba(3,4,10,0)");
      ctx.fillStyle = gr;
      ctx.fillRect(x - rr, y - rr, rr * 2, rr * 2);
    }
  }

  private starGlyph(s: SNode, sx: number, sy: number, rs: number, va: number, t: number, big: boolean) {
    const ctx = this.ctx, tw = this.reduce ? 1 : 0.85 + 0.15 * Math.sin(t * 1.6 + s.seed);
    if (s.level === 0) {
      ctx.globalCompositeOperation = "lighter";
      ctx.globalAlpha = va * (0.45 + 0.25 * Math.sin(t * 1.1 + s.seed));
      const g = rs * 7;
      ctx.drawImage(this.glow(18), sx - g / 2, sy - g / 2, g, g);
      ctx.globalCompositeOperation = "source-over";
      ctx.globalAlpha = va * 0.55;
      ctx.strokeStyle = "rgba(200,190,220,.7)";
      ctx.setLineDash([2, 4]);
      ctx.lineWidth = 1;
      ctx.beginPath();
      ctx.arc(sx, sy, rs * 3.2, 0, TAU);
      ctx.stroke();
      ctx.setLineDash([]);
      ctx.globalAlpha = 1;
      return;
    }
    ctx.globalCompositeOperation = "lighter";
    ctx.globalAlpha = va * tw;
    const g = rs * (big ? 9 : 7.5);
    ctx.drawImage(this.glow(s.hue), sx - g / 2, sy - g / 2, g, g);
    if (s.level >= 4) {
      ctx.strokeStyle = `hsla(${s.hue},80%,88%,${0.35 * va * tw})`;
      ctx.lineWidth = 1;
      const L = rs * 6;
      ctx.beginPath();
      ctx.moveTo(sx - L, sy); ctx.lineTo(sx + L, sy); ctx.moveTo(sx, sy - L); ctx.lineTo(sx, sy + L);
      ctx.stroke();
    }
    ctx.globalCompositeOperation = "source-over";
    ctx.globalAlpha = 1;
  }

  private drawStars(c: CNode, cs: number, t: number) {
    const { ctx, cam, W, H } = this, k = cam.k, va = ramp(cs, 34, 90), f = clamp(cs / 150, 0.5, 6);
    for (const s of c.stars) {
      const [sx, sy] = this.toS(s.x, s.y);
      if (sx < -80 || sy < -80 || sx > W + 80 || sy > H + 80) continue;
      if (s.level === 0) {
        const rs = 1.6 * f;
        this.fog(s, sx, sy, c.r * 0.5 * k, va);
        this.starGlyph(s, sx, sy, rs, va, t, false);
        s.rs = rs * 3;
        this.hits.push({ n: s, x: sx, y: sy, r: Math.max(10, rs * 3.5), d: 2 });
        continue;
      }
      const rs = (0.9 + s.level * 0.55) * f;
      s.rs = rs;
      // Notions orbit only the subject in focus, or one zoomed in enough to fill the view: a
      // sub-theme with hundreds of notions drawn at once is unreadable and slow.
      const showP = s.planets.length > 0 && s.u * k > 2.2 && (this.inFocus(s) || this.overOrbit(s, sx, sy) || s.u * k > 9), pa = ramp(s.u * k, 2.2, 5);
      if (showP) this.drawPlanets(s, sx, sy, t, pa, true);
      this.starGlyph(s, sx, sy, rs, va, t, s === this.hovered || s === this.selected);
      if (showP) this.drawPlanets(s, sx, sy, t, pa, false);
      if (s.links.length && cs > 60) {
        ctx.strokeStyle = `rgba(180,140,255,${0.45 * va})`;
        ctx.lineWidth = 1.2;
        const rr = rs * 3.2 + 4;
        ctx.beginPath();
        for (let i = 0; i < 3; i++) {
          const a0 = (i * TAU) / 3 + (this.reduce ? 0 : t * 1.4);
          ctx.moveTo(sx + Math.cos(a0) * rr, sy + Math.sin(a0) * rr);
          ctx.arc(sx, sy, rr, a0, a0 + 1.2);
        }
        ctx.stroke();
      }
      // Erosion hint only: an amber dot, the level itself never drops.
      if (s.stale && cs > 120) {
        ctx.fillStyle = "#E0A84A";
        ctx.globalAlpha = va;
        ctx.beginPath();
        ctx.arc(sx + rs * 1.6, sy - rs * 1.6, 2.4, 0, TAU);
        ctx.fill();
        ctx.globalAlpha = 1;
      }
      this.hits.push({ n: s, x: sx, y: sy, r: Math.max(9, rs * 2.4), d: 2 });
    }
  }

  // The pointer on its way from a star to one of its planets keeps the orbits drawn.
  private overOrbit(s: SNode, sx: number, sy: number): boolean {
    if (!this.pointer) return false;
    const outer = s.u * (3 + 2.2 * (s.planets.length - 1)) * this.cam.k + 12;
    const dx = (this.pointer[0] - sx) / outer, dy = (this.pointer[1] - sy) / (outer * 0.42 + 12);
    return dx * dx + dy * dy <= 1;
  }

  private inFocus(s: SNode): boolean {
    const sel = this.selected, hov = this.hovered;
    return s === sel || s === hov || (sel?.kind === "planet" && sel.star === s) || (hov?.kind === "planet" && hov.star === s);
  }

  private drawPlanets(s: SNode, sx: number, sy: number, t: number, pa: number, back: boolean) {
    const { ctx } = this, k = this.cam.k;
    for (const p of s.planets) {
      const orb = s.u * (3 + 2.2 * p.i) * k, ang = p.a0 + (this.reduce ? 0 : (t * 0.32) / (p.i + 1));
      const px = sx + Math.cos(ang) * orb, py = sy + Math.sin(ang) * orb * 0.42, isBack = Math.sin(ang) < 0;
      if (back) {
        ctx.strokeStyle = `rgba(170,190,230,${0.18 * pa})`;
        ctx.lineWidth = 1;
        ctx.beginPath();
        ctx.ellipse(sx, sy, orb, orb * 0.42, 0, 0, TAU);
        ctx.stroke();
      }
      if (isBack !== back) continue;
      const pr = clamp(s.u * k * 0.32, 2.2, 18) * (p === this.hovered || p === this.selected ? 1.25 : 1);
      const dx = sx - px, dy = sy - py, dl = Math.hypot(dx, dy) || 1;
      const gr = ctx.createRadialGradient(px + (dx / dl) * pr * 0.45, py + (dy / dl) * pr * 0.45, pr * 0.1, px, py, pr);
      gr.addColorStop(0, `hsl(${p.hue},70%,78%)`);
      gr.addColorStop(0.6, `hsl(${p.hue},45%,38%)`);
      gr.addColorStop(1, `hsl(${p.hue},40%,10%)`);
      ctx.globalAlpha = pa;
      ctx.fillStyle = gr;
      ctx.beginPath();
      ctx.arc(px, py, pr, 0, TAU);
      ctx.fill();
      ctx.globalAlpha = 1;
      p.sx = px; p.sy = py; p.rs = pr; p.pass = this.pass;
      p.x = s.x + (px - sx) / k; p.y = s.y + (py - sy) / k;
      this.hits.push({ n: p, x: px, y: py, r: Math.max(8, pr + 4), d: 3 });
    }
  }

  private drawWorms(t: number) {
    const sel = this.selected, s = sel?.kind === "planet" ? sel.star : sel;
    if (!s || s.kind !== "star" || !s.links.length) return;
    const ctx = this.ctx, p0 = this.toS(s.x, s.y);
    const bez = (a: number[], b: number[], c: number[], tt: number) => {
      const u = 1 - tt;
      return [u * u * a[0] + 2 * u * tt * c[0] + tt * tt * b[0], u * u * a[1] + 2 * u * tt * c[1] + tt * tt * b[1]];
    };
    s.links.forEach((o, i) => {
      const p1 = this.toS(o.x, o.y), mx = (p0[0] + p1[0]) / 2, my = (p0[1] + p1[1]) / 2, dx = p1[0] - p0[0], dy = p1[1] - p0[1];
      const bend = 0.28 * (i % 2 ? 1 : -1), c = [mx - dy * bend, my + dx * bend];
      const gr = ctx.createLinearGradient(p0[0], p0[1], p1[0], p1[1]);
      gr.addColorStop(0, "rgba(190,140,255,.75)");
      gr.addColorStop(1, "rgba(80,210,255,.75)");
      ctx.globalCompositeOperation = "lighter";
      ctx.strokeStyle = gr;
      ctx.lineWidth = 1.4;
      ctx.setLineDash([1, 5]);
      ctx.beginPath();
      ctx.moveTo(p0[0], p0[1]);
      ctx.quadraticCurveTo(c[0], c[1], p1[0], p1[1]);
      ctx.stroke();
      ctx.setLineDash([]);
      for (let j = 0; j < 14; j++) {
        const tt = ((this.reduce ? 0 : t * 0.22) + j / 14) % 1, [x, y] = bez(p0, p1, c, tt);
        ctx.fillStyle = `hsla(${260 - tt * 70},90%,78%,${0.8 * Math.sin(tt * Math.PI)})`;
        ctx.beginPath();
        ctx.arc(x, y, 1.8, 0, TAU);
        ctx.fill();
      }
      for (let r = 0; r < 3; r++) {
        const rr = 8 + r * 5, a0 = (this.reduce ? 0 : t * (2 - r * 0.4)) + r;
        ctx.strokeStyle = `rgba(${170 - r * 30},${150 + r * 30},255,${0.6 - r * 0.15})`;
        ctx.lineWidth = 1.3;
        ctx.beginPath();
        ctx.arc(p1[0], p1[1], rr, a0, a0 + 3.6);
        ctx.stroke();
      }
      ctx.globalCompositeOperation = "source-over";
    });
  }

  private drawLabels() {
    const { cam, W, H } = this, k = cam.k;
    for (const g of this.layout.galaxies) {
      const gs = g.R * k, a = ramp(gs, 20, 60) * (1 - ramp(gs, 1400, 2600));
      if (a < 0.02) continue;
      const [sx, sy] = this.toS(g.x, g.y);
      this.label(g.name.toUpperCase(), sx, sy - gs * g.tilt * 0.95 - 10, clamp(gs * 0.05, 12, 30), "700", a, `hsl(${g.hue},75%,84%)`, "3px", true);
    }
    for (const c of this.layout.clusters) {
      const cs = c.r * k, a = ramp(cs, 12, 26) * (1 - ramp(cs, 500, 900));
      if (a < 0.02) continue;
      const [sx, sy] = this.toS(c.x, c.y);
      if (sx < -200 || sx > W + 200 || sy < -50 || sy > H + 50) continue;
      this.label(c.name.toUpperCase(), sx, sy + (cs > 60 ? -cs * 1.05 : cs + 14), clamp(cs * 0.12, 10, 16), "600", a, `hsl(${c.hue},60%,86%)`, "2px", true);
    }
    for (const s of this.layout.stars) {
      const cs = s.c.r * k, focus = s === this.selected || s === this.hovered;
      const a = focus ? ramp(cs, 34, 50) : ramp(cs, 80, 130) * (s.level >= 3 || cs > 160 ? 1 : 0);
      if (a < 0.02) continue;
      const [sx, sy] = this.toS(s.x, s.y);
      if (sx < -100 || sx > W + 100 || sy < -30 || sy > H + 40) continue;
      this.label(s.name, sx, sy + s.rs * 2.2 + 14, s === this.selected ? 13 : 11.5, s === this.selected ? "600" : "500", a * (s.level === 0 ? 0.7 : 0.95), s.level === 0 ? "#B4AFC8" : "#EEF2FA");
    }
    for (const s of this.layout.stars) {
      if (s.u * k < 4) continue;
      for (const p of s.planets) if (p.pass === this.pass) this.label(p.name, p.sx, p.sy + p.rs + 13, 11, "500", ramp(s.u * k, 4, 7), "#C9D4EA");
    }
  }

  // --- interaction ---

  private hitTest(x: number, y: number): SkyNode | null {
    let best: (Hit & { dd: number }) | null = null;
    for (const h of this.hits) {
      const d = Math.hypot(h.x - x, h.y - y);
      if (d <= h.r && (!best || h.d > best.d || (h.d === best.d && d < best.dd))) best = { ...h, dd: d };
    }
    return best?.n ?? null;
  }

  private resize() {
    const rc = this.canvas.getBoundingClientRect();
    this.dpr = Math.min(devicePixelRatio || 1, 2);
    this.W = Math.max(1, rc.width);
    this.H = Math.max(1, rc.height);
    for (const c of [this.canvas, this.glCanvas]) {
      c.width = (this.W * this.dpr) | 0;
      c.height = (this.H * this.dpr) | 0;
    }
    this.gl?.resize(this.W, this.H, this.dpr);
    this.kick();
  }

  private bind() {
    const cv = this.canvas;
    const on = <K extends keyof HTMLElementEventMap>(el: HTMLElement, type: K, fn: (e: HTMLElementEventMap[K]) => void, opts?: AddEventListenerOptions) => {
      el.addEventListener(type, fn as EventListener, opts);
      this.cleanup.push(() => el.removeEventListener(type, fn as EventListener, opts));
    };
    const ro = new ResizeObserver(() => this.resize());
    ro.observe(cv);
    const io = new IntersectionObserver((e) => {
      this.visible = e[0].isIntersecting;
      if (this.visible) this.kick();
    });
    io.observe(cv);
    this.cleanup.push(() => { ro.disconnect(); io.disconnect(); });
    const onVis = () => { if (!document.hidden) this.kick(); };
    document.addEventListener("visibilitychange", onVis);
    this.cleanup.push(() => document.removeEventListener("visibilitychange", onVis));

    on(cv, "pointerdown", (e) => {
      cv.setPointerCapture(e.pointerId);
      this.ptrs.set(e.pointerId, [e.offsetX, e.offsetY]);
      this.flight = null;
      if (this.ptrs.size === 1) this.down = { x: e.offsetX, y: e.offsetY, cx: this.cam.x, cy: this.cam.y, moved: false };
      if (this.ptrs.size === 2) {
        const [a, b] = [...this.ptrs.values()];
        this.pinch = { d: Math.hypot(a[0] - b[0], a[1] - b[1]), k: this.cam.k };
        if (this.down) this.down.moved = true;
      }
    });
    on(cv, "pointermove", (e) => {
      this.pointer = [e.offsetX, e.offsetY];
      if (this.ptrs.has(e.pointerId)) this.ptrs.set(e.pointerId, [e.offsetX, e.offsetY]);
      if (this.ptrs.size === 2 && this.pinch) {
        const [a, b] = [...this.ptrs.values()];
        this.target.k = this.cam.k = clampK((this.pinch.k * Math.hypot(a[0] - b[0], a[1] - b[1])) / this.pinch.d);
        this.kick();
        return;
      }
      if (this.down && this.ptrs.size === 1) {
        const dx = e.offsetX - this.down.x, dy = e.offsetY - this.down.y;
        if (Math.hypot(dx, dy) > 4) {
          this.down.moved = true;
          cv.classList.add("drag");
        }
        if (this.down.moved) {
          this.target.x = this.cam.x = this.down.cx - dx / this.cam.k;
          this.target.y = this.cam.y = this.down.cy - dy / this.cam.k;
          this.kick();
        }
        return;
      }
      const h = this.hitTest(e.offsetX, e.offsetY);
      const hov = h && h.kind !== "galaxy" ? h : null;
      cv.style.cursor = h ? "pointer" : "grab";
      if (hov !== this.hovered) {
        this.hovered = hov;
        this.kick();
      }
    });
    const up = (e: PointerEvent) => {
      this.ptrs.delete(e.pointerId);
      cv.classList.remove("drag");
      if (this.ptrs.size < 2) this.pinch = null;
      if (this.down && !this.down.moved && this.ptrs.size === 0) {
        const h = this.hitTest(e.offsetX, e.offsetY);
        if (h) this.select(h.id, true);
      }
      if (!this.ptrs.size) this.down = null;
    };
    on(cv, "pointerup", up);
    on(cv, "pointercancel", up);
    on(cv, "pointerleave", () => {
      this.pointer = null;
      if (this.hovered) {
        this.hovered = null;
        this.kick();
      }
    });
    on(cv, "wheel", (e) => {
      e.preventDefault();
      this.flight = null;
      const [wx, wy] = this.toW(e.offsetX, e.offsetY), nk = clampK(this.target.k * Math.exp(-e.deltaY * 0.0016));
      this.target.k = nk;
      this.target.x = wx - (e.offsetX - this.W / 2) / nk;
      this.target.y = wy - (e.offsetY - this.H / 2) / nk;
      this.kick();
    }, { passive: false });
  }
}

export type { GNode, CNode, SNode, PNode, SkyNode };
