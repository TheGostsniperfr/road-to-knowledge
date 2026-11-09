# MCP: connecting an agent to your knowledge

`rtk` serves the same tools two ways:

- **stdio**: `rtk mcp --data DIR`. The client starts the process; nothing listens on the network.
- **streamable HTTP**: `http://127.0.0.1:7878/mcp` while `rtk serve` runs. The web app and the
  agent then share one process, so the map updates live when an agent captures a note.

Both are covered by `cmd/rtk/mcp_e2e_test.go`, which builds the binary and calls every tool
through the go-sdk client over each transport.

## Claude Code

Pick one, from the project where you want the tools (or add `--scope user` for every project):

```bash
# stdio: works without rtk serve running
claude mcp add knowledge -- rtk mcp --data ~/Documents/aepita/other-project/knowledge

# HTTP: needs rtk serve running
claude mcp add --transport http knowledge http://127.0.0.1:7878/mcp
```

Or commit a project `.mcp.json` (see [`.mcp.json.example`](../.mcp.json.example)):

```json
{ "mcpServers": { "knowledge": { "command": "rtk", "args": ["mcp", "--data", "/home/you/knowledge"] } } }
```

`rtk` must be on the `PATH` (`make build` puts it in `bin/`), or give the absolute path.
Check with `claude mcp list`, then ask "what do I know about Kubernetes?".

Tested on 2026-11-09 with `claude -p` and a project-scoped config: `profile` and `at_risk`
over stdio, `search` over HTTP, and full runs of the `inbox-triage` and `teach-back` skills.

## Claude Desktop

`~/.config/Claude/claude_desktop_config.json` (Linux) or the equivalent on macOS:

```json
{
  "mcpServers": {
    "knowledge": {
      "command": "/absolute/path/to/rtk",
      "args": ["mcp", "--data", "/home/you/knowledge"]
    }
  }
}
```

Desktop does not inherit your shell `PATH`: use absolute paths.

## Tools

Sizes are measured on `examples/knowledge` (about 4 bytes per token). They grow with the
number of themes, not with the size of fiches, except `fragments`.

| Tool | Reads or writes | Typical size | Example call |
|---|---|---|---|
| `profile` | read | ~120 tokens | `profile()` |
| `subgraph` | read | ~150 tokens | `subgraph(id: "tech", depth: 2)` |
| `search` | read | ~40 tokens | `search(query: "kana")` |
| `node` | read | ~110 tokens | `node(id: "langues/japonais/kana")` |
| `fragments` | read | the fiche | `fragments(id: "…/kana", section: "Hiragana")`, `raw: true` for the whole file |
| `frontier` | read | ~120 tokens | `frontier(scope: "langues", limit: 6)` |
| `at_risk` | read | ~240 tokens | `at_risk(limit: 6)` |
| `due` | read | ~330 tokens | `due(scope: "langues/japonais", limit: 10)` |
| `recent` | read | ~190 tokens | `recent(days: 7)` |
| `inbox` | read | ~30 tokens per note | `inbox()` |
| `capture` | writes `inbox/` | ~10 tokens | `capture(text: "…", source: "claude-code")` |
| `propose` | writes `proposals/<slug>` | ~15 tokens | `propose(slug: "fiche-keigo", message: "feat(japonais): keigo", files: [{path, content}])` |

The server instructions tell the agent the order: `profile` first, then `search` or
`subgraph`, then `node`, and `fragments` only when the brief is not enough.

### What the server enforces

- Ids are paths (`theme/sub-theme/subject`). An unknown id returns an error that names the
  tool to find the right one; an unknown section lists the sections.
- `propose` writes on `proposals/<slug>` through a temporary worktree: your checkout and
  your main branch are never touched (ADR-0004).
- Before committing, a proposal is loaded the way the app will load it after the merge.
  A malformed file, a path at the wrong depth, an out-of-range level or a level lower than
  the current one (ADR-0003) refuses the whole proposal, with the reasons, and leaves no
  branch behind. A proposal whose slug starts with `triage-` cannot raise a level either:
  the unattended routine never decides that you know something better.
- A proposal can delete files only under `inbox/`, so a triage can clear the notes it
  placed; it can never delete a subject.
- `capture` never overwrites: two notes in the same second get two files. The `source`
  field is reduced to one quoted line, so it cannot add frontmatter keys.
- Every git call times out after 30 s; the whole process group is killed (hooks, signer) and
  the `index.lock` it left is removed, so the repo is usable right after. If commit signing
  is on in the knowledge repo and the key is locked, the error says so instead of hanging:
  unlock it (`echo ok | gpg --clearsign`) or turn signing off for that repo only
  (`git config commit.gpgsign false`).

## Skills

In [`skills/`](../skills), linked into `.claude/skills/` so Claude Code finds them in this repo.
To use them in another project, copy or link the folder into that project's `.claude/skills/`.

| Skill | What it does | Writes |
|---|---|---|
| `seed` | bootstraps a knowledge repo from GitHub, local projects and notes | `proposals/seed` |
| `inbox-triage` | places each inbox note in a subject, card or link, or drops it | `proposals/triage-<date>` |
| `teach-back` | quizzes you on subjects at risk with open questions, adds cards for what slipped | `proposals/teach-back-<date>` |

`inbox-triage` is built to run unattended in the evening: see the routine prompt at the end of
its `SKILL.md`.

## Security

The server binds to `127.0.0.1` and refuses requests whose `Host` is not localhost, and
cross-origin writes from a browser. There is no authentication: anything running as you on
this machine can call the tools. Do not expose port 7878 (no reverse proxy, no tunnel) until
V2-01 adds OAuth 2.1.
