import type { StepView } from "../api";

export interface StudyRef { path: string; step: string | null }

// "L13 · Verbes au potentiel" belongs to "L13"; "Day 21 (part 2) · BPDU Guard" to "Day 21".
export function stepGroup(name: string): string {
  const i = name.indexOf(" · ");
  return i < 0 ? "" : name.slice(0, i).replace(/\s*\(.*\)\s*$/, "").trim();
}

export function stepTitle(name: string): string {
  const i = name.indexOf(" · ");
  return i < 0 ? name : name.slice(i + 3);
}

export interface StepGroup<T> { name: string; steps: T[] }

// Consecutive steps of the same lesson or day form one group of the outline.
export function groupSteps<T extends Pick<StepView, "name">>(steps: T[]): StepGroup<T>[] {
  const out: StepGroup<T>[] = [];
  for (const s of steps) {
    const name = stepGroup(s.name);
    const last = out[out.length - 1];
    if (last && last.name === name) last.steps.push(s);
    else out.push({ name, steps: [s] });
  }
  return out;
}

export function neighbours<T extends Pick<StepView, "id">>(steps: T[], id: string): { index: number; prev?: T; next?: T } {
  const index = steps.findIndex((s) => s.id === id);
  return { index, prev: index > 0 ? steps[index - 1] : undefined, next: index >= 0 ? steps[index + 1] : undefined };
}

// The step to open: the one asked for, else the next one to do, else the first.
export function openStep<T extends Pick<StepView, "id" | "status">>(steps: T[], want: string | null): T | undefined {
  return steps.find((s) => s.id === want) ?? steps.find((s) => s.status === "current") ?? steps.find((s) => s.status === "todo") ?? steps[0];
}

const enc = encodeURIComponent;

export function studyHash(r: StudyRef): string {
  return `#/study/${enc(r.path)}${r.step ? "/" + enc(r.step) : ""}`;
}

export function parseStudyHash(hash: string): StudyRef | null {
  const m = /^#\/study\/([^/]+)(?:\/([^/]+))?$/.exec(hash);
  if (!m) return null;
  try {
    return { path: decodeURIComponent(m[1]), step: m[2] ? decodeURIComponent(m[2]) : null };
  } catch {
    return null;
  }
}
