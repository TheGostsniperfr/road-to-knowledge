package embed

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func serve(t *testing.T, body string) Spec {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/org/model/resolve/abc123/onnx/model.onnx" {
			http.NotFound(w, r)
			return
		}
		w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	old := hubURL
	hubURL = srv.URL
	t.Cleanup(func() { hubURL = old })
	t.Setenv("RTK_MODELS", t.TempDir())
	sum := sha256.Sum256([]byte("weights"))
	return Spec{ID: "tiny", Repo: "org/model", Revision: "abc123", Files: []File{{"onnx/model.onnx", hex.EncodeToString(sum[:]), 7}}}
}

func TestPull_StoresAFileWhoseHashMatchesThePin(t *testing.T) {
	spec := serve(t, "weights")

	err := spec.Pull(context.Background(), nil)

	require.NoError(t, err)
	assert.NoError(t, spec.Present())
	dir, _ := spec.Dir()
	got, _ := os.ReadFile(filepath.Join(dir, "model.onnx"))
	assert.Equal(t, "weights", string(got))
}

func TestPull_RefusesAFileThatDoesNotMatchThePin(t *testing.T) {
	spec := serve(t, "tampere")

	err := spec.Pull(context.Background(), nil)

	assert.ErrorContains(t, err, "does not match its pinned hash")
	assert.ErrorIs(t, spec.Present(), ErrMissing)
}

func TestLookup_ListsTheKnownModels_WhenTheIDIsUnknown(t *testing.T) {
	_, err := Lookup("nope")

	assert.ErrorContains(t, err, "multilingual-e5-small")
}

func TestModels_PinEveryFileWithAHashAndASize(t *testing.T) {
	for id, spec := range Models {
		for _, f := range spec.Files {
			assert.Len(t, f.SHA256, 64, "%s %s", id, f.Path)
			assert.Positive(t, f.Size, "%s %s", id, f.Path)
		}
		assert.Len(t, spec.Revision, 40, id)
	}
}

func TestVerify_RemovesACorruptFileSoPullRepairsIt(t *testing.T) {
	spec := serve(t, "weights")
	require.NoError(t, spec.Pull(context.Background(), nil))
	dir, _ := spec.Dir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "model.onnx"), []byte("WEIGHTS"), 0o644))
	require.NoError(t, spec.Present(), "same size, so only a rehash can tell")

	bad, err := spec.Verify()
	require.NoError(t, err)
	require.NoError(t, spec.Pull(context.Background(), nil))

	assert.Equal(t, []string{"onnx/model.onnx"}, bad)
	got, _ := os.ReadFile(filepath.Join(dir, "model.onnx"))
	assert.Equal(t, "weights", string(got))
}

func TestPull_CleansTempFilesLeftByAnInterruptedPull(t *testing.T) {
	spec := serve(t, "weights")
	dir, _ := spec.Dir()
	require.NoError(t, os.MkdirAll(dir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".pull-123"), []byte("half"), 0o644))

	require.NoError(t, spec.Pull(context.Background(), nil))

	left, _ := filepath.Glob(filepath.Join(dir, ".pull-*"))
	assert.Empty(t, left)
}
