# ADR-0013: Search by meaning with a small model inside rtk

- Status: accepted, 2026-11-09

## Context

Search (Ctrl K, MCP `search`) matched words only: "isoler des conteneurs" did not find gVisor,
an English question did not find a French fiche, and `frontier` had few "two hops" because
few subjects carry `links:`. Brian wants search by meaning without a heavy stack: no Ollama
service to install and keep running, CPU is fine, nothing leaves the PC, and the same model
should later run on a phone or in a browser.

## Options

1. Ollama with an embedding model. Easy, but a separate service of several hundred MB to
   install and run next to `rtk`.
2. ONNX Runtime from Go (hugot ORT backend). Fast and runs int8/int4 models, but needs the
   C library and cgo: a heavier build and a non-static binary.
3. hugot's pure Go backend (GoMLX): the model is a file and `rtk` needs no cgo (it builds with `CGO_ENABLED=0`).
   Slower than ORT and supports fewer ONNX operators.

Measured with option 3 on an AMD Ryzen 9 9900X, CPU only, with `rtk bench search`:

| | Download | RAM | First index | Per search |
|---|---|---|---|---|
| multilingual-e5-small, int8 (MIT) | 135 MB | ~560 MB | 7 to 9 s | ~65 ms |
| EmbeddingGemma-300m, fp32 (Gemma terms) | 1.26 GB | ~2.6 GB | ~25 s | ~530 ms |

"First index" is the first start on 23 (examples) or 65 (Brian's repo) chunks, mostly graph
compilation; later starts read the vector cache in about a second. EmbeddingGemma's int8,
int4 and fp16 exports do not run on the pure Go backend (unsupported per-tensor
DequantizeLinear, INT4, FLOAT16), so only its 1.2 GB fp32 export is usable there.

Quality, recall@5 on **held-out** questions (not used to choose any threshold), paraphrases
and exact names apart:

| Held-out recall@5 | Examples: paraphrase / name (6 + 6) | Brian's repo: paraphrase / name (7 + 7) |
|---|---|---|
| all words (first version: every word must match) | 0.00 / 1.00 | 0.14 / 1.00 |
| words (stopwords dropped, any word as fallback) | 0.83 / 1.00 | 0.57 / 1.00 |
| hybrid, e5-small | 0.83 / 1.00 | 0.71 / 1.00 |
| hybrid, EmbeddingGemma | 1.00 / 1.00 | 0.86 / 1.00 |

Off-topic questions (four the repo cannot answer) return on average 1.0 / 4.2 results with
e5-small and 0.2 / 2.0 with EmbeddingGemma (examples / Brian's repo).

How the numbers were made: each set is split into a **tuning** part, used to choose the
per-model floors below, and a **held-out** part, written afterwards and only measured. The
examples sets are in `internal/app/eval.go` (26 tuning, 12 held-out); Brian's are 46 + 14
questions on his real repo, kept outside git. The sets are small, so a difference of one
question moves a held-out score by 0.07 to 0.17: read them as a direction, not a ranking.
Exact names are found by words alone already; meaning pays on paraphrases and the other
language. Fixing the word search (stopwords, any-word fallback) closed much of the gap on
its own, which is why it is the baseline here.

## Decision

Option 3. `internal/embed` defines an `Embedder` interface; the first implementation runs
hugot's pure Go backend. Default model: multilingual-e5-small, for its size; EmbeddingGemma is
one flag away (`--model embeddinggemma-300m`) for better quality at ten times the memory.

- `rtk models pull` downloads pinned files (revision and sha256 in code) into the user cache,
  never into git. Without the model, search is words only and never fails.
- Each subject summary and each notion is a chunk; its vector is cached in
  `.rtk/vectors/<model>.bin` under a hash of the text, so a reload embeds only what changed.
  Indexing runs in the background after start and after every reload; until the first build
  ends, search is words only.
- Word search drops French and English stopwords, requires every remaining word first and
  falls back to any word when that finds too little.
- Search fuses the FTS5 ranking and the vector ranking with reciprocal rank fusion (k = 60),
  so exact names ("Cilium") keep their place and paraphrases are added. Each hit says
  whether it was found by words, meaning or both; its score is the fused one, with or
  without a model, so API results have one scale.
- A per-model floor drops matches below a cosine score: small models give every text a high
  cosine, so without a floor an off-topic question returns the least bad subjects. The floors
  were chosen on the tuning questions only. A gate on how far the best match stands out from
  the rest (z-score) was tried and dropped: it did not separate real questions from
  off-topic ones for either model.
- Only `rtk serve` loads a model by default. Each Claude Code session starts its own
  `rtk mcp` over stdio, so that one is words only unless `--model` is given; agents get
  meaning through the HTTP transport of `rtk serve`, one model for every session.
- Searches go ahead of indexing: the build embeds one text at a time and yields to waiting
  searches, so a search during a rebuild waits for one text at most. Measured with
  EmbeddingGemma: 0.55 s idle, at most 1.0 s during a full rebuild (2.5 to 3.6 s with batches
  of eight).
- A build that a newer one overtook never swaps its vectors in or writes the cache.
- `neighbors(id)` and the frontier use subject vectors (the mean of the subject's chunks).

Swapping the model or the runtime (ORT, a browser with Transformers.js, a phone) is a new
`Embedder`; the cache file is named by model and by a fingerprint of how it embeds (revision,
prefixes, dimensions), so vectors made two ways never mix.

## Confirmation

`internal/semantic` and `internal/app` tests use a fake embedder (cache reuse, an outdated build
finishing last, fusion, modes, neighbors, frontier, status, shutdown order).
`internal/app/eval_test.go` runs the real default model on the examples when it is downloaded
and is skipped otherwise, so CI stays offline.
`rtk bench search --data DIR [--queries FILE] [--model ID]` reproduces the table.
