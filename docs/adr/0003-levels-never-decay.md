# ADR-0003: Levels never decay on their own

- Status: accepted, 2026-11-08

## Context
Brian does not want his knowledge to fade away in the tool. Forgetting is real, but the
tool should not decide it.

## Decision
The level (0-5) changes only by an explicit action. After `review_after_days` without a
review, a subject gets an amber dot (`stale`) and nothing else.

## Confirmation
API test `TestUniverse_ListsGalaxiesWithStaleHint`: a stale subject keeps its level.
