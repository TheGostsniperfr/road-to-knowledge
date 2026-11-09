// Package gitx commits changes to the knowledge repository. Edits from the app land on the current
// branch; edits from an agent land on a proposals/* branch through a temporary worktree, so the
// user's checkout is never touched.
package gitx

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"
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

// gitTimeout bounds every git call: a signing key waiting for a passphrase must fail an agent's
// request with a clear error, not hang it.
const gitTimeout = 30 * time.Second

const signingHint = " (commit signing is on for this repository: unlock the key, e.g. `echo ok | gpg --clearsign`, or run `git config commit.gpgsign false` in the knowledge repo)"

func run(ctx context.Context, dir string, args ...string) (string, error) {
	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, gitTimeout)
		defer cancel()
	}
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	// Hooks and signers are grandchildren of git: killing git alone leaves them holding the
	// output pipes, and Run would wait on them while r.mu stays locked. Kill the whole group,
	// and stop waiting for the pipes shortly after.
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error { return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL) }
	cmd.WaitDelay = 2 * time.Second
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	start := time.Now()
	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			removeOwnLock(dir, start)
		}
		msg := strings.TrimSpace(stderr.String())
		if args[0] == "commit" && (strings.Contains(msg, "gpg failed to sign") || ctx.Err() != nil) {
			msg += signingHint
		}
		return "", fmt.Errorf("git %s: %w: %s", args[0], err, msg)
	}
	return stdout.String(), nil
}

