package gitx_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/TheGostsniperfr/road-to-knowledge/internal/gitx"
)

func gitRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	for _, args := range [][]string{
		{"init", "-q", "-b", "main"},
		{"config", "user.email", "test@example.com"},
		{"config", "user.name", "test"},
		{"commit", "-q", "--allow-empty", "-m", "init"},
	} {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, string(out))
	}
	return dir
}

func git(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, string(out))
	return strings.TrimSpace(string(out))
}

func TestOpen_ReturnsNil_WhenDirectoryIsNotARepo(t *testing.T) {
	assert.Nil(t, gitx.Open(context.Background(), t.TempDir()))
}

func TestOpen_ReturnsNil_WhenDirectoryIsNestedInAnotherRepo(t *testing.T) {
	dir := gitRepo(t)
	nested := filepath.Join(dir, "examples", "knowledge")
	require.NoError(t, os.MkdirAll(nested, 0o755))

	assert.Nil(t, gitx.Open(context.Background(), nested))
}

func TestCommit_CommitsOnlyGivenPath(t *testing.T) {
	dir := gitRepo(t)
	r := gitx.Open(context.Background(), dir)
	require.NotNil(t, r)
	a, b := filepath.Join(dir, "a.md"), filepath.Join(dir, "b.md")
	require.NoError(t, os.WriteFile(a, []byte("a"), 0o644))
	require.NoError(t, os.WriteFile(b, []byte("b"), 0o644))

	require.NoError(t, r.Commit(context.Background(), "add a", a))

	assert.Equal(t, "a.md", git(t, dir, "show", "--name-only", "--format=", "HEAD"))
}

func TestCommit_IsNoop_WhenNothingChanged(t *testing.T) {
	dir := gitRepo(t)
	r := gitx.Open(context.Background(), dir)
	a := filepath.Join(dir, "a.md")
	require.NoError(t, os.WriteFile(a, []byte("a"), 0o644))
	require.NoError(t, r.Commit(context.Background(), "add a", a))

	require.NoError(t, r.Commit(context.Background(), "again", a))

	assert.Equal(t, "2", git(t, dir, "rev-list", "--count", "HEAD"))
}

func TestPropose_WritesOnBranchWithoutTouchingCheckout(t *testing.T) {
	dir := gitRepo(t)
	r := gitx.Open(context.Background(), dir)

	branch, err := r.Propose(context.Background(), "fiche-kana", "docs: fiche kana", []gitx.FileChange{{Path: "galaxies/lang/japonais/kana.md", Content: "---\nname: Kana\n---\n"}}, nil)

	require.NoError(t, err)
	assert.Equal(t, "proposals/fiche-kana", branch)
	assert.Equal(t, "main", git(t, dir, "branch", "--show-current"))
	assert.NoFileExists(t, filepath.Join(dir, "galaxies/lang/japonais/kana.md"))
	assert.Equal(t, "galaxies/lang/japonais/kana.md", git(t, dir, "show", "--name-only", "--format=", branch))
}

func TestPropose_AppendsToExistingBranch(t *testing.T) {
	dir := gitRepo(t)
	r := gitx.Open(context.Background(), dir)
	_, err := r.Propose(context.Background(), "seed", "one", []gitx.FileChange{{Path: "inbox/a.md", Content: "a"}}, nil)
	require.NoError(t, err)

	_, err = r.Propose(context.Background(), "seed", "two", []gitx.FileChange{{Path: "inbox/b.md", Content: "b"}}, nil)

	require.NoError(t, err)
	assert.Equal(t, "3", git(t, dir, "rev-list", "--count", "proposals/seed"))
}

func TestValidProposalPath_RejectsEscapes(t *testing.T) {
	tests := []struct{ name, path string }{
		{"parent traversal", "../etc/passwd.md"},
		{"nested traversal", "galaxies/../../x.md"},
		{"absolute", "/tmp/x.md"},
		{"outside knowledge folders", "review/log.md"},
		{"not markdown", "galaxies/a/b/c.sh"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := gitx.ValidProposalPath(tt.path)
			assert.ErrorIs(t, err, gitx.ErrBadPath)
		})
	}
}
