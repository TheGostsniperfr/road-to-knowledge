import { useCallback, useEffect, useRef, useState } from "preact/hooks";
import { api, type UniverseView } from "../api";
import { Sky, type ScaleName, type SkyNode } from "../sky/engine";
import { pathOf } from "../sky/layout";
import { Left } from "./Left";
import { Review } from "./Review";
import { Right } from "./Right";
import { dueIn, excluded, scopeOf } from "./scope";

export function App() {
  const glRef = useRef<HTMLCanvasElement>(null);
  const skyRef = useRef<HTMLCanvasElement>(null);
  const sky = useRef<Sky | null>(null);
  const [universe, setUniverse] = useState<UniverseView | null>(null);
  const [selected, setSelected] = useState<SkyNode | null>(null);
  const [scale, setScale] = useState<ScaleName>("Univers");
  const [fps, setFps] = useState(0);
  const [reviewing, setReviewing] = useState(false);
  const [webgl, setWebgl] = useState(true);
  const [err, setErr] = useState("");

  const load = useCallback(() => {
    api.universe().then((u) => { setUniverse(u); setErr(""); }).catch((e) => setErr(String(e)));
  }, []);

  useEffect(() => {
    const s = new Sky(glRef.current!, skyRef.current!, { onSelect: setSelected, onScale: setScale, onFps: setFps });
    sky.current = s;
    setWebgl(s.webgl);
    load();
    // The server pushes a new version whenever a file changes on disk, from the app, an editor or git pull.
    const es = new EventSource("/api/events");
    es.addEventListener("reload", load);
    return () => { es.close(); s.destroy(); };
  }, []);

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

  return (
    <div class="app">
      <Left universe={universe} selected={selected} go={go} />
      <div class="center">
        <canvas ref={glRef} class="gl" aria-hidden="true" />
        <canvas ref={skyRef} class="sky" aria-label="Univers des connaissances. La liste à gauche permet de naviguer au clavier." />
        {!webgl && <p class="nogl">WebGL2 indisponible : seules les étoiles de connaissance sont dessinées.</p>}
        <div class="hud">
          <nav class="crumbs glass" aria-label="Position">
            <button onClick={() => sky.current?.select(null, true)}>Univers</button>
            {crumbs.map((n) => <span key={n.id} style={{ display: "contents" }}><span>›</span><button onClick={() => go(n.id)}>{n.name}</button></span>)}
          </nav>
          <div class="tools">
            <div class="seg glass" role="group" aria-label="Zoom">
              <button aria-label="Dézoomer" onClick={() => sky.current?.zoomBy(1 / 1.8)}>−</button>
              <button onClick={() => sky.current?.select(null, true)}>Univers</button>
              <button aria-label="Zoomer" onClick={() => sky.current?.zoomBy(1.8)}>+</button>
            </div>
            <span class="lvl glass">{scale}</span>
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
        <span class="hint">glisser · molette ou pincer · cliquer</span>
        <span class="fps">{fps} i/s</span>
      </div>
      <Right universe={universe} selected={selected} go={go} review={() => setReviewing(true)} />
    </div>
  );
}
