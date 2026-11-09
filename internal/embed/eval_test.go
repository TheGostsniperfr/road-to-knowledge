package embed_test

import (
	"context"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/TheGostsniperfr/road-to-knowledge/internal/app"
	"github.com/TheGostsniperfr/road-to-knowledge/internal/embed"
	"github.com/TheGostsniperfr/road-to-knowledge/internal/semantic"
)

// TestEval_HybridSearchBeatsWordsOnTheExamples runs the real default model. It is skipped when the
// model is not downloaded, so CI stays offline; run `rtk models pull` to include it locally.
func TestEval_HybridSearchBeatsWordsOnTheExamples(t *testing.T) {
	spec := embed.Models[embed.Default]
	if err := spec.Present(); err != nil {
		t.Skip("model not downloaded:", err)
	}
	root := t.TempDir()
	require.NoError(t, os.CopyFS(root, os.DirFS("../../examples/knowledge")))
	svc, err := app.New(context.Background(), root, slog.New(slog.NewTextHandler(io.Discard, nil)))
	require.NoError(t, err)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	svc.EnableSemantic(ctx, func(ctx context.Context) (embed.Embedder, semantic.Thresholds, error) {
		h, err := embed.Open(ctx, embed.Default)
		if err != nil {
			return nil, semantic.Thresholds{}, err
		}
		t.Cleanup(func() { h.Close() })
		return h, semantic.Thresholds{Search: spec.MinScore, Near: spec.NearScore}, nil
	})
	require.NoError(t, svc.WaitSemantic(ctx))

	r, err := svc.EvalSearch(ctx, app.ExampleQueries)

	require.NoError(t, err)
	t.Logf("words %.2f/%.2f  meaning %.2f/%.2f  hybrid %.2f/%.2f  noise %.1f", r.Words.RecallAt5, r.Words.MRR, r.Meaning.RecallAt5, r.Meaning.MRR, r.Hybrid.RecallAt5, r.Hybrid.MRR, r.Noise)
	assert.Greater(t, r.Hybrid.RecallAt5, r.Words.RecallAt5)
	assert.GreaterOrEqual(t, r.Hybrid.RecallAt5, 0.6)
	_, err = os.Stat(filepath.Join(root, app.VectorsDir, embed.Default+".bin"))
	assert.NoError(t, err, "the vectors are cached")
}
