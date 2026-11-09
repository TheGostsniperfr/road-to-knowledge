# ADR-0010: Books live in a git-ignored library, fiches point to them by hash

- Status: accepted, 2026-11-09

## Context

V2-04 turns a book (PDF, EPUB, Markdown) into fiches and review cards. The fiches are
the user's own notes and belong in the knowledge repo like any subject. The original file
is a different kind of thing: 5 to 50 MB per book, binary, and usually under a licence
that does not allow copying it to a remote, even a private one. The server runs on
Brian's PC; there is no object store (the earlier plan named MinIO).

## Options

1. Commit the PDFs. One clone has everything, but git keeps every byte forever, the
   private GitHub repo grows by hundreds of MB, GitHub refuses files over 100 MB, and the
   books are redistributed to a third party.
2. Git LFS. Solves size, not licence, and adds a server-side quota and a client tool.
3. A `library/` folder inside the knowledge repo, listed in `.gitignore`. The fiche
   carries `source.file` (the path) and `source.sha256`. Another machine sees the fiche,
   the cards and the title, and knows exactly which file is missing.
4. MinIO or another object store. Right for a hosted V2 with a phone, too much for a PC.

## Decision

Option 3. `rtk book add FILE` copies the document to `library/<slug>-<hash8>.<ext>`,
adds `library/` to the repo's `.gitignore` if needed, and prints the `source:` block. The
server serves `library/` read-only at `/library/` (localhost guard as everything else) so
the fiche opens the PDF, at the page of a notion when its heading says `(p. N)`. A
`source.file` must be a clean relative path under `library/`; anything else is a
validation problem and the source is dropped. If the server moves off the PC (V2-01), the
same `file` and `sha256` can address an object store without changing the fiches.

`examples/knowledge/library/` is the one exception: it ships a 7-page demo PDF written
for this project, so the sample data shows a book whose link opens.

## Confirmation

`internal/library/library_test.go` (hash naming, idempotent add, `.gitignore` written
once, non-documents refused), `internal/kb/source_test.go` (paths outside `library/`
refused), `internal/httpapi/library_test.go` (availability flag, traversal blocked).