// removeOwnLock deletes the index.lock left by a git process we killed. A lock older than that
// process belongs to someone else and stays.
func removeOwnLock(dir string, start time.Time) {
	out, err := exec.Command("git", "-C", dir, "rev-parse", "--git-path", "index.lock").Output()
	if err != nil {
		return
	}
	lock := strings.TrimSpace(string(out))
	if !filepath.IsAbs(lock) {
		lock = filepath.Join(dir, lock)
	}
	if fi, err := os.Stat(lock); err == nil && !fi.ModTime().Before(start.Add(-time.Second)) {
		if err := os.Remove(lock); err != nil {
			slog.Warn("git: remove stale lock", "lock", lock, "err", err)
		}
	}
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
		// A removed file is staged as a deletion if git knew it; one git never saw has nothing to commit.
		if _, err := os.Lstat(p); errors.Is(err, os.ErrNotExist) {
			if out, err := r.git(ctx, "ls-files", "--", filepath.ToSlash(rp)); err != nil || strings.TrimSpace(out) == "" {
				continue
			}
		}
		rel = append(rel, filepath.ToSlash(rp))
	}
	if len(rel) == 0 {
		return nil
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

// Unstage puts the index back to HEAD for the given paths, after a commit that failed past `git add`.
func (r *Repo) Unstage(ctx context.Context, paths ...string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	rel := make([]string, 0, len(paths))
	for _, p := range paths {
		if rp, err := filepath.Rel(r.root, p); err == nil {
			rel = append(rel, filepath.ToSlash(rp))
		}
	}
	if _, err := r.git(ctx, append([]string{"reset", "-q", "--"}, rel...)...); err != nil {
		slog.Warn("unstage after failed commit", "err", err)
	}
}

// HasBranch reports whether a local branch exists.
func (r *Repo) HasBranch(ctx context.Context, name string) bool {
	_, err := r.git(ctx, "rev-parse", "--verify", "--quiet", "refs/heads/"+name)
	return err == nil
}

// Dirty lists which of the given paths (relative to the repo root) have uncommitted changes, untracked included.
func (r *Repo) Dirty(ctx context.Context, rel ...string) ([]string, error) {
	if len(rel) == 0 {
		return nil, nil
	}
	out, err := r.git(ctx, append([]string{"status", "--porcelain", "--untracked-files=all", "--"}, rel...)...)
	if err != nil {
		return nil, err
	}
	var dirty []string
	for _, line := range strings.Split(strings.TrimRight(out, "\n"), "\n") {
		if len(line) > 3 {
			dirty = append(dirty, line[3:])
		}
	}
	return dirty, nil
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
	// Delete removes the file instead of writing it. Only inbox notes can be deleted, so a triage
	// proposal can clear the notes it placed; subjects are never removed by an agent.
	Delete bool
	// Data carries a binary asset instead of Content. Only trusted callers (the Anki import)
	// build it: MCP tools never fill it, so agents still write Markdown only.
	Data []byte
	// From moves an existing file to Path before Content is written, so history follows it.
	// Only a restructure planned by the server sets it (see Restructure).
	From string
	// Restructure marks a change computed by app.Restructure from the current files: it may move
	// or delete subjects and append to review/renames.jsonl. The MCP propose tool never sets it,
	// so an agent still cannot remove a subject by writing raw file changes.
	Restructure bool
}

// RenamesFile is the only non-Markdown file a restructure proposal writes.
const RenamesFile = "review/renames.jsonl"

func validChange(f FileChange) (string, error) {
	if f.Data != nil {
		return ValidAssetPath(f.Path)
	}
	if f.Restructure && filepath.ToSlash(filepath.Clean(f.Path)) == RenamesFile {
		return RenamesFile, nil
	}
	clean, err := ValidProposalPath(f.Path)
	if err != nil {
		return "", err
	}
	if f.Delete && !strings.HasPrefix(clean, "inbox/") && !(f.Restructure && strings.HasPrefix(clean, "galaxies/")) {
		return "", ErrBadDelete
	}
	if f.From != "" {
		from, err := ValidProposalPath(f.From)
		if err != nil {
			return "", err
		}
		if !f.Restructure || !strings.HasPrefix(from, "galaxies/") || !strings.HasPrefix(clean, "galaxies/") {
			return "", ErrBadMove
		}
	}
	return clean, nil
}

var ErrBadMove = errors.New("only a restructure moves files, and only inside galaxies/")

var ErrBadAsset = errors.New("asset must be an image or audio file under assets/anki/")

var assetExt = map[string]bool{".png": true, ".jpg": true, ".jpeg": true, ".gif": true, ".webp": true, ".mp3": true, ".ogg": true, ".wav": true, ".m4a": true}

// ValidAssetPath keeps binary writes inside assets/anki/ and refuses formats a browser would run (svg, html).
func ValidAssetPath(p string) (string, error) {
	clean := filepath.ToSlash(filepath.Clean(p))
	if filepath.IsAbs(p) || strings.Contains("/"+clean+"/", "/../") || !strings.HasPrefix(clean, "assets/anki/") {
		return "", ErrBadAsset
	}
	if !assetExt[strings.ToLower(filepath.Ext(clean))] {
		return "", ErrBadAsset
	}
	return clean, nil
}

var ErrBadDelete = errors.New("only inbox/ notes can be deleted by a proposal")

// Check inspects the proposal's working tree before the commit; an error aborts the proposal.
type Check func(dir string, paths []string) error

// Propose writes files on branch proposals/<slug>, created from HEAD when missing. A failed check
// or commit leaves no new branch behind.
func (r *Repo) Propose(ctx context.Context, slug, message string, files []FileChange, check Check) (_ string, err error) {
	branch := "proposals/" + slug
	name := branch
	if _, err := run(ctx, r.root, "check-ref-format", "--branch", branch); err != nil {
		return "", fmt.Errorf("propose: invalid branch name %q: use lowercase words joined by dashes, e.g. fiche-kana", branch)
	}
	for i, f := range files {
		clean, err := validChange(f)
		if err != nil {
			return "", fmt.Errorf("propose %s: %w", f.Path, err)
		}
		files[i].Path = clean
		if f.From != "" {
			files[i].From = filepath.ToSlash(filepath.Clean(f.From))
		}
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	tmp, err := os.MkdirTemp("", "rtk-proposal-")
	if err != nil {
		return "", fmt.Errorf("propose: %w", err)
	}
	os.Remove(tmp)
	created := false
	if _, err := r.git(ctx, "rev-parse", "--verify", "--quiet", "refs/heads/"+branch); err == nil {
		if _, err := r.git(ctx, "worktree", "add", tmp, branch); err != nil {
			return "", err
		}
	} else if _, err := r.git(ctx, "worktree", "add", "-b", branch, tmp, "HEAD"); err != nil {
		return "", err
	} else {
		created = true
	}
	defer func() {
		bg := context.WithoutCancel(ctx)
		if _, cerr := r.git(bg, "worktree", "remove", "--force", tmp); cerr != nil {
			slog.Warn("propose: remove worktree", "dir", tmp, "err", cerr)
			os.RemoveAll(tmp)
			if _, perr := r.git(bg, "worktree", "prune"); perr != nil {
				slog.Warn("propose: prune worktrees", "err", perr)
			}
		}
		if err != nil && created {
			if _, cerr := r.git(bg, "branch", "-D", name); cerr != nil {
				slog.Warn("propose: delete refused branch", "branch", name, "err", cerr)
			}
		}
	}()
	rel := make([]string, 0, len(files))
	for _, f := range files {
		full := filepath.Join(tmp, filepath.FromSlash(f.Path))
		if f.Delete {
			if err := os.Remove(full); err != nil {
				return "", fmt.Errorf("propose: delete %s: %w", f.Path, err)
			}
			rel = append(rel, f.Path)
			continue
		}
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			return "", fmt.Errorf("propose: %w", err)
		}
		if f.From != "" {
			if err := os.Rename(filepath.Join(tmp, filepath.FromSlash(f.From)), full); err != nil {
				return "", fmt.Errorf("propose: move %s: %w", f.From, err)
			}
			rel = append(rel, f.From)
		}
		data := f.Data
		if data == nil {
			data = []byte(f.Content)
		}
		if err := os.WriteFile(full, data, 0o644); err != nil {
			return "", fmt.Errorf("propose: %w", err)
		}
		rel = append(rel, f.Path)
	}
	if check != nil {
		if err := check(tmp, rel); err != nil {
			return "", err
		}
	}
	if _, err := run(ctx, tmp, append([]string{"add", "--"}, rel...)...); err != nil {
		return "", err
	}
	if _, err := run(ctx, tmp, "commit", "-m", message); err != nil {
		return "", err
	}
	return branch, nil
}

type Commit struct {
	Hash    string   `json:"hash"`
	Date    string   `json:"date"`
	Subject string   `json:"subject"`
	Files   []string `json:"files"`
	More    int      `json:"more_files,omitempty"`
}

const maxLogFiles = 8

// Log lists commits of the current branch since t, newest first, with the files each one touched.
func (r *Repo) Log(ctx context.Context, since time.Time, limit int) ([]Commit, error) {
	out, err := r.git(ctx, "-c", "core.quotePath=false", "log", "--since="+since.Format(time.RFC3339), fmt.Sprintf("--max-count=%d", limit), "--name-only", "--format=%x00%h%x1f%cs%x1f%s")
	if err != nil {
		return nil, err
	}
	commits := []Commit{}
	for _, block := range strings.Split(out, "\x00")[1:] {
		lines := strings.Split(strings.TrimSpace(block), "\n")
		head := strings.SplitN(lines[0], "\x1f", 3)
		if len(head) != 3 {
			continue
		}
		c := Commit{Hash: head[0], Date: head[1], Subject: head[2], Files: []string{}}
		for _, f := range lines[1:] {
			if f = strings.TrimSpace(f); f == "" {
				continue
			}
			// A seed commit touches dozens of files: the count is enough past the first few.
			if len(c.Files) == maxLogFiles {
				c.More++
				continue
			}
			c.Files = append(c.Files, f)
		}
		commits = append(commits, c)
	}
	return commits, nil
}
