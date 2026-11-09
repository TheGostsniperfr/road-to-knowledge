---
name: book-ingest
description: Turn a book or long document (PDF, EPUB, Markdown) into Road to Knowledge fiches and review cards, on a proposals/book-<slug> branch for review. Use when the user gives a book, a PDF, a course or a long article and asks to "en faire des fiches", "ingérer ce livre", "fiches mémo", "cartes de révision".
---

# Ingest a book

Goal: in one session, a fiche the user would have written after reading the book with a
pen in hand. Few, sharp notions; atomic cards; links to what they already know. Not a
summary of every page.

## 0. Put the file in the library

```sh
rtk book add --data "$RTK_DATA" --title "<title>" --author "<author>" --year <year> <file>
```

It copies the file to `library/` (git-ignored, ADR-0010) and prints the `source:` block
to paste in the fiche. Never commit the book itself. If `rtk` is not on the PATH, run it
from the app repo with `go run ./cmd/rtk`.

## 1. Map the book before reading it

Work from a scratch outline file (`<scratchpad>/book-<slug>.md`), never from memory, and
never load the whole book at once.

- PDF: `pdfinfo <file>` for the page count, then the table of contents. Prefer text:
  `pdftotext -layout -f <first> -l <last> <file> -` costs a fraction of the tokens of page
  images. Use the Read tool with `pages: "12-20"` (at most 20 pages per call) only for
  pages where figures or tables matter. Both need poppler (`pdftoppm`, `pdftotext`): the
  app's `nix develop` shell has it, otherwise `nix shell nixpkgs#poppler-utils`. A PDF
  under ~10 pages can be read whole without page ranges. Note the printed page offset
  (book page 1 is often PDF page 15) and cite book pages in the fiche.
- EPUB: `unzip -l`, then read the nav/toc file and one chapter file at a time.
- Markdown or text: `grep -n '^#'` for the outline, then read by line ranges.

Write in the outline file: title, author, year, page count, and one line per chapter with
its page range. Ask the user at most two questions: which chapters matter (default: all),
and their current level on the topic (default: 1).

## 2. Read chapter by chapter

For each chapter, read its range (`pdftotext -layout -f a -l b`), then append to the outline file, in your own words:

- 1 to 3 ideas worth keeping, each with the book page where it is explained,
- candidate cards (question :: answer),
- terms that may match existing subjects.

Then drop the chapter from your working memory: the outline file is the state. For a
300+ page book this is 15 to 30 reads; keep going until the chapters chosen are done.

## 3. Find where it plugs in

- With the `knowledge` MCP server: `search()` each key term, `subgraph()` the likely
  themes, `node()` the matches. Without it: `grep -ril <term> galaxies/` in the repo.
- Keep links (wormholes) only to subjects the book really explains or uses; 2 to 6 is
  typical. A link is a bare slug or a full id in `links:`.
- New subjects the book opens that the user does not know: add them as fog subjects
  (level 0, state `explore`) in the right theme, one summary line each, at most 3.

## 4. Write the fiche

One subject for the book, in the user's book galaxy: the galaxy whose `_galaxy.md` has
`shape: lenticular` if there is one, otherwise create `galaxies/livres/_galaxy.md`
(`name: Livres`, `shape: lenticular`). Put it in a sub-theme named after the domain
(`tech`, `science`, `management`, …; create `_cluster.md` if missing). File name: the
book's slug.

```markdown
---
name: <Title>
level: 1                      # 1-2 unless the user says otherwise; reading is not mastering
state: progress
links: [<slug>, <theme/sub/slug>]
tags: [livre]
source:                       # pasted from `rtk book add`
  title: <Title>
  author: <Author>
  year: <Year>
  pages: <N>
  file: library/<slug>-<hash8>.pdf
  sha256: <hash>
---
One paragraph: what the book argues and who it is for, in the user's voice.

## À retenir : <idea> (p. 41)
Two to five sentences, paraphrased. Why it matters to the user if you know.

<question> :: <answer>
```

Notions (`##` headings) are the planets on the map: one per idea worth keeping, 4 to 12
for a book, each ending with `(p. N)` so the app opens the PDF at that page.

## 5. Cards

10 to 40 per book, placed under the notion they test. Rules, from spaced repetition practice:

- One fact per card. If the answer has "and", split it.
- The answer is a word, a number, a name or one short sentence. Never a list; turn a
  list of N items into N cards or into one "why" card.
- Ask for understanding, not wording: "Why does X need Y?" over "Complete: X needs ___".
  No cloze phrasings copied from the text.
- The question must make sense alone, months later, without the book: name the context.
- No card for what the user already knows at level 3 or more (check `node()`).
- Avoid yes/no questions and "what are the three…" questions.

## 6. Copyright

The fiche is the user's notes. Paraphrase everything. Quote at most one sentence at a
time, in quotation marks, with its page, and only when the exact words matter. Never
copy tables, code listings or figures; describe them. Never commit the source file.

## 7. Propose, never commit to main

With the MCP server: one `propose()` call, `slug: book-<slug>`, message
`feat(book): <title>`, files = the fiche, any new `_galaxy.md`/`_cluster.md`, and the fog
subjects. Without it, in the knowledge repo:

```sh
git switch -c proposals/book-<slug>
# write the files
rtk validate --data .
git add galaxies/livres <other paths> && git commit -m "feat(book): <title>"
```

Fix every problem `rtk validate` reports before proposing.

## 8. Hand over

Print a table: notions · cards · links · fog subjects added, the branch name, and how to
look at it (`git diff main...proposals/book-<slug>`, or `rtk serve` on that branch), plus
the chapters skipped and any card you were unsure about.
