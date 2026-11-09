package mcpserver_test

import (
	"context"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/TheGostsniperfr/road-to-knowledge/internal/app"
	"github.com/TheGostsniperfr/road-to-knowledge/internal/embed"
	"github.com/TheGostsniperfr/road-to-knowledge/internal/mcpserver"
	"github.com/TheGostsniperfr/road-to-knowledge/internal/semantic"
)

func semanticSession(t *testing.T) *mcp.ClientSession {
	t.Helper()
	root := t.TempDir()
	for p, c := range map[string]string{
		"galaxies/tech/k8s/gvisor.md":   "---\nname: gVisor\nlevel: 0\n---\nSandbox isolating containers from the kernel.\n",
		"galaxies/tech/k8s/runtimes.md": "---\nname: Container runtimes\nlevel: 4\n---\nContainers, runc and the kernel.\n",
	} {
		full := filepath.Join(root, p)
		require.NoError(t, os.MkdirAll(filepath.Dir(full), 0o755))
		require.NoError(t, os.WriteFile(full, []byte(c), 0o644))
	}
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	svc, err := app.New(ctx, root, slog.New(slog.NewTextHandler(io.Discard, nil)))
	require.NoError(t, err)
	svc.EnableSemantic(ctx, func(context.Context) (embed.Embedder, semantic.Thresholds, error) {
		return &embed.Fake{Synonyms: map[string]string{"isoler": "isolating"}}, semantic.Thresholds{Search: 0.2}, nil
	})
	wait, done := context.WithTimeout(ctx, 5*time.Second)
	defer done()
	require.NoError(t, svc.WaitSemantic(wait))
	st, ct := mcp.NewInMemoryTransports()
	_, err = mcpserver.New(svc, "test").Connect(ctx, st, nil)
	require.NoError(t, err)
	cs, err := mcp.NewClient(&mcp.Implementation{Name: "t"}, nil).Connect(ctx, ct, nil)
	require.NoError(t, err)
	t.Cleanup(func() { cs.Close() })
	return cs
}

func TestSearch_SaysAHitWasFoundByMeaning(t *testing.T) {
	cs := semanticSession(t)

	out, isErr := call(t, cs, "search", map[string]any{"query": "isoler"})

	assert.False(t, isErr, out)
	assert.Contains(t, out, `"id":"tech/k8s/gvisor"`)
	assert.Contains(t, out, `"mode":"meaning"`)
}

func TestNeighbors_ReturnsCloseSubjectsWithAScore(t *testing.T) {
	cs := semanticSession(t)

	out, isErr := call(t, cs, "neighbors", map[string]any{"id": "tech/k8s/gvisor"})

	assert.False(t, isErr, out)
	assert.Contains(t, out, `"id":"tech/k8s/runtimes"`)
	assert.Contains(t, out, `"score":`)
}

func TestNeighbors_ExplainsWhatToDo_WhenSearchByMeaningIsOff(t *testing.T) {
	cs, _ := session(t)

	out, isErr := call(t, cs, "neighbors", map[string]any{"id": "tech/k8s/gvisor"})

	assert.True(t, isErr)
	assert.Contains(t, out, "rtk models pull")
}
