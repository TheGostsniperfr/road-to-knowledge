import { describe, expect, it } from "vitest";
import { rebalance } from "./discovery";

const base = { near: 60, two_hops: 30, off_piste: 10, news: false };
const sum = (d: typeof base) => d.near + d.two_hops + d.off_piste;

describe("rebalance", () => {
  it("keeps the budget at 100 for every slider position", () => {
    for (const k of ["near", "two_hops", "off_piste"] as const)
      for (let v = 0; v <= 100; v += 5) expect(sum(rebalance(base, k, v))).toBe(100);
  });

  it("takes from the other shares in proportion", () => {
    expect(rebalance(base, "near", 20)).toEqual({ near: 20, two_hops: 60, off_piste: 20, news: false });
  });

  it("keeps the other shares on the slider step", () => {
    const d = rebalance(base, "near", 30);

    expect([d.two_hops % 5, d.off_piste % 5]).toEqual([0, 0]);
  });

  it("splits evenly when the other shares are both zero", () => {
    expect(rebalance({ near: 100, two_hops: 0, off_piste: 0, news: true }, "near", 40)).toEqual({ near: 40, two_hops: 30, off_piste: 30, news: true });
  });
});
