package anki

import (
	"archive/zip"
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"github.com/klauspost/compress/zstd"
	"modernc.org/sqlite"
)

// Anki 2.1.50+ declares deck and note type names COLLATE unicase, a collation of its own;
// without it SQLite refuses any query that touches those indexes.
var unicase = sync.OnceValue(func() error {
	return sqlite.RegisterCollationUtf8("unicase", func(a, b string) int {
		return strings.Compare(strings.ToLower(a), strings.ToLower(b))
	})
})

// Collection is what an .apkg holds that matters here: notes with their deck and field names, and media.
type Collection struct {
	Notes []RawNote
	Media map[string][]byte
}

type RawNote struct {
	GUID       string
	Model      string
	Deck       string
	FieldNames []string
	Fields     []string
	Tags       []string
}

var ErrNotAnki = errors.New("not an Anki package")

var zstdMagic = []byte{0x28, 0xb5, 0x2f, 0xfd}

// maxEntry bounds what one zip entry may expand to, so a crafted package cannot fill the disk or memory.
const maxEntry = 512 << 20

// Read opens an .apkg. It prefers collection.anki21b (Anki 2.1.50+, zstd, schema 18), then
// collection.anki21, then collection.anki2, because newer exports put a stub in the old file.
func Read(ctx context.Context, path string) (*Collection, error) {
	zr, err := zip.OpenReader(path)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrNotAnki, err)
	}
	defer zr.Close()
	files := map[string]*zip.File{}
	for _, f := range zr.File {
		files[f.Name] = f
	}
	var colFile *zip.File
	for _, name := range []string{"collection.anki21b", "collection.anki21", "collection.anki2"} {
		if f := files[name]; f != nil {
			colFile = f
			break
		}
	}
	if colFile == nil {
		return nil, fmt.Errorf("%w: no collection inside", ErrNotAnki)
	}
	raw, err := readEntry(colFile)
	if err != nil {
		return nil, err
	}
	tmp, err := os.MkdirTemp("", "rtk-anki-read-")
	if err != nil {
		return nil, fmt.Errorf("anki read: %w", err)
	}
	defer os.RemoveAll(tmp)
	dbPath := filepath.Join(tmp, "collection.db")
	if err := os.WriteFile(dbPath, raw, 0o600); err != nil {
		return nil, fmt.Errorf("anki read: %w", err)
	}
	notes, err := readNotes(ctx, dbPath)
	if err != nil {
		return nil, err
	}
	media, err := readMedia(files)
	if err != nil {
		return nil, err
	}
	return &Collection{Notes: notes, Media: media}, nil
}

func readEntry(f *zip.File) ([]byte, error) {
	rc, err := f.Open()
	if err != nil {
		return nil, fmt.Errorf("anki read %s: %w", f.Name, err)
	}
	defer rc.Close()
	data, err := io.ReadAll(io.LimitReader(rc, maxEntry+1))
	if err != nil {
		return nil, fmt.Errorf("anki read %s: %w", f.Name, err)
	}
	if len(data) > maxEntry {
		return nil, fmt.Errorf("anki read %s: entry larger than %d bytes", f.Name, maxEntry)
	}
	return unzstd(f.Name, data)
}

func unzstd(name string, data []byte) ([]byte, error) {
	if !bytes.HasPrefix(data, zstdMagic) {
		return data, nil
	}
	dec, err := zstd.NewReader(nil, zstd.WithDecoderMaxMemory(maxEntry))
	if err != nil {
		return nil, fmt.Errorf("anki read %s: %w", name, err)
	}
	defer dec.Close()
	out, err := dec.DecodeAll(data, nil)
	if err != nil {
		return nil, fmt.Errorf("anki read %s: zstd: %w", name, err)
	}
	return out, nil
}

type model struct {
	name   string
	fields []string
}

