# ADR-0015: Learning paths laid over the map

- Status: accepted, 2026-11-09

## Context

Brian follows courses that have an order: Genki II lesson by lesson, the 126 videos of
Jeremy's IT Lab CCNA course. The map says what he knows, not where he is in a course or
what to do next. Coming back after months, he cannot tell which lessons he saw.

## Options

1. One subject per lesson or per day, in a sub-theme per course. Simple, but the map
   becomes the course's table of contents: STP is spread over Day 20, 21 and 22 with no
   level of its own, and a second source on STP needs a duplicate subject.
2. A sub-theme per course with an `order:` and a cursor. Fewer files, same mixing of the
   course's order with the structure of the knowledge.
3. A path file per course, outside `galaxies/`, whose ordered steps point at subjects and
   notions organised by topic.

## Decision

Option 3. `paths/<id>.md` holds `name`, an optional `source` (same block as a fiche),
`at` (the current step) and `steps`, each with an `id`, a `name`, `covers` (subject ids or
`subject#notion` ids), an optional `url` (a video) or `page` (in the source file) and an
optional `done: true`.

- Steps before `at` are done, `at` is current unless marked done, later steps are to do
  unless marked done, since a course is not always followed in order. `at: end` finishes the
  path, so it can reach 100 %. The next step is the first step not done. "Étape terminée"
  in the app moves the cursor to the following step, or to `end` after the last one.
- A path never changes a level (ADR-0003). Where Brian is in a course and what he knows
  are two facts; the user, or a reviewed proposal, sets levels.
- A cover that resolves nowhere is a validation problem, but the path still loads: the
  view marks the cover missing. A proposal touching the file is refused only for problems it
  adds, so an agent can still move the cursor of a path with an old broken cover.
- A restructure (ADR-0014) rewrites the covers of every path like it rewrites `links:`;
  the API also resolves old ids through the rename log, deciding liveness on the subject id
  so a `subject#notion` cover is never redirected while the subject exists. Step keys the
  app does not know are kept through that rewrite.
- Agents write paths through `propose` (ADR-0004): `paths/*.md` is now a valid proposal
  path, flat only. The app moves the cursor with one commit (`PATCH /api/paths/{id}`).

## Study mode

"Continuer" opens a reading view over the map (`#/study/<path>/<step>`): the course outline,
the step's notions as lesson pages, a self-test from their cards, and "Étape terminée", which
moves the cursor with one commit and can be undone. Arrow keys browse without moving the
cursor. The map is paused, not unmounted, so coming back is instant.

The self-test writes real FSRS ratings (3 for "Je savais", 1 for "À revoir"), at most once per
card per study session, whether the card is due or not: answering right after reading is a
weak review, but Brian asked for one place to learn and test; the review log keeps the time,
so the schedule can be recomputed if this proves too generous.

## Confirmation

`internal/kb/paths_test.go` (status, next step, problems), `internal/app/paths_test.go`
(cursor commit, fiche back-references, restructure rewrite), `internal/httpapi/paths_test.go`,
MCP `paths` tests, and `rtk validate --data examples/knowledge` with
`examples/knowledge/paths/debuter-japonais.md`.
