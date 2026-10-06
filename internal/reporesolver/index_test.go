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

// The three discards are different and all three matter.
func TestTheIndexIgnoresWhatIsNotARepoWithAParsableRemote(t *testing.T) {
	root := t.TempDir()

	good := filepath.Join(root, "bueno")
	testutil.InitRepo(t, good)
	testutil.CommitFile(t, good, "a.txt", "a", "a")
	testutil.SetRemote(t, good, "origin", "https://github.com/acme/project.git")

	noRemote := filepath.Join(root, "without-remote")
	testutil.InitRepo(t, noRemote)
	testutil.CommitFile(t, noRemote, "a.txt", "a", "a")

	withPath := filepath.Join(root, "with-path")
	testutil.InitRepo(t, withPath)
	testutil.CommitFile(t, withPath, "a.txt", "a", "a")
	testutil.SetRemote(t, withPath, "origin", filepath.Join(root, "other-lugar"))

	plain := filepath.Join(root, "plano")
	if err := os.MkdirAll(plain, 0o755); err != nil {
		t.Fatal(err)
	}

	r := New(Options{
		Roots:    []string{root},
		CloneDir: filepath.Join(t.TempDir(), "clones"),
		MemoPath: filepath.Join(t.TempDir(), "memo.json"),
		Hosts:    map[string]string{"github.com": "github"},
	})
	idx := r.buildIndex()

	ref := model.RepoRef{Forge: "github", Host: "github.com", Project: "acme/project"}
	key := repoKey(ref)
	if got, ok := idx[key]; !ok {
		t.Errorf("the repo with a GitHub remote was not indexed; the index has %v", idx)
	} else if got != good {
		t.Errorf("the repo ended up at %q, want %q", got, good)
	}
	for _, notRepo := range []string{noRemote, withPath, plain} {
		for k, v := range idx {
			if v == notRepo {
				t.Errorf("%q was indexed under key %q: it is not a repo with a parsable remote", notRepo, k)
			}
		}
	}
	if len(idx) != 1 {
		t.Errorf("the index has %d entries, want 1: %v", len(idx), idx)
	}
}

// Two similar things that are not the same: `.git` is the directory and a linked worktree is a FILE.
func TestTheIndexDoesNotEnterGitDirsNorWorktrees(t *testing.T) {
	root := t.TempDir()
	repo := filepath.Join(root, "repo")
	testutil.InitRepo(t, repo)
	testutil.CommitFile(t, repo, "a.txt", "a", "a")
	testutil.SetRemote(t, repo, "origin", "https://github.com/acme/project.git")

	wt := filepath.Join(root, "my-worktree")
	testutil.RunGit(t, repo, "branch", "feat/x")
	testutil.RunGit(t, repo, "worktree", "add", "--quiet", wt, "feat/x")

	r := New(Options{
		Roots:    []string{root},
		CloneDir: filepath.Join(t.TempDir(), "clones"),
		MemoPath: filepath.Join(t.TempDir(), "memo.json"),
		Hosts:    map[string]string{"github.com": "github"},
	})
	idx := r.buildIndex()

	for k, v := range idx {
		if strings.Contains(v, string(filepath.Separator)+"my-worktree") {
			t.Errorf("the worktree was indexed as a repo of its own under %q: %q", k, v)
		}
	}
	ref := model.RepoRef{Forge: "github", Host: "github.com", Project: "acme/project"}
	if got := idx[repoKey(ref)]; got != repo {
		t.Errorf("the index resolved to %q, want the main repo %q", got, repo)
	}
	for _, v := range idx {
		if strings.Contains(v, string(filepath.Separator)+".git"+string(filepath.Separator)) {
			t.Errorf("the index has an entry from inside a .git: %q", v)
		}
	}
}

