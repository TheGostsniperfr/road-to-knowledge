package app_test

import (
	"context"
	"errors"
	"fmt"
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
)

func write(t *testing.T, root, rel, content string) {
	t.Helper()
	full := filepath.Join(root, rel)
	require.NoError(t, os.MkdirAll(filepath.Dir(full), 0o755))
	require.NoError(t, os.WriteFile(full, []byte(content), 0o644))
}

func TestRestructure_NeverRedirectsAnIdThatIsLiveAgain(t *testing.T) {
	svc, root := restructureRepo(t)
	ctx := context.Background()
	_, err := svc.Restructure(ctx, app.Op{Kind: "move", ID: "tech/k8s/helm", To: "tech/k8s/helm-v2"}, false, "")
	require.NoError(t, err)
	write(t, root, "galaxies/tech/k8s/helm.md", "---\nname: Helm 4\nlevel: 1\n---\nNew.\n\nnew? :: yes\n")
	require.NoError(t, svc.Reload(ctx))

	reviewed(t, svc, root, "new?")

	v, err := svc.Star("tech/k8s/helm")
	require.NoError(t, err)
	assert.Equal(t, "Helm 4", v.Name)
	assert.NotContains(t, svc.Universe().Redirects, "tech/k8s/helm")
	assert.Equal(t, uint64(1), reps(t, svc, root, "new?"))
	log, err := os.ReadFile(filepath.Join(root, review.LogPath))
	require.NoError(t, err)
	assert.Contains(t, string(log), `"card":"tech/k8s/helm:`)
}

func TestProposeRestructure_ReviewsMadeBeforeTheMergeFollowTheRename(t *testing.T) {
	svc, root := restructureRepo(t)
	ctx := context.Background()
	plan, err := svc.ProposeRestructure(ctx, app.Op{Kind: "move", ID: "tech/k8s/helm", To: "tech/nix"}, "", false)
	require.NoError(t, err)
	time.Sleep(1100 * time.Millisecond) // the review lands strictly after the rename's timestamp
	reviewed(t, svc, root, "helm?")

	gitIn(t, root, "merge", "-q", "--no-edit", plan.Branch)
	require.NoError(t, svc.Reload(ctx))

	assert.True(t, strings.HasPrefix(cardID(t, root, "helm?"), "tech/nix/helm:"))
	assert.Equal(t, uint64(1), reps(t, svc, root, "helm?"))
}

func TestRestructure_LeavesTheTreeAsItWas_WhenAStepFails(t *testing.T) {
	op := app.Op{Kind: "merge", Sources: []string{"tech/k8s/argo", "tech/k8s/helm"}, Into: "tech/k8s/deploy"}
	probe, _ := restructureRepo(t)
	plan, err := probe.Restructure(context.Background(), op, true, "")
	require.NoError(t, err)
	for step := 0; step <= len(plan.Files); step++ {
		t.Run(fmt.Sprintf("failure before step %d", step), func(t *testing.T) {
			svc, root := restructureRepo(t)
			app.SetApplyHook(svc, func(i int) error {
				if i == step {
					return errors.New("disk full")
				}
				return nil
			})

			_, err := svc.Restructure(context.Background(), op, false, "")

			require.ErrorContains(t, err, "disk full")
			assert.Empty(t, gitIn(t, root, "status", "--porcelain", "--untracked-files=all"))
			assert.False(t, exists(root, review.RenamesPath))
			assert.Equal(t, "1", gitIn(t, root, "rev-list", "--count", "HEAD"))
		})
	}
}

func TestRestructure_LeavesTheTreeAsItWas_WhenTheCommitFails(t *testing.T) {
	svc, root := restructureRepo(t)
	write(t, root, ".git/hooks/pre-commit", "#!/bin/sh\nexit 1\n")
	require.NoError(t, os.Chmod(filepath.Join(root, ".git/hooks/pre-commit"), 0o755))

	_, err := svc.Restructure(context.Background(), app.Op{Kind: "move", ID: "tech/k8s/helm", To: "tech/nix"}, false, "")

	require.Error(t, err)
	assert.Empty(t, gitIn(t, root, "status", "--porcelain", "--untracked-files=all"))
	v, err := svc.Star("tech/k8s/helm")
	require.NoError(t, err)
	assert.Equal(t, "tech/k8s/helm", v.ID)
}

