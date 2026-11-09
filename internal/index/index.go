// Package index is a full-text search over subjects, rebuilt in memory on every reload.
package index

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"unicode"

	_ "modernc.org/sqlite"

	"github.com/TheGostsniperfr/road-to-knowledge/internal/kb"
)

type Hit struct {
	ID      string  `json:"id"`
	Name    string  `json:"name"`
	Snippet string  `json:"snippet"`
	Score   float64 `json:"score"`
}

type Index struct{ db *sql.DB }

func Build(ctx context.Context, u *kb.Universe) (*Index, error) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		return nil, fmt.Errorf("index: open: %w", err)
	}
	// A :memory: database exists per connection, so the pool must never open a second one.
	db.SetMaxOpenConns(1)
	if _, err := db.ExecContext(ctx, `CREATE VIRTUAL TABLE docs USING fts5(id UNINDEXED, name, path, body, tokenize='unicode61 remove_diacritics 2')`); err != nil {
		db.Close()
		return nil, fmt.Errorf("index: create: %w", err)
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		db.Close()
		return nil, fmt.Errorf("index: begin: %w", err)
	}
	stmt, err := tx.PrepareContext(ctx, `INSERT INTO docs(id, name, path, body) VALUES (?, ?, ?, ?)`)
	if err != nil {
		tx.Rollback()
		db.Close()
		return nil, fmt.Errorf("index: prepare: %w", err)
	}
	for _, s := range u.Stars() {
		path := s.Cluster.Galaxy.Name + " " + s.Cluster.Name + " " + strings.Join(s.Tags, " ")
		if _, err := stmt.ExecContext(ctx, s.ID, s.Name, path, s.Body); err != nil {
			tx.Rollback()
			db.Close()
			return nil, fmt.Errorf("index: insert %s: %w", s.ID, err)
		}
	}
	if err := tx.Commit(); err != nil {
		db.Close()
		return nil, fmt.Errorf("index: commit: %w", err)
	}
	return &Index{db: db}, nil
}

func (ix *Index) Close() error { return ix.db.Close() }

// Search ranks name matches above body matches and accepts prefixes, so "kan" finds "Kana".
// Every meaningful word must match first; when that finds fewer than limit subjects, subjects
// matching any of the words follow, so a question phrased in full still finds something.
func (ix *Index) Search(ctx context.Context, query string, limit int) ([]Hit, error) {
	words := terms(query)
	if kept := withoutStopwords(words); len(kept) > 0 {
		words = kept
	}
	hits, err := ix.match(ctx, strings.Join(words, " "), limit)
	if err != nil || len(hits) >= limit || len(words) < 2 {
		return hits, err
	}
	more, err := ix.match(ctx, strings.Join(words, " OR "), limit)
	if err != nil {
		return hits, err
	}
	seen := map[string]bool{}
	for _, h := range hits {
		seen[h.ID] = true
	}
	for _, h := range more {
		if len(hits) == limit {
			break
		}
		if !seen[h.ID] {
			hits = append(hits, h)
		}
	}
	return hits, nil
}

// SearchAllWords is the first version: every word, stopwords included, must match. It stays as
// the baseline of `rtk bench search`.
func (ix *Index) SearchAllWords(ctx context.Context, query string, limit int) ([]Hit, error) {
	return ix.match(ctx, strings.Join(terms(query), " "), limit)
}

func (ix *Index) match(ctx context.Context, q string, limit int) ([]Hit, error) {
	if q == "" {
		return nil, nil
	}
	rows, err := ix.db.QueryContext(ctx, `SELECT id, name, snippet(docs, 3, '[', ']', '…', 12), bm25(docs, 0, 10, 3, 1)
		FROM docs WHERE docs MATCH ? ORDER BY bm25(docs, 0, 10, 3, 1) LIMIT ?`, q, limit)
	if err != nil {
		return nil, fmt.Errorf("index: search: %w", err)
	}
	defer rows.Close()
	var hits []Hit
	for rows.Next() {
		var h Hit
		if err := rows.Scan(&h.ID, &h.Name, &h.Snippet, &h.Score); err != nil {
			return nil, fmt.Errorf("index: scan: %w", err)
		}
		hits = append(hits, h)
	}
	return hits, rows.Err()
}

// terms turns free text into safe FTS5 terms: every word becomes a quoted prefix term.
func terms(q string) []string {
	words := strings.FieldsFunc(q, func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsNumber(r) })
	for i, w := range words {
		words[i] = `"` + w + `"*`
	}
	return words
}

func withoutStopwords(quoted []string) []string {
	var out []string
	for _, t := range quoted {
		if !stopwords[strings.ToLower(strings.TrimSuffix(strings.Trim(t, `"*`), `"`))] {
			out = append(out, t)
		}
	}
	return out
}

// stopwords are the French and English words that carry no subject on their own; as prefix terms
// they would also match half the corpus ("de" matches "des", "deux", "dev").
var stopwords = func() map[string]bool {
	m := map[string]bool{}
	for _, w := range strings.Fields(`a à au aux avec ce ces cet cette comme dans de des du elle en est et
		il je la le les leur lui ma mais me mes mon ne ni nos notre nous on ou où par pas pour qu que qui
		sa sans se ses son sur ta te tes ton tu un une vos votre vous y d l c j s n m t qu est-ce quoi
		comment quel quelle quels quelles faire fait
		a an and are as at be by can do does for from has have how i in is it its my not of on or so
		that the their them then there these this to was we what when where which who why will with you your`) {
		m[w] = true
	}
	return m
}()
