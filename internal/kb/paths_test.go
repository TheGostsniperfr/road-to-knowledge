package kb_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/TheGostsniperfr/road-to-knowledge/internal/kb"
)

const genkiPath = `---
name: Genki I
at: l2
source:
  title: Genki I
steps:
  - id: l1
    name: Leçon 1
    covers: [lang/japonais/kana#hiragana]
  - id: l2
    name: Leçon 2
    covers: [lang/japonais/kana]
    page: 40
  - id: l3
    name: Leçon 3
  - id: l4
    name: Leçon 4
    done: true
---
Le manuel dans l'ordre.
`

func TestLoad_ReadsPathStepsInOrder(t *testing.T) {
	root := fixture(t)
	writeFile(t, root, "paths/genki-1.md", genkiPath)

	u, problems, err := kb.Load(root)

	require.NoError(t, err)
	require.Empty(t, problems)
	p := u.Path("genki-1")
	require.NotNil(t, p)
	assert.Equal(t, []string{"l1", "l2", "l3", "l4"}, stepIDs(p))
}

func TestPath_Status_MarksStepsBeforeTheCursorDoneAndLaterOnesTodo(t *testing.T) {
	root := fixture(t)
	writeFile(t, root, "paths/genki-1.md", genkiPath)
	u, _, err := kb.Load(root)
	require.NoError(t, err)
	p := u.Path("genki-1")

	got := []kb.StepStatus{p.Status(0), p.Status(1), p.Status(2), p.Status(3)}

	assert.Equal(t, []kb.StepStatus{kb.StepDone, kb.StepCurrent, kb.StepTodo, kb.StepDone}, got)
}

func TestPath_Next_IsTheCursorOrTheFirstStepNotDone(t *testing.T) {
	tests := []struct {
		name, at string
		want     string
		ok       bool
	}{
		{"cursor is the next step", "l3", "l3", true},
		{"not started begins at the first step not done", "", "l2", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := &kb.Path{At: tt.at, Steps: []kb.Step{{ID: "l1", Done: true}, {ID: "l2"}, {ID: "l3"}, {ID: "l4"}}}

			i, ok := p.Next()

			assert.Equal(t, tt.ok, ok)
			assert.Equal(t, tt.want, p.Steps[i].ID)
		})
	}
}

func TestPath_Next_MovesPastACursorMarkedDone(t *testing.T) {
	p := &kb.Path{At: "l1", Steps: []kb.Step{{ID: "l1", Done: true}, {ID: "l2"}}}

	i, ok := p.Next()

	require.True(t, ok)
	assert.Equal(t, "l2", p.Steps[i].ID)
}

func TestPath_Status_MarksEveryStepDone_WhenTheCursorIsAtTheEnd(t *testing.T) {
	p := &kb.Path{At: kb.PathEnd, Steps: []kb.Step{{ID: "l1"}, {ID: "l2"}}}

	_, ok := p.Next()

	assert.False(t, ok)
	assert.Equal(t, kb.StepDone, p.Status(1))
}

func TestPath_Next_ReportsNothingLeft_WhenEveryStepIsDone(t *testing.T) {
	p := &kb.Path{At: "", Steps: []kb.Step{{ID: "l1", Done: true}, {ID: "l2", Done: true}}}

	_, ok := p.Next()

	assert.False(t, ok)
}

func TestLoad_ReportsPathProblems_AndKeepsThePath(t *testing.T) {
	tests := []struct{ name, file, want string }{
		{"cover to nowhere", "---\nname: X\nsteps:\n  - id: a\n    name: A\n    covers: [lang/japonais/nope]\n---\n", `covers "lang/japonais/nope"`},
		{"unknown notion", "---\nname: X\nsteps:\n  - id: a\n    name: A\n    covers: [lang/japonais/kana#kanji]\n---\n", `covers "lang/japonais/kana#kanji"`},
		{"cursor on no step", "---\nname: X\nat: b\nsteps:\n  - id: a\n    name: A\n---\n", `at "b" is no step`},
		{"duplicate step", "---\nname: X\nsteps:\n  - id: a\n    name: A\n  - id: a\n    name: B\n---\n", `step "a" appears twice`},
		{"bad step id", "---\nname: X\nsteps:\n  - id: Day 1\n    name: A\n---\n", `step id "Day 1"`},
		{"reserved step id", "---\nname: X\nsteps:\n  - id: end\n    name: A\n---\n", `step id "end"`},
		{"script url", "---\nname: X\nsteps:\n  - id: a\n    name: A\n    url: javascript:alert(1)\n---\n", "url must start with http"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := fixture(t)
			writeFile(t, root, "paths/x.md", tt.file)

			u, problems, err := kb.Load(root)

			require.NoError(t, err)
			require.Len(t, problems, 1)
			assert.Equal(t, "paths/x.md", problems[0].Path)
			assert.Contains(t, problems[0].Message, tt.want)
			assert.NotNil(t, u.Path("x"))
		})
	}
}

func stepIDs(p *kb.Path) []string {
	var out []string
	for _, s := range p.Steps {
		out = append(out, s.ID)
	}
	return out
}

func TestLoad_ReportsAPathFileWithABadName(t *testing.T) {
	root := fixture(t)
	writeFile(t, root, "paths/_draft.md", "---\nname: X\n---\n")

	u, problems, err := kb.Load(root)

	require.NoError(t, err)
	require.Len(t, problems, 1)
	assert.Empty(t, u.Paths)
}
