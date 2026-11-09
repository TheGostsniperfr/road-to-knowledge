package httpapi_test

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/TheGostsniperfr/road-to-knowledge/internal/app"
	"github.com/TheGostsniperfr/road-to-knowledge/internal/httpapi"
	"github.com/TheGostsniperfr/road-to-knowledge/internal/mcpserver"
)

func knowledge(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	files := map[string]string{
		"galaxies/lang/_galaxy.md":        "---\nname: Langues\nreview_after_days: 30\n---\n",
		"galaxies/lang/japonais/kana.md":  "---\nname: Kana\nlevel: 4\nreviewed: 2026-01-01\n---\nDeux syllabaires.\n\nあ :: a\nか :: ka\n\n## Hiragana\nRonds.\n",
		"galaxies/lang/japonais/keigo.md": "---\nname: Keigo\nlevel: 0\n---\nPolitesse.\n",
		"assets/kana.svg":                 "<svg/>",
	}
	for p, c := range files {
		full := filepath.Join(root, p)
		require.NoError(t, os.MkdirAll(filepath.Dir(full), 0o755))
		require.NoError(t, os.WriteFile(full, []byte(c), 0o644))
	}
	for _, args := range [][]string{{"init", "-q", "-b", "main"}, {"config", "user.email", "t@e.st"}, {"config", "user.name", "t"}, {"add", "."}, {"commit", "-qm", "init"}} {
		cmd := exec.Command("git", args...)
		cmd.Dir = root
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, string(out))
	}
	return root
}

func server(t *testing.T) (*httptest.Server, string) {
	t.Helper()
	root := knowledge(t)
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	svc, err := app.New(context.Background(), root, log)
	require.NoError(t, err)
	ts := httptest.NewServer(httpapi.New(svc, log, nil, nil).Handler())
	t.Cleanup(ts.Close)
	return ts, root
}

