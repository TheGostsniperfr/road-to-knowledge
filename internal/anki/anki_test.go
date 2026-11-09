package anki_test

import (
	"archive/zip"
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/klauspost/compress/zstd"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	_ "modernc.org/sqlite"

	"github.com/TheGostsniperfr/road-to-knowledge/internal/anki"
	"github.com/TheGostsniperfr/road-to-knowledge/internal/kb"
)

var now = time.Date(2026, 11, 9, 10, 0, 0, 0, time.UTC)

func write(t *testing.T, root, rel, content string) {
	t.Helper()
	p := filepath.Join(root, rel)
	require.NoError(t, os.MkdirAll(filepath.Dir(p), 0o755))
	require.NoError(t, os.WriteFile(p, []byte(content), 0o644))
}

func universe(t *testing.T) *kb.Universe {
	t.Helper()
	root := t.TempDir()
	write(t, root, "galaxies/langues/_galaxy.md", "---\nname: Langues\n---\n")
	write(t, root, "galaxies/langues/japonais/_cluster.md", "---\nname: Japonais\n---\n")
	write(t, root, "galaxies/langues/japonais/kana.md", "---\nname: Kana\nlevel: 3\ntags: [writing system]\n---\nLes syllabaires.\n\nあ :: a\n- か :: ka\n")
	write(t, root, "galaxies/tech/k8s/cilium.md", "---\nname: Cilium\nlevel: 4\n---\neBPF.\n\nQui remplace kube-proxy ? :: Cilium en mode kube-proxy replacement\n")
	u, problems, err := kb.Load(root)
	require.NoError(t, err)
	require.Empty(t, problems)
	return u
}

func exportFile(t *testing.T, notes []anki.Note) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "out.apkg")
	f, err := os.Create(path)
	require.NoError(t, err)
	require.NoError(t, anki.Export(context.Background(), notes, f, now))
	require.NoError(t, f.Close())
	return path
}

func TestExport_RoundTripKeepsQuestionsAnswersAndGUIDs(t *testing.T) {
	u := universe(t)
	notes, _, err := anki.Collect(u, "")
	require.NoError(t, err)

	col, err := anki.Read(context.Background(), exportFile(t, notes))

	require.NoError(t, err)
	type qa struct{ guid, deck, q, a string }
	var want, got []qa
	for _, n := range notes {
		want = append(want, qa{n.GUID, n.Deck, n.Front, n.Back})
	}
	var st anki.Stats
	for _, n := range col.Notes {
		c := anki.Cards(n, &st)
		require.Len(t, c, 1)
		got = append(got, qa{n.GUID, c[0].Deck, c[0].Q, c[0].A})
	}
	assert.ElementsMatch(t, want, got)
}

func TestExport_GivesTheSameGUIDsEveryTime(t *testing.T) {
	u := universe(t)
	guids := func() []string {
		notes, _, err := anki.Collect(u, "langues")
		require.NoError(t, err)
		col, err := anki.Read(context.Background(), exportFile(t, notes))
		require.NoError(t, err)
		var out []string
		for _, n := range col.Notes {
			out = append(out, n.GUID)
		}
		return out
	}

	first := guids()

	assert.ElementsMatch(t, first, guids())
	assert.Len(t, first, 2)
}

func TestCollect_LimitsToTheScopeAndNestsDecks(t *testing.T) {
	tests := []struct {
		name, scope string
		want        int
	}{
		{"universe", "", 3},
		{"galaxy", "langues", 2},
		{"sub-theme", "tech/k8s", 1},
		{"subject", "langues/japonais/kana", 2},
	}
	u := universe(t)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			notes, _, err := anki.Collect(u, tt.scope)

			require.NoError(t, err)
			assert.Len(t, notes, tt.want)
		})
	}
	notes, _, _ := anki.Collect(u, "langues")
	assert.Equal(t, "RtK::Langues::Japonais", notes[0].Deck)
}

func TestCollect_RejectsAnUnknownScope(t *testing.T) {
	_, _, err := anki.Collect(universe(t), "nope")

	assert.ErrorIs(t, err, anki.ErrScope)
}

