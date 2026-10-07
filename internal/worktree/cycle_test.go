package worktree

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"prdash/internal/testutil"
)

func mountWorktreeWithBranch(t *testing.T, root, label, branch string) Worktree {
	t.Helper()
	repo := filepath.Join(t.TempDir(), "repo")
	testutil.InitRepo(t, repo)
	testutil.CommitFile(t, repo, "base.txt", "base", "base")

	// The branch must not be main: git does not allow two worktrees on the same branch.
	if branch == "main" {
		branch = "pr-" + label
	}
	testutil.RunGit(t, repo, "branch", branch)
	spec := Spec{
		Repo:   repo,
		Branch: branch,
		Path:   filepath.Join(root, label),
		Label:  label,
	}
	wt, err := NewGitDirect(root).Create(context.Background(), spec)
	if err != nil {
		t.Fatalf("creating the worktree %s: %v", label, err)
	}
	return wt
}

// The asymmetry is the point: the SAME branch reuses, a DIFFERENT one does not.
func TestCreatingOverAWorktreeAlreadyOnTheSameBranchReusesIt(t *testing.T) {
	root := t.TempDir()
	first := mountWorktreeWithBranch(t, root, "prdash-pr-1", "feat/x")

	g := NewGitDirect(root)
	second, err := g.Create(context.Background(), Spec{
		Repo: first.Repo, Branch: "feat/x",
		Path: first.Path, Label: "prdash-pr-1",
	})
	if err != nil {
		t.Fatalf("creating again over the same branch gave an error: %v", err)
	}
	if second.Path != first.Path || second.Branch != "feat/x" {
		t.Errorf("the reused worktree is not the same one: %+v", second)
	}
	if n := len(strings.Split(strings.TrimSpace(
		testutil.RunGit(t, first.Repo, "worktree", "list")), "\n")); n != 2 {
		t.Errorf("there are %d lines in worktree list, want 2", n)
	}

	testutil.RunGit(t, first.Repo, "branch", "other")
	_, err = g.Create(context.Background(), Spec{
		Repo: first.Repo, Branch: "other",
		Path: first.Path, Label: "prdash-pr-1",
	})
	if err == nil {
		t.Fatal("creating with another branch over the same worktree gave nil")
	}
	for _, want := range []string{"feat/x", "other"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("the error %q does not mention %q", err, want)
		}
	}
	if got := testutil.RunGit(t, first.Path, "rev-parse", "--abbrev-ref", "HEAD"); got != "feat/x" {
		t.Errorf("after the rejection the branch ended up at %q, want feat/x", got)
	}
}

