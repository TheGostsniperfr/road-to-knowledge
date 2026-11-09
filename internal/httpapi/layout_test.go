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

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/TheGostsniperfr/road-to-knowledge/internal/app"
	"github.com/TheGostsniperfr/road-to-knowledge/internal/httpapi"
)

func lastCommit(t *testing.T, root string) string {
	t.Helper()
	cmd := exec.Command("git", "log", "-1", "--format=%s")
	cmd.Dir = root
	out, err := cmd.Output()
	require.NoError(t, err)
	return strings.TrimSpace(string(out))
}

func TestPutLayout_PinsGalaxiesAndCommits(t *testing.T) {
	ts, root := server(t)

	res := do(t, "PUT", ts.URL+"/api/layout", `{"galaxies":{"lang":{"x":100,"y":-50}}}`)

	require.Equal(t, http.StatusOK, res.StatusCode)
	var u app.UniverseView
	require.NoError(t, json.NewDecoder(res.Body).Decode(&u))
	require.NotNil(t, u.Galaxies[0].Pin)
	assert.Equal(t, 100.0, u.Galaxies[0].Pin.X)
	assert.Equal(t, "chore(layout): pin galaxies", lastCommit(t, root))
}

func TestPutLayout_Returns404_WhenGalaxyIsUnknown(t *testing.T) {
	ts, root := server(t)

	res := do(t, "PUT", ts.URL+"/api/layout", `{"galaxies":{"cooking":{"x":0,"y":0}}}`)

	assert.Equal(t, http.StatusNotFound, res.StatusCode)
	assert.NoFileExists(t, filepath.Join(root, "layout.json"))
}

func TestPatchGalaxy_WritesSettingsAndCommits(t *testing.T) {
	ts, root := server(t)

	res := do(t, "PATCH", ts.URL+"/api/galaxy/lang", `{"review_after_days":21,"discovery":{"near":70,"two_hops":20,"off_piste":10,"news":true}}`)

	require.Equal(t, http.StatusOK, res.StatusCode)
	var g app.GalaxyView
	require.NoError(t, json.NewDecoder(res.Body).Decode(&g))
	assert.Equal(t, 21, g.ReviewAfter)
	assert.True(t, g.Discovery.News)
	src, err := os.ReadFile(filepath.Join(root, "galaxies/lang/_galaxy.md"))
	require.NoError(t, err)
	assert.Contains(t, string(src), "name: Langues", "other keys are kept")
	assert.Equal(t, "chore(knowledge): Langues settings", lastCommit(t, root))
}

func TestPatchGalaxy_Returns400_WhenDiscoveryDoesNotSumToHundred(t *testing.T) {
	ts, _ := server(t)

	res := do(t, "PATCH", ts.URL+"/api/galaxy/lang", `{"discovery":{"near":70,"two_hops":20,"off_piste":20,"news":false}}`)

	assert.Equal(t, http.StatusBadRequest, res.StatusCode)
}

func TestPatchGalaxy_CreatesGalaxyFile_WhenItIsMissing(t *testing.T) {
	root := knowledge(t)
	require.NoError(t, os.MkdirAll(filepath.Join(root, "galaxies/cuisine/bases"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, "galaxies/cuisine/bases/riz.md"), []byte("---\nname: Riz\n---\n"), 0o644))
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	svc, err := app.New(context.Background(), root, log)
	require.NoError(t, err)
	ts := httptest.NewServer(httpapi.New(svc, log, nil, nil).Handler())
	t.Cleanup(ts.Close)

	res := do(t, "PATCH", ts.URL+"/api/galaxy/cuisine", `{"review_after_days":60}`)

	require.Equal(t, http.StatusOK, res.StatusCode)
	assert.FileExists(t, filepath.Join(root, "galaxies/cuisine/_galaxy.md"))
}
