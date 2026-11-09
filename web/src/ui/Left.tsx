import { useEffect, useRef, useState } from "preact/hooks";
import { api, type Bucket, type FrontierView, type Hit, type InboxItem, type SemanticStatus, type UniverseView } from "../api";
import type { SkyNode } from "../sky/engine";
import { News } from "./News";
import { useTerms } from "./terms";

type Tab = "plan" | "todo" | "news" | "inbox";

const dot = (s: { level: number; stale?: boolean }, hue: number) =>
  s.level === 0 ? <span class="dot fog" /> : <span class={`dot${s.stale ? " stale" : ""}`} style={{ background: `hsl(${hue},80%,${45 + s.level * 8}%)` }} />;

export function Left({ universe, selected, go }: { universe: UniverseView | null; selected: SkyNode | null; go: (id: string) => void }) {
  const [tab, setTab] = useState<Tab>("plan");
  const [query, setQuery] = useState("");
  const [hits, setHits] = useState<Hit[]>([]);
  const [sem, setSem] = useState<SemanticStatus | null>(null);
  const input = useRef<HTMLInputElement>(null);

  // Indexing runs in the background after start; poll only until it settles.
  useEffect(() => {
    let live = true;
    let timer = 0;
    const poll = () => api.semantic().then((s) => {
      if (!live) return;
      setSem(s);
      if (s.state === "loading" || s.state === "indexing") timer = window.setTimeout(poll, 2000);
    }).catch(() => undefined);
    poll();
    return () => { live = false; clearTimeout(timer); };
  }, [universe?.version]);

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if ((e.ctrlKey || e.metaKey) && e.key.toLowerCase() === "k") {
        e.preventDefault();
        input.current?.focus();
      }
    };
    addEventListener("keydown", onKey);
    return () => removeEventListener("keydown", onKey);
  }, []);

  useEffect(() => {
    if (query.trim().length < 2) {
      setHits([]);
      return;
    }
    const t = setTimeout(() => api.search(query).then(setHits).catch(() => setHits([])), 140);
    return () => clearTimeout(t);
  }, [query, universe?.version]);

  const selId = selected?.id;
  const openPath = new Set<string>();
  if (selected) {
    const parts = selected.id.split("#")[0].split("/");
    for (let i = 1; i <= parts.length; i++) openPath.add(parts.slice(0, i).join("/"));
  }

  return (
    <aside class="left" aria-label="Navigation">
      <div class="brand"><i />Road to Knowledge</div>
      <div class="tabs" role="tablist">
        <button role="tab" aria-selected={tab === "plan"} onClick={() => setTab("plan")}>Plan</button>
        <button role="tab" aria-selected={tab === "todo"} onClick={() => setTab("todo")}>À explorer</button>
        <button role="tab" aria-selected={tab === "news"} onClick={() => setTab("news")}>Veille</button>
        <button role="tab" aria-selected={tab === "inbox"} onClick={() => setTab("inbox")}>
          Boîte {universe?.inbox ? <span class="n">{universe.inbox}</span> : null}
        </button>
      </div>
      <div class="search">
        <input ref={input} type="search" placeholder="Rechercher (Ctrl K)" value={query} aria-label="Rechercher"
          onInput={(e) => { setQuery(e.currentTarget.value); setTab("plan"); }}
          onKeyDown={(e) => { if (e.key === "Enter" && hits[0]) go(hits[0].id); }} />
        {query.trim().length > 0 && semLine(sem) && <small class="semstat">{semLine(sem)}</small>}
      </div>
      <div class="lbody">
        {tab === "plan" && (query.trim().length >= 2 ? <Hits hits={hits} go={go} /> : <Tree universe={universe} selId={selId} open={openPath} go={go} />)}
        {tab === "todo" && <Todo universe={universe} selected={selected} go={go} />}
        {tab === "news" && <News universe={universe} go={go} />}
        {tab === "inbox" && <Inbox version={universe?.version ?? 0} />}
      </div>
    </aside>
  );
}

