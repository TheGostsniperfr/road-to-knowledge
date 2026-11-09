package kb_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/TheGostsniperfr/road-to-knowledge/internal/kb"
)

func writeFile(t *testing.T, root, rel, content string) string {
	t.Helper()
	p := filepath.Join(root, filepath.FromSlash(rel))
	require.NoError(t, os.MkdirAll(filepath.Dir(p), 0o755))
	require.NoError(t, os.WriteFile(p, []byte(content), 0o644))
	return p
}

const kana = `---
name: Kana
level: 4
links: [genki-1]
reviewed: 2026-10-01
---
Deux syllabaires phonétiques de 46 signes.

あ :: a
- か :: ka

` + "```" + `
not :: a card
` + "```" + `

## Hiragana
Signes arrondis.

## Katakana
Mots empruntés.
`

func fixture(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	writeFile(t, root, "galaxies/lang/_galaxy.md", "---\nname: Langues\nhue: 168\nshape: spiral\n---\n")
	writeFile(t, root, "galaxies/lang/japonais/_cluster.md", "---\nname: Japonais\n---\n")
	writeFile(t, root, "galaxies/lang/japonais/kana.md", kana)
	writeFile(t, root, "galaxies/books/japonais/genki-1.md", "---\nname: Genki I\nlevel: 3\n---\nManuel.\n")
	return root
}

func TestLoad_BuildsHierarchyFromDirectories(t *testing.T) {
	root := fixture(t)

	u, problems, err := kb.Load(root)

	require.NoError(t, err)
	assert.Empty(t, problems)
	s := u.Star("lang/japonais/kana")
	require.NotNil(t, s)
	assert.Equal(t, "Japonais", s.Cluster.Name)
	assert.Equal(t, "Langues", s.Cluster.Galaxy.Name)
}

func TestLoad_ExtractsCardsOutsideCodeFences(t *testing.T) {
	u, _, err := kb.Load(fixture(t))
	require.NoError(t, err)

	cards := u.Star("lang/japonais/kana").Cards

	require.Len(t, cards, 2)
	assert.Equal(t, "か", cards[1].Question)
	assert.Equal(t, "ka", cards[1].Answer)
}

func TestLoad_TurnsLevelTwoHeadingsIntoPlanets(t *testing.T) {
	u, _, err := kb.Load(fixture(t))
	require.NoError(t, err)

	planets := u.Star("lang/japonais/kana").Planets

	require.Len(t, planets, 2)
	assert.Equal(t, "lang/japonais/kana#katakana", planets[1].ID)
	assert.Equal(t, "Mots empruntés.", planets[1].Body)
}

func TestLoad_MakesWormholesSymmetric_WhenLinkUsesBareSlug(t *testing.T) {
	u, _, err := kb.Load(fixture(t))
	require.NoError(t, err)

	genki := u.Star("books/japonais/genki-1")

	assert.Equal(t, []string{"lang/japonais/kana"}, genki.Links)
}

func TestLoad_ReportsProblem_WhenLevelIsOutOfRange(t *testing.T) {
	root := fixture(t)
	writeFile(t, root, "galaxies/lang/japonais/keigo.md", "---\nname: Keigo\nlevel: 9\n---\n")

	u, problems, err := kb.Load(root)

	require.NoError(t, err)
	require.Len(t, problems, 1)
	assert.Contains(t, problems[0].Message, "level")
	assert.Equal(t, 5, u.Star("lang/japonais/keigo").Level)
}

func TestLoad_ReportsProblem_WhenLinkTargetIsMissing(t *testing.T) {
	root := fixture(t)
	writeFile(t, root, "galaxies/lang/japonais/keigo.md", "---\nname: Keigo\nlinks: [nowhere]\n---\n")

	_, problems, err := kb.Load(root)

	require.NoError(t, err)
	require.Len(t, problems, 1)
	assert.Contains(t, problems[0].Message, "nowhere")
}

func TestLoad_DerivesStateFromLevel_WhenStateIsAbsent(t *testing.T) {
	u, _, err := kb.Load(fixture(t))
	require.NoError(t, err)

	assert.Equal(t, kb.StateAcquired, u.Star("lang/japonais/kana").State)
}

func TestSetFields_KeepsBodyAndOtherKeys(t *testing.T) {
	root := fixture(t)
	path := filepath.Join(root, "galaxies/lang/japonais/kana.md")

	require.NoError(t, kb.SetFields(path, map[string]any{"level": 5, "state": "acquired"}))

	out, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.True(t, strings.HasPrefix(string(out), "---\nname: Kana\nlevel: 5\n"))
	assert.Contains(t, string(out), "state: acquired")
	assert.True(t, strings.HasSuffix(string(out), "Mots empruntés.\n"))
}

func TestSummary_SkipsHeadingsAndCards(t *testing.T) {
	u, _, err := kb.Load(fixture(t))
	require.NoError(t, err)

	assert.Equal(t, "Deux syllabaires phonétiques de 46 signes.", u.Star("lang/japonais/kana").Summary)
}
