# Knowledge format

The knowledge repository is the source of truth (ADR-0001). Everything the app shows can
be rebuilt from these files; nothing else is stored.

## Layout

```
galaxies/<theme>/_galaxy.md
galaxies/<theme>/<sub-theme>/_cluster.md
galaxies/<theme>/<sub-theme>/<subject>.md
review/log.jsonl
inbox/*.md
assets/*
layout.json
library/*          git-ignored originals of books and long documents (ADR-0010)
```

The folder names are the ids: `langues/japonais/kana`. Renaming a folder renames the id,
so links and review history follow only if you update them too. `_galaxy.md` and
`_cluster.md` are optional; without them the folder name is the display name.

## `_galaxy.md`

```yaml
---
name: Langues
hue: 168                 # 0-360, the galaxy's colour; derived from the id when absent
shape: spiral            # spiral | barred | ring | elliptical | lenticular | irregular
review_after_days: 21    # after this, an unreviewed subject gets the amber dot
discovery: { near: 70, two_hops: 20, off_piste: 10, news: false }   # shares sum to 100
sources:                 # RSS or Atom feeds for the "Veille" tab and news()
  - url: https://www.tofugu.com/feed.xml
    name: Tofugu
---
```

`discovery` shares the suggestions of "À explorer" and `frontier()` between three
buckets, in percent. Candidates are subjects in state `idea` or `explore` below level 3; a subject is
known at level 3 or more, or when marked `acquired`.

- `near`: in a sub-theme where you know something, or linked to a subject you know.
- `two_hops`: two links away from a subject you know.
- `off_piste`: no known neighbour, least covered sub-themes first. Taken from the scope and one level above
  the scope, so a sub-theme points to its siblings and a galaxy to other galaxies.

Percentages cannot be negative and must sum to 100. A bucket without enough candidates gives its slots to the others.

`news` decides what the theme's feeds may show. Off (the default), only articles that
mention one of your subjects or sub-themes appear; on, every article of the theme's feeds
appears, so the news can bring subjects you do not have yet. "Tout" in the app and
`all: true` in `news()` show everything regardless.

`sources` lists feeds (RSS 2.0, RSS 1.0 or Atom) for the theme; `_cluster.md` accepts the
same key for a sub-theme. Only `http`/`https` URLs without credentials are accepted; others
are reported as problems. Articles are matched to subjects by name, by each part of a
compound name ("Helm et Kustomize"), by `tags` and by sub-theme name. Names under three
characters ("Go", "C") only match through a longer tag such as `golang`.

Fetched articles are a cache in `.rtk/news/` (git-ignored, kept 30 days), never knowledge:
delete the folder and the next fetch rebuilds it. `rtk serve` fetches at start and every
six hours (`--news-every`), `rtk news fetch` on demand.

The galaxy fiche in the app edits `review_after_days` and `discovery` and commits the file.

## Subject file

```markdown
---
name: Kana
level: 4                 # 0 fog, 1 heard of, 2 understood, 3 practised in a lab, 4 used in production, 5 can teach it
state: acquired          # idea | explore | progress | acquired; derived from level when absent
links: [genki-1]         # wormholes: full ids or bare subject slugs, made symmetric on load
evidence: [https://github.com/me/lab]   # what backs the level
tags: [writing-system]
reviewed: 2026-10-28     # set by the app when you confirm a level or state
---
First paragraph: the summary shown in lists and given to agents.

あ :: a                   # a review card, anywhere outside code fences
- か :: ka               # list items work too

## Hiragana               # each level-two heading is a notion, drawn as a planet
46 signes arrondis.
```

Images: `![](assets/kana.png)` from any fiche; the server exposes `assets/` read-only.
Diagrams: fenced `mermaid` blocks are rendered in the fiche.

## `source:` (books and long documents)

A subject written from a book or a long document says where it comes from. The
`book-ingest` skill fills it from `rtk book add FILE`, which copies the file into the
git-ignored `library/` and prints this block (ADR-0010).

```yaml
source:
  title: Le journal avant tout     # required
  author: Road to Knowledge
  year: 2026
  pages: 7
  file: library/journal-6e5505f8.pdf   # optional, must be inside library/
  sha256: 6e5505f8…                    # 64 hex chars, finds the file again on another machine
  url: https://…                       # optional, for online documents
```

The fiche shows the source and links to the file when it is on this machine; the server
serves `library/` read-only at `/library/`. A notion heading that ends with `(p. N)`
opens the PDF at that page. Book notions are written `## À retenir : <idée> (p. N)`.
An invalid source (no title, a path outside `library/`, a bad hash) is a validation
problem and is ignored; the subject still loads.

## `layout.json`

Optional. Pins galaxy positions so adding a theme never moves the others (ADR-0008).
The app writes it when you press "Figer la carte"; you can also edit it by hand.

```json
{
  "galaxies": {
    "tech": { "x": -1699, "y": -42 },
    "langues": { "x": 1337, "y": 2145, "rotation": 1.2 }
  }
}
```

Coordinates are map units, within ±1,000,000. `rotation` (radians) turns the galaxy in its
plane; without it the angle comes from the id. A galaxy missing from the file is placed
automatically around the pinned ones.

## Review log

`review/log.jsonl`, one review per line, never rewritten:

```json
{"card":"langues/japonais/kana:1f3a9c20","rating":3,"at":"2026-11-08T21:04:00Z"}
```

A card id is the subject id plus a hash of the question, so editing the answer keeps the
history and editing the question starts a new card. Ratings: 1 again, 2 hard, 3 good,
4 easy. The schedule is rebuilt by replaying the log in time order (ADR-0006).

## Validation

`rtk validate --data DIR` lists every problem and exits 1 if there is one. The app shows
the same list in a banner and keeps working on the valid files.
