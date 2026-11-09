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
	"github.com/TheGostsniperfr/road-to-knowledge/internal/news"
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

	// writeMu serializes read-modify-write of knowledge files with their commit and reload.
	writeMu sync.Mutex

	mu       sync.RWMutex
	u        *kb.Universe
	problems []kb.Problem
	idx      *index.Index
	version  int64

	subsMu sync.Mutex
	subs   map[chan int64]struct{}

	newsOnce  sync.Once
	newsStore *news.Store
	newsErr   error
	newsMu    sync.Mutex

	sem semanticState
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
	s.reindex()
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
	w.Add(s.root)
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
			if !strings.HasSuffix(ev.Name, ".md") && !strings.HasSuffix(ev.Name, ".jsonl") && filepath.Base(ev.Name) != kb.LayoutFile && ev.Op&(fsnotify.Create|fsnotify.Remove|fsnotify.Rename) == 0 {
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
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
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
	// The source comes from an agent: one short line, quoted, so it can never add frontmatter keys.
	source = strings.Join(strings.Fields(source), " ")
	if r := []rune(source); len(r) > 80 {
		source = string(r[:80])
	}
	content := fmt.Sprintf("---\ncaptured: %s\nsource: %q\n---\n%s\n", now.Format(time.RFC3339), source, text)
	// Agents capture in bursts: two notes in the same second get distinct files instead of one overwrite.
	stamp := now.Format("2006-01-02-150405")
	for i := 1; ; i++ {
		name := stamp + ".md"
		if i > 1 {
			name = fmt.Sprintf("%s-%d.md", stamp, i)
		}
		path := filepath.Join(dir, name)
		f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
		if errors.Is(err, os.ErrExist) && i < 100 {
			continue
		}
		if err != nil {
			return "", fmt.Errorf("capture: %w", err)
		}
		_, werr := f.WriteString(content)
		if cerr := f.Close(); werr == nil {
			werr = cerr
		}
		if werr != nil {
			return "", fmt.Errorf("capture: %w", werr)
		}
		s.commit(ctx, "chore(inbox): capture from "+source, path)
		return "inbox/" + name, nil
	}
}

func (s *Service) Propose(ctx context.Context, slug, message string, files []gitx.FileChange) (string, error) {
	if s.git == nil {
		return "", fmt.Errorf("propose needs the knowledge folder to be a git repository: %w", ErrInvalid)
	}
	if len(files) == 0 {
		return "", fmt.Errorf("propose: no files: %w", ErrInvalid)
	}
	if strings.TrimSpace(message) == "" {
		return "", fmt.Errorf("propose: empty commit message, use a conventional one like \"feat(seed): japonais\": %w", ErrInvalid)
	}
	check := func(dir string, paths []string) error { return s.checkProposal(slug, dir, paths) }
	branch, err := s.git.Propose(ctx, slug, message, files, check)
	if errors.Is(err, gitx.ErrBadPath) || errors.Is(err, gitx.ErrBadDelete) || errors.Is(err, errBadProposal) || errors.Is(err, os.ErrNotExist) {
		return "", fmt.Errorf("%w: %w", ErrInvalid, err)
	}
	return branch, err
}

var errBadProposal = errors.New("proposal refused")

// subjectPathProblem rejects galaxies/ paths the loader would ignore or misread: a subject sits at
// galaxies/<theme>/<sub-theme>/<subject>.md, settings in _galaxy.md and _cluster.md.
func subjectPathProblem(p string) string {
	if !strings.HasPrefix(p, "galaxies/") {
		return ""
	}
	parts := strings.Split(p, "/")
	name := parts[len(parts)-1]
	switch {
	case len(parts) == 3 && name == "_galaxy.md":
	case len(parts) == 4 && (name == "_cluster.md" || !strings.HasPrefix(name, "_")):
	default:
		return "not a subject path: use galaxies/<theme>/<sub-theme>/<subject>.md (or _galaxy.md, _cluster.md)"
	}
	return ""
}

