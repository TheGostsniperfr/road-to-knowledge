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

var t0 = time.Date(2026, 11, 1, 9, 0, 0, 0, time.UTC)

func TestState_IsNewAndDue_WhenCardWasNeverReviewed(t *testing.T) {
	b, err := review.Open(t.TempDir())
	require.NoError(t, err)

	st := b.State("kana:1", t0)

	assert.True(t, st.IsNew)
	assert.True(t, b.IsDue("kana:1", t0))
}

func TestRecord_SchedulesGoodCardInTheFuture(t *testing.T) {
	b, err := review.Open(t.TempDir())
	require.NoError(t, err)

	st, err := b.Record("kana:1", review.Good, t0)

	require.NoError(t, err)
	assert.True(t, st.Due.After(t0))
}

func TestReload_RebuildsSameStateFromLog(t *testing.T) {
	root := t.TempDir()
	b, err := review.Open(root)
	require.NoError(t, err)
	_, err = b.Record("kana:1", review.Good, t0)
	require.NoError(t, err)
	want, err := b.Record("kana:1", review.Easy, t0.Add(48*time.Hour))
	require.NoError(t, err)

	again, err := review.Open(root)
	require.NoError(t, err)

	assert.Equal(t, want.Due, again.State("kana:1", t0).Due)
}

func TestReload_IgnoresLineOrder_WhenMergeInterleavesMachines(t *testing.T) {
	root := t.TempDir()
	log := filepath.Join(root, review.LogPath)
	require.NoError(t, os.MkdirAll(filepath.Dir(log), 0o755))
	require.NoError(t, os.WriteFile(log, []byte(
		`{"card":"c","rating":4,"at":"2026-11-03T09:00:00Z"}`+"\n"+
			`{"card":"c","rating":3,"at":"2026-11-01T09:00:00Z"}`+"\n"), 0o644))
	sorted := t.TempDir()
	log2 := filepath.Join(sorted, review.LogPath)
	require.NoError(t, os.MkdirAll(filepath.Dir(log2), 0o755))
	require.NoError(t, os.WriteFile(log2, []byte(
		`{"card":"c","rating":3,"at":"2026-11-01T09:00:00Z"}`+"\n"+
			`{"card":"c","rating":4,"at":"2026-11-03T09:00:00Z"}`+"\n"), 0o644))

	a, err := review.Open(root)
	require.NoError(t, err)
	b, err := review.Open(sorted)
	require.NoError(t, err)

	assert.Equal(t, b.State("c", t0).Due, a.State("c", t0).Due)
}

func TestRecord_RejectsInvalidRating(t *testing.T) {
	b, err := review.Open(t.TempDir())
	require.NoError(t, err)

	_, err = b.Record("c", 7, t0)

	assert.Error(t, err)
}

func TestPreview_GivesLongerIntervalForEasyThanAgain(t *testing.T) {
	b, err := review.Open(t.TempDir())
	require.NoError(t, err)

	p := b.Preview("c", t0)

	assert.Greater(t, p[review.Easy], p[review.Again])
}
