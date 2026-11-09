package review_test

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/TheGostsniperfr/road-to-knowledge/internal/review"
)

var r0 = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

func at(h float64) time.Time { return r0.Add(time.Duration(h * float64(time.Hour))) }

func liveSet(ids ...string) review.Live {
	return func(id string) bool { return slices.Contains(ids, id) }
}

func TestRedirects_ResolveIdsToTheLiveOne(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		renames []review.Rename
		live    review.Live
		in      string
		want    string
	}{
		{"an unknown id passes through", nil, nil, "a/b/c", "a/b/c"},
		{"a moved subject resolves", []review.Rename{{"a/b/c", "a/d/c", at(1)}}, liveSet("a/d/c"), "a/b/c", "a/d/c"},
		{"two moves resolve to the last place", []review.Rename{{"a/b/c", "a/d/c", at(1)}, {"a/d/c", "x/y/z", at(2)}}, liveSet("x/y/z"), "a/b/c", "x/y/z"},
		{"order follows time, not the file", []review.Rename{{"a/d/c", "x/y/z", at(2)}, {"a/b/c", "a/d/c", at(1)}}, liveSet("x/y/z"), "a/b/c", "x/y/z"},
		{"a move back keeps the old id where it is", []review.Rename{{"a/b/c", "a/d/c", at(1)}, {"a/d/c", "a/b/c", at(2)}}, liveSet("a/b/c"), "a/b/c", "a/b/c"},
		{"a move back redirects the id in between", []review.Rename{{"a/b/c", "a/d/c", at(1)}, {"a/d/c", "a/b/c", at(2)}}, liveSet("a/b/c"), "a/d/c", "a/b/c"},
		{"a reused id is never redirected", []review.Rename{{"j/k/kana", "j/k/hira", at(1)}}, liveSet("j/k/kana", "j/k/hira"), "j/k/kana", "j/k/kana"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := review.NewRedirects(tt.renames, tt.live)

			got, _ := r.ID(tt.in)

			assert.Equal(t, tt.want, got)
		})
	}
}

func TestRedirects_All_LeavesOutReusedIds(t *testing.T) {
	t.Parallel()
	r := review.NewRedirects([]review.Rename{{"j/k/kana", "j/k/hira", at(1)}, {"j/k/old", "j/k/new", at(1)}}, liveSet("j/k/kana", "j/k/hira", "j/k/new"))

	assert.Equal(t, map[string]string{"j/k/old": "j/k/new"}, r.All())
}

func TestRedirects_CardAt(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		renames []review.Rename
		live    review.Live
		card    string
		written time.Time
		want    string
	}{
		{
			"a card follows its moved subject",
			[]review.Rename{{"a/b/c", "a/b/d", at(1)}}, liveSet("a/b/d"),
			"a/b/c:12", at(0), "a/b/d:12",
		},
		{
			"an explicit card rename wins over the subject",
			[]review.Rename{{"a/b/c:12", "a/b/e:12", at(1)}, {"a/b/c", "a/b/d", at(1)}}, liveSet("a/b/d", "a/b/e"),
			"a/b/c:12", at(0), "a/b/e:12",
		},
		{
			"a review after a proposal was stamped still follows once merged",
			[]review.Rename{{"a/b/c", "a/b/d", at(1)}}, liveSet("a/b/d"),
			"a/b/c:12", at(5), "a/b/d:12",
		},
		{
			"a card split out and merged back ends in the merged subject",
			[]review.Rename{{"j/k/kana:1b", "j/k/katakana:1b", at(1)}, {"j/k/katakana", "j/k/kana", at(2)}}, liveSet("j/k/kana", "j/k/kana:1b"),
			"j/k/kana:1b", at(0), "j/k/kana:1b",
		},
		{
			"a review written in the split part goes back with the merge",
			[]review.Rename{{"j/k/kana:1b", "j/k/katakana:1b", at(1)}, {"j/k/katakana", "j/k/kana", at(2)}}, liveSet("j/k/kana", "j/k/kana:1b"),
			"j/k/katakana:1b", at(1.5), "j/k/kana:1b",
		},
		{
			"a reused subject keeps the reviews written after the rename",
			[]review.Rename{{"j/k/kana", "j/k/hira", at(1)}}, liveSet("j/k/kana", "j/k/hira"),
			"j/k/kana:12", at(2), "j/k/kana:12",
		},
		{
			"a reused subject gives the reviews from before the rename to the moved one",
			[]review.Rename{{"j/k/kana", "j/k/hira", at(1)}}, liveSet("j/k/kana", "j/k/hira"),
			"j/k/kana:12", at(0), "j/k/hira:12",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := review.NewRedirects(tt.renames, tt.live)

			assert.Equal(t, tt.want, r.CardAt(tt.card, tt.written))
		})
	}
}

func TestBook_ReplaysReviewsUnderTheNewId_AfterARename(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	b, err := review.Open(root)
	require.NoError(t, err)
	b.SetLive(liveSet("x/y/z"))
	now := time.Now()
	_, err = b.Record("a/b/c:1234", review.Good, now.Add(-time.Hour))
	require.NoError(t, err)

	require.NoError(t, b.AppendRenames([]review.Rename{{From: "a/b/c", To: "x/y/z", At: now}}))

	assert.Equal(t, uint64(1), b.State("x/y/z:1234", now).Reps)
	assert.True(t, b.State("a/b/c:1234", now).IsNew)
	_, err = os.Stat(filepath.Join(root, review.RenamesPath))
	assert.NoError(t, err)
}

func TestBook_RecordKeepsTheIdItIsGiven(t *testing.T) {
	t.Parallel()
	b, err := review.Open(t.TempDir())
	require.NoError(t, err)
	b.SetLive(liveSet("j/k/kana", "j/k/hira", "j/k/kana:12"))
	require.NoError(t, b.AppendRenames([]review.Rename{{From: "j/k/kana", To: "j/k/hira", At: time.Now().Add(-time.Hour)}}))

	st, err := b.Record("j/k/kana:12", review.Good, time.Now())

	require.NoError(t, err)
	assert.Equal(t, "j/k/kana:12", st.ID)
	assert.Equal(t, uint64(1), b.State("j/k/kana:12", time.Now()).Reps)
}