// checkProposal loads the proposal's tree the way the app will after the merge, so an agent learns
// about a broken file now rather than the user after merging. It also holds ADR-0003: a proposal
// never lowers a level, and an unattended triage (slug triage-*) never raises one either: only
// the user decides that they know something better.
func (s *Service) checkProposal(slug, dir string, paths []string) error {
	proposed := map[string]bool{}
	var msgs []string
	for _, p := range paths {
		proposed[p] = true
		if msg := subjectPathProblem(p); msg != "" {
			msgs = append(msgs, p+": "+msg)
		}
	}
	next, problems, err := kb.Load(dir)
	if err != nil {
		return fmt.Errorf("propose: load proposal: %w", err)
	}
	for _, p := range problems {
		if proposed[p.Path] {
			msgs = append(msgs, p.Path+": "+p.Message)
		}
	}
	cur, _, _, _ := s.snapshot()
	for _, st := range next.Stars() {
		rel, _ := filepath.Rel(dir, st.Path)
		if !proposed[filepath.ToSlash(rel)] {
			continue
		}
		old := cur.Star(st.ID)
		if old != nil && st.Level < old.Level {
			msgs = append(msgs, fmt.Sprintf("%s: level %d is below the current %d; levels never drop through a proposal (ADR-0003), keep %d", filepath.ToSlash(rel), st.Level, old.Level, old.Level))
		}
		was := 0
		if old != nil {
			was = old.Level
		}
		if strings.HasPrefix(slug, "triage-") && st.Level > was {
			msgs = append(msgs, fmt.Sprintf("%s: a triage cannot raise a level (%d to %d); keep %d and let the user decide", filepath.ToSlash(rel), was, st.Level, was))
		}
	}
	if len(msgs) > 0 {
		return fmt.Errorf("%w, nothing committed: %s", errBadProposal, strings.Join(msgs, "; "))
	}
	return nil
}

// SetLayout replaces the pinned galaxy positions. Pins for galaxies that no longer exist are refused, not kept.
func (s *Service) SetLayout(ctx context.Context, pins map[string]kb.Pin) (UniverseView, error) {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	u, _, _, _ := s.snapshot()
	for id, p := range pins {
		if u.Galaxy(id) == nil {
			return UniverseView{}, fmt.Errorf("pin for unknown galaxy %q: %w", id, ErrInvalid)
		}
		if !p.Valid() {
			return UniverseView{}, fmt.Errorf("pin for %q out of range: %w", id, ErrInvalid)
		}
	}
	path, err := kb.WriteLayout(s.root, kb.Layout{Galaxies: pins})
	if err != nil {
		return UniverseView{}, err
	}
	s.commit(ctx, "chore(layout): pin galaxies", path)
	if err := s.Reload(ctx); err != nil {
		return UniverseView{}, err
	}
	return s.Universe(), nil
}

// GalaxySettings carries the fields of _galaxy.md the app may edit; nil means unchanged.
type GalaxySettings struct {
	ReviewAfter *int          `json:"review_after_days"`
	Discovery   *kb.Discovery `json:"discovery"`
}

func (s *Service) SetGalaxy(ctx context.Context, id string, in GalaxySettings) (GalaxyView, error) {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	u, _, _, _ := s.snapshot()
	g := u.Galaxy(id)
	if g == nil {
		return GalaxyView{}, fmt.Errorf("galaxy %q: %w", id, ErrNotFound)
	}
	fields := map[string]any{}
	if in.ReviewAfter != nil {
		if *in.ReviewAfter < 1 || *in.ReviewAfter > 3650 {
			return GalaxyView{}, fmt.Errorf("review_after_days %d not in 1..3650: %w", *in.ReviewAfter, ErrInvalid)
		}
		fields["review_after_days"] = *in.ReviewAfter
	}
	if in.Discovery != nil {
		if !in.Discovery.Valid() {
			return GalaxyView{}, fmt.Errorf("discovery shares must sum to 100: %w", ErrInvalid)
		}
		fields["discovery"] = *in.Discovery
	}
	if len(fields) == 0 {
		return GalaxyView{}, fmt.Errorf("nothing to change: %w", ErrInvalid)
	}
	if _, err := os.Stat(g.Path); os.IsNotExist(err) {
		if err := os.WriteFile(g.Path, nil, 0o644); err != nil {
			return GalaxyView{}, fmt.Errorf("set galaxy: %w", err)
		}
	}
	if err := kb.SetFields(g.Path, fields); err != nil {
		return GalaxyView{}, err
	}
	s.commit(ctx, fmt.Sprintf("chore(knowledge): %s settings", g.Name), g.Path)
	if err := s.Reload(ctx); err != nil {
		return GalaxyView{}, err
	}
	for _, gv := range s.Universe().Galaxies {
		if gv.ID == id {
			return gv, nil
		}
	}
	return GalaxyView{}, fmt.Errorf("galaxy %q vanished after write: %w", id, ErrNotFound)
}
