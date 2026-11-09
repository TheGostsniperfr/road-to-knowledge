# ADR-0014: Restructure subjects with a rename log

- Status: accepted, 2026-11-09

## Context

A knowledge map changes shape as it grows: a subject becomes two, two subjects turn out to
be one, a sub-theme moves to another theme. Ids are paths (ADR-0002) and a card id is
`<subject id>:<hash of the question>`, so any move changes ids. Done by hand, a move loses
the FSRS history of every card (ADR-0006 replays the log by card id), breaks `links:` in
other subjects, and breaks bookmarks and inbox notes that name the old id.

## Options

1. **Rewrite the review log** with the new ids. Simple to read, but the log stops being
   append-only: a rewrite on one machine and a review on another conflict in git.
2. **`aliases:` in the subject's frontmatter**, listing its former ids. History follows a
   subject, but not a card that moved to another subject in a split, and the aliases of a
   merged subject must be collected from several files.
3. **An append-only rename log**, `review/renames.jsonl`, with one line per id change
   (`{"from","to","at"}`), for subjects, sub-themes and single cards. The review log is
   replayed through it, in time order.

## Decision

Option 3. It keeps both logs append-only, so merges between machines only add lines, and
it handles every operation the same way:

- **move / rename** (subject or sub-theme): one line per subject id; cards follow their
  subject without a line of their own.
- **split**: the original keeps its id; each card whose notion leaves gets its own line.
- **merge**: each absorbed subject gets a line to the target; the target keeps the
  highest level and the most advanced state of all sources (ADR-0003: a level never drops).

**Which reviews follow a rename.** An id that is gone from the files follows every rename
whatever the time of the review. The `at` of a rename made by a proposal is the time the agent
wrote it, not the time the user merged it; reviews of the old id in between, on this machine or
another, must still follow. Time only decides for an id that is **live again** (a subject moved
away and back, a notion split out and merged back, or a new subject that reuses an old id): its
reviews from before the rename follow it, those written after belong to what lives there now.

**Redirects never touch a live id.** The same log serves `GET /api/star/<old id>`, `node`,
`fragments`, `subgraph`, `neighbors`, a review sent with a stale card id, and the UI's deep
links, but only for ids that no longer exist. A reused id always opens, and logs reviews for,
the subject that has it today.

Every operation is planned from the current files first (dry run), then applied:

- from the app or the CLI (`rtk mv`, `rtk split`, `rtk merge`), on the current branch with
  one commit, because the user asked for it;
- from an agent (`restructure` MCP tool), on `proposals/<slug>` (ADR-0004). The server
  computes the moves and deletions; `propose` still refuses an agent's own deletes outside
  `inbox/`, so a subject can only disappear by being merged into another. Each restructure
  gets its own branch: an existing `proposals/<slug>` is refused rather than overwritten, and
  the default slug carries a hash of the plan.

Applying is all or nothing. Each file step records how to undo it; if a step, the rename log
or the commit fails, the steps are undone in reverse and the index is reset, so the tree is as
before. The rename log is appended only after every file step succeeded. The app sends back
the hash of the plan it showed; if the files changed since, the apply is refused and the new
plan is shown.

Links in other subjects (full ids, bare slugs) and deep links in the text (`#/id`,
`#/id#notion`) are rewritten in the same commit, including references to a notion a split or
merge moved. A sub-theme a plan leaves without subjects keeps its folder through a
`_cluster.md`, since git does not keep empty folders. Files are moved, not copied, so
`git log --follow` keeps their history.

## Confirmation

`internal/app/restructure_test.go`: a reviewed card is not new again after a move, rename,
split, merge, merge into a new id, sub-theme move, and a split followed by a merge back;
links are rewritten; old ids open the live subject; `git log --follow` sees both commits;
collisions, unknown notions, targets outside `galaxies/` and conflicting answers are
refused with nothing written. `internal/app/restructure_safety_test.go`: a reused id is never
redirected; reviews made between a proposal and its merge follow; a failure before any step,
or of the commit, leaves the tree, the rename log and the history as they were; an emptied
sub-theme is kept; deep links to a moved notion are rewritten; the plan shown is the plan
applied. `internal/review/renames_test.go` covers the resolution rules.
