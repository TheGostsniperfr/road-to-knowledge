import type { UniverseView } from "../api";
import type { SkyNode } from "../sky/engine";

export interface ScopeChild { id: string; name: string; due: number }

/** Review scope follows the zoom: the selected galaxy, sub-theme or subject, or the whole universe. */
export function scopeOf(n: SkyNode | null): { id: string; name: string } {
  if (!n) return { id: "", name: "Tout l'univers" };
  const s = n.kind === "planet" ? n.star : n;
  return { id: s.id, name: s.name };
}

export function scopeChildren(u: UniverseView | null, scopeId: string): ScopeChild[] {
  if (!u) return [];
  const sum = (stars: { due: number }[]) => stars.reduce((a, s) => a + s.due, 0);
  if (!scopeId) return u.galaxies.map((g) => ({ id: g.id, name: g.name, due: sum(g.clusters.flatMap((c) => c.stars)) }));
  const g = u.galaxies.find((x) => x.id === scopeId);
  if (g) return g.clusters.map((c) => ({ id: c.id, name: c.name, due: sum(c.stars) }));
  for (const gg of u.galaxies) {
    const c = gg.clusters.find((x) => x.id === scopeId);
    if (c) return c.stars.filter((s) => s.cards > 0).map((s) => ({ id: s.id, name: s.name, due: s.due }));
  }
  return [];
}

export function dueIn(u: UniverseView | null, scopeId: string, exclude: string[]): number {
  if (!u) return 0;
  let n = 0;
  for (const g of u.galaxies)
    for (const c of g.clusters)
      for (const s of c.stars) {
        const inScope = !scopeId || s.id === scopeId || s.id.startsWith(scopeId + "/");
        const excluded = exclude.some((x) => s.id === x || s.id.startsWith(x + "/"));
        if (inScope && !excluded) n += s.due;
      }
  return n;
}

// Scope choices stay on this device: they are a viewing preference, not knowledge, so never in git.
const key = (id: string) => `rtk.scope.${id || "universe"}`;
export const excluded = {
  get(id: string): string[] {
    try {
      return JSON.parse(localStorage.getItem(key(id)) ?? "[]");
    } catch {
      return [];
    }
  },
  set(id: string, ids: string[]) {
    try {
      localStorage.setItem(key(id), JSON.stringify(ids));
    } catch {
      /* private mode: the choice just lasts for this session */
    }
  },
};
