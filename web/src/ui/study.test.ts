import { describe, expect, it } from "vitest";
import { splitCards } from "../markdown";
import { groupSteps, neighbours, openStep, parseStudyHash, stepGroup, stepTitle, studyHash } from "./study";

describe("stepGroup", () => {
  it.each([
    ["L13 · Verbes au potentiel", "L13"],
    ["Day 21 (part 2) · BPDU Guard", "Day 21"],
    ["Genki I en entier", ""],
  ])("puts %s in group %s", (name, want) => {
    expect(stepGroup(name)).toBe(want);
  });

  it("keeps the title after the group", () => {
    expect(stepTitle("L13 · ～し")).toBe("～し");
  });
});

describe("groupSteps", () => {
  it("groups consecutive steps of the same lesson", () => {
    const got = groupSteps([{ name: "L1 · a" }, { name: "L1 · b" }, { name: "L2 · c" }]);

    expect(got.map((g) => [g.name, g.steps.length])).toEqual([["L1", 2], ["L2", 1]]);
  });
});

describe("neighbours", () => {
  it("gives the steps around one", () => {
    const steps = [{ id: "a" }, { id: "b" }, { id: "c" }];

    const n = neighbours(steps, "b");

    expect([n.index, n.prev?.id, n.next?.id]).toEqual([1, "a", "c"]);
  });
});

describe("openStep", () => {
  const steps = [{ id: "a", status: "done" as const }, { id: "b", status: "current" as const }, { id: "c", status: "todo" as const }];

  it("opens the step asked for", () => {
    expect(openStep(steps, "c")?.id).toBe("c");
  });

  it("opens the current step when none is asked", () => {
    expect(openStep(steps, null)?.id).toBe("b");
  });

  it("falls back to the current step for an unknown id", () => {
    expect(openStep(steps, "zz")?.id).toBe("b");
  });
});

describe("study hash", () => {
  it("round-trips a path and a step", () => {
    const r = { path: "genki-2", step: "l13-potentiel" };

    expect(parseStudyHash(studyHash(r))).toEqual(r);
  });

  it("ignores a subject hash", () => {
    expect(parseStudyHash("#/langues/japonais")).toBeNull();
  });
});

describe("splitCards", () => {
  it("takes cards out of the text but leaves code alone", () => {
    const got = splitCards("Intro.\n\nq1 :: a1\n```\nx :: y\n```\nq2 :: a2");

    expect(got.cards).toEqual([{ q: "q1", a: "a1" }, { q: "q2", a: "a2" }]);
    expect(got.text).toContain("x :: y");
  });
});
