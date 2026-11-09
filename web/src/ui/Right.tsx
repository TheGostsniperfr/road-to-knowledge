import { useEffect, useRef, useState } from "preact/hooks";
import { api, type AIPreview, type State, type StarView, type UniverseView } from "../api";
import { renderMarkdown, renderMermaid, section } from "../markdown";
import type { SkyNode } from "../sky/engine";

const LEVELS = ["Brouillard", "Entendu parler", "Compris", "Pratiqué en lab", "Maîtrisé en prod", "Je peux l'enseigner"];
const STATES: [State, string][] = [["idea", "Idée"], ["explore", "À explorer"], ["progress", "En cours"], ["acquired", "Acquis"]];

export function Right(props: { universe: UniverseView | null; selected: SkyNode | null; go: (id: string) => void; review: () => void }) {
  const [tab, setTab] = useState<"fiche" | "ai">("fiche");
  return (
    <aside class="right" aria-label="Fiche">
      <div class="tabs" role="tablist">
        <button role="tab" aria-selected={tab === "fiche"} onClick={() => setTab("fiche")}>Fiche</button>
        <button role="tab" aria-selected={tab === "ai"} onClick={() => setTab("ai")}>Ce que l'IA reçoit</button>
      </div>
      <div class="rbody" aria-live="polite">{tab === "fiche" ? <Fiche {...props} /> : <AI selected={props.selected} version={props.universe?.version ?? 0} />}</div>
    </aside>
  );
}

function Fiche({ universe, selected, go, review }: { universe: UniverseView | null; selected: SkyNode | null; go: (id: string) => void; review: () => void }) {
  if (!universe) return <p>Chargement…</p>;
  if (!selected) {
    const stars = universe.galaxies.flatMap((g) => g.clusters.flatMap((c) => c.stars));
    const strongest = [...stars].filter((s) => s.level >= 4).slice(0, 3);
    return (
      <>
        <span class="path">Univers</span>
        <h3>Ton univers</h3>
        <p>{stars.filter((s) => s.level > 0).length} étoiles explorées, {stars.filter((s) => s.level === 0).length} dans le brouillard, {universe.galaxies.reduce((a, g) => a + g.clusters.length, 0)} amas, {universe.galaxies.length} galaxies.</p>
        {strongest.length > 0 && (
          <div class="blk"><span class="k">Tes étoiles les plus brillantes</span>
            <div class="row">{strongest.map((s) => <button key={s.id} class="btn ghost" onClick={() => go(s.id)}>{s.name}</button>)}</div>
          </div>
        )}
        <p style={{ fontSize: ".78rem", color: "var(--muted)" }}>Clique une galaxie pour y plonger. Les fiches sont des fichiers Markdown dans ton repo : écris-les dans ton éditeur, la carte se met à jour seule.</p>
      </>
    );
  }
  if (selected.kind === "galaxy") {
    const g = selected.data, stars = g.clusters.flatMap((c) => c.stars);
    return (
      <>
        <span class="path">Galaxie</span>
        <h3>{g.name}</h3>
        <p>{stars.filter((s) => s.level > 0).length} étoiles explorées, {stars.filter((s) => s.level === 0).length} dans le brouillard, {stars.reduce((a, s) => a + s.cards, 0)} cartes.</p>
        <div class="blk"><span class="k">Sous-thèmes</span>
          {g.clusters.map((c) => <button key={c.id} class="worm plain" onClick={() => go(c.id)}>{c.name}<small>{c.stars.length} sujets</small></button>)}
        </div>
        <div class="blk"><span class="k">Réglages</span>
          <p>À revoir après {g.review_after_days} jours sans révision. Découverte : {g.discovery.near} % proche, {g.discovery.two_hops} % à deux sauts, {g.discovery.off_piste} % hors-piste.</p>
          <p class="file">Modifiable dans galaxies/{g.id}/_galaxy.md</p>
        </div>
      </>
    );
  }
  if (selected.kind === "cluster") {
    const stars = [...selected.stars].sort((a, b) => b.level - a.level);
    return (
      <>
        <span class="path">{selected.g.name}</span>
        <h3>{selected.name}</h3>
        <p>{stars.length} sujets · {stars.reduce((a, s) => a + s.data.cards, 0)} cartes</p>
        <div class="blk"><span class="k">Étoiles par maîtrise</span>
          {stars.map((s) => (
            <button key={s.id} class="worm plain" onClick={() => go(s.id)}>
              <span class={`dot${s.level ? "" : " fog"}`} style={s.level ? { background: `hsl(${s.hue},80%,${45 + s.level * 8}%)` } : undefined} />
              {s.name}<small>{s.level ? LEVELS[s.level] : "brouillard"}</small>
            </button>
          ))}
        </div>
      </>
    );
  }
  const starId = selected.kind === "planet" ? selected.star.id : selected.id;
  return <StarFiche id={starId} planet={selected.kind === "planet" ? selected.name : null} version={universe.version} go={go} review={review} />;
}

