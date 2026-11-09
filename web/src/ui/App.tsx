import { useCallback, useEffect, useRef, useState } from "preact/hooks";
import { api, type UniverseView } from "../api";
import { AtlasView } from "../atlas/atlas";
import { Sky, type SkyNode } from "../sky/engine";
import { pathOf } from "../sky/layout";
import { Left } from "./Left";
import { Review } from "./Review";
import { Right } from "./Right";
import { dueIn, excluded, scopeOf } from "./scope";
import { applyTheme, prefs, type Mode, type ViewName } from "./prefs";
import { TermsContext, termsFor } from "./terms";

type MapView = Sky | AtlasView;

export function App() {
  const glRef = useRef<HTMLCanvasElement>(null);
  const skyRef = useRef<HTMLCanvasElement>(null);
  const atlasRef = useRef<HTMLCanvasElement>(null);
  const sky = useRef<MapView | null>(null);
  const [view, setView] = useState<ViewName>(prefs.view.get());
  const [mode, setMode] = useState<Mode>(prefs.mode.get());
  const [universe, setUniverse] = useState<UniverseView | null>(null);
  const [selected, setSelected] = useState<SkyNode | null>(null);
  const [scale, setScale] = useState("");
  const [fps, setFps] = useState(0);
  const [reviewing, setReviewing] = useState(false);
  const [webgl, setWebgl] = useState(true);
  const [err, setErr] = useState("");

  const load = useCallback(() => {
    api.universe().then((u) => { setUniverse(u); setErr(""); }).catch((e) => setErr(String(e)));
  }, []);

  useEffect(() => {
    load();
    // The server pushes a new version whenever a file changes on disk, from the app, an editor or git pull.
    const es = new EventSource("/api/events");
    es.addEventListener("reload", load);
    return () => es.close();
  }, []);

  useEffect(() => applyTheme(view, mode), [view, mode]);

  const universeRef = useRef<UniverseView | null>(null);
  const selectedRef = useRef<SkyNode | null>(null);
  universeRef.current = universe;
  selectedRef.current = selected;

  useEffect(() => {
    const ev = { onSelect: setSelected, onScale: setScale, onFps: setFps };
    const s: MapView = view === "atlas" ? new AtlasView(atlasRef.current!, ev) : new Sky(glRef.current!, skyRef.current!, ev);
    sky.current = s;
    setWebgl(s instanceof Sky ? s.webgl : true);
    // Switching views keeps the subject in focus: the new map opens where the old one was.
    if (universeRef.current) {
      s.setUniverse(universeRef.current);
      const id = selectedRef.current?.id ?? null;
      if (s instanceof Sky) setTimeout(() => s.select(id, true), 400);
      else s.select(id, false);
    }
    return () => s.destroy();
  }, [view]);

  const first = useRef(true);
  const initialHash = useRef(location.hash);
  useEffect(() => {
    if (!universe) return;
    sky.current?.setUniverse(universe);
    if (first.current) {
      first.current = false;
      const id = decodeURIComponent(initialHash.current.replace(/^#\/?/, ""));
      if (id) setTimeout(() => sky.current?.select(id, true), 400);
    }
  }, [universe]);

  // The hash mirrors the selection so a subject can be bookmarked or opened from a terminal.
  useEffect(() => {
    if (first.current) return;
    const h = selected ? "#/" + selected.id.split("/").map(encodeURIComponent).join("/") : " ";
    history.replaceState(null, "", h === " " ? location.pathname : h);
  }, [selected]);

  const go = useCallback((id: string) => sky.current?.select(id, true), []);
  const scope = scopeOf(selected);
  const due = dueIn(universe, scope.id, excluded.get(scope.id));
  const crumbs = selected ? pathOf(selected) : [];
  const problems = universe?.problems ?? [];

  const t = termsFor(view);
  return (
    <TermsContext.Provider value={{ view, t }}>
    <div class="app">
      <Left universe={universe} selected={selected} go={go} />
      <div class="center">
        {view === "galaxy" ? (
          <>
            <canvas key="gl" ref={glRef} class="gl" aria-hidden="true" />
            <canvas key="sky" ref={skyRef} class="sky" aria-label="Univers des connaissances. La liste à gauche permet de naviguer au clavier." />
          </>
        ) : (
          <canvas key="atlas" ref={atlasRef} class="atlas" aria-label="Atlas des connaissances. La liste à gauche permet de naviguer au clavier." />
        )}
        {!webgl && <p class="nogl">WebGL2 indisponible : seules les étoiles de connaissance sont dessinées.</p>}
        <div class="hud">
          <nav class="crumbs glass" aria-label="Position">
            <button onClick={() => sky.current?.select(null, true)}>{t.root}</button>
            {crumbs.map((n) => <span key={n.id} style={{ display: "contents" }}><span>›</span><button onClick={() => go(n.id)}>{n.name}</button></span>)}
          </nav>
          <div class="tools">
            <div class="seg glass" role="group" aria-label="Zoom">
              <button aria-label="Dézoomer" onClick={() => sky.current?.zoomBy(1 / 1.8)}>−</button>
              <button onClick={() => sky.current?.select(null, true)}>{t.root}</button>
              <button aria-label="Zoomer" onClick={() => sky.current?.zoomBy(1.8)}>+</button>
            </div>
            <span class="lvl glass">{scale}</span>
            <div class="seg glass" role="group" aria-label="Vue">
              <button aria-pressed={view === "galaxy"} onClick={() => { prefs.view.set("galaxy"); setView("galaxy"); }}>Galaxie</button>
              <button aria-pressed={view === "atlas"} onClick={() => { prefs.view.set("atlas"); setView("atlas"); }}>Atlas</button>
            </div>
            {view === "atlas" && (
              <div class="seg glass">
                <button aria-label={mode === "dark" ? "Thème clair" : "Thème sombre"} title={mode === "dark" ? "Thème clair" : "Thème sombre"}
                  onClick={() => { const m: Mode = mode === "dark" ? "light" : "dark"; prefs.mode.set(m); setMode(m); }}>
                  {mode === "dark"
                    ? <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" aria-hidden="true"><circle cx="12" cy="12" r="4" /><path d="M12 2v2M12 20v2M4.9 4.9l1.4 1.4M17.7 17.7l1.4 1.4M2 12h2M20 12h2M4.9 19.1l1.4-1.4M17.7 6.3l1.4-1.4" /></svg>
                    : <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" aria-hidden="true"><path d="M21 12.8A9 9 0 1 1 11.2 3a7 7 0 0 0 9.8 9.8z" /></svg>}
                </button>
              </div>
            )}
            <button class="revbtn" onClick={() => setReviewing(!reviewing)}>Réviser · {due}</button>
          </div>
        </div>
        {reviewing && <Review universe={universe} scope={scope} onClose={() => { setReviewing(false); load(); }} />}
        {err && <div class="banner glass"><b>Serveur injoignable.</b> {err}</div>}
        {!err && problems.length > 0 && !reviewing && (
          <details class="banner glass">
            <summary><b>{problems.length} fichier{problems.length > 1 ? "s" : ""} à corriger</b></summary>
            <ul>{problems.map((p) => <li key={p.path + p.message}>{p.path} : {p.message}</li>)}</ul>
          </details>
        )}
        <span class="hint">glisser · molette ou pincer · cliquer{view === "atlas" ? " · Échap pour remonter" : ""}</span>
        {view === "galaxy" && <span class="fps">{fps} i/s</span>}
        {view === "atlas" && (
          <div class="legend glass" aria-label="Légende">
            <span>Niveau</span>
            {[1, 2, 3, 4, 5].map((l) => <i key={l} style={{ background: `hsl(215 ${mode === "dark" ? 52 : 62}% ${mode === "dark" ? 24 + l * 7 : 86 - l * 7}%)` }} title={`Niveau ${l}`} />)}
            <i class="fog" title="À explorer" /><span>à explorer</span>
            <i class="stale" title="À revoir" /><span>à revoir</span>
          </div>
        )}
      </div>
      <Right universe={universe} selected={selected} go={go} review={() => setReviewing(true)} />
    </div>
    </TermsContext.Provider>
  );
}