// legacyApkg builds a schema 11 package with one Basic note, one cloze note and one image.
func legacyApkg(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "collection.anki2")
	db, err := sql.Open("sqlite", dbPath)
	require.NoError(t, err)
	_, err = db.Exec(`CREATE TABLE col (models text, decks text);
		CREATE TABLE notes (id integer, guid text, mid integer, tags text, flds text);
		CREATE TABLE cards (nid integer, did integer, ord integer);`)
	require.NoError(t, err)
	models := `{"10": {"name": "Basic", "flds": [{"name": "Front", "ord": 0}, {"name": "Back", "ord": 1}]},
		"20": {"name": "Cloze", "flds": [{"name": "Text", "ord": 0}, {"name": "Back Extra", "ord": 1}]}}`
	decks := `{"1": {"name": "Default"}, "5": {"name": "Japonais::Vocabulaire"}}`
	_, err = db.Exec(`INSERT INTO col VALUES (?, ?)`, models, decks)
	require.NoError(t, err)
	_, err = db.Exec(`INSERT INTO notes VALUES
		(1, 'g1', 10, ' vocab ', ?),
		(2, 'g2', 20, '', ?)`,
		"<div><b>猫</b>&nbsp;(neko)</div><img src=\"cat.png\">\x1fchat<br>animal [sound:neko.mp3]",
		"Tokyo est la {{c1::capitale}} du {{c2::Japon::pays}}\x1f")
	require.NoError(t, err)
	_, err = db.Exec(`INSERT INTO cards VALUES (1, 5, 0), (2, 5, 0), (2, 5, 1)`)
	require.NoError(t, err)
	require.NoError(t, db.Close())
	col, err := os.ReadFile(dbPath)
	require.NoError(t, err)
	return zipFiles(t, map[string][]byte{
		"collection.anki2": col,
		"media":            []byte(`{"0": "cat.png", "1": "neko.mp3", "2": "evil.svg"}`),
		"0":                []byte("\x89PNG-cat"),
		"1":                []byte("ID3-neko"),
		"2":                []byte("<svg onload=alert(1)>"),
	})
}

func zipFiles(t *testing.T, files map[string][]byte) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "in.apkg")
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, data := range files {
		w, err := zw.Create(name)
		require.NoError(t, err)
		_, err = w.Write(data)
		require.NoError(t, err)
	}
	require.NoError(t, zw.Close())
	require.NoError(t, os.WriteFile(path, buf.Bytes(), 0o644))
	return path
}

func TestPlan_ConvertsHTMLClozeAndMediaIntoAnInboxFile(t *testing.T) {
	col, err := anki.Read(context.Background(), legacyApkg(t))
	require.NoError(t, err)

	files, st := anki.Plan(universe(t), col, nil, false, now)

	require.Len(t, files, 3)
	inbox := files[0]
	assert.Equal(t, "inbox/anki-japonais-vocabulaire.md", inbox.Path)
	assert.Contains(t, inbox.Content, "- 猫 (neko) :: chat / animal\n")
	assert.Contains(t, inbox.Content, "- Tokyo est la […] du Japon :: capitale\n")
	assert.Contains(t, inbox.Content, "- Tokyo est la capitale du [pays] :: Japon\n")
	assert.Regexp(t, `\n  !\[\]\(assets/anki/[0-9a-f]{8}-cat\.png\)\n`, inbox.Content)
	assert.Regexp(t, `\n  \[neko\.mp3\]\(assets/anki/[0-9a-f]{8}-neko\.mp3\)\n`, inbox.Content)
	assert.Equal(t, []byte("\x89PNG-cat"), files[1].Data)
	assert.Equal(t, anki.Stats{Notes: 2, Cards: 3, Cloze: 1, ToInbox: 3, MediaCopied: 2, Sounds: 1}, st)
}

func TestPlan_SkipsCardsTheRepoAlreadyHas(t *testing.T) {
	u := universe(t)
	notes, _, err := anki.Collect(u, "")
	require.NoError(t, err)
	col, err := anki.Read(context.Background(), exportFile(t, notes))
	require.NoError(t, err)

	files, st := anki.Plan(u, col, nil, true, now)

	assert.Empty(t, files)
	assert.Equal(t, 3, st.Known)
}

