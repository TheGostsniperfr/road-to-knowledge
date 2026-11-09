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
   replayed through it: a review written at time t follows every rename at or after t, in
   time order.

## Decision

Option 3. It keeps both logs append-only, so merges between machines only add lines, and
it handles every operation the same way:

- **move / rename** (subject or sub-theme): one line per subject id; cards follow their
  subject without a line of their own.
- **split**: the original keeps its id; each card whose notion leaves gets its own line.
- **merge**: each absorbed subject gets a line to the target; the target keeps the
  highest level and the most advanced state of all sources (ADR-0003: a level never drops).

Resolving with the review's own time, not a final map, is what makes round trips exact: a
notion split out and merged back, or a subject moved away and back, ends where it is today,
and a new subject that later reuses an old id keeps its own reviews.

The same log serves redirects: `GET /api/star/<old id>`, `node`, `fragments`, `subgraph`,
`neighbors` and the UI's deep links resolve an old id to the live one.

Every operation is planned from the current files first (dry run), then applied:

- from the app or the CLI (`rtk mv`, `rtk split`, `rtk merge`), on the current branch with
  one commit, because the user asked for it;
- from an agent (`restructure` MCP tool), on `proposals/<slug>` (ADR-0004). The server
  computes the moves and deletions; `propose` still refuses an agent's own deletes outside
  `inbox/`, so a subject can only disappear by being merged into another.

Links in other subjects (full ids or bare slugs) are rewritten to the new ids in the same
commit. Files are moved, not copied, so `git log --follow` keeps their history.

## Confirmation

`internal/app/restructure_test.go`: a reviewed card is not new again after a move, rename,
split, merge, merge into a new id, sub-theme move, and a split followed by a merge back;
links are rewritten; old ids open the live subject; `git log --follow` sees both commits;
collisions, unknown notions, targets outside `galaxies/` and conflicting answers are
refused with nothing written. `internal/review/renames_test.go` covers the time-ordered
resolution.
