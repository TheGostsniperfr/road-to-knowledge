import type { UniverseView } from "../api";
import { buildLayout, type Layout, type SNode, type SkyNode } from "../sky/layout";
import { buildAtlas, circleOf, hit, type Atlas, type Circle } from "./pack";

export interface AtlasEvents {
  onSelect(node: SkyNode | null): void;
  onScale?(name: string): void;
}

interface Cam { x: number; y: number; k: number }
interface Flight { t0: number; dur: number; at(t: number): Cam }
interface Palette { light: boolean; space: string; ink: string; muted: string; faint: string; accent: string; stale: string }

const SCALE: Record<Circle["kind"], string> = { root: "Vue d'ensemble", galaxy: "Thème", cluster: "Sous-thème", star: "Sujet" };
const FONT = '"Inter Variable", system-ui, sans-serif';
const RHO = 1.4;

export class AtlasView {
  private ctx: CanvasRenderingContext2D;
  private layout: Layout | null = null;
  private atlas: Atlas | null = null;
  private cam: Cam = { x: 500, y: 500, k: 1 };
  private flight: Flight | null = null;
  private selected: Circle | null = null;
  private hover: Circle | null = null;
  private focus: Set<Circle> | null = null;
  private mouse = { x: 0, y: 0 };
  private w = 0;
  private h = 0;
  private dpr = 1;
  private pal!: Palette;
  private frame = 0;
  private cleanup: (() => void)[] = [];
  private reduce = matchMedia("(prefers-reduced-motion: reduce)").matches;

  constructor(private canvas: HTMLCanvasElement, private ev: AtlasEvents) {
    this.ctx = canvas.getContext("2d")!;
    this.readPalette();
    this.bind();
    this.resize();
    document.fonts?.ready.then(() => this.kick());
  }

  setUniverse(u: UniverseView) {
    const first = !this.atlas;
    this.layout = buildLayout(u);
    this.atlas = buildAtlas(this.layout);
    const sel = this.selected ? this.atlas.byId.get(this.selected.id) ?? null : null;
    this.selected = sel;
    this.hover = null;
    if (first) { this.cam = this.fit(this.atlas.root); this.ev.onScale?.(SCALE.root); }
    this.kick();
  }

  select(id: string | null, fly = true) {
    if (!this.atlas || !this.layout) return;
    const node = id ? this.layout.byId.get(id) ?? null : null;
    const c = circleOf(this.atlas, node);
    this.selected = c === this.atlas.root ? null : c;
    const to = this.fit(c);
    if (fly && !this.reduce) this.flyTo(to);
    else { this.flight = null; this.cam = to; }
    this.ev.onSelect(node);
    this.ev.onScale?.(SCALE[c.kind]);
    this.kick();
  }

  zoomBy(f: number) {
    this.flight = null;
    this.cam = { ...this.cam, k: this.clampK(this.cam.k * f) };
    this.kick();
  }

  destroy() {
    cancelAnimationFrame(this.frame);
    this.cleanup.forEach((f) => f());
  }

  // --- camera ---

  private fit(c: Circle): Cam {
    const view = Math.min(this.w, this.h) || 800;
    if (c.kind !== "star" || !c.parent) return { x: c.x, y: c.y, k: (view * 0.9) / (2 * c.r) };
    // A subject is shown inside its sub-theme so its neighbours stay readable, unless the
    // sub-theme is so crowded that the subject would be too small to label.
    const p = c.parent;
    const kParent = (view * 0.9) / (2 * p.r);
    if (c.r * kParent >= 42) return { x: p.x, y: p.y, k: kParent };
    return { x: c.x, y: c.y, k: (view * 0.9) / (2 * c.r * 3.2) };
  }

  private clampK(k: number) {
    if (!this.atlas) return k;
    const view = Math.min(this.w, this.h) || 800;
    const min = (view * 0.5) / (2 * this.atlas.root.r);
    return Math.min(Math.max(k, min), min * 400);
  }

