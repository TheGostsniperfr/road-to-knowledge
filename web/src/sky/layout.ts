import type { GalaxyView, Pin, State, StarSummary, UniverseView } from "../api";
import { TAU, clamp, hash, mkRng, smooth } from "./rand";

export interface GNode {
  kind: "galaxy";
  id: string;
  name: string;
  hue: number;
  shape: string;
  x: number;
  y: number;
  R: number;
  m: number;
  e: number;
  kap: number;
  bar: number;
  tilt: number;
  ct: number;
  st: number;
  cp: number;
  sp: number;
  zt: number;
  mass: number;
  seed: number;
  clusters: CNode[];
  data: GalaxyView;
}

export interface CNode {
  kind: "cluster";
  id: string;
  name: string;
  hue: number;
  x: number;
  y: number;
  r: number;
  mass: number;
  seed: number;
  g: GNode;
  stars: SNode[];
}

export interface SNode {
  kind: "star";
  id: string;
  name: string;
  hue: number;
  x: number;
  y: number;
  u: number;
  level: number;
  state: State;
  stale: boolean;
  seed: number;
  c: CNode;
  planets: PNode[];
  links: SNode[];
  data: StarSummary;
  rs: number;
}

export interface PNode {
  kind: "planet";
  id: string;
  name: string;
  hue: number;
  i: number;
  a0: number;
  star: SNode;
  x: number;
  y: number;
  sx: number;
  sy: number;
  rs: number;
}

export type SkyNode = GNode | CNode | SNode | PNode;

export interface Layout {
  galaxies: GNode[];
  clusters: CNode[];
  stars: SNode[];
  byId: Map<string, SkyNode>;
}

export function orbitE(g: GNode, a: number) {
  return g.bar ? 0.8 + (g.e - 0.8) * smooth(g.bar - 0.04, g.bar + 0.1, a) : g.e;
}
export function orbitTheta(g: GNode, a: number) {
  return g.kap * Math.max(0, a - g.bar);
}
// Density-wave crowding: orbits r = a(1 - e/2 (1 - cos m·φ)) rotated by θ(a) bunch up where dr/da is minimal.
export function armPhase(g: GNode, a: number) {
  if (a < g.bar) return 0;
  const e = orbitE(g, a), B = e / 2, C = a * B * g.m * g.kap;
  return Math.atan2(-C, -B) / g.m;
}
export function orbitR(g: GNode, a: number, fr: number, e: number) {
  return a * (1 - (e / 2) * (1 - Math.cos(g.m * fr)));
}

const starMass = (s: StarSummary) => s.level || 0.3;

export function parent(n: SkyNode): SkyNode | null {
  switch (n.kind) {
    case "galaxy": return null;
    case "cluster": return n.g;
    case "star": return n.c;
    case "planet": return n.star;
  }
}

export function pathOf(n: SkyNode): SkyNode[] {
  const p: SkyNode[] = [];
  for (let c: SkyNode | null = n; c; c = parent(c)) p.unshift(c);
  return p;
}

export function isSpiral(g: GNode) {
  return g.shape === "spiral" || g.shape === "barred";
}

