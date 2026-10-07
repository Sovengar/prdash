package worktree

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"prdash/internal/testutil"
)

func TestOwnedRecognizesOnlyThePrdashMark(t *testing.T) {
	cases := []struct {
		label string
		path  string
		want  bool
	}{
		{"prdash-pr-3", "/x/y/prdash-pr-3", true},
		{"", "/x/y/prdash-pr-3", true},
		{"prdash-pr-3", "/x/y/other-cosa", true},
		{"other-tool", "/x/y/other-tool", false},
		{"", "/x/y/other-tool", false},
		{"repodash", "/x/y/repodash", false},
	}
	for _, c := range cases {
		if got := Owned(c.label, c.path); got != c.want {
			t.Errorf("Owned(%q, %q) = %v, want %v", c.label, c.path, got, c.want)
		}
	}
}

func TestAuditListsOnlyOwnedWorktrees(t *testing.T) {
	repo := newRepo(t)
	testutil.RunGit(t, repo, "branch", "propia")
	testutil.RunGit(t, repo, "branch", "ajena")

	base := t.TempDir()
	owned := filepath.Join(base, "prdash-pr-1")
	foreign := filepath.Join(base, "other-tool")
	if _, err := NewGitDirect(base).Create(context.Background(), Spec{Repo: repo, Branch: "propia", Path: owned, Label: "prdash-pr-1"}); err != nil {
		t.Fatalf("preparing our own worktree: %v", err)
	}
	testutil.RunGit(t, repo, "worktree", "add", "--quiet", foreign, "ajena")

	entries := NewGitDirect(base).Audit(context.Background())
	if len(entries) != 1 {
		t.Fatalf("Audit = %+v, want only our own worktree", entries)
	}
	if entries[0].Path != owned || entries[0].Orphan {
		t.Fatalf("entry = %+v", entries[0])
	}
	if entries[0].Branch != "propia" {
		t.Fatalf("branch = %q", entries[0].Branch)
	}
}

func TestAuditSortsByPath(t *testing.T) {
	repo := newRepo(t)
	base := t.TempDir()

	var want []string
	for _, n := range []string{"prdash-pr-10", "prdash-pr-2", "prdash-pr-1"} {
		testutil.RunGit(t, repo, "branch", n)
		path := filepath.Join(base, n)
		if _, err := NewGitDirect(base).Create(context.Background(), Spec{Repo: repo, Branch: n, Path: path, Label: n}); err != nil {
			t.Fatalf("creating %s: %v", n, err)
		}
		want = append(want, path)
	}
	// The expected order is LEXICOGRAPHIC on the paths, not creation order.
	slices.Sort(want)

	entries := NewGitDirect(base).Audit(context.Background())
	if len(entries) != len(want) {
		t.Fatalf("Audit = %d entries, want %d", len(entries), len(want))
	}
	for i, e := range entries {
		if e.Path != want[i] {
			var got []string
			for _, x := range entries {
				got = append(got, filepath.Base(x.Path))
			}
			t.Errorf("Audit is not sorted by path: %v", got)
			break
		}
	}
}

func TestAuditFlagsOrphanWhenSourceGone(t *testing.T) {
	repo := newRepo(t)
	testutil.RunGit(t, repo, "branch", "feature")

	base := t.TempDir()
	dest := filepath.Join(base, "prdash-pr-1")
	if _, err := NewGitDirect(base).Create(context.Background(), Spec{Repo: repo, Branch: "feature", Path: dest, Label: "prdash-pr-1"}); err != nil {
		t.Fatalf("preparing a worktree: %v", err)
	}
	if err := os.RemoveAll(repo); err != nil {
		t.Fatal(err)
	}

	entries := NewGitDirect(base).Audit(context.Background())
	if len(entries) != 1 || !entries[0].Orphan || entries[0].Reason == "" {
		t.Fatalf("expected an orphan with a reason, got %+v", entries)
	}
}

