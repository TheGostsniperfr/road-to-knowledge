package httpapi_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/TheGostsniperfr/road-to-knowledge/internal/app"
)

func TestGetPaths_ListsCoursesWithTheirNextStep(t *testing.T) {
	ts, _ := server(t)

	res := do(t, "GET", ts.URL+"/api/paths", "")

	require.Equal(t, http.StatusOK, res.StatusCode)
	var got []app.PathSummary
	require.NoError(t, json.NewDecoder(res.Body).Decode(&got))
	require.Len(t, got, 1)
	require.NotNil(t, got[0].Next)
	assert.Equal(t, "l1", got[0].Next.ID)
}

func TestPatchPath_MovesTheCursorAndCommits(t *testing.T) {
	ts, root := server(t)

	res := do(t, "PATCH", ts.URL+"/api/paths/genki", `{"at":"l2"}`)

	require.Equal(t, http.StatusOK, res.StatusCode)
	assert.Equal(t, "chore(path): genki at l2", lastCommit(t, root))
}

func TestPatchPath_Returns404_WhenThePathDoesNotExist(t *testing.T) {
	ts, _ := server(t)

	res := do(t, "PATCH", ts.URL+"/api/paths/ccna", `{"at":"l2"}`)

	assert.Equal(t, http.StatusNotFound, res.StatusCode)
}