export function buildLayout(u: UniverseView): Layout {
  const byId = new Map<string, SkyNode>();
  const galaxies: GNode[] = [];
  const clusters: CNode[] = [];
  const stars: SNode[] = [];
  const masses = u.galaxies.map((g) => g.clusters.reduce((a, c) => a + c.stars.reduce((b, s) => b + starMass(s), 0), 0));
  const maxMass = Math.max(1, ...masses);

  u.galaxies.forEach((gv, gi) => {
    const R = mkRng(hash(gv.id));
    const mass = masses[gi];
    const ns = gv.clusters.length;
    const g: GNode = {
      kind: "galaxy", id: gv.id, name: gv.name, hue: gv.hue, shape: gv.shape, x: 0, y: 0,
      R: 620 + 880 * Math.sqrt(mass / maxMass), m: 2, e: 0, kap: 0, bar: 0,
      tilt: 0.5 + R() * 0.28, ct: 0, st: 0, cp: 0, sp: 0, zt: 0, mass, seed: hash(gv.id), clusters: [], data: gv,
    };
    const rolled = R() * TAU, th = gv.pin?.rotation ?? rolled, phi = (R() - 0.5) * 1.2;
    // Real spirals: barred and grand-design ones have 2 arms, a few have 3 or 4. Sub-themes spread along the arms.
    if (gv.shape === "barred") Object.assign(g, { m: 2, e: 0.46, kap: 5.4, bar: 0.27 });
    if (gv.shape === "spiral") {
      g.m = ns <= 4 ? 2 : ns <= 8 ? 3 : 4;
      Object.assign(g, g.m > 2 ? { e: 0.32, kap: 4.6 } : { e: 0.42, kap: 5.6 });
    }
    g.ct = Math.cos(th); g.st = Math.sin(th); g.cp = Math.cos(phi); g.sp = Math.sin(phi); g.zt = Math.sqrt(1 - g.tilt * g.tilt);
    galaxies.push(g);
    byId.set(g.id, g);
  });

  placeGalaxies(galaxies);

  for (const g of galaxies) {
    const R = mkRng(g.seed ^ 0x9e3779b9);
    const loc = (lx: number, ly: number) => {
      const x = lx * g.ct - ly * g.st, y = (lx * g.st + ly * g.ct) * g.tilt;
      return [x * g.cp - y * g.sp, x * g.sp + y * g.cp];
    };
    const ns = g.data.clusters.length;
    const spiral = isSpiral(g);
    g.data.clusters.forEach((cv, j) => {
      const known = cv.stars.filter((s) => s.level > 0);
      const avg = known.length ? known.reduce((a, s) => a + s.level, 0) / cv.stars.length : 0;
      let lx: number, ly: number;
      if (g.shape === "ring") {
        const a = (j / ns) * TAU + 0.6;
        lx = Math.cos(a) * 0.68; ly = Math.sin(a) * 0.68;
      } else if (spiral) {
        const per = Math.ceil(ns / g.m), slot = Math.floor(j / g.m);
        const a = clamp(g.bar + 0.1 + (0.86 - g.bar - 0.1) * (slot + 0.5) / per + (R() - 0.5) * 0.04, 0.3, 0.88);
        const fr = armPhase(g, a) + (j % g.m) * TAU / g.m, psi = orbitTheta(g, a) + fr, r = orbitR(g, a, fr, orbitE(g, a));
        lx = Math.cos(psi) * r; ly = Math.sin(psi) * r;
      } else {
        const a = (j / Math.max(1, ns)) * TAU + R() * 0.8, rr = ns === 1 ? 0.18 : 0.25 + 0.35 * (1 - avg / 5);
        lx = Math.cos(a) * rr; ly = Math.sin(a) * rr;
      }
      const [px, py] = loc(lx, ly);
      const Rc = Math.max(70, g.R * 0.085);
      const c: CNode = {
        kind: "cluster", id: cv.id, name: cv.name, hue: g.hue, x: g.x + px * g.R, y: g.y + py * g.R, r: Rc,
        mass: cv.stars.reduce((a, s) => a + starMass(s), 0), seed: hash(cv.id), g, stars: [],
      };
      cv.stars.forEach((sv, i) => {
        const sr = mkRng(hash(sv.id));
        const ang = i * 2.39996 + sr() * 0.5, d = Rc * (0.14 + 0.74 * (5 - sv.level) / 5) + (sr() - 0.5) * Rc * 0.06;
        const s: SNode = {
          kind: "star", id: sv.id, name: sv.name, hue: g.hue + (sr() - 0.5) * 24,
          x: c.x + Math.cos(ang) * d, y: c.y + Math.sin(ang) * d * 0.85, u: Rc * 0.012,
          level: sv.level, state: sv.state, stale: sv.stale, seed: sr() * TAU, c, planets: [], links: [], data: sv, rs: 4,
        };
        (sv.planets ?? []).forEach((p, k) => {
          const pn: PNode = { kind: "planet", id: p.id, name: p.name, hue: (g.hue + 60 * k + sr() * 30) % 360, i: k, a0: sr() * TAU, star: s, x: s.x, y: s.y, sx: 0, sy: 0, rs: 4 };
          s.planets.push(pn);
          byId.set(pn.id, pn);
        });
        c.stars.push(s);
        stars.push(s);
        byId.set(s.id, s);
      });
      g.clusters.push(c);
      clusters.push(c);
      byId.set(c.id, c);
    });
    separateClusters(g.clusters);
  }

  for (const s of stars) {
    s.links = (s.data.links ?? []).map((id) => byId.get(id)).filter((n): n is SNode => n?.kind === "star");
  }
  return { galaxies, clusters, stars, byId };
}