// Proved with the real thing, not a double: filepath.Abs on a RELATIVE root whose working
// directory is gone.
func TestAnInaccessibleRootDoesNotBreakTheIndex(t *testing.T) {
	vanished := filepath.Join(t.TempDir(), "is-valid")
	if err := os.MkdirAll(vanished, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(vanished); err != nil {
		t.Fatal(err)
	}

	good := filepath.Join(t.TempDir(), "bueno")
	testutil.InitRepo(t, good)
	testutil.CommitFile(t, good, "a.txt", "a", "a")
	testutil.SetRemote(t, good, "origin", "https://github.com/acme/project.git")

	t.Run("relative root with dead cwd", func(t *testing.T) {
		other := filepath.Join(t.TempDir(), "cwd-muerto")
		if err := os.MkdirAll(other, 0o755); err != nil {
			t.Fatal(err)
		}
		t.Chdir(other)

		r := New(Options{
			Roots:    []string{"repo-that-does-not-exist", good},
			CloneDir: filepath.Join(t.TempDir(), "clones"),
			MemoPath: filepath.Join(t.TempDir(), "memo.json"),
			Hosts:    map[string]string{"github.com": "github"},
		})
		idx := r.buildIndex()
		ref := model.RepoRef{Forge: "github", Host: "github.com", Project: "acme/project"}
		if got := idx[repoKey(ref)]; got != good {
			t.Errorf("an invalid root took down the whole index: it resolved to %q, want %q",
				got, good)
		}
	})

	r := New(Options{
		Roots:    []string{filepath.Join(t.TempDir(), "no-existe"), good},
		CloneDir: filepath.Join(t.TempDir(), "clones"),
		MemoPath: filepath.Join(t.TempDir(), "memo.json"),
		Hosts:    map[string]string{"github.com": "github"},
	})
	idx := r.buildIndex()
	ref := model.RepoRef{Forge: "github", Host: "github.com", Project: "acme/project"}
	if got := idx[repoKey(ref)]; got != good {
		t.Errorf("a nonexistent root took down the whole index: it resolved to %q, want %q", got, good)
	}
}

func TestARemoteWithAnEmptyPartDoesNotBecomeARepo(t *testing.T) {
	for _, remote := range []string{
		"https://github.com//project.git",
		"https://github.com/acme//project.git",
		"https://github.com///.git",
		"git@github.com:acme//project.git",
		"ssh://git@github.com",
		"not-a-remote",
	} {
		ref, ok := parseRemoteForTest(remote)
		if ok {
			t.Errorf("%q became a repo: %+v", remote, ref)
			continue
		}
		if ref.Project != "" {
			t.Errorf("%q gave ok=false but Project=%q: a Caller that ignored the ok "+
				"would get an index key with a slash", remote, ref.Project)
		}
	}

	// The good path, because otherwise "everything is rejected" would count as a test.
	// Three forms that my first version put on the rejected list.
	for _, good := range []string{
		"https://github.com/acme/project.git",
		"https://github.com/acme/project//.git",
		"git@github.com:acme/project.git",
		"ssh://git@github.com/acme/project.git",
	} {
		if _, ok := parseRemoteForTest(good); !ok {
			t.Errorf("%q was not interpreted, and it is a valid remote form", good)
		}
	}

	ref, ok := parseRemoteForTest("https://github.com/acme/group/project.git")
	if !ok {
		t.Fatal("a well-written remote was not interpreted")
	}
	if ref.Project != "acme/group/project" {
		t.Errorf("Project = %q, want acme/group/project: the project is the COMPLETE path, "+
			"with groups, because that is what goes to `git clone`", ref.Project)
	}
	if ref.Forge != "github" || ref.Host != "github.com" {
		t.Errorf("forge/host = %s/%s", ref.Forge, ref.Host)
	}
	if ref.Owner != "group" || ref.Name != "project" {
		t.Errorf("Owner/Name = %s/%s, want group/project", ref.Owner, ref.Name)
	}
}

// It delegates to the package's REAL parsing, with the same hosts table production uses, so it does
// not test a copy.
func parseRemoteForTest(raw string) (model.RepoRef, bool) {
	return ParseRemoteURL(raw, testHosts(), nil)
}

// The message has to say WHICH ref was attempted, because there are two.
func TestFetchReviewRefPropagatesTheFetchFailureWithItsRef(t *testing.T) {
	origin, repo := fixture(t)
	r := newResolver(t, origin, ghRef())
	it := model.NewItem(ghRef(), 7)
	it.Number = 7

	_, err := r.FetchReviewRef(context.Background(), repo, it)
	if err == nil {
		t.Fatal("fetching a ref that does not exist gave nil")
	}
	if !strings.Contains(err.Error(), "fetch") {
		t.Errorf("the error %q does not say the fetch failed", err)
	}
	if !strings.Contains(err.Error(), "acme") {
		t.Errorf("the error %q does not name the project it failed on", err)
	}
	for _, ref := range []string{"prdash/pr-7", "refs/prdash/github/7"} {
		if testutil.RefExists(t, repo, ref) {
			t.Errorf("the failed fetch left %s in the repo: Create would reuse it and the "+
				"review would mount empty", ref)
		}
	}
}
