# Road to Knowledge

A zoomable universe of what you know. Each theme is a galaxy, each sub-theme a star
cluster, each subject a star whose brightness is your level, and its notions are planets
in orbit. Subjects you have not explored yet sit in the fog. Books and subjects from
different galaxies are joined by wormholes.

Your knowledge stays in a git repository of plain Markdown files. The app reads it, draws
it, schedules your reviews and gives AI agents a cheap way to ask what you know, through
MCP, without loading everything.

## What it does

- **Map**: WebGL galaxies generated from your data. Spiral arms follow density-wave
  orbits, clusters follow a Plummer profile, star colours follow their temperature, and
  only the parts holding subjects shine. Zoom from the universe down to a subject's notions.
- **Fiches**: one Markdown file per subject, rendered with tables, images and Mermaid
  diagrams. `## Heading` sections become planets.
- **Review**: write `question :: answer` anywhere in a fiche. Cards are scheduled with
  FSRS from an append-only log, scoped to wherever you are zoomed.
- **Anki**: `rtk anki export` writes an `.apkg` (also a button on every theme), `rtk anki push`
  updates a running Anki through AnkiConnect, `rtk anki import` turns any deck into cards in
  `inbox/`. Re-exports update the same notes in Anki instead of duplicating them (ADR-0010).
- **Never forgets for you**: a level never drops on its own. A subject not reviewed for
  longer than its theme allows only gets an amber dot.
- **Veille**: list RSS or Atom feeds in a theme (`sources:`). The "Veille" tab shows the
  articles that mention your subjects, and the `morning-brief` skill turns them into a short
  brief each morning.
- **AI-ready**: `rtk mcp` exposes `profile`, `subgraph`, `node`, `fragments`, `search`,
  `frontier`, `news`, `at_risk`, `due`, `recent`, `inbox`, `capture` and `propose`. Agents write to
  `inbox/` or to a `proposals/*` branch, never to your main branch, and a proposal that would
  break a file or lower a level is refused. The "Ce que l'IA reçoit" tab shows exactly what an
  agent gets. Skills for Claude Code: `seed`, `book-ingest`, `inbox-triage`, `teach-back`, `morning-brief`. See [docs/MCP.md](docs/MCP.md).

## Quick start

```bash
nix develop            # or bring Go 1.26 and Node 24 yourself
make init
make build
./bin/rtk init ~/knowledge && cd ~/knowledge && git init && git add . && git commit -m "init"
./bin/rtk serve --data ~/knowledge     # http://127.0.0.1:7878
```

Try it on the bundled example first: `./bin/rtk serve --data examples/knowledge`.

Use it from Claude Code:

```bash
claude mcp add knowledge -- rtk mcp --data /path/to/knowledge               # stdio
claude mcp add --transport http knowledge http://127.0.0.1:7878/mcp          # while rtk serve runs
```

Claude Desktop, the tool list with sizes, and what the server enforces: [docs/MCP.md](docs/MCP.md).

## Knowledge format

```
knowledge/
  galaxies/<theme>/_galaxy.md                 name, hue, shape, review_after_days
  galaxies/<theme>/<sub-theme>/_cluster.md    name
  galaxies/<theme>/<sub-theme>/<subject>.md   name, level 0-5, state, links, evidence, tags
  review/log.jsonl                            append-only review log
  inbox/                                      quick captures waiting for triage
  assets/                                     images used by fiches
```

Details in [`docs/DATA-FORMAT.md`](docs/DATA-FORMAT.md).

## Development

`make dev DATA=examples/knowledge` runs the Go API and Vite with hot reload on
http://localhost:5173. `make check` runs what CI runs. Design and decisions:
[`docs/VISION.md`](docs/VISION.md), [`docs/ARCHITECTURE.md`](docs/ARCHITECTURE.md),
[`docs/adr/`](docs/adr/).

## Licence

Apache-2.0.