function StarFiche({ id, planet, version, go, review }: { id: string; planet: string | null; version: number; go: (id: string) => void; review: () => void }) {
  const [star, setStar] = useState<StarView | null>(null);
  const [err, setErr] = useState("");
  const body = useRef<HTMLDivElement>(null);

  useEffect(() => {
    let live = true;
    api.star(id).then((s) => { if (live) { setStar(s); setErr(""); } }).catch((e) => live && setErr(String(e)));
    return () => { live = false; };
  }, [id, version]);

  const md = star ? (planet ? section(star.body, planet) : star.body) : "";
  useEffect(() => { if (body.current) renderMermaid(body.current); }, [md]);

  if (err) return <p>{err}</p>;
  if (!star) return <p>Chargement…</p>;
  const set = async (patch: { level?: number; state?: State }) => {
    try {
      setStar(await api.setMeta(star.id, patch));
    } catch (e) {
      setErr(String(e));
    }
  };

  if (planet) {
    return (
      <>
        <span class="path">{[...star.path, star.name].join(" › ")}</span>
        <h3>{planet}</h3>
        <div class="md" ref={body} dangerouslySetInnerHTML={{ __html: renderMarkdown(md || "_Notion à écrire._") }} />
        <button class="btn ghost" onClick={() => go(star.id)}>← {star.name}</button>
      </>
    );
  }
  return (
    <>
      <span class="path">{star.path.join(" › ")}</span>
      <h3>{star.name}</h3>
      <div style={{ display: "grid", gap: "6px" }}>
        <span class="k">Niveau {star.level} · {LEVELS[star.level]}</span>
        <div class="lvbar" role="group" aria-label="Niveau">
          {[1, 2, 3, 4, 5].map((i) => (
            <button key={i} class={i <= star.level ? "on" : ""} title={LEVELS[i]} aria-label={`Niveau ${i} : ${LEVELS[i]}`}
              onClick={() => set({ level: i === star.level ? i - 1 : i })} />
          ))}
        </div>
      </div>
      <div class="state" role="group" aria-label="État">
        {STATES.map(([k, l]) => <button key={k} aria-pressed={star.state === k} onClick={() => set({ state: k })}>{l}</button>)}
      </div>
      {star.stale && <div class="fresh"><i />Pas revu depuis {star.days} j. Ton niveau ne bouge pas.</div>}
      <div class="md" ref={body} dangerouslySetInnerHTML={{ __html: renderMarkdown(md || "_Fiche à écrire. Ouvre le fichier ci-dessous dans ton éditeur, ou demande un brouillon à un agent avec propose()._") }} />
      {!!star.planets?.length && (
        <div><span class="k">Notions</span>
          <div class="row" style={{ marginTop: "6px" }}>{star.planets.map((p) => <button key={p.id} class="tagn" onClick={() => go(p.id)}>{p.name}</button>)}</div>
        </div>
      )}
      {!!star.link_refs?.length && (
        <div style={{ display: "grid", gap: "6px" }}><span class="k">Trous de ver</span>
          {star.link_refs.map((l) => <button key={l.id} class="worm" onClick={() => go(l.id)}><i />{l.name}<small>{l.galaxy} · traverser</small></button>)}
        </div>
      )}
      {!!star.evidence?.length && (
        <div><span class="k">Preuves</span>
          <ul class="md">{star.evidence.map((e) => <li key={e}>{/^https?:\/\//.test(e) ? <a href={e} target="_blank" rel="noreferrer">{e}</a> : e}</li>)}</ul>
        </div>
      )}
      {star.cards > 0 && <div class="row"><span class="k">{star.cards} cartes · {star.due} à revoir</span><button class="btn ghost" onClick={review}>Réviser cette étoile</button></div>}
      <p class="file">{star.file}</p>
    </>
  );
}

function AI({ selected, version }: { selected: SkyNode | null; version: number }) {
  const [res, setRes] = useState<AIPreview | null>(null);
  const [err, setErr] = useState("");
  const n = selected?.kind === "planet" ? selected.star : selected;
  const tool = !n ? "profile" : n.kind === "star" ? "node" : "subgraph";
  const id = n?.id ?? "";
  useEffect(() => {
    api.ai(tool, id).then((r) => { setRes(r); setErr(""); }).catch((e) => setErr(String(e)));
  }, [tool, id, version]);
  if (err) return <p>{err}</p>;
  if (!res) return <p>Chargement…</p>;
  const ratio = res.full_tokens ? Math.max(1.5, (res.tokens / res.full_tokens) * 100) : 100;
  return (
    <>
      <span class="path">Ce que l'IA reçoit</span>
      <h3>{n?.name ?? "Profil"}</h3>
      <p>Un agent n'ouvre jamais tout l'univers. Il appelle un outil MCP et reçoit seulement ceci.</p>
      <pre class="mcp"><b>→ {res.call}</b>{"\n"}{res.payload}</pre>
      <div class="blk"><span class="k">Coût</span>
        <div class="tokbar"><i style={{ width: `${Math.min(100, ratio)}%` }} /></div>
        <p class="file">~{res.tokens} tokens ici, contre ~{res.full_tokens} pour tout le graphe avec les fiches.</p>
      </div>
    </>
  );
}