func TestPlan_ReturnsNewCardsOfAnExportedSubjectToTheirFile_WhenProposing(t *testing.T) {
	u := universe(t)
	notes, _, err := anki.Collect(u, "tech/k8s/cilium")
	require.NoError(t, err)
	notes[0].CardID = "tech/k8s/cilium:deadbeef"
	notes[0].Front = "Quel mode remplace iptables ?"
	col, err := anki.Read(context.Background(), exportFile(t, notes))
	require.NoError(t, err)

	files, st := anki.Plan(u, col, nil, true, now)

	require.Len(t, files, 1)
	assert.Equal(t, "galaxies/tech/k8s/cilium.md", files[0].Path)
	assert.True(t, strings.HasPrefix(files[0].Content, "---\nname: Cilium"))
	assert.Contains(t, files[0].Content, "\n\n- Quel mode remplace iptables ? :: Cilium en mode kube-proxy replacement\n")
	assert.Equal(t, 1, st.ToSubjects)
}

func TestText_FlattensFieldsIntoOneSafeLine(t *testing.T) {
	tests := []struct{ name, in, want string }{
		{"line breaks", "a<br>b<div>c</div>", "a / b / c"},
		{"entities", "R&amp;D&nbsp;&lt;3", "R&D <3"},
		{"separator", "x :: y", "x : y"},
		{"nested tags", "<span style='x'><b>gras</b></span>", "gras"},
		{"script and style bodies", "a<script>alert(1)</script><style>p{}</style>b", "a b"},
		{"escaped tags never come out as tags", "&lt;img src=x onerror=alert(1)&gt;ok", "ok"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, _, _ := anki.Text(tt.in)

			assert.Equal(t, tt.want, got)
		})
	}
}

func TestCollect_KeepsTheFirstCard_WhenAQuestionRepeatsInASubject(t *testing.T) {
	root := t.TempDir()
	write(t, root, "galaxies/lang/jp/keigo.md", "---\nname: Keigo\n---\nです :: copule polie\n- です :: copule\n")
	u, _, err := kb.Load(root)
	require.NoError(t, err)

	notes, dups, err := anki.Collect(u, "")

	require.NoError(t, err)
	require.Len(t, notes, 1)
	assert.Equal(t, "copule polie", notes[0].Back)
	assert.Len(t, dups, 1)
	assert.NotEmpty(t, exportFile(t, notes))
}

func TestPlan_AppendsToTheInboxFileAndSkipsQuestionsAlreadyThere(t *testing.T) {
	col, err := anki.Read(context.Background(), legacyApkg(t))
	require.NoError(t, err)
	prev := "---\nsource: anki\n---\nDéjà là.\n\n- 猫 (neko) :: chat / animal\n"

	files, st := anki.Plan(universe(t), col, map[string]string{"inbox/anki-japonais-vocabulaire.md": prev}, false, now)

	require.NotEmpty(t, files)
	assert.Equal(t, 1, st.Known)
	assert.True(t, strings.HasPrefix(files[0].Content, prev))
	assert.Equal(t, 1, strings.Count(files[0].Content, "---\nsource"))
	assert.Equal(t, 1, strings.Count(files[0].Content, "猫"))
	assert.Contains(t, files[0].Content, "- Tokyo est la […] du Japon :: capitale\n")
}

func TestRead_LoadsOnlyTheMediaTheNotesUse(t *testing.T) {
	col, err := anki.Read(context.Background(), legacyApkg(t))

	require.NoError(t, err)
	assert.Contains(t, col.Media, "cat.png")
	assert.NotContains(t, col.Media, "evil.svg")
}

func TestRead_RefusesAPackageWithTooManyEntries(t *testing.T) {
	path := filepath.Join(t.TempDir(), "many.apkg")
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for i := 0; i <= anki.MaxEntries; i++ {
		_, err := zw.CreateHeader(&zip.FileHeader{Name: strconv.Itoa(i), Method: zip.Store})
		require.NoError(t, err)
	}
	require.NoError(t, zw.Close())
	require.NoError(t, os.WriteFile(path, buf.Bytes(), 0o644))

	_, err := anki.Read(context.Background(), path)

	assert.ErrorIs(t, err, anki.ErrTooLarge)
}

func TestPlan_EscapesBracketsInMediaLinks(t *testing.T) {
	col := &anki.Collection{
		Notes: []anki.RawNote{{Deck: "Sons", Fields: []string{"écoute [sound:a(b).mp3]", "réponse"}}},
		Media: map[string][]byte{"a(b).mp3": []byte("ID3")},
	}

	files, _ := anki.Plan(universe(t), col, nil, false, now)

	require.NotEmpty(t, files)
	assert.Regexp(t, `\n  \[a\\\(b\\\)\.mp3\]\(assets/anki/[0-9a-f]{8}-a_b_\.mp3\)\n`, files[0].Content)
}

