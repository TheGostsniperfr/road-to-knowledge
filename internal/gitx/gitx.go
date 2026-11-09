// Package gitx commits changes to the knowledge repository. Edits from the app land on the current
// branch; edits from an agent land on a proposals/* branch through a temporary worktree, so the
// user's checkout is never touched.
package gitx

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
)

type Repo struct {
	root string
	mu   sync.Mutex
}

// Open returns nil unless root is the top of a git work tree. A knowledge folder nested inside
// another repository (like the examples in the app repo) must never receive commits.
func Open(ctx context.Context, root string) *Repo {
	r := &Repo{root: root}
	out, err := r.git(ctx, "rev-parse", "--show-toplevel")
	if err != nil {
		return nil
	}
	top, err1 := filepath.EvalSymlinks(strings.TrimSpace(out))
	abs, err2 := filepath.EvalSymlinks(root)
	if err1 != nil || err2 != nil || top != abs {
		return nil
	}
	return r
}

func (r *Repo) git(ctx context.Context, args ...string) (string, error) {
	return run(ctx, r.root, args...)
}

func run(ctx context.Context, dir string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("git %s: %w: %s", args[0], err, strings.TrimSpace(stderr.String()))
	}
	return stdout.String(), nil
}

// Commit stages exactly the given paths and commits them; it is a no-op when nothing changed.
func (r *Repo) Commit(ctx context.Context, message string, paths ...string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	rel := make([]string, 0, len(paths))
	for _, p := range paths {
		rp, err := filepath.Rel(r.root, p)
		if err != nil {
			return fmt.Errorf("commit: %w", err)
		}
		rel = append(rel, filepath.ToSlash(rp))
	}
	if _, err := r.git(ctx, append([]string{"add", "--"}, rel...)...); err != nil {
		return err
	}
	if _, err := r.git(ctx, append([]string{"diff", "--cached", "--quiet", "--"}, rel...)...); err == nil {
		return nil
	}
	_, err := r.git(ctx, append([]string{"commit", "-m", message, "--"}, rel...)...)
	return err
}

var ErrBadPath = errors.New("path must be a relative .md file under galaxies/ or inbox/")

// ValidProposalPath blocks traversal and keeps agents inside the knowledge folders.
func ValidProposalPath(p string) (string, error) {
	clean := filepath.ToSlash(filepath.Clean(p))
	if filepath.IsAbs(p) || strings.HasPrefix(clean, "../") || clean == ".." || !strings.HasSuffix(clean, ".md") {
		return "", ErrBadPath
	}
	if !strings.HasPrefix(clean, "galaxies/") && !strings.HasPrefix(clean, "inbox/") {
		return "", ErrBadPath
	}
	return clean, nil
}

type FileChange struct {
	Path    string
	Content string
}

// Propose writes files on branch proposals/<slug>, created from HEAD when missing.
func (r *Repo) Propose(ctx context.Context, slug, message string, files []FileChange) (string, error) {
	branch := "proposals/" + slug
	if _, err := run(ctx, r.root, "check-ref-format", "--branch", branch); err != nil {
		return "", fmt.Errorf("propose: invalid branch name %q", branch)
	}
	for i, f := range files {
		clean, err := ValidProposalPath(f.Path)
		if err != nil {
			return "", fmt.Errorf("propose %s: %w", f.Path, err)
		}
		files[i].Path = clean
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	tmp, err := os.MkdirTemp("", "rtk-proposal-")
	if err != nil {
		return "", fmt.Errorf("propose: %w", err)
	}
	os.Remove(tmp)
	if _, err := r.git(ctx, "rev-parse", "--verify", "--quiet", "refs/heads/"+branch); err == nil {
		_, err = r.git(ctx, "worktree", "add", tmp, branch)
		if err != nil {
			return "", err
		}
	} else if _, err := r.git(ctx, "worktree", "add", "-b", branch, tmp, "HEAD"); err != nil {
		return "", err
	}
	defer func() {
		r.git(context.WithoutCancel(ctx), "worktree", "remove", "--force", tmp)
	}()
	rel := make([]string, 0, len(files))
	for _, f := range files {
		full := filepath.Join(tmp, filepath.FromSlash(f.Path))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			return "", fmt.Errorf("propose: %w", err)
		}
		if err := os.WriteFile(full, []byte(f.Content), 0o644); err != nil {
			return "", fmt.Errorf("propose: %w", err)
		}
		rel = append(rel, f.Path)
	}
	if _, err := run(ctx, tmp, append([]string{"add", "--"}, rel...)...); err != nil {
		return "", err
	}
	if _, err := run(ctx, tmp, "commit", "-m", message); err != nil {
		return "", err
	}
	return branch, nil
}
