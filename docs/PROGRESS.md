# Progress

Handoff between sessions, newest entry first. Update it at the end of every session:
what changed, what is open, where the next session starts.

## Start here

1. `make init`, then `make check`: fix anything red before new work.
2. Backlog: the GitHub project board "Road to Knowledge" (`scripts/session-context.sh`
   prints the open items). Roadmap and ids: [`ROADMAP.md`](ROADMAP.md).
3. Next action: V3-01. Write the Genki I, Genki II and CCNA paths into Brian's knowledge repo
   (`TheGostsniperfr/knowledge`) through Sonnet agents, with the cursor where he is (Genki II
   L13, workbook ex. 5, potential form; CCNA video 41, Day 21 BPDU Guard and Filter). The
   Genki II PDFs are scans without a text layer: read pages as images. The CCNA playlist
   order is in the V2-13 log entry. V1-08 (a week of daily use) runs alongside.

## Log

### 2026-11-09 · V2-13 learning paths

- `paths/<id>.md` (ADR-0015): a course's reading order laid over the map. Steps cover subject
  or notion ids; `at` is the cursor; steps before it are done; `done: true` for a step seen
  out of order. A path never changes a level.
- `kb` loads and validates paths (a bad cover names the subject's notion ids); `app` gives
  progress, next step and, in each fiche, the steps that cover it; `PATCH /api/paths/{id}`
  moves the cursor with one commit; restructures rewrite covers; `propose` accepts
  `paths/*.md`; MCP tool `paths`.
- UI: a "Parcours" block in the fiche of the universe, a theme and a sub-theme lists the paths
  that cover it (progress, "Continuer" to the step's first subject, steps around the cursor
  with "J'en suis là" and "Étape terminée"); a subject's fiche lists the steps that cover it;
  the HUD has a pill per course under way. "Demander un parcours" drops a request in the inbox.
  A first version used a fifth tab in the left panel; Brian found it hard to discover.
- Research for the next steps (not in the repo): Jeremy's IT Lab playlist
  `PLxbwE86jKRgMpuZuLBivzlM8s2Dk5lXBQ` has 126 videos, position 41 is Day 21 part 2 (BPDU
  Guard and Filter); his Anki decks sit behind an email sign-up at jitl.jp/ccna-files.
  Genki II textbook p. 6 of the PDF lists the kanji of each lesson, L3 to L23.
- Study mode (same day): "Continuer", the resume dock at the bottom of the map, a fiche's
  course steps and the E key open a reading view per course (outline grouped by lesson,
  notions as lesson pages, self-test feeding FSRS, "Étape terminée" with undo). Reviewed by a
  code reviewer and a design reviewer; their MUST items are fixed.
- Map: planet labels no longer stay on screen after their planet is gone; a sub-theme only
  draws the notions of the subject in focus.
- Content (branch proposals/parcours-cours of the knowledge repo, worktree
  ../knowledge-parcours): Genki I and II and the CCNA paths, topic subjects, L13 and STP
  notions rewritten as lessons. Not merged yet.
- Open: Brian's Chrome stops painting a page with Japanese text when driven by Claude in
  Chrome (headless renders fine, CPU idle); to confirm on his screen.
- Open: V3 items in the roadmap (course content, `expand` skill, sticking points, learn
  mode, goals, practice mode).

### 2026-11-09 · V2-08 restructure: move, rename, split, merge

- `internal/app/restructure.go` (ADR-0014): plans an operation from the current files (dry
  run), then applies it with one commit (app, `rtk mv|split|merge`) or writes it on a
  proposal branch (MCP `restructure`). Links in other subjects are rewritten; a merge keeps
  the highest level and refuses a question with two different answers.
- `review/renames.jsonl`: append-only id changes. The review log is replayed through it with
  each review's own time, so history survives move, split, merge and round trips; old ids
  redirect in the API, MCP tools and deep links (`redirects` in `/api/universe`).
- `gitx`: a change may move a file (`From`) and a restructure may delete subjects and write
  the rename log; `propose` from an agent still cannot. `Commit` stages deletions.
- UI: "Réorganiser" in the subject fiche (Déplacer, Renommer, Scinder, Fusionner avec…)
  shows the plan before applying, in both themes.
- Safety: an apply is all or nothing (undo in reverse, rename log last, index reset on a failed
  commit); the app applies only the plan it showed (hash, 409 otherwise); a live id is never
  redirected; an existing proposal branch is never overwritten; an emptied sub-theme keeps a
  `_cluster.md`; deep links to moved notions are rewritten.
- Open: galaxy (theme) renames are not supported; a sub-theme can only move into an existing
  theme; the UI splits one part at a time (the CLI takes several).

### 2026-11-09 · V2-02 search by meaning

- `internal/embed` + `internal/semantic` (ADR-0013): embeddings in process with hugot's pure-Go
  backend, no Ollama, no cgo. Default model multilingual-e5-small (135 MB, ~65 ms per search
  on CPU); EmbeddingGemma-300m opt-in (`--model embeddinggemma-300m`, 1.26 GB, more precise).
  `rtk models pull [--verify]`; vectors cached in `.rtk/vectors`, rebuilt in background.
- Hybrid search (words + meaning, rank fusion) in Ctrl K, API and MCP `search`; word search
  now drops stopwords and falls back from AND to OR. New MCP tool `neighbors`; frontier uses
  semantic proximity.
- `rtk mcp` (stdio) loads no model by default; use the HTTP transport of `rtk serve` for
  search by meaning. `rtk bench search` reports tuning and held-out sets.
- Open: inbox triage and news matching do not call `neighbors` yet; sets are small.

### 2026-11-09 · V2 wave 2: Anki, books, news, AI-ready

- V2-12 (#29): every MCP tool exercised from a real client over stdio and HTTP
  (`cmd/rtk/mcp_e2e_test.go`), and with `claude -p`. Fixed capture collisions and
  frontmatter injection; `propose` now validates like the app loads, refuses level drops,
  refuses raises from `triage-*` slugs, may delete `inbox/` notes only; git calls time out
  and kill hung hooks. New tools `due`, `at_risk`, `recent`, `inbox`. Setup in `docs/MCP.md`.
- V2-07 / V2-10: skills `inbox-triage` (evening routine) and `teach-back`.
- V2-04 (#26): `book-ingest` skill, `rtk book add`, git-ignored `library/` (ADR-0010),
  `source:` with page links in the fiche, `/library/` and `/assets/` served through `os.Root`
  with `CSP: sandbox`. Skills propose through a worktree, never `git switch` the live checkout.
- V2-03 (#27): `rtk anki export|import|push`, export button, stable GUIDs, no review
  history exchanged (ADR-0011). Imports with media go to `proposals/anki-import`.
- V2-06 (#28): `sources:` feeds per theme, built-in RSS/Atom reader with cache in
  `.rtk/news` (ADR-0012), "Veille" tab, MCP `news`, `morning-brief` skill.
- Every PR had an independent review; all MAJOR findings fixed before merge.
- Open: Brian's knowledge repo has no `sources:` yet; `rtk anki push` untested against a real
  Anki desktop; book-ingest not yet run on a 300-page book; `at_risk` uses rules, not FSRS
  retrievability.

### 2026-11-09 · V2-11 atlas view

- `web/src/atlas`: circle packing of themes, sub-themes and subjects (ADR-0009), Canvas2D,
  van Wijk zoom, labels that adapt to the zoom, links bent toward their common theme and
  highlighted around the focused subject, legend for levels.
- Switch "Galaxie / Atlas" in the HUD, light and dark atlas themes; both stored per device.
- Panels speak plain words in the atlas (sujets, thèmes, liens) through `ui/terms.ts`.
- Colours of the panels are now tokens in `styles.css`, so a theme only redefines variables.
- Open: offscreen link targets are not labelled at the edge of the view.

### 2026-11-09 · V2-09 pinned positions and galaxy settings

- `layout.json` (ADR-0008): optional pins per galaxy, loaded and validated by `kb`, written
  by `PUT /api/layout` with one commit. `placeGalaxies` keeps pins fixed and places only
  new galaxies around them. "Figer la carte" in the universe fiche writes the current
  positions; nothing is pinned without that gesture.
- Galaxy fiche edits `review_after_days` and the discovery budget (sliders that always
  sum to 100) through `PATCH /api/galaxy/{id}`; `_galaxy.md` is created if missing.
- Validation: discovery shares that do not sum to 100 are now a problem.
- The watcher also reloads on `layout.json` at the repo root.

### 2026-11-09 · V2-05 discovery frontier

- `app.Frontier`: ranks `idea`/`explore` subjects for a scope into near, two hops and off
  piste, following the galaxy's `discovery` budget, with a French reason per suggestion.
  At universe and galaxy scope it alternates themes so one dense theme cannot fill the list.
- Exposed as `GET /api/frontier` and the MCP tool `frontier(scope, limit)`.
- "À explorer" tab: scope chips (universe, galaxy, sub-theme of the selection), groups per
  bucket with their budget, then subjects in progress.
- Open: the seeded repo has few links on fog subjects, so `two_hops` is often empty there;
  adding `links:` to fog subjects makes the suggestions better.

### 2026-11-09 · V1

- V1-00 to V1-07 built in one session from the validated design artifact (v4.4).
- Go: `kb`, `index`, `review`, `gitx`, `app`, `httpapi`, `mcpserver`, CLI `rtk`. Tests
  for every package, including API integration on a real git repo and MCP over in-memory
  transports.
- Web: WebGL engine ported to TypeScript, data-driven layout with tests, Preact panels.
  Verified in headless Chrome on `examples/knowledge`: map, deep zoom to a star system,
  fiche with Mermaid, review session committed on close, level change committed.
- Known limits: galaxy positions shift a little when a theme is added (V2-09 pins);
  the UI is in French only; no mobile layout.
