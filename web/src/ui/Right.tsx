import { useEffect, useRef, useState } from "preact/hooks";
import { api, type AIPreview, type Discovery, type GalaxyView, type Source, type State, type StarView, type UniverseView } from "../api";
import { renderMarkdown, renderMermaid, section } from "../markdown";
import type { SkyNode } from "../sky/engine";
import { buildLayout, currentPins } from "../sky/layout";
import { SHARES, STEP, rebalance } from "./discovery";
import { pageOf, sourceHref, sourceLabel } from "./source";
import { useTerms } from "./terms";

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
  const { t, view } = useTerms();
  if (!universe) return <p>Chargement…</p>;
  if (!selected) {
    const stars = universe.galaxies.flatMap((g) => g.clusters.flatMap((c) => c.stars));
    const strongest = [...stars].filter((s) => s.level >= 4).slice(0, 3);
    return (
      <>
        <span class="path">{t.root}</span>
        <h3>{t.rootTitle}</h3>
        <p>{t.summary(stars.filter((s) => s.level > 0).length, stars.filter((s) => s.level === 0).length)}, {t.counts(universe.galaxies.reduce((a, g) => a + g.clusters.length, 0), universe.galaxies.length)}</p>
        {strongest.length > 0 && (
          <div class="blk"><span class="k">{t.strongest}</span>
            <div class="row">{strongest.map((s) => <button key={s.id} class="btn ghost" onClick={() => go(s.id)}>{s.name}</button>)}</div>
          </div>
        )}
        {view === "galaxy" && <PinMap universe={universe} />}
        <p style={{ fontSize: ".78rem", color: "var(--muted)" }}>{t.hint} Les fiches sont des fichiers Markdown dans ton repo : écris-les dans ton éditeur, la carte se met à jour seule.</p>
      </>
    );
  }
  if (selected.kind === "galaxy") {
    const g = selected.data, stars = g.clusters.flatMap((c) => c.stars);
    return (
      <>
        <span class="path">{t.theme}</span>
        <h3>{g.name}</h3>
        <p>{t.summary(stars.filter((s) => s.level > 0).length, stars.filter((s) => s.level === 0).length)}, {stars.reduce((a, s) => a + s.cards, 0)} cartes.</p>
        <div class="blk"><span class="k">Sous-thèmes</span>
          {g.clusters.map((c) => <button key={c.id} class="worm plain" onClick={() => go(c.id)}>{c.name}<small>{c.stars.length} sujets</small></button>)}
        </div>
        <GalaxySettings key={g.id + JSON.stringify([g.review_after_days, g.discovery])} g={g} />
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
        <div class="blk"><span class="k">{t.byLevel}</span>
          {stars.map((s) => (
            <button key={s.id} class="worm plain" onClick={() => go(s.id)}>
              <span class={`dot${s.level ? "" : " fog"}`} style={s.level ? { background: `hsl(${s.hue},80%,${45 + s.level * 8}%)` } : undefined} />
              {s.name}<small>{s.level ? LEVELS[s.level] : t.fog}</small>
            </button>
          ))}
        </div>
      </>
    );
  }
  const starId = selected.kind === "planet" ? selected.star.id : selected.id;
  return <StarFiche id={starId} planet={selected.kind === "planet" ? selected.name : null} version={universe.version} go={go} review={review} />;
}

function PinMap({ universe }: { universe: UniverseView }) {
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState("");
  const free = universe.galaxies.filter((g) => !g.pin);
  if (universe.galaxies.length === 0) return null;
  const pin = async () => {
    setBusy(true);
    try {
      await api.setLayout(currentPins(buildLayout(universe)));
      setErr("");
    } catch (e) {
      setErr(String(e));
    } finally {
      setBusy(false);
    }
  };
  return (
    <div class="blk"><span class="k">Carte</span>
      {free.length === 0
        ? <p>Carte figée : une nouvelle galaxie se place autour des autres sans les déplacer.</p>
        : free.length === universe.galaxies.length
          ? <p>Les galaxies sont placées automatiquement et bougent un peu quand un thème apparaît. Fige-les pour garder tes repères.</p>
          : <p>{free.length} galaxie{free.length > 1 ? "s" : ""} pas encore figée{free.length > 1 ? "s" : ""} : {free.map((g) => g.name).join(", ")}.</p>}
      {free.length > 0 && <button class="btn ghost" disabled={busy} onClick={pin}>Figer la carte</button>}
      {err && <p class="file">{err}</p>}
      <p class="file">Positions dans layout.json, à la racine du repo.</p>
    </div>
  );
}

