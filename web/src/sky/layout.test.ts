import { describe, expect, it } from "vitest";
import type { GalaxyView, UniverseView } from "../api";
import { buildLayout } from "./layout";

function galaxy(id: string, shape: string, clusters: number, stars = 3): GalaxyView {
  return {
    id, name: id, hue: 200, shape, review_after_days: 90, discovery: { near: 60, two_hops: 30, off_piste: 10, news: false },
    clusters: Array.from({ length: clusters }, (_, c) => ({
      id: `${id}/c${c}`, name: `c${c}`,
      stars: Array.from({ length: stars }, (_, s) => ({ id: `${id}/c${c}/s${s}`, name: `s${s}`, level: (s % 5) + 1, state: "acquired" as const, days: null, stale: false, cards: 0, due: 0 })),
    })),
  };
}

const universe = (gs: GalaxyView[]): UniverseView => ({ version: 1, galaxies: gs, problems: [], inbox: 0 });

describe("buildLayout", () => {
  it("places the same data at the same positions every time", () => {
    const u = universe([galaxy("tech", "barred", 6), galaxy("lang", "spiral", 2)]);

    const a = buildLayout(u), b = buildLayout(u);

    expect(a.stars.map((s) => [s.x, s.y])).toEqual(b.stars.map((s) => [s.x, s.y]));
  });

  it("keeps a subject in place when an unrelated theme is added", () => {
    const before = buildLayout(universe([galaxy("tech", "barred", 2)]));
    const after = buildLayout(universe([galaxy("tech", "barred", 2), galaxy("cook", "elliptical", 1, 1)]));

    const s0 = before.stars[0], s1 = after.byId.get(s0.id)!;
    const g0 = before.galaxies[0], g1 = after.galaxies[0];

    expect(s1.x - g1.x).toBeCloseTo(s0.x - g0.x, 6);
    expect(s1.y - g1.y).toBeCloseTo(s0.y - g0.y, 6);
  });

  it("keeps galaxies apart", () => {
    const gs = ["a", "b", "c", "d", "e", "f", "g"].map((id) => galaxy(id, "spiral", 3));

    const { galaxies } = buildLayout(universe(gs));

    for (const a of galaxies)
      for (const b of galaxies)
        if (a !== b) expect(Math.hypot(a.x - b.x, a.y - b.y)).toBeGreaterThan((a.R + b.R) * 1.1);
  });

  it("caps spiral arms at four however many sub-themes there are", () => {
    const { galaxies } = buildLayout(universe([galaxy("big", "spiral", 12, 1), galaxy("bar", "barred", 9, 1)]));

    expect(galaxies.map((g) => g.m)).toEqual([4, 2]);
  });

  it("resolves wormholes to the linked subject", () => {
    const g = galaxy("lang", "spiral", 1, 2);
    g.clusters[0].stars[0].links = ["lang/c0/s1"];

    const { stars } = buildLayout(universe([g]));

    expect(stars[0].links.map((s) => s.id)).toEqual(["lang/c0/s1"]);
  });
});
