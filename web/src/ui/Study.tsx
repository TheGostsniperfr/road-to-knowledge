import { useEffect, useMemo, useRef, useState } from "preact/hooks";
import { api, pathsApi, type CoverView, type PathView, type StarView, type StepView } from "../api";
import { renderMarkdown, renderMermaid, section, splitCards, type QA } from "../markdown";
import { LEVELS } from "./levels";
import { afterStep, stepHref } from "./paths";
import { groupSteps, neighbours, openStep, stepGroup, stepTitle } from "./study";

const mark = { done: "✓", current: "▶", todo: "" } as const;

type Props = {
  path: string;
  step: string | null;
  version: number;
  onStep: (step: string) => void;
  onClose: () => void;
  onMap: (id: string) => void;
};

// Subjects are fetched once per universe version; an older answer stays on screen until the new one lands.
type StarCache = { get: (id: string) => Promise<StarView> };

function useStarCache(version: number): StarCache {
  const cache = useRef(new Map<string, { v: number; p: Promise<StarView> }>());
  return useMemo(() => ({
    get(id: string) {
      const hit = cache.current.get(id);
      if (hit && hit.v === version) return hit.p;
      const p = api.star(id);
      cache.current.set(id, { v: version, p });
      p.catch(() => { if (cache.current.get(id)?.p === p) cache.current.delete(id); });
      return p;
    },
  }), [version]);
}

// What the learner answered stays for the whole session: coming back to a step, or a reload of the
// universe after a rating, never offers the same card again or rates it twice.
type Answers = Map<string, { shown: boolean; ok?: boolean }>;

