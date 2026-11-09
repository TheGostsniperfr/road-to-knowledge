package review

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// RenamesPath records id changes from restructures (ADR-0014). Like the review log it is
// append-only: replaying it in time order maps every old id to the id that is live today.
const RenamesPath = "review/renames.jsonl"

// Rename moves an id. A subject, sub-theme or theme id has no ":"; a card id is "<subject>:<hash>".
type Rename struct {
	From string    `json:"from"`
	To   string    `json:"to"`
	At   time.Time `json:"at"`
}

// Live says whether an id (theme, sub-theme, subject or card) exists in the files today.
type Live func(id string) bool

// Redirects resolves old ids to live ones by replaying the renames in time order.
//
// An id that is not live today follows every rename, whatever the time of the review: a
// rename stamped when a proposal was written still catches the reviews made before the user
// merged it, here or on another machine. Time only decides for an id that is live again (a
// subject moved away and back, or a new subject that reuses an old id): its reviews written
// after the rename belong to the subject living there now and stay put.
type Redirects struct {
	renames []Rename
	live    Live
}

// NewRedirects builds a resolver; a nil live treats every id as gone, which is right for a
// log read before the files are loaded.
func NewRedirects(renames []Rename, live Live) Redirects {
	sorted := make([]Rename, 0, len(renames))
	for _, r := range renames {
		if r.From != "" && r.To != "" && r.From != r.To {
			sorted = append(sorted, r)
		}
	}
	sort.SliceStable(sorted, func(i, j int) bool { return sorted[i].At.Before(sorted[j].At) })
	if live == nil {
		live = func(string) bool { return false }
	}
	return Redirects{renames: sorted, live: live}
}

// resolve replays renames on an id written at t (zero when unknown: a live id then stays). A card
// id ("<subject>:<hash>") follows an explicit card rename, otherwise its subject, so a card that
// moved with its subject needs no line of its own.
func (r Redirects) resolve(id string, t time.Time) string {
	if t.IsZero() && r.live(id) {
		return id
	}
	for _, x := range r.renames {
		subject := false
		if id != x.From {
			i := strings.LastIndexByte(id, ':')
			if i <= 0 || id[:i] != x.From {
				continue
			}
			subject = true
		}
		if r.live(x.From) && (t.IsZero() || t.After(x.At)) {
			continue
		}
		if subject {
			id = x.To + id[strings.LastIndexByte(id, ':'):]
		} else {
			id = x.To
		}
	}
	return id
}

// ID returns the live id for an id that may have been renamed, and whether it was.
func (r Redirects) ID(id string) (string, bool) {
	to := r.resolve(id, time.Time{})
	return to, to != id
}

// Card resolves a card id written at an unknown time, e.g. by a page opened before a restructure.
func (r Redirects) Card(id string) string { return r.resolve(id, time.Time{}) }

// CardAt resolves a card id as it was written at t.
func (r Redirects) CardAt(id string, t time.Time) string { return r.resolve(id, t) }

// All maps every renamed theme, sub-theme or subject id that is not live to its live id, for the
// UI's deep links.
func (r Redirects) All() map[string]string {
	out := map[string]string{}
	for _, x := range r.renames {
		if strings.Contains(x.From, ":") {
			continue
		}
		if to, ok := r.ID(x.From); ok {
			out[x.From] = to
		}
	}
	return out
}

func readRenames(path string) ([]Rename, error) {
	f, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("review: open renames: %w", err)
	}
	defer f.Close()
	var out []Rename
	sc := bufio.NewScanner(f)
	for line := 1; sc.Scan(); line++ {
		if len(sc.Bytes()) == 0 {
			continue
		}
		var r Rename
		if err := json.Unmarshal(sc.Bytes(), &r); err != nil {
			return nil, fmt.Errorf("review: %s line %d: %w", path, line, err)
		}
		out = append(out, r)
	}
	return out, sc.Err()
}

// RenameLines encodes renames as JSON lines, the format appended to RenamesPath.
func RenameLines(rs []Rename) ([]byte, error) {
	var out []byte
	for _, r := range rs {
		r.At = r.At.UTC().Truncate(time.Second)
		line, err := json.Marshal(r)
		if err != nil {
			return nil, fmt.Errorf("review: encode rename: %w", err)
		}
		out = append(append(out, line...), '\n')
	}
	return out, nil
}

// AppendRenames writes renames to the log and replays everything, so states follow the new ids at once.
func (b *Book) AppendRenames(rs []Rename) error {
	if len(rs) == 0 {
		return nil
	}
	data, err := RenameLines(rs)
	if err != nil {
		return err
	}
	path := b.renamesPath()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("review: %w", err)
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("review: open renames: %w", err)
	}
	if _, err := f.Write(data); err != nil {
		f.Close()
		return fmt.Errorf("review: append renames: %w", err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("review: close renames: %w", err)
	}
	return b.Reload()
}

func (b *Book) renamesPath() string {
	return filepath.Join(filepath.Dir(filepath.Dir(b.path)), filepath.FromSlash(RenamesPath))
}

// RenamesFile is the absolute path of the rename log, for commits.
func (b *Book) RenamesFile() string { return b.renamesPath() }

// SetLive tells the book which ids exist in the files; the next Reload resolves renames with it.
func (b *Book) SetLive(live Live) {
	b.mu.Lock()
	b.live = live
	b.mu.Unlock()
}

// Redirects returns the resolver built at the last reload.
func (b *Book) Redirects() Redirects {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.redirects
}
