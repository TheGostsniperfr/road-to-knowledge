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
	notes, err := anki.Collect(u, scope)
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

// AnkiNotes lists the cards of a scope for an AnkiConnect push.
func (s *Service) AnkiNotes(scope string) ([]anki.Note, error) {
	u, _, _, _ := s.snapshot()
	notes, err := anki.Collect(u, scope)
	if errors.Is(err, anki.ErrScope) {
		return nil, fmt.Errorf("%w: %w", ErrNotFound, err)
	}
	return notes, err
}

// AnkiImport reads an .apkg. By default the new cards land in inbox/ on the current branch, like
// a capture. With propose they go to proposals/anki-import, and cards that came from a subject
// return to that subject's file there (ADR-0004: nothing but the inbox is written on main).
func (s *Service) AnkiImport(ctx context.Context, path string, propose bool) (anki.Stats, string, error) {
	col, err := anki.Read(ctx, path)
	if errors.Is(err, anki.ErrNotAnki) {
		return anki.Stats{}, "", fmt.Errorf("%w: %w", ErrInvalid, err)
	}
	if err != nil {
		return anki.Stats{}, "", err
	}
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	u, _, _, _ := s.snapshot()
	files, st := anki.Plan(u, col, propose, s.now())
	if len(files) == 0 {
		return st, "", nil
	}
	msg := fmt.Sprintf("feat(anki): import %d cards", st.ToInbox+st.ToSubjects)
	if propose {
		branch, err := s.Propose(ctx, "anki-import", msg, files)
		return st, branch, err
	}
	paths := make([]string, 0, len(files))
	for _, f := range files {
		if strings.HasPrefix(f.Path, kb.GalaxiesDir+"/") {
			return st, "", fmt.Errorf("anki import: %s outside the inbox needs --propose", f.Path)
		}
		full := filepath.Join(s.root, filepath.FromSlash(f.Path))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			return st, "", fmt.Errorf("anki import: %w", err)
		}
		data := f.Data
		if data == nil {
			data = []byte(f.Content)
		}
		if err := os.WriteFile(full, data, 0o644); err != nil {
			return st, "", fmt.Errorf("anki import: %w", err)
		}
		paths = append(paths, full)
	}
	s.commit(ctx, msg, paths...)
	return st, "", s.Reload(ctx)
}
