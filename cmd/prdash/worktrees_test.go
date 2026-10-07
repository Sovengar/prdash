package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"bytes"
	"prdash/internal/testutil"
	"prdash/internal/worktree"
)

func worktreeRepoFixture(t *testing.T) (repo, base, owned, foreign string) {
	t.Helper()
	repo = filepath.Join(t.TempDir(), "repo")
	testutil.InitRepo(t, repo)
	testutil.CommitFile(t, repo, "base.txt", "base", "base")
	testutil.RunGit(t, repo, "branch", "own")
	testutil.RunGit(t, repo, "branch", "foreign")

	base = t.TempDir()
	owned = filepath.Join(base, "prdash-pr-1")
	foreign = filepath.Join(base, "other-tool")
	testutil.RunGit(t, repo, "worktree", "add", "--quiet", owned, "own")
	testutil.RunGit(t, repo, "worktree", "add", "--quiet", foreign, "foreign")
	return repo, base, owned, foreign
}

func worktreeFixture(t *testing.T) (base, owned, foreign string) {
	t.Helper()
	_, base, owned, foreign = worktreeRepoFixture(t)
	return base, owned, foreign
}

func worktreeOrphanFixture(t *testing.T) (base string, orphans []string, healthy, foreign string) {
	t.Helper()
	base = t.TempDir()

	repoOrphans := filepath.Join(t.TempDir(), "repo-orphans")
	testutil.InitRepo(t, repoOrphans)
	testutil.CommitFile(t, repoOrphans, "base.txt", "base", "base")
	testutil.RunGit(t, repoOrphans, "branch", "one")
	testutil.RunGit(t, repoOrphans, "branch", "two")
	o1 := filepath.Join(base, "prdash-pr-1")
	o2 := filepath.Join(base, "prdash-pr-2")
	testutil.RunGit(t, repoOrphans, "worktree", "add", "--quiet", o1, "one")
	testutil.RunGit(t, repoOrphans, "worktree", "add", "--quiet", o2, "two")

	repoHealthy := filepath.Join(t.TempDir(), "repo-healthy")
	testutil.InitRepo(t, repoHealthy)
	testutil.CommitFile(t, repoHealthy, "base.txt", "base", "base")
	testutil.RunGit(t, repoHealthy, "branch", "healthy")
	testutil.RunGit(t, repoHealthy, "branch", "foreign")
	healthy = filepath.Join(base, "prdash-pr-3")
	foreign = filepath.Join(base, "other-tool")
	testutil.RunGit(t, repoHealthy, "worktree", "add", "--quiet", healthy, "healthy")
	testutil.RunGit(t, repoHealthy, "worktree", "add", "--quiet", foreign, "foreign")

	if err := os.RemoveAll(repoOrphans); err != nil {
		t.Fatal(err)
	}
	return base, []string{o1, o2}, healthy, foreign
}

type fakeProvisioner struct {
	entries []worktree.Entry
	delays  map[string]time.Duration
	removed []string
}

func (f *fakeProvisioner) Create(context.Context, worktree.Spec) (worktree.Worktree, error) {
	return worktree.Worktree{}, nil
}
func (f *fakeProvisioner) RemoveIfClean(context.Context, string) (bool, string, error) {
	return false, "", nil
}
func (f *fakeProvisioner) List(context.Context) []worktree.Worktree { return nil }
func (f *fakeProvisioner) Audit(context.Context) []worktree.Entry   { return f.entries }
func (f *fakeProvisioner) Remove(ctx context.Context, id string) error {
	if d := f.delays[id]; d > 0 {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(d):
		}
	}
	f.removed = append(f.removed, id)
	return nil
}

// Each deletion in the batch has its own budget.
func TestRunWorktreesRemoveOrphansPerItemBudget(t *testing.T) {
	const (
		slow = "/base/prdash-pr-1"
		fast = "/base/prdash-pr-2"
	)
	pr := &fakeProvisioner{
		entries: []worktree.Entry{
			{Worktree: worktree.Worktree{Path: slow}, Orphan: true},
			{Worktree: worktree.Worktree{Path: fast}, Orphan: true},
		},
		delays: map[string]time.Duration{slow: 200 * time.Millisecond},
	}

	var stdout, stderr bytes.Buffer
	code := removeWorktreesWithin(pr, &stdout, &stderr, true, false, nil, 40*time.Millisecond)

	if code != 1 {
		t.Fatalf("an item that exhausts its budget should mark a failure, code=%d", code)
	}
	if errOut := stderr.String(); !strings.Contains(errOut, slow) {
		t.Errorf("stderr = %q, want it to name the item that exhausted its budget", errOut)
	}
	if len(pr.removed) != 1 || pr.removed[0] != fast {
		t.Fatalf("removed = %v, want the second item removed despite the first", pr.removed)
	}
}

