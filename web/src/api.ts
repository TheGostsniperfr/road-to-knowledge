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

export interface UniverseView { version: number; galaxies: GalaxyView[]; problems: Problem[]; inbox: number; redirects?: Record<string, string> }

export interface LinkRef { id: string; name: string; galaxy: string }

export interface StarView extends StarSummary {
  path: string[];
  summary: string;
  body: string;
  evidence?: string[];
  tags?: string[];
  link_refs?: LinkRef[];
  file: string;
  source?: Source;
}

export interface Source { title: string; author?: string; year?: number; pages?: number; file?: string; url?: string; available: boolean }

export interface Hit { id: string; name: string; snippet: string; mode?: "words" | "meaning" | "both" }

export interface SemanticStatus { state: "off" | "missing" | "loading" | "indexing" | "ready" | "error"; model?: string; done: number; total: number; message?: string }

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
  semantic: () => req<SemanticStatus>("GET", "/api/semantic"),
  due: (scope: string, exclude: string[]) => req<DueCard[]>("GET", `/api/review/due?scope=${encodeURIComponent(scope)}&exclude=${encodeURIComponent(exclude.join(","))}`),
  review: (card: string, rating: number) => req<void>("POST", "/api/review", { card, rating }),
  flush: (count: number) => req<void>("POST", "/api/review/flush", { count }),
  capture: (text: string) => req<{ saved: string }>("POST", "/api/capture", { text }),
  inbox: () => req<InboxItem[]>("GET", "/api/inbox"),
  frontier: (scope: string, limit = 12) => req<FrontierView>("GET", `/api/frontier?scope=${encodeURIComponent(scope)}&limit=${limit}`),
  ai: (tool: "profile" | "subgraph" | "node", id = "") => req<AIPreview>("GET", `/api/ai?tool=${tool}&id=${encodeURIComponent(id)}`),
};

export interface NewsMatch { id: string; name: string; level: number; kind?: "cluster" }

export interface NewsItem { id: string; title: string; link: string; summary?: string; source: string; scope: string; scope_name: string; galaxy: string; published: string; subjects: NewsMatch[] }

export interface NewsSource { url: string; name: string; scope: string; scope_name: string; last_ok?: string; error?: string }

export interface NewsView { items: NewsItem[]; sources: NewsSource[]; hidden: number }

export interface NewsReport { fetched: number; not_modified: number; skipped: number; failed: number; new_items: number; errors?: string[] }

export interface SplitPart { id: string; name?: string; notions: string[] }

export interface RestructureOp { kind: "move" | "split" | "merge"; id?: string; to?: string; name?: string; parts?: SplitPart[]; sources?: string[]; into?: string }

export interface FileOp { kind: "move" | "write" | "delete"; path: string; from?: string }

export interface Rename { from: string; to: string; at: string }

export interface RestructurePlan { op: RestructureOp; files: FileOp[]; renames: Rename[]; summary: string[]; message: string; hash: string }

export const restructureApi = {
  run: (op: RestructureOp, dryRun: boolean, planHash = "") => req<RestructurePlan>("POST", "/api/restructure", { ...op, dry_run: dryRun, plan_hash: planHash }),
};

export const newsApi = {
  list: (opts: { scope?: string; since?: string; all?: boolean; limit?: number }) =>
    req<NewsView>("GET", `/api/news?scope=${encodeURIComponent(opts.scope ?? "")}&since=${encodeURIComponent(opts.since ?? "30d")}&limit=${opts.limit ?? 60}${opts.all ? "&all=1" : ""}`),
  fetch: () => req<NewsReport>("POST", "/api/news/fetch"),
};
