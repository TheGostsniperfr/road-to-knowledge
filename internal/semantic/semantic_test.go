package semantic_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/TheGostsniperfr/road-to-knowledge/internal/embed"
	"github.com/TheGostsniperfr/road-to-knowledge/internal/kb"
	"github.com/TheGostsniperfr/road-to-knowledge/internal/semantic"
)

func write(t *testing.T, root, rel, content string) {
	t.Helper()
	p := filepath.Join(root, rel)
	require.NoError(t, os.MkdirAll(filepath.Dir(p), 0o755))
	require.NoError(t, os.WriteFile(p, []byte(content), 0o644))
}

func universe(t *testing.T) (string, *kb.Universe) {
	t.Helper()
	root := t.TempDir()
	write(t, root, "galaxies/tech/k8s/gvisor.md", "---\nname: gVisor\nlevel: 0\n---\nSandbox runtime that isolates containers from the host kernel.\n\n## Sentry\n\nA user-space kernel intercepting syscalls.\n")
	write(t, root, "galaxies/tech/k8s/argo.md", "---\nname: Argo CD\nlevel: 4\n---\nGitOps delivery: sync applications from git.\n")
	write(t, root, "galaxies/cuisine/patisserie/tarte.md", "---\nname: Tarte aux pommes\nlevel: 2\n---\nRecette de tarte avec pommes et pâte brisée.\n")
	u, problems, err := kb.Load(root)
	require.NoError(t, err)
	require.Empty(t, problems)
	return root, u
}

func newIndex(t *testing.T, emb embed.Embedder) (*semantic.Index, string) {
	t.Helper()
	dir := t.TempDir()
	return semantic.New(emb, semantic.Thresholds{Search: 0.1, Near: 0.5}, dir), dir
}

func TestChunks_GiveOneChunkPerSubjectAndPerNotion(t *testing.T) {
	_, u := universe(t)

	chunks := semantic.Chunks(u)

	ids := make([]string, len(chunks))
	for i, c := range chunks {
		ids[i] = c.ID
	}
	assert.ElementsMatch(t, []string{"tech/k8s/gvisor", "tech/k8s/gvisor#sentry", "tech/k8s/argo", "cuisine/patisserie/tarte"}, ids)
}

func TestSearch_FindsSubjectsByMeaningThroughSynonyms(t *testing.T) {
	_, u := universe(t)
	ix, _ := newIndex(t, &embed.Fake{Synonyms: map[string]string{"isoler": "isolates", "conteneurs": "containers"}})
	_, err := ix.Build(context.Background(), semantic.Chunks(u))
	require.NoError(t, err)

	hits, err := ix.Search(context.Background(), "isoler conteneurs", 3)

	require.NoError(t, err)
	require.NotEmpty(t, hits)
	assert.Equal(t, "tech/k8s/gvisor", hits[0].Subject)
}

func TestSearch_ReturnsNothingBeforeTheIndexIsBuilt(t *testing.T) {
	ix, _ := newIndex(t, &embed.Fake{})

	hits, err := ix.Search(context.Background(), "containers", 3)

	require.NoError(t, err)
	assert.Empty(t, hits)
	assert.Equal(t, semantic.StateIndexing, ix.Status().State)
}

func TestBuild_EmbedsOnlyChangedChunksOnTheNextRun(t *testing.T) {
	root, u := universe(t)
	fake := &embed.Fake{}
	dir := t.TempDir()
	_, err := semantic.New(fake, semantic.Thresholds{Search: 0.1}, dir).Build(context.Background(), semantic.Chunks(u))
	require.NoError(t, err)
	before := fake.Texts.Load()
	write(t, root, "galaxies/tech/k8s/argo.md", "---\nname: Argo CD\nlevel: 4\n---\nGitOps delivery, now with rollouts.\n")
	u2, _, err := kb.Load(root)
	require.NoError(t, err)

	embedded, err := semantic.New(fake, semantic.Thresholds{Search: 0.1}, dir).Build(context.Background(), semantic.Chunks(u2))

	require.NoError(t, err)
	assert.Equal(t, 1, embedded)
	assert.Equal(t, before+1, fake.Texts.Load())
}

