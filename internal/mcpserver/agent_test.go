package mcpserver_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func gitIn(t *testing.T, root string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = root
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, string(out))
	return string(out)
}

func savedPath(out string) string {
	return out[strings.Index(out, "inbox/") : strings.Index(out, ".md")+3]
}

func TestPropose_RefusesMalformedFile_AndLeavesNoBranch(t *testing.T) {
	cs, root := session(t)

	out, isErr := call(t, cs, "propose", map[string]any{"slug": "bad", "message": "feat: bad", "files": []map[string]string{{"path": "galaxies/lang/japonais/bad.md", "content": "---\nname: Bad\nlevel: 9\n---\n"}}})

	assert.True(t, isErr, out)
	assert.Contains(t, out, "galaxies/lang/japonais/bad.md")
	assert.NotContains(t, gitIn(t, root, "branch", "--list", "proposals/*"), "proposals/bad")
}

func TestPropose_RefusesToLowerALevel(t *testing.T) {
	cs, _ := session(t)

	out, isErr := call(t, cs, "propose", map[string]any{"slug": "down", "message": "feat: down", "files": []map[string]string{{"path": "galaxies/lang/japonais/kana.md", "content": "---\nname: Kana\nlevel: 2\n---\nDeux syllabaires.\n"}}})

	assert.True(t, isErr, out)
	assert.Contains(t, out, "ADR-0003")
}

func TestPropose_DeletesPlacedInboxNote_OnTheProposalBranchOnly(t *testing.T) {
	cs, root := session(t)
	saved, _ := call(t, cs, "capture", map[string]any{"text": "keigo: politesse", "source": "test"})
	note := savedPath(saved)

	out, isErr := call(t, cs, "propose", map[string]any{"slug": "triage", "message": "feat: triage", "files": []map[string]any{
		{"path": "galaxies/lang/japonais/keigo.md", "content": "---\nname: Keigo\n---\nPolitesse.\n"},
		{"path": note, "delete": true},
	}})

	require.False(t, isErr, out)
	assert.FileExists(t, filepath.Join(root, note))
	assert.Contains(t, gitIn(t, root, "show", "--name-status", "--format=", "proposals/triage"), "D\t"+note)
}

func TestPropose_RefusesToDeleteASubject(t *testing.T) {
	cs, _ := session(t)

	out, isErr := call(t, cs, "propose", map[string]any{"slug": "rm", "message": "feat: rm", "files": []map[string]any{{"path": "galaxies/lang/japonais/kana.md", "delete": true}}})

	assert.True(t, isErr)
	assert.Contains(t, out, "inbox/")
}

func TestPropose_RefusesEmptyMessage(t *testing.T) {
	cs, _ := session(t)

	out, isErr := call(t, cs, "propose", map[string]any{"slug": "x", "message": " ", "files": []map[string]string{{"path": "inbox/x.md", "content": "x"}}})

	assert.True(t, isErr)
	assert.Contains(t, out, "conventional")
}

func TestCapture_KeepsBothNotes_WhenTwoArriveInTheSameSecond(t *testing.T) {
	cs, root := session(t)

	a, _ := call(t, cs, "capture", map[string]any{"text": "un"})
	b, _ := call(t, cs, "capture", map[string]any{"text": "deux"})

	assert.NotEqual(t, a, b)
	entries, err := os.ReadDir(filepath.Join(root, "inbox"))
	require.NoError(t, err)
	assert.Len(t, entries, 2)
}

func TestInbox_KeepsCaptureOrder_WithinTheSameSecond(t *testing.T) {
	cs, _ := session(t)
	for _, text := range []string{"un", "deux", "trois"} {
		call(t, cs, "capture", map[string]any{"text": text})
	}

	out, _ := call(t, cs, "inbox", map[string]any{})

	assert.Less(t, strings.Index(out, `"un"`), strings.Index(out, `"deux"`), out)
	assert.Less(t, strings.Index(out, `"deux"`), strings.Index(out, `"trois"`), out)
}

