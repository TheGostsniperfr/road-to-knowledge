import { useEffect, useRef, useState } from "preact/hooks";
import { api, type DueCard, type UniverseView } from "../api";
import { excluded, scopeChildren } from "./scope";
import { useTerms } from "./terms";

const GRADES: [number, string][] = [[1, "À revoir"], [2, "Difficile"], [3, "Bien"], [4, "Facile"]];

export function Review({ universe, scope, onClose }: { universe: UniverseView | null; scope: { id: string; name: string }; onClose: () => void }) {
  const { t } = useTerms();
  const [ex, setEx] = useState<string[]>(() => excluded.get(scope.id));
  const [deck, setDeck] = useState<DueCard[] | null>(null);
  const [i, setI] = useState(0);
  const [shown, setShown] = useState(false);
  const [err, setErr] = useState("");
  const done = useRef(0);
  const kids = scopeChildren(universe, scope.id);

  useEffect(() => { setEx(excluded.get(scope.id)); }, [scope.id]);
  useEffect(() => {
    api.due(scope.id, ex).then((d) => { setDeck(d); setI(0); setShown(false); }).catch((e) => setErr(String(e)));
  }, [scope.id, ex.join(",")]);

  // The review log is committed once when the drawer closes, not once per card.
  const close = () => {
    if (done.current) api.flush(done.current).catch(() => {});
    done.current = 0;
    onClose();
  };
  useEffect(() => () => { if (done.current) api.flush(done.current).catch(() => {}); }, []);

  const card = deck && i < deck.length ? deck[i] : null;
  const grade = async (r: number) => {
    if (!card) return;
    try {
      await api.review(card.id, r);
      done.current++;
      setI(i + 1);
      setShown(false);
    } catch (e) {
      setErr(String(e));
    }
  };

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (e.target instanceof HTMLInputElement || e.target instanceof HTMLTextAreaElement) return;
      // Study mode sits on top: keys belong to it, never to a review hidden underneath.
      if (document.querySelector(".study")) return;
      if (e.key === "Escape") close();
      else if (e.key === " " && card && !shown) { e.preventDefault(); setShown(true); }
      else if (shown && ["1", "2", "3", "4"].includes(e.key)) grade(+e.key);
    };
    addEventListener("keydown", onKey);
    return () => removeEventListener("keydown", onKey);
  });

  const toggle = (id: string, on: boolean) => {
    const next = on ? ex.filter((x) => x !== id) : [...new Set([...ex, id])];
    excluded.set(scope.id, next);
    setEx(next);
  };
  const big = card && /^[぀-ヿ一-鿿]{1,3}$/.test(card.q);

  return (
    <div class="review glass" role="dialog" aria-label="Révision">
      <div class="scope">
        <h4>Révision</h4>
        <span class="k">{scope.name}</span>
        {kids.length ? kids.map((k) => (
          <label key={k.id}>
            <input type="checkbox" checked={!ex.includes(k.id)} onChange={(e) => toggle(k.id, e.currentTarget.checked)} />
            {k.name}<span class="n">{k.due}</span>
          </label>
        )) : <p class="kbd">{t.oneInScope}</p>}
        <p class="kbd">Choix retenu sur cet appareil seulement.</p>
      </div>
      <div class="cardv">
        <div class="row" style={{ justifyContent: "space-between" }}>
          <span class="k">{deck ? (card ? `${i + 1} / ${deck.length} · ${card.star_name}${card.new ? " · nouvelle" : ""}` : `${done.current} cartes revues`) : "…"}</span>
          <button class="btn ghost" onClick={close}>Fermer</button>
        </div>
        {err && <p class="kbd">{err}</p>}
        <div class="face">
          {card ? (
            <div>
              <div class={`q${big ? " big" : ""}`}>{card.q}</div>
              {shown && <div class={`a${/[぀-ヿ]/.test(card.a) ? " jp" : ""}`}>{card.a}</div>}
            </div>
          ) : (
            <span class="kbd">{deck?.length === 0 ? "Aucune carte à revoir ici. Écris-en dans une fiche : « question :: réponse »." : "Session terminée."}</span>
          )}
        </div>
        {card && (shown ? (
          <div class="grade">
            {GRADES.map(([r, l]) => <button key={r} onClick={() => grade(r)}>{l}<small>{card.next[String(r)]} · {r}</small></button>)}
          </div>
        ) : (
          <button class="btn" onClick={() => setShown(true)}>Afficher la réponse <span class="kbd">Espace</span></button>
        ))}
      </div>
    </div>
  );
}
