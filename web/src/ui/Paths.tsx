import { useEffect, useState } from "preact/hooks";
import { api, pathsApi, type PathSummary } from "../api";
import { continueTarget, inScope, inProgress, progress, requestText } from "./paths";

// Paths refetch on every universe version: the server bumps it after each commit.
export function usePaths(version: number | undefined): PathSummary[] {
  const [list, setList] = useState<PathSummary[]>([]);
  useEffect(() => {
    let live = true;
    pathsApi.list().then((l) => { if (live) setList(l); }).catch(() => undefined);
    return () => { live = false; };
  }, [version]);
  return list;
}

// The HUD keeps the courses under way one click away.
export function ContinuePills({ paths, onStudy }: { paths: PathSummary[]; onStudy: (path: string) => void }) {
  const going = inProgress(paths).slice(0, 3);
  if (!going.length) return null;
  return (
    <div class="resume" role="group" aria-label="Reprendre un parcours">
      {going.map((p) => (
        <button key={p.id} class="pill glass" onClick={() => onStudy(p.id)} title={`Reprendre ${p.name}`}>
          <span class="play" aria-hidden="true">▶</span>
          <span class="txt"><b>{p.name}</b><small>{p.next?.name}</small></span>
          <span class="pct">{p.done}/{p.total}</span>
        </button>
      ))}
    </div>
  );
}

export function PathsBlock({ paths, scope, onStudy, go }: { paths: PathSummary[]; scope: string; onStudy: (path: string) => void; go: (id: string) => void }) {
  const here = paths.filter((p) => inScope(p, scope));
  return (
    <div class="blk paths"><span class="k">{scope ? "Parcours" : "Reprendre"}</span>
      {here.map((p) => <PathCard key={p.id} p={p} onStudy={onStudy} go={go} />)}
      {here.length === 0 && <p class="hintp">Aucun parcours ici. Un parcours suit l'ordre d'un livre ou d'un cours et dit quelle est la prochaine étape.</p>}
      <Request />
    </div>
  );
}

function PathCard({ p, onStudy, go }: { p: PathSummary; onStudy: (path: string) => void; go: (id: string) => void }) {
  const target = continueTarget(p.next);
  if (!p.next) {
    return (
      <div class="pathdone">
        <span>✓</span><b>{p.name}</b><small>terminé</small>
        <button class="chip" onClick={() => onStudy(p.id)}>Revoir</button>
      </div>
    );
  }
  return (
    <article class="pathcard">
      <div class="pathtitle">{p.name}<span class="n">{p.done}/{p.total}</span></div>
      <Bar p={p} />
      <small>Prochaine étape : {p.next.name}</small>
      <div class="row">
        <button class="btn" onClick={() => onStudy(p.id)}>{p.done > 0 || p.at ? "Continuer" : "Commencer"}</button>
        {target && <button class="chip" onClick={() => go(target)}>Sur la carte</button>}
      </div>
    </article>
  );
}

function Bar({ p }: { p: { done: number; total: number } }) {
  const pct = progress(p);
  return <div class="bar" role="progressbar" aria-valuenow={pct} aria-valuemin={0} aria-valuemax={100}><i style={{ width: `${pct}%` }} /></div>;
}

// Nothing is generated on the spot: the request waits in the inbox until an agent writes the path.
function Request() {
  const [what, setWhat] = useState("");
  const [sent, setSent] = useState("");
  const [fail, setFail] = useState("");
  const send = async (e: Event) => {
    e.preventDefault();
    if (!what.trim()) return;
    try {
      await api.capture(requestText(what));
      setSent(what.trim());
      setWhat("");
      setFail("");
    } catch (err) {
      setSent("");
      setFail(String(err));
    }
  };
  return (
    <details class="request">
      <summary>Demander un parcours</summary>
      <form onSubmit={send}>
        <input value={what} onInput={(e) => setWhat(e.currentTarget.value)} placeholder="Un livre, une playlist, un cours…" aria-label="Livre ou cours" />
        <button class="btn ghost" disabled={!what.trim()}>Envoyer</button>
      </form>
      <small>La demande va dans l'inbox ; un agent prépare ensuite le parcours sur une branche à relire.</small>
      {sent && <small>Demande « {sent} » déposée.</small>}
      {fail && <small>{fail}</small>}
    </details>
  );
}