  // van Wijk and Nuij, "Smooth and efficient zooming and panning": the camera rises before it travels.
  private flyTo(to: Cam) {
    const view = Math.min(this.w, this.h) || 800;
    const from = { ...this.cam };
    const w0 = view / from.k, w1 = view / to.k;
    const dx = to.x - from.x, dy = to.y - from.y, d2 = dx * dx + dy * dy, d1 = Math.sqrt(d2);
    let at: (t: number) => Cam, S: number;
    if (d1 < 1e-6) {
      S = Math.log(w1 / w0) / RHO;
      at = (t) => ({ x: from.x + t * dx, y: from.y + t * dy, k: view / (w0 * Math.exp(RHO * t * S)) });
    } else {
      const b0 = (w1 * w1 - w0 * w0 + RHO ** 4 * d2) / (2 * w0 * RHO ** 2 * d1);
      const b1 = (w1 * w1 - w0 * w0 - RHO ** 4 * d2) / (2 * w1 * RHO ** 2 * d1);
      const r0 = Math.log(Math.sqrt(b0 * b0 + 1) - b0), r1 = Math.log(Math.sqrt(b1 * b1 + 1) - b1);
      S = (r1 - r0) / RHO;
      at = (t) => {
        const s = t * S, c0 = Math.cosh(r0);
        const u = (w0 / (RHO ** 2 * d1)) * (c0 * Math.tanh(RHO * s + r0) - Math.sinh(r0));
        return { x: from.x + u * dx, y: from.y + u * dy, k: view / ((w0 * c0) / Math.cosh(RHO * s + r0)) };
      };
    }
    const dur = Math.min(1100, Math.max(380, Math.abs(S) * 520));
    this.flight = { t0: performance.now(), dur, at };
  }

  private toScreen(x: number, y: number): [number, number] {
    return [(x - this.cam.x) * this.cam.k + this.w / 2, (y - this.cam.y) * this.cam.k + this.h / 2];
  }

  private toWorld(sx: number, sy: number): [number, number] {
    return [(sx - this.w / 2) / this.cam.k + this.cam.x, (sy - this.h / 2) / this.cam.k + this.cam.y];
  }

  // --- input ---

  private bind() {
    const c = this.canvas;
    let down: { x: number; y: number; cx: number; cy: number; moved: boolean } | null = null;
    const on = <K extends keyof HTMLElementEventMap>(t: EventTarget, type: K | string, f: (e: never) => void, o?: AddEventListenerOptions) => {
      t.addEventListener(type, f as EventListener, o);
      this.cleanup.push(() => t.removeEventListener(type, f as EventListener));
    };
    on(c, "pointerdown", (e: PointerEvent) => {
      c.setPointerCapture(e.pointerId);
      down = { x: e.offsetX, y: e.offsetY, cx: this.cam.x, cy: this.cam.y, moved: false };
    });
    on(c, "pointermove", (e: PointerEvent) => {
      this.mouse = { x: e.offsetX, y: e.offsetY };
      if (down) {
        const dx = e.offsetX - down.x, dy = e.offsetY - down.y;
        if (Math.abs(dx) + Math.abs(dy) > 4) down.moved = true;
        if (down.moved) {
          this.flight = null;
          this.cam = { ...this.cam, x: down.cx - dx / this.cam.k, y: down.cy - dy / this.cam.k };
          c.classList.add("drag");
        }
      }
      this.updateHover();
      this.kick();
    });
    on(c, "pointerup", (e: PointerEvent) => {
      c.classList.remove("drag");
      if (down && !down.moved && this.atlas) {
        const t = hit(this.atlas.root, ...this.toWorld(e.offsetX, e.offsetY));
        this.select(t === this.atlas.root ? null : t.id);
      }
      down = null;
    });
    on(c, "pointerleave", () => { this.hover = null; this.kick(); });
    on(c, "wheel", (e: WheelEvent) => {
      e.preventDefault();
      this.flight = null;
      const [wx, wy] = this.toWorld(e.offsetX, e.offsetY);
      const k = this.clampK(this.cam.k * Math.exp(-e.deltaY * (e.ctrlKey ? 0.01 : 0.0018)));
      this.cam = { k, x: wx - (e.offsetX - this.w / 2) / k, y: wy - (e.offsetY - this.h / 2) / k };
      this.kick();
    }, { passive: false });
    on(window, "keydown", (e: KeyboardEvent) => {
      if (e.key !== "Escape" || !this.selected || (e.target as HTMLElement)?.closest?.("input,textarea")) return;
      const p = this.selected.parent;
      this.select(p && p.kind !== "root" ? p.id : null);
    });
    const ro = new ResizeObserver(() => this.resize());
    ro.observe(c);
    this.cleanup.push(() => ro.disconnect());
    const mo = new MutationObserver(() => { this.readPalette(); this.kick(); });
    mo.observe(document.documentElement, { attributes: true, attributeFilter: ["data-theme"] });
    this.cleanup.push(() => mo.disconnect());
  }