export function Study({ path, step, version, onStep, onClose, onMap }: Props) {
  const [view, setView] = useState<PathView | null>(null);
  const [err, setErr] = useState("");
  const [busy, setBusy] = useState(false);
  const [toast, setToast] = useState<{ text: string; undo?: string } | null>(null);
  const [unfold, setUnfold] = useState(false);
  const [, bump] = useState(0);
  const answers = useRef<Answers>(new Map());
  const pending = useRef(0);
  const stars = useStarCache(version);
  const main = useRef<HTMLElement>(null);
  const outline = useRef<HTMLElement>(null);
  const title = useRef<HTMLHeadingElement>(null);
  const shown = useRef<string | null>(null);
  const cb = useRef({ onStep, onClose });
  cb.current = { onStep, onClose };

  const load = () => pathsApi.get(path).then((v) => { setView(v); setErr(""); }).catch(() => setErr("Impossible de charger ce parcours."));
  useEffect(() => { load(); }, [path, version]);

  // Ratings sit in memory on the server until a flush commits them; leaving the page flushes too.
  useEffect(() => {
    const flush = () => {
      if (!pending.current) return;
      navigator.sendBeacon("/api/review/flush", new Blob([JSON.stringify({ count: pending.current })], { type: "application/json" }));
      pending.current = 0;
    };
    addEventListener("pagehide", flush);
    return () => {
      removeEventListener("pagehide", flush);
      if (pending.current) api.flush(pending.current).catch(() => undefined);
    };
  }, []);

  // A modal: focus moves in on open and back to where it was on close.
  useEffect(() => {
    const before = document.activeElement as HTMLElement | null;
    return () => { if (before?.isConnected) before.focus(); };
  }, []);

  const cur = view ? openStep(view.steps, step) : undefined;
  shown.current = cur?.id ?? null;
  const nb = view && cur ? neighbours(view.steps, cur.id) : { index: -1 };
  const groups = useMemo(() => (view ? groupSteps(view.steps) : []), [view]);

  useEffect(() => {
    if (!cur) return;
    main.current?.scrollTo({ top: 0 });
    title.current?.focus({ preventScroll: true });
    outline.current?.querySelector('[aria-current="page"]')?.scrollIntoView({ block: "center" });
  }, [cur?.id]);

  // The next step's subjects load while this one is read, so moving on feels instant.
  useEffect(() => {
    nb.next?.covers.forEach((c) => { if (!c.missing) stars.get(c.star).catch(() => undefined); });
  }, [cur?.id, version]);

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (e.altKey || e.ctrlKey || e.metaKey) return;
      const t = e.target;
      if (t instanceof HTMLInputElement || t instanceof HTMLTextAreaElement || (t instanceof HTMLElement && t.isContentEditable)) return;
      if (e.key === "Escape") cb.current.onClose();
      else if (e.key === "ArrowRight" && nb.next) cb.current.onStep(nb.next.id);
      else if (e.key === "ArrowLeft" && nb.prev) cb.current.onStep(nb.prev.id);
    };
    addEventListener("keydown", onKey);
    return () => removeEventListener("keydown", onKey);
  }, [nb.prev?.id, nb.next?.id]);

  useEffect(() => {
    if (!toast) return;
    const t = setTimeout(() => setToast(null), 6000);
    return () => clearTimeout(t);
  }, [toast]);

  // Moving the cursor commits; the view only follows if the learner is still on the step clicked.
  const setAt = async (at: string, from: string, then?: string) => {
    setBusy(true);
    try {
      const v = await pathsApi.setAt(path, at);
      setView(v);
      setErr("");
      return v;
    } catch {
      setErr("La progression n'a pas pu être enregistrée.");
      return null;
    } finally {
      setBusy(false);
      if (then && shown.current === from) cb.current.onStep(then);
    }
  };

  const finish = async (s: StepView) => {
    if (!view) return;
    const before = view.at ?? "";
    const g = stepGroup(s.name);
    const lastOfGroup = !!g && stepGroup(nb.next?.name ?? "") !== g;
    const v = await setAt(afterStep(view.steps, s.id), s.id, nb.next?.id);
    if (!v) return;
    const text = !nb.next ? `${view.name} terminé, bravo.` : lastOfGroup ? `${g} terminée. On passe à ${stepGroup(nb.next.name) || "la suite"}.` : `« ${stepTitle(s.name)} » marquée comme vue.`;
    setToast({ text, undo: before || undefined });
  };

  const undo = async () => {
    if (!toast?.undo) return;
    const back = toast.undo;
    setToast(null);
    const v = await setAt(back, shown.current ?? "");
    if (v) cb.current.onStep(back);
  };

  const rate = (cardKey: string, id: string | undefined, ok: boolean) => {
    const prev = answers.current.get(cardKey);
    if (prev?.ok !== undefined) return;
    answers.current.set(cardKey, { shown: true, ok });
    bump((n) => n + 1);
    if (!id) return;
    api.review(id, ok ? 3 : 1).then(() => { pending.current++; }).catch(() => {
      answers.current.set(cardKey, { shown: true });
      bump((n) => n + 1);
      setErr("Réponse non enregistrée : la carte a peut-être changé. Recharge la page.");
    });
  };
  const reveal = (cardKey: string) => { answers.current.set(cardKey, { shown: true }); bump((n) => n + 1); };
  const redo = (keys: string[]) => { keys.forEach((k) => answers.current.delete(k)); bump((n) => n + 1); };

  if (!view || !cur) {
    return (
      <div class="study" role="dialog" aria-modal="true" aria-label="Mode étude">
        <div class="study-empty">
          {err ? <p>{err} <button class="btn ghost" onClick={load}>Réessayer</button></p> : <><div class="skeleton" /><div class="skeleton short" /></>}
          <button class="btn ghost" onClick={onClose}>Retour à la carte</button>
        </div>
      </div>
    );
  }

  const href = stepHref(cur, view.source);
  const isVideo = !!href && href === cur.url;
  const group = stepGroup(cur.name);
  const canFinish = cur.status === "current" || cur.id === view.next?.id;
  const firstOpen = groups.findIndex((g) => g.steps.some((s) => s.status !== "done" || s.id === cur.id));
  const folded = unfold || firstOpen <= 1 ? 0 : firstOpen;

  return (
    <div class="study" role="dialog" aria-modal="true" aria-label={`Mode étude : ${view.name}`}>
      <aside class="study-nav" ref={outline}>
        <div class="study-head">
          <button class="study-back" onClick={onClose} title="Échap">← Carte</button>
          <h2>{view.name}</h2>
          <div class="study-prog">
            <div class="bar" role="progressbar" aria-valuenow={view.done} aria-valuemin={0} aria-valuemax={view.total}><i style={{ width: `${view.total ? (view.done / view.total) * 100 : 0}%` }} /></div>
            <small>{view.done} étape{view.done > 1 ? "s" : ""} vue{view.done > 1 ? "s" : ""} sur {view.total}</small>
          </div>
        </div>
        <nav class="outline" aria-label="Plan du parcours">
          {folded > 0 && (
            <button class="folded" onClick={() => setUnfold(true)}>
              <b>✓</b>{groups[0].name || "Début"} à {groups[folded - 1].name} · terminés
            </button>
          )}
          {groups.slice(folded).map((g) => {
            const done = g.steps.filter((s) => s.status === "done").length;
            const here = g.steps.some((s) => s.id === cur.id);
            return (
              <details key={g.name + g.steps[0].id} open={here || undefined}>
                <summary class={done === g.steps.length ? "all-done" : ""}>
                  <span>{g.name || "Étapes"}</span><small>{done === g.steps.length ? "✓" : `${done}/${g.steps.length}`}</small>
                </summary>
                <ol>
                  {g.steps.map((s) => (
                    <li key={s.id}>
                      <button class={`ostep is-${s.status}`} aria-current={s.id === cur.id ? "page" : undefined} onClick={() => onStep(s.id)}>
                        <i aria-hidden="true">{mark[s.status]}</i>{g.name ? stepTitle(s.name) : s.name}
                      </button>
                    </li>
                  ))}
                </ol>
              </details>
            );
          })}
        </nav>
      </aside>

      <main class="study-main" ref={main}>
        <div class="lesson">
          <header class="lesson-head">
            <div class="lesson-meta">
              <span>{view.name}{group ? ` › ${group}` : ""}</span>
              {cur.status === "done" && <span class="status is-done">Déjà vue</span>}
              {cur.status === "todo" && !canFinish && <span class="status">En avance sur ta progression</span>}
              <span class="count">Étape {nb.index + 1} sur {view.steps.length}</span>
            </div>
            <h1 ref={title} tabIndex={-1}>{group ? stepTitle(cur.name) : cur.name}</h1>
            {cur.covers.length === 1 && <Source cover={cur.covers[0]} stars={stars} version={version} onMap={onMap} />}
            {(href || (cur.status === "todo" && !canFinish)) && (
              <div class="lesson-actions">
                {href && <a class={isVideo ? "btn" : "btn ghost"} href={href} target="_blank" rel="noopener noreferrer">{isVideo ? "▶ Regarder la vidéo" : `Ouvrir le livre, p. ${cur.page}`}</a>}
                {cur.status === "todo" && !canFinish && <button class="btn ghost" disabled={busy} onClick={() => setAt(cur.id, cur.id)}>J'en suis là</button>}
              </div>
            )}
          </header>

          {err && <p class="lesson-err">{err}</p>}
          {cur.covers.length === 0 && <p class="lesson-note">Pas de fiche pour cette étape{href ? (isVideo ? " : suis la vidéo." : " : suis le livre.") : "."}</p>}
          {cur.covers.map((c) => (
            <Cover key={c.id} cover={c} stars={stars} version={version} single={cur.covers.length === 1} onMap={onMap}
              answers={answers.current} rate={rate} reveal={reveal} redo={redo} />
          ))}

          <footer class="lesson-foot">
            {canFinish && (
              <button class="btn big" disabled={busy} onClick={() => finish(cur)}>
                {nb.next ? "Étape terminée, suivante →" : "Terminer le parcours ✓"}
              </button>
            )}
            <div class="lesson-nav">
              {nb.prev ? <NavCard dir="prev" s={nb.prev} onStep={onStep} /> : <span />}
              {nb.next ? <NavCard dir="next" s={nb.next} onStep={onStep} /> : <span />}
            </div>
            <p class="keys">← → pour feuilleter sans marquer l'étape · Échap pour revenir à la carte</p>
          </footer>
        </div>
      </main>
      {toast && (
        <div class="toast" role="status">
          <span>{toast.text}</span>
          {toast.undo && <button class="chip" onClick={undo}>Annuler</button>}
        </div>
      )}
    </div>
  );
}

