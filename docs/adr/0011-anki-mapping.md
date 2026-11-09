# ADR-0011: How cards map to Anki

- Status: accepted, 2026-11-09

## Context

Brian already reviews in Anki on his phone, and shared Anki decks are the largest source of
ready-made cards. Cards must go both ways without duplicates piling up in either place, and
without a second source of truth for the schedule (ADR-0001, ADR-0006).

## Options

1. Export Anki's built-in Basic note type, guid random. Simple, but every export creates new
   notes: Anki cannot tell a re-export from new cards.
2. Our own note type with the card id in a field, guid derived from the card id. Re-exports
   and AnkiConnect pushes update the same note.
3. Option 2 plus the FSRS state and review log, both ways.

## Decision

Option 2.

- **Note type** "Road to Knowledge": fields Front, Back, Subject (breadcrumb shown under the
  answer), RtkID (the card id). One card per note.
- **Guid**: base91 of the first 64 bits of `sha1("rtk:" + card id)`, the same shape as Anki's
  own guids. The same card always exports with the same guid, so Anki updates the note when
  the answer changed and keeps its own schedule for it. Editing the question makes a new card
  id (DATA-FORMAT), so Anki gets a new note; the old one stays until deleted there. A
  question repeated in one subject would give two notes the same guid: export keeps the first
  and warns, and `rtk validate` reports the repeat.
- **Decks**: `RtK::<galaxy name>::<sub-theme name>`, parents included. Tags: `rtk`,
  `rtk::<galaxy id>::<sub-theme folder>` and the subject's tags.
- **No schedule in either direction.** Anki's scheduler and ours would each count the same
  review; the repo replays only `review/log.jsonl`. Exported cards arrive as new in Anki; an
  import never reads Anki's revlog. Brian picks one place to review each deck.
- **Import** never writes a subject or a binary file on the current branch (ADR-0004).
  Without media and without `--propose`, new cards are appended to `inbox/anki-<deck>.md`
  and committed like a capture; the import refuses to touch an inbox file with uncommitted
  changes. With `--propose`, or as soon as the deck carries images or sounds, everything goes
  to branch `proposals/anki-import` instead: there, cards whose RtkID names an existing
  subject go back into that subject's file, and the media sit in `assets/anki/` (never SVG
  or HTML). Cards whose card id or question already exists in a subject or in an inbox file
  are skipped. HTML is flattened to one line (script and style bodies dropped, entities
  decoded before tags are stripped), cloze notes give one card per deletion, media are
  linked on the line under the card.
- **Limits**: a package may hold at most 100,000 entries and expand to 1 GiB in total
  (512 MiB per entry); only media the notes refer to are read.
- **AnkiConnect** push adds and updates by RtkID, never deletes, only talks to a loopback
  address and never follows a redirect.

## Confirmation

`internal/anki`: round trip keeps questions, answers and guids; two exports give the same
guids; a package written by Anki 25.09 (`testdata/anki-25.09.apkg`, anki21b with zstd and
unicase collations) imports with HTML, cloze and media; the push adds, updates and leaves
unchanged notes against a fake AnkiConnect; `gitx.ValidAssetPath` refuses SVG and traversal;
`internal/app`: a re-import appends without repeating cards, a dirty inbox file is left alone,
media go to the proposal branch even without `--propose`.
Checked by hand with the Anki 25.09 library: importing the same export twice gives 0 new
notes the second time, and an export after an answer edit updates the note.
