package app_test

import (
	"context"
	"io"
	"log/slog"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/TheGostsniperfr/road-to-knowledge/internal/app"
	"github.com/TheGostsniperfr/road-to-knowledge/internal/embed"
	"github.com/TheGostsniperfr/road-to-knowledge/internal/semantic"
)

func TestReadEvalQueries_DefaultsToATuningParaphrase(t *testing.T) {
	in := "# comment\nisoler\ta/b/c\nCilium\tx/y/z,u/v/w\tname\theldout\n"

	qs, err := app.ReadEvalQueries(strings.NewReader(in))

	require.NoError(t, err)
	assert.Equal(t, []app.EvalQuery{
		{Q: "isoler", Want: []string{"a/b/c"}, Kind: app.KindParaphrase, Set: app.SetTune},
		{Q: "Cilium", Want: []string{"x/y/z", "u/v/w"}, Kind: app.KindName, Set: app.SetHeldOut},
	}, qs)
}

func TestReadEvalQueries_RejectsAnUnknownSet(t *testing.T) {
	_, err := app.ReadEvalQueries(strings.NewReader("q\ta/b/c\tname\ttest\n"))

	assert.ErrorIs(t, err, app.ErrInvalid)
}

// TestEval_HybridNeverDoesWorseThanWords_AndFindsMoreParaphrases runs the real default model. It is skipped when the
// model is not downloaded, so CI stays offline; run `rtk models pull` to include it locally.
func TestEval_HybridNeverDoesWorseThanWords_AndFindsMoreParaphrases(t *testing.T) {
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
		return h, semantic.Thresholds{Search: spec.MinScore, Near: spec.NearScore}, nil
	})
	t.Cleanup(svc.StopSemantic)
	require.NoError(t, svc.WaitSemantic(ctx))

	r, err := svc.EvalSearch(ctx, app.ExampleQueries)

	require.NoError(t, err)
	for _, set := range []string{app.SetTune, app.SetHeldOut} {
		for _, kind := range []string{app.KindParaphrase, app.KindName} {
			words, _ := r.Row("words", set, kind)
			hybrid, _ := r.Row("hybrid", set, kind)
			t.Logf("%s %s recall@5: words %.2f, hybrid %.2f", set, kind, words.RecallAt5, hybrid.RecallAt5)
			assert.GreaterOrEqual(t, hybrid.RecallAt5, words.RecallAt5, "%s %s", set, kind)
		}
	}
	words, _ := r.Row("words", app.SetTune, app.KindParaphrase)
	hybrid, _ := r.Row("hybrid", app.SetTune, app.KindParaphrase)
	assert.Greater(t, hybrid.RecallAt5, words.RecallAt5)
}
