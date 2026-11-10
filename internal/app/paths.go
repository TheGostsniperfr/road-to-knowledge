package app

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/TheGostsniperfr/road-to-knowledge/internal/kb"
)

type CoverView struct {
	ID      string   `json:"id"`
	Star    string   `json:"star"`
	Name    string   `json:"name"`
	Level   int      `json:"level"`
	State   kb.State `json:"state"`
	Missing bool     `json:"missing,omitempty"`
}

type StepView struct {
	ID     string        `json:"id"`
	Name   string        `json:"name"`
	Status kb.StepStatus `json:"status"`
	URL    string        `json:"url,omitempty"`
	Page   int           `json:"page,omitempty"`
	Covers []CoverView   `json:"covers"`
}

type PathSummary struct {
	ID      string    `json:"id"`
	Name    string    `json:"name"`
	Summary string    `json:"summary"`
	Done    int       `json:"done"`
	Total   int       `json:"total"`
	At      string    `json:"at,omitempty"`
	Next    *StepView `json:"next,omitempty"`
	// Stars lists the live subjects the path covers, so a theme or sub-theme can show its paths.
	Stars []string `json:"stars"`
}

type PathView struct {
	PathSummary
	Source *SourceView `json:"source,omitempty"`
	Steps  []StepView  `json:"steps"`
	File   string      `json:"file"`
}

// StepRef tells a fiche which course steps cover it.
type StepRef struct {
	Path     string        `json:"path"`
	PathName string        `json:"path_name"`
	Step     string        `json:"step"`
	StepName string        `json:"step_name"`
	Status   kb.StepStatus `json:"status"`
}

func (s *Service) cover(u *kb.Universe, ref string) CoverView {
	ref = s.live(u, ref)
	base, notion, hasNotion := strings.Cut(ref, "#")
	c := CoverView{ID: ref, Star: base}
	st := u.Star(base)
	if st == nil || !u.Resolves(ref) {
		c.Missing, c.Name = true, ref
		return c
	}
	c.Name, c.Level, c.State = st.Name, st.Level, st.State
	if hasNotion {
		for _, pl := range st.Planets {
			if pl.ID == base+"#"+notion {
				c.Name = st.Name + " · " + pl.Name
			}
		}
	}
	return c
}

func (s *Service) stepView(u *kb.Universe, p *kb.Path, i int) StepView {
	st := p.Steps[i]
	v := StepView{ID: st.ID, Name: st.Name, Status: p.Status(i), URL: st.URL, Page: st.Page, Covers: []CoverView{}}
	for _, c := range st.Covers {
		v.Covers = append(v.Covers, s.cover(u, c))
	}
	return v
}

func (s *Service) pathSummary(u *kb.Universe, p *kb.Path) PathSummary {
	sum := PathSummary{ID: p.ID, Name: p.Name, Summary: p.Summary, Total: len(p.Steps), At: p.At, Stars: []string{}}
	seen := map[string]bool{}
	for i, st := range p.Steps {
		if p.Status(i) == kb.StepDone {
			sum.Done++
		}
		for _, c := range st.Covers {
			c = s.live(u, c)
			base, _, _ := strings.Cut(c, "#")
			if !seen[base] && u.Resolves(c) {
				seen[base] = true
				sum.Stars = append(sum.Stars, base)
			}
		}
	}
	sort.Strings(sum.Stars)
	if i, ok := p.Next(); ok {
		next := s.stepView(u, p, i)
		sum.Next = &next
	}
	return sum
}

func (s *Service) Paths() []PathSummary {
	u, _, _, _ := s.snapshot()
	out := []PathSummary{}
	for _, p := range u.Paths {
		out = append(out, s.pathSummary(u, p))
	}
	return out
}

func (s *Service) Path(id string) (PathView, error) {
	u, _, _, _ := s.snapshot()
	p := u.Path(id)
	if p == nil {
		return PathView{}, fmt.Errorf("path %q: %w", id, ErrNotFound)
	}
	v := PathView{PathSummary: s.pathSummary(u, p), Steps: []StepView{}, File: kb.PathRel(p.ID)}
	if p.Source != nil {
		v.Source = &SourceView{Source: *p.Source, Available: p.Source.Available(s.root)}
	}
	for i := range p.Steps {
		v.Steps = append(v.Steps, s.stepView(u, p, i))
	}
	return v, nil
}

// stepsCovering lists the steps that point at a subject or at one of its notions, following
// renames like the path view does.
func (s *Service) stepsCovering(u *kb.Universe, starID string) []StepRef {
	var out []StepRef
	for _, p := range u.Paths {
		for i, st := range p.Steps {
			for _, c := range st.Covers {
				c = s.live(u, c)
				if base, _, _ := strings.Cut(c, "#"); base == starID && u.Resolves(c) {
					out = append(out, StepRef{Path: p.ID, PathName: p.Name, Step: st.ID, StepName: st.Name, Status: p.Status(i)})
					break
				}
			}
		}
	}
	return out
}

// SetPathAt moves the cursor of a path; it never changes a level (ADR-0003), the user does that.
func (s *Service) SetPathAt(ctx context.Context, id, at string) (PathView, error) {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	u, _, _, _ := s.snapshot()
	p := u.Path(id)
	if p == nil {
		return PathView{}, fmt.Errorf("path %q: %w", id, ErrNotFound)
	}
	if p.StepIndex(at) < 0 {
		return PathView{}, fmt.Errorf("step %q is not in path %q: %w", at, id, ErrInvalid)
	}
	if err := kb.SetFields(p.File, map[string]any{"at": at}); err != nil {
		return PathView{}, fmt.Errorf("set path cursor: %w", err)
	}
	s.commit(ctx, fmt.Sprintf("chore(path): %s at %s", id, at), p.File)
	if err := s.Reload(ctx); err != nil {
		return PathView{}, err
	}
	return s.Path(id)
}
