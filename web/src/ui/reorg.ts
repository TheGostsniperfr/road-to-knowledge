import type { RestructurePlan, UniverseView } from "../api";

// Same rule as kb.Slug on the server: letters and digits kept, everything else becomes one dash.
export function slugify(s: string): string {
  return s.toLowerCase().replace(/[^\p{L}\p{N}]+/gu, "-").replace(/^-+|-+$/g, "");
}

// liveId follows a restructure: an old deep link or a bookmark opens the subject where it is now.
// An id that exists today is never redirected, even if an older subject once had it.
export function liveId(id: string, u: UniverseView | null | undefined): string {
  const redirects = u?.redirects;
  if (!redirects) return id;
  const hash = id.indexOf("#");
  const base = hash < 0 ? id : id.slice(0, hash);
  if (idsOf(u).has(base)) return id;
  const to = redirects[base];
  if (!to) return id;
  return hash < 0 ? to : to + id.slice(hash);
}

const idCache = new WeakMap<UniverseView, Set<string>>();

export function idsOf(u: UniverseView): Set<string> {
  let ids = idCache.get(u);
  if (!ids) {
    ids = new Set();
    for (const g of u.galaxies) {
      ids.add(g.id);
      for (const c of g.clusters) {
        ids.add(c.id);
        for (const s of c.stars) ids.add(s.id);
      }
    }
    idCache.set(u, ids);
  }
  return ids;
}

// afterPlan is where the subject shown in the fiche lives once the plan is applied.
export function afterPlan(id: string, plan: RestructurePlan): string {
  return plan.renames.find((r) => r.from === id)?.to ?? id;
}

export interface Place { id: string; label: string }

export function subThemes(u: UniverseView | null, except: string): Place[] {
  if (!u) return [];
  return u.galaxies.flatMap((g) => g.clusters.filter((c) => c.id !== except).map((c) => ({ id: c.id, label: `${g.name} › ${c.name}` })));
}

export function subjects(u: UniverseView | null, except: string): Place[] {
  if (!u) return [];
  return u.galaxies.flatMap((g) => g.clusters.flatMap((c) => c.stars.filter((s) => s.id !== except).map((s) => ({ id: s.id, label: `${s.name} · ${g.name} › ${c.name}` }))));
}
