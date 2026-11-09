package app

import (
	"context"
	"errors"
	"path/filepath"
	"slices"
	"sort"
	"sync"
	"time"

	"github.com/TheGostsniperfr/road-to-knowledge/internal/embed"
	"github.com/TheGostsniperfr/road-to-knowledge/internal/index"
	"github.com/TheGostsniperfr/road-to-knowledge/internal/kb"
	"github.com/TheGostsniperfr/road-to-knowledge/internal/semantic"
)

// VectorsDir holds the vector cache. Like the news cache it is rebuilt from the files, never committed.
const VectorsDir = ".rtk/vectors"

// Opener loads an embedder. It runs in the background so a slow or missing model never delays startup.
type Opener func(ctx context.Context) (embed.Embedder, semantic.Thresholds, error)

type semanticState struct {
	mu  sync.Mutex
	ctx context.Context
	emb embed.Embedder
	ix  *semantic.Index
	// stopped makes a model that finishes loading after StopSemantic release itself at once.
	stopped bool
	loaded  chan struct{}
	status  semantic.Status
	cancel  context.CancelFunc
	built   chan struct{}
}

// EnableSemantic turns on search by meaning. Until the model is loaded and the universe indexed,
// every search falls back to words only.
func (s *Service) EnableSemantic(ctx context.Context, open Opener) {
	s.sem.mu.Lock()
	s.sem.ctx, s.sem.stopped = ctx, false
	s.sem.status = semantic.Status{State: semantic.StateLoading}
	loaded := make(chan struct{})
	s.sem.loaded = loaded
	s.sem.mu.Unlock()
	go func() {
		defer close(loaded)
		emb, th, err := open(ctx)
		s.sem.mu.Lock()
		defer s.sem.mu.Unlock()
		switch {
		case errors.Is(err, embed.ErrMissing):
			s.sem.status = semantic.Status{State: semantic.StateMissing, Message: "rtk models pull"}
			return
		case err != nil:
			s.log.Warn("semantic search disabled", "err", err)
			s.sem.status = semantic.Status{State: semantic.StateError, Message: err.Error()}
			return
		}
		if ctx.Err() != nil || s.sem.stopped {
			closeEmbedder(emb)
			return
		}
		s.sem.emb = emb
		s.sem.ix = semantic.New(emb, th, filepath.Join(s.root, VectorsDir))
		s.reindexLocked()
	}()
}

// StopSemantic cancels the running build, waits for it, then releases the model: closing the
// model under a running build would pull it out from under the embedding batch.
func (s *Service) StopSemantic() {
	s.sem.mu.Lock()
	if s.sem.cancel != nil {
		s.sem.cancel()
	}
	loaded := s.sem.loaded
	s.sem.stopped = true
	s.sem.mu.Unlock()
	if loaded != nil {
		<-loaded
	}
	s.sem.mu.Lock()
	done, emb := s.sem.built, s.sem.emb
	s.sem.ix, s.sem.emb, s.sem.built, s.sem.loaded = nil, nil, nil, nil
	s.sem.status = semantic.Status{State: semantic.StateOff}
	s.sem.mu.Unlock()
	if done != nil {
		<-done
	}
	closeEmbedder(emb)
}

func closeEmbedder(emb embed.Embedder) {
	if c, ok := emb.(interface{ Close() error }); ok {
		c.Close()
	}
}

// reindex embeds what changed since the last build, in the background; a newer reload cancels it.
func (s *Service) reindex() {
	s.sem.mu.Lock()
	defer s.sem.mu.Unlock()
	s.reindexLocked()
}

func (s *Service) reindexLocked() {
	if s.sem.ix == nil {
		return
	}
	if s.sem.cancel != nil {
		s.sem.cancel()
	}
	ctx, cancel := context.WithCancel(s.sem.ctx)
	s.sem.cancel = cancel
	done := make(chan struct{})
	s.sem.built = done
	u, _, _, _ := s.snapshot()
	ix := s.sem.ix
	go func() {
		defer close(done)
		start := time.Now()
		n, err := ix.Build(ctx, semantic.Chunks(u))
		if err != nil {
			if !errors.Is(err, context.Canceled) && !errors.Is(err, semantic.ErrOutdated) {
				s.log.Warn("semantic index", "err", err)
			}
			return
		}
		s.log.Info("semantic index ready", "model", ix.Model(), "embedded", n, "took", time.Since(start).Round(time.Millisecond))
	}()
}

// WaitSemantic blocks until the current build is over; tests and the eval command use it.
func (s *Service) WaitSemantic(ctx context.Context) error {
	for {
		s.sem.mu.Lock()
		loading := s.sem.status.State == semantic.StateLoading && s.sem.ix == nil
		done := s.sem.built
		s.sem.mu.Unlock()
		if !loading && done == nil {
			return nil
		}
		if done == nil {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(20 * time.Millisecond):
			}
			continue
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-done:
			return nil
		}
	}
}

func (s *Service) SemanticStatus() semantic.Status {
	s.sem.mu.Lock()
	defer s.sem.mu.Unlock()
	if s.sem.ix != nil {
		return s.sem.ix.Status()
	}
	if s.sem.status.State == "" {
		return semantic.Status{State: semantic.StateOff}
	}
	return s.sem.status
}

func (s *Service) semanticIndex() *semantic.Index {
	s.sem.mu.Lock()
	defer s.sem.mu.Unlock()
	if s.sem.ix == nil || !s.sem.ix.Ready() {
		return nil
	}
	return s.sem.ix
}

const (
	ModeWords   = "words"
	ModeMeaning = "meaning"
	ModeBoth    = "both"
)

type SearchHit struct {
	index.Hit
	// Mode says how the subject was found, so an agent or the UI can tell a paraphrase from a word match.
	Mode string `json:"mode"`
}

