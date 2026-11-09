package httpapi_test

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRestructure_DryRunReturnsThePlanAndWritesNothing(t *testing.T) {
	ts, root := server(t)

	res := do(t, "POST", ts.URL+"/api/restructure", `{"kind":"move","id":"lang/japonais/keigo","to":"lang/japonais/keigo-politesse","dry_run":true}`)

	require.Equal(t, http.StatusOK, res.StatusCode)
	var plan struct {
		Files []struct{ Kind, Path, From string }
	}
	require.NoError(t, json.NewDecoder(res.Body).Decode(&plan))
	require.Len(t, plan.Files, 1)
	assert.Equal(t, "galaxies/lang/japonais/keigo-politesse.md", plan.Files[0].Path)
	assert.FileExists(t, filepath.Join(root, "galaxies/lang/japonais/keigo.md"))
}

func TestRestructure_AppliesAndRedirectsTheOldId(t *testing.T) {
	ts, root := server(t)

	res := do(t, "POST", ts.URL+"/api/restructure", `{"kind":"move","id":"lang/japonais/keigo","to":"lang/japonais/keigo-politesse","name":"Keigo (politesse)"}`)

	require.Equal(t, http.StatusOK, res.StatusCode)
	_, err := os.Stat(filepath.Join(root, "galaxies/lang/japonais/keigo.md"))
	assert.True(t, os.IsNotExist(err))
	star := do(t, "GET", ts.URL+"/api/star/lang/japonais/keigo", "")
	require.Equal(t, http.StatusOK, star.StatusCode)
	var v struct{ ID, Name string }
	require.NoError(t, json.NewDecoder(star.Body).Decode(&v))
	assert.Equal(t, "lang/japonais/keigo-politesse", v.ID)
	assert.Equal(t, "Keigo (politesse)", v.Name)
}

func TestRestructure_Returns409_WhenFilesChangedSinceThePlanWasShown(t *testing.T) {
	ts, root := server(t)
	res := do(t, "POST", ts.URL+"/api/restructure", `{"kind":"move","id":"lang/japonais/keigo","to":"lang/japonais/keigo-politesse","dry_run":true}`)
	require.Equal(t, http.StatusOK, res.StatusCode)
	var plan struct{ Hash string }
	require.NoError(t, json.NewDecoder(res.Body).Decode(&plan))
	require.NoError(t, os.WriteFile(filepath.Join(root, "galaxies/lang/japonais/keigo.md"), []byte("---\nname: Keigo\n---\nChanged.\n"), 0o644))

	res = do(t, "POST", ts.URL+"/api/restructure", `{"kind":"move","id":"lang/japonais/keigo","to":"lang/japonais/keigo-politesse","plan_hash":"`+plan.Hash+`"}`)

	assert.Equal(t, http.StatusConflict, res.StatusCode)
	assert.FileExists(t, filepath.Join(root, "galaxies/lang/japonais/keigo.md"))
}

func TestRestructure_Returns400_ForACollision(t *testing.T) {
	ts, _ := server(t)

	res := do(t, "POST", ts.URL+"/api/restructure", `{"kind":"move","id":"lang/japonais/keigo","to":"lang/japonais/kana"}`)

	assert.Equal(t, http.StatusBadRequest, res.StatusCode)
}
