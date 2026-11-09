import { describe, expect, it } from "vitest";
import type { StarSummary, UniverseView } from "../api";
import { buildLayout } from "../sky/layout";
import { buildAtlas, commonAncestor, hit, type Circle } from "./pack";

const star = (id: string, level: number, links: string[] = []): StarSummary => ({
  id, name: id.split("/").pop()!, level, state: level ? "progress" : "explore", days: null, stale: false, links, cards: 0, due: 0,
});

const universe = (): UniverseView => ({
  version: 1, inbox: 0, problems: [],
  galaxies: [
    { id: "tech", name: "Tech", hue: 200, shape: "barred", review_after_days: 90, discovery: { near: 60, two_hops: 30, off_piste: 10, news: false },
      clusters: [
        { id: "tech/k8s", name: "K8s", stars: [star("tech/k8s/cilium", 4, ["langues/jp/kana"]), star("tech/k8s/helm", 2)] },
        { id: "tech/nix", name: "Nix", stars: [star("tech/nix/flakes", 3), star("tech/nix/sops", 0)] },
      ] },
    { id: "langues", name: "Langues", hue: 20, shape: "spiral", review_after_days: 90, discovery: { near: 60, two_hops: 30, off_piste: 10, news: false },
      clusters: [{ id: "langues/jp", name: "Japonais", stars: [star("langues/jp/kana", 2)] }] },
  ],
});

const inside = (c: Circle, p: Circle) => Math.hypot(c.x - p.x, c.y - p.y) + c.r <= p.r + 1e-6;

describe("buildAtlas", () => {
  it("nests every circle inside its parent", () => {
    const a = buildAtlas(buildLayout(universe()));

    const bad = [...a.byId.values()].filter((c) => c.parent && !inside(c, c.parent));

    expect(bad.map((c) => c.id)).toEqual([]);
  });

  it("gives a better known subject a bigger circle", () => {
    const a = buildAtlas(buildLayout(universe()));

    expect(a.byId.get("tech/k8s/cilium")!.r).toBeGreaterThan(a.byId.get("tech/k8s/helm")!.r);
  });

  it("is deterministic for the same universe", () => {
    const pos = () => [...buildAtlas(buildLayout(universe())).byId.values()].map((c) => [c.id, c.x.toFixed(6), c.y.toFixed(6)]);

    expect(pos()).toEqual(pos());
  });

  it("keeps one edge per link even when both ends declare it", () => {
    const u = universe();
    u.galaxies[1].clusters[0].stars[0].links = ["tech/k8s/cilium"];

    const a = buildAtlas(buildLayout(u));

    expect(a.edges).toHaveLength(1);
  });

  it("finds the root as the common ancestor of subjects in different themes", () => {
    const a = buildAtlas(buildLayout(universe()));

    expect(commonAncestor(a.byId.get("tech/k8s/cilium")!, a.byId.get("langues/jp/kana")!)).toBe(a.root);
  });

  it("hits the deepest circle under the point", () => {
    const a = buildAtlas(buildLayout(universe()));
    const s = a.byId.get("tech/nix/flakes")!;

    expect(hit(a.root, s.x, s.y).id).toBe("tech/nix/flakes");
  });
});
