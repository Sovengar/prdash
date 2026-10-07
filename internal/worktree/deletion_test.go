package worktree

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"prdash/internal/testutil"
)

var gitSeparator = string(filepath.Separator) + ".git" + string(filepath.Separator)

// It leaves the parent unwritable, so removing or creating fails.
func makeParentReadOnly(t *testing.T, parent string) {
	t.Helper()
	if err := os.Chmod(parent, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		// Restored, or t.TempDir() cannot clean up and reports an unrelated error.
		_ = os.Chmod(parent, 0o755)
	})
}

func TestCreateFailsWhenTheDestinationParentCannotBeCreatedAndLeavesNothing(t *testing.T) {
	repo := repoWithBranch(t, "feat/x")
	root := t.TempDir()
	g := NewGitDirect(root)

	blocker := filepath.Join(root, "prdash-bloqueado")
	if err := os.WriteFile(blocker, []byte("I am a file"), 0o644); err != nil {
		t.Fatal(err)
	}

	dest := filepath.Join(blocker, "prdash-pr-7")
	_, err := g.Create(context.Background(), Spec{
		Repo: repo, Branch: "feat/x", Path: dest, Label: "prdash-pr-7",
	})
	if err == nil {
		t.Fatal("creating a worktree under a parent that is a file gave nil")
	}
	if !strings.Contains(err.Error(), "prepare the worktree destination") {
		t.Errorf("the error %q does not say preparing the destination failed", err)
	}
	raw, err := os.ReadFile(blocker)
	if err != nil || string(raw) != "I am a file" {
		t.Errorf("the blocker changed: %q %v", raw, err)
	}
}

func TestRemovePropagatesTheFailureToRemoveTheCheckout(t *testing.T) {
	for _, c := range []struct {
		name  string
		setup func(t *testing.T, root string) string
		want  string
	}{
		{
			name: "orphan with no repo behind it",
			setup: func(t *testing.T, root string) string {
				d := filepath.Join(root, "prdash-pr-7")
				if err := os.MkdirAll(d, 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(d, ".git"), []byte("gitdir: \n"), 0o644); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(d, "trabajo.txt"), []byte("x"), 0o644); err != nil {
					t.Fatal(err)
				}
				return d
			},
			want: "remove the worktree checkout",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			root := filepath.Join(t.TempDir(), "raiz")
			if err := os.MkdirAll(root, 0o755); err != nil {
				t.Fatal(err)
			}
			id := c.setup(t, root)
			makeParentReadOnly(t, root)

			err := NewGitDirect(root).Remove(context.Background(), id)
			if err == nil {
				t.Fatal("Remove with the parent read-only gave nil: the checkout is still there " +
					"and nobody noticed")
			}
			if !strings.Contains(err.Error(), c.want) {
				t.Errorf("the error %q does not say %q", err, c.want)
			}
			if !strings.Contains(err.Error(), id) {
				t.Errorf("the error %q does not name the path that could not be removed", err)
			}
		})
	}
}

func TestRemoveIfCleanPropagatesTheRemovalFailureWhenItIsClean(t *testing.T) {
	root := filepath.Join(t.TempDir(), "raiz")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	d := filepath.Join(root, "prdash-pr-7")
	if err := os.MkdirAll(d, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(d, ".git"), []byte("gitdir: \n"), 0o644); err != nil {
		t.Fatal(err)
	}
	g := NewGitDirect(root)

	removed, reason, err := g.RemoveIfClean(context.Background(), d)
	if err != nil {
		t.Fatalf("RemoveIfClean of an orphan: %v", err)
	}
	if removed {
		t.Error("an orphan with no repo was deleted: it cannot be checked for being clean")
	}
	if !strings.Contains(reason, "status") {
		t.Errorf("reason = %q, and it should say the status could not be read", reason)
	}

	repo := repoWithBranch(t, "feat/x")
	root2 := filepath.Join(t.TempDir(), "raiz")
	if err := os.MkdirAll(root2, 0o755); err != nil {
		t.Fatal(err)
	}
	wt, err := NewGitDirect(root2).Create(context.Background(), Spec{
		Repo: repo, Branch: "feat/x", Path: filepath.Join(root2, "prdash-pr-7"),
		Label: "prdash-pr-7",
	})
	if err != nil {
		t.Fatal(err)
	}
	makeParentReadOnly(t, root2)
	if err := os.Chmod(wt.Path, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(wt.Path, 0o755) })

	removed, reason, err = gitAt(root2).RemoveIfClean(context.Background(), wt.Path)
	if err == nil {
		t.Fatal("the removal failure was not propagated: the worktree is still there without anyone knowing")
	}
	if removed {
		t.Error("it said it removed it and it did not")
	}
	if reason != "" {
		t.Errorf("reason = %q: a removal failure is not a reason for not removing", reason)
	}
	if !Exists(wt.Path) {
		t.Error("the worktree was removed despite the error")
	}
}

