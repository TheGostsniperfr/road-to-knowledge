package app_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/TheGostsniperfr/road-to-knowledge/internal/app"
	"github.com/TheGostsniperfr/road-to-knowledge/internal/kb"
)

func universe(t *testing.T, files map[string]string) *kb.Universe {
	t.Helper()
	root := t.TempDir()
	for p, c := range files {
		full := filepath.Join(root, p)
		require.NoError(t, os.MkdirAll(filepath.Dir(full), 0o755))
		require.NoError(t, os.WriteFile(full, []byte(c), 0o644))
	}
	u, problems, err := kb.Load(root)
	require.NoError(t, err)
	require.Empty(t, problems)
	return u
}

// tech/k8s: gvisor known, runwasi linked to it, wasm two hops away through runwasi.
// tech/reseau: ebpf fog with no known neighbour. art/peinture: aquarelle fog, other galaxy.
var sample = map[string]string{
	"galaxies/tech/_galaxy.md":           "---\nname: Tech\ndiscovery: { near: 60, two_hops: 30, off_piste: 10 }\n---\n",
	"galaxies/tech/k8s/gvisor.md":        "---\nname: gVisor\nlevel: 4\n---\n",
	"galaxies/tech/k8s/kata.md":          "---\nname: Kata\nlevel: 0\n---\n",
	"galaxies/tech/k8s/runwasi.md":       "---\nname: runwasi\nlevel: 1\nstate: idea\nlinks: [gvisor]\n---\n",
	"galaxies/tech/wasm/wasm.md":         "---\nname: WebAssembly\nlevel: 0\nlinks: [runwasi]\n---\n",
	"galaxies/tech/reseau/ebpf.md":       "---\nname: eBPF\nlevel: 0\n---\n",
	"galaxies/tech/reseau/cilium.md":     "---\nname: Cilium\nlevel: 2\n---\n",
	"galaxies/art/peinture/aquarelle.md": "---\nname: Aquarelle\nlevel: 0\n---\n",
}

func ids(f app.FrontierView) map[string]app.Bucket {
	out := map[string]app.Bucket{}
	for _, s := range f.Suggestions {
		out[s.ID] = s.Bucket
	}
	return out
}

func TestFrontier_ClassifiesCandidatesByDistanceToKnownSubjects(t *testing.T) {
	u := universe(t, sample)

	f, err := app.Frontier(u, "tech", 20)

	require.NoError(t, err)
	assert.Equal(t, map[string]app.Bucket{
		"tech/k8s/runwasi":       app.BucketNear,
		"tech/k8s/kata":          app.BucketNear,
		"tech/wasm/wasm":         app.BucketTwoHops,
		"tech/reseau/ebpf":       app.BucketOffPiste,
		"art/peinture/aquarelle": app.BucketOffPiste,
	}, ids(f))
}

func TestFrontier_ExplainsEachBucketInFrench(t *testing.T) {
	tests := []struct {
		name, id, reason string
	}{
		{"a linked subject names the known one", "tech/k8s/runwasi", "lié à gVisor que tu maîtrises"},
		{"a cluster mate names its neighbour", "tech/k8s/kata", "dans k8s, à côté de gVisor que tu maîtrises"},
		{"a two hop subject names the path", "tech/wasm/wasm", "à deux pas de gVisor, via runwasi"},
		{"an off-piste subject gives its cluster coverage", "tech/reseau/ebpf", "reseau (Tech), exploré à 0 %"},
	}
	u := universe(t, sample)
	f, err := app.Frontier(u, "tech", 20)
	require.NoError(t, err)
	reasons := map[string]string{}
	for _, s := range f.Suggestions {
		reasons[s.ID] = s.Reason
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.reason, reasons[tt.id])
		})
	}
}

func TestFrontier_RanksLinkedSubjectsBeforeClusterMates(t *testing.T) {
	u := universe(t, sample)

	f, err := app.Frontier(u, "tech", 20)

	require.NoError(t, err)
	assert.Equal(t, "tech/k8s/runwasi", f.Suggestions[0].ID)
}

