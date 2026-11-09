# ADR-0009: A second map, the atlas, beside the galaxy

- Status: accepted, 2026-11-09

## Context

The galaxy map is the identity of the project and the way Brian wants to explore his own
knowledge. Shown to a colleague or in a meeting it reads as a game: stars, nebulae and
French space vocabulary on a black field. The same data needs a view that looks like a
work tool, without a second data model or a second set of panels.

## Options

1. Restyle the galaxy (lighter background, flatter particles). Keeps one renderer, but
   the metaphor stays and the realism the galaxy was tuned for is lost.
2. A node-link graph with force layout. Familiar, but positions move between loads and
   dense themes turn into hairballs.
3. Circle packing: themes contain sub-themes contain subjects, area grows with level,
   links drawn as curves bent toward the smallest circle holding both ends. Deterministic,
   hierarchy readable at a glance, zoom maps onto the same four scales as the galaxy.

## Decision

Option 3, as `web/src/atlas`, a Canvas2D renderer with the same public surface as the
galaxy engine (`setUniverse`, `select`, `zoomBy`, `destroy`, `onSelect`, `onScale`). Both
views build their nodes with `buildLayout`, so the panels, the hash deep links and the
review scope do not know which map is shown. The atlas comes with its own light and dark
themes (Inter, neutral greys, the Arffornia orange as the only accent); the galaxy keeps
the observatory theme. The choice of view and theme is a per-device preference in
`localStorage`, never in the knowledge repo.

## Confirmation

`web/src/atlas/pack.test.ts`: circles nest inside their parent, a higher level gives a
bigger circle, the layout is deterministic, links are deduplicated, hit testing returns
the deepest circle.