  private updateHover() {
    if (!this.atlas) return;
    const t = hit(this.atlas.root, ...this.toWorld(this.mouse.x, this.mouse.y));
    this.hover = t === this.atlas.root ? null : t;
    this.canvas.style.cursor = this.hover ? "pointer" : "default";
  }

  private resize() {
    this.dpr = Math.min(window.devicePixelRatio || 1, 2);
    this.w = this.canvas.clientWidth;
    this.h = this.canvas.clientHeight;
    this.canvas.width = Math.round(this.w * this.dpr);
    this.canvas.height = Math.round(this.h * this.dpr);
    if (this.atlas && !this.flight) this.cam = this.fit(this.selected ?? this.atlas.root);
    this.kick();
  }

  private readPalette() {
    const cs = getComputedStyle(document.documentElement);
    const v = (n: string) => cs.getPropertyValue(n).trim();
    this.pal = { light: v("color-scheme") === "light", space: v("--space"), ink: v("--ink-strong"), muted: v("--muted"), faint: v("--faint"), accent: v("--accent"), stale: v("--stale") };
  }

  // --- drawing ---

  private kick() {
    if (!this.frame) this.frame = requestAnimationFrame(() => { this.frame = 0; this.draw(); });
  }

  private draw() {
    if (this.flight) {
      const t = Math.min(1, (performance.now() - this.flight.t0) / this.flight.dur);
      const e = t < 0.5 ? 4 * t * t * t : 1 - (-2 * t + 2) ** 3 / 2;
      this.cam = this.flight.at(e);
      if (t >= 1) this.flight = null;
      this.kick();
    }
    const { ctx, pal } = this;
    ctx.setTransform(this.dpr, 0, 0, this.dpr, 0, 0);
    ctx.fillStyle = pal.space;
    ctx.fillRect(0, 0, this.w, this.h);
    this.drawGrid();
    if (!this.atlas) return;

    const focus = this.focusSet();
    this.focus = focus;
    // Containers, then quiet links, then subjects, then the focused links: lines never cross a subject at rest.
    for (const g of this.atlas.root.children) this.drawCircle(g, focus, false);
    this.drawEdges(focus, false);
    for (const g of this.atlas.root.children) this.drawCircle(g, focus, true);
    this.drawEdges(focus, true);
    for (const g of this.atlas.root.children) this.drawLabels(g);
    this.drawTooltip();
  }

  private drawGrid() {
    const { ctx } = this;
    const step = 24;
    const ox = ((-this.cam.x * this.cam.k + this.w / 2) % step + step) % step;
    const oy = ((-this.cam.y * this.cam.k + this.h / 2) % step + step) % step;
    ctx.fillStyle = this.pal.faint;
    ctx.globalAlpha = this.pal.light ? 0.35 : 0.28;
    for (let x = ox; x < this.w; x += step) for (let y = oy; y < this.h; y += step) ctx.fillRect(x - 0.5, y - 0.5, 1, 1);
    ctx.globalAlpha = 1;
  }

  // Stars that stay at full strength: the focused subject and its links, or everything when nothing is focused.
  private focusSet(): Set<Circle> | null {
    const f = this.hover?.kind === "star" ? this.hover : this.selected?.kind === "star" ? this.selected : null;
    if (!f || !this.atlas) return null;
    const s = new Set<Circle>([f]);
    for (const l of (f.node as SNode).links) { const c = this.atlas.byId.get(l.id); if (c) s.add(c); }
    return s.size > 1 ? s : null;
  }

