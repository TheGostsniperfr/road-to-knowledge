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

Models, measured with option 3 on an AMD Ryzen 9 9900X, CPU only (`rtk bench search`):

| Model | Download | RAM | First index | Per search | Examples recall@5 / MRR | Brian's repo recall@5 / MRR | Off-topic noise |
|---|---|---|---|---|---|---|---|
| words only (FTS5) | 0 | 0 | 0 | ~1 ms | 0.10 / 0.10 | 0.08 / 0.08 | 0 |
| multilingual-e5-small, int8 (MIT) | 135 MB | ~560 MB | ~10 s | ~45 ms | 0.76 / 0.60 | 0.76 / 0.64 | 1.0 / 4.2 |
| EmbeddingGemma-300m, fp32 (Gemma terms) | 1.26 GB | ~2.6 GB | 19 to 36 s | ~370 ms | 0.95 / 0.85 | 0.84 / 0.70 | 0.2 / 2.0 |

Rows two and three use the hybrid search below with the floors in `internal/embed/models.go`.
"First index" is the first start on 23 (examples) and 65 (Brian's repo) chunks, mostly graph
compilation; later starts read the vector cache in about a second. "Off-topic noise" is the
average number of results for four questions the repo cannot answer (examples / Brian's repo).
The examples set is 21 questions in `internal/app/eval.go`, mostly paraphrases and the other
language; Brian's set is 37 questions on his real repo, kept outside git. EmbeddingGemma's
int8, int4 and fp16 exports do not run on the pure Go backend (unsupported per-tensor
DequantizeLinear, INT4, FLOAT16), so only its 1.2 GB fp32 export is usable there.

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
- Search fuses the FTS5 ranking and the vector ranking with reciprocal rank fusion (k = 60),
  so exact names ("Cilium") keep their place and paraphrases are added. Each hit says
  whether it was found by words, meaning or both.
- A per-model floor drops matches below a cosine score: small models give every text a high
  cosine, so without a floor an off-topic question returns the least bad subjects. The floors
  were chosen with `rtk bench search`, which also counts results for off-topic questions.
- `neighbors(id)` and the frontier use subject vectors (the mean of the subject's chunks).

Swapping the model or the runtime (ORT, a browser with Transformers.js, a phone) is a new
`Embedder`; the cache keys by model, so vectors from two models never mix.

## Confirmation

`internal/semantic` and `internal/app` tests use a fake embedder (cache reuse, fusion, modes,
neighbors, frontier, status). `internal/embed/eval_test.go` runs the real default model on
the examples when it is downloaded and is skipped otherwise, so CI stays offline.
`rtk bench search --data DIR [--queries FILE] [--model ID]` reproduces the table.
