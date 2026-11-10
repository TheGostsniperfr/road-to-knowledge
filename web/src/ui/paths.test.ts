import { describe, expect, it } from "vitest";
import { afterStep, continueTarget, inProgress, inScope, progress, requestText, stepHref } from "./paths";

const src = { title: "Genki II", file: "library/genki-2.pdf", available: true };

describe("progress", () => {
  it("rounds the share of done steps to a percent", () => {
    expect(progress({ done: 1, total: 3 })).toBe(33);
  });

  it("is zero for a path without steps", () => {
    expect(progress({ done: 0, total: 0 })).toBe(0);
  });
});

describe("stepHref", () => {
  it("prefers the step's own URL over the book page", () => {
    expect(stepHref({ url: "https://youtu.be/x", page: 12 }, src)).toBe("https://youtu.be/x");
  });

  it("opens the book at the step's page when the file is here", () => {
    expect(stepHref({ page: 12 }, src)).toBe("/library/genki-2.pdf#page=12");
  });

  it("gives nothing for a script URL", () => {
    expect(stepHref({ url: "javascript:alert(1)" })).toBeNull();
  });
});

describe("continueTarget", () => {
  it("skips covers that no longer exist", () => {
    const step = { id: "l13", name: "L13", status: "current" as const, covers: [
      { id: "a/b/gone", star: "a/b/gone", name: "gone", level: 0, state: "idea" as const, missing: true },
      { id: "a/b/potentiel", star: "a/b/potentiel", name: "Potentiel", level: 1, state: "progress" as const },
    ] };

    expect(continueTarget(step)).toBe("a/b/potentiel");
  });
});

describe("requestText", () => {
  it("names what the user wants a path for", () => {
    expect(requestText("  Genki II  ")).toMatch(/^Demande de parcours : Genki II\n/);
  });
});

describe("afterStep", () => {
  const steps = [{ id: "l13" }, { id: "l14" }];

  it("moves the cursor to the next step", () => {
    expect(afterStep(steps, "l13")).toBe("l14");
  });

  it("finishes the path after the last step", () => {
    expect(afterStep(steps, "l14")).toBe("end");
  });
});

describe("inScope", () => {
  const p = { stars: ["langues/japonais/verbes", "tech/reseau/stp"] };

  it("shows every path at the universe", () => {
    expect(inScope({ stars: [] }, "")).toBe(true);
  });

  it("shows a path in a theme that holds one of its subjects", () => {
    expect(inScope(p, "langues/japonais")).toBe(true);
  });

  it("does not match a theme that only shares a prefix", () => {
    expect(inScope(p, "tech/res")).toBe(false);
  });
});

describe("inProgress", () => {
  it("keeps started paths that are not finished", () => {
    const next = { id: "a", name: "A", status: "current" as const, covers: [] };
    const got = inProgress([{ id: "new", done: 0, next }, { id: "on", done: 3, next }, { id: "first", done: 0, at: "a", next }, { id: "end", done: 9, next: undefined }]);

    expect(got.map((p) => p.id)).toEqual(["on", "first"]);
  });
});
