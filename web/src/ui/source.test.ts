import { describe, expect, it } from "vitest";
import { pageOf, sourceHref, sourceLabel } from "./source";

const book = { title: "Genki I", author: "Banno", year: 2020, pages: 400, file: "library/genki-0123abcd.pdf", available: true };

describe("source", () => {
  it("reads the page from a notion heading", () => {
    expect(pageOf("Le journal (p. 41-58)")).toBe(41);
  });

  it("has no page when the notion does not name one", () => {
    expect(pageOf("Le journal")).toBeNull();
  });

  it("names the page of the notion instead of the page count", () => {
    expect(sourceLabel(book, 41)).toBe("Genki I · Banno, 2020, p. 41");
  });

  it("opens the library copy at the page", () => {
    expect(sourceHref(book, 41)).toBe("/library/genki-0123abcd.pdf#page=41");
  });

  it("does not link a file that is on another machine", () => {
    expect(sourceHref({ ...book, available: false }, null)).toBeNull();
  });

  it("never links outside the library", () => {
    expect(sourceHref({ ...book, file: "galaxies/x.md" }, null)).toBeNull();
  });
});
