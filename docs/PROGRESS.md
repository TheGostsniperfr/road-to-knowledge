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
