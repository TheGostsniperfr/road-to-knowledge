# ADR-0012: A built-in feed reader for the news, Miniflux later

- Status: accepted, 2026-11-09

## Context

V2-06 plans a morning brief fed by news about the themes Brian follows. The roadmap named
Miniflux as the collector. In V1 everything runs on his PC (ADR-0007): one more service to
install, keep running and authenticate against is friction for a feature that reads a
dozen feeds a few times a day.

## Options

1. Miniflux now. Mature, good parsing, a web UI of its own. Needs PostgreSQL and a running
   service, and the feed list would live in Miniflux, not in the knowledge repo.
2. A Claude routine that browses the sites each morning. No code, but slow, costly, and
   unreliable to tell what is new since yesterday.
3. A small RSS/Atom reader inside `rtk`: feeds declared as `sources:` in the theme and
   sub-theme files, items cached in `.rtk/news/`, an HTTP endpoint and an MCP tool.

## Decision

Option 3. The feed list is knowledge (which sources you trust for a theme), so it lives in
`_galaxy.md` and `_cluster.md` and is versioned. Fetched items are cache: `.rtk/news/state.json`,
git-ignored, rebuilt by the next fetch, never committed. Matching an item to subjects happens
when it is read, so a subject added today also tags last week's items.

Safety: URLs are user-authored, but only http(s) is fetched, redirects to another scheme are
refused, credentials in a URL are a validation problem (they would be committed), responses
are capped at 4 MiB, requests time out after 20 s and failing feeds back off up to a day.
Links shown in the UI are http(s) only.

`news.Fetcher` is the seam: a Miniflux adapter can later fill the same `Store` from the
Miniflux API when the server moves off the PC (V2-01).

## Confirmation

`internal/news/news_test.go` (parsing RSS 2.0, RSS 1.0 and Atom, conditional GET, backoff,
refused redirect to `file:`, size limit, retention, matching) and
`internal/httpapi/news_test.go` (the fetch never commits, scope and date filters).