function GalaxySettings({ g }: { g: GalaxyView }) {
  const [disc, setDisc] = useState<Discovery>(g.discovery);
  const [days, setDays] = useState(g.review_after_days);
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState("");
  const dirty = days !== g.review_after_days || SHARES.some(([k]) => disc[k] !== g.discovery[k]) || disc.news !== g.discovery.news;
  const save = async () => {
    setBusy(true);
    try {
      await api.setGalaxy(g.id, { review_after_days: days, discovery: disc });
      setErr("");
    } catch (e) {
      setErr(String(e));
    } finally {
      setBusy(false);
    }
  };
  return (
    <div class="blk settings"><span class="k">Réglages</span>
      <label class="field">À revoir après
        <input type="number" min={1} max={3650} value={days} onInput={(e) => setDays(Math.max(1, Math.min(3650, Number((e.target as HTMLInputElement).value) || 1)))} /> jours sans révision
      </label>
      <span class="k">Découverte</span>
      {SHARES.map(([k, label]) => (
        <label key={k} class="share">
          <span>{label}</span>
          <input type="range" min={0} max={100} step={STEP} value={disc[k]} aria-label={`${label}, en pourcentage`}
            onInput={(e) => setDisc(rebalance(disc, k, Number((e.target as HTMLInputElement).value)))} />
          <b>{disc[k]} %</b>
        </label>
      ))}
      <label class="field"><input type="checkbox" checked={disc.news} onChange={(e) => setDisc({ ...disc, news: (e.target as HTMLInputElement).checked })} /> Inclure l'actualité du thème</label>
      <div class="row">
        <button class="btn" disabled={!dirty || busy} onClick={save}>Enregistrer</button>
        {dirty && <button class="btn ghost" onClick={() => { setDisc(g.discovery); setDays(g.review_after_days); }}>Annuler</button>}
      </div>
      {err && <p class="file">{err}</p>}
      <p class="file">Écrit dans galaxies/{g.id}/_galaxy.md</p>
    </div>
  );
}

function StarFiche({ id, planet, version, go, review }: { id: string; planet: string | null; version: number; go: (id: string) => void; review: () => void }) {
  const { t } = useTerms();
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
        {star.source && <SourceLine src={star.source} planet={planet} />}
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
        <div style={{ display: "grid", gap: "6px" }}><span class="k">{t.links}</span>
          {star.link_refs.map((l) => <button key={l.id} class="worm" onClick={() => go(l.id)}><i />{l.name}<small>{l.galaxy} · {t.traverse}</small></button>)}
        </div>
      )}
      {!!star.evidence?.length && (
        <div><span class="k">Preuves</span>
          <ul class="md">{star.evidence.map((e) => <li key={e}>{/^https?:\/\//.test(e) ? <a href={e} target="_blank" rel="noreferrer">{e}</a> : e}</li>)}</ul>
        </div>
      )}
      {star.source && <SourceLine src={star.source} planet={planet} />}
      {star.cards > 0 && <div class="row"><span class="k">{star.cards} cartes · {star.due} à revoir</span><button class="btn ghost" onClick={review}>{t.reviewOne}</button></div>}
      <p class="file">{star.file}</p>
    </>
  );
}

function SourceLine({ src, planet }: { src: Source; planet: string | null }) {
  const page = pageOf(planet);
  const href = sourceHref(src, page);
  return (
    <div class="row"><span class="k">Source</span>
      {href ? <a class="srcl" href={href} target="_blank" rel="noreferrer">{sourceLabel(src, page)}</a> : <span class="srcl">{sourceLabel(src, page)}</span>}
      {src.file && !src.available && <small class="file">fichier absent de cette machine ({src.file})</small>}
    </div>
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