function Tree({ universe, selId, open, go }: { universe: UniverseView | null; selId?: string; open: Set<string>; go: (id: string) => void }) {
  const { t } = useTerms();
  if (!universe) return <p class="empty">Chargement…</p>;
  if (!universe.galaxies.length) return <p class="empty">{t.noTheme}</p>;
  return (
    <div class="tree">
      {universe.galaxies.map((g) => (
        <details key={g.id} open={open.has(g.id)}>
          <summary class={selId === g.id ? "sel" : ""} onClick={(e) => { e.preventDefault(); go(g.id); (e.currentTarget.parentElement as HTMLDetailsElement).open = true; }}>
            <span class="dot" style={{ background: `hsl(${g.hue},80%,65%)` }} /><b>{g.name}</b>
          </summary>
          {g.clusters.map((c) => (
            <details key={c.id} class="sub" open={open.has(c.id)}>
              <summary class={selId === c.id ? "sel" : ""} onClick={(e) => { e.preventDefault(); go(c.id); (e.currentTarget.parentElement as HTMLDetailsElement).open = true; }}>
                {c.name}<span class="n">{c.stars.length}</span>
              </summary>
              {c.stars.map((s) => (
                <div key={s.id}>
                  <button class={`item${selId === s.id ? " sel" : ""}`} onClick={() => go(s.id)}>
                    {dot(s, g.hue)}<span>{s.name}</span>
                    {s.links?.length ? <span class="n" title={t.links}>⟡</span> : s.state === "progress" ? <span class="n">en cours</span> : null}
                  </button>
                  {open.has(s.id) && s.planets?.map((p) => (
                    <button key={p.id} class={`item pl${selId === p.id ? " sel" : ""}`} onClick={() => go(p.id)}>○ {p.name}</button>
                  ))}
                </div>
              ))}
            </details>
          ))}
        </details>
      ))}
    </div>
  );
}

function semLine(s: SemanticStatus | null): string {
  switch (s?.state) {
    case "ready": return "Recherche par mots et par sens";
    case "indexing": return `Recherche par sens : indexation ${s.total ? Math.round((100 * s.done) / s.total) : 0} %`;
    case "loading": return "Recherche par sens : chargement du modèle";
    case "missing": return "Recherche par sens : modèle absent (rtk models pull)";
    case "error": return "Recherche par sens indisponible : mots seulement";
    default: return "";
  }
}

function Hits({ hits, go }: { hits: Hit[]; go: (id: string) => void }) {
  if (!hits.length) return <p class="empty">Rien trouvé.</p>;
  return (
    <div>
      {hits.map((h) => (
        <button key={h.id} class="hit" onClick={() => go(h.id)}>
          <span>{h.name}{h.mode === "meaning" && <em class="bysense" title="Trouvé par le sens, sans mot en commun">sens</em>}</span>
          <small>{h.snippet.replace(/[\[\]]/g, "")}</small>
        </button>
      ))}
    </div>
  );
}

const buckets: { key: Bucket; label: string }[] = [
  { key: "near", label: "Proche" },
  { key: "two_hops", label: "À deux pas" },
  { key: "off_piste", label: "Hors piste" },
];

// The frontier follows the selection down to its sub-theme; the chips widen it back.
function scopesOf(universe: UniverseView, selected: SkyNode | null) {
  const out = [{ id: "", name: "Tout" }];
  if (!selected) return out;
  const parts = selected.id.split("#")[0].split("/");
  const g = universe.galaxies.find((x) => x.id === parts[0]);
  if (!g) return out;
  out.push({ id: g.id, name: g.name });
  const c = parts.length > 1 ? g.clusters.find((x) => x.id === parts.slice(0, 2).join("/")) : undefined;
  if (c) out.push({ id: c.id, name: c.name });
  return out;
}

