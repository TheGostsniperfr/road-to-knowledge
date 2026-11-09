import { describe, expect, it } from "vitest";
import type { RestructurePlan, UniverseView } from "../api";
import { afterPlan, liveId, slugify, subThemes } from "./reorg";

const uni = (stars: string[], redirects: Record<string, string>): UniverseView => ({
  version: 1, inbox: 0, problems: [], redirects,
  galaxies: [{ id: "tech", name: "Tech", clusters: [{ id: "tech/k8s", name: "K8s", stars: stars.map((id) => ({ id })) }] }],
} as unknown as UniverseView);

describe("liveId", () => {
  it("opens a moved subject at its new id", () => {
    expect(liveId("tech/k8s/helm", uni(["tech/k8s/helm-v2"], { "tech/k8s/helm": "tech/k8s/helm-v2" }))).toBe("tech/k8s/helm-v2");
  });

  it("keeps the notion part of a deep link", () => {
    expect(liveId("tech/k8s/helm#hooks", uni(["tech/k8s/helm-v2"], { "tech/k8s/helm": "tech/k8s/helm-v2" }))).toBe("tech/k8s/helm-v2#hooks");
  });

  it("never redirects an id that a subject has again today", () => {
    expect(liveId("tech/k8s/helm", uni(["tech/k8s/helm", "tech/k8s/helm-v2"], { "tech/k8s/helm": "tech/k8s/helm-v2" }))).toBe("tech/k8s/helm");
  });

  it("leaves ids that were never renamed alone", () => {
    expect(liveId("tech/k8s/argo", uni(["tech/k8s/argo"], { "tech/k8s/helm": "tech/k8s/helm-v2" }))).toBe("tech/k8s/argo");
  });
});

describe("slugify", () => {
  it("matches the server's slug rule and keeps accents", () => {
    expect(slugify("  Gestion des Hooks / Helm ")).toBe("gestion-des-hooks-helm");
    expect(slugify("Écriture")).toBe("écriture");
  });
});

describe("afterPlan", () => {
  const plan = (renames: { from: string; to: string }[]): RestructurePlan => ({ op: { kind: "move" }, files: [], renames: renames.map((r) => ({ ...r, at: "" })), summary: [], message: "", hash: "" });

  it("follows the subject when the plan renames it", () => {
    expect(afterPlan("a/b/c", plan([{ from: "a/b/c", to: "a/d/c" }]))).toBe("a/d/c");
  });

  it("stays on the subject when only cards moved", () => {
    expect(afterPlan("a/b/c", plan([{ from: "a/b/c:12", to: "a/b/x:12" }]))).toBe("a/b/c");
  });
});

describe("subThemes", () => {
  it("lists every sub-theme but the current one, with its theme", () => {
    const u = { version: 1, inbox: 0, problems: [], galaxies: [{ id: "t", name: "Tech", clusters: [{ id: "t/a", name: "A", stars: [] }, { id: "t/b", name: "B", stars: [] }] }] } as unknown as UniverseView;

    expect(subThemes(u, "t/a")).toEqual([{ id: "t/b", label: "Tech › B" }]);
  });
});
