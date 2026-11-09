# Architecture

One Go binary, `rtk`, over one knowledge folder.

```
knowledge repo (Markdown, git)
        │ fsnotify, debounced reload
        ▼
internal/kb        parse folders and frontmatter, cards, notions, links, problems
internal/index     SQLite FTS5 in memory, rebuilt on reload
internal/library   git-ignored library/ of book originals, named by content hash (ADR-0010)
internal/review    FSRS, append-only log replayed in time order
internal/gitx      commits from the app; proposals/* branches through a temporary worktree
        ▼
internal/app       Service: the loaded universe and the views shared by API and MCP
   ├── internal/httpapi    JSON API, SSE reload events, /assets, /mcp, embedded web app
   └── internal/mcpserver  MCP tools over stdio (`rtk mcp`) or HTTP (`/mcp`), see MCP.md

web/ (Vite, TypeScript, Preact)
   src/sky/layout.ts     deterministic placement from ids and data
   src/sky/particles.ts  galaxy and cluster particles, physics-inspired
   src/sky/gl.ts         WebGL2: nebula shader, particles, slot texture
   src/sky/engine.ts     camera (van Wijk zoom), overlay of knowledge stars, input
   src/atlas/pack.ts     atlas view: circle packing of themes, sub-themes, subjects
   src/atlas/atlas.ts    atlas renderer (Canvas2D), same surface as the sky engine
   src/ui/prefs.ts       per-device view and theme choice
   src/ui/*              panels, fiche, AI preview, review drawer
```

## Flows

- **Edit a fiche in your editor**: fsnotify sees the file, the service reloads, the
  server sends `reload` over `/api/events`, the UI refetches and redraws.
- **Change a level in the app**: `PATCH /api/star/{id}` rewrites only that frontmatter
  key, commits the file, reloads. `PATCH /api/galaxy/{id}` does the same for review and
  discovery settings in `_galaxy.md`, `PUT /api/layout` for pinned positions.
- **Review**: each grade appends one line to `review/log.jsonl`; closing the drawer
  commits the log once.
- **What next**: `app.Frontier` ranks `idea` and `explore` subjects around a scope and
  splits them by the galaxy's `discovery` budget; the "À explorer" tab and the MCP tool
  `frontier` read the same function, through `GET /api/frontier`.
- **Agent**: `propose` writes files on `proposals/<slug>` from a worktree in a temp dir;
  the user reviews with `git diff main...proposals/<slug>` and merges.

## Security

The server listens on 127.0.0.1 only and rejects other Host headers (DNS rebinding) and
cross-origin writes. Agents can only write `.md` files under `galaxies/` or `inbox/`, on a
branch. `assets/` is served read-only through `os.DirFS`, which refuses `..`. A knowledge
folder nested inside another repository never receives commits.

## Rendering budget

About 240 000 particles for a large universe, drawn in four calls. Slot data (centre,
scale, orientation, alpha per galaxy or cluster) is a float texture, so the number of
sub-themes is not limited by uniforms. The nebula renders at half resolution and only
when the camera moves.