func TestListExcludesForeignWorktrees(t *testing.T) {
	repo := newRepo(t)
	testutil.RunGit(t, repo, "branch", "propia")
	testutil.RunGit(t, repo, "branch", "ajena")

	base := t.TempDir()
	owned := filepath.Join(base, "prdash-pr-1")
	foreign := filepath.Join(base, "other-tool")
	if _, err := NewGitDirect(base).Create(context.Background(), Spec{Repo: repo, Branch: "propia", Path: owned, Label: "prdash-pr-1"}); err != nil {
		t.Fatalf("preparing our own worktree: %v", err)
	}
	testutil.RunGit(t, repo, "worktree", "add", "--quiet", foreign, "ajena")

	list := NewGitDirect(base).List(context.Background())
	if len(list) != 1 || list[0].Path != owned {
		t.Fatalf("List = %+v, want only our own worktree", list)
	}
}

func TestRemoveOrphanDeletesCheckout(t *testing.T) {
	repo := newRepo(t)
	testutil.RunGit(t, repo, "branch", "feature")

	base := t.TempDir()
	dest := filepath.Join(base, "prdash-pr-1")
	g := NewGitDirect(base)
	if _, err := g.Create(context.Background(), Spec{Repo: repo, Branch: "feature", Path: dest, Label: "prdash-pr-1"}); err != nil {
		t.Fatalf("preparing a worktree: %v", err)
	}
	if err := os.RemoveAll(repo); err != nil {
		t.Fatal(err)
	}

	if err := g.Remove(context.Background(), dest); err != nil {
		t.Fatalf("Remove of an orphan: %v", err)
	}
	if _, err := os.Stat(dest); !os.IsNotExist(err) {
		t.Fatalf("the orphaned checkout should have been deleted: %v", err)
	}
}

func TestRemoveRefusesForeignWorktree(t *testing.T) {
	repo := newRepo(t)
	testutil.RunGit(t, repo, "branch", "feature")

	base := t.TempDir()
	foreign := filepath.Join(base, "other-tool")
	testutil.RunGit(t, repo, "worktree", "add", "--quiet", foreign, "feature")

	g := NewGitDirect(base)
	if err := g.Remove(context.Background(), foreign); err == nil {
		t.Fatal("it should not delete a foreign worktree")
	}
	if _, err := os.Stat(foreign); err != nil {
		t.Fatalf("the foreign worktree should not be touched: %v", err)
	}
}

func TestRemoveRefusesPathOutsideBase(t *testing.T) {
	repo := newRepo(t)
	testutil.RunGit(t, repo, "branch", "feature")

	outside := filepath.Join(t.TempDir(), "prdash-pr-1")
	testutil.RunGit(t, repo, "worktree", "add", "--quiet", outside, "feature")

	g := NewGitDirect(t.TempDir())
	if err := g.Remove(context.Background(), outside); err == nil {
		t.Fatal("it should not delete outside the managed root")
	}
	if _, err := os.Stat(outside); err != nil {
		t.Fatalf("the worktree outside the root should not be touched: %v", err)
	}
}

func TestAuditSkipsNonWorktreeDirs(t *testing.T) {
	base := t.TempDir()
	if err := os.MkdirAll(filepath.Join(base, "prdash-pr-9"), 0o755); err != nil {
		t.Fatal(err)
	}
	if got := NewGitDirect(base).Audit(context.Background()); len(got) != 0 {
		t.Fatalf("a directory without a worktree should not be listed: %+v", got)
	}
}

func TestRemoveOrphanWithoutGitDirDeletesCheckout(t *testing.T) {
	repo := newRepo(t)
	testutil.RunGit(t, repo, "branch", "feature")

	base := t.TempDir()
	dest := filepath.Join(base, "prdash-pr-1")
	g := NewGitDirect(base)
	if _, err := g.Create(context.Background(), Spec{Repo: repo, Branch: "feature", Path: dest, Label: "prdash-pr-1"}); err != nil {
		t.Fatalf("preparing a worktree: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dest, ".git"), []byte("garbage\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := g.Remove(context.Background(), dest); err != nil {
		t.Fatalf("Remove of an orphan without gitdir: %v", err)
	}
	if _, err := os.Stat(dest); !os.IsNotExist(err) {
		t.Fatalf("the checkout should have been deleted: %v", err)
	}
}

func TestRemoveRefusesNonLinkedDir(t *testing.T) {
	base := t.TempDir()
	dir := filepath.Join(base, "prdash-not-a-worktree")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}

	if err := NewGitDirect(base).Remove(context.Background(), dir); err == nil {
		t.Fatal("a directory that is not a linked worktree should not be deleted")
	}
	if _, err := os.Stat(dir); err != nil {
		t.Fatalf("the directory should not be touched: %v", err)
	}
}