  private visible(sx: number, sy: number, R: number) {
    return R > 0.4 && sx + R > 0 && sy + R > 0 && sx - R < this.w && sy - R < this.h;
  }

  private colors(c: Circle) {
    const h = c.hue, L = this.pal.light;
    switch (c.kind) {
      case "galaxy": return { fill: L ? `hsl(${h} 30% 98.6%)` : `hsl(${h} 10% 10.5%)`, line: L ? `hsl(${h} 22% 86%)` : `hsl(${h} 12% 20%)` };
      case "cluster": return { fill: L ? `hsl(${h} 42% 95.5%)` : `hsl(${h} 14% 13.5%)`, line: L ? `hsl(${h} 26% 85%)` : `hsl(${h} 14% 22%)` };
      default: {
        const lv = (c.node as SNode).level;
        if (!lv) return { fill: L ? `hsl(${h} 30% 97.5%)` : `hsl(${h} 10% 12%)`, line: L ? `hsl(${h} 22% 66%)` : `hsl(${h} 12% 40%)` };
        const light = L ? 86 - lv * 7 : 24 + lv * 7;
        return { fill: `hsl(${h} ${L ? 62 : 52}% ${light}%)`, line: "", light };
      }
    }
  }

  private drawCircle(c: Circle, focus: Set<Circle> | null, stars: boolean) {
    const [sx, sy] = this.toScreen(c.x, c.y), R = c.r * this.cam.k;
    if (!this.visible(sx, sy, R)) return;
    if (c.kind !== "star" && stars) {
      if (R > 2.5) for (const k of c.children) this.drawCircle(k, focus, true);
      if (c === this.selected) { this.ring(sx, sy, R + 3, this.pal.accent, 6, 0.16); this.ring(sx, sy, R + 1, this.pal.accent, 2, 1); }
      return;
    }
    if (c.kind === "star" && !stars) return;
    const { ctx } = this;
    const col = this.colors(c);
    const fog = c.kind === "star" && !(c.node as SNode).level;
    ctx.globalAlpha = c.kind === "star" && focus && !focus.has(c) ? 0.38 : 1;
    ctx.beginPath();
    ctx.arc(sx, sy, R, 0, Math.PI * 2);
    ctx.fillStyle = col.fill;
    ctx.fill();
    if (col.line) {
      ctx.lineWidth = 1;
      ctx.strokeStyle = col.line;
      if (fog) ctx.setLineDash([3, 3]);
      ctx.stroke();
      ctx.setLineDash([]);
    }
    if (c.kind === "star" && (c.node as SNode).stale && R > 5) {
      ctx.beginPath();
      ctx.arc(sx + R * 0.7, sy - R * 0.7, Math.max(2.5, R * 0.09), 0, Math.PI * 2);
      ctx.fillStyle = this.pal.stale;
      ctx.fill();
    }
    ctx.globalAlpha = 1;
    if (c === this.hover && c !== this.selected) this.ring(sx, sy, R, this.pal.ink, 1.25, 0.45);
    if (R > 2.5 && c.kind !== "star") for (const k of c.children) this.drawCircle(k, focus, false);
    if (c.kind === "star" && c === this.selected) {
      this.ring(sx, sy, R + 3, this.pal.accent, 6, 0.16);
      this.ring(sx, sy, R + 1, this.pal.accent, 2, 1);
    }
  }

  private ring(x: number, y: number, R: number, color: string, width: number, alpha: number) {
    const { ctx } = this;
    ctx.globalAlpha = alpha;
    ctx.beginPath();
    ctx.arc(x, y, R, 0, Math.PI * 2);
    ctx.lineWidth = width;
    ctx.strokeStyle = color;
    ctx.stroke();
    ctx.globalAlpha = 1;
  }