func gitAt(root string) *GitDirect { return NewGitDirect(root) }

func TestAuditSkipsNestedGitDirectories(t *testing.T) {
	root := t.TempDir()
	repo := filepath.Join(root, "no-es-prdash")
	testutil.InitRepo(t, repo)
	testutil.CommitFile(t, repo, "f.txt", "x", "x")

	source := repoWithBranch(t, "feat/x")
	g := NewGitDirect(root)
	if _, err := g.Create(context.Background(), Spec{
		Repo: source, Branch: "feat/x", Path: filepath.Join(root, "prdash-pr-7"),
		Label: "prdash-pr-7",
	}); err != nil {
		t.Fatal(err)
	}

	entries := g.Audit(context.Background())
	if len(entries) != 1 || entries[0].Path != filepath.Join(root, "prdash-pr-7") {
		t.Fatalf("Audit returned %+v, want only our own worktree", entries)
	}
	for _, e := range entries {
		if strings.Contains(e.Path, gitSeparator) {
			t.Errorf("Audit listed something from inside a .git: %q", e.Path)
		}
	}
	for _, w := range g.List(context.Background()) {
		if strings.Contains(w.Path, gitSeparator) {
			t.Errorf("List returned something from inside a .git: %q", w.Path)
		}
	}
}

func TestTheNativeProvisioningDelegatesAuditToTheScanAndPropagatesItsFailures(t *testing.T) {
	root := t.TempDir()
	repo := repoWithBranch(t, "feat/x")
	g := NewGitDirect(root)
	if _, err := g.Create(context.Background(), Spec{
		Repo: repo, Branch: "feat/x", Path: filepath.Join(root, "prdash-pr-7"),
		Label: "prdash-pr-7",
	}); err != nil {
		t.Fatal(err)
	}
	native := NewHerdrNative(&fakeRunner{available: true}, root)

	entries := native.Audit(context.Background())
	if len(entries) != 1 {
		t.Errorf("native Audit returned %d entries, want 1", len(entries))
	}
	if len(native.List(context.Background())) != 1 {
		t.Errorf("native List returned %d worktrees, want 1", len(native.List(context.Background())))
	}

	failing := NewHerdrNative(&fakeRunner{
		available: true, createErr: errors.New("workspace_limit"),
	}, filepath.Join(t.TempDir(), "raiz-vacia"))
	_, err := failing.Create(context.Background(), Spec{
		Repo: repo, Branch: "feat/x", Path: filepath.Join(t.TempDir(), "prdash-pr-7"),
		Label: "prdash-pr-7",
	})
	if err == nil {
		t.Fatal("native Create with Herdr failing gave nil: the popup would open without a review")
	}
	if !strings.Contains(err.Error(), "workspace_limit") {
		t.Errorf("the error %q does not carry Herdr's cause", err)
	}
	if !strings.Contains(err.Error(), "create the native worktree") {
		t.Errorf("the error %q does not say the native creation failed", err)
	}

	if _, err := failing.Create(context.Background(), Spec{
		Repo: repo, Branch: "feat/x", Path: filepath.Join(t.TempDir(), "no-existe"),
		Label: "prdash-pr-7",
	}); err == nil {
		t.Error("native Create over an empty destination with Herdr not creating gave nil")
	}
}