function NavCard({ dir, s, onStep }: { dir: "prev" | "next"; s: StepView; onStep: (id: string) => void }) {
  return (
    <button class={`navcard ${dir}`} onClick={() => onStep(s.id)}>
      <small>{dir === "prev" ? "← Précédent" : "Suivant →"}</small>
      <span>{s.name}</span>
    </button>
  );
}

function useStar(cover: CoverView, stars: StarCache, version: number) {
  const [sv, setSv] = useState<StarView | null>(null);
  const [err, setErr] = useState("");
  useEffect(() => {
    if (cover.missing) return;
    let live = true;
    stars.get(cover.star).then((v) => { if (live) { setSv(v); setErr(""); } }).catch(() => { if (live && !sv) setErr("Impossible de charger cette fiche."); });
    return () => { live = false; };
  }, [cover.star, version]);
  return { sv, err };
}

function notionOf(cover: CoverView, sv: StarView | null): string | null {
  return cover.id.includes("#") ? sv?.planets?.find((p) => p.id === cover.id)?.name ?? null : null;
}

function Source({ cover, stars, version, onMap }: { cover: CoverView; stars: StarCache; version: number; onMap: (id: string) => void }) {
  const { sv } = useStar(cover, stars, version);
  if (!sv) return null;
  return (
    <p class="lesson-src">
      Fiche : {sv.name} · {LEVELS[sv.level]}
      <button onClick={() => onMap(cover.id)}>Voir sur la carte</button>
    </p>
  );
}

