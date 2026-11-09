import type { Discovery } from "../api";

export type Share = "near" | "two_hops" | "off_piste";
export const SHARES: [Share, string][] = [["near", "Proche"], ["two_hops", "À deux sauts"], ["off_piste", "Hors-piste"]];

export const STEP = 5;

// Moving one share takes from, or gives to, the other two in proportion, so the budget always sums to 100.
export function rebalance(d: Discovery, key: Share, value: number): Discovery {
  const v = Math.max(0, Math.min(100, Math.round(value)));
  const [a, b] = SHARES.map(([k]) => k).filter((k) => k !== key);
  const rest = 100 - v, before = d[a] + d[b];
  const toStep = (x: number) => Math.round(x / STEP) * STEP;
  const na = Math.min(rest, toStep(before === 0 ? rest / 2 : (rest * d[a]) / before));
  return { ...d, [key]: v, [a]: na, [b]: rest - na };
}
