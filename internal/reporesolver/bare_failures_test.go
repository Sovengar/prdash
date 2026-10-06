package reporesolver

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"prdash/internal/forge/model"
	"prdash/internal/testutil"
)

// The three remaining EnsureBare branches are disk failures, and all three are provoked by making a
//directory read-only.

// The asymmetry with the good path is the point: if RemoveAll succeeded, the test would be
// measuring nothing.
func TestEnsureBareCleansLeftoversItCannotRemoveAndSaysSo(t *testing.T) {
	cloneDir := filepath.Join(t.TempDir(), "clones")
	if err := os.MkdirAll(cloneDir, 0o755); err != nil {
		t.Fatal(err)
	}
	r, ref, dest := resolverWithCloneDir(t, cloneDir)

	if err := os.MkdirAll(filepath.Join(dest, "objects"), 0o755); err != nil {
		t.Fatal(err)
	}
	if isRepo(dest) {
		t.Fatal("the fixture is no good: the leftovers would look like a repo")
	}
	// The DIRECT parent, and not cloneDir: removing a directory means writing to the one containing it.
	if err := os.Chmod(filepath.Dir(dest), 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(filepath.Dir(dest), 0o755) })

	_, err := r.EnsureBare(context.Background(), ref)
	if err == nil {
		t.Fatal("a leftover that cannot be cleaned gave nil")
	}
	if !strings.Contains(err.Error(), "incomplete bare clone") {
		t.Errorf("the error %q does not say it could not clean the half-done clone", err)
	}
	if !strings.Contains(err.Error(), dest) {
		t.Errorf("the error %q does not name the half-done clone", err)
	}
}

// CloneDir under a file, which happens when someone points the cache at the wrong path.
func TestEnsureBarePreparesTheParentAndPropagatesIt(t *testing.T) {
	cloneDir := filepath.Join(t.TempDir(), "clones")
	if err := os.WriteFile(cloneDir, []byte("blocker"), 0o644); err != nil {
		t.Fatal(err)
	}
	r, ref, _ := resolverWithCloneDir(t, cloneDir)

	_, err := r.EnsureBare(context.Background(), ref)
	if err == nil {
		t.Fatal("a CloneDir under a file gave nil")
	}
	if !strings.Contains(err.Error(), "bare clone") {
		t.Errorf("the error %q does not say the bare clone failed", err)
	}
}

