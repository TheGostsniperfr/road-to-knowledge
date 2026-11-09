package kb

import "strings"

// Section is one "## heading" block of a subject body, heading line included, so a notion and
// the cards written under it move together.
type Section struct {
	Name string
	Slug string
	Text string
}

// Sections splits a body into the part before the first notion (summary and loose cards) and
// the notions, the same way extractPlanets reads them: headings inside code fences do not count.
func Sections(body string) (preamble string, sections []Section) {
	var pre []string
	var cur *Section
	var buf []string
	flush := func() {
		if cur != nil {
			cur.Text = strings.TrimRight(strings.Join(buf, "\n"), "\n") + "\n"
			sections = append(sections, *cur)
		}
	}
	inFence := false
	for _, line := range strings.Split(body, "\n") {
		if fence.MatchString(line) {
			inFence = !inFence
		}
		if !inFence && strings.HasPrefix(line, "## ") {
			flush()
			name := strings.TrimSpace(strings.TrimPrefix(line, "## "))
			cur, buf = &Section{Name: name, Slug: Slug(name)}, []string{line}
			continue
		}
		if cur != nil {
			buf = append(buf, line)
		} else {
			pre = append(pre, line)
		}
	}
	flush()
	return strings.TrimRight(strings.Join(pre, "\n"), "\n"), sections
}

// Summarize returns the first paragraph of a body, as the loader does.
func Summarize(body string) string { return summarize(body) }

// CardsIn lists the cards of a body as they would be read for the given subject id.
func CardsIn(starID, body string) []Card { return extractCards(starID, body) }
