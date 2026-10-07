package sim

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"prdash/internal/forge/model"
	"prdash/internal/gitcmd"
)

// It fails by SUBCOMMAND, not by argument: breaking clone too would test the clone error where the branch error belongs.
func gitFailingAt(t *testing.T, sub string) *gitcmd.Runner {
	t.Helper()
	dir := t.TempDir()
	script := filepath.Join(dir, "git")

	// The message goes through stderr because that is where gitcmd.Runner reads it.
	content := "#!/bin/sh\n" +
		"for arg in \"$@\"; do\n" +
		"  if [ \"$arg\" = \"" + sub + "\" ]; then\n" +
		"    printf '%s\\n' 'fatal: the subcommand " + sub + " did not complete' >&2\n" +
		"    exit 1\n" +
		"  fi\n" +
		"done\n" +
		"exec git \"$@\"\n"
	if err := os.WriteFile(script, []byte(content), 0o755); err != nil {
		t.Fatal(err)
	}
	return &gitcmd.Runner{Bin: script, Timeout: gitcmd.DefaultTimeout}
}

func serviceWithBadGit(t *testing.T, repo, branch, sub string) *Service {
	t.Helper()
	loc := fakeLocator{ok: true, place: Place{Repo: repo, Branch: branch}}
	s := newService(t, loc, fakeSim(t, writeJPEG(t)))
	s.Git = gitFailingAt(t, sub)
	return s
}

// Each names its own subcommand and none may name another's: the value is in the comparison.
func TestThreeStagingGitFailuresAreWarnedWithTheirSubcommandAndReason(t *testing.T) {
	// Not the loose word "clone" but the full invocation: "clone" collides with "the simulation clone".
	for _, c := range []struct {
		name   string
		sub    string
		want   []string
		absent []string
	}{
		{
			name:   "the clone fails",
			sub:    "clone",
			want:   []string{"clone --quiet --shared", "clone the local refs of"},
			absent: []string{"branch --quiet", "checkout --quiet"},
		},
		{
			name: "the item's branch cannot be materialized",
			sub:  "branch",
			// The item's branch name, not the base's: materialize goes first with it.
			want:   []string{"branch --quiet main-origin origin/main-origin", "create the branch main-origin"},
			absent: []string{"clone --quiet", "checkout --quiet"},
		},
		{
			name:   "the active branch's checkout fails",
			sub:    "checkout",
			want:   []string{"checkout --quiet main", "check out main in the simulation clone"},
			absent: []string{"clone --quiet", "branch --quiet"},
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			repo, _ := simRepoMount(t)
			it := model.NewItem(model.RepoRef{Forge: "github", Host: "github.com", Project: "acme/widget"}, 7)
			it.TargetBranch = "main"
			s := serviceWithBadGit(t, repo, "main-origin", c.sub)

			res, err := s.Simulate(context.Background(), it, KindMerge)
			if err == nil {
				t.Fatalf("Simulate with %s broken gave nil and an image at %s: a graph that "+
					"could not be prepared would be published", c.sub, res.Path)
			}
			msg := err.Error()
			for _, q := range c.want {
				if !strings.Contains(msg, q) {
					t.Errorf("the warning %q does not say %q: the wrap does not add the context", msg, q)
				}
			}
			for _, n := range c.absent {
				if strings.Contains(msg, n) {
					t.Errorf("the warning %q brings %q, which is another failure's invocation: the "+
						"user would go look at what did not fail", msg, n)
				}
			}
			// Git's reason is inside the wrap: what tells "could not complete" from a generic failure.
			if !strings.Contains(msg, "did not complete") {
				t.Errorf("the warning %q does not bring git's reason", msg)
			}
			if res.Path != "" {
				t.Errorf("with %s broken it returned the path %s", c.sub, res.Path)
			}
		})
	}
}

func TestCheckoutUsesTheBranchTheModeSaysAndTheMessageNamesIt(t *testing.T) {
	for _, c := range []struct {
		kind Kind
		base string
	}{
		{KindMerge, "main"},
		{KindRebase, "main-origin"},
	} {
		repo, _ := simRepoMount(t)
		it := model.NewItem(model.RepoRef{Forge: "github", Host: "github.com", Project: "acme/widget"}, 7)
		it.TargetBranch = "main"
		s := serviceWithBadGit(t, repo, "main-origin", "checkout")

		_, err := s.Simulate(context.Background(), it, c.kind)
		if err == nil {
			t.Fatalf("%s with the checkout broken gave nil", c.kind)
		}
		if !strings.Contains(err.Error(), "check out "+c.base) {
			t.Errorf("%s: the warning %q does not name the active branch (%s)", c.kind, err, c.base)
		}
	}
}

// Called directly, not through Simulate: Simulate asks for the cache dir AFTER cloning and materialising.
func TestKeepWithoutCacheDirComplainsInsteadOfGuessingWhereToSave(t *testing.T) {
	t.Setenv("HOME", "")
	t.Setenv("XDG_CACHE_HOME", "")

	s := &Service{}
	it := model.NewItem(model.RepoRef{Forge: "github", Host: "github.com", Project: "acme/widget"}, 7)

	dst, err := s.keep("no-existe.jpg", it, KindMerge)
	if err == nil {
		t.Fatalf("keep without a cache gave nil and the path %q: someone would save the image "+
			"in a place that is not theirs", dst)
	}
	if dst != "" {
		t.Errorf("keep returned the path %q along with the error: the caller would warn about a "+
			"save that never happened", dst)
	}
	// The wrap says what was being looked for, so the warning is not someone else's `exec: ...`.
	if !strings.Contains(err.Error(), "locate the simulation cache") {
		t.Errorf("the warning %q does not say it is the simulation cache", err)
	}
	// UserCacheDir's cause stays inside: "no cache" and "unreadable cache" are different.
	if !strings.Contains(err.Error(), "HOME") {
		t.Errorf("the warning %q does not bring os.UserCacheDir's cause, which is what names the "+
			"missing variable", err)
	}

	s.CacheDir = t.TempDir()
	if _, err := s.keep("no-existe.jpg", it, KindMerge); err == nil ||
		!strings.Contains(err.Error(), "keep the simulation image") {
		t.Errorf("with CacheDir set the warning %q is the cache's one and not the missing image's",
			err)
	}
}