func TestConnect_DoesNotFollowRedirects(t *testing.T) {
	hit := false
	elsewhere := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { hit = true }))
	defer elsewhere.Close()
	redirect := httptest.NewServer(http.RedirectHandler(elsewhere.URL, http.StatusTemporaryRedirect))
	defer redirect.Close()
	c, err := anki.NewConnect(redirect.URL)
	require.NoError(t, err)

	_, err = c.Push(context.Background(), nil)

	assert.Error(t, err)
	assert.False(t, hit)
}

// modernApkg builds an Anki 2.1.50+ package: zstd collection with schema 18 tables, zstd media
// index in protobuf, zstd media files.
func modernApkg(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "c.db")
	db, err := sql.Open("sqlite", dbPath)
	require.NoError(t, err)
	_, err = db.Exec(`CREATE TABLE notetypes (id integer, name text);
		CREATE TABLE fields (ntid integer, ord integer, name text);
		CREATE TABLE decks (id integer, name text);
		CREATE TABLE notes (id integer, guid text, mid integer, tags text, flds text);
		CREATE TABLE cards (nid integer, did integer, ord integer);
		INSERT INTO notetypes VALUES (7, 'Basique');
		INSERT INTO fields VALUES (7, 1, 'Verso'), (7, 0, 'Recto');
		INSERT INTO decks VALUES (3, 'Langues' || char(31) || 'Polonais');
		INSERT INTO notes VALUES (1, 'gp', 7, '', 'dziękuję<img src="pl.png">' || char(31) || 'merci');
		INSERT INTO cards VALUES (1, 3, 0);`)
	require.NoError(t, err)
	require.NoError(t, db.Close())
	raw, err := os.ReadFile(dbPath)
	require.NoError(t, err)
	enc, err := zstd.NewWriter(nil)
	require.NoError(t, err)
	z := func(b []byte) []byte { return enc.EncodeAll(b, nil) }
	entry := append([]byte{0x0a, byte(len("pl.png"))}, "pl.png"...)
	index := append([]byte{0x0a, byte(len(entry))}, entry...)
	return zipFiles(t, map[string][]byte{
		"collection.anki2":   []byte("stub that must be ignored"),
		"collection.anki21b": z(raw),
		"media":              z(index),
		"0":                  z([]byte("\x89PNG-pl")),
	})
}

func TestRead_UnderstandsTheModernZstdFormat(t *testing.T) {
	col, err := anki.Read(context.Background(), modernApkg(t))

	require.NoError(t, err)
	require.Len(t, col.Notes, 1)
	assert.Equal(t, "Langues::Polonais", col.Notes[0].Deck)
	assert.Equal(t, []string{"Recto", "Verso"}, col.Notes[0].FieldNames)
	assert.Equal(t, []byte("\x89PNG-pl"), col.Media["pl.png"])
}

// testdata/anki-25.09.apkg was written by Anki 25.09 itself (anki21b, unicase collations, zstd
// media): a Basic note with HTML and an image, and a cloze note with two deletions and a hint.
func TestPlan_ImportsAPackageWrittenByAnki2509(t *testing.T) {
	col, err := anki.Read(context.Background(), "testdata/anki-25.09.apkg")
	require.NoError(t, err)

	files, st := anki.Plan(universe(t), col, nil, false, now)

	require.Len(t, files, 2)
	assert.Equal(t, "inbox/anki-cuisine-pâtisserie.md", files[0].Path)
	assert.Contains(t, files[0].Content, "- Pâte à choux / cuisson ? :: 180 °C, 30 min\n  ![](assets/anki/be13d62f-choux.png)\n")
	assert.Contains(t, files[0].Content, "- Le […] fond à 32 °C :: beurre\n")
	assert.Contains(t, files[0].Content, "- Le beurre fond à [température] :: 32 °C\n")
	assert.Equal(t, []byte("\x89PNG-choux"), files[1].Data)
	assert.Equal(t, 3, st.ToInbox)
}

func TestRead_RejectsAFileThatIsNotAnAnkiPackage(t *testing.T) {
	p := filepath.Join(t.TempDir(), "x.apkg")
	require.NoError(t, os.WriteFile(p, []byte("hello"), 0o644))

	_, err := anki.Read(context.Background(), p)

	assert.ErrorIs(t, err, anki.ErrNotAnki)
}

