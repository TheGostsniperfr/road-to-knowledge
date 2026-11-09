package gitx_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/TheGostsniperfr/road-to-knowledge/internal/gitx"
)

func TestCommit_ReturnsAndReleasesTheRepo_WhenAHookHangs(t *testing.T) {
	dir := gitRepo(t)
	hook := filepath.Join(dir, ".git", "hooks", "pre-commit")
	require.NoError(t, os.WriteFile(hook, []byte("#!/bin/sh\nsleep 60\n"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "a.md"), []byte("a"), 0o644))
	r := gitx.Open(context.Background(), dir)
	require.NotNil(t, r)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	start := time.Now()
	err := r.Commit(ctx, "chore: a", filepath.Join(dir, "a.md"))

	require.Error(t, err)
	assert.Less(t, time.Since(start), 6*time.Second)
	require.NoError(t, os.Remove(hook))
	assert.NoError(t, r.Commit(context.Background(), "chore: a", filepath.Join(dir, "a.md")), "the repo lock must be free again")
}

func TestLog_KeepsAccentsReadable_AndCapsFilesPerCommit(t *testing.T) {
	dir := gitRepo(t)
	r := gitx.Open(context.Background(), dir)
	accent := filepath.Join(dir, "thème.md")
	require.NoError(t, os.WriteFile(accent, []byte("x"), 0o644))
	require.NoError(t, r.Commit(context.Background(), "feat: thème", accent))
	var paths []string
	for _, name := range []string{"a.md", "b.md", "c.md", "d.md", "e.md", "f.md", "g.md", "h.md", "i.md", "j.md"} {
		p := filepath.Join(dir, name)
		require.NoError(t, os.WriteFile(p, []byte("x"), 0o644))
		paths = append(paths, p)
	}
	require.NoError(t, r.Commit(context.Background(), "feat: seed", paths...))

	commits, err := r.Log(context.Background(), time.Now().Add(-time.Hour), 5)

	require.NoError(t, err)
	require.GreaterOrEqual(t, len(commits), 2)
	assert.Len(t, commits[0].Files, 8)
	assert.Equal(t, 2, commits[0].More)
	assert.Equal(t, []string{"thème.md"}, commits[1].Files)
}
