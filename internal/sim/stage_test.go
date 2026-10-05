package sim

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"prdash/internal/testutil"
)

func simRepoMount(t *testing.T) (repo, tmp string) {
	t.Helper()
	repo = filepath.Join(t.TempDir(), "repo")
	testutil.InitRepo(t, repo)
	testutil.CommitFile(t, repo, "base.txt", "base", "base")
	testutil.SetRemote(t, repo, "origin", repo)
	testutil.CommitFile(t, repo, "feat.txt", "feat", "feat")
	if _, err := os.Stat(filepath.Join(repo, ".git")); err != nil {
		t.Fatal(err)
	}
	git(t, repo, "branch", "feat/x")
	git(t, repo, "branch", "main-origin", "main")
	tmp = t.TempDir()
	return repo, tmp
}

func git(t *testing.T, dir string, args ...string) string {
	t.Helper()
	return testutil.RunGit(t, dir, args...)
}

func TestStageLeavesTheBaseActiveForMergeAndTheItemBranchForRebase(t *testing.T) {
	ctx := context.Background()

	repo, tmp := simRepoMount(t)
	s := New(locatorForStage())
	path, spec, err := s.stage(ctx, Place{Repo: repo, Branch: "feat/x"}, KindMerge, "main", tmp)
	if err != nil {
		t.Fatalf("merge: %v", err)
	}
	if got := git(t, path, "rev-parse", "--abbrev-ref", "HEAD"); got != "main" {
		t.Errorf("merge left %q active, want main: a merge's graph is the base with "+
			"the branch on top", got)
	}
	if spec.Ref != "feat/x" {
		t.Errorf("the drawn ref is %q, want the item's branch", spec.Ref)
	}
	if spec.Kind != KindMerge {
		t.Errorf("the spec does not carry the mode: %+v", spec)
	}

	repo, tmp = simRepoMount(t)
	path, spec, err = s.stage(ctx, Place{Repo: repo, Branch: "feat/x"}, KindRebase, "main", tmp)
	if err != nil {
		t.Fatalf("rebase: %v", err)
	}
	if got := git(t, path, "rev-parse", "--abbrev-ref", "HEAD"); got != "feat/x" {
		t.Errorf("rebase left %q active, want the item's branch: a rebase is computed "+
			"from the branch towards the base", got)
	}
	if spec.Ref != "main" {
		t.Errorf("the drawn ref is %q, want the base", spec.Ref)
	}
}

// The check goes BEFORE the clone; the order is what is pinned.
func TestStageDoesNotCloneIfTheItemBranchIsUnknown(t *testing.T) {
	repo, tmp := simRepoMount(t)
	tmpBefore := countEntries(t, tmp)

	_, _, err := New(locatorForStage()).stage(context.Background(),
		Place{Repo: repo, Branch: ""}, KindMerge, "main", tmp)

	if err == nil {
		t.Fatal("without the item's branch it gave nil")
	}
	if !strings.Contains(err.Error(), "branch") {
		t.Errorf("the error %q does not say the branch is missing", err)
	}
	if !strings.Contains(err.Error(), "review") {
		t.Errorf("the error %q does not say what to do: mount the review", err)
	}
	// And nothing was cloned, which is half the reason for the order.
	if after := countEntries(t, tmp); after != tmpBefore {
		t.Errorf("it cloned despite not knowing the branch: %d entries before, %d after", tmpBefore, after)
	}
}

// The NORMAL case, not the rare one: local before remote.
func TestMaterializeUsesTheLocalBranchIfItAlreadyExists(t *testing.T) {
	repo, tmp := simRepoMount(t)
	s := New(locatorForStage())
	path := filepath.Join(tmp, "clone")
	git(t, tmp, "clone", "--quiet", "--shared", repo, path)

	testutil.CommitFile(t, path, "local.txt", "local", "solo local")
	before := git(t, path, "rev-parse", "main")

	if err := s.materialize(context.Background(), path, "main"); err != nil {
		t.Fatalf("materialize of a local branch: %v", err)
	}

	after := git(t, path, "rev-parse", "main")
	if before != after {
		t.Errorf("materialize moved the local branch from %s to %s: a fetch does not bring "+
			"the local commit and the graph would stop being the review's", before, after)
	}
	if _, err := os.Stat(filepath.Join(path, "local.txt")); err != nil {
		t.Errorf("materialize threw away the local commit: %v", err)
	}
}

// The usual case for the item's branch.
func TestMaterializeCreatesTheItemBranchFromTheRemote(t *testing.T) {
	repo, tmp := simRepoMount(t)
	s := New(locatorForStage())
	path := filepath.Join(tmp, "clone")
	git(t, tmp, "clone", "--quiet", "--shared", repo, path)

	if present := git(t, path, "branch", "--list", "feat/x"); strings.TrimSpace(present) != "" {
		t.Fatalf("feat/x was already local, and this test needs it not to be: %q", present)
	}
	if err := s.materialize(context.Background(), path, "feat/x"); err != nil {
		t.Fatalf("materialize of a remote branch: %v", err)
	}
	local := git(t, path, "rev-parse", "feat/x")
	remote := git(t, path, "rev-parse", "origin/feat/x")
	if local != remote {
		t.Errorf("the created branch points at %s and the remote at %s", local, remote)
	}

	err := s.materialize(context.Background(), path, "no-existe")
	if err == nil {
		t.Fatal("a missing branch gave nil")
	}
	if !strings.Contains(err.Error(), "no-existe") {
		t.Errorf("the error %q does not name the branch", err)
	}
	if !strings.Contains(err.Error(), "does not exist") {
		t.Errorf("the error %q does not say it does not exist", err)
	}
	// The message says where to look: the local clone, which is where it was searched.
	if !strings.Contains(err.Error(), "local clone") {
		t.Errorf("the error %q does not say where it looked", err)
	}
}

// --shared is not only an optimisation.
func TestSimulationCloneIsTemporaryAndShared(t *testing.T) {
	repo, tmp := simRepoMount(t)
	s := New(locatorForStage())

	path, _, err := s.stage(context.Background(),
		Place{Repo: repo, Branch: "feat/x"}, KindMerge, "main", tmp)
	if err != nil {
		t.Fatal(err)
	}

	if filepath.Dir(path) != tmp {
		t.Errorf("the clone ended up at %q, want inside %q: the simulation clone cannot "+
			"live next to the user's repo", filepath.Dir(path), tmp)
	}
	branches := git(t, repo, "branch", "--list")
	if strings.Contains(branches, "sim") || strings.Contains(branches, "tmp") {
		t.Errorf("the simulation left branches in the source repo: %q", branches)
	}
	// The source repo still has ONE worktree, its own, because a simulation clone is not registered as
	//one.
	lines := strings.Split(strings.TrimSpace(git(t, repo, "worktree", "list")), "\n")
	if len(lines) != 1 {
		t.Errorf("the simulation left worktrees in the source repo: %d", len(lines)-1)
	}
}

func locatorForStage() Locator { return fakeLocator{} }

func countEntries(t *testing.T, dir string) int {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	return len(entries)
}
