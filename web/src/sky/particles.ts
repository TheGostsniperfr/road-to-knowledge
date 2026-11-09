import type { CNode, GNode, Layout } from "./layout";
import { armPhase, orbitE, orbitTheta } from "./layout";
import { TAU, clamp, gauss, mkRng, type Rng } from "./rand";

// Per particle: a, θ, e, φ0 | ω, dr, z, slot | r, g, b, alpha | size. The vertex shader moves them along their orbit.
export const STRIDE = 13;

export interface Buf { d: number[]; n: number }
export interface Particles { stars: Buf; glow: Buf; dust: Buf; hii: Buf }
export type Slot = { kind: "g"; g: GNode; i: number } | { kind: "c"; c: CNode; gi: number };
export const MAX_SLOTS = 2048;

type RGB = [number, number, number];

// Blackbody colour (Tanner Helland fit), so star colours follow their temperature.
export function kelvin(T: number): RGB {
  T /= 100;
  let r: number, g: number, b: number;
  if (T <= 66) {
    r = 255; g = 99.47 * Math.log(T) - 161.12; b = T <= 19 ? 0 : 138.52 * Math.log(T - 10) - 305.04;
  } else {
    r = 329.7 * Math.pow(T - 60, -0.1332); g = 288.12 * Math.pow(T - 60, -0.0755); b = 255;
  }
  return [clamp(r, 0, 255) / 255, clamp(g, 0, 255) / 255, clamp(b, 0, 255) / 255];
}

export function hsl(h: number, s: number, l: number): RGB {
  h = (((h % 360) + 360) % 360) / 360;
  const f = (n: number) => {
    const k = (n + h * 12) % 12, a = s * Math.min(l, 1 - l);
    return l - a * Math.max(-1, Math.min(k - 3, 9 - k, 1));
  };
  return [f(0), f(8), f(4)];
}

const mix = (a: RGB, b: RGB, t: number): RGB => [a[0] + (b[0] - a[0]) * t, a[1] + (b[1] - a[1]) * t, a[2] + (b[2] - a[2]) * t];
const omega = (a: number) => 0.022 / (a + 0.12);

function push(B: Buf, a: number, th: number, e: number, f0: number, w: number, dr: number, z: number, slot: number, c: RGB, al: number, size: number) {
  B.d.push(a, th, e, f0, w, dr, z, slot, c[0], c[1], c[2], al, size);
  B.n++;
}

export function synthesize(layout: Layout): { particles: Particles; slots: Slot[] } {
  const P: Particles = { stars: { d: [], n: 0 }, glow: { d: [], n: 0 }, dust: { d: [], n: 0 }, hii: { d: [], n: 0 } };
  const slots: Slot[] = [];
  const maxMass = Math.max(1, ...layout.galaxies.map((g) => g.mass));
  layout.galaxies.forEach((g, i) => {
    if (slots.length >= MAX_SLOTS) return;
    slots.push({ kind: "g", g, i });
    synthGalaxy(P, g, slots.length - 1, maxMass);
  });
  layout.clusters.forEach((c) => {
    if (slots.length >= MAX_SLOTS) return;
    slots.push({ kind: "c", c, gi: layout.galaxies.indexOf(c.g) });
    synthCluster(P, c, slots.length - 1);
  });
  return { particles: P, slots };
}

