# ADR-0006: Append-only review log, FSRS state rebuilt by replay

- Status: accepted, 2026-11-08

## Context
Reviews happen on several machines and land in git. Per-card state files would conflict.

## Decision
`review/log.jsonl` only gets new lines. Card state is computed by replaying the log sorted
by time with go-fsrs. A merge can only add lines; order in the file does not matter.
The app commits the log once per review session.

## Confirmation
`TestReload_IgnoresLineOrder_WhenMergeInterleavesMachines`.