func TestRestructure_KeepsASubThemeItEmpties(t *testing.T) {
	svc, root := restructureRepo(t)
	ctx := context.Background()
	_, err := svc.Restructure(ctx, app.Op{Kind: "move", ID: "tech/k8s/argo", To: "tech/nix"}, false, "")
	require.NoError(t, err)

	plan, err := svc.Restructure(ctx, app.Op{Kind: "move", ID: "tech/k8s/helm", To: "tech/nix"}, false, "")

	require.NoError(t, err)
	assert.Contains(t, strings.Join(plan.Summary, "\n"), "tech/k8s garde son dossier")
	assert.True(t, exists(root, "galaxies/tech/k8s/_cluster.md"))
	u, _, err := kb.Load(root)
	require.NoError(t, err)
	assert.NotNil(t, u.Cluster("tech/k8s"))
}

func TestRestructure_RewritesDeepLinksToANotionThatMoved(t *testing.T) {
	svc, root := restructureRepo(t)
	write(t, root, "galaxies/tech/nix/flakes.md", "---\nname: Flakes\nlevel: 4\n---\nSee [hooks](#/tech/k8s/helm#hooks) and [helm](#/tech/k8s/helm).\n")
	gitIn(t, root, "commit", "-qam", "deep links")
	require.NoError(t, svc.Reload(context.Background()))

	_, err := svc.Restructure(context.Background(), app.Op{Kind: "split", ID: "tech/k8s/helm", Parts: []app.SplitPart{{ID: "helm-hooks", Notions: []string{"Hooks"}}}}, false, "")

	require.NoError(t, err)
	b, err := os.ReadFile(filepath.Join(root, "galaxies/tech/nix/flakes.md"))
	require.NoError(t, err)
	assert.Contains(t, string(b), "(#/tech/k8s/helm-hooks#hooks)")
	assert.Contains(t, string(b), "(#/tech/k8s/helm)")
}

func TestRestructure_MergeKeepsCardLikeLinesInsideCodeBlocks(t *testing.T) {
	svc, root := restructureRepo(t)
	write(t, root, "galaxies/tech/k8s/argo.md", "---\nname: Argo\nlevel: 2\n---\nGitOps.\n\n```\nhelm? :: package manager\n```\n")
	gitIn(t, root, "commit", "-qam", "code")
	require.NoError(t, svc.Reload(context.Background()))

	_, err := svc.Restructure(context.Background(), app.Op{Kind: "merge", Sources: []string{"tech/k8s/helm", "tech/k8s/argo"}}, false, "")

	require.NoError(t, err)
	b, err := os.ReadFile(filepath.Join(root, "galaxies/tech/k8s/helm.md"))
	require.NoError(t, err)
	assert.Equal(t, 2, strings.Count(string(b), "helm? :: package manager"))
}

func TestRestructure_AppliesOnlyThePlanThatWasShown(t *testing.T) {
	svc, root := restructureRepo(t)
	ctx := context.Background()
	op := app.Op{Kind: "move", ID: "tech/k8s/helm", To: "tech/nix"}
	shown, err := svc.Restructure(ctx, op, true, "")
	require.NoError(t, err)
	write(t, root, "galaxies/tech/k8s/helm.md", "---\nname: Helm\nlevel: 3\n---\nEdited meanwhile.\n")

	_, err = svc.Restructure(ctx, op, false, shown.Hash)

	assert.ErrorIs(t, err, app.ErrPlanChanged)
	assert.True(t, exists(root, "galaxies/tech/k8s/helm.md"))
	again, err := svc.Restructure(ctx, op, true, "")
	require.NoError(t, err)
	_, err = svc.Restructure(ctx, op, false, again.Hash)
	assert.NoError(t, err)
}