func TestFrontier_FollowsTheBudget_WhenEveryBucketIsFull(t *testing.T) {
	files := map[string]string{
		"galaxies/g/_galaxy.md": "---\nname: G\ndiscovery: { near: 50, two_hops: 25, off_piste: 25 }\n---\n",
		"galaxies/g/a/known.md": "---\nname: Known\nlevel: 5\n---\n",
		"galaxies/g/a/mid.md":   "---\nname: Mid\nlevel: 2\nlinks: [known]\n---\n",
	}
	for _, n := range []string{"n1", "n2", "n3", "n4"} {
		files["galaxies/g/a/"+n+".md"] = "---\nname: " + n + "\nlevel: 0\n---\n"
	}
	for _, n := range []string{"t1", "t2", "t3"} {
		files["galaxies/g/b/"+n+".md"] = "---\nname: " + n + "\nlevel: 0\nlinks: [mid]\n---\n"
	}
	for _, n := range []string{"o1", "o2", "o3"} {
		files["galaxies/g/c/"+n+".md"] = "---\nname: " + n + "\nlevel: 0\n---\n"
	}
	u := universe(t, files)

	f, err := app.Frontier(u, "g", 4)

	require.NoError(t, err)
	count := map[app.Bucket]int{}
	for _, s := range f.Suggestions {
		count[s.Bucket]++
	}
	assert.Equal(t, map[app.Bucket]int{app.BucketNear: 2, app.BucketTwoHops: 1, app.BucketOffPiste: 1}, count)
}

func TestFrontier_GivesUnusedSlotsToOtherBuckets(t *testing.T) {
	u := universe(t, sample)

	f, err := app.Frontier(u, "tech/k8s", 3)

	require.NoError(t, err)
	assert.Len(t, f.Suggestions, 3, "k8s has only two near candidates, the third slot goes off piste")
}

func TestFrontier_LooksOneLevelUpForOffPiste_WhenScopeIsACluster(t *testing.T) {
	u := universe(t, sample)

	f, err := app.Frontier(u, "tech/k8s", 20)

	require.NoError(t, err)
	got := ids(f)
	assert.Equal(t, app.BucketOffPiste, got["tech/reseau/ebpf"])
	assert.NotContains(t, got, "art/peinture/aquarelle", "another galaxy is two levels up from a cluster")
}

func TestFrontier_NeverSuggestsKnownOrInProgressSubjects(t *testing.T) {
	u := universe(t, sample)

	f, err := app.Frontier(u, "", 50)

	require.NoError(t, err)
	got := ids(f)
	assert.NotContains(t, got, "tech/k8s/gvisor")
	assert.NotContains(t, got, "tech/reseau/cilium")
}

func TestFrontier_ReturnsNotFound_WhenScopeIsUnknown(t *testing.T) {
	u := universe(t, sample)

	_, err := app.Frontier(u, "nope", 5)

	assert.ErrorIs(t, err, app.ErrNotFound)
}

func TestFrontier_IsDeterministic(t *testing.T) {
	u := universe(t, sample)

	a, _ := app.Frontier(u, "", 9)
	b, _ := app.Frontier(u, "", 9)

	assert.Equal(t, a, b)
}

func TestFrontier_AlternatesThemes_WhenScopeIsTheUniverse(t *testing.T) {
	files := map[string]string{
		"galaxies/big/a/k.md":   "---\nname: K\nlevel: 4\n---\n",
		"galaxies/small/b/k.md": "---\nname: K2\nlevel: 3\n---\n",
		"galaxies/small/b/z.md": "---\nname: Z\nlevel: 0\n---\n",
	}
	for _, n := range []string{"a1", "a2", "a3"} {
		files["galaxies/big/a/"+n+".md"] = "---\nname: " + n + "\nlevel: 0\n---\n"
	}
	u := universe(t, files)

	f, err := app.Frontier(u, "", 2)

	require.NoError(t, err)
	assert.Equal(t, []string{"big/a/a1", "small/b/z"}, []string{f.Suggestions[0].ID, f.Suggestions[1].ID})
}
