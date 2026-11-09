import { useEffect, useRef, useState } from "preact/hooks";
import { api, type Hit, type InboxItem, type StarSummary, type UniverseView } from "../api";
import type { SkyNode } from "../sky/engine";

type Tab = "plan" | "todo" | "inbox";

const dot = (s: StarSummary, hue: number) =>
  s.level === 0 ? <span class="dot fog" /> : <span class={`dot${s.stale ? " stale" : ""}`} style={{ background: `hsl(${hue},80%,${45 + s.level * 8}%)` }} />;

export function Left({ universe, selected, go }: { universe: UniverseView | null; selected: SkyNode | null; go: (id: string) => void }) {
  const [tab, setTab] = useState<Tab>("plan");
  const [query, setQuery] = useState("");
  const [hits, setHits] = useState<Hit[]>([]);
  const input = useRef<HTMLInputElement>(null);

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
        <button role="tab" aria-selected={tab === "inbox"} onClick={() => setTab("inbox")}>
          Boîte {universe?.inbox ? <span class="n">{universe.inbox}</span> : null}
        </button>
      </div>
      <div class="search">
        <input ref={input} type="search" placeholder="Rechercher (Ctrl K)" value={query} aria-label="Rechercher"
          onInput={(e) => { setQuery(e.currentTarget.value); setTab("plan"); }}
          onKeyDown={(e) => { if (e.key === "Enter" && hits[0]) go(hits[0].id); }} />
      </div>
      <div class="lbody">
        {tab === "plan" && (query.trim().length >= 2 ? <Hits hits={hits} go={go} /> : <Tree universe={universe} selId={selId} open={openPath} go={go} />)}
        {tab === "todo" && <Todo universe={universe} go={go} />}
        {tab === "inbox" && <Inbox version={universe?.version ?? 0} />}
      </div>
    </aside>
  );
}

function Tree({ universe, selId, open, go }: { universe: UniverseView | null; selId?: string; open: Set<string>; go: (id: string) => void }) {
  if (!universe) return <p class="empty">Chargement…</p>;
  if (!universe.galaxies.length) return <p class="empty">Aucune galaxie. Ajoute un dossier dans galaxies/ ou lance la skill seed.</p>;
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
                    {s.links?.length ? <span class="n" title="trous de ver">⟡</span> : s.state === "progress" ? <span class="n">en cours</span> : null}
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

function Hits({ hits, go }: { hits: Hit[]; go: (id: string) => void }) {
  if (!hits.length) return <p class="empty">Rien trouvé.</p>;
  return (
    <div>
      {hits.map((h) => (
        <button key={h.id} class="hit" onClick={() => go(h.id)}>
          <span>{h.name}</span>
          <small>{h.snippet.replace(/[\[\]]/g, "")}</small>
        </button>
      ))}
    </div>
  );
}

function Todo({ universe, go }: { universe: UniverseView | null; go: (id: string) => void }) {
  if (!universe) return null;
  const items = universe.galaxies.flatMap((g) => g.clusters.flatMap((c) => c.stars.filter((s) => s.state !== "acquired").map((s) => ({ s, g }))));
  const order = { progress: 0, explore: 1, idea: 2, acquired: 3 };
  items.sort((a, b) => order[a.s.state] - order[b.s.state]);
  const label = { progress: "en cours", explore: "à explorer", idea: "idée", acquired: "acquis" };
  if (!items.length) return <p class="empty">Rien en attente. Le brouillard est dissipé.</p>;
  return (
    <div>
      {items.map(({ s, g }) => (
        <div key={s.id} class="todo">
          {dot(s, g.hue)}
          <button onClick={() => go(s.id)}>
            {s.name}
            <small>{g.name} · {label[s.state]}</small>
          </button>
        </div>
      ))}
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
