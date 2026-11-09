// Package semantic is the search by meaning: subjects and notions as vectors, kept in memory and
// cached on disk by content hash so a reload only embeds what changed (ADR-0013).
package semantic

import (
	"context"
	"crypto/sha256"
	"encoding/gob"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/TheGostsniperfr/road-to-knowledge/internal/embed"
	"github.com/TheGostsniperfr/road-to-knowledge/internal/kb"
)

// Chunk is what gets a vector: a subject's summary, or one of its notions.
type Chunk struct {
	ID      string // subject id, or the notion id (subject#notion)
	Subject string
	Label   string // the notion's name, empty for the subject itself
	Text    string
	Hash    string
}

func Chunks(u *kb.Universe) []Chunk {
	var out []Chunk
	for _, s := range u.Stars() {
		path := s.Cluster.Galaxy.Name + " > " + s.Cluster.Name
		head := s.Name + " (" + path + ")"
		summary := s.Summary
		if len(s.Tags) > 0 {
			summary += " " + strings.Join(s.Tags, ", ")
		}
		out = append(out, chunk(s.ID, s.ID, "", head+". "+summary))
		for _, p := range s.Planets {
			out = append(out, chunk(p.ID, s.ID, p.Name, s.Name+" > "+p.Name+". "+strings.TrimSpace(p.Body)))
		}
	}
	return out
}

func chunk(id, subject, label, text string) Chunk {
	text = strings.Join(strings.Fields(text), " ")
	sum := sha256.Sum256([]byte(text))
	return Chunk{ID: id, Subject: subject, Label: label, Text: text, Hash: hex.EncodeToString(sum[:12])}
}

type State string

const (
	StateOff      State = "off"
	StateMissing  State = "missing"
	StateLoading  State = "loading"
	StateIndexing State = "indexing"
	StateReady    State = "ready"
	StateError    State = "error"
)

type Status struct {
	State   State  `json:"state"`
	Model   string `json:"model,omitempty"`
	Done    int    `json:"done"`
	Total   int    `json:"total"`
	Message string `json:"message,omitempty"`
}

type entry struct {
	chunk Chunk
	vec   []float32
}

// Index holds the vectors of the current universe. It is safe for concurrent use; a rebuild swaps
// the whole set at once, so a search sees either the old or the new universe, never a mix.
// Thresholds are per model: each model spreads its cosines differently.
type Thresholds struct {
	// Search drops query matches below it.
	Search float32
	// Near is the subject-to-subject similarity above which two subjects talk about the same thing.
	Near float32
}

type Index struct {
	emb       embed.Embedder
	th        Thresholds
	cachePath string

	mu       sync.RWMutex
	entries  []entry
	subjects map[string][]float32
	status   Status
}

func New(emb embed.Embedder, th Thresholds, cacheDir string) *Index {
	return &Index{
		emb: emb, th: th,
		cachePath: filepath.Join(cacheDir, emb.Model()+".bin"),
		status:    Status{State: StateIndexing, Model: emb.Model()},
	}
}

func (ix *Index) Status() Status {
	ix.mu.RLock()
	defer ix.mu.RUnlock()
	return ix.status
}

func (ix *Index) Model() string { return ix.emb.Model() }

func (ix *Index) Thresholds() Thresholds { return ix.th }

func (ix *Index) setStatus(f func(*Status)) {
	ix.mu.Lock()
	f(&ix.status)
	ix.mu.Unlock()
}

// cacheFile maps content hashes to vectors for one model; the model id is in the file name.
type cacheFile struct {
	Dims    int
	Vectors map[string][]float32
}

func (ix *Index) loadCache() map[string][]float32 {
	f, err := os.Open(ix.cachePath)
	if err != nil {
		return map[string][]float32{}
	}
	defer f.Close()
	var c cacheFile
	if err := gob.NewDecoder(f).Decode(&c); err != nil || c.Dims != ix.emb.Dims() || c.Vectors == nil {
		return map[string][]float32{}
	}
	return c.Vectors
}

