import { describe, expect, it } from "vitest";
import type { NewsItem } from "../api";
import { ago, keepText } from "./News";

const now = Date.parse("2026-11-09T12:00:00Z");

describe("ago", () => {
  it.each([
    ["2026-11-09T11:40:00Z", "à l'instant"],
    ["2026-11-09T09:00:00Z", "il y a 3 h"],
    ["2026-11-08T10:00:00Z", "hier"],
    ["2026-11-05T12:00:00Z", "il y a 4 j"],
  ])("says %s as %s", (iso, want) => {
    expect(ago(iso, now)).toBe(want);
  });
});

describe("keepText", () => {
  it("puts the title, the link and the subjects in the captured note", () => {
    const item: NewsItem = { id: "1", title: "Cilium 1.18", link: "https://e.com/c", source: "K8s blog", scope: "tech", scope_name: "Tech", galaxy: "tech",
      published: "2026-11-09T10:00:00Z", subjects: [{ id: "tech/k8s/cilium", name: "Cilium", level: 4 }] };

    expect(keepText(item)).toBe("Cilium 1.18\nhttps://e.com/c\n\nVu dans K8s blog (Tech).\nSujets : Cilium");
  });
});
