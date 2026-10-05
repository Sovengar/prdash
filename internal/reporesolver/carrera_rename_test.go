package reporesolver

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"prdash/internal/forge/model"
	"prdash/internal/gitcmd"
	"prdash/internal/testutil"
)

// The case that fails the rename is NOT a race, which is what it looks like: the mounts come from
//Bubbletea's single update goroutine and are serialised by construction.

// The script derives dest from the temporary it was handed, rather than through the environment.
func gitPublishingTheCloneFirst(t *testing.T, source string) *gitcmd.Runner {
	t.Helper()
	dir := t.TempDir()
	script := filepath.Join(dir, "git")

	content := "#!/bin/sh\n" +
		"if [ \"$1\" = \"clone\" ]; then\n" +
		"  git \"$@\" || exit $?\n" +
		"  for ultimo; do :; done\n" +
		"  destino=\"${ultimo%%.tmp-*}\"\n" +
		"  git clone --bare --quiet -- " + source + " \"$destino\" >/dev/null 2>&1\n" +
		"  exit 0\n" +
		"fi\n" +
		"exec git \"$@\"\n"
	if err := os.WriteFile(script, []byte(content), 0o755); err != nil {
		t.Fatal(err)
	}
	return &gitcmd.Runner{Bin: script, Timeout: gitcmd.DefaultTimeout}
}

// The reason it warns is that `dest` being occupied means something unexpected, which deserves a
// warning rather than an assumption.
func TestAFailedRenameLeavesNoTempKeepsTheForeignCloneAndAllowsRetry(t *testing.T) {
	base := t.TempDir()

	// A normal remote with a commit, because a bare has no working tree.
	origin := filepath.Join(base, "fuente")
	testutil.InitRepo(t, origin)
	testutil.CommitFile(t, origin, "f.txt", "base", "base")

	cloneDir := filepath.Join(base, "clones")
	freshResolver := func() *Resolver {
		return New(Options{
			Roots:    []string{base},
			CloneDir: cloneDir,
			MemoPath: filepath.Join(base, "memo.json"),
			Hosts:    map[string]string{"github.com": "github"},
			CloneURL: func(model.RepoRef) string { return origin },
			Git:      gitPublishingTheCloneFirst(t, origin),
		})
	}
	ref := model.RepoRef{Forge: "github", Host: "github.com", Project: "acme/proyecto"}
	dest := freshResolver().barePath(ref)

	published, err := freshResolver().EnsureBare(context.Background(), ref)
	if err == nil {
		t.Fatalf("EnsureBare returned nil and %q: the clone was published over an occupied destination "+
			"without warning, and whoever occupies it may have lost its work", published)
	}

	// ENOTEMPTY and EXDEV are different repairs (an occupant and a different device) and without
	//the system's reason the diagnosis is the same for both.
	if !strings.Contains(err.Error(), "publish the bare clone") {
		t.Errorf("the warning %q does not say the publish failed", err)
	}
	if !strings.Contains(err.Error(), dest) {
		t.Errorf("the warning %q does not name the destination %s", err, dest)
	}
	if !strings.Contains(err.Error(), ".tmp-") {
		t.Errorf("the warning %q does not carry the system's reason, which is what says whether the "+
			"destination is occupied or the temp fell on another device", err)
	}
	// And it does NOT return a path with the error: the executor would believe it has a local repo.
	if published != "" {
		t.Errorf("EnsureBare returned %q with the error", published)
	}

	temps, errGlob := filepath.Glob(filepath.Join(cloneDir, "**", "*.tmp-*"))
	if errGlob != nil {
		t.Fatal(errGlob)
	}
	if len(temps) != 0 {
		t.Errorf("%d temps left after a failed publish: %v", len(temps), temps)
	}

	// The foreign clone stays whole and usable, which is what makes the warning the right call.
	if !isRepo(dest) {
		t.Errorf("EnsureBare ran over the clone that was already at %s: an occupant is "+
			"not necessarily garbage", dest)
	}
	r := freshResolver()
	p, err := r.EnsureBare(context.Background(), ref)
	if err != nil {
		t.Fatalf("the retry failed with %v: the previous error left the tree in a state from which "+
			"it cannot be retried without cleaning by hand", err)
	}
	if p != dest {
		t.Errorf("the retry returned %q, want %q", p, dest)
	}
	if out := testutil.RunGit(t, p, "rev-parse", "--verify", "--quiet", "main"); out == "" {
		t.Error("the clone that stayed has no main branch: it is no good for mounting")
	}
}