func (ix *Index) saveCache(vectors map[string][]float32) error {
	if err := os.MkdirAll(filepath.Dir(ix.cachePath), 0o755); err != nil {
		return fmt.Errorf("semantic: %w", err)
	}
	tmp, err := os.CreateTemp(filepath.Dir(ix.cachePath), ".vectors-*")
	if err != nil {
		return fmt.Errorf("semantic: %w", err)
	}
	defer os.Remove(tmp.Name())
	if err := gob.NewEncoder(tmp).Encode(cacheFile{Dims: ix.emb.Dims(), Vectors: vectors}); err != nil {
		tmp.Close()
		return fmt.Errorf("semantic: write cache: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("semantic: %w", err)
	}
	return os.Rename(tmp.Name(), ix.cachePath)
}

// Build embeds the chunks that are not cached yet, then swaps them in. It returns how many chunks
// it had to embed. Cancelling ctx leaves the previous set in place.
func (ix *Index) Build(ctx context.Context, chunks []Chunk) (int, error) {
	cache := ix.loadCache()
	var todo []Chunk
	seen := map[string]bool{}
	for _, c := range chunks {
		if _, ok := cache[c.Hash]; !ok && !seen[c.Hash] {
			todo = append(todo, c)
			seen[c.Hash] = true
		}
	}
	ix.setStatus(func(s *Status) {
		s.State, s.Done, s.Total, s.Message = StateIndexing, len(chunks)-len(todo), len(chunks), ""
	})
	const step = 32
	for start := 0; start < len(todo); start += step {
		batch := todo[start:min(start+step, len(todo))]
		texts := make([]string, len(batch))
		for i, c := range batch {
			texts[i] = c.Text
		}
		vecs, err := ix.emb.Embed(ctx, texts, embed.Document)
		if err != nil {
			if errors.Is(err, context.Canceled) {
				ix.mu.Lock()
				if ix.entries != nil {
					ix.status = Status{State: StateReady, Model: ix.emb.Model(), Done: len(ix.entries), Total: len(ix.entries)}
				}
				ix.mu.Unlock()
				return 0, err
			}
			ix.setStatus(func(s *Status) { s.State, s.Message = StateError, err.Error() })
			return 0, err
		}
		for i, c := range batch {
			cache[c.Hash] = vecs[i]
		}
		done := start + len(batch)
		ix.setStatus(func(s *Status) { s.Done = len(chunks) - len(todo) + done })
	}
	entries := make([]entry, 0, len(chunks))
	keep := make(map[string][]float32, len(chunks))
	for _, c := range chunks {
		entries = append(entries, entry{chunk: c, vec: cache[c.Hash]})
		keep[c.Hash] = cache[c.Hash]
	}
	subjects := averages(entries)
	ix.mu.Lock()
	ix.entries, ix.subjects = entries, subjects
	ix.status = Status{State: StateReady, Model: ix.emb.Model(), Done: len(chunks), Total: len(chunks)}
	ix.mu.Unlock()
	if len(todo) > 0 || len(keep) != len(cache) {
		if err := ix.saveCache(keep); err != nil {
			return len(todo), err
		}
	}
	return len(todo), nil
}

// Ready reports whether searches use vectors; before the first build callers fall back to words
// only. A rebuild in progress keeps serving the previous vectors.
func (ix *Index) Ready() bool {
	ix.mu.RLock()
	defer ix.mu.RUnlock()
	return ix.entries != nil
}

type Hit struct {
	Subject string
	Label   string // best notion, empty when the summary matched best
	Score   float32
}

// Search returns subjects by meaning, best chunk per subject, above the model's noise floor.
func (ix *Index) Search(ctx context.Context, query string, limit int) ([]Hit, error) {
	if !ix.Ready() || strings.TrimSpace(query) == "" {
		return nil, nil
	}
	v, err := ix.emb.Embed(ctx, []string{query}, embed.Query)
	if err != nil {
		return nil, err
	}
	ix.mu.RLock()
	best := map[string]Hit{}
	for _, e := range ix.entries {
		sc := embed.Dot(v[0], e.vec)
		if sc < ix.th.Search {
			continue
		}
		if h, ok := best[e.chunk.Subject]; !ok || sc > h.Score {
			best[e.chunk.Subject] = Hit{Subject: e.chunk.Subject, Label: e.chunk.Label, Score: sc}
		}
	}
	ix.mu.RUnlock()
	return top(best, limit), nil
}

func top(m map[string]Hit, limit int) []Hit {
	out := make([]Hit, 0, len(m))
	for _, h := range m {
		out = append(out, h)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Score != out[j].Score {
			return out[i].Score > out[j].Score
		}
		return out[i].Subject < out[j].Subject
	})
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out
}

// averages gives each subject the mean of its chunks: a subject is close to another when their
// summaries and notions talk about the same things overall, not when one sentence happens to match.
func averages(entries []entry) map[string][]float32 {
	sums := map[string][]float32{}
	for _, e := range entries {
		s := sums[e.chunk.Subject]
		if s == nil {
			s = make([]float32, len(e.vec))
			sums[e.chunk.Subject] = s
		}
		for i, x := range e.vec {
			s[i] += x
		}
	}
	for _, s := range sums {
		embed.Normalize(s)
	}
	return sums
}

// Neighbors returns the subjects closest in meaning to id, skipping those for which skip is true.
func (ix *Index) Neighbors(id string, limit int, skip func(string) bool) []Hit {
	if !ix.Ready() {
		return nil
	}
	vecs := ix.SubjectVectors()
	me, ok := vecs[id]
	if !ok {
		return nil
	}
	best := map[string]Hit{}
	for other, v := range vecs {
		if other == id || (skip != nil && skip(other)) {
			continue
		}
		best[other] = Hit{Subject: other, Score: embed.Dot(me, v)}
	}
	return top(best, limit)
}

// SubjectVectors is the averaged vector of every subject. The map is replaced, never mutated, on
// rebuild, so callers may read it without the lock.
func (ix *Index) SubjectVectors() map[string][]float32 {
	if !ix.Ready() {
		return nil
	}
	ix.mu.RLock()
	defer ix.mu.RUnlock()
	return ix.subjects
}
