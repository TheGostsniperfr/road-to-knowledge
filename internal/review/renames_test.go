package review_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/TheGostsniperfr/road-to-knowledge/internal/review"
)

func TestRedirects_ResolveIdsToTheLiveOne(t *testing.T) {
	t.Parallel()
	t0 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	at := func(h int) time.Time { return t0.Add(time.Duration(h) * time.Hour) }
	tests := []struct {
		name    string
		renames []review.Rename
		in      string
		want    string
	}{
		{"an unknown id passes through", nil, "a/b/c", "a/b/c"},
		{"a moved subject resolves", []review.Rename{{"a/b/c", "a/d/c", at(1)}}, "a/b/c", "a/d/c"},
		{"two moves resolve to the last place", []review.Rename{{"a/b/c", "a/d/c", at(1)}, {"a/d/c", "x/y/z", at(2)}}, "a/b/c", "x/y/z"},
		{"order follows time, not the file", []review.Rename{{"a/d/c", "x/y/z", at(2)}, {"a/b/c", "a/d/c", at(1)}}, "a/b/c", "x/y/z"},
		{"a move back makes the old id live again", []review.Rename{{"a/b/c", "a/d/c", at(1)}, {"a/d/c", "a/b/c", at(2)}}, "a/b/c", "a/b/c"},
		{"a move back redirects the id in between", []review.Rename{{"a/b/c", "a/d/c", at(1)}, {"a/d/c", "a/b/c", at(2)}}, "a/d/c", "a/b/c"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := review.NewRedirects(tt.renames)

			got, _ := r.ID(tt.in)

			assert.Equal(t, tt.want, got)
		})
	}
}

func TestRedirects_CardFollowsItsSubjectUnlessRenamedItself(t *testing.T) {
	t.Parallel()
	now := time.Now()
	r := review.NewRedirects([]review.Rename{
		{From: "a/b/c:1234", To: "a/b/e:1234", At: now},
		{From: "a/b/c", To: "a/b/d", At: now},
	})

	assert.Equal(t, "a/b/d:9999", r.Card("a/b/c:9999"))
	assert.Equal(t, "a/b/e:1234", r.Card("a/b/c:1234"))
}

func TestRedirects_CardSplitOutAndMergedBack_EndsInTheMergedSubject(t *testing.T) {
	t.Parallel()
	t0 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	r := review.NewRedirects([]review.Rename{
		{From: "j/k/kana:1b4c", To: "j/k/katakana:1b4c", At: t0.Add(time.Hour)},
		{From: "j/k/katakana", To: "j/k/kana", At: t0.Add(2 * time.Hour)},
	})

	assert.Equal(t, "j/k/kana:1b4c", r.CardAt("j/k/kana:1b4c", t0))
	assert.Equal(t, "j/k/kana:1b4c", r.CardAt("j/k/katakana:1b4c", t0.Add(90*time.Minute)))
}

func TestRedirects_OnlyRenamesAfterTheReviewApply(t *testing.T) {
	t.Parallel()
	t0 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	r := review.NewRedirects([]review.Rename{{From: "a/b/c", To: "a/b/d", At: t0}})

	assert.Equal(t, "a/b/c:1", r.CardAt("a/b/c:1", t0.Add(time.Hour)), "a new subject that reuses an old id keeps its own reviews")
	assert.Equal(t, "a/b/d:1", r.CardAt("a/b/c:1", t0.Add(-time.Hour)))
}

func TestBook_ReplaysReviewsUnderTheNewId_AfterARename(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	b, err := review.Open(root)
	require.NoError(t, err)
	now := time.Now()
	_, err = b.Record("a/b/c:1234", review.Good, now.Add(-time.Hour))
	require.NoError(t, err)

	require.NoError(t, b.AppendRenames([]review.Rename{{From: "a/b/c", To: "x/y/z", At: now}}))

	assert.Equal(t, uint64(1), b.State("x/y/z:1234", now).Reps)
	assert.True(t, b.State("a/b/c:1234", now).IsNew)
	_, err = os.Stat(filepath.Join(root, review.RenamesPath))
	assert.NoError(t, err)
}
