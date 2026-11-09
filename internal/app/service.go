// Package app holds the loaded knowledge and the views shared by the HTTP API and the MCP server,
// so what the UI shows under "Ce que l'IA reçoit" is exactly what an agent gets.
package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"

	"github.com/TheGostsniperfr/road-to-knowledge/internal/gitx"
	"github.com/TheGostsniperfr/road-to-knowledge/internal/index"
	"github.com/TheGostsniperfr/road-to-knowledge/internal/kb"
	"github.com/TheGostsniperfr/road-to-knowledge/internal/review"
)

var (
	ErrNotFound = errors.New("not found")
	ErrInvalid  = errors.New("invalid input")
)

type Service struct {
	root string
	log  *slog.Logger
	now  func() time.Time
	git  *gitx.Repo
	book *review.Book

	mu       sync.RWMutex
	u        *kb.Universe
	problems []kb.Problem
	idx      *index.Index
	version  int64

	subsMu sync.Mutex
	subs   map[chan int64]struct{}
}

func New(ctx context.Context, root string, log *slog.Logger) (*Service, error) {
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("app: %w", err)
	}
	if st, err := os.Stat(filepath.Join(abs, kb.GalaxiesDir)); err != nil || !st.IsDir() {
		return nil, fmt.Errorf("app: %s has no %s/ folder; run `rtk init %s` first", abs, kb.GalaxiesDir, root)
	}
	book, err := review.Open(abs)
	if err != nil {
		return nil, err
	}
	s := &Service{root: abs, log: log, now: time.Now, git: gitx.Open(ctx, abs), book: book, subs: map[chan int64]struct{}{}}
	if err := s.Reload(ctx); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *Service) Root() string { return s.root }

func (s *Service) Reload(ctx context.Context) error {
	u, problems, err := kb.Load(s.root)
	if err != nil {
		return err
	}
	idx, err := index.Build(ctx, u)
	if err != nil {
		return err
	}
	if err := s.book.Reload(); err != nil {
		return err
	}
	s.mu.Lock()
	old := s.idx
	s.u, s.problems, s.idx = u, problems, idx
	s.version++
	v := s.version
	s.mu.Unlock()
	if old != nil {
		old.Close()
	}
	for _, p := range problems {
		s.log.Warn("knowledge problem", "path", p.Path, "msg", p.Message)
	}
	s.notify(v)
	return nil
}

// Subscribe returns a channel that receives the new version after each reload.
func (s *Service) Subscribe() (<-chan int64, func()) {
	ch := make(chan int64, 1)
	s.subsMu.Lock()
	s.subs[ch] = struct{}{}
	s.subsMu.Unlock()
	return ch, func() {
		s.subsMu.Lock()
		delete(s.subs, ch)
		s.subsMu.Unlock()
	}
}

func (s *Service) notify(v int64) {
	s.subsMu.Lock()
	defer s.subsMu.Unlock()
	for ch := range s.subs {
		select {
		case ch <- v:
		default:
		}
	}
}

// Watch reloads when Markdown files or the review log change, debounced so editor saves coalesce.
func (s *Service) Watch(ctx context.Context) error {
	w, err := fsnotify.NewWatcher()
	if err != nil {
		return fmt.Errorf("watch: %w", err)
	}
	defer w.Close()
	addTree := func(dir string) {
		filepath.WalkDir(dir, func(p string, d os.DirEntry, err error) error {
			if err == nil && d.IsDir() {
				if strings.HasPrefix(d.Name(), ".") && p != dir {
					return filepath.SkipDir
				}
				w.Add(p)
			}
			return nil
		})
	}
	addTree(filepath.Join(s.root, kb.GalaxiesDir))
	os.MkdirAll(filepath.Dir(s.book.Path()), 0o755)
	w.Add(filepath.Dir(s.book.Path()))
	var timer *time.Timer
	fire := make(chan struct{}, 1)
	for {
		select {
		case <-ctx.Done():
			return nil
		case ev := <-w.Events:
			if ev.Op&fsnotify.Create != 0 {
				if st, err := os.Stat(ev.Name); err == nil && st.IsDir() {
					addTree(ev.Name)
				}
			}
			if !strings.HasSuffix(ev.Name, ".md") && !strings.HasSuffix(ev.Name, ".jsonl") && ev.Op&(fsnotify.Create|fsnotify.Remove|fsnotify.Rename) == 0 {
				continue
			}
			if timer != nil {
				timer.Stop()
			}
			timer = time.AfterFunc(300*time.Millisecond, func() {
				select {
				case fire <- struct{}{}:
				default:
				}
			})
		case <-fire:
			if err := s.Reload(ctx); err != nil {
				s.log.Error("reload failed", "err", err)
			}
		case err := <-w.Errors:
			s.log.Warn("watch error", "err", err)
		}
	}
}

