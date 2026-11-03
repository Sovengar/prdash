package sim

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"prdash/internal/forge/model"
	"prdash/internal/testutil"
)

// localClone returns a clone of the fixture's bare remote, with the forge's PR ref published there.
func localClone(t *testing.T) (repo, local string) {
	t.Helper()
	repo, _, _ = fixture(t)
	head := strings.TrimSpace(testutil.RunGit(t, repo, "rev-parse", "refs/heads/prdash/pr-7"))
	testutil.RunGit(t, repo, "update-ref", "refs/pull/7/head", head)

	local = filepath.Join(t.TempDir(), "local")
	testutil.RunGit(t, filepath.Dir(local), "clone", "--quiet", repo, local)
	return repo, local
}

func TestSimulateFetchesTheReviewWhenNoWorktreeIsMounted(t *testing.T) {
	_, local := localClone(t)
	it := itemForSim()
	svc := newService(t, stubLocator{ok: true, place: Place{Repo: local, Source: "refs/pull/7/head"}}, fakeSim(t, writeJPEG(t)))

	res, err := svc.Simulate(context.Background(), it, KindMerge)
	if err != nil {
		t.Fatalf("Simulate without a mounted review: %v", err)
	}
	if res.Path == "" {
		t.Fatal("Simulate returned no image")
	}
	if res.Ref != simBranch {
		t.Errorf("Ref = %q, want the fetched %s branch", res.Ref, simBranch)
	}

	// The fetch happened in the throwaway clone: the user's clone keeps its branches.
	branches := testutil.RunGit(t, local, "branch", "--format=%(refname)")
	if strings.Contains(branches, simBranch) {
		t.Errorf("the user's clone holds %s after the render:\n%s", simBranch, branches)
	}
	if strings.TrimSpace(branches) != "refs/heads/main" {
		t.Errorf("the user's clone has extra branches:\n%s", branches)
	}
}

func TestSimulateFetchesTheReviewForARebaseToo(t *testing.T) {
	_, local := localClone(t)
	it := itemForSim()
	svc := newService(t, stubLocator{ok: true, place: Place{Repo: local, Source: "refs/pull/7/head"}}, fakeSim(t, writeJPEG(t)))

	res, err := svc.Simulate(context.Background(), it, KindRebase)
	if err != nil {
		t.Fatalf("Simulate(rebase) without a mounted review: %v", err)
	}
	if res.Ref != "main" {
		t.Errorf("Ref = %q, want the base branch", res.Ref)
	}
}

func TestSimulateWithoutAMountFailsWhenTheFetchFails(t *testing.T) {
	_, local := localClone(t)
	it := itemForSim()
	svc := newService(t, stubLocator{ok: true, place: Place{Repo: local, Source: "refs/pull/999/head"}}, fakeSim(t, writeJPEG(t)))

	_, err := svc.Simulate(context.Background(), it, KindMerge)
	if err == nil {
		t.Fatal("Simulate rendered a ref the remote does not have")
	}
	if !strings.Contains(err.Error(), "fetch") {
		t.Errorf("the error %q does not say the fetch failed", err)
	}
}

func TestSimulateWithoutAMountNeedsACloneWithAnOrigin(t *testing.T) {
	repo, _, _ := fixture(t)
	it := itemForSim()
	svc := newService(t, stubLocator{ok: true, place: Place{Repo: repo, Source: "refs/pull/7/head"}}, fakeSim(t, writeJPEG(t)))

	_, err := svc.Simulate(context.Background(), it, KindMerge)
	if err == nil {
		t.Fatal("Simulate fetched from a clone with no origin")
	}
	if !strings.Contains(err.Error(), "origin") {
		t.Errorf("the error %q does not mention the missing origin", err)
	}
}

func TestSimulateStillNeedsABranchOrASource(t *testing.T) {
	_, local := localClone(t)
	svc := newService(t, stubLocator{ok: true, place: Place{Repo: local}}, fakeSim(t, writeJPEG(t)))

	_, err := svc.Simulate(context.Background(), itemForSim(), KindMerge)
	if err == nil || !strings.Contains(err.Error(), "mount the review first") {
		t.Errorf("err = %v, want the mount-first guard", err)
	}
}

func itemForSim() model.Item {
	it := model.NewItem(model.RepoRef{Forge: "github", Host: "github.com", Project: "acme/widget"}, 7)
	it.SourceBranch = "feat/x"
	it.TargetBranch = "main"
	return it
}