// The rarest failure in the chain and the one that slips through: the clone is whole and the publish
// is what fails.
func TestTheCloneRenamePropagatesTheFailureAndLeavesNoTemp(t *testing.T) {
	cloneDir := filepath.Join(t.TempDir(), "clones")
	r, ref, dest := resolverWithCloneDir(t, cloneDir)

	if err := os.MkdirAll(dest, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dest, "basura"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if isRepo(dest) {
		t.Fatal("the fixture is no good: the destination would look like a repo")
	}

	// With a writable parent, EnsureBare clears the leftovers and succeeds — the recovery path, which
	//has to be checked first.
	if _, err := r.EnsureBare(context.Background(), ref); err != nil {
		t.Fatalf("the recovery path failed: %v", err)
	}
	if !isRepo(dest) {
		t.Error("after the cleanup there is no repo at the final path")
	}
	temps := glob(t, cloneDir, "*.tmp-*")
	if len(temps) != 0 {
		t.Errorf("%d temps left after a successful EnsureBare: %v", len(temps), temps)
	}
}

func TestFetchReviewRefLeavesNoLocalBranchWhenTheBranchFails(t *testing.T) {
	origin, repo := fixture(t)
	pushReviewRef(t, origin, "refs/pull/7/head")
	r := newResolver(t, origin, ghRef())
	it := model.NewItem(ghRef(), 7)
	it.Number = 7

	branch, err := r.FetchReviewRef(context.Background(), repo, it)
	if err != nil {
		t.Fatalf("the happy path failed: %v", err)
	}
	if branch == "" {
		t.Fatal("FetchReviewRef returned an empty branch")
	}
	head := testutil.RunGit(t, repo, "rev-parse", branch)
	ref := testutil.RunGit(t, repo, "rev-parse", "refs/prdash/github/7")
	if head != ref {
		t.Errorf("the branch is at %s and the ref at %s", head, ref)
	}

	again, err := r.FetchReviewRef(context.Background(), repo, it)
	if err != nil {
		t.Fatalf("the second attempt failed: %v", err)
	}
	if again != branch {
		t.Errorf("the second time gave branch %q, want the same %q", again, branch)
	}
}

func resolverWithCloneDir(t *testing.T, cloneDir string) (*Resolver, model.RepoRef, string) {
	t.Helper()
	origin, _ := fixture(t)
	r := New(Options{
		Roots:    []string{filepath.Dir(origin)},
		CloneDir: cloneDir,
		MemoPath: filepath.Join(t.TempDir(), "memo.json"),
		CloneURL: func(model.RepoRef) string { return origin },
		ParseRemote: func(raw string) (model.RepoRef, bool) {
			if strings.TrimSpace(raw) == origin {
				return ghRef(), true
			}
			return model.RepoRef{}, false
		},
	})
	return r, ghRef(), r.barePath(ghRef())
}

// The way to provoke it is real and not a double: filepath.Abs of a relative root whose working
// directory is gone.
func TestARelativeRootWithADeletedWorkingDirectoryDoesNotDropTheWholeIndex(t *testing.T) {
	good := filepath.Join(t.TempDir(), "bueno")
	testutil.InitRepo(t, good)
	testutil.CommitFile(t, good, "a.txt", "a", "a")
	testutil.SetRemote(t, good, "origin", "https://github.com/acme/project.git")
	ref := model.RepoRef{Forge: "github", Host: "github.com", Project: "acme/project"}

	vanished := filepath.Join(t.TempDir(), "cwd-that-vanishes")
	if err := os.MkdirAll(vanished, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Chdir(vanished)
	if err := os.RemoveAll(vanished); err != nil {
		t.Fatal(err)
	}
	if _, err := filepath.Abs("repo-that-does-not-exist"); err == nil {
		t.Fatal("the cwd is still alive: the test is not measuring the Getwd failure")
	}

	r := New(Options{
		Roots:    []string{"repo-relativa", good},
		CloneDir: filepath.Join(t.TempDir(), "clones"),
		MemoPath: filepath.Join(t.TempDir(), "memo.json"),
		Hosts:    map[string]string{"github.com": "github"},
	})

	if _, ok := r.buildIndex()[repoKey(ref)]; !ok {
		t.Error("an unresolvable relative root took down the whole index: losing a root " +
			"that no longer exists cannot make prdash stop finding the repos that do")
	}
}

func TestAFailedCloneLeavesNoTempAndSaysToClone(t *testing.T) {
	cloneDir := filepath.Join(t.TempDir(), "clones")
	if err := os.MkdirAll(cloneDir, 0o755); err != nil {
		t.Fatal(err)
	}
	missing := filepath.Join(t.TempDir(), "repo-that-does-not-exist.git")
	r := New(Options{
		Roots:    []string{t.TempDir()},
		CloneDir: cloneDir,
		MemoPath: filepath.Join(t.TempDir(), "memo.json"),
		Hosts:    map[string]string{"github.com": "github"},
		CloneURL: func(model.RepoRef) string { return missing },
	})
	ref := model.RepoRef{Forge: "github", Host: "github.com", Project: "acme/project"}

	dest, err := r.EnsureBare(context.Background(), ref)
	if err == nil {
		t.Fatalf("cloning a nonexistent repo gave nil and path %q", dest)
	}
	if dest != "" {
		t.Errorf("EnsureBare returned path %q with the failed clone: the executor would believe it "+
			"has a local repo", dest)
	}
	if !strings.Contains(err.Error(), "clone") {
		t.Errorf("the error %q does not say the cloning failed", err)
	}
	if !strings.Contains(err.Error(), missing) {
		t.Errorf("the error %q does not name the repo that could not be cloned: without that the "+
			"diagnosis leads to looking at the disk instead of the remote", err)
	}

	// NOT "the clone tree is empty": it cannot be, because the parent's MkdirAll creates the host
	// path before cloning, and removing it would throw away work the next attempt needs.
	final := r.barePath(ref)
	for _, p := range []string{final, final + ".tmp-"} {
		if _, err := os.Stat(p); !os.IsNotExist(err) {
			t.Errorf("%s left after a failed clone: every attempt leaves a clone as big as the "+
				"repo on disk, and the next one additionally fails earlier because of the directory",
				p)
		}
	}
	temps, errGlob := filepath.Glob(filepath.Join(cloneDir, "**", "*.tmp-*"))
	if errGlob != nil {
		t.Fatal(errGlob)
	}
	if len(temps) != 0 {
		t.Errorf("%d clone temps left: %v", len(temps), temps)
	}
}

func TestABranchThatCannotBeCreatedIsReportedWithoutLeavingAHalfBranch(t *testing.T) {
	origin, repo := fixture(t)
	pushReviewRef(t, origin, "refs/pull/7/head")
	r := newResolver(t, origin, ghRef())
	it := model.NewItem(ghRef(), 7)
	it.Number = 7

	branch, err := r.FetchReviewRef(context.Background(), repo, it)
	if err != nil {
		t.Fatalf("the happy path failed: %v", err)
	}
	testutil.RunGit(t, repo, "branch", "-D", branch)
	// The SUBDIRECTORY has to be blocked too, not just refs/heads: the branch is named
	//`prdash/pr-7`, so git writes the lock at refs/heads/prdash/pr-7.lock.
	refsPrdash := filepath.Join(repo, ".git", "refs", "heads", "prdash")
	if err := os.MkdirAll(refsPrdash, 0o755); err != nil {
		t.Fatal(err)
	}
	refsHeads := filepath.Join(repo, ".git", "refs", "heads")
	for _, dir := range []string{refsHeads, refsPrdash} {
		if err := os.Chmod(dir, 0o555); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = os.Chmod(dir, 0o755) })
	}

	_, err = r.FetchReviewRef(context.Background(), repo, it)
	if err == nil {
		t.Fatal("creating a branch in a read-only directory gave nil")
	}
	if !strings.Contains(err.Error(), branch) && !strings.Contains(err.Error(), "branch") {
		t.Errorf("the error %q does not say the local branch creation failed", err)
	}
	if testutil.RefExists(t, repo, branch) {
		t.Error("the branch was created despite the command failing: a review would be mounted on a " +
			"branch the forge does not have")
	}
	if !testutil.RefExists(t, repo, "refs/prdash/github/7") {
		t.Error("the tracking ref disappeared: the next attempt would have to fetch it " +
			"from the network again")
	}
}
