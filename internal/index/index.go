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
func (ix *Index) Search(ctx context.Context, query string, limit int) ([]Hit, error) {
	q := toMatch(query)
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

// toMatch turns free text into a safe FTS5 query: every word becomes a quoted prefix term.
func toMatch(q string) string {
	words := strings.FieldsFunc(q, func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsNumber(r) })
	for i, w := range words {
		words[i] = `"` + w + `"*`
	}
	return strings.Join(words, " ")
}
