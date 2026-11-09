package mcpserver_test

import (
	"os/exec"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The book-ingest skill sends a whole book in one propose(): a new galaxy, a sub-theme,
// the fiche with its source, and a fog subject in another theme.
func TestPropose_TakesAWholeBookInOneCall(t *testing.T) {
	cs, root := session(t)
	fiche := "---\nname: Le journal avant tout\nlevel: 1\nlinks: [raft]\nsource:\n  title: Le journal avant tout\n  file: library/journal-6e5505f8.pdf\n---\nRésumé.\n\n## À retenir : la règle du WAL (p. 3)\nTexte.\n\nQue signifie WAL ? :: Write-ahead log\n"
	files := []map[string]string{
		{"path": "galaxies/livres/_galaxy.md", "content": "---\nname: Livres\nshape: lenticular\n---\n"},
		{"path": "galaxies/livres/tech/_cluster.md", "content": "---\nname: Tech\n---\n"},
		{"path": "galaxies/livres/tech/journal-avant-tout.md", "content": fiche},
		{"path": "galaxies/lang/japonais/raft.md", "content": "---\nname: Raft\nlevel: 0\nstate: explore\n---\nConsensus.\n"},
	}

	out, isErr := call(t, cs, "propose", map[string]any{"slug": "book-journal", "message": "feat(book): le journal avant tout", "files": files})

	require.False(t, isErr, out)
	ls, err := exec.Command("git", "-C", root, "ls-tree", "-r", "--name-only", "proposals/book-journal").Output()
	require.NoError(t, err)
	for _, f := range files {
		assert.Contains(t, strings.Split(strings.TrimSpace(string(ls)), "\n"), f["path"])
	}
}

func TestPropose_RefusesToCommitTheBookItself(t *testing.T) {
	cs, _ := session(t)

	_, isErr := call(t, cs, "propose", map[string]any{"slug": "book-x", "message": "x", "files": []map[string]string{{"path": "library/book.pdf", "content": "%PDF"}}})

	assert.True(t, isErr)
}
