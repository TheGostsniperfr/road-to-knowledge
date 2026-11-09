package gitx_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/TheGostsniperfr/road-to-knowledge/internal/gitx"
)

func repoWithSubject(t *testing.T) (string, *gitx.Repo) {
	t.Helper()
	dir := gitRepo(t)
	p := filepath.Join(dir, "galaxies/a/b/c.md")
	require.NoError(t, os.MkdirAll(filepath.Dir(p), 0o755))
	require.NoError(t, os.WriteFile(p, []byte("---\nname: C\n---\nBody.\n"), 0o644))
	git(t, dir, "add", ".")
	git(t, dir, "commit", "-qm", "subject")
	return dir, gitx.Open(context.Background(), dir)
}

func TestPropose_MovesAFile_WhenTheChangeIsARestructure(t *testing.T) {
	dir, r := repoWithSubject(t)

	branch, err := r.Propose(context.Background(), "mv", "refactor: mv", []gitx.FileChange{
		{Path: "galaxies/a/d/c.md", From: "galaxies/a/b/c.md", Content: "---\nname: C\n---\nBody.\n", Restructure: true},
		{Path: gitx.RenamesFile, Content: "{}\n", Restructure: true},
	}, nil)

	require.NoError(t, err)
	assert.Contains(t, git(t, dir, "show", "-M", "--name-status", "--format=", branch), "R100\tgalaxies/a/b/c.md\tgalaxies/a/d/c.md")
}

func TestPropose_RefusesMovesDeletesAndTheRenameLog_FromAnAgent(t *testing.T) {
	tests := []struct {
		name   string
		change gitx.FileChange
		want   error
	}{
		{"a move", gitx.FileChange{Path: "galaxies/a/d/c.md", From: "galaxies/a/b/c.md", Content: "x"}, gitx.ErrBadMove},
		{"a subject delete", gitx.FileChange{Path: "galaxies/a/b/c.md", Delete: true}, gitx.ErrBadDelete},
		{"the rename log", gitx.FileChange{Path: gitx.RenamesFile, Content: "{}"}, gitx.ErrBadPath},
		{"a restructure moving outside galaxies", gitx.FileChange{Path: "inbox/c.md", From: "galaxies/a/b/c.md", Content: "x", Restructure: true}, gitx.ErrBadMove},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, r := repoWithSubject(t)

			_, err := r.Propose(context.Background(), "x", "x", []gitx.FileChange{tt.change}, nil)

			assert.ErrorIs(t, err, tt.want)
		})
	}
}

func TestCommit_StagesADeletedFile_AndSkipsOneGitNeverSaw(t *testing.T) {
	dir, r := repoWithSubject(t)
	old := filepath.Join(dir, "galaxies/a/b/c.md")
	require.NoError(t, os.Remove(old))

	err := r.Commit(context.Background(), "chore: rm", old, filepath.Join(dir, "galaxies/never/was.md"))

	require.NoError(t, err)
	assert.Equal(t, "D\tgalaxies/a/b/c.md", git(t, dir, "show", "--name-status", "--format=", "HEAD"))
}
