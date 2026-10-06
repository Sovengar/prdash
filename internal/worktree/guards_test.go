package worktree

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"prdash/internal/testutil"
)

func repoWithBranch(t *testing.T, extra ...string) string {
	t.Helper()
	repo := filepath.Join(t.TempDir(), "repo")
	testutil.InitRepo(t, repo)
	testutil.CommitFile(t, repo, "base.txt", "base", "base")
	for _, r := range extra {
		testutil.RunGit(t, repo, "branch", r)
	}
	return repo
}

// Create's first guard.
func TestAnIncompleteSpecIsNotProcessedAndNothingIsTouched(t *testing.T) {
	repo := repoWithBranch(t)
	root := t.TempDir()
	g := NewGitDirect(root)

	for _, c := range []struct {
		name string
		spec Spec
	}{
		{"without repo", Spec{Branch: "main", Path: filepath.Join(root, "wt")}},
		{"without branch", Spec{Repo: repo, Path: filepath.Join(root, "wt")}},
		{"without destination", Spec{Repo: repo, Branch: "main"}},
		{"all empty", Spec{}},
	} {
		_, err := g.Create(context.Background(), c.spec)
		if err == nil {
			t.Errorf("%s: Create gave nil", c.name)
			continue
		}
		// The message says WHAT is missing, not "incomplete spec": the reader has to know whether
		// mounting another item's review fixes it.
		if !strings.Contains(err.Error(), "incomplete spec") {
			t.Errorf("%s: the error %q does not say the spec is incomplete", c.name, err)
		}
	}

	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Errorf("an incomplete spec left %d entries at the root: %v", len(entries), entries)
	}
}

// cleanPartial's cleanup.
func TestABranchThatDoesNotExistFailsAndLeavesNoResidueDirectory(t *testing.T) {
	repo := repoWithBranch(t)
	root := t.TempDir()
	g := NewGitDirect(root)
	dest := filepath.Join(root, "prdash-pr-7")

	_, err := g.Create(context.Background(), Spec{
		Repo: repo, Branch: "feat/no-existe", Path: dest, Label: "prdash-pr-7",
	})
	if err == nil {
		t.Fatal("creating a worktree of a branch that does not exist gave nil")
	}
	if !strings.Contains(err.Error(), "worktree") {
		t.Errorf("the error %q does not say the worktree failed", err)
	}

	if _, err := os.Stat(dest); !os.IsNotExist(err) {
		t.Errorf("directory %s was left after a failed add: the next attempt fails with "+
			"'already exists' and the message does not point at the first failure", dest)
	}
	lines := strings.Split(strings.TrimSpace(
		testutil.RunGit(t, repo, "worktree", "list")), "\n")
	for _, l := range lines {
		if strings.Contains(l, "prdash-pr-7") {
			t.Errorf("the failed worktree stayed in git's record: %q", l)
		}
	}
	testutil.RunGit(t, repo, "branch", "other")
	wt, err := g.Create(context.Background(), Spec{
		Repo: repo, Branch: "other", Path: filepath.Join(root, "prdash-pr-8"),
		Label: "prdash-pr-8",
	})
	if err != nil {
		t.Fatalf("the root is no good after a failure: %v", err)
	}
	if !Exists(wt.Path) {
		t.Error("the second worktree does not exist after a failure of the first")
	}
}

func TestADestinationThatIsAlreadyARepoIsNeitherOverwrittenNorReadAsAWorktree(t *testing.T) {
	root := t.TempDir()
	repo := filepath.Join(root, "prdash-pr-7")
	testutil.InitRepo(t, repo)
	testutil.CommitFile(t, repo, "trabajo.txt", "important", "important")

	g := NewGitDirect(root)
	ctx := context.Background()

	seen, ok, err := g.inspect(ctx, repo)
	if err == nil {
		t.Error("inspect of a normal repo gave nil: someone could take it for a worktree")
	}
	if ok {
		t.Error("inspect said a normal repo is a linked worktree")
	}
	if seen.Path != "" {
		t.Errorf("inspect returned a worktree: %+v", seen)
	}
	if !strings.Contains(err.Error(), "not a linked worktree") {
		t.Errorf("the error %q does not say it is not a linked worktree", err)
	}

	if _, err := g.Create(ctx, Spec{
		Repo: filepath.Join(t.TempDir(), "other"), Branch: "main",
		Path: repo, Label: "prdash-pr-7",
	}); err == nil {
		t.Error("Create over an existing repo gave nil")
	}
	if _, err := os.Stat(filepath.Join(repo, "trabajo.txt")); err != nil {
		t.Errorf("the user's repo lost its content: %v", err)
	}

	if err := g.Remove(ctx, repo); err == nil {
		t.Error("Remove of a normal repo gave nil")
	}
	if _, err := os.Stat(filepath.Join(repo, "trabajo.txt")); err != nil {
		t.Errorf("Remove took the user's repo with it: %v", err)
	}
}

// Skipping .git directories is not an optimisation.
func TestAuditDoesNotEnterGitDirectories(t *testing.T) {
	root := t.TempDir()
	repo := repoWithBranch(t, "feat/x")
	g := NewGitDirect(root)

	if _, err := g.Create(context.Background(), Spec{
		Repo: repo, Branch: "feat/x", Path: filepath.Join(root, "prdash-pr-7"),
		Label: "prdash-pr-7",
	}); err != nil {
		t.Fatal(err)
	}

	entries := g.Audit(context.Background())
	if len(entries) != 1 {
		t.Fatalf("Audit returned %d entries, want 1: %+v", len(entries), entries)
	}
	if entries[0].Path != filepath.Join(root, "prdash-pr-7") {
		t.Errorf("Audit returned %q", entries[0].Path)
	}

	for _, e := range entries {
		if strings.Contains(e.Path, string(filepath.Separator)+".git"+string(filepath.Separator)) {
			t.Errorf("Audit listed something from inside a .git: %q", e.Path)
		}
		if !strings.HasPrefix(filepath.Base(e.Path), LabelPrefix) {
			t.Errorf("Audit listed %q, which does not carry the ownership prefix", e.Path)
		}
	}
}

// The lock's last turn.
func TestRemoveIfCleanPropagatesTheFailureToRemoveIt(t *testing.T) {
	root := t.TempDir()
	repo := repoWithBranch(t, "feat/x")
	g := NewGitDirect(root)
	wt, err := g.Create(context.Background(), Spec{
		Repo: repo, Branch: "feat/x", Path: filepath.Join(root, "prdash-pr-7"),
		Label: "prdash-pr-7",
	})
	if err != nil {
		t.Fatal(err)
	}

	removed, reason, err := g.RemoveIfClean(context.Background(), wt.Path)
	if err != nil || !removed || reason != "" {
		t.Fatalf("RemoveIfClean of a clean worktree: (%v, %q, %v)", removed, reason, err)
	}

	if _, _, err := g.RemoveIfClean(context.Background(), filepath.Join(t.TempDir(), "ajeno")); err == nil {
		t.Error("RemoveIfClean of a foreign path gave nil")
	}
}
