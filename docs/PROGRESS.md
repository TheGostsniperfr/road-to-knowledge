# Progress

Handoff between sessions, newest entry first. Update it at the end of every session:
what changed, what is open, where the next session starts.

## Start here

1. `make init`, then `make check`: fix anything red before new work.
2. Backlog: the GitHub project board "Road to Knowledge" (`scripts/session-context.sh`
   prints the open items). Roadmap and ids: [`ROADMAP.md`](ROADMAP.md).
3. Next action: V1-08. Brian uses the app on his real knowledge repo
   (`TheGostsniperfr/knowledge`) for a week, with the MCP server connected (`docs/MCP.md`);
   collect feedback in issues. Then V2-02 (semantic, Ollama), V2-08 (granularity) or V2-01
   (mobile) with him.

## Log

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