func TestPropose_RefusesSubjectAtTheWrongDepth(t *testing.T) {
	cs, _ := session(t)

	out, isErr := call(t, cs, "propose", map[string]any{"slug": "deep", "message": "feat: deep", "files": []map[string]string{{"path": "galaxies/lang/kana.md", "content": "---\nname: Kana\n---\n"}}})

	assert.True(t, isErr)
	assert.Contains(t, out, "galaxies/<theme>/<sub-theme>/<subject>.md")
}

func TestPropose_RefusesLevelRaise_WhenSlugIsATriage(t *testing.T) {
	cs, _ := session(t)

	out, isErr := call(t, cs, "propose", map[string]any{"slug": "triage-2026-11-09", "message": "feat: triage", "files": []map[string]string{{"path": "galaxies/tech/k8s/gvisor.md", "content": "---\nname: gVisor\nlevel: 2\n---\nNoyau.\n"}}})

	assert.True(t, isErr)
	assert.Contains(t, out, "triage cannot raise")
}

func TestPropose_AllowsLevelRaise_OutsideTriage(t *testing.T) {
	cs, _ := session(t)

	out, isErr := call(t, cs, "propose", map[string]any{"slug": "teach-back-2026-11-09", "message": "feat: up", "files": []map[string]string{{"path": "galaxies/tech/k8s/gvisor.md", "content": "---\nname: gVisor\nlevel: 2\n---\nNoyau.\n"}}})

	assert.False(t, isErr, out)
}

func TestCapture_CannotInjectFrontmatter_ThroughSource(t *testing.T) {
	cs, root := session(t)

	out, _ := call(t, cs, "capture", map[string]any{"text": "note", "source": "x\nlevel: 5\n---\nbody"})

	b, err := os.ReadFile(filepath.Join(root, savedPath(out)))
	require.NoError(t, err)
	assert.NotContains(t, string(b), "\nlevel: 5\n", string(b))
}

func TestErrors_SuggestTheNextCall_WhenIdIsUnknown(t *testing.T) {
	cs, _ := session(t)

	out, isErr := call(t, cs, "node", map[string]any{"id": "nope"})

	assert.True(t, isErr)
	assert.Contains(t, out, "search(")
}

func TestFragments_ListsSections_WhenSectionIsUnknown(t *testing.T) {
	cs, _ := session(t)

	out, isErr := call(t, cs, "fragments", map[string]any{"id": "lang/japonais/kana", "section": "kanji"})

	assert.True(t, isErr)
	assert.Contains(t, out, "sections: Hiragana, Katakana")
}

func TestFragments_ReturnsWholeFile_WhenRaw(t *testing.T) {
	cs, _ := session(t)

	out, isErr := call(t, cs, "fragments", map[string]any{"id": "lang/japonais/kana", "raw": true})

	assert.False(t, isErr, out)
	assert.True(t, strings.HasPrefix(out, "---\nname: Kana\nlevel: 4\n---\n"), out)
}

func TestDue_ListsNewCardsWithAnswers(t *testing.T) {
	cs, _ := session(t)

	out, isErr := call(t, cs, "due", map[string]any{"scope": "lang"})

	assert.False(t, isErr, out)
	assert.Contains(t, out, `"q":"あ","a":"a"`)
}

func TestAtRisk_SkipsSubjectsBelowLevelTwo(t *testing.T) {
	cs, _ := session(t)

	out, isErr := call(t, cs, "at_risk", map[string]any{})

	assert.False(t, isErr, out)
	assert.Contains(t, out, "lang/japonais/kana")
	assert.NotContains(t, out, "gvisor")
}

func TestRecent_ShowsCommitsOfTheWindow(t *testing.T) {
	cs, _ := session(t)
	call(t, cs, "capture", map[string]any{"text": "note", "source": "test"})

	out, isErr := call(t, cs, "recent", map[string]any{"days": 1})

	assert.False(t, isErr, out)
	assert.Contains(t, out, "chore(inbox): capture from test")
}

func TestInbox_ListsCapturedNotesWithTheirFile(t *testing.T) {
	cs, _ := session(t)
	call(t, cs, "capture", map[string]any{"text": "lire la doc de Cilium"})

	out, isErr := call(t, cs, "inbox", map[string]any{})

	assert.False(t, isErr, out)
	assert.Contains(t, out, `"total":1`)
	assert.Contains(t, out, "lire la doc de Cilium")
}
