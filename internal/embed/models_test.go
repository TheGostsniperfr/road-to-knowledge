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
