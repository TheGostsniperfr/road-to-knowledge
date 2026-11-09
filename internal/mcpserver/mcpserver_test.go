package mcpserver_test

import (
	"context"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/TheGostsniperfr/road-to-knowledge/internal/app"
	"github.com/TheGostsniperfr/road-to-knowledge/internal/mcpserver"
)

func session(t *testing.T) (*mcp.ClientSession, string) {
	t.Helper()
	root := t.TempDir()
	files := map[string]string{
		"galaxies/lang/japonais/kana.md": "---\nname: Kana\nlevel: 4\n---\nDeux syllabaires phonétiques.\n\nあ :: a\n\n## Hiragana\nSignes ronds pour la grammaire.\n\n## Katakana\nMots empruntés.\n",
		"galaxies/tech/k8s/gvisor.md":    "---\nname: gVisor\nlevel: 0\n---\nNoyau en espace utilisateur.\n",
	}
	for p, c := range files {
		full := filepath.Join(root, p)
		require.NoError(t, os.MkdirAll(filepath.Dir(full), 0o755))
		require.NoError(t, os.WriteFile(full, []byte(c), 0o644))
	}
	for _, args := range [][]string{{"init", "-q", "-b", "main"}, {"config", "user.email", "t@e.st"}, {"config", "user.name", "t"}, {"add", "."}, {"commit", "-qm", "init"}} {
		cmd := exec.Command("git", args...)
		cmd.Dir = root
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, string(out))
	}
	ctx := context.Background()
	svc, err := app.New(ctx, root, slog.New(slog.NewTextHandler(io.Discard, nil)))
	require.NoError(t, err)
	st, ct := mcp.NewInMemoryTransports()
	_, err = mcpserver.New(svc, "test").Connect(ctx, st, nil)
	require.NoError(t, err)
	cs, err := mcp.NewClient(&mcp.Implementation{Name: "test"}, nil).Connect(ctx, ct, nil)
	require.NoError(t, err)
	t.Cleanup(func() { cs.Close() })
	return cs, root
}

func call(t *testing.T, cs *mcp.ClientSession, name string, args map[string]any) (string, bool) {
	t.Helper()
	res, err := cs.CallTool(context.Background(), &mcp.CallToolParams{Name: name, Arguments: args})
	require.NoError(t, err)
	return res.Content[0].(*mcp.TextContent).Text, res.IsError
}

func TestProfile_ListsThemesAndFrontier(t *testing.T) {
	cs, _ := session(t)

	out, isErr := call(t, cs, "profile", map[string]any{})

	assert.False(t, isErr)
	assert.Contains(t, out, `"frontier":["tech/k8s/gvisor"]`)
}

func TestNode_PointsToFragmentsInsteadOfReturningBody(t *testing.T) {
	cs, _ := session(t)

	out, _ := call(t, cs, "node", map[string]any{"id": "lang/japonais/kana"})

	assert.Contains(t, out, `"planets":["Hiragana","Katakana"]`)
	assert.NotContains(t, out, "Mots empruntés")
	assert.Contains(t, out, "fragments(")
}

func TestFragments_ReturnsOneNotion(t *testing.T) {
	cs, _ := session(t)

	out, _ := call(t, cs, "fragments", map[string]any{"id": "lang/japonais/kana", "section": "katakana"})

	assert.Equal(t, "## Katakana\nMots empruntés.", out)
}

func TestSearch_FindsSubject(t *testing.T) {
	cs, _ := session(t)

	out, _ := call(t, cs, "search", map[string]any{"query": "syllabaires"})

	assert.Contains(t, out, "lang/japonais/kana")
}

func TestNode_ReturnsToolError_WhenUnknown(t *testing.T) {
	cs, _ := session(t)

	_, isErr := call(t, cs, "node", map[string]any{"id": "nope"})

	assert.True(t, isErr)
}

func TestFrontier_ExplainsEachSuggestion(t *testing.T) {
	cs, _ := session(t)

	out, isErr := call(t, cs, "frontier", map[string]any{})

	assert.False(t, isErr)
	assert.Contains(t, out, `"reason":"k8s (tech), exploré à 0 %"`)
}

func TestFrontier_ReturnsToolError_WhenScopeIsUnknown(t *testing.T) {
	cs, _ := session(t)

	_, isErr := call(t, cs, "frontier", map[string]any{"scope": "nope"})

	assert.True(t, isErr)
}

func TestPropose_LandsOnProposalBranchOnly(t *testing.T) {
	cs, root := session(t)

	out, isErr := call(t, cs, "propose", map[string]any{"slug": "fiche-keigo", "message": "docs: keigo", "files": []map[string]string{{"path": "galaxies/lang/japonais/keigo.md", "content": "---\nname: Keigo\n---\n"}}})

	require.False(t, isErr, out)
	assert.Contains(t, out, "proposals/fiche-keigo")
	assert.NoFileExists(t, filepath.Join(root, "galaxies/lang/japonais/keigo.md"))
}

func TestPropose_RejectsPathOutsideKnowledge(t *testing.T) {
	cs, _ := session(t)

	out, isErr := call(t, cs, "propose", map[string]any{"slug": "x", "message": "x", "files": []map[string]string{{"path": "../../.bashrc.md", "content": "x"}}})

	assert.True(t, isErr)
	assert.True(t, strings.Contains(out, "galaxies/"))
}
