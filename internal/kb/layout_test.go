package kb_test

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/TheGostsniperfr/road-to-knowledge/internal/kb"
)

func TestLoad_AppliesPinnedPositionsFromLayoutFile(t *testing.T) {
	root := fixture(t)
	writeFile(t, root, "layout.json", `{"galaxies":{"lang":{"x":120,"y":-40,"rotation":1.5}}}`)

	u, problems, err := kb.Load(root)

	require.NoError(t, err)
	require.Empty(t, problems)
	require.NotNil(t, u.Galaxy("lang").Pin)
	assert.Equal(t, 120.0, u.Galaxy("lang").Pin.X)
	assert.Nil(t, u.Galaxy("books").Pin, "galaxies without a pin are placed by the app")
}

func TestLoad_ReportsProblems_WhenLayoutIsWrong(t *testing.T) {
	tests := []struct{ name, layout, want string }{
		{"unknown galaxy", `{"galaxies":{"cooking":{"x":0,"y":0}}}`, `unknown galaxy "cooking"`},
		{"out of range", `{"galaxies":{"lang":{"x":1e9,"y":0}}}`, "within"},
		{"unknown field", `{"galaxies":{},"zoom":2}`, "invalid JSON"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := fixture(t)
			writeFile(t, root, "layout.json", tt.layout)

			_, problems, err := kb.Load(root)

			require.NoError(t, err)
			require.Len(t, problems, 1)
			assert.Equal(t, "layout.json", problems[0].Path)
			assert.Contains(t, problems[0].Message, tt.want)
		})
	}
}

func TestLoad_ReportsProblem_WhenDiscoveryDoesNotSumToHundred(t *testing.T) {
	root := fixture(t)
	writeFile(t, root, "galaxies/lang/_galaxy.md", "---\nname: Langues\ndiscovery: { near: 70, two_hops: 20, off_piste: 20 }\n---\n")

	u, problems, err := kb.Load(root)

	require.NoError(t, err)
	require.Len(t, problems, 1)
	assert.Contains(t, problems[0].Message, "sum to 100")
	assert.Equal(t, 60, u.Galaxy("lang").Discovery.Near, "an invalid budget falls back to the default")
}

func TestWriteLayout_RoundTripsThroughLoad(t *testing.T) {
	root := fixture(t)
	rot := 0.25

	_, err := kb.WriteLayout(root, kb.Layout{Galaxies: map[string]kb.Pin{"lang": {X: 1, Y: 2, Rotation: &rot}, "books": {X: -3, Y: 4}}})
	require.NoError(t, err)
	u, problems, err := kb.Load(root)

	require.NoError(t, err)
	require.Empty(t, problems)
	assert.Equal(t, kb.Pin{X: 1, Y: 2, Rotation: &rot}, *u.Galaxy("lang").Pin)
}

func TestSetFields_WritesMapsInFlowStyle(t *testing.T) {
	root := t.TempDir()
	p := writeFile(t, root, "_galaxy.md", "---\nname: Langues\n---\nBody.\n")

	require.NoError(t, kb.SetFields(p, map[string]any{"discovery": kb.Discovery{Near: 50, TwoHops: 30, OffPiste: 20}}))

	out, err := os.ReadFile(p)
	require.NoError(t, err)
	assert.Contains(t, string(out), "discovery: {near: 50, two_hops: 30, off_piste: 20, news: false}")
}
