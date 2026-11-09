# ADR-0004: Agents write to the inbox or to proposal branches

- Status: accepted, 2026-11-08

## Decision
MCP `capture` writes `inbox/*.md` and commits it. MCP `propose` writes `.md` files under
`galaxies/` or `inbox/` on branch `proposals/<slug>`, through a temporary git worktree,
so the user's checkout is untouched. Nothing an agent writes reaches the main branch
without a human merge.

## Confirmation
`TestPropose_LandsOnProposalBranchOnly`, `TestValidProposalPath_RejectsEscapes`.
