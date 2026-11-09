import { hierarchy, pack } from "d3-hierarchy";
import type { Layout, SNode, SkyNode } from "../sky/layout";

export type Kind = "root" | "galaxy" | "cluster" | "star";

export interface Circle {
  id: string;
  kind: Kind;
  name: string;
  hue: number;
  x: number;
  y: number;
  r: number;
  depth: number;
  node: SkyNode | null;
  parent: Circle | null;
  children: Circle[];
}

export interface Edge {
  a: Circle;
  b: Circle;
  // Control point pulled toward the smallest circle holding both ends, so links that stay
  // inside a theme bend inside it and cross-theme links arc through the middle.
  cx: number;
  cy: number;
}

export interface Atlas {
  root: Circle;
  byId: Map<string, Circle>;
  edges: Edge[];
}

interface Datum { id: string; kind: Kind; name: string; hue: number; node: SkyNode | null; value: number; children?: Datum[] }

export const SIZE = 1000;

// A fog subject still takes room, so a theme you are only starting reads as a place, not a speck.
const weight = (s: SNode) => (s.level ? 1 + s.level : 0.7);

export function buildAtlas(l: Layout): Atlas {
  const data: Datum = {
    id: "", kind: "root", name: "Univers", hue: 0, node: null, value: 0,
    // d3 gives an empty container a zero radius, which no camera can frame: leave it out.
    children: l.galaxies.filter((g) => g.clusters.some((c) => c.stars.length)).map((g) => ({
      id: g.id, kind: "galaxy", name: g.name, hue: g.hue, node: g, value: 0,
      children: g.clusters.filter((c) => c.stars.length).map((c) => ({
        id: c.id, kind: "cluster", name: c.name, hue: c.hue, node: c, value: 0,
        children: c.stars.map((s) => ({ id: s.id, kind: "star", name: s.name, hue: s.hue, node: s, value: weight(s) })),
      })),
    })),
  };

  const h = hierarchy(data)
    .sum((d) => (d.kind === "star" ? d.value : 0))
    .sort((a, b) => (b.value ?? 0) - (a.value ?? 0) || (a.data.id < b.data.id ? -1 : 1));
  const packed = pack<Datum>().size([SIZE, SIZE]).padding((n) => [SIZE * 0.03, SIZE * 0.012, SIZE * 0.004, 0][n.depth] ?? 0)(h);

  const byId = new Map<string, Circle>();
  const toCircle = (n: typeof packed, parent: Circle | null): Circle => {
    const c: Circle = {
      id: n.data.id, kind: n.data.kind, name: n.data.name, hue: n.data.hue,
      x: n.x, y: n.y, r: n.r, depth: n.depth, node: n.data.node, parent, children: [],
    };
    byId.set(c.id, c);
    c.children = (n.children ?? []).map((k) => toCircle(k, c));
    return c;
  };
  const root = toCircle(packed, null);

  const edges: Edge[] = [];
  const seen = new Set<string>();
  for (const s of l.stars) {
    for (const t of s.links) {
      const key = s.id < t.id ? s.id + "|" + t.id : t.id + "|" + s.id;
      if (seen.has(key)) continue;
      seen.add(key);
      const a = byId.get(s.id), b = byId.get(t.id);
      if (!a || !b) continue;
      const lca = commonAncestor(a, b);
      const mx = (a.x + b.x) / 2, my = (a.y + b.y) / 2;
      edges.push({ a, b, cx: mx + (lca.x - mx) * 0.6, cy: my + (lca.y - my) * 0.6 });
    }
  }
  return { root, byId, edges };
}

export function commonAncestor(a: Circle, b: Circle): Circle {
  const up = new Set<Circle>();
  for (let n: Circle | null = a; n; n = n.parent) up.add(n);
  for (let n: Circle | null = b; n; n = n.parent) if (up.has(n)) return n;
  throw new Error("atlas: circles from different trees");
}

// Deepest circle under the point, walking down only through circles that contain it.
export function hit(root: Circle, x: number, y: number): Circle {
  let cur = root;
  for (;;) {
    const next = cur.children.find((c) => (x - c.x) ** 2 + (y - c.y) ** 2 <= c.r * c.r);
    if (!next) return cur;
    cur = next;
  }
}

// Planets (notions) have no circle of their own: they resolve to their subject.
export function circleOf(a: Atlas, n: SkyNode | null): Circle {
  if (!n) return a.root;
  const id = n.kind === "planet" ? n.star.id : n.id;
  return a.byId.get(id) ?? a.root;
}
