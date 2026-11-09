package httpapi_test

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

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/TheGostsniperfr/road-to-knowledge/internal/app"
	"github.com/TheGostsniperfr/road-to-knowledge/internal/httpapi"
)

func bookServer(t *testing.T, withPDF bool) *httptest.Server {
	t.Helper()
	root := knowledge(t)
	fiche := "---\nname: Genki\nlevel: 1\nsource:\n  title: Genki I\n  pages: 400\n  file: library/genki-0123abcd.pdf\n---\nManuel.\n"
	require.NoError(t, os.WriteFile(filepath.Join(root, "galaxies/lang/japonais/genki.md"), []byte(fiche), 0o644))
	if withPDF {
		require.NoError(t, os.MkdirAll(filepath.Join(root, "library"), 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(root, "library/genki-0123abcd.pdf"), []byte("%PDF"), 0o644))
	}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	svc, err := app.New(context.Background(), root, log)
	require.NoError(t, err)
	ts := httptest.NewServer(httpapi.New(svc, log, nil, nil).Handler())
	t.Cleanup(ts.Close)
	return ts
}

func sourceOf(t *testing.T, ts *httptest.Server) map[string]any {
	t.Helper()
	res := do(t, "GET", ts.URL+"/api/star/lang/japonais/genki", "")
	require.Equal(t, http.StatusOK, res.StatusCode)
	var v struct {
		Source map[string]any `json:"source"`
	}
	require.NoError(t, json.NewDecoder(res.Body).Decode(&v))
	return v.Source
}

func TestStar_SaysTheSourceIsAvailable_WhenThePDFIsInTheLibrary(t *testing.T) {
	ts := bookServer(t, true)

	src := sourceOf(t, ts)

	assert.Equal(t, "Genki I", src["title"])
	assert.Equal(t, true, src["available"])
}

func TestStar_SaysTheSourceIsMissing_WhenThePDFIsOnAnotherMachine(t *testing.T) {
	ts := bookServer(t, false)

	src := sourceOf(t, ts)

	assert.Equal(t, false, src["available"])
}

func TestStaticFolders_RefuseSymlinksThatLeaveTheFolder(t *testing.T) {
	root := knowledge(t)
	secret := filepath.Join(t.TempDir(), "secret.txt")
	require.NoError(t, os.WriteFile(secret, []byte("token"), 0o644))
	require.NoError(t, os.MkdirAll(filepath.Join(root, "library"), 0o755))
	require.NoError(t, os.Symlink(secret, filepath.Join(root, "library", "leak.pdf")))
	require.NoError(t, os.Symlink(secret, filepath.Join(root, "assets", "leak.svg")))
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	svc, err := app.New(context.Background(), root, log)
	require.NoError(t, err)
	ts := httptest.NewServer(httpapi.New(svc, log, nil, nil).Handler())
	t.Cleanup(ts.Close)

	for _, url := range []string{"/library/leak.pdf", "/assets/leak.svg"} {
		res := do(t, "GET", ts.URL+url, "")
		body, _ := io.ReadAll(res.Body)

		assert.NotEqual(t, http.StatusOK, res.StatusCode, url)
		assert.NotContains(t, string(body), "token", url)
	}
}

func TestStaticFolders_AreSandboxed(t *testing.T) {
	ts := bookServer(t, true)

	for _, url := range []string{"/library/genki-0123abcd.pdf", "/assets/kana.svg"} {
		res := do(t, "GET", ts.URL+url, "")

		assert.Equal(t, http.StatusOK, res.StatusCode, url)
		assert.Equal(t, "sandbox", res.Header.Get("Content-Security-Policy"), url)
	}
}

func TestLibrary_Returns404_WhenNoBookWasAddedYet(t *testing.T) {
	ts := bookServer(t, false)

	res := do(t, "GET", ts.URL+"/library/genki-0123abcd.pdf", "")

	assert.Equal(t, http.StatusNotFound, res.StatusCode)
}

func TestLibrary_ServesTheOriginalAndBlocksTraversal(t *testing.T) {
	ts := bookServer(t, true)

	ok := do(t, "GET", ts.URL+"/library/genki-0123abcd.pdf", "")
	bad := do(t, "GET", ts.URL+"/library/..%2fgalaxies/lang/japonais/kana.md", "")

	assert.Equal(t, http.StatusOK, ok.StatusCode)
	assert.NotEqual(t, http.StatusOK, bad.StatusCode)
}
