---
name: inbox-triage
description: Triage the Road to Knowledge inbox. Reads the notes captured during the day through the `knowledge` MCP server and proposes, on one proposals/triage-<date> branch, where each note goes (new subject, notion or card in an existing subject, link between subjects) or that it can be dropped. Use when the user asks to "trier l'inbox", "ranger mes notes", "triage", or when a scheduled evening routine runs it.
---

# Evening inbox triage

Goal: an inbox at zero after the user merges one branch, with every note placed where they
would have put it themselves. When in doubt, leave the note in the inbox: a wrong placement
costs more than a note that waits a day.

Needs the MCP server `knowledge` (`rtk mcp --data <repo>`, see docs/MCP.md). Never edit the
knowledge repo with file tools: every write goes through `propose`.

## 1. Read

1. `inbox()`. If `total` is 0, say so in one line and stop.
2. `profile()` once, to know the themes and their ids.
3. For each note, find where it belongs with `search(query)` on its key words, then
   `subgraph(theme id)` if the search is empty. Read `node(id)` of the best candidate; read
   `fragments(id, raw: true)` only for the subjects you will edit: it gives the whole file,
   frontmatter included, which you send back with your addition.

## 2. Decide, one line per note

| Note says | Action |
|---|---|
| a fact or definition about a known subject | add it to that subject: a sentence in its notion (`## heading`), or a `question :: answer` card if it is worth remembering word for word |
| something new, worth its own subject | new file `galaxies/<theme>/<sub-theme>/<slug>.md`, `level: 0`, `state: explore` (or `idea`), one summary line, the source in `evidence:` if it is a URL |
| "X is related to Y" | add Y's id to X's `links:` (links are symmetric when loaded: one side is enough) |
| an article or link to read later | new subject in `state: idea` if it opens a topic, otherwise a line in the closest subject |
| a todo, a duplicate, noise | drop it |
| unclear, or no fitting theme | leave it in the inbox and say why |

Rules:
- Never change `level` (ADR-0003: levels move only when the user decides). The server refuses
  a proposal that lowers one; do not raise one either.
- Keep the user's words. French stays French.
- Edit an existing subject only by sending its whole current file from `fragments(id, raw: true)`
  with your addition, nothing else changed. Never read the repository with file tools.
- New ids are lowercase slugs with dashes, inside an existing theme and sub-theme. Create a
  new sub-theme only when three or more notes call for it.

## 3. Propose

One call, slug `triage-<YYYY-MM-DD>`, message `feat(inbox): triage du <date>`:
- every created or edited subject file;
- every placed or dropped note with `{"path": "inbox/<file>", "delete": true}`;
- nothing for the notes you leave in the inbox.

If `propose` is refused, read the reasons, fix the files and call again with the same slug.

## 4. Report

A table: note (first words) · decision · target id. Then the command to review:
`git diff main...proposals/triage-<date>` in the knowledge repo, and the notes left in the inbox
with the question that would settle each one.

## Running it every evening

As a Claude Code routine (`/schedule`), in a project where the `knowledge` MCP server is
configured, cron `0 21 * * *`, prompt:

> Run the inbox-triage skill on the knowledge MCP server. Do not ask questions; leave unclear
> notes in the inbox and list them in your report.

The routine only creates a branch; the user merges it when they want.
