package app_test

import (
	"context"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/TheGostsniperfr/road-to-knowledge/internal/anki"
	"github.com/TheGostsniperfr/road-to-knowledge/internal/app"
	"github.com/TheGostsniperfr/road-to-knowledge/internal/kb"
)

func gitIn(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, string(out))
	return strings.TrimSpace(string(out))
}

func ankiRepo(t *testing.T) (*app.Service, string) {
	t.Helper()
	root := t.TempDir()
	p := filepath.Join(root, "galaxies/lang/japonais/kana.md")
	require.NoError(t, os.MkdirAll(filepath.Dir(p), 0o755))
	require.NoError(t, os.WriteFile(p, []byte("---\nname: Kana\nlevel: 3\n---\nSyllabaires.\n\nあ :: a\n"), 0o644))
	gitIn(t, root, "init", "-q", "-b", "main")
	gitIn(t, root, "config", "user.email", "t@e.st")
	gitIn(t, root, "config", "user.name", "t")
	gitIn(t, root, "add", ".")
	gitIn(t, root, "commit", "-qm", "init")
	svc, err := app.New(context.Background(), root, slog.New(slog.NewTextHandler(io.Discard, nil)))
	require.NoError(t, err)
	return svc, root
}

// apkgWith exports the repo's cards after edit has changed them, as if they had been edited in Anki.
func apkgWith(t *testing.T, root string, edit func([]anki.Note) []anki.Note) string {
	t.Helper()
	u, _, err := kb.Load(root)
	require.NoError(t, err)
	notes, _, err := anki.Collect(u, "")
	require.NoError(t, err)
	path := filepath.Join(t.TempDir(), "x.apkg")
	f, err := os.Create(path)
	require.NoError(t, err)
	require.NoError(t, anki.Export(context.Background(), edit(notes), f, time.Now()))
	require.NoError(t, f.Close())
	return path
}

func TestAnkiImport_CommitsNewCardsToTheInbox_ByDefault(t *testing.T) {
	svc, root := ankiRepo(t)
	path := apkgWith(t, root, func(n []anki.Note) []anki.Note {
		return append(n, anki.Note{Deck: "Perso::Cuisine", Front: "Température du caramel", Back: "170 °C"})
	})

	st, branch, err := svc.AnkiImport(context.Background(), path, false)

	require.NoError(t, err)
	assert.Empty(t, branch)
	assert.Equal(t, 1, st.ToInbox)
	assert.Equal(t, 1, st.Known)
	assert.Equal(t, "inbox/anki-perso-cuisine.md", gitIn(t, root, "show", "--name-only", "--format=", "HEAD"))
}

func TestAnkiImport_PutsCardsBackInTheirSubjectOnAProposalBranch_WhenProposing(t *testing.T) {
	svc, root := ankiRepo(t)
	path := apkgWith(t, root, func(n []anki.Note) []anki.Note {
		n[0].CardID, n[0].Front, n[0].Back = "lang/japonais/kana:00000000", "い", "i"
		return n
	})

	_, branch, err := svc.AnkiImport(context.Background(), path, true)

	require.NoError(t, err)
	assert.Equal(t, "proposals/anki-import", branch)
	assert.Equal(t, "main", gitIn(t, root, "branch", "--show-current"))
	assert.Contains(t, gitIn(t, root, "show", branch+":galaxies/lang/japonais/kana.md"), "- い :: i")
	assert.NotContains(t, gitIn(t, root, "show", "main:galaxies/lang/japonais/kana.md"), "い")
}

func TestAnkiImport_RejectsAFileThatIsNotAnApkg(t *testing.T) {
	svc, _ := ankiRepo(t)
	p := filepath.Join(t.TempDir(), "x.apkg")
	require.NoError(t, os.WriteFile(p, []byte("nope"), 0o644))

	_, _, err := svc.AnkiImport(context.Background(), p, false)

	assert.ErrorIs(t, err, app.ErrInvalid)
}

func cuisine(n []anki.Note, cards ...[2]string) []anki.Note {
	for _, c := range cards {
		n = append(n, anki.Note{CardID: "perso:" + c[0], GUID: anki.GUID("perso:" + c[0]), Deck: "Perso::Cuisine", Front: c[0], Back: c[1]})
	}
	return n
}

func TestAnkiImport_AppendsToTheInboxFileAndSkipsCardsAlreadyThere(t *testing.T) {
	svc, root := ankiRepo(t)
	first := apkgWith(t, root, func(n []anki.Note) []anki.Note { return cuisine(n, [2]string{"Caramel", "170 °C"}) })
	second := apkgWith(t, root, func(n []anki.Note) []anki.Note {
		return cuisine(n, [2]string{"Caramel", "170 °C"}, [2]string{"Choux", "180 °C"})
	})
	_, _, err := svc.AnkiImport(context.Background(), first, false)
	require.NoError(t, err)

	st, _, err := svc.AnkiImport(context.Background(), second, false)

	require.NoError(t, err)
	assert.Equal(t, 1, st.ToInbox)
	body, err := os.ReadFile(filepath.Join(root, "inbox/anki-perso-cuisine.md"))
	require.NoError(t, err)
	assert.Equal(t, 1, strings.Count(string(body), "Caramel"))
	assert.Contains(t, string(body), "- Choux :: 180 °C")
	assert.Equal(t, "2", gitIn(t, root, "rev-list", "--count", "HEAD", "--", "inbox/anki-perso-cuisine.md"))
}

func TestAnkiImport_RefusesToOverwriteAnInboxFileWithUncommittedChanges(t *testing.T) {
	svc, root := ankiRepo(t)
	path := apkgWith(t, root, func(n []anki.Note) []anki.Note { return cuisine(n, [2]string{"Caramel", "170 °C"}) })
	inbox := filepath.Join(root, "inbox/anki-perso-cuisine.md")
	require.NoError(t, os.MkdirAll(filepath.Dir(inbox), 0o755))
	require.NoError(t, os.WriteFile(inbox, []byte("---\nsource: anki\n---\nnotes à moi\n"), 0o644))

	_, _, err := svc.AnkiImport(context.Background(), path, false)

	assert.ErrorIs(t, err, app.ErrDirty)
	body, _ := os.ReadFile(inbox)
	assert.Equal(t, "---\nsource: anki\n---\nnotes à moi\n", string(body))
}

func TestAnkiImport_SendsMediaToAProposalBranch_EvenWithoutPropose(t *testing.T) {
	svc, root := ankiRepo(t)

	st, branch, err := svc.AnkiImport(context.Background(), "../anki/testdata/anki-25.09.apkg", false)

	require.NoError(t, err)
	assert.Equal(t, 1, st.MediaCopied)
	assert.Equal(t, "proposals/anki-import", branch)
	assert.NoDirExists(t, filepath.Join(root, "assets"))
	assert.Equal(t, "init", gitIn(t, root, "log", "-1", "--format=%s", "main"))
	assert.Contains(t, gitIn(t, root, "show", "--name-only", "--format=", branch), "assets/anki/be13d62f-choux.png")
}