func TestNeighbors_SkipsTheSubjectItselfAndWhatTheCallerExcludes(t *testing.T) {
	_, u := universe(t)
	ix, _ := newIndex(t, &embed.Fake{})
	_, err := ix.Build(context.Background(), semantic.Chunks(u))
	require.NoError(t, err)

	hits := ix.Neighbors("tech/k8s/gvisor", 5, func(id string) bool { return id == "tech/k8s/argo" })

	require.Len(t, hits, 1)
	assert.Equal(t, "cuisine/patisserie/tarte", hits[0].Subject)
}

func TestBuild_KeepsThePreviousVectorsWhenCancelled(t *testing.T) {
	_, u := universe(t)
	ix, _ := newIndex(t, &embed.Fake{})
	_, err := ix.Build(context.Background(), semantic.Chunks(u))
	require.NoError(t, err)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	u.Stars()[0].Summary = "changed so the cache misses"

	_, err = ix.Build(ctx, semantic.Chunks(u))

	assert.ErrorIs(t, err, context.Canceled)
	assert.True(t, ix.Ready())
	assert.Equal(t, semantic.StateReady, ix.Status().State)
}

// gated blocks any batch containing "slow" until release is closed, to interleave two builds.
type gated struct {
	embed.Fake
	started chan struct{}
	release chan struct{}
}

func (g *gated) Embed(ctx context.Context, texts []string, kind embed.Kind) ([][]float32, error) {
	for _, t := range texts {
		if strings.Contains(t, "slow") {
			close(g.started)
			<-g.release
		}
	}
	return g.Fake.Embed(ctx, texts, kind)
}

func TestBuild_DropsAnOlderBuildThatFinishesAfterANewerOne(t *testing.T) {
	g := &gated{started: make(chan struct{}), release: make(chan struct{})}
	dir := t.TempDir()
	ix := semantic.New(g, semantic.Thresholds{}, dir)
	old := []semantic.Chunk{{ID: "a", Subject: "a", Text: "slow old text", Hash: "h-old"}}
	fresh := []semantic.Chunk{{ID: "b", Subject: "b", Text: "fresh text", Hash: "h-new"}}
	errs := make(chan error, 1)
	go func() { _, err := ix.Build(context.Background(), old); errs <- err }()
	<-g.started

	_, err := ix.Build(context.Background(), fresh)
	require.NoError(t, err)
	close(g.release)

	assert.ErrorIs(t, <-errs, semantic.ErrOutdated)
	assert.Equal(t, []string{"b"}, subjectsOf(ix))
	embedded, err := semantic.New(&embed.Fake{}, semantic.Thresholds{}, dir).Build(context.Background(), fresh)
	require.NoError(t, err)
	assert.Zero(t, embedded, "the cache holds the newer build's vectors")
}

func subjectsOf(ix *semantic.Index) []string {
	var out []string
	for id := range ix.SubjectVectors() {
		out = append(out, id)
	}
	return out
}

func TestBuild_ReembedsEverything_WhenTheCacheFileIsUnreadable(t *testing.T) {
	_, u := universe(t)
	dir := t.TempDir()
	_, err := semantic.New(&embed.Fake{}, semantic.Thresholds{}, dir).Build(context.Background(), semantic.Chunks(u))
	require.NoError(t, err)
	files, _ := filepath.Glob(filepath.Join(dir, "*.bin"))
	require.Len(t, files, 1)
	require.NoError(t, os.WriteFile(files[0], []byte("not a gob"), 0o644))

	embedded, err := semantic.New(&embed.Fake{}, semantic.Thresholds{}, dir).Build(context.Background(), semantic.Chunks(u))

	require.NoError(t, err)
	assert.Equal(t, len(semantic.Chunks(u)), embedded)
}
