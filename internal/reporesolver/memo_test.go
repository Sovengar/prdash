package reporesolver

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"prdash/internal/forge/model"
	"prdash/internal/testutil"
)

func TestRemovingTheBareDeletesWhatIsThereAndToleratesWhatIsNot(t *testing.T) {
	r := New(Options{MemoPath: filepath.Join(t.TempDir(), "memo.json")})
	ref := model.RepoRef{Forge: "github", Host: "github.com", Project: "o/r", Owner: "o", Name: "r"}

	bare := r.barePath(ref)
	if _, err := os.Stat(bare); err == nil {
		t.Fatal("the bare already existed before testing")
	}
	if err := r.RemoveBare(ref); err != nil {
		t.Errorf("removing a bare that does not exist gave %v, want nil: the cleanup cannot "+
			"fail and mask the mount error", err)
	}
	// Checking the parent would prove nothing here.
	if _, err := os.Stat(bare); err == nil {
		t.Error("removing a nonexistent bare left it created")
	}

	if err := os.MkdirAll(filepath.Join(bare, "objects", "pack"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(bare, "HEAD"), []byte("ref: refs/heads/main\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := r.RemoveBare(ref); err != nil {
		t.Fatalf("RemoveBare: %v", err)
	}
	if _, err := os.Stat(bare); err == nil {
		t.Error("the bare is still on disk after removing it")
	}
	if _, err := os.Stat(filepath.Dir(bare)); err != nil {
		t.Errorf("RemoveBare took the parent directory with it: %v", err)
	}

	if err := r.RemoveBare(ref); err != nil {
		t.Errorf("the second time gave %v, want nil", err)
	}

	other := model.RepoRef{Forge: "github", Host: "github.com", Project: "o/other", Owner: "o", Name: "other"}
	if err := os.MkdirAll(r.barePath(other), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := r.RemoveBare(ref); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(r.barePath(other)); err != nil {
		t.Errorf("removing one repo took the other's bare with it: %v", err)
	}
}

// Pruning hidden directories is noise with consequences: a `.git` directory is hidden.
func TestTheIndexPrunesHiddenDirsAndDoesNotIndexNonRepos(t *testing.T) {
	base := t.TempDir()

	repo := filepath.Join(base, "project")
	testutil.InitRepo(t, repo)
	testutil.CommitFile(t, repo, "base.txt", "base", "base")
	testutil.SetRemote(t, repo, "origin", "https://github.com/acme/project.git")

	hidden := filepath.Join(base, ".cache")
	if err := os.MkdirAll(hidden, 0o755); err != nil {
		t.Fatal(err)
	}
	inside := filepath.Join(hidden, "dentro")
	testutil.InitRepo(t, inside)
	testutil.CommitFile(t, inside, "x.txt", "x", "x")
	testutil.SetRemote(t, inside, "origin", "https://github.com/other/hidden.git")

	if err := os.MkdirAll(filepath.Join(base, "normal"), 0o755); err != nil {
		t.Fatal(err)
	}
	noRemote := filepath.Join(base, "without-remote")
	testutil.InitRepo(t, noRemote)
	testutil.CommitFile(t, noRemote, "y.txt", "y", "y")

	r := resolverWithHosts(t, base)
	idx := r.buildIndex()

	goodKey := repoKey(model.RepoRef{Forge: "github", Host: "github.com", Project: "acme/project",
		Owner: "acme", Name: "project"})
	hiddenKey := repoKey(model.RepoRef{Forge: "github", Host: "github.com", Project: "other/hidden",
		Owner: "other", Name: "hidden"})

	if _, ok := idx[goodKey]; !ok {
		t.Errorf("the normal repo was not indexed: %v", idx)
	}
	if _, ok := idx[hiddenKey]; ok {
		t.Errorf("a repo inside a hidden directory was indexed: %v", idx)
	}
	for k, local := range idx {
		if strings.Contains(local, string(os.PathSeparator)+".cache") {
			t.Errorf("an index entry points inside a hidden dir: %s -> %s", k, local)
		}
		if !strings.HasPrefix(local, base) {
			t.Errorf("an index entry points outside the root: %s -> %s", k, local)
		}
		// The key is the canonical one, which is what lets ResolveLocal and the index agree.
		if !strings.Contains(k, "acme/project") {
			t.Errorf("an index key does not look canonical: %q", k)
		}
	}
	if got := idx[goodKey]; got != repo {
		t.Errorf("the index entry points at %q, want %q", got, repo)
	}
}

// Not general robustness: a configured root that does not exist is normal (an unmounted volume).
func TestAMissingRootDoesNotTakeDownTheGoodOne(t *testing.T) {
	base := t.TempDir()
	repo := filepath.Join(base, "project")
	testutil.InitRepo(t, repo)
	testutil.CommitFile(t, repo, "base.txt", "base", "base")
	testutil.SetRemote(t, repo, "origin", "https://github.com/acme/project.git")

	good := repoKey(model.RepoRef{Forge: "github", Host: "github.com", Project: "acme/project",
		Owner: "acme", Name: "project"})
	missing := filepath.Join(base, "no-existe")
	withBad := resolverWithHosts(t, missing, base).buildIndex()
	goodOnly := resolverWithHosts(t, base).buildIndex()

	if _, ok := withBad[good]; !ok {
		t.Fatalf("a nonexistent root took down the good one: %v", withBad)
	}
	if len(withBad) != len(goodOnly) {
		t.Errorf("a nonexistent root changed the index: %d entries with the bad one, %d without it",
			len(withBad), len(goodOnly))
	}
	file := filepath.Join(base, "a-file")
	if err := os.WriteFile(file, []byte("I am not a directory"), 0o644); err != nil {
		t.Fatal(err)
	}
	withFile := resolverWithHosts(t, file, base).buildIndex()
	if len(withFile) != len(goodOnly) {
		t.Errorf("a root that is a file changed the index: %v", withFile)
	}

	empty := resolverWithHosts(t).buildIndex()
	if len(empty) != 0 {
		t.Errorf("with no roots it gave %v", empty)
	}
}

// Without the mapping, parseRemote does not know the host.
func resolverWithHosts(t *testing.T, roots ...string) *Resolver {
	t.Helper()
	return New(Options{
		Roots:    roots,
		MemoPath: filepath.Join(t.TempDir(), "memo.json"),
		Hosts:    map[string]string{"github.com": "github"},
	})
}
