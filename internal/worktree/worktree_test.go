package worktree

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"prdash/internal/testutil"
)

func newRepo(t *testing.T) string {
	t.Helper()
	repo := filepath.Join(t.TempDir(), "repo")
	testutil.InitRepo(t, repo)
	testutil.CommitFile(t, repo, "base.txt", "base", "base")
	return repo
}

func TestCreateListRemove(t *testing.T) {
	repo := newRepo(t)
	testutil.RunGit(t, repo, "branch", "feature")

	base := filepath.Join(t.TempDir(), "worktrees")
	dest := filepath.Join(base, "github", "github.com", "acme", "widget", "prdash-pr-1")
	g := NewGitDirect(base)
	ctx := context.Background()

	wt, err := g.Create(ctx, Spec{Repo: repo, Branch: "feature", Path: dest, Label: "prdash-pr-1"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if wt.Path != dest || wt.Branch != "feature" || wt.Label != "prdash-pr-1" {
		t.Fatalf("worktree = %+v", wt)
	}
	if !Exists(dest) {
		t.Fatal("the worktree should exist")
	}
	if _, err := os.Stat(filepath.Join(dest, "base.txt")); err != nil {
		t.Fatalf("the worktree does not bring the branch content: %v", err)
	}

	list := g.List(ctx)
	if len(list) != 1 || list[0].Path != dest || list[0].Branch != "feature" {
		t.Fatalf("List = %+v", list)
	}

	if err := g.Remove(ctx, dest); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if _, err := os.Stat(dest); !os.IsNotExist(err) {
		t.Fatalf("the worktree should have been removed: %v", err)
	}
	if got := g.List(ctx); len(got) != 0 {
		t.Fatalf("List after Remove = %+v", got)
	}
}

func TestCreateReusesExistingWorktree(t *testing.T) {
	repo := newRepo(t)
	testutil.RunGit(t, repo, "branch", "feature")

	base := t.TempDir()
	dest := filepath.Join(base, "prdash-pr-1")
	g := NewGitDirect(base)
	ctx := context.Background()

	first, err := g.Create(ctx, Spec{Repo: repo, Branch: "feature", Path: dest, Label: "prdash-pr-1"})
	if err != nil {
		t.Fatalf("Create 1: %v", err)
	}
	second, err := g.Create(ctx, Spec{Repo: repo, Branch: "feature", Path: dest, Label: "prdash-pr-1"})
	if err != nil {
		t.Fatalf("Create 2: %v", err)
	}
	if first.Path != second.Path {
		t.Fatalf("different paths: %q vs %q", first.Path, second.Path)
	}
	if got := g.List(ctx); len(got) != 1 {
		t.Fatalf("it should not duplicate the worktree: %+v", got)
	}
}

func TestCoexistingWorktreesSameRepo(t *testing.T) {
	repo := newRepo(t)
	testutil.RunGit(t, repo, "branch", "feature-1")
	testutil.RunGit(t, repo, "branch", "feature-2")

	base := t.TempDir()
	g := NewGitDirect(base)
	ctx := context.Background()

	w1, err := g.Create(ctx, Spec{Repo: repo, Branch: "feature-1", Path: filepath.Join(base, "prdash-pr-1"), Label: "prdash-pr-1"})
	if err != nil {
		t.Fatalf("Create 1: %v", err)
	}
	w2, err := g.Create(ctx, Spec{Repo: repo, Branch: "feature-2", Path: filepath.Join(base, "prdash-pr-2"), Label: "prdash-pr-2"})
	if err != nil {
		t.Fatalf("Create 2: %v", err)
	}
	if w1.Path == w2.Path {
		t.Fatal("two items of the same repo must have different worktrees")
	}
	if !Exists(w1.Path) || !Exists(w2.Path) {
		t.Fatal("both worktrees must coexist")
	}
	if got := g.List(ctx); len(got) != 2 {
		t.Fatalf("List = %+v, want 2", got)
	}
}

func TestCreateBranchMismatchErrors(t *testing.T) {
	repo := newRepo(t)
	testutil.RunGit(t, repo, "branch", "feature-1")
	testutil.RunGit(t, repo, "branch", "feature-2")

	base := t.TempDir()
	dest := filepath.Join(base, "prdash-pr-1")
	g := NewGitDirect(base)
	ctx := context.Background()

	if _, err := g.Create(ctx, Spec{Repo: repo, Branch: "feature-1", Path: dest, Label: "prdash-pr-1"}); err != nil {
		t.Fatalf("Create: %v", err)
	}
	_, err := g.Create(ctx, Spec{Repo: repo, Branch: "feature-2", Path: dest, Label: "prdash-pr-1"})
	if err == nil || !strings.Contains(err.Error(), "feature-1") {
		t.Fatalf("expected an error for an already-present branch, got %v", err)
	}
}

func TestCreateIncompleteSpecErrors(t *testing.T) {
	g := NewGitDirect(t.TempDir())
	if _, err := g.Create(context.Background(), Spec{Repo: "x"}); err == nil {
		t.Fatal("an incomplete spec should fail")
	}
}

func TestCreateFailureLeavesNoPartialDir(t *testing.T) {
	repo := newRepo(t)
	base := t.TempDir()
	dest := filepath.Join(base, "prdash-pr-1")
	g := NewGitDirect(base)

	if _, err := g.Create(context.Background(), Spec{Repo: repo, Branch: "no-existe", Path: dest, Label: "prdash-pr-1"}); err == nil {
		t.Fatal("expected an error checking out a nonexistent branch")
	}
	if _, err := os.Stat(dest); !os.IsNotExist(err) {
		t.Fatalf("no half-done worktree should remain: %v", err)
	}
}

func newCleanWorktree(t *testing.T) (base, dest, repo string) {
	t.Helper()
	repo = newRepo(t)
	testutil.RunGit(t, repo, "branch", "feature")
	base = t.TempDir()
	dest = filepath.Join(base, "prdash-pr-1")
	if _, err := NewGitDirect(base).Create(context.Background(), Spec{Repo: repo, Branch: "feature", Path: dest, Label: "prdash-pr-1"}); err != nil {
		t.Fatalf("preparing a clean worktree: %v", err)
	}
	return base, dest, repo
}

func TestRemoveIfCleanRemovesCleanWorktree(t *testing.T) {
	base, dest, _ := newCleanWorktree(t)
	removed, reason, err := NewGitDirect(base).RemoveIfClean(context.Background(), dest)
	if err != nil {
		t.Fatalf("RemoveIfClean: %v", err)
	}
	if !removed || reason != "" {
		t.Fatalf("removed=%v reason=%q, want deleted with no reason", removed, reason)
	}
	if _, err := os.Stat(dest); !os.IsNotExist(err) {
		t.Fatalf("the clean worktree should have been deleted: %v", err)
	}
}

func TestRemoveIfCleanKeepsDirtyWorktree(t *testing.T) {
	base, dest, _ := newCleanWorktree(t)
	if err := os.WriteFile(filepath.Join(dest, "base.txt"), []byte("edited"), 0o644); err != nil {
		t.Fatal(err)
	}

	removed, reason, err := NewGitDirect(base).RemoveIfClean(context.Background(), dest)
	if err != nil {
		t.Fatalf("RemoveIfClean: %v", err)
	}
	if removed || reason != KeptUncommitted {
		t.Fatalf("removed=%v reason=%q, want kept for being dirty", removed, reason)
	}
	if _, err := os.Stat(dest); err != nil {
		t.Fatalf("the dirty worktree should not be touched: %v", err)
	}
}

// An untracked file counts as dirty too: `git diff --quiet` ignores it and `status
// --porcelain` does not.
func TestRemoveIfCleanKeepsUntrackedWorktree(t *testing.T) {
	base, dest, _ := newCleanWorktree(t)
	if err := os.WriteFile(filepath.Join(dest, "new.txt"), []byte("untracked"), 0o644); err != nil {
		t.Fatal(err)
	}

	removed, reason, err := NewGitDirect(base).RemoveIfClean(context.Background(), dest)
	if err != nil {
		t.Fatalf("RemoveIfClean: %v", err)
	}
	if removed || reason != KeptUncommitted {
		t.Fatalf("removed=%v reason=%q, want kept for untracked", removed, reason)
	}
	if _, err := os.Stat(dest); err != nil {
		t.Fatalf("the worktree with untracked files should not be touched: %v", err)
	}
}

// The fail-safe: if git's status cannot be read, the worktree is kept instead of deleted.
func TestRemoveIfCleanKeepsOnUnreadableStatus(t *testing.T) {
	base, dest, _ := newCleanWorktree(t)
	if err := os.WriteFile(filepath.Join(dest, ".git"), []byte("gitdir: /nonexistent/prdash-gitdir\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	removed, reason, err := NewGitDirect(base).RemoveIfClean(context.Background(), dest)
	if err != nil {
		t.Fatalf("RemoveIfClean: %v", err)
	}
	if removed || reason != KeptUnreadable {
		t.Fatalf("removed=%v reason=%q, want kept for unreadable status", removed, reason)
	}
	if _, err := os.Stat(dest); err != nil {
		t.Fatalf("the worktree with unreadable status should not be touched: %v", err)
	}
}

// Already gone is a no-op without an error, which is what makes it idempotent.
func TestRemoveIfCleanAbsentPathIsNoop(t *testing.T) {
	base := t.TempDir()
	dest := filepath.Join(base, "prdash-pr-1")
	removed, reason, err := NewGitDirect(base).RemoveIfClean(context.Background(), dest)
	if err != nil || removed || reason != "" {
		t.Fatalf("removed=%v reason=%q err=%v, want no-op without error", removed, reason, err)
	}
}

func TestRemoveIfCleanRefusesForeign(t *testing.T) {
	repo := newRepo(t)
	testutil.RunGit(t, repo, "branch", "feature")
	base := t.TempDir()
	foreign := filepath.Join(base, "other-tool")
	testutil.RunGit(t, repo, "worktree", "add", "--quiet", foreign, "feature")

	removed, _, err := NewGitDirect(base).RemoveIfClean(context.Background(), foreign)
	if err == nil {
		t.Fatal("it should not accept deleting a foreign worktree")
	}
	if removed {
		t.Fatal("removed should be false")
	}
	if _, err := os.Stat(foreign); err != nil {
		t.Fatalf("the foreign worktree should not be touched: %v", err)
	}
}

func TestRemoveIfCleanRefusesPathOutsideBase(t *testing.T) {
	repo := newRepo(t)
	testutil.RunGit(t, repo, "branch", "feature")
	outside := filepath.Join(t.TempDir(), "prdash-pr-1")
	testutil.RunGit(t, repo, "worktree", "add", "--quiet", outside, "feature")

	removed, _, err := NewGitDirect(t.TempDir()).RemoveIfClean(context.Background(), outside)
	if err == nil {
		t.Fatal("it should not accept deleting outside the managed root")
	}
	if removed {
		t.Fatal("removed should be false")
	}
	if _, err := os.Stat(outside); err != nil {
		t.Fatalf("the worktree outside the root should not be touched: %v", err)
	}
}
