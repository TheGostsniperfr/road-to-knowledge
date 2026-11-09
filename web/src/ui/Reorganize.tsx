import { useState } from "preact/hooks";
import { restructureApi, type RestructureOp, type RestructurePlan, type StarView, type UniverseView } from "../api";
import { afterPlan, slugify, subjects, subThemes } from "./reorg";

type Mode = "move" | "rename" | "split" | "merge";

const MODES: [Mode, string][] = [["move", "Déplacer"], ["rename", "Renommer"], ["split", "Scinder"], ["merge", "Fusionner avec…"]];

// Reorganize builds one restructure, shows the plan the server would apply, then applies it with
// one commit. History, links and old deep links follow (ADR-0014), so nothing here is lossy.
export function Reorganize({ star, universe, done }: { star: StarView; universe: UniverseView | null; done: (id: string) => Promise<void> }) {
  const cluster = star.id.slice(0, star.id.lastIndexOf("/"));
  const slug = star.id.slice(star.id.lastIndexOf("/") + 1);
  const [mode, setMode] = useState<Mode>("move");
  const [target, setTarget] = useState("");
  const [name, setName] = useState(star.name);
  const [newSlug, setNewSlug] = useState(slug);
  const [picked, setPicked] = useState<string[]>([]);
  const [partName, setPartName] = useState("");
  const [plan, setPlan] = useState<RestructurePlan | null>(null);
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState("");

  const reset = (m: Mode) => { setMode(m); setPlan(null); setErr(""); setTarget(""); };

  const op = (): RestructureOp | null => {
    switch (mode) {
      case "move":
        return target ? { kind: "move", id: star.id, to: target } : null;
      case "rename": {
        const s = slugify(newSlug);
        if (!s || (s === slug && name === star.name)) return null;
        return { kind: "move", id: star.id, to: `${cluster}/${s}`, name: name !== star.name ? name : undefined };
      }
      case "split": {
        const pn = partName.trim() || (picked.length === 1 ? picked[0] : "");
        if (!picked.length || !pn) return null;
        return { kind: "split", id: star.id, parts: [{ id: slugify(pn), name: pn, notions: picked }] };
      }
      case "merge":
        return target ? { kind: "merge", sources: [star.id, target], into: star.id } : null;
    }
  };

  const run = async (dry: boolean) => {
    const o = op();
    if (!o) return;
    setBusy(true);
    setErr("");
    try {
      // Applying sends the hash of the plan on screen: the server refuses if the files changed since.
      const p = await restructureApi.run(o, dry, dry ? "" : plan?.hash ?? "");
      if (dry) setPlan(p);
      else {
        setPlan(null);
        await done(afterPlan(star.id, p));
      }
    } catch (e) {
      if (!dry && String(e).includes("plan changed")) {
        setPlan(await restructureApi.run(o, true).catch(() => null));
        setErr("Les fichiers ont changé depuis ce plan : relis le nouveau plan avant d'appliquer.");
      } else setErr(String(e));
    } finally {
      setBusy(false);
    }
  };

  const ready = op() !== null;
  return (
    <details class="blk reorg" onToggle={() => setPlan(null)}>
      <summary class="k">Réorganiser</summary>
      <div class="seg" role="group" aria-label="Opération">
        {MODES.map(([m, l]) => <button key={m} aria-pressed={mode === m} onClick={() => reset(m)}>{l}</button>)}
      </div>
      {mode === "move" && (
        <label class="field">Vers
          <select value={target} onChange={(e) => { setTarget((e.target as HTMLSelectElement).value); setPlan(null); }}>
            <option value="">Choisir un sous-thème…</option>
            {subThemes(universe, cluster).map((p) => <option key={p.id} value={p.id}>{p.label}</option>)}
          </select>
        </label>
      )}
      {mode === "rename" && (
        <>
          <label class="field">Nom <input value={name} onInput={(e) => { setName((e.target as HTMLInputElement).value); setPlan(null); }} /></label>
          <label class="field">Identifiant <input value={newSlug} onInput={(e) => { setNewSlug((e.target as HTMLInputElement).value); setPlan(null); }} /></label>
          <p class="file">{cluster}/{slugify(newSlug) || "…"}</p>
        </>
      )}
      {mode === "split" && (
        star.planets?.length ? (
          <>
            <span class="file">Notions qui partent dans un nouveau sujet, avec leurs cartes :</span>
            {star.planets.map((p) => (
              <label key={p.id} class="field">
                <input type="checkbox" checked={picked.includes(p.name)} onChange={(e) => {
                  const on = (e.target as HTMLInputElement).checked;
                  setPicked(on ? [...picked, p.name] : picked.filter((x) => x !== p.name));
                  setPlan(null);
                }} />{p.name}
              </label>
            ))}
            <label class="field">Nouveau sujet <input value={partName} placeholder={picked.length === 1 ? picked[0] : "Nom"} onInput={(e) => { setPartName((e.target as HTMLInputElement).value); setPlan(null); }} /></label>
          </>
        ) : <p class="file">Ce sujet n'a pas de notion (## titre) à détacher.</p>
      )}
      {mode === "merge" && (
        <>
          <label class="field">Absorber
            <select value={target} onChange={(e) => { setTarget((e.target as HTMLSelectElement).value); setPlan(null); }}>
              <option value="">Choisir un sujet…</option>
              {subjects(universe, star.id).map((p) => <option key={p.id} value={p.id}>{p.label}</option>)}
            </select>
          </label>
          <p class="file">Ses notions et cartes rejoignent « {star.name} », le niveau le plus haut est gardé.</p>
        </>
      )}
      <div class="row">
        <button class="btn ghost" disabled={!ready || busy} onClick={() => run(true)}>Voir le plan</button>
        {plan && <button class="btn" disabled={busy} onClick={() => run(false)}>Appliquer</button>}
      </div>
      {plan && (
        <div class="plan">
          <ul>{plan.summary.map((s) => <li key={s}>{s}</li>)}</ul>
          <ul class="files">
            {plan.files.map((f) => <li key={f.kind + f.path}><b>{f.kind === "move" ? "déplacé" : f.kind === "write" ? "écrit" : "supprimé"}</b> {f.from ? `${f.from} → ` : ""}{f.path}</li>)}
          </ul>
          <p class="file">{plan.renames.length ? `${plan.renames.length} identifiant${plan.renames.length > 1 ? "s" : ""} redirigé${plan.renames.length > 1 ? "s" : ""} : l'historique de révision suit. ` : ""}Un commit : {plan.message}</p>
        </div>
      )}
      {err && <p class="file">{err}</p>}
    </details>
  );
}