func (s *Service) snapshot() (*kb.Universe, *index.Index, []kb.Problem, int64) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.u, s.idx, s.problems, s.version
}

func (s *Service) commit(ctx context.Context, msg string, paths ...string) {
	if s.git == nil {
		return
	}
	if err := s.git.Commit(ctx, msg, paths...); err != nil {
		s.log.Error("commit failed", "err", err, "msg", msg)
	}
}

// SetMeta changes the level or state of a subject. It never lowers a level on its own: only an explicit call does.
func (s *Service) SetMeta(ctx context.Context, id string, level *int, state *kb.State) (StarView, error) {
	u, _, _, _ := s.snapshot()
	st := u.Star(id)
	if st == nil {
		return StarView{}, fmt.Errorf("star %q: %w", id, ErrNotFound)
	}
	fields := map[string]any{"reviewed": s.now().Format("2006-01-02")}
	var parts []string
	if level != nil {
		if *level < 0 || *level > 5 {
			return StarView{}, fmt.Errorf("level %d: %w", *level, ErrInvalid)
		}
		fields["level"] = *level
		parts = append(parts, fmt.Sprintf("level %d", *level))
	}
	if state != nil {
		if !state.Valid() {
			return StarView{}, fmt.Errorf("state %q: %w", *state, ErrInvalid)
		}
		fields["state"] = string(*state)
		parts = append(parts, string(*state))
	}
	if len(parts) == 0 {
		parts = append(parts, "reviewed")
	}
	if err := kb.SetFields(st.Path, fields); err != nil {
		return StarView{}, err
	}
	s.commit(ctx, fmt.Sprintf("chore(knowledge): %s %s", st.Name, strings.Join(parts, ", ")), st.Path)
	if err := s.Reload(ctx); err != nil {
		return StarView{}, err
	}
	u, _, _, _ = s.snapshot()
	return s.starView(u, u.Star(id)), nil
}

// Capture drops a note in inbox/ for the evening triage.
func (s *Service) Capture(ctx context.Context, text, source string) (string, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return "", fmt.Errorf("empty capture: %w", ErrInvalid)
	}
	now := s.now()
	dir := filepath.Join(s.root, "inbox")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("capture: %w", err)
	}
	name := now.Format("2006-01-02-150405") + ".md"
	path := filepath.Join(dir, name)
	content := fmt.Sprintf("---\ncaptured: %s\nsource: %s\n---\n%s\n", now.Format(time.RFC3339), source, text)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return "", fmt.Errorf("capture: %w", err)
	}
	s.commit(ctx, "chore(inbox): capture from "+source, path)
	return "inbox/" + name, nil
}

func (s *Service) Propose(ctx context.Context, slug, message string, files []gitx.FileChange) (string, error) {
	if s.git == nil {
		return "", fmt.Errorf("propose needs the knowledge folder to be a git repository: %w", ErrInvalid)
	}
	if len(files) == 0 {
		return "", fmt.Errorf("propose: no files: %w", ErrInvalid)
	}
	branch, err := s.git.Propose(ctx, slug, message, files)
	if errors.Is(err, gitx.ErrBadPath) {
		return "", fmt.Errorf("%w: %w", ErrInvalid, err)
	}
	return branch, err
}
