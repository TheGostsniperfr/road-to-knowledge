package kb_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/TheGostsniperfr/road-to-knowledge/internal/kb"
)

func TestCheckFeedURL(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		url   string
		valid bool
	}{
		{"accepts https", "https://go.dev/blog/feed.atom", true},
		{"accepts http", "http://example.com/rss", true},
		{"rejects file scheme", "file:///etc/passwd", false},
		{"rejects a missing host", "https:///feed", false},
		{"rejects credentials in the URL", "https://me:secret@example.com/rss", false},
		{"rejects a relative path", "feed.xml", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := kb.CheckFeedURL(tt.url)

			assert.Equal(t, tt.valid, err == nil, "err: %v", err)
		})
	}
}

func TestLoad_ReadsSourcesOfThemesAndSubThemes_AndReportsBadOnes(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "galaxies/tech/_galaxy.md", "---\nname: Tech\nsources:\n  - url: https://go.dev/blog/feed.atom\n    name: Go blog\n  - url: file:///etc/passwd\n  - url: https://go.dev/blog/feed.atom\n---\n")
	writeFile(t, root, "galaxies/tech/k8s/_cluster.md", "---\nname: K8s\nsources:\n  - url: https://kubernetes.io/feed.xml\n---\n")
	writeFile(t, root, "galaxies/tech/k8s/cilium.md", "---\nname: Cilium\nlevel: 3\n---\n")

	u, problems, err := kb.Load(root)

	require.NoError(t, err)
	assert.Equal(t, []kb.Feed{{URL: "https://go.dev/blog/feed.atom", Name: "Go blog"}}, u.Galaxy("tech").Sources)
	assert.Equal(t, []kb.Feed{{URL: "https://kubernetes.io/feed.xml"}}, u.Cluster("tech/k8s").Sources)
	require.Len(t, problems, 1)
	assert.Contains(t, problems[0].Message, "file:///etc/passwd")
}