function synthGalaxy(P: Particles, g: GNode, slot: number, maxMass: number) {
  const R: Rng = mkRng(g.seed ^ 0x51ed270b);
  const G = () => gauss(R);
  // Light follows knowledge: a galaxy with few subjects stays dim, only its sub-themes shine.
  const gb = 0.4 + 0.45 * Math.sqrt(g.mass / maxMass);
  const tint = hsl(g.hue, 0.8, 0.62);
  const sp = g.shape === "spiral" || g.shape === "barred";
  const n = Math.round(14000 + 38000 * Math.sqrt(g.mass / maxMass));
  const star = (a: number, fr: number, dr: number, z: number, T: number, al: number, size: number, w: number, th: number, e: number) =>
    push(P.stars, a, th, e, fr, w, dr, z, slot, mix(kelvin(T), tint, 0.32), al * gb, size);
  const bulge = g.shape === "elliptical" ? 0.62 : g.shape === "lenticular" ? 0.32 : g.shape === "ring" ? 0.2 : g.shape === "irregular" ? 0.04 : 0.16;
  const clumps = g.shape === "irregular" ? Array.from({ length: 7 }, () => [G() * 0.32, G() * 0.26, 0.05 + R() * 0.13]) : [];

  for (let i = 0; i < n; i++) {
    if (R() < bulge) {
      const a = Math.min(1.05, (g.shape === "elliptical" ? 0.11 : 0.065) * Math.pow(-Math.log(R() * R() + 1e-9), 1.5));
      const e = g.shape === "elliptical" ? 0.28 : sp ? orbitE(g, a) * 0.5 : 0;
      star(a, R() * TAU, G() * 0.008, G() * a * 0.55, 3300 + R() * 2200, 0.45 + R() * 0.4, 0.9 + R() * 0.6, omega(a) * 0.6, 0, e);
      continue;
    }
    if (g.shape === "ring") {
      if (R() < 0.7) {
        const a = 0.68 + G() * 0.035, young = R() < 0.5;
        star(a, R() * TAU, G() * 0.01, G() * 0.01, young ? 9000 + R() * 16000 : 4500 + R() * 2000, 0.5 + R() * 0.4, young ? 1.2 + R() * 1.1 : 1, omega(a) * 0.4, 0, 0.04);
      } else {
        const a = Math.min(1, 0.12 + Math.abs(G()) * 0.35);
        star(a, R() * TAU, G() * 0.02, G() * 0.01, 4200 + R() * 2000, 0.25 + R() * 0.3, 1, omega(a) * 0.4, 0, 0);
      }
      continue;
    }
    if (g.shape === "irregular") {
      const k = clumps[i % clumps.length], x = k[0] + G() * k[2], y = k[1] + G() * k[2];
      star(Math.hypot(x, y), Math.atan2(y, x), 0, G() * 0.02, 4200 + R() * 2400, 0.45 + R() * 0.4, 1, 0.004, 0, 0);
      continue;
    }
    let a: number;
    do a = -0.29 * Math.log(R() * R() + 1e-9); while (a > 1.12);
    if (!sp) {
      star(a, R() * TAU, G() * 0.01, G() * 0.012, 4000 + R() * 3000, 0.35 + R() * 0.4, 0.9 + R() * 0.6, omega(a) * 0.7, 0, 0.05);
      continue;
    }
    const e = orbitE(g, a), th = orbitTheta(g, a), ap = armPhase(g, a), young = a > g.bar && R() < 0.12;
    if (young) {
      const fr = ap + Math.floor(R() * g.m) * TAU / g.m + G() * 0.09;
      star(a, fr, G() * 0.012, G() * 0.006, 9000 + R() * 20000, 0.55 + R() * 0.4, 1.1 + R() * 1.3, omega(a) * 0.12, th, e);
    } else {
      star(a, R() * TAU, G() * 0.016, G() * 0.014, 3800 + R() * 3200, 0.3 + R() * 0.45, 0.85 + R() * 0.55, omega(a), th, e);
    }
  }

  const quad = (B: Buf, a: number, fr: number, dr: number, c: RGB, al: number, size: number, w: number, th: number, e: number) =>
    push(B, a, th, e, fr, w, dr, 0, slot, c, B === P.dust ? al : al * gb, size);
  const core = g.shape === "elliptical" ? 0.5 : 0.3;
  quad(P.glow, 0, 0, 0, kelvin(4200), 0.32, core, 0, 0, 0);
  quad(P.glow, 0, 0, 0, mix(kelvin(5200), tint, 0.4), 0.22, core * 0.42, 0, 0, 0);
  quad(P.glow, 0, 0, 0, tint, 0.1, 2.3, 0, 0, 0);
  if (sp) {
    for (let i = 0; i < 1600; i++) {
      let a: number;
      do a = -0.3 * Math.log(R() * R() + 1e-9); while (a > 1.05 || a < g.bar * 0.7);
      const e = orbitE(g, a), th = orbitTheta(g, a), fr = armPhase(g, a) + Math.floor(R() * g.m) * TAU / g.m;
      if (i % 2) quad(P.glow, a, fr + G() * 0.3, G() * 0.035, mix(tint, [0.55, 0.7, 1], 0.25), 0.018 + R() * 0.02, 0.06 + R() * 0.08, 0, th, e);
      if (R() < 0.5) quad(P.dust, a, fr - 0.13 * Math.sign(g.kap || 1) + G() * 0.06, G() * 0.01, [0, 0, 0], 0.025 + R() * 0.035, 0.04 + R() * 0.05, 0, th, e);
      if (R() < 0.025 && a > g.bar) quad(P.hii, a, fr + 0.05 + G() * 0.05, G() * 0.008, R() < 0.7 ? [1, 0.38, 0.62] : [0.6, 0.75, 1], 0.35 + R() * 0.35, 0.008 + R() * 0.022, 0, th, e);
    }
    if (g.bar) {
      for (let i = 0; i < 220; i++) {
        const a = R() * g.bar, fr = (R() < 0.5 ? 0 : Math.PI) + G() * 0.12;
        quad(P.dust, a, fr + 0.2, G() * 0.01, [0, 0, 0], 0.02 + R() * 0.03, 0.03 + R() * 0.03, 0, 0, orbitE(g, a));
      }
    }
  } else if (g.shape === "ring") {
    for (let i = 0; i < 500; i++) {
      const a = 0.68 + G() * 0.03, fr = R() * TAU;
      quad(P.glow, a, fr, 0, mix(tint, [0.6, 0.75, 1], 0.3), 0.08 + R() * 0.06, 0.05 + R() * 0.05, omega(a) * 0.4, 0, 0.04);
      if (R() < 0.3) quad(P.hii, a, fr, 0, [1, 0.4, 0.65], 0.35 + R() * 0.3, 0.01 + R() * 0.02, omega(a) * 0.4, 0, 0.04);
      if (R() < 0.4) quad(P.dust, a - 0.03, fr, 0, [0, 0, 0], 0.2, 0.03, omega(a) * 0.4, 0, 0.04);
    }
  } else if (g.shape === "irregular") {
    for (const k of clumps) {
      for (let i = 0; i < 70; i++) {
        const x = k[0] + G() * k[2], y = k[1] + G() * k[2];
        quad(P.glow, Math.hypot(x, y), Math.atan2(y, x), 0, tint, 0.025, 0.06 + R() * 0.06, 0.004, 0, 0);
        if (R() < 0.3) quad(P.dust, Math.hypot(x, y) + 0.02, Math.atan2(y, x), 0, [0, 0, 0], 0.3, 0.04, 0.004, 0, 0);
      }
    }
  } else if (g.shape === "lenticular") {
    for (let i = 0; i < 500; i++) quad(P.dust, 0.38 + G() * 0.02, R() * TAU, 0, [0, 0, 0], 0.22, 0.025 + R() * 0.02, 0, 0, 0.05);
    for (let i = 0; i < 400; i++) quad(P.glow, Math.abs(G()) * 0.5, R() * TAU, 0, mix(kelvin(5000), tint, 0.3), 0.05, 0.08 + R() * 0.08, 0, 0, 0.05);
  } else {
    for (let i = 0; i < 300; i++) quad(P.glow, Math.abs(G()) * 0.35, R() * TAU, 0, mix(kelvin(4300), tint, 0.25), 0.06, 0.1 + R() * 0.1, 0, 0, 0.28);
  }
}

