package anki

import (
	"archive/zip"
	"context"
	"crypto/sha1"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// The legacy schema 11 collection: every Anki version since 2.1 imports it, and the newer
// formats are only an optimisation of the same tables.
const schema = `
CREATE TABLE col (id integer primary key, crt integer not null, mod integer not null, scm integer not null, ver integer not null, dty integer not null, usn integer not null, ls integer not null, conf text not null, models text not null, decks text not null, dconf text not null, tags text not null);
CREATE TABLE notes (id integer primary key, guid text not null, mid integer not null, mod integer not null, usn integer not null, tags text not null, flds text not null, sfld integer not null, csum integer not null, flags integer not null, data text not null);
CREATE TABLE cards (id integer primary key, nid integer not null, did integer not null, ord integer not null, mod integer not null, usn integer not null, type integer not null, queue integer not null, due integer not null, ivl integer not null, factor integer not null, reps integer not null, lapses integer not null, left integer not null, odue integer not null, odid integer not null, flags integer not null, data text not null);
CREATE TABLE revlog (id integer primary key, cid integer not null, usn integer not null, ease integer not null, ivl integer not null, lastIvl integer not null, factor integer not null, time integer not null, type integer not null);
CREATE TABLE graves (usn integer not null, oid integer not null, type integer not null);
CREATE INDEX ix_notes_usn on notes (usn);
CREATE INDEX ix_cards_usn on cards (usn);
CREATE INDEX ix_revlog_usn on revlog (usn);
CREATE INDEX ix_cards_nid on cards (nid);
CREATE INDEX ix_cards_sched on cards (did, queue, due);
CREATE INDEX ix_revlog_cid on revlog (cid);
CREATE INDEX ix_notes_csum on notes (csum);
`

const css = `.card { font-family: Inter, system-ui, sans-serif; font-size: 22px; text-align: center; color: #15171b; background: #fff; }
.rtk-subject { margin-top: 18px; font-size: 13px; color: #6b7079; }
.nightMode .card { color: #e3e4e7; background: #141517; }`

const (
	front = "{{Front}}"
	back  = `{{FrontSide}}<hr id=answer>{{Back}}<div class="rtk-subject">{{Subject}}</div>`
)

var modelID = stableID("model", ModelName)

// Export writes the notes as an .apkg. FSRS state is not exported (ADR-0010): the cards arrive
// as new and Anki schedules them on its own.
func Export(ctx context.Context, notes []Note, w io.Writer, now time.Time) error {
	tmp, err := os.MkdirTemp("", "rtk-anki-")
	if err != nil {
		return fmt.Errorf("anki export: %w", err)
	}
	defer os.RemoveAll(tmp)
	path := filepath.Join(tmp, "collection.anki2")
	if err := writeCollection(ctx, path, notes, now); err != nil {
		return err
	}
	col, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("anki export: %w", err)
	}
	zw := zip.NewWriter(w)
	for _, f := range []struct {
		name string
		data []byte
	}{{"collection.anki2", col}, {"media", []byte("{}")}} {
		fw, err := zw.Create(f.name)
		if err != nil {
			return fmt.Errorf("anki export: %w", err)
		}
		if _, err := fw.Write(f.data); err != nil {
			return fmt.Errorf("anki export: %w", err)
		}
	}
	if err := zw.Close(); err != nil {
		return fmt.Errorf("anki export: %w", err)
	}
	return nil
}

