package httpapi_test

import (
	"archive/zip"
	"bytes"
	"io"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAnkiExport_ReturnsAnApkgAttachment(t *testing.T) {
	ts, _ := server(t)

	res := do(t, http.MethodGet, ts.URL+"/api/anki/export?scope=lang", "")

	require.Equal(t, http.StatusOK, res.StatusCode)
	assert.Equal(t, `attachment; filename="road-to-knowledge-lang.apkg"`, res.Header.Get("Content-Disposition"))
	body, err := io.ReadAll(res.Body)
	require.NoError(t, err)
	zr, err := zip.NewReader(bytes.NewReader(body), int64(len(body)))
	require.NoError(t, err)
	var names []string
	for _, f := range zr.File {
		names = append(names, f.Name)
	}
	assert.ElementsMatch(t, []string{"collection.anki2", "media"}, names)
}

func TestAnkiExport_Returns404_WhenScopeIsUnknown(t *testing.T) {
	ts, _ := server(t)

	res := do(t, http.MethodGet, ts.URL+"/api/anki/export?scope=nope", "")

	assert.Equal(t, http.StatusNotFound, res.StatusCode)
}
