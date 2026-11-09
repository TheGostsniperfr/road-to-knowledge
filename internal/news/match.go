package news

import (
	"sort"
	"strings"
	"unicode"
)

// Subject is a knowledge subject the matcher looks for in titles and summaries.
type Subject struct {
	ID    string
	Name  string
	Level int
	Tags  []string
	// Kind is "cluster" for a sub-theme name; empty for a subject.
	Kind string
}

type Match struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Level int    `json:"level"`
	Kind  string `json:"kind,omitempty"`
}

type Matcher struct {
	terms    []term
	subjects []Subject
}

type term struct {
	text string
	subj int
}

// NewMatcher indexes subject names, the parts of compound names ("Helm et Kustomize" gives
// "helm" and "kustomize") and tags. Terms under three characters ("Go", "C") would match
// ordinary words, so they only count through a longer tag such as "golang".
func NewMatcher(subjects []Subject) *Matcher {
	m := &Matcher{}
	seen := map[string]bool{}
	for i, s := range subjects {
		add := func(raw string) {
			t := normalize(raw)
			if len([]rune(strings.ReplaceAll(t, " ", ""))) < 3 && !strings.ContainsAny(t, "+#") {
				return
			}
			key := t + "\x00" + s.ID
			if t == "" || seen[key] {
				return
			}
			seen[key] = true
			m.terms = append(m.terms, term{text: t, subj: i})
		}
		add(s.Name)
		for _, part := range splitName(s.Name) {
			add(part)
		}
		for _, tag := range s.Tags {
			add(tag)
		}
	}
	m.subjects = subjects
	return m
}

func (m *Matcher) Match(text string) []Match {
	t := " " + normalize(text) + " "
	hit := map[int]bool{}
	for _, tm := range m.terms {
		if !hit[tm.subj] && strings.Contains(t, " "+tm.text+" ") {
			hit[tm.subj] = true
		}
	}
	out := make([]Match, 0, len(hit))
	for i := range hit {
		s := m.subjects[i]
		out = append(out, Match{ID: s.ID, Name: s.Name, Level: s.Level, Kind: s.Kind})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Level != out[j].Level {
			return out[i].Level > out[j].Level
		}
		return out[i].ID < out[j].ID
	})
	if len(out) > 5 {
		out = out[:5]
	}
	return out
}

func splitName(name string) []string {
	f := func(r rune) bool { return r == '/' || r == ',' || r == '(' || r == ')' || r == '&' }
	var out []string
	for _, p := range strings.FieldsFunc(name, f) {
		for _, q := range strings.Split(" "+p+" ", " et ") {
			for _, r := range strings.Split(q, " and ") {
				if r = strings.TrimSpace(r); r != "" && r != strings.TrimSpace(name) {
					out = append(out, r)
				}
			}
		}
	}
	return out
}

var folds = strings.NewReplacer("à", "a", "â", "a", "ä", "a", "á", "a", "ç", "c", "é", "e", "è", "e", "ê", "e", "ë", "e",
	"î", "i", "ï", "i", "í", "i", "ô", "o", "ö", "o", "ó", "o", "ù", "u", "û", "u", "ü", "u", "ú", "u", "ÿ", "y", "ñ", "n", "œ", "oe", "æ", "ae")

// normalize lowercases, folds accents and turns punctuation into spaces, keeping + and # so
// "C++" and "C#" stay distinct words.
func normalize(s string) string {
	s = folds.Replace(strings.ToLower(s))
	var b strings.Builder
	space := true
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '+' || r == '#' {
			b.WriteRune(r)
			space = false
		} else if !space {
			b.WriteByte(' ')
			space = true
		}
	}
	return strings.TrimSpace(b.String())
}
