package app_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/TheGostsniperfr/road-to-knowledge/internal/app"
	"github.com/TheGostsniperfr/road-to-knowledge/internal/gitx"
	"github.com/TheGostsniperfr/road-to-knowledge/internal/kb"
)

const coursePath = `---
name: Cours K8s
at: s2
steps:
  - id: s1
    name: Helm
    covers: [tech/k8s/helm]
  - id: s2
    name: Hooks
    covers: [tech/k8s/helm#hooks]
    note: relire
  - id: s3
    name: Argo
    covers: [tech/k8s/argo]
---
Un cours.
`

func pathsRepo(t *testing.T) (*app.Service, string) {
	t.Helper()
	svc, root := restructureRepo(t)
	full := filepath.Join(root, "paths", "k8s.md")
	require.NoError(t, os.MkdirAll(filepath.Dir(full), 0o755))
	require.NoError(t, os.WriteFile(full, []byte(coursePath), 0o644))
	gitIn(t, root, "add", ".")
	gitIn(t, root, "commit", "-qm", "path")
	require.NoError(t, svc.Reload(context.Background()))
	return svc, root
}

func TestPath_ShowsEachStepStatusAndTheNextStep(t *testing.T) {
	svc, _ := pathsRepo(t)

	v, err := svc.Path("k8s")

	require.NoError(t, err)
	assert.Equal(t, []kb.StepStatus{kb.StepDone, kb.StepCurrent, kb.StepTodo}, []kb.StepStatus{v.Steps[0].Status, v.Steps[1].Status, v.Steps[2].Status})
	assert.Equal(t, 1, v.Done)
	require.NotNil(t, v.Next)
	assert.Equal(t, "Helm · Hooks", v.Next.Covers[0].Name)
}

func TestPaths_ListsTheSubjectsEachPathCovers(t *testing.T) {
	svc, _ := pathsRepo(t)

	got := svc.Paths()

	require.Len(t, got, 1)
	assert.Equal(t, []string{"tech/k8s/argo", "tech/k8s/helm"}, got[0].Stars)
}

func TestSetPathAt_MovesTheCursorWithOneCommit(t *testing.T) {
	svc, root := pathsRepo(t)

	v, err := svc.SetPathAt(context.Background(), "k8s", "s3")

	require.NoError(t, err)
	assert.Equal(t, "s3", v.At)
	assert.Equal(t, "chore(path): k8s at s3", gitIn(t, root, "log", "-1", "--format=%s"))
}

func TestSetPathAt_RefusesAStepThePathDoesNotHave(t *testing.T) {
	svc, _ := pathsRepo(t)

	_, err := svc.SetPathAt(context.Background(), "k8s", "s9")

	assert.ErrorIs(t, err, app.ErrInvalid)
}

func TestStar_ListsTheCourseStepsThatCoverIt(t *testing.T) {
	svc, _ := pathsRepo(t)

	v, err := svc.Star("tech/k8s/helm")

	require.NoError(t, err)
	require.Len(t, v.Steps, 2)
	assert.Equal(t, "s2", v.Steps[1].Step)
	assert.Equal(t, kb.StepCurrent, v.Steps[1].Status)
}

func TestRestructure_RewritesPathCovers_WhenASubjectMoves(t *testing.T) {
	svc, root := pathsRepo(t)

	_, err := svc.Restructure(context.Background(), app.Op{Kind: "move", ID: "tech/k8s/helm", To: "tech/nix/helm"}, false, "")

	require.NoError(t, err)
	u, problems, err := kb.Load(root)
	require.NoError(t, err)
	assert.Empty(t, problems)
	assert.Equal(t, []string{"tech/nix/helm#hooks"}, u.Path("k8s").Steps[1].Covers)
	assert.Equal(t, "relire", u.Path("k8s").Steps[1].Extra["note"], "keys the app does not know survive the rewrite")
}

func TestRestructure_RewritesNotionCovers_WhenASplitTakesTheNotion(t *testing.T) {
	svc, root := pathsRepo(t)

	_, err := svc.Restructure(context.Background(), app.Op{Kind: "split", ID: "tech/k8s/helm", Parts: []app.SplitPart{{ID: "helm-hooks", Notions: []string{"Hooks"}}}}, false, "")

	require.NoError(t, err)
	u, problems, err := kb.Load(root)
	require.NoError(t, err)
	assert.Empty(t, problems)
	assert.Equal(t, []string{"tech/k8s/helm-hooks#hooks"}, u.Path("k8s").Steps[1].Covers)
}

func TestPath_NeverRedirectsANotionOfASubjectThatExists(t *testing.T) {
	svc, root := pathsRepo(t)
	ctx := context.Background()
	_, err := svc.Restructure(ctx, app.Op{Kind: "move", ID: "tech/k8s/argo", To: "tech/nix/argo"}, false, "")
	require.NoError(t, err)
	full := filepath.Join(root, "galaxies/tech/k8s/argo.md")
	require.NoError(t, os.WriteFile(full, []byte("---\nname: Argo neuf\nlevel: 1\n---\nNeuf.\n\n## Rollouts\nCanary.\n"), 0o644))
	reused := strings.Replace(coursePath, "covers: [tech/k8s/argo]", "covers: [tech/k8s/argo#rollouts]", 1)
	require.NoError(t, os.WriteFile(filepath.Join(root, "paths/k8s.md"), []byte(reused), 0o644))
	require.NoError(t, svc.Reload(ctx))

	v, err := svc.Path("k8s")

	require.NoError(t, err)
	assert.Equal(t, []string{"tech/k8s/argo#rollouts"}, coverIDs(v.Steps[2]))
	assert.Equal(t, "Argo neuf · Rollouts", v.Steps[2].Covers[0].Name)
}

func TestPropose_AcceptsACursorMove_WhenThePathAlreadyHasABrokenCover(t *testing.T) {
	svc, root := pathsRepo(t)
	broken := strings.Replace(coursePath, "tech/k8s/argo]", "tech/k8s/nope]", 1)
	require.NoError(t, os.WriteFile(filepath.Join(root, "paths/k8s.md"), []byte(broken), 0o644))
	gitIn(t, root, "commit", "-qam", "broken cover")
	require.NoError(t, svc.Reload(context.Background()))
	moved := strings.Replace(broken, "at: s2", "at: s3", 1)

	_, err := svc.Propose(context.Background(), "k8s-cursor", "chore(path): k8s at s3", []gitx.FileChange{{Path: "paths/k8s.md", Content: moved}})

	assert.NoError(t, err)
}

func TestPropose_RefusesAPathThatAddsABrokenCover(t *testing.T) {
	svc, _ := pathsRepo(t)
	broken := strings.Replace(coursePath, "tech/k8s/argo]", "tech/k8s/nope]", 1)

	_, err := svc.Propose(context.Background(), "k8s-broken", "chore(path): k8s", []gitx.FileChange{{Path: "paths/k8s.md", Content: broken}})

	assert.ErrorIs(t, err, app.ErrInvalid)
}

func coverIDs(s app.StepView) []string {
	var out []string
	for _, c := range s.Covers {
		out = append(out, c.ID)
	}
	return out
}
