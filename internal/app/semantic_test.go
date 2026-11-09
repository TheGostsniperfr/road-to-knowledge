package app_test

import (
	"context"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/TheGostsniperfr/road-to-knowledge/internal/app"
	"github.com/TheGostsniperfr/road-to-knowledge/internal/embed"
	"github.com/TheGostsniperfr/road-to-knowledge/internal/semantic"
)

// The fake embedder maps these French words onto the English ones, standing in for a real model
// that knows "isoler" and "isolates" mean the same thing.
var synonyms = map[string]string{"isoler": "isolates", "conteneurs": "containers", "noyau": "kernel", "proche": "close"}

func semanticRepo(t *testing.T) (*app.Service, string) {
	t.Helper()
	root := t.TempDir()
	files := map[string]string{
		"galaxies/tech/securite/gvisor.md":     "---\nname: gVisor\nlevel: 0\nstate: explore\n---\nSandbox that isolates containers from the host kernel.\n",
		"galaxies/tech/k8s/runtimes.md":        "---\nname: Container runtimes\nlevel: 4\n---\nHow containers reach the kernel: CRI, containerd, runc.\n",
		"galaxies/tech/k8s/argo.md":            "---\nname: Argo CD\nlevel: 4\nlinks: [helm]\n---\nGitOps delivery from git.\n",
		"galaxies/tech/k8s/helm.md":            "---\nname: Helm\nlevel: 2\n---\nTemplates for Kubernetes manifests.\n",
		"galaxies/cuisine/patisserie/tarte.md": "---\nname: Tarte\nlevel: 1\n---\nPommes et pâte brisée.\n",
	}
	for rel, content := range files {
		p := filepath.Join(root, rel)
		require.NoError(t, os.MkdirAll(filepath.Dir(p), 0o755))
		require.NoError(t, os.WriteFile(p, []byte(content), 0o644))
	}
	svc, err := app.New(context.Background(), root, slog.New(slog.NewTextHandler(io.Discard, nil)))
	require.NoError(t, err)
	return svc, root
}

func enable(t *testing.T, svc *app.Service, th semantic.Thresholds) *embed.Fake {
	t.Helper()
	fake := &embed.Fake{Synonyms: synonyms}
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	svc.EnableSemantic(ctx, func(context.Context) (embed.Embedder, semantic.Thresholds, error) { return fake, th, nil })
	wait, done := context.WithTimeout(ctx, 5*time.Second)
	defer done()
	require.NoError(t, svc.WaitSemantic(wait))
	require.Equal(t, semantic.StateReady, svc.SemanticStatus().State)
	return fake
}

func TestSearch_UsesWordsOnly_WhenSemanticSearchIsOff(t *testing.T) {
	svc, _ := semanticRepo(t)

	hits, err := svc.Search(context.Background(), "isoler conteneurs", 5)

	require.NoError(t, err)
	assert.Empty(t, hits)
	assert.Equal(t, semantic.StateOff, svc.SemanticStatus().State)
}

func TestSearch_FindsAParaphraseByMeaning_WhenNoWordMatches(t *testing.T) {
	svc, _ := semanticRepo(t)
	enable(t, svc, semantic.Thresholds{Search: 0.3})

	hits, err := svc.Search(context.Background(), "isoler conteneurs", 5)

	require.NoError(t, err)
	require.NotEmpty(t, hits)
	assert.Equal(t, "tech/securite/gvisor", hits[0].ID)
	assert.Equal(t, app.ModeMeaning, hits[0].Mode)
}

func TestSearch_MarksAHitFoundBothWays(t *testing.T) {
	svc, _ := semanticRepo(t)
	enable(t, svc, semantic.Thresholds{Search: 0.3})

	hits, err := svc.Search(context.Background(), "gVisor sandbox", 5)

	require.NoError(t, err)
	require.NotEmpty(t, hits)
	assert.Equal(t, "tech/securite/gvisor", hits[0].ID)
	assert.Equal(t, app.ModeBoth, hits[0].Mode)
}

