package library_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/TheGostsniperfr/road-to-knowledge/internal/library"
)

func writeBook(t *testing.T, name, content string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), name)
	require.NoError(t, os.WriteFile(p, []byte(content), 0o644))
	return p
}

func TestAdd_CopiesTheFileUnderANameWithItsHash(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	src := writeBook(t, "Designing Data Apps.pdf", "%PDF-1.7 fake")

	e, err := library.Add(root, src)

	require.NoError(t, err)
	assert.Regexp(t, `^library/designing-data-apps-[0-9a-f]{8}\.pdf$`, e.File)
	got, err := os.ReadFile(filepath.Join(root, e.File))
	require.NoError(t, err)
	assert.Equal(t, "%PDF-1.7 fake", string(got))
}

func TestAdd_DoesNotCopyTwice_WhenTheSameContentIsAlreadyThere(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	src := writeBook(t, "book.pdf", "same bytes")
	first, err := library.Add(root, src)
	require.NoError(t, err)

	again, err := library.Add(root, src)

	require.NoError(t, err)
	assert.Equal(t, first.File, again.File)
	assert.False(t, again.Added)
}

func TestAdd_IgnoresTheLibraryInGitOnce(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, ".gitignore"), []byte(".rtk/"), 0o644))
	_, err := library.Add(root, writeBook(t, "a.pdf", "a"))
	require.NoError(t, err)

	_, err = library.Add(root, writeBook(t, "b.pdf", "b"))

	require.NoError(t, err)
	b, _ := os.ReadFile(filepath.Join(root, ".gitignore"))
	assert.Equal(t, 1, strings.Count(string(b), "library/"))
	assert.Equal(t, ".rtk/\nlibrary/\n", string(b))
}

func TestAdd_KeepsGitignoreUntouched_WhenTheLibraryIsAlreadyIgnored(t *testing.T) {
	t.Parallel()
	for _, rule := range []string{"library", "library/", "/library/", "library/*", "/library/*"} {
		t.Run(rule, func(t *testing.T) {
			t.Parallel()
			root := t.TempDir()
			want := ".rtk/\n" + rule + "\n"
			require.NoError(t, os.WriteFile(filepath.Join(root, ".gitignore"), []byte(want), 0o644))

			_, err := library.Add(root, writeBook(t, "a.pdf", "a"))

			require.NoError(t, err)
			got, _ := os.ReadFile(filepath.Join(root, ".gitignore"))
			assert.Equal(t, want, string(got))
		})
	}
}

func TestAdd_RefusesFilesThatAreNotDocuments(t *testing.T) {
	t.Parallel()

	_, err := library.Add(t.TempDir(), writeBook(t, "run.sh", "echo"))

	assert.ErrorIs(t, err, library.ErrUnsupported)
}