func writeCollection(ctx context.Context, path string, notes []Note, now time.Time) error {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return fmt.Errorf("anki export: %w", err)
	}
	defer db.Close()
	if _, err := db.ExecContext(ctx, schema); err != nil {
		return fmt.Errorf("anki export: schema: %w", err)
	}
	sec, ms := now.Unix(), now.UnixMilli()

	decks := map[string]any{"1": deckJSON(1, "Default", sec)}
	deckIDs := map[string]int64{}
	for _, d := range decksOf(notes) {
		parts := strings.Split(d, "::")
		// Parents first, so Anki shows RtK › Galaxy › Sub-theme even on versions that do not create them.
		for i := 1; i <= len(parts); i++ {
			name := strings.Join(parts[:i], "::")
			id := stableID("deck", name)
			deckIDs[name] = id
			decks[strconv.FormatInt(id, 10)] = deckJSON(id, name, sec)
		}
	}
	firstDeck := int64(1)
	if len(notes) > 0 {
		firstDeck = deckIDs[notes[0].Deck]
	}
	models := map[string]any{strconv.FormatInt(modelID, 10): modelJSON(firstDeck, sec)}
	conf := map[string]any{
		"activeDecks": []int64{1}, "addToCur": true, "collapseTime": 1200, "curDeck": 1,
		"curModel": strconv.FormatInt(modelID, 10), "dueCounts": true, "estTimes": true, "newBury": true,
		"newSpread": 0, "nextPos": len(notes) + 1, "sortBackwards": false, "sortType": "noteFld", "timeLim": 0,
	}
	dconf := map[string]any{"1": map[string]any{
		"autoplay": true, "id": 1, "maxTaken": 60, "mod": 0, "name": "Default", "replayq": true, "timer": 0, "usn": 0,
		"lapse": map[string]any{"delays": []int{10}, "leechAction": 0, "leechFails": 8, "minInt": 1, "mult": 0},
		"new":   map[string]any{"bury": true, "delays": []int{1, 10}, "initialFactor": 2500, "ints": []int{1, 4, 7}, "order": 1, "perDay": 20, "separate": true},
		"rev":   map[string]any{"bury": true, "ease4": 1.3, "fuzz": 0.05, "ivlFct": 1, "maxIvl": 36500, "minSpace": 1, "perDay": 100},
	}}
	js := func(v any) string { b, _ := json.Marshal(v); return string(b) }
	if _, err := db.ExecContext(ctx, `INSERT INTO col VALUES (1, ?, ?, ?, 11, 0, 0, 0, ?, ?, ?, ?, '{}')`,
		sec, ms, ms, js(conf), js(models), js(decks), js(dconf)); err != nil {
		return fmt.Errorf("anki export: col: %w", err)
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("anki export: %w", err)
	}
	defer tx.Rollback()
	for i, n := range notes {
		id := stableID("note", n.CardID)
		fields := []string{html.EscapeString(n.Front), html.EscapeString(n.Back), html.EscapeString(n.Subject), n.CardID}
		sum := sha1.Sum([]byte(n.Front))
		csum, _ := strconv.ParseInt(hex.EncodeToString(sum[:4]), 16, 64)
		tags := " " + strings.Join(n.Tags, " ") + " "
		if _, err := tx.ExecContext(ctx, `INSERT INTO notes VALUES (?, ?, ?, ?, -1, ?, ?, ?, ?, 0, '')`,
			id, n.GUID, modelID, sec, tags, strings.Join(fields, "\x1f"), n.Front, csum); err != nil {
			return fmt.Errorf("anki export: note %s: %w", n.CardID, err)
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO cards VALUES (?, ?, ?, 0, ?, -1, 0, 0, ?, 0, 0, 0, 0, 0, 0, 0, 0, '')`,
			id, id, deckIDs[n.Deck], sec, i+1); err != nil {
			return fmt.Errorf("anki export: card %s: %w", n.CardID, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("anki export: %w", err)
	}
	return nil
}

func deckJSON(id int64, name string, mod int64) map[string]any {
	return map[string]any{
		"collapsed": false, "conf": 1, "desc": "", "dyn": 0, "extendNew": 10, "extendRev": 50, "id": id,
		"lrnToday": []int{0, 0}, "mod": mod, "name": name, "newToday": []int{0, 0}, "revToday": []int{0, 0},
		"timeToday": []int{0, 0}, "usn": -1,
	}
}

func modelJSON(did, mod int64) map[string]any {
	flds := make([]map[string]any, len(Fields))
	for i, f := range Fields {
		flds[i] = map[string]any{"font": "Arial", "media": []string{}, "name": f, "ord": i, "rtl": false, "size": 20, "sticky": false}
	}
	return map[string]any{
		"css": css, "did": did, "flds": flds, "id": modelID, "mod": mod, "name": ModelName, "sortf": 0,
		"latexPre":  "\\documentclass[12pt]{article}\n\\special{papersize=3in,5in}\n\\usepackage[utf8]{inputenc}\n\\usepackage{amssymb,amsmath}\n\\pagestyle{empty}\n\\setlength{\\parindent}{0in}\n\\begin{document}\n",
		"latexPost": "\\end{document}",
		"req":       []any{[]any{0, "all", []int{0}}},
		"tags":      []string{},
		"tmpls":     []map[string]any{{"afmt": back, "bafmt": "", "bqfmt": "", "did": nil, "name": "Card 1", "ord": 0, "qfmt": front}},
		"type":      0, "usn": -1, "vers": []any{},
	}
}