  private drawEdges(focus: Set<Circle> | null, hotPass: boolean) {
    const { ctx } = this;
    const f = this.hover?.kind === "star" ? this.hover : this.selected?.kind === "star" ? this.selected : null;
    ctx.lineCap = "round";
    for (const e of this.atlas!.edges) {
      const hot = f && (e.a === f || e.b === f);
      if ((focus && !hot) || Boolean(hot) !== hotPass) continue;
      const [ax, ay] = this.toScreen(e.a.x, e.a.y), [bx, by] = this.toScreen(e.b.x, e.b.y), [cx, cy] = this.toScreen(e.cx, e.cy);
      const ra = e.a.r * this.cam.k, rb = e.b.r * this.cam.k;
      // Start and end on the rims, not the centres, so the curve never sits on top of a label.
      const [a0, a1] = this.toward(ax, ay, cx, cy, ra), [b0, b1] = this.toward(bx, by, cx, cy, rb);
      ctx.beginPath();
      ctx.moveTo(a0, a1);
      ctx.quadraticCurveTo(cx, cy, b0, b1);
      ctx.strokeStyle = hot ? this.pal.accent : this.pal.ink;
      ctx.globalAlpha = hot ? 0.9 : this.pal.light ? 0.09 : 0.14;
      ctx.lineWidth = hot ? 1.6 : 1;
      ctx.stroke();
      if (hot) for (const [x, y] of [[a0, a1], [b0, b1]]) { ctx.beginPath(); ctx.arc(x, y, 2.4, 0, Math.PI * 2); ctx.fillStyle = this.pal.accent; ctx.fill(); }
    }
    ctx.globalAlpha = 1;
  }

  private toward(x: number, y: number, tx: number, ty: number, r: number): [number, number] {
    const dx = tx - x, dy = ty - y, d = Math.hypot(dx, dy) || 1;
    return [x + (dx / d) * r, y + (dy / d) * r];
  }

  private drawLabels(c: Circle) {
    const [sx, sy] = this.toScreen(c.x, c.y), R = c.r * this.cam.k;
    if (!this.visible(sx, sy, R)) return;
    const kidsBig = c.children.some((k) => k.r * this.cam.k >= (c.kind === "galaxy" ? 34 : 22));
    if (c.kind === "galaxy") {
      // Theme names sit above their circle, like a caption, so they never cover a sub-theme.
      if (R >= 30 && sy - R - 10 > 8) this.caption(c.name, R > 90 ? this.count(c) : "", sx, sy - R - 10, Math.min(19, Math.max(13, R * 0.05)));
    } else if (R >= 24) {
      // Sub-theme names sit on the top rim with a halo when their subjects are labelled,
      // and in the middle when the subjects are still dots.
      if (kidsBig) { if (sy - R > 8) this.text(c.name, sx, sy - R + 4, Math.min(12.5, Math.max(10.5, R * 0.05)), 600, this.pal.muted, R * 1.4, true); }
      else this.text(c.name, sx, sy + 4, Math.min(15, Math.max(10.5, R * 0.15)), 600, this.pal.ink, R * 2.2, true);
    }
    if (c.kind === "galaxy") for (const k of c.children) this.drawLabels(k);
    else if (kidsBig) for (const k of c.children) {
      const [kx, ky] = this.toScreen(k.x, k.y), KR = k.r * this.cam.k;
      if (this.visible(kx, ky, KR)) this.starLabel(k, kx, ky, KR);
    }
  }

  private caption(name: string, meta: string, x: number, y: number, size: number) {
    const { ctx } = this;
    ctx.font = `650 ${size}px ${FONT}`;
    const wn = ctx.measureText(name).width;
    ctx.font = `450 ${size * 0.72}px ${FONT}`;
    const wm = meta ? ctx.measureText(meta).width + 8 : 0;
    const left = x - (wn + wm) / 2;
    this.text(name, left + wn / 2, y, size, 650, this.pal.ink, 1e4);
    if (meta) this.text(meta, left + wn + 8 + (wm - 8) / 2, y, size * 0.72, 450, this.pal.muted, 1e4);
  }

