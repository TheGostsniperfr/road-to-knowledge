package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/TheGostsniperfr/road-to-knowledge/internal/anki"
	"github.com/TheGostsniperfr/road-to-knowledge/internal/kb"
)

// AnkiExport writes the cards of a scope as an .apkg and returns a file name for the download.
func (s *Service) AnkiExport(ctx context.Context, scope string, w io.Writer) (string, error) {
	u, _, _, _ := s.snapshot()
	notes, _, err := anki.Collect(u, scope)
	if errors.Is(err, anki.ErrScope) {
		return "", fmt.Errorf("%w: %w", ErrNotFound, err)
	}
	if err != nil {
		return "", err
	}
	if err := anki.Export(ctx, notes, w, s.now()); err != nil {
		return "", err
	}
	name := "road-to-knowledge"
	if scope != "" {
		name += "-" + kb.Slug(scope)
	}
	return name + ".apkg", nil
}

// AnkiNotes lists the cards of a scope for an AnkiConnect push, and the repeated questions it left out.
func (s *Service) AnkiNotes(scope string) ([]anki.Note, []kb.Card, error) {
	u, _, _, _ := s.snapshot()
	notes, dups, err := anki.Collect(u, scope)
	if errors.Is(err, anki.ErrScope) {
		return nil, nil, fmt.Errorf("%w: %w", ErrNotFound, err)
	}
	return notes, dups, err
}

// ErrDirty means an import would overwrite a file that has uncommitted changes.
var ErrDirty = errors.New("uncommitted changes")

// AnkiImport reads an .apkg. By default the new cards land in inbox/ on the current branch, like
// a capture. Images and sounds never go to the current branch (ADR-0004 keeps main to Markdown
// the user wrote or captured), so an import that carries media goes to proposals/anki-import,
// as does any import with propose; there, cards that came from a subject also return to that
// subject's file. The branch is returned when one was written.
func (s *Service) AnkiImport(ctx context.Context, path string, propose bool) (anki.Stats, string, error) {
	col, err := anki.Read(ctx, path)
	if errors.Is(err, anki.ErrNotAnki) || errors.Is(err, anki.ErrTooLarge) {
		return anki.Stats{}, "", fmt.Errorf("%w: %w", ErrInvalid, err)
	}
	if err != nil {
		return anki.Stats{}, "", err
	}
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	inbox, err := s.inboxFiles()
	if err != nil {
		return anki.Stats{}, "", err
	}
	u, _, _, _ := s.snapshot()
	files, st := anki.Plan(u, col, inbox, propose, s.now())
	if len(files) == 0 {
		return st, "", nil
	}
	msg := fmt.Sprintf("feat(anki): import %d cards", st.ToInbox+st.ToSubjects)
	if propose || st.MediaCopied > 0 {
		branch, err := s.Propose(ctx, "anki-import", msg, files)
		return st, branch, err
	}
	rel := make([]string, len(files))
	for i, f := range files {
		if !strings.HasPrefix(f.Path, "inbox/") || f.Data != nil {
			return st, "", fmt.Errorf("anki import: %s outside the inbox needs a proposal", f.Path)
		}
		rel[i] = f.Path
	}
	repo := s.git
	if repo != nil {
		dirty, err := repo.Dirty(ctx, rel...)
		if err != nil {
			return st, "", fmt.Errorf("anki import: %w", err)
		}
		if len(dirty) > 0 {
			return st, "", fmt.Errorf("anki import: commit or discard your changes to %s first: %w", strings.Join(dirty, ", "), ErrDirty)
		}
	}
	paths := make([]string, len(files))
	for i, f := range files {
		full := filepath.Join(s.root, filepath.FromSlash(f.Path))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			return st, "", fmt.Errorf("anki import: %w", err)
		}
		if err := os.WriteFile(full, []byte(f.Content), 0o644); err != nil {
			return st, "", fmt.Errorf("anki import: %w", err)
		}
		paths[i] = full
	}
	if repo != nil {
		if err := repo.Commit(ctx, msg, paths...); err != nil {
			return st, "", fmt.Errorf("anki import: the cards are written but not committed: %w", err)
		}
	}
	return st, "", s.Reload(ctx)
}

func (s *Service) inboxFiles() (map[string]string, error) {
	out := map[string]string{}
	entries, err := os.ReadDir(filepath.Join(s.root, "inbox"))
	if os.IsNotExist(err) {
		return out, nil
	}
	if err != nil {
		return nil, fmt.Errorf("anki import: %w", err)
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		b, err := os.ReadFile(filepath.Join(s.root, "inbox", e.Name()))
		if err != nil {
			return nil, fmt.Errorf("anki import: %w", err)
		}
		out["inbox/"+e.Name()] = string(b)
	}
	return out, nil
}
