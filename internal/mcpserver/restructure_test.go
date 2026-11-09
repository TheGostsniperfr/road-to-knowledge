package mcpserver_test

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRestructure_SplitsOnAProposalBranch_AndLeavesMainAlone(t *testing.T) {
	cs, root := session(t)

	out, isErr := call(t, cs, "restructure", map[string]any{"kind": "split", "id": "lang/japonais/kana", "parts": []map[string]any{{"id": "katakana", "notions": []string{"Katakana"}}}})

	require.False(t, isErr, out)
	assert.NoFileExists(t, filepath.Join(root, "galaxies/lang/japonais/katakana.md"))
	changed := gitIn(t, root, "diff", "--name-status", "main", "proposals/restructure-split-lang-japonais-kana")
	assert.Contains(t, changed, "A\tgalaxies/lang/japonais/katakana.md")
	assert.Contains(t, changed, "M\tgalaxies/lang/japonais/kana.md")
}

func TestRestructure_DryRunReturnsThePlanWithoutABranch(t *testing.T) {
	cs, root := session(t)

	out, isErr := call(t, cs, "restructure", map[string]any{"kind": "move", "id": "tech/k8s/gvisor", "to": "lang/japonais", "dry_run": true})

	require.False(t, isErr, out)
	assert.Contains(t, out, "lang/japonais/gvisor")
	assert.Empty(t, gitIn(t, root, "branch", "--list", "proposals/*"))
}

func TestRestructure_ExplainsARefusedMove(t *testing.T) {
	cs, _ := session(t)

	out, isErr := call(t, cs, "restructure", map[string]any{"kind": "move", "id": "tech/k8s/gvisor", "to": "lang/japonais/kana"})

	assert.True(t, isErr)
	assert.Contains(t, out, "already exists")
}

func TestPropose_StillRefusesToDeleteASubject(t *testing.T) {
	cs, _ := session(t)

	out, isErr := call(t, cs, "propose", map[string]any{"slug": "rm", "message": "chore: rm", "files": []map[string]any{{"path": "galaxies/tech/k8s/gvisor.md", "delete": true}}})

	assert.True(t, isErr, out)
}