  private starLabel(c: Circle, sx: number, sy: number, R: number) {
    if (R < 13) return;
    const s = c.node as SNode;
    const col = this.colors(c) as { light?: number };
    const dimmed = this.focus && !this.focus.has(c);
    const dark = col.light === undefined || dimmed ? this.pal.ink : col.light > 58 ? "#15171b" : "#ffffff";
    const sub = col.light === undefined || dimmed ? this.pal.muted : dark;
    const size = Math.min(15, Math.max(10, R * 0.28));
    const lines = R >= 34 ? 1 : 0;
    const notions = R >= 110 ? s.planets.slice(0, 5) : [];
    const blockH = size + lines * 15 + notions.length * 15;
    let y = sy - blockH / 2 + size * 0.8;
    this.ctx.font = `600 ${size}px ${FONT}`;
    if (R < 30 && this.ctx.measureText(c.name).width > R * 1.8) return;
    this.text(c.name, sx, y, size, 600, dark, R * 1.7);
    if (lines) {
      y += 15;
      this.text(s.level ? `Niveau ${s.level}${s.stale ? " · à revoir" : ""}` : "À explorer", sx, y, 10.5, 450, sub, R * 1.6, false, 0.78);
    }
    for (const p of notions) { y += 15; this.text(p.name, sx, y, 11, 450, sub, R * 1.5, false, 0.9); }
  }

  private count(c: Circle) {
    let n = 0;
    for (const k of c.children) n += k.children.length;
    return `${n} sujet${n > 1 ? "s" : ""}`;
  }

  private text(s: string, x: number, y: number, size: number, weight: number, color: string, maxW: number, halo = false, alpha = 1) {
    const { ctx } = this;
    ctx.font = `${weight} ${size}px ${FONT}`;
    ctx.textAlign = "center";
    ctx.textBaseline = "alphabetic";
    let t = s;
    if (ctx.measureText(t).width > maxW) {
      while (t.length > 1 && ctx.measureText(t + "…").width > maxW) t = t.slice(0, -1);
      t = t.trimEnd() + "…";
    }
    ctx.globalAlpha = alpha;
    if (halo) {
      ctx.lineWidth = 3.5;
      ctx.lineJoin = "round";
      ctx.strokeStyle = this.pal.space;
      ctx.globalAlpha = alpha * 0.85;
      ctx.strokeText(t, x, y);
      ctx.globalAlpha = alpha;
    }
    ctx.fillStyle = color;
    ctx.fillText(t, x, y);
    ctx.globalAlpha = 1;
  }

  private drawTooltip() {
    const c = this.hover;
    if (!c || c.r * this.cam.k >= (c.kind === "star" ? 34 : 1e9)) return;
    const { ctx } = this;
    const s = c.kind === "star" ? (c.node as SNode) : null;
    const line2 = s ? (s.level ? `Niveau ${s.level}` : "À explorer") : this.kindLabel(c);
    ctx.font = `600 12.5px ${FONT}`;
    const w1 = ctx.measureText(c.name).width;
    ctx.font = `450 11px ${FONT}`;
    const w = Math.max(w1, ctx.measureText(line2).width) + 20, h = 42;
    let x = this.mouse.x + 14, y = this.mouse.y + 14;
    if (x + w > this.w - 6) x = this.mouse.x - w - 10;
    if (y + h > this.h - 6) y = this.mouse.y - h - 10;
    ctx.fillStyle = this.pal.light ? "#ffffff" : "#1d1e21";
    ctx.strokeStyle = this.pal.light ? "rgba(0,0,0,.1)" : "rgba(255,255,255,.1)";
    ctx.shadowColor = "rgba(0,0,0,.12)";
    ctx.shadowBlur = 12;
    ctx.shadowOffsetY = 3;
    ctx.beginPath();
    ctx.roundRect(x, y, w, h, 7);
    ctx.fill();
    ctx.shadowColor = "transparent";
    ctx.stroke();
    ctx.textAlign = "left";
    ctx.font = `600 12.5px ${FONT}`;
    ctx.fillStyle = this.pal.ink;
    ctx.fillText(c.name, x + 10, y + 17);
    ctx.font = `450 11px ${FONT}`;
    ctx.fillStyle = this.pal.muted;
    ctx.fillText(line2, x + 10, y + 33);
  }

  private kindLabel(c: Circle) {
    return c.kind === "galaxy" ? `Thème · ${this.count(c)}` : `Sous-thème · ${c.children.length} sujet${c.children.length > 1 ? "s" : ""}`;
  }
}
