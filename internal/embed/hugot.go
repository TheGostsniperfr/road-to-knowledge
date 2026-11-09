package embed

import (
	"context"
	"fmt"
	"sync"

	"github.com/knights-analytics/hugot"
	"github.com/knights-analytics/hugot/pipelines"
)

// maxChars keeps inputs well inside the models' token window; longer notions are cut, not split.
const maxChars = 1500

const batchSize = 8

// Hugot runs a model with hugot's pure Go backend: no cgo, no Python, no separate server.
type Hugot struct {
	spec     Spec
	mu       sync.Mutex
	session  *hugot.Session
	pipeline *pipelines.FeatureExtractionPipeline
}

func Open(ctx context.Context, id string) (*Hugot, error) {
	spec, err := Lookup(id)
	if err != nil {
		return nil, err
	}
	if err := spec.Present(); err != nil {
		return nil, err
	}
	dir, err := spec.Dir()
	if err != nil {
		return nil, err
	}
	session, err := hugot.NewGoSession(ctx)
	if err != nil {
		return nil, fmt.Errorf("embed: session: %w", err)
	}
	opts := []hugot.FeatureExtractionOption{pipelines.WithNormalization()}
	if spec.Output != "" {
		opts = append(opts, pipelines.WithOutputName(spec.Output))
	}
	p, err := hugot.NewPipeline(session, hugot.FeatureExtractionConfig{ModelPath: dir, Name: spec.ID, OnnxFilename: spec.Onnx, Options: opts})
	if err != nil {
		session.Destroy()
		return nil, fmt.Errorf("embed: load %s: %w", spec.ID, err)
	}
	return &Hugot{spec: spec, session: session, pipeline: p}, nil
}

func (h *Hugot) Model() string { return h.spec.ID }
func (h *Hugot) Dims() int     { return h.spec.Dims }
func (h *Hugot) Spec() Spec    { return h.spec }

func (h *Hugot) Close() error {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.session.Destroy()
}

func (h *Hugot) Embed(ctx context.Context, texts []string, kind Kind) ([][]float32, error) {
	prefix := h.spec.DocPrefix
	if kind == Query {
		prefix = h.spec.QueryPrefix
	}
	out := make([][]float32, 0, len(texts))
	for start := 0; start < len(texts); start += batchSize {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		batch := texts[start:min(start+batchSize, len(texts))]
		in := make([]string, len(batch))
		for i, t := range batch {
			if r := []rune(t); len(r) > maxChars {
				t = string(r[:maxChars])
			}
			in[i] = prefix + t
		}
		h.mu.Lock()
		res, err := h.pipeline.RunPipeline(ctx, in)
		h.mu.Unlock()
		if err != nil {
			return nil, fmt.Errorf("embed: %s: %w", h.spec.ID, err)
		}
		for _, v := range res.Embeddings {
			if h.spec.Truncate > 0 && len(v) > h.spec.Truncate {
				v = v[:h.spec.Truncate]
			}
			out = append(out, Normalize(v))
		}
	}
	return out, nil
}
