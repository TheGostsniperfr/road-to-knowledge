// Package review schedules cards with FSRS. The log is append-only and the card state is always
// rebuilt by replaying it, so nothing is lost and a merge conflict can only add lines.
package review

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	fsrs "github.com/open-spaced-repetition/go-fsrs/v3"
)

const LogPath = "review/log.jsonl"

type Rating int

const (
	Again Rating = 1
	Hard  Rating = 2
	Good  Rating = 3
	Easy  Rating = 4
)

func (r Rating) Valid() bool { return r >= Again && r <= Easy }

type Entry struct {
	Card   string    `json:"card"`
	Rating Rating    `json:"rating"`
	At     time.Time `json:"at"`
}

type CardState struct {
	ID         string    `json:"id"`
	Due        time.Time `json:"due"`
	Reps       uint64    `json:"reps"`
	Lapses     uint64    `json:"lapses"`
	LastReview time.Time `json:"last_review"`
	IsNew      bool      `json:"new"`
}

// Preview is the interval each rating would give, shown under the grade buttons.
type Preview map[Rating]time.Duration

type Book struct {
	mu      sync.Mutex
	path    string
	sched   *fsrs.FSRS
	entries []Entry
	cards   map[string]fsrs.Card
	// redirects maps ids from before a restructure; entries are replayed under the live id.
	redirects Redirects
	live      Live
}

func Open(root string) (*Book, error) {
	b := &Book{path: filepath.Join(root, filepath.FromSlash(LogPath)), sched: fsrs.NewFSRS(fsrs.DefaultParam())}
	if err := b.Reload(); err != nil {
		return nil, err
	}
	return b, nil
}

func (b *Book) Path() string { return b.path }

// Since returns the reviews recorded at or after t, oldest first.
func (b *Book) Since(t time.Time) []Entry {
	b.mu.Lock()
	defer b.mu.Unlock()
	var out []Entry
	for _, e := range b.entries {
		if !e.At.Before(t) {
			out = append(out, e)
		}
	}
	return out
}

// Reload re-reads the log, for instance after a git pull brought reviews from another machine.
func (b *Book) Reload() error {
	entries, err := readLog(b.path)
	if err != nil {
		return err
	}
	renames, err := readRenames(b.renamesPath())
	if err != nil {
		return err
	}
	b.mu.Lock()
	live := b.live
	b.mu.Unlock()
	redirects := NewRedirects(renames, live)
	for i := range entries {
		entries[i].Card = redirects.CardAt(entries[i].Card, entries[i].At)
	}
	sort.SliceStable(entries, func(i, j int) bool { return entries[i].At.Before(entries[j].At) })
	cards := map[string]fsrs.Card{}
	for _, e := range entries {
		c, ok := cards[e.Card]
		if !ok {
			c = fsrs.NewCard()
		}
		cards[e.Card] = b.sched.Next(c, e.At, fsrs.Rating(e.Rating)).Card
	}
	b.mu.Lock()
	b.entries, b.cards, b.redirects = entries, cards, redirects
	b.mu.Unlock()
	return nil
}

func readLog(path string) ([]Entry, error) {
	f, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("review: open log: %w", err)
	}
	defer f.Close()
	var out []Entry
	sc := bufio.NewScanner(f)
	for line := 1; sc.Scan(); line++ {
		if len(sc.Bytes()) == 0 {
			continue
		}
		var e Entry
		if err := json.Unmarshal(sc.Bytes(), &e); err != nil {
			return nil, fmt.Errorf("review: %s line %d: %w", path, line, err)
		}
		if e.Rating.Valid() && e.Card != "" {
			out = append(out, e)
		}
	}
	return out, sc.Err()
}

func (b *Book) State(cardID string, now time.Time) CardState {
	b.mu.Lock()
	defer b.mu.Unlock()
	c, ok := b.cards[cardID]
	if !ok {
		return CardState{ID: cardID, Due: now, IsNew: true}
	}
	return CardState{ID: cardID, Due: c.Due, Reps: c.Reps, Lapses: c.Lapses, LastReview: c.LastReview}
}

func (b *Book) IsDue(cardID string, now time.Time) bool {
	return !b.State(cardID, now).Due.After(now)
}

func (b *Book) Preview(cardID string, now time.Time) Preview {
	b.mu.Lock()
	c, ok := b.cards[cardID]
	b.mu.Unlock()
	if !ok {
		c = fsrs.NewCard()
	}
	p := Preview{}
	for r, info := range b.sched.Repeat(c, now) {
		p[Rating(r)] = info.Card.Due.Sub(now)
	}
	return p
}

// Record appends one review and updates the in-memory state.
func (b *Book) Record(cardID string, rating Rating, now time.Time) (CardState, error) {
	if !rating.Valid() {
		return CardState{}, fmt.Errorf("review: invalid rating %d", rating)
	}
	e := Entry{Card: cardID, Rating: rating, At: now.UTC().Truncate(time.Second)}
	line, err := json.Marshal(e)
	if err != nil {
		return CardState{}, fmt.Errorf("review: encode: %w", err)
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if err := os.MkdirAll(filepath.Dir(b.path), 0o755); err != nil {
		return CardState{}, fmt.Errorf("review: %w", err)
	}
	f, err := os.OpenFile(b.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return CardState{}, fmt.Errorf("review: open log: %w", err)
	}
	if _, err := f.Write(append(line, '\n')); err != nil {
		f.Close()
		return CardState{}, fmt.Errorf("review: append: %w", err)
	}
	if err := f.Close(); err != nil {
		return CardState{}, fmt.Errorf("review: close log: %w", err)
	}
	c, ok := b.cards[cardID]
	if !ok {
		c = fsrs.NewCard()
	}
	c = b.sched.Next(c, e.At, fsrs.Rating(rating)).Card
	b.cards[cardID] = c
	b.entries = append(b.entries, e)
	return CardState{ID: cardID, Due: c.Due, Reps: c.Reps, Lapses: c.Lapses, LastReview: c.LastReview}, nil
}

// LastReviewOf returns the latest review among the given cards, zero if none.
func (b *Book) LastReviewOf(cardIDs []string) time.Time {
	b.mu.Lock()
	defer b.mu.Unlock()
	var last time.Time
	for _, id := range cardIDs {
		if c, ok := b.cards[id]; ok && c.LastReview.After(last) {
			last = c.LastReview
		}
	}
	return last
}
