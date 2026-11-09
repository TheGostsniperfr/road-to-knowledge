# Progress

Handoff between sessions, newest entry first. Update it at the end of every session:
what changed, what is open, where the next session starts.

## Start here

1. `make init`, then `make check`: fix anything red before new work.
2. Backlog: the GitHub project board "Road to Knowledge" (`scripts/session-context.sh`
   prints the open items). Roadmap and ids: [`ROADMAP.md`](ROADMAP.md).
3. Next action: V1-08. Brian uses V1 on his real knowledge repo
   (`TheGostsniperfr/knowledge`) for a week; collect feedback in issues, then pick V2-01
   or V2-02 with him.

## Log

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
