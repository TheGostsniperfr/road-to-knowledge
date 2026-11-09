package mcpserver_test

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/TheGostsniperfr/road-to-knowledge/internal/app"
	"github.com/TheGostsniperfr/road-to-knowledge/internal/mcpserver"
)

func TestNews_ReturnsRecentItemsWithTheSubjectsTheyMention(t *testing.T) {
	pub := time.Now().Add(-3 * time.Hour).UTC().Format(time.RFC1123Z)
	old := time.Now().AddDate(0, 0, -5).UTC().Format(time.RFC1123Z)
	feed := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`<rss version="2.0"><channel><title>K8s</title>
<item><title>gVisor on GKE</title><link>https://example.com/gvisor</link><pubDate>` + pub + `</pubDate></item>
<item><title>gVisor history</title><link>https://example.com/old</link><pubDate>` + old + `</pubDate></item>
</channel></rss>`))
	}))
	defer feed.Close()
	root := t.TempDir()
	for p, c := range map[string]string{
		"galaxies/tech/_galaxy.md":    "---\nname: Tech\nsources:\n  - url: " + feed.URL + "\n    name: K8s blog\n---\n",
		"galaxies/tech/k8s/gvisor.md": "---\nname: gVisor\nlevel: 2\n---\nNoyau en espace utilisateur.\n",
	} {
		require.NoError(t, os.MkdirAll(filepath.Dir(filepath.Join(root, p)), 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(root, p), []byte(c), 0o644))
	}
	ctx := context.Background()
	svc, err := app.New(ctx, root, slog.New(slog.NewTextHandler(io.Discard, nil)))
	require.NoError(t, err)
	_, err = svc.FetchNews(ctx)
	require.NoError(t, err)
	st, ct := mcp.NewInMemoryTransports()
	_, err = mcpserver.New(svc, "test").Connect(ctx, st, nil)
	require.NoError(t, err)
	cs, err := mcp.NewClient(&mcp.Implementation{Name: "test"}, nil).Connect(ctx, ct, nil)
	require.NoError(t, err)
	defer cs.Close()

	out, isErr := call(t, cs, "news", map[string]any{})

	require.False(t, isErr, out)
	var got struct {
		Items []struct {
			Title    string   `json:"title"`
			Subjects []string `json:"subjects"`
		} `json:"items"`
		Feeds int `json:"feeds"`
	}
	require.NoError(t, json.Unmarshal([]byte(out), &got))
	require.Len(t, got.Items, 1, "the default window is two days")
	assert.Equal(t, "gVisor on GKE", got.Items[0].Title)
	assert.Equal(t, []string{"gVisor (level 2)"}, got.Items[0].Subjects)
	assert.Equal(t, 1, got.Feeds)
}

func TestNews_ReportsAnUnreadableSince(t *testing.T) {
	cs, _ := session(t)

	out, isErr := call(t, cs, "news", map[string]any{"since": "last week"})

	assert.True(t, isErr)
	assert.Contains(t, out, "since")
}
