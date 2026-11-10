import type { PathSummary, Source, StepView } from "../api";
import { sourceHref } from "./source";

export function progress(p: Pick<PathSummary, "done" | "total">): number {
  return p.total ? Math.round((p.done / p.total) * 100) : 0;
}

// A step opens its own URL (a video) first, else the book page when the file is on this machine.
export function stepHref(step: Pick<StepView, "url" | "page">, src?: Source): string | null {
  if (step.url && /^https?:\/\//.test(step.url)) return step.url;
  if (src && step.page) return sourceHref(src, step.page);
  return null;
}

// "Continuer" lands on what the step teaches: its first cover that still exists.
export function continueTarget(step?: StepView): string | null {
  return step?.covers.find((c) => !c.missing)?.id ?? null;
}

export function requestText(what: string): string {
  return `Demande de parcours : ${what.trim()}\n\nÀ préparer par un agent : un fichier paths/<id>.md avec les étapes dans l'ordre, ce qu'elles couvrent sur la carte, et le curseur là où j'en suis.`;
}

// Finishing a step moves the cursor to the next one; after the last step the path is finished.
export function afterStep(steps: Pick<StepView, "id">[], id: string): string {
  const i = steps.findIndex((s) => s.id === id);
  return steps[i + 1]?.id ?? "end";
}

// A theme or sub-theme shows the paths that cover one of its subjects; the universe shows all.
export function inScope(p: Pick<PathSummary, "stars">, scope: string): boolean {
  return scope === "" || p.stars.some((s) => s === scope || s.startsWith(scope + "/"));
}

// Under way: a cursor is set or a step is done, and the path is not finished.
export function inProgress<T extends Pick<PathSummary, "done" | "next" | "at">>(paths: T[]): T[] {
  return paths.filter((p) => p.next && (p.done > 0 || !!p.at));
}