func do(t *testing.T, method, url, body string) *http.Response {
	t.Helper()
	req, err := http.NewRequest(method, url, strings.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	res, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	t.Cleanup(func() { res.Body.Close() })
	return res
}

func TestHandler_ServesAppAndMCPTogether(t *testing.T) {
	root := knowledge(t)
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	svc, err := app.New(context.Background(), root, log)
	require.NoError(t, err)
	web := fstest.MapFS{"index.html": {Data: []byte("<html>rtk</html>")}}
	ts := httptest.NewServer(httpapi.New(svc, log, web, mcpserver.New(svc, "test")).Handler())
	t.Cleanup(ts.Close)

	res := do(t, "GET", ts.URL+"/galaxie/profonde", "")

	body, _ := io.ReadAll(res.Body)
	assert.Equal(t, "<html>rtk</html>", string(body), "unknown paths fall back to the single-page app")
}

func TestUniverse_ListsGalaxiesWithStaleHint(t *testing.T) {
	ts, _ := server(t)

	res := do(t, "GET", ts.URL+"/api/universe", "")

	var u app.UniverseView
	require.NoError(t, json.NewDecoder(res.Body).Decode(&u))
	require.Len(t, u.Galaxies, 1)
	stars := u.Galaxies[0].Clusters[0].Stars
	require.Len(t, stars, 2)
	assert.True(t, stars[0].Stale, "kana was reviewed long ago with a 30 day budget")
	assert.Equal(t, 4, stars[0].Level, "staleness never lowers the level")
}

func TestPatchStar_UpdatesLevelAndCommits(t *testing.T) {
	ts, root := server(t)

	res := do(t, "PATCH", ts.URL+"/api/star/lang/japonais/keigo", `{"level":2,"state":"progress"}`)

	require.Equal(t, http.StatusOK, res.StatusCode)
	cmd := exec.Command("git", "log", "-1", "--format=%s")
	cmd.Dir = root
	out, err := cmd.Output()
	require.NoError(t, err)
	assert.Equal(t, "chore(knowledge): Keigo level 2, progress", strings.TrimSpace(string(out)))
}

func TestPatchStar_Returns400_WhenLevelIsOutOfRange(t *testing.T) {
	ts, _ := server(t)

	res := do(t, "PATCH", ts.URL+"/api/star/lang/japonais/keigo", `{"level":8}`)

	assert.Equal(t, http.StatusBadRequest, res.StatusCode)
}

func TestStar_Returns404_WhenUnknown(t *testing.T) {
	ts, _ := server(t)

	res := do(t, "GET", ts.URL+"/api/star/lang/nope", "")

	assert.Equal(t, http.StatusNotFound, res.StatusCode)
}

func TestReview_RemovesCardFromDueList(t *testing.T) {
	ts, _ := server(t)
	var before []app.DueCard
	require.NoError(t, json.NewDecoder(do(t, "GET", ts.URL+"/api/review/due?scope=lang/japonais/kana", "").Body).Decode(&before))
	require.Len(t, before, 2)

	res := do(t, "POST", ts.URL+"/api/review", `{"card":"`+before[0].ID+`","rating":4}`)

	require.Equal(t, http.StatusNoContent, res.StatusCode)
	var after []app.DueCard
	require.NoError(t, json.NewDecoder(do(t, "GET", ts.URL+"/api/review/due?scope=lang/japonais/kana", "").Body).Decode(&after))
	assert.Len(t, after, 1)
}

func TestDue_HonoursExcludedChildren(t *testing.T) {
	ts, _ := server(t)

	var cards []app.DueCard
	require.NoError(t, json.NewDecoder(do(t, "GET", ts.URL+"/api/review/due?scope=lang&exclude=lang/japonais", "").Body).Decode(&cards))

	assert.Empty(t, cards)
}

func TestCapture_WritesInboxFile(t *testing.T) {
	ts, root := server(t)

	res := do(t, "POST", ts.URL+"/api/capture", `{"text":"regarder Kueue"}`)

	require.Equal(t, http.StatusCreated, res.StatusCode)
	files, _ := filepath.Glob(filepath.Join(root, "inbox", "*.md"))
	assert.Len(t, files, 1)
}

func TestAIPreview_IsMuchSmallerThanWholeGraph(t *testing.T) {
	ts, _ := server(t)

	var out struct {
		Call       string `json:"call"`
		Tokens     int    `json:"tokens"`
		FullTokens int    `json:"full_tokens"`
	}
	require.NoError(t, json.NewDecoder(do(t, "GET", ts.URL+"/api/ai?tool=node&id=lang/japonais/kana", "").Body).Decode(&out))

	assert.Equal(t, `node("lang/japonais/kana")`, out.Call)
	assert.Positive(t, out.Tokens)
}

func TestAssets_BlocksTraversal(t *testing.T) {
	ts, _ := server(t)

	ok := do(t, "GET", ts.URL+"/assets/kana.svg", "")
	bad := do(t, "GET", ts.URL+"/assets/..%2fgalaxies/lang/japonais/kana.md", "")

	assert.Equal(t, http.StatusOK, ok.StatusCode)
	assert.NotEqual(t, http.StatusOK, bad.StatusCode)
}

func TestGuard_RejectsForeignHost(t *testing.T) {
	ts, _ := server(t)
	req, _ := http.NewRequest("GET", ts.URL+"/api/universe", nil)
	req.Host = "evil.example"

	res, err := http.DefaultClient.Do(req)

	require.NoError(t, err)
	defer res.Body.Close()
	assert.Equal(t, http.StatusForbidden, res.StatusCode)
}

func TestGuard_RejectsCrossSiteWrite(t *testing.T) {
	ts, _ := server(t)
	req, _ := http.NewRequest("POST", ts.URL+"/api/capture", strings.NewReader(`{"text":"x"}`))
	req.Header.Set("Origin", "https://evil.example")

	res, err := http.DefaultClient.Do(req)

	require.NoError(t, err)
	defer res.Body.Close()
	assert.Equal(t, http.StatusForbidden, res.StatusCode)
}