func TestNewConnect_RefusesAddressesOutsideLocalhost(t *testing.T) {
	tests := []struct {
		url string
		ok  bool
	}{
		{"http://127.0.0.1:8765", true},
		{"http://localhost:8765", true},
		{"http://[::1]:8765", true},
		{"http://192.168.1.10:8765", false},
		{"http://anki.example.com", false},
		{"file:///etc/passwd", false},
	}
	for _, tt := range tests {
		t.Run(tt.url, func(t *testing.T) {
			_, err := anki.NewConnect(tt.url)

			if tt.ok {
				assert.NoError(t, err)
			} else {
				assert.ErrorIs(t, err, anki.ErrNotLocal)
			}
		})
	}
}

// fakeAnki answers the AnkiConnect actions Push uses and records what it was asked to change.
type fakeAnki struct {
	models, decks []string
	notes         map[int64]map[string]string
	added         []map[string]any
	updated       []int64
}

func (f *fakeAnki) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Action string          `json:"action"`
		Params json.RawMessage `json:"params"`
	}
	body, _ := io.ReadAll(r.Body)
	_ = json.Unmarshal(body, &req)
	var result any
	switch req.Action {
	case "version":
		result = 6
	case "modelNames":
		result = f.models
	case "createModel":
		f.models = append(f.models, anki.ModelName)
	case "deckNames":
		result = f.decks
	case "createDeck":
		var p struct{ Deck string }
		_ = json.Unmarshal(req.Params, &p)
		f.decks = append(f.decks, p.Deck)
	case "findNotes":
		ids := []int64{}
		for id := range f.notes {
			ids = append(ids, id)
		}
		result = ids
	case "notesInfo":
		var out []map[string]any
		for id, fields := range f.notes {
			fs := map[string]any{}
			for k, v := range fields {
				fs[k] = map[string]string{"value": v}
			}
			out = append(out, map[string]any{"noteId": id, "fields": fs})
		}
		result = out
	case "multi":
		var p struct {
			Actions []struct {
				Params struct {
					Note struct{ ID int64 } `json:"note"`
				} `json:"params"`
			} `json:"actions"`
		}
		_ = json.Unmarshal(req.Params, &p)
		for _, a := range p.Actions {
			f.updated = append(f.updated, a.Params.Note.ID)
		}
		result = make([]any, len(p.Actions))
	case "addNotes":
		var p struct{ Notes []map[string]any }
		_ = json.Unmarshal(req.Params, &p)
		f.added = append(f.added, p.Notes...)
		ids := make([]int64, len(p.Notes))
		for i := range ids {
			ids[i] = int64(100 + i)
		}
		result = ids
	default:
		_ = json.NewEncoder(w).Encode(map[string]any{"result": nil, "error": "unsupported action"})
		return
	}
	_ = json.NewEncoder(w).Encode(map[string]any{"result": result, "error": nil})
}

func TestPush_AddsNewNotesUpdatesChangedOnesAndLeavesTheRest(t *testing.T) {
	notes, _, err := anki.Collect(universe(t), "langues")
	require.NoError(t, err)
	fake := &fakeAnki{notes: map[int64]map[string]string{
		1: {"Front": "あ", "Back": "a", "Subject": notes[0].Subject, "RtkID": notes[0].CardID},
		2: {"Front": "か", "Back": "ga", "Subject": notes[1].Subject, "RtkID": notes[1].CardID},
	}}
	srv := httptest.NewServer(fake)
	defer srv.Close()
	srv.URL = strings.Replace(srv.URL, "127.0.0.1", "localhost", 1)
	cilium, _, _ := anki.Collect(universe(t), "tech")
	c, err := anki.NewConnect(srv.URL)
	require.NoError(t, err)

	st, err := c.Push(context.Background(), append(notes, cilium...))

	require.NoError(t, err)
	assert.Equal(t, anki.PushStats{Added: 1, Updated: 1, Unchanged: 1}, st)
	assert.Equal(t, []int64{2}, fake.updated)
	assert.Contains(t, fake.models, anki.ModelName)
	assert.Contains(t, fake.decks, "RtK::tech::k8s")
}

func TestPush_ReportsThatAnkiIsNotRunning_WhenNothingListens(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	url := srv.URL
	srv.Close()
	c, err := anki.NewConnect(url)
	require.NoError(t, err)

	_, err = c.Push(context.Background(), nil)

	assert.ErrorIs(t, err, anki.ErrAnkiNotRunning)
}