func TestTheWorktreeLabelComesFromTheDirectoryNameWhenNotGiven(t *testing.T) {
	root := t.TempDir()
	repo := filepath.Join(t.TempDir(), "repo")
	testutil.InitRepo(t, repo)
	testutil.CommitFile(t, repo, "a.txt", "a", "a")

	g := NewGitDirect(root)

	testutil.RunGit(t, repo, "branch", "pr-7")
	wt, err := g.Create(context.Background(), Spec{
		Repo: repo, Branch: "pr-7", Path: filepath.Join(root, "prdash-pr-7"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if wt.Label != "prdash-pr-7" {
		t.Errorf("with no label it gave %q, want the directory name", wt.Label)
	}
	if !Owned(wt.Label, wt.Path) {
		t.Errorf("the default label %q is not recognised by Owned", wt.Label)
	}

	testutil.RunGit(t, repo, "branch", "pr-9")
	withLabel, err := g.Create(context.Background(), Spec{
		Repo: repo, Branch: "pr-9",
		Path: filepath.Join(root, "without-prefix"), Label: "prdash-pr-9",
	})
	if err != nil {
		t.Fatal(err)
	}
	if withLabel.Label != "prdash-pr-9" {
		t.Errorf("with a label it gave %q", withLabel.Label)
	}
	if !Owned(withLabel.Label, withLabel.Path) {
		t.Errorf("with label %q in a directory without the prefix, Owned does not recognise it",
			withLabel.Label)
	}
	if withLabel.ID != withLabel.Path || withLabel.Path == "" {
		t.Errorf("ID/Path are not the path: %+v", withLabel)
	}
	if withLabel.Repo != repo {
		t.Errorf("Repo = %q, want %q", withLabel.Repo, repo)
	}
}

func TestRemoveIfCleanDoesNotDeleteADirtyWorktreeAndExplainsWhy(t *testing.T) {
	root := t.TempDir()
	wt := mountWorktreeWithBranch(t, root, "prdash-pr-1", "main")
	g := NewGitDirect(root)
	ctx := context.Background()

	removed, reason, err := g.RemoveIfClean(ctx, wt.Path)
	if err != nil {
		t.Fatalf("RemoveIfClean of a clean worktree: %v", err)
	}
	if !removed {
		t.Errorf("a clean worktree was not deleted: %q", reason)
	}
	if reason != "" {
		t.Errorf("after deleting, reason %q was left, and there is no reason for anything", reason)
	}
	record := testutil.RunGit(t, wt.Repo, "worktree", "list")
	if strings.Contains(record, "prdash-pr-1") {
		t.Errorf("after Remove the worktree is still in git's record: %q", record)
	}

	dirty := mountWorktreeWithBranch(t, root, "prdash-pr-2", "main")
	if err := os.WriteFile(filepath.Join(dirty.Path, "cambiado.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	removed, reason, err = g.RemoveIfClean(ctx, dirty.Path)
	if err != nil {
		t.Fatalf("RemoveIfClean of a dirty worktree: %v", err)
	}
	if removed {
		t.Error("a worktree with uncommitted changes was deleted: the edits were lost")
	}
	if !exists(t, dirty.Path) {
		t.Error("the dirty worktree disappeared anyway")
	}
	if strings.TrimSpace(reason) == "" {
		t.Fatal("it was not deleted and there is no reason: the user has no way to know why")
	}
	if !strings.Contains(strings.ToLower(reason), "uncommitted") &&
		!strings.Contains(strings.ToLower(reason), "changes") {
		t.Logf("the reason does not mention the changes: %q", reason)
	}
}

// Why `diff` is not enough: an untracked file is uncommitted work and diff ignores it.
func TestDirtyCountsUntrackedFiles(t *testing.T) {
	root := t.TempDir()
	wt := mountWorktreeWithBranch(t, root, "prdash-pr-1", "main")
	g := NewGitDirect(root)
	ctx := context.Background()

	dirty, err := g.dirty(ctx, wt.Path)
	if err != nil {
		t.Fatal(err)
	}
	if dirty {
		t.Error("a freshly created worktree came back dirty")
	}

	fresh := filepath.Join(wt.Path, "new.txt")
	if err := os.WriteFile(fresh, []byte("uncommitted work"), 0o644); err != nil {
		t.Fatal(err)
	}
	dirty, err = g.dirty(ctx, wt.Path)
	if err != nil {
		t.Fatal(err)
	}
	if !dirty {
		t.Error("an untracked file does not count as work: diff ignores it and the " +
			"deletion would throw it away")
	}

	if err := os.WriteFile(fresh, []byte("changed again"), 0o644); err != nil {
		t.Fatal(err)
	}
	testutil.RunGit(t, wt.Path, "add", "new.txt")
	dirty, err = g.dirty(ctx, wt.Path)
	if err != nil {
		t.Fatal(err)
	}
	if !dirty {
		t.Error("uncommitted changes in the index do not count as work")
	}

	testutil.RunGit(t, wt.Path, "commit", "-m", "whatever")
	dirty, err = g.dirty(ctx, wt.Path)
	if err != nil {
		t.Fatal(err)
	}
	if dirty {
		t.Error("after committing it still comes back dirty")
	}
}

// Three refusals, each preventing a different damage.
func TestRemoveRefusesToTouchWhatItDoesNotOwn(t *testing.T) {
	root := t.TempDir()
	wt := mountWorktreeWithBranch(t, root, "prdash-pr-1", "main")
	g := NewGitDirect(root)
	ctx := context.Background()

	foreign := filepath.Join(t.TempDir(), "prdash-mio")
	testutil.InitRepo(t, foreign)
	testutil.CommitFile(t, foreign, "importante.txt", "do not delete me", "important")

	// shouldRemain differs per case: a path that does not exist cannot "stay there".
	dir := filepath.Join(root, "prdash-carpeta-vacia")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		name         string
		path         string
		shouldRemain bool
	}{
		{"outside the root", foreign, true},
		{"nonexistent", filepath.Join(root, "prdash-no-existe"), false},
		{"not a worktree", dir, true},
	} {
		err := g.Remove(ctx, c.path)
		if err == nil {
			t.Errorf("%s: Remove gave nil, and it has to refuse", c.name)
			continue
		}
		if c.shouldRemain && !exists(t, c.path) {
			t.Errorf("%s: Remove deleted %s, which is not its own", c.name, c.path)
		}
	}

	if _, err := os.Stat(filepath.Join(foreign, "importante.txt")); err != nil {
		t.Errorf("the foreign repo lost its content: %v", err)
	}

	if err := g.Remove(ctx, wt.Path); err != nil {
		t.Fatalf("Remove of our own worktree: %v", err)
	}
	if strings.Contains(testutil.RunGit(t, wt.Repo, "worktree", "list"), "prdash-pr-1") {
		t.Error("our own worktree is still in the record after Remove")
	}
}

// inspect exists to avoid creating the worktree just to read it.
func TestInspectBringsTheWorktreeStateWithoutMountingAnything(t *testing.T) {
	root := t.TempDir()
	wt := mountWorktreeWithBranch(t, root, "prdash-pr-1", "main")
	g := NewGitDirect(root)

	seen, ok, err := g.inspect(context.Background(), wt.Path)
	if err != nil {
		t.Fatalf("inspect: %v", err)
	}
	if !ok {
		t.Fatal("inspect did not see a worktree that exists")
	}
	if seen.Branch == "" {
		t.Error("inspect did not read the worktree's branch")
	}
	if seen.Path != wt.Path {
		t.Errorf("Path = %q, want %q", seen.Path, wt.Path)
	}

	dir := filepath.Join(root, "prdash-carpeta")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, ok, err := g.inspect(context.Background(), dir); ok || err != nil {
		t.Errorf("a loose directory gave (ok=%v, err=%v), want (false, nil)", ok, err)
	}
	lines := strings.Split(strings.TrimSpace(
		testutil.RunGit(t, wt.Repo, "worktree", "list")), "\n")
	if len(lines) != 2 {
		t.Errorf("inspect left %d lines in worktree list, want 2", len(lines))
	}
}

func exists(t *testing.T, path string) bool {
	t.Helper()
	_, err := os.Stat(path)
	return err == nil
}
