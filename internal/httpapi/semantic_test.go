package httpapi_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSemantic_ReportsOff_WhenNoModelIsEnabled(t *testing.T) {
	ts, _ := server(t)

	res := do(t, http.MethodGet, ts.URL+"/api/semantic", "")

	require.Equal(t, http.StatusOK, res.StatusCode)
	var st map[string]any
	require.NoError(t, json.NewDecoder(res.Body).Decode(&st))
	assert.Equal(t, "off", st["state"])
}

func TestNeighbors_Returns503_WhileSearchByMeaningIsNotReady(t *testing.T) {
	ts, _ := server(t)

	res := do(t, http.MethodGet, ts.URL+"/api/neighbors/lang/japonais/kana", "")

	assert.Equal(t, http.StatusServiceUnavailable, res.StatusCode)
}

func TestNeighbors_Returns404_ForAnUnknownSubject(t *testing.T) {
	ts, _ := server(t)

	res := do(t, http.MethodGet, ts.URL+"/api/neighbors/nope/nope/nope", "")

	assert.Equal(t, http.StatusNotFound, res.StatusCode)
}