function synthCluster(P: Particles, c: CNode, slot: number) {
  const R: Rng = mkRng(c.seed ^ 0x2545f491);
  const G = () => gauss(R);
  const tint = hsl(c.hue, 0.75, 0.66);
  const fog = c.stars.filter((s) => s.level === 0).length / Math.max(1, c.stars.length);
  const nm = clamp(c.mass / 18, 0.3, 1.4);
  // Plummer sphere: the classic density law for star clusters.
  for (let i = 0, n = Math.round(500 + 1300 * nm); i < n; i++) {
    const u = Math.max(1e-4, R()), a = Math.min(1.6, 0.28 / Math.sqrt(Math.pow(u, -2 / 3) - 1)), young = R() < 0.3;
    push(P.stars, a, 0, 0, R() * TAU, 0.01 / (a + 0.25), G() * 0.01, G() * a * 0.5, slot, mix(kelvin(young ? 9000 + R() * 14000 : 3800 + R() * 3000), tint, 0.3), 0.55 + R() * 0.45, young ? 1.1 + R() * 1.3 : 0.9 + R() * 0.7);
  }
  push(P.glow, 0, 0, 0, 0, 0, 0, 0, slot, tint, 0.25 + 0.3 * nm, 2.1);
  push(P.glow, 0, 0, 0, 0, 0, 0, 0, slot, mix(tint, [1, 1, 1], 0.45), 0.35 + 0.4 * nm, 0.6);
  for (let i = 0, n = Math.round(4 + 10 * nm); i < n; i++) push(P.hii, Math.abs(G()) * 0.6, 0, 0, R() * TAU, 0, 0, 0, slot, R() < 0.7 ? [1, 0.38, 0.62] : [0.6, 0.75, 1], 0.5 + R() * 0.4, 0.05 + R() * 0.08);
  for (let i = 0; i < Math.round(fog * 14); i++) push(P.dust, 0.5 + R() * 0.5, 0, 0, R() * TAU, 0, 0, 0, slot, [0, 0, 0], 0.3 + R() * 0.2, 0.25 + R() * 0.3);
}
