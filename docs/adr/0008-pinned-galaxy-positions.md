# ADR-0008: Galaxy positions pinned in layout.json, on request

- Status: accepted, 2026-11-09

## Context
Galaxies are placed from the data: the biggest in the middle, the others on a spiral and
pushed apart. Adding a theme changes the masses and the order, so known galaxies drift.
Users navigate by memory of the map, and the map must still be rebuildable from the repo
(ADR-0001).

## Options
- Store positions in browser storage: lost on another machine, not in git.
- Store a position in each `_galaxy.md`: spreads one concern over many files and many
  commits, and a galaxy without `_galaxy.md` would need one created.
- One `layout.json` at the repo root, keyed by galaxy id.
- Pin automatically on first load: commits on the user's behalf without a gesture, and
  freezes a placement the user may not have looked at yet.

## Decision
`layout.json` holds `{ "galaxies": { "<id>": { "x", "y", "rotation"? } } }`. It is
optional. Pinned galaxies never move; unpinned ones take their spiral slot and are pushed
away from pinned ones only. The app writes it when the user presses "Figer la carte" in
the universe fiche (`PUT /api/layout`, one commit). Unknown ids and out-of-range values are
validation problems, and the galaxy falls back to automatic placement.

## Confirmation
`TestLoad_ReportsProblems_WhenLayoutIsWrong`, `TestPutLayout_PinsGalaxiesAndCommits`, and
the vitest case "keeps pinned galaxies fixed when a theme is added".
