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
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/TheGostsniperfr/road-to-knowledge/internal/app"
	"github.com/TheGostsniperfr/road-to-knowledge/internal/httpapi"
)

func feedXML() string {
	h1 := time.Now().Add(-time.Hour).UTC().Format(time.RFC1123Z)
	h2 := time.Now().Add(-2 * time.Hour).UTC().Format(time.RFC1123Z)
	return `<?xml version="1.0"?><rss version="2.0"><channel><title>JP</title>
<item><title>Kana practice sheets</title><link>https://example.com/kana</link><guid>1</guid><pubDate>` + h1 + `</pubDate></item>
<item><title>Tokyo travel tips</title><link>https://example.com/tokyo</link><guid>2</guid><pubDate>` + h2 + `</pubDate></item>
</channel></rss>`
}

func newsServer(t *testing.T) (*httptest.Server, string) {
	t.Helper()
	feed := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte(feedXML())) }))
	t.Cleanup(feed.Close)
	root := knowledge(t)
	galaxy := "---\nname: Langues\nsources:\n  - url: " + feed.URL + "/jp.xml\n    name: JP news\n---\n"
	require.NoError(t, os.WriteFile(filepath.Join(root, "galaxies/lang/_galaxy.md"), []byte(galaxy), 0o644))
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	svc, err := app.New(context.Background(), root, log)
	require.NoError(t, err)
	ts := httptest.NewServer(httpapi.New(svc, log, nil, nil).Handler())
	t.Cleanup(ts.Close)
	res := do(t, http.MethodPost, ts.URL+"/api/news/fetch", "")
	require.Equal(t, http.StatusOK, res.StatusCode)
	return ts, root
}

func getNews(t *testing.T, url string) app.NewsView {
	t.Helper()
	res := do(t, http.MethodGet, url, "")
	require.Equal(t, http.StatusOK, res.StatusCode)
	var v app.NewsView
	require.NoError(t, json.NewDecoder(res.Body).Decode(&v))
	return v
}

func TestNews_ReturnsOnlyItemsAboutKnownSubjects_WhenNewsDiscoveryIsOff(t *testing.T) {
	ts, _ := newsServer(t)

	v := getNews(t, ts.URL+"/api/news")

	require.Len(t, v.Items, 1)
	assert.Equal(t, "Kana practice sheets", v.Items[0].Title)
	assert.Equal(t, "lang/japonais/kana", v.Items[0].Subjects[0].ID)
	assert.Equal(t, 1, v.Hidden)
	assert.Equal(t, "JP news", v.Sources[0].Name)
}

func TestNews_ReturnsEveryItem_WhenAllIsSet(t *testing.T) {
	ts, _ := newsServer(t)

	v := getNews(t, ts.URL+"/api/news?all=1")

	assert.Len(t, v.Items, 2)
}

func TestNews_FiltersByScopeAndDate(t *testing.T) {
	ts, _ := newsServer(t)

	inScope := getNews(t, ts.URL+"/api/news?all=1&scope=lang/japonais")
	future := getNews(t, ts.URL+"/api/news?all=1&since="+time.Now().AddDate(0, 0, 1).Format("2006-01-02"))

	assert.Len(t, inScope.Items, 1, "a sub-theme without its own feed sees the items that mention its subjects")
	assert.Empty(t, future.Items)
}

func TestNews_RejectsBadQueries(t *testing.T) {
	ts, _ := newsServer(t)
	tests := []struct {
		name   string
		query  string
		status int
	}{
		{"unknown scope", "scope=nope", http.StatusNotFound},
		{"unreadable since", "since=yesterday", http.StatusBadRequest},
		{"zero limit", "limit=0", http.StatusBadRequest},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res := do(t, http.MethodGet, ts.URL+"/api/news?"+tt.query, "")

			assert.Equal(t, tt.status, res.StatusCode)
		})
	}
}

func TestNewsFetch_KeepsTheCacheOutOfGitHistory(t *testing.T) {
	_, root := newsServer(t)

	out, err := exec.Command("git", "-C", root, "log", "--oneline").CombinedOutput()

	require.NoError(t, err)
	assert.Equal(t, 1, strings.Count(strings.TrimSpace(string(out)), "\n")+1, "fetching must not commit: %s", out)
	assert.FileExists(t, filepath.Join(root, app.NewsDir, "state.json"))
}
