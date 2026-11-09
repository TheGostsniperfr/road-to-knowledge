import { describe, expect, it } from "vitest";
import type { GalaxyView, UniverseView } from "../api";
import { buildLayout, currentPins } from "./layout";

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

  it("keeps pinned galaxies fixed when a theme is added", () => {
    const gs = [galaxy("tech", "barred", 4), galaxy("lang", "spiral", 3), galaxy("ia", "spiral", 2)];
    const pins = currentPins(buildLayout(universe(gs)));
    const pinnedGs = gs.map((g) => ({ ...g, pin: pins[g.id] }));

    const after = buildLayout(universe([...pinnedGs, galaxy("cook", "spiral", 6, 8)]));

    for (const g of after.galaxies.filter((g) => g.id !== "cook")) expect([g.x, g.y]).toEqual([pins[g.id].x, pins[g.id].y]);
  });

  it("places a new galaxy away from pinned ones", () => {
    const gs = [galaxy("tech", "barred", 4), galaxy("lang", "spiral", 3)];
    const pins = currentPins(buildLayout(universe(gs)));

    const { galaxies } = buildLayout(universe([...gs.map((g) => ({ ...g, pin: pins[g.id] })), galaxy("cook", "spiral", 6, 8)]));

    const cook = galaxies.find((g) => g.id === "cook")!;
    for (const g of galaxies) if (g !== cook) expect(Math.hypot(g.x - cook.x, g.y - cook.y)).toBeGreaterThan((g.R + cook.R) * 1.1);
  });

  it("uses the pinned rotation when there is one", () => {
    const g = { ...galaxy("tech", "barred", 2), pin: { x: 0, y: 0, rotation: 0 } };

    const { galaxies } = buildLayout(universe([g]));

    expect([galaxies[0].ct, galaxies[0].st]).toEqual([1, 0]);
  });
});
