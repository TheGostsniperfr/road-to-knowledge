package app_test

import (
	"context"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/TheGostsniperfr/road-to-knowledge/internal/app"
	"github.com/TheGostsniperfr/road-to-knowledge/internal/kb"
	"github.com/TheGostsniperfr/road-to-knowledge/internal/review"
	"github.com/TheGostsniperfr/road-to-knowledge/internal/semantic"
)

var restructureFiles = map[string]string{
	"galaxies/tech/k8s/helm.md":   "---\nname: Helm\nlevel: 3\n---\nCharts.\n\nhelm? :: package manager\n\n## Templates\n\ntpl? :: go templates\n\n## Hooks\n\nhook? :: lifecycle\n",
	"galaxies/tech/k8s/argo.md":   "---\nname: Argo\nlevel: 2\nlinks: [helm]\n---\nGitOps.\n\nargo? :: gitops\n",
	"galaxies/tech/nix/flakes.md": "---\nname: Flakes\nlevel: 4\nlinks: [tech/k8s/helm]\n---\nPinned inputs.\n",
}

func restructureRepo(t *testing.T) (*app.Service, string) {
	t.Helper()
	root := t.TempDir()
	for p, c := range restructureFiles {
		full := filepath.Join(root, p)
		require.NoError(t, os.MkdirAll(filepath.Dir(full), 0o755))
		require.NoError(t, os.WriteFile(full, []byte(c), 0o644))
	}
	gitIn(t, root, "init", "-q", "-b", "main")
	gitIn(t, root, "config", "user.email", "t@e.st")
	gitIn(t, root, "config", "user.name", "t")
	gitIn(t, root, "config", "commit.gpgsign", "false")
	gitIn(t, root, "add", ".")
	gitIn(t, root, "commit", "-qm", "init")
	svc, err := app.New(context.Background(), root, slog.New(slog.NewTextHandler(io.Discard, nil)))
	require.NoError(t, err)
	return svc, root
}

// cardID finds a card by question in the repo as it is on disk now.
func cardID(t *testing.T, root, question string) string {
	t.Helper()
	u, _, err := kb.Load(root)
	require.NoError(t, err)
	for _, c := range u.Cards() {
		if c.Question == question {
			return c.ID
		}
	}
	t.Fatalf("no card %q", question)
	return ""
}

func reviewed(t *testing.T, svc *app.Service, root, question string) {
	t.Helper()
	require.NoError(t, svc.Review(context.Background(), cardID(t, root, question), int(review.Good)))
	svc.FlushReviews(context.Background(), 1)
}

func reps(t *testing.T, svc *app.Service, root, question string) uint64 {
	t.Helper()
	cards, err := svc.Due("", nil)
	require.NoError(t, err)
	id := cardID(t, root, question)
	for _, c := range cards {
		if c.ID == id {
			return 0 // still due as a new card: its history was lost
		}
	}
	return 1
}

func exists(root, rel string) bool {
	_, err := os.Stat(filepath.Join(root, rel))
	return err == nil
}

func links(t *testing.T, root, rel string) []string {
	t.Helper()
	var m struct {
		Links []string `yaml:"links"`
	}
	_, err := kb.ReadFile(filepath.Join(root, rel), &m)
	require.NoError(t, err)
	return m.Links
}

func TestRestructure_MovesASubjectIntoAnotherSubTheme_WithOneCommit(t *testing.T) {
	svc, root := restructureRepo(t)
	before := gitIn(t, root, "rev-list", "--count", "HEAD")

	_, err := svc.Restructure(context.Background(), app.Op{Kind: "move", ID: "tech/k8s/helm", To: "tech/nix"}, false, "")

	require.NoError(t, err)
	assert.True(t, exists(root, "galaxies/tech/nix/helm.md"))
	assert.False(t, exists(root, "galaxies/tech/k8s/helm.md"))
	assert.Equal(t, "2", gitIn(t, root, "rev-list", "--count", "HEAD"))
	assert.Equal(t, "1", before)
	assert.Empty(t, gitIn(t, root, "status", "--porcelain"))
}

func TestRestructure_KeepsReviewHistory_AfterEachOperation(t *testing.T) {
	tests := []struct {
		name     string
		op       app.Op
		question string
	}{
		{"move", app.Op{Kind: "move", ID: "tech/k8s/helm", To: "tech/nix/helm"}, "helm?"},
		{"rename", app.Op{Kind: "move", ID: "tech/k8s/helm", To: "tech/k8s/helm-charts"}, "tpl?"},
		{"split", app.Op{Kind: "split", ID: "tech/k8s/helm", Parts: []app.SplitPart{{ID: "helm-hooks", Notions: []string{"Hooks"}}}}, "hook?"},
		{"merge", app.Op{Kind: "merge", Sources: []string{"tech/k8s/argo", "tech/k8s/helm"}, Into: "tech/k8s/helm"}, "argo?"},
		{"merge into a new id", app.Op{Kind: "merge", Sources: []string{"tech/k8s/argo", "tech/k8s/helm"}, Into: "tech/k8s/deploy"}, "helm?"},
		{"sub-theme move", app.Op{Kind: "move", ID: "tech/k8s", To: "tech/kubernetes"}, "argo?"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc, root := restructureRepo(t)
			reviewed(t, svc, root, tt.question)
			require.Equal(t, uint64(1), reps(t, svc, root, tt.question))

			_, err := svc.Restructure(context.Background(), tt.op, false, "")

			require.NoError(t, err)
			assert.Equal(t, uint64(1), reps(t, svc, root, tt.question), "the card reviewed before must not be new again")
		})
	}
}

