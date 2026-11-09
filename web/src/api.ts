export type State = "idea" | "explore" | "progress" | "acquired";

export interface PlanetRef { id: string; name: string }

export interface StarSummary {
  id: string;
  name: string;
  level: number;
  state: State;
  days: number | null;
  stale: boolean;
  links?: string[];
  planets?: PlanetRef[];
  cards: number;
  due: number;
}

export interface ClusterView { id: string; name: string; stars: StarSummary[] }

export interface Discovery { near: number; two_hops: number; off_piste: number; news: boolean }

export interface Pin { x: number; y: number; rotation?: number }

export interface GalaxyView {
  id: string;
  name: string;
  hue: number;
  shape: string;
  review_after_days: number;
  discovery: Discovery;
  pin?: Pin;
  clusters: ClusterView[];
}

export interface Problem { path: string; message: string }

export interface UniverseView { version: number; galaxies: GalaxyView[]; problems: Problem[]; inbox: number }

export interface LinkRef { id: string; name: string; galaxy: string }

export interface StarView extends StarSummary {
  path: string[];
  summary: string;
  body: string;
  evidence?: string[];
  tags?: string[];
  link_refs?: LinkRef[];
  file: string;
}

export interface Hit { id: string; name: string; snippet: string }

export interface DueCard { id: string; star: string; star_name: string; q: string; a: string; new: boolean; next: Record<string, string> }

export interface AIPreview { call: string; payload: string; tokens: number; full_tokens: number }

export interface InboxItem { name: string; text: string; captured: string }

export type Bucket = "near" | "two_hops" | "off_piste";

export interface Suggestion { id: string; name: string; galaxy: string; cluster: string; level: number; state: State; bucket: Bucket; reason: string }

export interface FrontierView { scope: string; budget: Discovery; suggestions: Suggestion[] }

async function req<T>(method: string, url: string, body?: unknown): Promise<T> {
  const res = await fetch(url, {
    method,
    headers: body === undefined ? undefined : { "Content-Type": "application/json" },
    body: body === undefined ? undefined : JSON.stringify(body),
  });
  if (!res.ok) {
    let msg = `${res.status}`;
    try { msg = (await res.json()).error ?? msg; } catch { /* body was not JSON */ }
    throw new Error(msg);
  }
  return res.status === 204 ? (undefined as T) : res.json();
}

const enc = (id: string) => id.split("/").map(encodeURIComponent).join("/");

export const api = {
  universe: () => req<UniverseView>("GET", "/api/universe"),
  star: (id: string) => req<StarView>("GET", `/api/star/${enc(id)}`),
  setMeta: (id: string, patch: { level?: number; state?: State }) => req<StarView>("PATCH", `/api/star/${enc(id)}`, patch),
  setGalaxy: (id: string, patch: { review_after_days?: number; discovery?: Discovery }) => req<GalaxyView>("PATCH", `/api/galaxy/${encodeURIComponent(id)}`, patch),
  setLayout: (galaxies: Record<string, Pin>) => req<UniverseView>("PUT", "/api/layout", { galaxies }),
  search: (q: string) => req<Hit[]>("GET", `/api/search?q=${encodeURIComponent(q)}`),
  due: (scope: string, exclude: string[]) => req<DueCard[]>("GET", `/api/review/due?scope=${encodeURIComponent(scope)}&exclude=${encodeURIComponent(exclude.join(","))}`),
  review: (card: string, rating: number) => req<void>("POST", "/api/review", { card, rating }),
  flush: (count: number) => req<void>("POST", "/api/review/flush", { count }),
  capture: (text: string) => req<{ saved: string }>("POST", "/api/capture", { text }),
  inbox: () => req<InboxItem[]>("GET", "/api/inbox"),
  frontier: (scope: string, limit = 12) => req<FrontierView>("GET", `/api/frontier?scope=${encodeURIComponent(scope)}&limit=${limit}`),
  ai: (tool: "profile" | "subgraph" | "node", id = "") => req<AIPreview>("GET", `/api/ai?tool=${tool}&id=${encodeURIComponent(id)}`),
};
