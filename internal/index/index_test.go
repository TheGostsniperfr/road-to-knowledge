package index_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/TheGostsniperfr/road-to-knowledge/internal/index"
	"github.com/TheGostsniperfr/road-to-knowledge/internal/kb"
)

func universe(t *testing.T) *kb.Universe {
	t.Helper()
	root := t.TempDir()
	files := map[string]string{
		"galaxies/lang/japonais/kana.md":     "---\nname: Kana\nlevel: 4\n---\nDeux syllabaires phonétiques.\n",
		"galaxies/tech/k8s/gvisor.md":        "---\nname: gVisor\nlevel: 1\n---\nNoyau en espace utilisateur, isolation forte.\n",
		"galaxies/tech/k8s/runtime-class.md": "---\nname: RuntimeClass\nlevel: 2\n---\nChoisir gVisor ou runc par pod.\n",
	}
	for p, c := range files {
		full := filepath.Join(root, p)
		require.NoError(t, os.MkdirAll(filepath.Dir(full), 0o755))
		require.NoError(t, os.WriteFile(full, []byte(c), 0o644))
	}
	u, _, err := kb.Load(root)
	require.NoError(t, err)
	return u
}

func TestSearch_FindsByNamePrefix(t *testing.T) {
	ix, err := index.Build(context.Background(), universe(t))
	require.NoError(t, err)

	hits, err := ix.Search(context.Background(), "kan", 5)

	require.NoError(t, err)
	require.NotEmpty(t, hits)
	assert.Equal(t, "lang/japonais/kana", hits[0].ID)
}

func TestSearch_RanksNameMatchAboveBodyMatch(t *testing.T) {
	ix, err := index.Build(context.Background(), universe(t))
	require.NoError(t, err)

	hits, err := ix.Search(context.Background(), "gvisor", 5)

	require.NoError(t, err)
	require.Len(t, hits, 2)
	assert.Equal(t, "tech/k8s/gvisor", hits[0].ID)
}

func TestSearch_IgnoresAccentsAndFtsSyntax(t *testing.T) {
	ix, err := index.Build(context.Background(), universe(t))
	require.NoError(t, err)

	hits, err := ix.Search(context.Background(), `phonetiques"(`, 5)

	require.NoError(t, err)
	require.Len(t, hits, 1)
	assert.Equal(t, "lang/japonais/kana", hits[0].ID)
}