func TestSearch_ReturnsNothingByMeaning_BelowTheModelsNoiseFloor(t *testing.T) {
	svc, _ := semanticRepo(t)
	enable(t, svc, semantic.Thresholds{Search: 0.99})

	hits, err := svc.SearchWith(context.Background(), "isoler conteneurs", 5, app.MeaningOnly)

	require.NoError(t, err)
	assert.Empty(t, hits)
}

func TestReload_EmbedsOnlyTheSubjectThatChanged(t *testing.T) {
	svc, root := semanticRepo(t)
	fake := enable(t, svc, semantic.Thresholds{Search: 0.3})
	before := fake.Texts.Load()
	require.NoError(t, os.WriteFile(filepath.Join(root, "galaxies/cuisine/patisserie/tarte.md"), []byte("---\nname: Tarte\nlevel: 1\n---\nPommes, cannelle.\n"), 0o644))

	require.NoError(t, svc.Reload(context.Background()))
	require.NoError(t, svc.WaitSemantic(context.Background()))

	assert.Equal(t, before+1, fake.Texts.Load())
}

func TestNeighbors_LeaveOutSubjectsAlreadyLinked(t *testing.T) {
	svc, _ := semanticRepo(t)
	enable(t, svc, semantic.Thresholds{Search: 0.3})

	n, err := svc.Neighbors("tech/k8s/argo", 10)

	require.NoError(t, err)
	ids := make([]string, len(n))
	for i, x := range n {
		ids[i] = x.ID
	}
	assert.NotContains(t, ids, "tech/k8s/helm")
	assert.NotContains(t, ids, "tech/k8s/argo")
}

func TestNeighbors_FailsClearly_WhenSemanticSearchIsOff(t *testing.T) {
	svc, _ := semanticRepo(t)

	_, err := svc.Neighbors("tech/k8s/argo", 5)

	assert.ErrorIs(t, err, app.ErrSemanticOff)
}

func TestSemanticStatus_SaysTheModelIsMissing_WhenNotDownloaded(t *testing.T) {
	svc, _ := semanticRepo(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	svc.EnableSemantic(ctx, func(context.Context) (embed.Embedder, semantic.Thresholds, error) {
		return nil, semantic.Thresholds{}, embed.ErrMissing
	})

	require.NoError(t, svc.WaitSemantic(ctx))

	assert.Equal(t, semantic.StateMissing, svc.SemanticStatus().State)
}

func TestFrontier_PlacesASubjectCloseInMeaningToAKnownOne_TwoStepsAway(t *testing.T) {
	svc, _ := semanticRepo(t)
	enable(t, svc, semantic.Thresholds{Search: 0.3, Near: 0.3})

	f, err := svc.Frontier("", 10)

	require.NoError(t, err)
	var reason string
	for _, s := range f.Suggestions {
		if s.ID == "tech/securite/gvisor" {
			reason = s.Reason
			assert.Equal(t, app.BucketTwoHops, s.Bucket)
		}
	}
	assert.Equal(t, "proche de Container runtimes par le sens", reason)
}

// slowCloser embeds slowly and records any use after Close, like a model freed under a batch.
type slowCloser struct {
	embed.Fake
	closed, usedAfterClose atomic.Bool
}

func (c *slowCloser) Embed(ctx context.Context, texts []string, kind embed.Kind) ([][]float32, error) {
	time.Sleep(20 * time.Millisecond)
	if c.closed.Load() {
		c.usedAfterClose.Store(true)
	}
	return c.Fake.Embed(ctx, texts, kind)
}

func (c *slowCloser) Close() error { c.closed.Store(true); return nil }

func TestStopSemantic_ClosesTheModelOnlyAfterTheBuildHasStopped(t *testing.T) {
	svc, _ := semanticRepo(t)
	model := &slowCloser{}
	loaded := make(chan struct{})
	svc.EnableSemantic(context.Background(), func(context.Context) (embed.Embedder, semantic.Thresholds, error) {
		defer close(loaded)
		return model, semantic.Thresholds{}, nil
	})
	<-loaded
	time.Sleep(5 * time.Millisecond)

	svc.StopSemantic()

	assert.True(t, model.closed.Load())
	assert.False(t, model.usedAfterClose.Load())
	assert.Equal(t, semantic.StateOff, svc.SemanticStatus().State)
}