func TestRestructure_KeepsReviewHistory_WhenANotionIsSplitOutAndMergedBack(t *testing.T) {
	svc, root := restructureRepo(t)
	reviewed(t, svc, root, "hook?")
	ctx := context.Background()

	_, err := svc.Restructure(ctx, app.Op{Kind: "split", ID: "tech/k8s/helm", Parts: []app.SplitPart{{ID: "helm-hooks", Notions: []string{"Hooks"}}}}, false, "")
	require.NoError(t, err)
	_, err = svc.Restructure(ctx, app.Op{Kind: "merge", Sources: []string{"tech/k8s/helm-hooks", "tech/k8s/helm"}, Into: "tech/k8s/helm"}, false, "")
	require.NoError(t, err)

	assert.Equal(t, "tech/k8s/helm", cardID(t, root, "hook?")[:len("tech/k8s/helm")])
	assert.Equal(t, uint64(1), reps(t, svc, root, "hook?"))
	body, err := os.ReadFile(filepath.Join(root, "galaxies/tech/k8s/helm.md"))
	require.NoError(t, err)
	assert.Contains(t, string(body), "\n## Hooks\n")
	assert.NotContains(t, string(body), "Détaché de")
}

func TestRestructure_RewritesLinksToTheMovedSubject(t *testing.T) {
	svc, root := restructureRepo(t)

	_, err := svc.Restructure(context.Background(), app.Op{Kind: "move", ID: "tech/k8s/helm", To: "tech/nix/helm-charts"}, false, "")

	require.NoError(t, err)
	assert.Equal(t, []string{"tech/nix/helm-charts"}, links(t, root, "galaxies/tech/k8s/argo.md"))
	assert.Equal(t, []string{"tech/nix/helm-charts"}, links(t, root, "galaxies/tech/nix/flakes.md"))
	_, problems, err := kb.Load(root)
	require.NoError(t, err)
	assert.Empty(t, problems)
}

func TestRestructure_OpensOldIdsThroughRedirects(t *testing.T) {
	svc, _ := restructureRepo(t)

	_, err := svc.Restructure(context.Background(), app.Op{Kind: "move", ID: "tech/k8s/helm", To: "tech/nix"}, false, "")

	require.NoError(t, err)
	v, err := svc.Star("tech/k8s/helm")
	require.NoError(t, err)
	assert.Equal(t, "tech/nix/helm", v.ID)
	assert.Equal(t, "tech/nix/helm", svc.Universe().Redirects["tech/k8s/helm"])
}

func TestRestructure_KeepsGitHistoryOfTheMovedFile(t *testing.T) {
	svc, root := restructureRepo(t)

	_, err := svc.Restructure(context.Background(), app.Op{Kind: "move", ID: "tech/k8s/helm", To: "tech/nix"}, false, "")

	require.NoError(t, err)
	log := gitIn(t, root, "log", "--follow", "--format=%s", "--", "galaxies/tech/nix/helm.md")
	assert.Equal(t, 2, len(strings.Split(log, "\n")), log)
}

func TestRestructure_DryRunChangesNothing(t *testing.T) {
	svc, root := restructureRepo(t)

	plan, err := svc.Restructure(context.Background(), app.Op{Kind: "merge", Sources: []string{"tech/k8s/argo", "tech/k8s/helm"}}, true, "")

	require.NoError(t, err)
	assert.NotEmpty(t, plan.Files)
	assert.True(t, exists(root, "galaxies/tech/k8s/helm.md"))
	assert.Empty(t, gitIn(t, root, "status", "--porcelain"))
	assert.False(t, exists(root, review.RenamesPath))
}

func TestRestructure_MergeKeepsTheHighestLevel(t *testing.T) {
	svc, root := restructureRepo(t)

	_, err := svc.Restructure(context.Background(), app.Op{Kind: "merge", Sources: []string{"tech/k8s/helm", "tech/k8s/argo"}, Into: "tech/k8s/argo"}, false, "")

	require.NoError(t, err)
	v, err := svc.Star("tech/k8s/argo")
	require.NoError(t, err)
	assert.Equal(t, 3, v.Level)
	assert.False(t, exists(root, "galaxies/tech/k8s/helm.md"))
	assert.Equal(t, 4, v.Cards)
	assert.Equal(t, []string{"tech/k8s/argo"}, links(t, root, "galaxies/tech/nix/flakes.md"))
}

