---
name: morning-brief
description: Write Brian's short morning brief from his Road to Knowledge universe: what is new in the feeds he follows about subjects he knows, what to learn next, and the review backlog. Keeps the one to three articles worth reading in the inbox. Use when asked for "le brief", "ma veille", "quoi de neuf ce matin", or from a scheduled morning routine.
---

# Morning brief

Goal: five minutes of reading, not a digest of everything. The brief is French, like the app.

Needs the MCP server `knowledge` (`rtk mcp --data <repo>` or `http://127.0.0.1:7878/mcp`).
If it is missing, say so and stop: never read the knowledge files directly for this.

## 1. Collect

- `news(since: "1d")`: articles from the feeds of each theme, newest first, with the known
  subjects they mention. Use `since: "3d"` on Mondays. If `broken_feeds` is not empty, note it
  in one line at the end.
- `frontier(limit: 5)`: what to learn next, with the reason for each suggestion.
- `profile()`: only the review backlog line (cards due) and the subjects in progress.

Do not call `fragments()` or `search()` for the brief.

## 2. Choose

From `news`, keep at most five items, in this order of value:

1. An item about a subject in progress, or at level 1 to 3: it moves what Brian is learning.
2. An item about a level 4-5 subject that announces a change (release, deprecation,
   security): he runs it in production.
3. An item that touches a frontier suggestion: it is the natural next step.

Drop marketing, event announcements and anything already covered by a kept item.

## 3. Write

```
## Brief du <jour> <date>

**À lire** (n)
- <titre> — <source>. <Une phrase : pourquoi pour toi, en citant le sujet et son niveau.> <lien>

**À explorer**
- <sujet> : <raison donnée par frontier>

**Révisions** : <n> cartes dues. <Une phrase si un sujet en cours n'a pas été révisé depuis longtemps.>
```

Never more than 25 lines. No item without a reason tied to a subject.

## 4. Keep

For the one to three best items, call `capture(text, source: "morning-brief")` with the title,
the link and the one-line reason. Nothing else is written: no `propose()`, no level change.

## Scheduling

Run it every weekday morning with a Claude Code routine (`/schedule`), for example
`0 8 * * 1-5` with the prompt "Lance la skill morning-brief et écris le brief." The routine
needs the MCP server reachable: keep `rtk serve` running, which also refreshes the feeds
every six hours (`--news-every`), or run `rtk news fetch --data <repo>` first.