type QuizApi = {
  answers: Answers;
  rate: (key: string, id: string | undefined, ok: boolean) => void;
  reveal: (key: string) => void;
  redo: (keys: string[]) => void;
};

function Cover({ cover, stars, version, single, onMap, ...quiz }: { cover: CoverView; stars: StarCache; version: number; single: boolean; onMap: (id: string) => void } & QuizApi) {
  const { sv, err } = useStar(cover, stars, version);
  const body = useRef<HTMLDivElement>(null);
  const notion = notionOf(cover, sv);
  const parts = useMemo(() => (sv ? splitCards(notion ? section(sv.body, notion) : sv.body) : null), [sv, notion]);
  const html = useMemo(() => (parts?.text ? renderMarkdown(parts.text) : ""), [parts]);

  useEffect(() => {
    if (!body.current) return;
    markJapanese(body.current);
    renderMermaid(body.current);
  }, [html]);

  const head = !single && (
    <div class="cover-head">
      <h2>{notion ?? sv?.name ?? cover.name}</h2>
      {sv && <button class="chip" onClick={() => onMap(cover.id)} title="Voir ce sujet sur la carte">{notion ? sv.name : "Sur la carte"} · {LEVELS[sv.level]}</button>}
    </div>
  );

  if (cover.missing) return <section class="cover"><h2>{cover.name}</h2><p class="lesson-note">Cette fiche n'existe plus.</p></section>;
  if (err && !sv) return <section class="cover">{head}<p class="lesson-err">{err}</p></section>;
  if (!sv || !parts) return <section class="cover loading" aria-busy="true">{head}<div class="skeleton" /><div class="skeleton" /><div class="skeleton short" /></section>;

  const ids = new Map((sv.card_list ?? []).map((c) => [c.q.trim(), c.id]));
  return (
    <section class="cover">
      {head}
      {html
        ? <div class="md lesson-md" ref={body} dangerouslySetInnerHTML={{ __html: html }} />
        : <p class="lesson-note">Cette fiche n'est pas encore rédigée. Tu peux demander une rédaction à un agent avec propose().</p>}
      {parts.cards.length > 0 && <Quiz star={sv.id} cards={parts.cards} ids={ids} {...quiz} />}
    </section>
  );
}

// Japanese runs get lang="ja" so the reading font and size apply to them only.
const cjk = /[\u3000-\u30ff\u3400-\u9fff\uff00-\uffef]+/g;
const hasCjk = /[\u3000-\u30ff\u3400-\u9fff\uff00-\uffef]/;

function markJapanese(root: HTMLElement) {
  const walker = document.createTreeWalker(root, NodeFilter.SHOW_TEXT, {
    acceptNode: (n) => (n.parentElement?.closest("pre, code, [lang]") ? NodeFilter.FILTER_REJECT : NodeFilter.FILTER_ACCEPT),
  });
  const nodes: Text[] = [];
  for (let n = walker.nextNode(); n; n = walker.nextNode()) if (hasCjk.test((n as Text).data)) nodes.push(n as Text);
  for (const t of nodes) {
    const frag = document.createDocumentFragment();
    let last = 0;
    for (const m of t.data.matchAll(cjk)) {
      frag.append(t.data.slice(last, m.index));
      const span = document.createElement("span");
      span.lang = "ja";
      span.textContent = m[0];
      frag.append(span);
      last = m.index! + m[0].length;
    }
    frag.append(t.data.slice(last));
    t.replaceWith(frag);
  }
}

// Self-test: reveal, then say whether it was known; a known card id is rated once in the review log.
function Quiz({ star, cards, ids, answers, rate, reveal, redo }: { star: string; cards: QA[]; ids: Map<string, string> } & QuizApi) {
  const keyOf = (c: QA) => ids.get(c.q.trim()) ?? `${star}|${c.q.trim()}`;
  const states = cards.map((c) => answers.get(keyOf(c)));
  const answered = states.filter((s) => s?.ok !== undefined).length;
  const known = states.filter((s) => s?.ok).length;
  const missed = cards.filter((_, i) => states[i]?.ok === false).map(keyOf);

  return (
    <div class="quiz">
      <div class="quiz-head">
        <h3>Teste-toi</h3>
        <small>{answered ? `${known} bonne${known > 1 ? "s" : ""} réponse${known > 1 ? "s" : ""} sur ${answered}` : `${cards.length} question${cards.length > 1 ? "s" : ""}`}</small>
      </div>
      <ol>
        {cards.map((c, i) => {
          const k = keyOf(c), st = states[i];
          return (
            <li key={i} class={st?.ok === undefined ? "" : st.ok ? "ok" : "ko"}>
              <p class="q">{c.q}</p>
              {st?.shown
                ? (
                  <div class="a">
                    <span>{c.a}</span>
                    {st.ok === undefined
                      ? <span class="rate"><button class="chip" onClick={() => rate(k, ids.get(c.q.trim()), false)}>À revoir</button><button class="chip on" onClick={() => rate(k, ids.get(c.q.trim()), true)}>Je savais</button></span>
                      : <small>{st.ok ? "Su" : "À revoir"}</small>}
                  </div>
                )
                : <button class="reveal" onClick={() => reveal(k)}>Voir la réponse</button>}
            </li>
          );
        })}
      </ol>
      {missed.length > 0 && answered === cards.length && <button class="chip redo" onClick={() => redo(missed)}>Refaire les {missed.length} ratée{missed.length > 1 ? "s" : ""}</button>}
    </div>
  );
}