func TestRestructure_SplitMovesNotionsWithTheirCards(t *testing.T) {
	svc, _ := restructureRepo(t)

	_, err := svc.Restructure(context.Background(), app.Op{Kind: "split", ID: "tech/k8s/helm", Parts: []app.SplitPart{{ID: "helm-hooks", Notions: []string{"hooks"}}}}, false, "")

	require.NoError(t, err)
	part, err := svc.Star("tech/k8s/helm-hooks")
	require.NoError(t, err)
	orig, err := svc.Star("tech/k8s/helm")
	require.NoError(t, err)
	assert.Equal(t, "Hooks", part.Name)
	assert.Equal(t, 3, part.Level)
	assert.Equal(t, 1, part.Cards)
	assert.Equal(t, 2, orig.Cards)
	assert.Contains(t, part.Links, "tech/k8s/helm")
	assert.NotContains(t, orig.Body, "## Hooks")
}

func TestRestructure_RefusesBadOperations(t *testing.T) {
	tests := []struct {
		name string
		op   app.Op
	}{
		{"collision with an existing subject", app.Op{Kind: "move", ID: "tech/k8s/argo", To: "tech/k8s/helm"}},
		{"target outside galaxies", app.Op{Kind: "move", ID: "tech/k8s/argo", To: "../../etc/passwd"}},
		{"sub-theme that does not exist", app.Op{Kind: "move", ID: "tech/k8s/argo", To: "tech/nowhere/argo"}},
		{"unknown subject", app.Op{Kind: "move", ID: "tech/k8s/nope", To: "tech/nix"}},
		{"split with an unknown notion", app.Op{Kind: "split", ID: "tech/k8s/helm", Parts: []app.SplitPart{{ID: "x", Notions: []string{"Nope"}}}}},
		{"split giving a notion twice", app.Op{Kind: "split", ID: "tech/k8s/helm", Parts: []app.SplitPart{{ID: "x", Notions: []string{"Hooks"}}, {ID: "y", Notions: []string{"Hooks"}}}}},
		{"merge of one subject", app.Op{Kind: "merge", Sources: []string{"tech/k8s/helm"}}},
		{"unknown kind", app.Op{Kind: "explode", ID: "tech/k8s/helm"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc, root := restructureRepo(t)

			_, err := svc.Restructure(context.Background(), tt.op, false, "")

			require.Error(t, err)
			assert.Empty(t, gitIn(t, root, "status", "--porcelain"))
		})
	}
}

func TestRestructure_RefusesAMergeWhereOneQuestionHasTwoAnswers(t *testing.T) {
	svc, root := restructureRepo(t)
	p := filepath.Join(root, "galaxies/tech/k8s/argo.md")
	require.NoError(t, os.WriteFile(p, []byte("---\nname: Argo\nlevel: 2\n---\nGitOps.\n\nhelm? :: a chart tool\n"), 0o644))

	_, err := svc.Restructure(context.Background(), app.Op{Kind: "merge", Sources: []string{"tech/k8s/helm", "tech/k8s/argo"}}, false, "")

	assert.ErrorIs(t, err, app.ErrInvalid)
}

func TestProposeRestructure_WritesABranchAndLeavesMainAlone(t *testing.T) {
	svc, root := restructureRepo(t)
	head := gitIn(t, root, "rev-parse", "HEAD")

	plan, err := svc.ProposeRestructure(context.Background(), app.Op{Kind: "merge", Sources: []string{"tech/k8s/argo", "tech/k8s/helm"}, Into: "tech/k8s/helm"}, "", false)

	require.NoError(t, err)
	assert.Equal(t, head, gitIn(t, root, "rev-parse", "HEAD"))
	assert.True(t, exists(root, "galaxies/tech/k8s/argo.md"))
	files := gitIn(t, root, "diff", "--name-status", "main", plan.Branch)
	assert.Contains(t, files, "D\tgalaxies/tech/k8s/argo.md")
	assert.Contains(t, files, "review/renames.jsonl")
}

func TestRestructure_KeepsNotionVectorsCachedAcrossAMove(t *testing.T) {
	svc, root := restructureRepo(t)
	u, _, err := kb.Load(root)
	require.NoError(t, err)
	before := map[string]bool{}
	for _, c := range semantic.Chunks(u) {
		before[c.Hash] = true
	}

	_, err = svc.Restructure(context.Background(), app.Op{Kind: "move", ID: "tech/k8s/helm", To: "tech/k8s/helm-charts"}, false, "")

	require.NoError(t, err)
	u, _, err = kb.Load(root)
	require.NoError(t, err)
	for _, c := range semantic.Chunks(u) {
		assert.True(t, before[c.Hash], "chunk %s must reuse its cached vector", c.ID)
	}
}

func TestRestructure_RecordsRenamesWithTheirTime(t *testing.T) {
	svc, root := restructureRepo(t)

	plan, err := svc.Restructure(context.Background(), app.Op{Kind: "move", ID: "tech/k8s/helm", To: "tech/nix"}, false, "")

	require.NoError(t, err)
	require.Len(t, plan.Renames, 1)
	assert.WithinDuration(t, time.Now(), plan.Renames[0].At, time.Minute)
	b, err := os.ReadFile(filepath.Join(root, review.RenamesPath))
	require.NoError(t, err)
	assert.Contains(t, string(b), `"from":"tech/k8s/helm","to":"tech/nix/helm"`)
}
