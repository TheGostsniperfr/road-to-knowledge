import { useEffect, useState } from "preact/hooks";
import { api, newsApi, type NewsItem, type NewsView, type UniverseView } from "../api";

export function ago(iso: string, now = Date.now()): string {
  const h = Math.floor((now - Date.parse(iso)) / 3_600_000);
  if (h < 1) return "à l'instant";
  if (h < 24) return `il y a ${h} h`;
  const d = Math.floor(h / 24);
  return d === 1 ? "hier" : `il y a ${d} j`;
}

export function keepText(it: NewsItem): string {
  const about = it.subjects.length ? `\nSujets : ${it.subjects.map((s) => s.name).join(", ")}` : "";
  return `${it.title}\n${it.link}\n\nVu dans ${it.source} (${it.scope_name}).${about}`;
}

export function News({ universe, go }: { universe: UniverseView | null; go: (id: string) => void }) {
  const [view, setView] = useState<NewsView | null>(null);
  const [all, setAll] = useState(false);
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState("");
  const [kept, setKept] = useState<Set<string>>(new Set());

  useEffect(() => {
    let live = true;
    newsApi.list({ all }).then((v) => { if (live) { setView(v); setErr(""); } }).catch((e) => { if (live) { setView(null); setErr(String(e)); } });
    return () => { live = false; };
  }, [all, universe?.version]);

  const refresh = async () => {
    setBusy(true);
    try {
      const rep = await newsApi.fetch();
      setView(await newsApi.list({ all }));
      setErr(rep.failed ? `${rep.failed} source${rep.failed > 1 ? "s" : ""} en erreur` : "");
    } catch (e) {
      setErr(String(e));
    } finally {
      setBusy(false);
    }
  };

  const keep = async (it: NewsItem) => {
    try {
      await api.capture(keepText(it));
      setKept(new Set(kept).add(it.id));
    } catch (e) {
      setErr(String(e));
    }
  };

  if (!universe) return null;
  const hue = new Map(universe.galaxies.map((g) => [g.id, g.hue]));
  const name = new Map(universe.galaxies.map((g) => [g.id, g.name]));
  const groups = new Map<string, NewsItem[]>();
  for (const it of view?.items ?? []) groups.set(it.galaxy, [...(groups.get(it.galaxy) ?? []), it]);

  return (
    <div>
      <div class="scopes" role="group" aria-label="Filtre">
        <button class={`chip${all ? "" : " on"}`} aria-pressed={!all} onClick={() => setAll(false)}>Mes sujets</button>
        <button class={`chip${all ? " on" : ""}`} aria-pressed={all} onClick={() => setAll(true)}>Tout</button>
        <button class="chip all" disabled={busy} onClick={refresh}>{busy ? "…" : "Actualiser"}</button>
      </div>
      {err && <p class="empty">{err}</p>}
      {view && view.sources.length === 0 && (
        <p class="empty">Aucune source suivie. Ajoute des flux RSS ou Atom dans <code>galaxies/&lt;thème&gt;/_galaxy.md</code> :<br />
          <code class="snippet">{"sources:\n  - url: https://go.dev/blog/feed.atom\n    name: Go blog"}</code></p>
      )}
      {view && view.sources.length > 0 && view.items.length === 0 && (
        <p class="empty">Rien de neuf sur 30 jours{view.hidden ? ` qui parle de tes sujets (${view.hidden} autre${view.hidden > 1 ? "s" : ""} dans « Tout »)` : ""}.</p>
      )}
      {[...groups].map(([gid, items]) => (
        <section key={gid} class="fgroup">
          <h4><span class="dot" style={{ background: `hsl(${hue.get(gid) ?? 0},80%,65%)`, marginRight: "6px" }} />{name.get(gid) ?? gid}<span class="n">{items.length}</span></h4>
          {items.map((it) => (
            <article key={it.id} class="newsitem">
              <a href={it.link} target="_blank" rel="noopener noreferrer">{it.title}</a>
              <small>{it.source} · {ago(it.published)}</small>
              {it.summary && <p>{it.summary}</p>}
              <div class="row">
                {it.subjects.map((s) => <button key={s.id} class="tagn" onClick={() => go(s.id)}>{s.kind === "cluster" ? s.name : `${s.name} · ${s.level}`}</button>)}
                <button class="chip keep" disabled={kept.has(it.id)} onClick={() => keep(it)}>{kept.has(it.id) ? "Gardé" : "Garder"}</button>
              </div>
            </article>
          ))}
        </section>
      ))}
      {view && view.hidden > 0 && !all && view.items.length > 0 && (
        <p class="empty">{view.hidden} article{view.hidden > 1 ? "s" : ""} sans lien avec tes sujets, visibles dans « Tout ».</p>
      )}
      {view && view.sources.length > 0 && (
        <details class="sources">
          <summary>{view.sources.length} source{view.sources.length > 1 ? "s" : ""}</summary>
          {view.sources.map((s) => (
            <div key={s.url} class={`src${s.error ? " bad" : ""}`}>
              <b>{s.name}</b><small>{s.scope_name} · {s.error ? s.error : s.last_ok ? `lu ${ago(s.last_ok)}` : "jamais lu"}</small>
            </div>
          ))}
        </details>
      )}
    </div>
  );
}