function Todo({ universe, selected, go }: { universe: UniverseView | null; selected: SkyNode | null; go: (id: string) => void }) {
  const { t } = useTerms();
  const scopes = universe ? scopesOf(universe, selected) : [];
  const deepest = scopes[scopes.length - 1]?.id ?? "";
  const [scope, setScope] = useState(deepest);
  const [frontier, setFrontier] = useState<FrontierView | null>(null);
  const [err, setErr] = useState("");
  const [all, setAll] = useState(false);
  useEffect(() => setScope(deepest), [deepest]);
  useEffect(() => {
    if (!universe) return;
    let live = true;
    setFrontier(null);
    api.frontier(scope)
      .then((f) => { if (live) { setFrontier(f); setErr(""); } })
      .catch((e) => { if (live) { setFrontier(null); setErr(String(e)); } });
    return () => { live = false; };
  }, [scope, universe?.version]);
  if (!universe) return null;

  const inScope = (id: string) => !scope || id.startsWith(scope + "/");
  const hue = new Map(universe.galaxies.map((g) => [g.id, g.hue]));
  const progress = universe.galaxies.flatMap((g) => g.clusters.flatMap((c) =>
    c.stars.filter((s) => s.state === "progress" && inScope(s.id)).map((s) => ({ s, g }))));
  const suggestions = frontier?.suggestions ?? [];

  const chips = (
    <div class="scopes" role="group" aria-label="Portée">
      {scopes.map((s) => (
        <button key={s.id} class={`chip${s.id === scope ? " on" : ""}`} aria-pressed={s.id === scope} onClick={() => setScope(s.id)}>{s.name}</button>
      ))}
      <button class={`chip all${all ? " on" : ""}`} aria-pressed={all} onClick={() => setAll(!all)}>Tout voir</button>
    </div>
  );

  if (all) {
    const order = { progress: 0, explore: 1, idea: 2, acquired: 3 };
    const label = { progress: "en cours", explore: "à explorer", idea: "idée", acquired: "acquis" };
    const items = universe.galaxies.flatMap((g) => g.clusters.flatMap((c) =>
      c.stars.filter((s) => s.state !== "acquired" && inScope(s.id)).map((s) => ({ s, g }))));
    items.sort((a, b) => order[a.s.state] - order[b.s.state]);
    return (
      <div>
        {chips}
        {!items.length && <p class="empty">{t.allExplored}</p>}
        {items.map(({ s, g }) => (
          <div key={s.id} class="todo">
            {dot(s, g.hue)}
            <button onClick={() => go(s.id)}>{s.name}<small>{g.name} · {label[s.state]}</small></button>
          </div>
        ))}
      </div>
    );
  }

  return (
    <div>
      {chips}
      {err && <p class="empty">{err}</p>}
      {frontier && !suggestions.length && <p class="empty">Aucun sujet en idée ou à explorer sous le niveau 3 ici. Ajoute-en ou élargis la portée.</p>}
      {frontier && buckets.map((b) => {
        const items = suggestions.filter((s) => s.bucket === b.key);
        if (!items.length) return null;
        return (
          <section key={b.key} class="fgroup">
            <h4>{b.label}<span class="n">{frontier.budget[b.key]} %</span></h4>
            {items.map((s) => (
              <div key={s.id} class="todo">
                {dot(s, hue.get(s.id.split("/")[0]) ?? 0)}
                <button onClick={() => go(s.id)}>
                  {s.name}
                  <small>{s.reason}</small>
                </button>
              </div>
            ))}
          </section>
        );
      })}
      {progress.length > 0 && (
        <section class="fgroup">
          <h4>En cours<span class="n">{progress.length}</span></h4>
          {progress.map(({ s, g }) => (
            <div key={s.id} class="todo">
              {dot(s, g.hue)}
              <button onClick={() => go(s.id)}>{s.name}<small>{g.name} · niveau {s.level}</small></button>
            </div>
          ))}
        </section>
      )}
    </div>
  );
}

function Inbox({ version }: { version: number }) {
  const [items, setItems] = useState<InboxItem[]>([]);
  const [text, setText] = useState("");
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState("");
  useEffect(() => { api.inbox().then(setItems).catch((e) => setErr(String(e))); }, [version]);
  const save = async () => {
    if (!text.trim()) return;
    setBusy(true);
    try {
      await api.capture(text);
      setText("");
      setItems(await api.inbox());
      setErr("");
    } catch (e) {
      setErr(String(e));
    } finally {
      setBusy(false);
    }
  };
  return (
    <div>
      <div class="capture">
        <textarea placeholder="Une idée, un lien, un sujet à creuser…" value={text} onInput={(e) => setText(e.currentTarget.value)}
          onKeyDown={(e) => { if (e.key === "Enter" && (e.ctrlKey || e.metaKey)) save(); }} aria-label="Capture" />
        <div class="row"><button class="btn" disabled={busy || !text.trim()} onClick={save}>Capturer</button><span class="kbd">Ctrl Entrée</span></div>
        {err && <p class="empty">{err}</p>}
      </div>
      {items.length === 0 && <p class="empty">Boîte vide. Un agent peut aussi y déposer des notes avec capture().</p>}
      {items.map((it) => (
        <div key={it.name} class="note"><small>{it.captured || it.name}</small><p>{it.text}</p></div>
      ))}
    </div>
  );
}