func TestRunWorktreesListsOnlyOwned(t *testing.T) {
	base, owned, foreign := worktreeFixture(t)
	pr := worktree.NewGitDirect(base)

	var stdout, stderr bytes.Buffer
	code := runWorktrees(pr, &stdout, &stderr, []string{"list"})
	if code != 0 {
		t.Fatalf("exit code = %d (stderr: %s)", code, stderr.String())
	}
	out := stdout.String()
	for _, want := range []string{"prdash-pr-1", owned, "own", "ok"} {
		if !strings.Contains(out, want) {
			t.Errorf("the listing does not contain %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, foreign) || strings.Contains(out, "other-tool") {
		t.Errorf("the listing should not include foreign worktrees:\n%s", out)
	}
}

func TestRunWorktreesDefaultsToList(t *testing.T) {
	base, _, _ := worktreeFixture(t)
	var stdout, stderr bytes.Buffer
	code := runWorktrees(worktree.NewGitDirect(base), &stdout, &stderr, nil)
	out := stdout.String()
	if code != 0 || !strings.Contains(out, "prdash-pr-1") {
		t.Fatalf("code=%d out=%q stderr=%q", code, out, stderr.String())
	}
}

func TestRunWorktreesRemoveRefusesForeign(t *testing.T) {
	base, owned, foreign := worktreeFixture(t)
	pr := worktree.NewGitDirect(base)

	var stdout, stderr bytes.Buffer
	if code := runWorktrees(pr, &stdout, &stderr, []string{"remove", foreign}); code == 0 {
		t.Fatal("removing a foreign worktree should fail")
	}
	if !worktree.Exists(foreign) {
		t.Fatal("the foreign worktree should not have been touched")
	}
	if !worktree.Exists(owned) {
		t.Fatal("the owned worktree should not be touched when rejecting another one")
	}
}

func TestRunWorktreesRemoveOwned(t *testing.T) {
	base, owned, foreign := worktreeFixture(t)
	pr := worktree.NewGitDirect(base)

	var stdout, stderr bytes.Buffer
	if code := runWorktrees(pr, &stdout, &stderr, []string{"remove", owned}); code != 0 {
		t.Fatalf("exit code = %d", code)
	}
	if worktree.Exists(owned) {
		t.Fatal("the owned worktree should have been removed")
	}
	if !worktree.Exists(foreign) {
		t.Fatal("the foreign worktree should not be touched")
	}
}

func TestRunWorktreesRemoveOrphan(t *testing.T) {
	repo, base, owned, _ := worktreeRepoFixture(t)
	if err := os.RemoveAll(repo); err != nil {
		t.Fatal(err)
	}
	pr := worktree.NewGitDirect(base)

	var stdout, stderr bytes.Buffer
	if code := runWorktrees(pr, &stdout, &stderr, []string{"remove", owned}); code != 0 {
		t.Fatalf("removing your own orphan should work, code=%d", code)
	}
	if worktree.Exists(owned) {
		t.Fatal("the orphaned checkout should have been removed")
	}
}

func TestRunWorktreesUsageErrors(t *testing.T) {
	base, _, _ := worktreeFixture(t)
	pr := worktree.NewGitDirect(base)

	var stdout, stderr bytes.Buffer
	if code := runWorktrees(pr, &stdout, &stderr, []string{"bogus"}); code != 2 {
		t.Fatalf("an unknown subcommand should exit with 2, got %d", code)
	}
	if code := runWorktrees(pr, &stdout, &stderr, []string{"remove"}); code != 2 {
		t.Fatalf("remove without paths should exit with 2, got %d", code)
	}
}

func TestRunWorktreesRemoveNonexistentRefused(t *testing.T) {
	base, owned, foreign := worktreeFixture(t)
	pr := worktree.NewGitDirect(base)
	missing := filepath.Join(base, "prdash-pr-404")

	var stdout, stderr bytes.Buffer
	code := runWorktrees(pr, &stdout, &stderr, []string{"remove", missing})
	if code != 1 {
		t.Fatalf("exit code = %d, want 1", code)
	}
	if errOut := stderr.String(); !strings.Contains(errOut, "not a prdash worktree") {
		t.Errorf("stderr = %q, want the rejection of the path", errOut)
	}
	if _, err := os.Stat(missing); !os.IsNotExist(err) {
		t.Errorf("the nonexistent path should not have been created: %v", err)
	}
	if !worktree.Exists(owned) || !worktree.Exists(foreign) {
		t.Error("a refusal should not touch existing worktrees")
	}
}

func TestRunWorktreesRemoveOrphansMalformedGitDoesNotBreakBatch(t *testing.T) {
	base, orphans, healthy, _ := worktreeOrphanFixture(t)
	malformed := orphans[0]
	if err := os.WriteFile(filepath.Join(malformed, ".git"), []byte("garbage\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	pr := worktree.NewGitDirect(base)

	var stdout, stderr bytes.Buffer
	code := runWorktrees(pr, &stdout, &stderr, []string{"remove", "--orphans"})
	out := stdout.String()
	if code != 0 {
		t.Fatalf("an orphan with an unresolvable .git should not fail the batch: code=%d\n%s", code, out)
	}
	for _, o := range orphans {
		if worktree.Exists(o) {
			t.Errorf("%s should have been removed", o)
		}
		if !strings.Contains(out, o) {
			t.Errorf("the output does not name %s:\n%s", o, out)
		}
	}
	if !worktree.Exists(healthy) {
		t.Error("the healthy worktree should not be touched")
	}
}

func TestRunWorktreesRemoveTwoExplicitPaths(t *testing.T) {
	repo := filepath.Join(t.TempDir(), "repo")
	testutil.InitRepo(t, repo)
	testutil.CommitFile(t, repo, "base.txt", "base", "base")
	testutil.RunGit(t, repo, "branch", "one")
	testutil.RunGit(t, repo, "branch", "two")
	testutil.RunGit(t, repo, "branch", "foreign")

	base := t.TempDir()
	owned1 := filepath.Join(base, "prdash-pr-1")
	owned2 := filepath.Join(base, "prdash-pr-2")
	foreign := filepath.Join(base, "other-tool")
	testutil.RunGit(t, repo, "worktree", "add", "--quiet", owned1, "one")
	testutil.RunGit(t, repo, "worktree", "add", "--quiet", owned2, "two")
	testutil.RunGit(t, repo, "worktree", "add", "--quiet", foreign, "foreign")

	pr := worktree.NewGitDirect(base)
	var stdout, stderr bytes.Buffer
	code := runWorktrees(pr, &stdout, &stderr, []string{"remove", owned1, owned2})
	out := stdout.String()
	if code != 0 {
		t.Fatalf("exit code = %d, out=%q", code, out)
	}
	if worktree.Exists(owned1) || worktree.Exists(owned2) {
		t.Error("the two owned paths should have been removed")
	}
	if !worktree.Exists(foreign) {
		t.Error("the foreign worktree should not be touched")
	}
}

func TestRunWorktreesRemoveOrphansDryRunExcludesOthers(t *testing.T) {
	base, orphans, healthy, foreign := worktreeOrphanFixture(t)
	pr := worktree.NewGitDirect(base)

	var stdout, stderr bytes.Buffer
	code := runWorktrees(pr, &stdout, &stderr, []string{"remove", "--orphans", "--dry-run"})
	out := stdout.String()
	if code != 0 {
		t.Fatalf("exit code = %d, out=%q", code, out)
	}
	for _, o := range orphans {
		if !strings.Contains(out, o) {
			t.Errorf("the batch should include %s:\n%s", o, out)
		}
	}
	if strings.Contains(out, healthy) {
		t.Errorf("the batch should not include the healthy worktree %s:\n%s", healthy, out)
	}
	if strings.Contains(out, foreign) {
		t.Errorf("the batch should not include the foreign worktree %s:\n%s", foreign, out)
	}
}

func TestRunWorktreesRemoveMalformedGitOrphanByPath(t *testing.T) {
	base, orphans, healthy, _ := worktreeOrphanFixture(t)
	malformed := orphans[0]
	if err := os.WriteFile(filepath.Join(malformed, ".git"), []byte("garbage\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	pr := worktree.NewGitDirect(base)

	var code int
	var stdout, stderr bytes.Buffer
	code = runWorktrees(pr, &stdout, &stderr, []string{"remove", malformed})
	out := stdout.String()
	if code != 0 {
		t.Fatalf("removing by path an orphan with an unresolvable .git should work: code=%d\n%s", code, out)
	}
	if worktree.Exists(malformed) {
		t.Error("the named orphan should have been removed")
	}
	if !worktree.Exists(orphans[1]) {
		t.Error("the other orphan should not be touched when removing a single path")
	}
	if !worktree.Exists(healthy) {
		t.Error("the healthy worktree should not be touched")
	}
}

func TestRunWorktreesRemoveOrphans(t *testing.T) {
	base, orphans, healthy, foreign := worktreeOrphanFixture(t)
	pr := worktree.NewGitDirect(base)

	var stdout, stderr bytes.Buffer
	code := runWorktrees(pr, &stdout, &stderr, []string{"remove", "--orphans"})
	out := stdout.String()
	if code != 0 {
		t.Fatalf("exit code = %d, out=%q", code, out)
	}
	for _, o := range orphans {
		if worktree.Exists(o) {
			t.Errorf("the orphan %s should have been removed", o)
		}
		if !strings.Contains(out, o) {
			t.Errorf("the output does not name the orphan %s:\n%s", o, out)
		}
	}
	if !worktree.Exists(healthy) {
		t.Error("your own healthy worktree should not be removed")
	}
	if !worktree.Exists(foreign) {
		t.Error("the foreign worktree should not be touched")
	}
}

func TestRunWorktreesRemoveOrphansDryRun(t *testing.T) {
	base, orphans, healthy, foreign := worktreeOrphanFixture(t)
	pr := worktree.NewGitDirect(base)

	var stdout, stderr bytes.Buffer
	code := runWorktrees(pr, &stdout, &stderr, []string{"remove", "--orphans", "--dry-run"})
	out := stdout.String()
	if code != 0 {
		t.Fatalf("exit code = %d, out=%q", code, out)
	}
	for _, o := range orphans {
		if !strings.Contains(out, o) {
			t.Errorf("--dry-run should print %s:\n%s", o, out)
		}
		if !worktree.Exists(o) {
			t.Errorf("--dry-run should not remove %s", o)
		}
	}
	if !worktree.Exists(healthy) || !worktree.Exists(foreign) {
		t.Error("--dry-run should touch neither the healthy nor the foreign worktree")
	}
}

func TestRunWorktreesRemoveOrphansNoneIsSuccess(t *testing.T) {
	base, _, _ := worktreeFixture(t)
	pr := worktree.NewGitDirect(base)

	var stdout, stderr bytes.Buffer
	code := runWorktrees(pr, &stdout, &stderr, []string{"remove", "--orphans"})
	out, errOut := stdout.String(), stderr.String()
	if code != 0 {
		t.Fatalf("zero orphans should exit with 0, got %d", code)
	}
	if !strings.Contains(out, "no orphan") {
		t.Errorf("it should report that there are no orphans:\n%s", out)
	}
	if errOut != "" {
		t.Errorf("stderr = %q, want empty", errOut)
	}
}

func TestRunWorktreesRemoveOrphansDryRunNoneIsSuccess(t *testing.T) {
	base, _, _ := worktreeFixture(t)
	pr := worktree.NewGitDirect(base)

	var stdout, stderr bytes.Buffer
	code := runWorktrees(pr, &stdout, &stderr, []string{"remove", "--orphans", "--dry-run"})
	out, errOut := stdout.String(), stderr.String()
	if code != 0 {
		t.Fatalf("zero orphans with --dry-run should exit with 0, got %d", code)
	}
	if strings.Contains(out, base) {
		t.Errorf("it should not print any orphan path:\n%s", out)
	}
	if errOut != "" {
		t.Errorf("stderr = %q, want empty", errOut)
	}
}

func TestRunWorktreesRemoveOrphansUsageErrors(t *testing.T) {
	base, orphans, healthy, _ := worktreeOrphanFixture(t)
	pr := worktree.NewGitDirect(base)

	cases := []struct {
		name string
		args []string
		want string
	}{
		{"orphans and paths", []string{"remove", "--orphans", healthy}, "cannot be mixed"},
		{"dry-run without orphans", []string{"remove", "--dry-run"}, "--orphans"},
		{"unknown flag", []string{"remove", "--bogus"}, "unknown flag"},
		{"typo of orphans", []string{"remove", "--orphan"}, "unknown flag"},
		{"path starting with a dash", []string{"remove", "-prdash-pr-1"}, "unknown flag"},
		{"no paths and no orphans", []string{"remove"}, "path"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var out, errOut bytes.Buffer
			code := runWorktrees(pr, &out, &errOut, tc.args)
			stderr := errOut.String()
			if code != 2 {
				t.Fatalf("exit code = %d, want 2", code)
			}
			if !strings.Contains(stderr, tc.want) {
				t.Errorf("stderr = %q, want it to mention %q", stderr, tc.want)
			}
			for _, o := range orphans {
				if !worktree.Exists(o) {
					t.Errorf("an invalid usage should not remove %s", o)
				}
			}
			if !worktree.Exists(healthy) {
				t.Error("an invalid usage should not remove the healthy worktree")
			}
		})
	}
}