// Biggest theme in the middle, the others around it on a golden-angle spiral, then pushed apart.
// Pinned galaxies (layout.json) never move: only the new ones are placed and pushed away from them.
function placeGalaxies(gs: GNode[]) {
  const order = [...gs].sort((a, b) => b.mass - a.mass || a.id.localeCompare(b.id));
  const pinned = (g: GNode) => !!g.data.pin;
  order.forEach((g, i) => {
    if (g.data.pin) {
      g.x = g.data.pin.x;
      g.y = g.data.pin.y;
      return;
    }
    const a = i * 2.39996 + 0.7, r = i === 0 ? 0 : 1900 * Math.sqrt(i);
    g.x = Math.cos(a) * r;
    g.y = Math.sin(a) * r * 0.8;
  });
  const anyPinned = order.some(pinned);
  for (let it = 0; it < 80; it++) {
    for (let a = 0; a < order.length; a++) {
      for (let b = a + 1; b < order.length; b++) {
        const A = order[a], B = order[b], pa = pinned(A), pb = pinned(B);
        if (pa && pb) continue;
        const dx = B.x - A.x, dy = B.y - A.y, d = Math.hypot(dx, dy) || 1, min = (A.R + B.R) * 1.35;
        if (d < min) {
          const p = (min - d) / d / (pa || pb ? 1 : 2);
          if (!pa) { A.x -= dx * p; A.y -= dy * p; }
          if (!pb) { B.x += dx * p; B.y += dy * p; }
        }
      }
    }
    // Pulling free galaxies to the origin would drag them into pinned ones; with pins the spiral slot is enough.
    if (anyPinned) continue;
    for (const g of order.slice(1)) {
      g.x *= 0.985;
      g.y *= 0.985;
    }
  }
}

// Positions to write to layout.json so the map stays as it looks now.
// A rotation set by hand is carried over, so re-pinning never drops it.
export function currentPins(l: Layout): Record<string, Pin> {
  return Object.fromEntries(l.galaxies.map((g) => {
    const rotation = g.data.pin?.rotation;
    return [g.id, rotation === undefined ? { x: Math.round(g.x), y: Math.round(g.y) } : { x: Math.round(g.x), y: Math.round(g.y), rotation }];
  }));
}

function separateClusters(cls: CNode[]) {
  for (let it = 0; it < 40; it++) {
    for (let a = 0; a < cls.length; a++) {
      for (let b = a + 1; b < cls.length; b++) {
        const A = cls[a], B = cls[b];
        const dx = B.x - A.x, dy = B.y - A.y, d = Math.hypot(dx, dy) || 1, min = (A.r + B.r) * 1.25;
        if (d < min) {
          const p = (min - d) / 2 / d;
          A.x -= dx * p; A.y -= dy * p; B.x += dx * p; B.y += dy * p;
          for (const s of A.stars) { s.x -= dx * p; s.y -= dy * p; }
          for (const s of B.stars) { s.x += dx * p; s.y += dy * p; }
        }
      }
    }
  }
}

