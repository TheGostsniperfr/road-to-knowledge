package kb

import (
	"fmt"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// PathsDir holds learning paths: the reading order of a book or a course laid over the map (ADR-0015).
const PathsDir = "paths"

type StepStatus string

const (
	StepDone    StepStatus = "done"
	StepCurrent StepStatus = "current"
	StepTodo    StepStatus = "todo"
)

type Step struct {
	ID     string   `yaml:"id" json:"id"`
	Name   string   `yaml:"name" json:"name"`
	Covers []string `yaml:"covers,omitempty" json:"covers,omitempty"`
	URL    string   `yaml:"url,omitempty" json:"url,omitempty"`
	Page   int      `yaml:"page,omitempty" json:"page,omitempty"`
	Done   bool     `yaml:"done,omitempty" json:"done,omitempty"`
	// Extra keeps keys this version does not know, so a rewrite (restructure) never drops them.
	Extra map[string]any `yaml:",inline" json:"-"`
}

// PathEnd is the cursor of a finished path: every step is done.
const PathEnd = "end"

type Path struct {
	ID      string  `json:"id"`
	Name    string  `json:"name"`
	At      string  `json:"at,omitempty"`
	Source  *Source `json:"source,omitempty"`
	Steps   []Step  `json:"steps"`
	Summary string  `json:"summary"`
	Body    string  `json:"-"`
	File    string  `json:"-"`
}

type pathMeta struct {
	Name   string  `yaml:"name"`
	At     string  `yaml:"at"`
	Source *Source `yaml:"source"`
	Steps  []Step  `yaml:"steps"`
}

var stepID = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`)

// Status says where a step stands: everything before the cursor is done, the cursor is the
// current step unless marked done, and a later step counts as done only when marked so, since a
// course is not always followed in order. The cursor `end` marks the whole path done.
func (p *Path) Status(i int) StepStatus {
	switch {
	case p.At == PathEnd || p.Steps[i].Done || i < p.cursor():
		return StepDone
	case p.Steps[i].ID == p.At:
		return StepCurrent
	}
	return StepTodo
}

func (p *Path) cursor() int {
	for i, s := range p.Steps {
		if s.ID == p.At {
			return i
		}
	}
	return 0
}

// Next is the recommended step: the first step not done, which is the cursor when it is not done.
func (p *Path) Next() (int, bool) {
	for i := range p.Steps {
		if p.Status(i) != StepDone {
			return i, true
		}
	}
	return 0, false
}

// StepIndex finds a step by id; the end cursor sits after the last step.
func (p *Path) StepIndex(id string) int {
	if id == PathEnd {
		return len(p.Steps)
	}
	for i, s := range p.Steps {
		if s.ID == id {
			return i
		}
	}
	return -1
}

func (u *Universe) Path(id string) *Path { return u.paths[id] }

// Resolves reports whether a cover points at a live subject, or at a notion of one.
func (u *Universe) Resolves(ref string) bool {
	base, notion, hasNotion := strings.Cut(ref, "#")
	st := u.stars[base]
	if st == nil {
		return false
	}
	if !hasNotion {
		return true
	}
	for _, pl := range st.Planets {
		if pl.ID == base+"#"+notion {
			return true
		}
	}
	return false
}

// loadPaths keeps a path with a cover that resolves nowhere: the view marks it missing, so a
// typo or a restructure never hides the whole course.
func loadPaths(u *Universe, report func(string, string, ...any)) {
	u.paths = map[string]*Path{}
	files, _ := filepath.Glob(filepath.Join(u.Root, PathsDir, "*.md"))
	sort.Strings(files)
	for _, f := range files {
		id := strings.TrimSuffix(filepath.Base(f), ".md")
		if !ValidPathID(id) {
			report(f, "a path file name must be lowercase letters, digits and dashes")
			continue
		}
		var pm pathMeta
		body, ok := readNode(f, &pm, report, true)
		if !ok {
			continue
		}
		p := &Path{ID: id, Name: pm.Name, At: pm.At, Body: body, File: f, Summary: summarize(body)}
		if p.Name == "" {
			p.Name = id
		}
		if pm.Source != nil {
			if bad := pm.Source.Problems(); len(bad) > 0 {
				report(f, "%s", strings.Join(bad, "; "))
			} else {
				p.Source = pm.Source
			}
		}
		seen := map[string]bool{}
		for _, s := range pm.Steps {
			switch {
			case !stepID.MatchString(s.ID) || s.ID == PathEnd:
				report(f, "step id %q must be lowercase letters, digits and dashes, and not %q", s.ID, PathEnd)
				continue
			case seen[s.ID]:
				report(f, "step %q appears twice; only the first is kept", s.ID)
				continue
			}
			if strings.TrimSpace(s.Name) == "" {
				report(f, "step %q needs a name", s.ID)
			}
			if s.Page < 0 {
				report(f, "step %q: page cannot be negative", s.ID)
				s.Page = 0
			}
			if s.URL != "" && !strings.HasPrefix(s.URL, "https://") && !strings.HasPrefix(s.URL, "http://") {
				report(f, "step %q: url must start with http:// or https://", s.ID)
				s.URL = ""
			}
			seen[s.ID] = true
			for _, c := range s.Covers {
				if !u.Resolves(c) {
					report(f, "step %q covers %q, which is no subject or notion%s", s.ID, c, notionHint(u, c))
				}
			}
			p.Steps = append(p.Steps, s)
		}
		if p.At != "" && p.At != PathEnd && !seen[p.At] {
			report(f, "at %q is no step of this path", p.At)
			p.At = ""
		}
		u.paths[id] = p
		u.Paths = append(u.Paths, p)
	}
}

// notionHint lists the notion ids of a subject that exists, since a heading's slug is not always
// what one guesses.
func notionHint(u *Universe, ref string) string {
	base, _, hasNotion := strings.Cut(ref, "#")
	st := u.stars[base]
	if st == nil || !hasNotion || len(st.Planets) == 0 {
		return ""
	}
	ids := make([]string, len(st.Planets))
	for i, pl := range st.Planets {
		ids[i] = pl.ID
	}
	return "; its notions are " + strings.Join(ids, ", ")
}

func ValidPathID(id string) bool { return stepID.MatchString(id) }

// PathRel is the repository path of a path file, for commits and proposals.
func PathRel(id string) string { return fmt.Sprintf("%s/%s.md", PathsDir, id) }