// Strategy picks what a search uses; only the evaluation asks for anything but Hybrid.
type Strategy int

const (
	Hybrid Strategy = iota
	WordsOnly
	MeaningOnly
	// AllWordsOnly is the first word search (every word must match), kept as a baseline.
	AllWordsOnly
)

// rrfK is the usual constant of reciprocal rank fusion: it keeps one list's top hit from drowning the other.
const rrfK = 60

// Search mixes words (FTS5, best for names like "Cilium") and meaning (vectors, best for
// paraphrases and the other language) with reciprocal rank fusion. Every hit's score is on that
// one scale, with or without meaning: higher is better, comparable within one answer only.
func (s *Service) Search(ctx context.Context, q string, limit int) ([]SearchHit, error) {
	return s.SearchWith(ctx, q, limit, Hybrid)
}

func (s *Service) SearchWith(ctx context.Context, q string, limit int, how Strategy) ([]SearchHit, error) {
	u, idx, _, _ := s.snapshot()
	if limit <= 0 || limit > 50 {
		limit = 10
	}
	var words []index.Hit
	var err error
	switch how {
	case Hybrid, WordsOnly:
		words, err = idx.Search(ctx, q, limit*3)
	case AllWordsOnly:
		words, err = idx.SearchAllWords(ctx, q, limit*3)
	}
	if err != nil {
		return []SearchHit{}, err
	}
	var meaning []semantic.Hit
	if ix := s.semanticIndex(); ix != nil && (how == Hybrid || how == MeaningOnly) {
		if meaning, err = ix.Search(ctx, q, limit*3); err != nil {
			s.log.Warn("semantic search", "err", err)
		}
	}
	type fused struct {
		hit         SearchHit
		score       float64
		word, sense bool
	}
	all := map[string]*fused{}
	for rank, h := range words {
		all[h.ID] = &fused{hit: SearchHit{Hit: h}, score: 1 / float64(rrfK+rank+1), word: true}
	}
	for rank, h := range meaning {
		f := all[h.Subject]
		if f == nil {
			st := u.Star(h.Subject)
			if st == nil {
				continue
			}
			snippet := st.Summary
			if h.Label != "" {
				snippet = h.Label + " · " + snippet
			}
			f = &fused{hit: SearchHit{Hit: index.Hit{ID: st.ID, Name: st.Name, Snippet: clip(snippet, 140)}}}
			all[h.Subject] = f
		}
		f.score += 1 / float64(rrfK+rank+1)
		f.sense = true
	}
	out := make([]*fused, 0, len(all))
	for _, f := range all {
		switch {
		case f.word && f.sense:
			f.hit.Mode = ModeBoth
		case f.word:
			f.hit.Mode = ModeWords
		default:
			f.hit.Mode = ModeMeaning
		}
		f.hit.Score = f.score
		out = append(out, f)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].score != out[j].score {
			return out[i].score > out[j].score
		}
		return out[i].hit.ID < out[j].hit.ID
	})
	hits := make([]SearchHit, 0, min(limit, len(out)))
	for _, f := range out[:min(limit, len(out))] {
		hits = append(hits, f.hit)
	}
	return hits, nil
}

func clip(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}

type NeighborView struct {
	ID    string   `json:"id"`
	Name  string   `json:"name"`
	Path  string   `json:"path"`
	Level int      `json:"level"`
	State kb.State `json:"state"`
	Score float32  `json:"score"`
}

var ErrSemanticOff = errors.New("search by meaning is not ready: see the status, or run `rtk models pull`")

// Neighbors lists the subjects closest in meaning to id that are not linked to it yet: the links
// you could add, or what to read next.
func (s *Service) Neighbors(id string, limit int) ([]NeighborView, error) {
	u, _, _, _ := s.snapshot()
	id = s.live(u, id)
	st := u.Star(id)
	if st == nil {
		return nil, ErrNotFound
	}
	ix := s.semanticIndex()
	if ix == nil {
		return nil, ErrSemanticOff
	}
	if limit <= 0 || limit > 30 {
		limit = 8
	}
	linked := func(other string) bool {
		if slices.Contains(st.Links, other) {
			return true
		}
		o := u.Star(other)
		return o != nil && slices.Contains(o.Links, st.ID)
	}
	out := []NeighborView{}
	for _, h := range ix.Neighbors(id, limit, linked) {
		o := u.Star(h.Subject)
		if o == nil {
			continue
		}
		out = append(out, NeighborView{ID: o.ID, Name: o.Name, Path: o.Cluster.Galaxy.Name + " > " + o.Cluster.Name, Level: o.Level, State: o.State, Score: h.Score})
	}
	return out, nil
}

// similar matches a candidate with the known subject closest in meaning, or returns nil while the
// index is not ready so the frontier keeps working on links alone.
func (s *Service) similar(u *kb.Universe) Similar {
	ix := s.semanticIndex()
	if ix == nil {
		return nil
	}
	vecs := ix.SubjectVectors()
	near := ix.Thresholds().Near
	var known []*kb.Star
	for _, st := range u.Stars() {
		if isKnown(st) && vecs[st.ID] != nil {
			known = append(known, st)
		}
	}
	return func(c *kb.Star) (*kb.Star, float32, bool) {
		v := vecs[c.ID]
		if v == nil {
			return nil, 0, false
		}
		var best *kb.Star
		var bestScore float32
		for _, k := range known {
			if sc := embed.Dot(v, vecs[k.ID]); sc >= near && (best == nil || sc > bestScore || (sc == bestScore && k.ID < best.ID)) {
				best, bestScore = k, sc
			}
		}
		return best, bestScore, best != nil
	}
}
