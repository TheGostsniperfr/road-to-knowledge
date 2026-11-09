package kb_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/TheGostsniperfr/road-to-knowledge/internal/kb"
)

func TestSourceProblems(t *testing.T) {
	t.Parallel()
	hash := strings.Repeat("a", 64)
	tests := []struct {
		name string
		src  kb.Source
		want string
	}{
		{"accepts a full source", kb.Source{Title: "T", File: "library/t-12345678.pdf", SHA256: hash, Pages: 300}, ""},
		{"accepts a title alone", kb.Source{Title: "T"}, ""},
		{"needs a title", kb.Source{File: "library/t.pdf"}, "title"},
		{"refuses a file outside the library", kb.Source{Title: "T", File: "galaxies/x.md"}, "library/"},
		{"refuses a path that climbs out", kb.Source{Title: "T", File: "library/../secret.pdf"}, "library/"},
		{"refuses an absolute path", kb.Source{Title: "T", File: "/etc/passwd"}, "library/"},
		{"accepts two dots inside a file name", kb.Source{Title: "T", File: "library/vol..2.pdf"}, ""},
		{"refuses the library folder itself", kb.Source{Title: "T", File: "library/"}, "library/"},
		{"refuses a short hash", kb.Source{Title: "T", SHA256: "abc"}, "sha256"},
		{"refuses negative pages", kb.Source{Title: "T", Pages: -1}, "negative"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := strings.Join(tt.src.Problems(), "; ")

			if tt.want == "" {
				assert.Empty(t, got)
			} else {
				assert.Contains(t, got, tt.want)
			}
		})
	}
}

func writeFiche(t *testing.T, root, rel, content string) {
	t.Helper()
	p := filepath.Join(root, filepath.FromSlash(rel))
	require.NoError(t, os.MkdirAll(filepath.Dir(p), 0o755))
	require.NoError(t, os.WriteFile(p, []byte(content), 0o644))
}

func TestLoad_ReadsTheSourceOfABookFiche(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeFiche(t, root, "galaxies/livres/tech/ddia.md", "---\nname: DDIA\nlevel: 1\nsource:\n  title: Designing Data-Intensive Applications\n  author: Martin Kleppmann\n  pages: 562\n  file: library/ddia-0123abcd.pdf\n---\nRésumé.\n")

	u, problems, err := kb.Load(root)

	require.NoError(t, err)
	assert.Empty(t, problems)
	require.NotNil(t, u.Star("livres/tech/ddia").Source)
	assert.Equal(t, 562, u.Star("livres/tech/ddia").Source.Pages)
}

func TestLoad_ReportsABadSourceAndKeepsTheSubject(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeFiche(t, root, "galaxies/livres/tech/x.md", "---\nname: X\nsource:\n  title: X\n  file: ../../etc/passwd\n---\nRésumé.\n")

	u, problems, err := kb.Load(root)

	require.NoError(t, err)
	require.Len(t, problems, 1)
	assert.Contains(t, problems[0].Message, "library/")
	assert.Nil(t, u.Star("livres/tech/x").Source)
}

func TestSourceAvailable_IsFalse_WhenTheFileIsOnAnotherMachine(t *testing.T) {
	t.Parallel()

	ok := kb.Source{Title: "T", File: "library/absent.pdf"}.Available(t.TempDir())

	assert.False(t, ok)
}
