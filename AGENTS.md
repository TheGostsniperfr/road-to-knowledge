# Working on Road to Knowledge

## Session protocol

Every session starts without memory. The repository carries the state.

**Start.** The `SessionStart` hook (`scripts/session-context.sh`) prints "Start here"
from `docs/PROGRESS.md`, the open items of the GitHub project board and the git state.
Run `make init` then `make check` and fix anything red before new work.

**Work.** One board item at a time, on a branch `feat/<name>` or `fix/<name>`, with a PR
that says `Closes #NN`. A choice a future reader would ask "why?" about gets an ADR from
`docs/adr/_template.md`; never rewrite an accepted ADR, supersede it.

**End.** Add an entry to `docs/PROGRESS.md` (what changed, what is open, next action),
update "Start here", commit.

## Backlog

GitHub project "Road to Knowledge" (owner TheGostsniperfr), columns Backlog, Todo,
In Progress, Review, Done. Read it with `gh project item-list <number> --owner TheGostsniperfr`.
Ids `V1-xx` and `V2-xx` come from `docs/ROADMAP.md` and appear in issue bodies.

## Read first

`docs/VISION.md`, `docs/ARCHITECTURE.md`, `docs/DATA-FORMAT.md`, `docs/adr/`.

## Rules

- The knowledge repo is the source of truth (ADR-0001). Never add state the files cannot rebuild.
- Levels never drop on their own (ADR-0003).
- Agent writes go to `inbox/` or `proposals/*` (ADR-0004). Never commit to the user's main branch from an agent path.
- Server stays on localhost until V2-01 adds OAuth.
- Go: `gofmt`, `go vet`, errors wrapped with context, table-driven tests with testify, test names that read as sentences.
- TypeScript: strict, no `any` outside the engine's GL glue, vitest for pure logic.
- Code and docs in English. The UI is in French for now.

## Commands

| Command | When |
|---|---|
| `make init` | after cloning or when dependencies changed |
| `make check` | before every commit; CI runs the same |
| `make dev DATA=examples/knowledge` | UI work with hot reload on :5173 |
| `make build && ./bin/rtk serve --data DIR` | run the real thing on :7878 |
| `rtk validate --data DIR` | check a knowledge repo |
| `rtk mcp --data DIR` | MCP over stdio for Claude Code |

## Layout

`cmd/rtk` CLI · `internal/*` Go packages (see ARCHITECTURE) · `web/` front end, built
into `web/dist` and embedded in the binary · `examples/knowledge` sample data ·
`skills/` agent skills, linked in `.claude/skills` (see `docs/MCP.md`): `seed` bootstraps a
knowledge repo, `book-ingest` turns a PDF into fiches and cards, `inbox-triage`,
`teach-back` and `morning-brief`.