func readNotes(ctx context.Context, path string) ([]RawNote, error) {
	if err := unicase(); err != nil {
		return nil, fmt.Errorf("anki read: %w", err)
	}
	db, err := sql.Open("sqlite", "file:"+path+"?mode=ro")
	if err != nil {
		return nil, fmt.Errorf("anki read: %w", err)
	}
	defer db.Close()
	var modern int
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM sqlite_master WHERE type='table' AND name='notetypes'`).Scan(&modern); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrNotAnki, err)
	}
	var models map[int64]model
	var decks map[int64]string
	if modern > 0 {
		models, decks, err = modernMeta(ctx, db)
	} else {
		models, decks, err = legacyMeta(ctx, db)
	}
	if err != nil {
		return nil, err
	}

	deckOf := map[int64]int64{}
	rows, err := db.QueryContext(ctx, `SELECT nid, did FROM cards ORDER BY nid, ord`)
	if err != nil {
		return nil, fmt.Errorf("anki read cards: %w", err)
	}
	for rows.Next() {
		var nid, did int64
		if err := rows.Scan(&nid, &did); err != nil {
			rows.Close()
			return nil, fmt.Errorf("anki read cards: %w", err)
		}
		if _, ok := deckOf[nid]; !ok {
			deckOf[nid] = did
		}
	}
	rows.Close()

	rows, err = db.QueryContext(ctx, `SELECT id, guid, mid, tags, flds FROM notes ORDER BY id`)
	if err != nil {
		return nil, fmt.Errorf("anki read notes: %w", err)
	}
	defer rows.Close()
	var out []RawNote
	for rows.Next() {
		var id, mid int64
		var guid, tags, flds string
		if err := rows.Scan(&id, &guid, &mid, &tags, &flds); err != nil {
			return nil, fmt.Errorf("anki read notes: %w", err)
		}
		m := models[mid]
		out = append(out, RawNote{
			GUID: guid, Model: m.name, Deck: decks[deckOf[id]], FieldNames: m.fields,
			Fields: strings.Split(flds, "\x1f"), Tags: strings.Fields(tags),
		})
	}
	return out, rows.Err()
}

func legacyMeta(ctx context.Context, db *sql.DB) (map[int64]model, map[int64]string, error) {
	var modelsJS, decksJS string
	if err := db.QueryRowContext(ctx, `SELECT models, decks FROM col`).Scan(&modelsJS, &decksJS); err != nil {
		return nil, nil, fmt.Errorf("%w: col: %w", ErrNotAnki, err)
	}
	var rawModels map[string]struct {
		Name string `json:"name"`
		Flds []struct {
			Name string `json:"name"`
			Ord  int    `json:"ord"`
		} `json:"flds"`
	}
	var rawDecks map[string]struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal([]byte(modelsJS), &rawModels); err != nil {
		return nil, nil, fmt.Errorf("%w: models: %w", ErrNotAnki, err)
	}
	if err := json.Unmarshal([]byte(decksJS), &rawDecks); err != nil {
		return nil, nil, fmt.Errorf("%w: decks: %w", ErrNotAnki, err)
	}
	models := map[int64]model{}
	for k, m := range rawModels {
		id, _ := strconv.ParseInt(k, 10, 64)
		names := make([]string, len(m.Flds))
		for _, f := range m.Flds {
			if f.Ord >= 0 && f.Ord < len(names) {
				names[f.Ord] = f.Name
			}
		}
		models[id] = model{name: m.Name, fields: names}
	}
	decks := map[int64]string{}
	for k, d := range rawDecks {
		id, _ := strconv.ParseInt(k, 10, 64)
		decks[id] = d.Name
	}
	return models, decks, nil
}

func modernMeta(ctx context.Context, db *sql.DB) (map[int64]model, map[int64]string, error) {
	models := map[int64]model{}
	rows, err := db.QueryContext(ctx, `SELECT id, name FROM notetypes`)
	if err != nil {
		return nil, nil, fmt.Errorf("anki read notetypes: %w", err)
	}
	for rows.Next() {
		var id int64
		var name string
		if err := rows.Scan(&id, &name); err != nil {
			rows.Close()
			return nil, nil, fmt.Errorf("anki read notetypes: %w", err)
		}
		models[id] = model{name: name}
	}
	rows.Close()
	rows, err = db.QueryContext(ctx, `SELECT ntid, name FROM fields ORDER BY ntid, ord`)
	if err != nil {
		return nil, nil, fmt.Errorf("anki read fields: %w", err)
	}
	for rows.Next() {
		var ntid int64
		var name string
		if err := rows.Scan(&ntid, &name); err != nil {
			rows.Close()
			return nil, nil, fmt.Errorf("anki read fields: %w", err)
		}
		m := models[ntid]
		m.fields = append(m.fields, name)
		models[ntid] = m
	}
	rows.Close()
	decks := map[int64]string{}
	rows, err = db.QueryContext(ctx, `SELECT id, name FROM decks`)
	if err != nil {
		return nil, nil, fmt.Errorf("anki read decks: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var id int64
		var name string
		if err := rows.Scan(&id, &name); err != nil {
			return nil, nil, fmt.Errorf("anki read decks: %w", err)
		}
		// Schema 18 separates deck levels with 0x1f instead of "::".
		decks[id] = strings.ReplaceAll(name, "\x1f", "::")
	}
	return models, decks, rows.Err()
}

// readMedia maps media names to bytes. The index is JSON {"0": "a.png"} in old packages and a
// zstd protobuf list in anki21b ones, where entry i lives in the zip under its position.
func readMedia(files map[string]*zip.File) (map[string][]byte, error) {
	out := map[string][]byte{}
	idx := files["media"]
	if idx == nil {
		return out, nil
	}
	data, err := readEntry(idx)
	if err != nil {
		return nil, err
	}
	names := map[string]string{}
	if t := bytes.TrimSpace(data); len(t) > 0 && t[0] == '{' {
		if err := json.Unmarshal(t, &names); err != nil {
			return nil, fmt.Errorf("anki read media index: %w", err)
		}
	} else {
		entries, err := mediaEntries(data)
		if err != nil {
			return nil, err
		}
		for i, e := range entries {
			key := strconv.Itoa(i)
			if e.zipName >= 0 {
				key = strconv.Itoa(e.zipName)
			}
			names[key] = e.name
		}
	}
	for key, name := range names {
		f := files[key]
		if f == nil || name == "" {
			continue
		}
		b, err := readEntry(f)
		if err != nil {
			return nil, err
		}
		out[name] = b
	}
	return out, nil
}

type mediaEntry struct {
	name    string
	zipName int
}

// mediaEntries decodes MediaEntries { repeated MediaEntry entries = 1 } with
// MediaEntry { string name = 1; uint32 size = 2; bytes sha1 = 3; uint32 legacy_zip_filename = 255 }.
func mediaEntries(b []byte) ([]mediaEntry, error) {
	var out []mediaEntry
	err := protoFields(b, func(num int, wire int, v uint64, data []byte) error {
		if num != 1 || wire != 2 {
			return nil
		}
		e := mediaEntry{zipName: -1}
		if err := protoFields(data, func(num int, wire int, v uint64, data []byte) error {
			switch {
			case num == 1 && wire == 2:
				e.name = string(data)
			case num == 255 && wire == 0:
				e.zipName = int(v)
			}
			return nil
		}); err != nil {
			return err
		}
		out = append(out, e)
		return nil
	})
	return out, err
}

var errProto = errors.New("anki read media index: malformed protobuf")

func protoFields(b []byte, f func(num, wire int, v uint64, data []byte) error) error {
	for len(b) > 0 {
		key, n := uvarint(b)
		if n <= 0 {
			return errProto
		}
		b = b[n:]
		num, wire := int(key>>3), int(key&7)
		switch wire {
		case 0:
			v, n := uvarint(b)
			if n <= 0 {
				return errProto
			}
			b = b[n:]
			if err := f(num, wire, v, nil); err != nil {
				return err
			}
		case 2:
			l, n := uvarint(b)
			if n <= 0 || uint64(len(b)-n) < l {
				return errProto
			}
			data := b[n : n+int(l)]
			b = b[n+int(l):]
			if err := f(num, wire, 0, data); err != nil {
				return err
			}
		case 5:
			if len(b) < 4 {
				return errProto
			}
			b = b[4:]
		case 1:
			if len(b) < 8 {
				return errProto
			}
			b = b[8:]
		default:
			return errProto
		}
	}
	return nil
}

func uvarint(b []byte) (uint64, int) {
	var v uint64
	for i := 0; i < len(b) && i < 10; i++ {
		v |= uint64(b[i]&0x7f) << (7 * i)
		if b[i] < 0x80 {
			return v, i + 1
		}
	}
	return 0, 0
}
