package sim

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"prdash/internal/forge/model"
)

// A file where a directory was expected denies a write without needing permissions.
func blockWithAFile(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("blocker"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestWithoutCacheDirSimulationIsImpossibleAndTheErrorSaysSo(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", "")
	t.Setenv("HOME", "")

	if _, err := DefaultCacheDir(); err == nil {
		t.Fatal("DefaultCacheDir without HOME nor XDG_CACHE_HOME gave nil")
	}

	s := &Service{}
	if _, err := s.cacheDir(); err == nil {
		t.Error("cacheDir without CacheDir nor environment gave nil")
	}

	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	dir, err := DefaultCacheDir()
	if err != nil {
		t.Fatalf("DefaultCacheDir with environment: %v", err)
	}
	if filepath.Base(dir) != "sim" {
		t.Errorf("DefaultCacheDir gave %q, want .../prdash/sim", dir)
	}
}

// The one that fires in a container.
func TestWithoutTMPDIRTheSimulationDirectoryIsNotPrepared(t *testing.T) {
	blockWithAFile(t, filepath.Join(t.TempDir(), "tmp-bloqueado"))
	t.Setenv("TMPDIR", filepath.Join(t.TempDir(), "tmp-bloqueado"))

	repo, review, it := fixture(t)
	it.SourceBranch = "prdash/pr-7"
	s := newService(t, fakeLocator{ok: true, place: Place{Repo: repo, Branch: "prdash/pr-7"}},
		fakeSim(t, writeJPEG(t)))

	_, err := s.Simulate(context.Background(), it, KindMerge)
	if err == nil {
		t.Fatal("without a usable TMPDIR the simulation ran anyway")
	}
	if !strings.Contains(err.Error(), "simulation directory") {
		t.Errorf("the error %q does not say the simulation directory failed: there are three "+
			"temps on the way and knowing which one tells whether to wait", err)
	}
	_ = review
}

func TestRepoThatNoLongerExistsFailsOnCloneNotCheckout(t *testing.T) {
	// The repo Locator gives does not exist, which is stronger than deleting it afterwards.
	s := newService(t, fakeLocator{ok: true, place: Place{
		Repo: filepath.Join(t.TempDir(), "repo-that-does-not-exist"), Branch: "prdash/pr-7",
	}}, fakeSim(t, writeJPEG(t)))

	_, _, err := s.stage(context.Background(), s.locatorPlace(), KindMerge, "main", t.TempDir())
	if err == nil {
		t.Fatal("a repo that does not exist got cloned")
	}
	if !strings.Contains(err.Error(), "clone") {
		t.Errorf("the error %q does not say the clone failed", err)
	}
	if !strings.Contains(err.Error(), "repo-that-does-not-exist") {
		t.Errorf("the error %q does not name the repo that could not be cloned", err)
	}
}

func TestBranchMissingFromTheCloneFailsOnMaterialize(t *testing.T) {
	repo, _ := simRepoMount(t)

	for _, c := range []struct {
		name       string
		base       string
		itemBranch string
		missing    string
	}{
		// In both cases ONE of the two is missing: the error has to name the one that failed.
		{"the item's branch is missing", "main", "feat/inexistente", "feat/inexistente"},
		{"the base is missing", "base/inexistente", "feat/x", "base/inexistente"},
	} {
		_, _, err := New(locatorForStage()).stage(context.Background(),
			Place{Repo: repo, Branch: c.itemBranch}, KindMerge, c.base, t.TempDir())
		if err == nil {
			t.Errorf("%s: it passed without the branch", c.name)
			continue
		}
		if !strings.Contains(err.Error(), c.missing) {
			t.Errorf("%s: the error %q does not name the missing ref (%q)",
				c.name, err, c.missing)
		}
	}

	// The good case, or "always fails" would prove nothing about which guard fired.
	if _, _, err := New(locatorForStage()).stage(context.Background(),
		Place{Repo: repo, Branch: "feat/x"}, KindMerge, "main", t.TempDir()); err != nil {
		t.Errorf("a branch that does exist gave an error: %v", err)
	}
}

func TestKeepPropagatesCacheCreationFailureAndLeavesNothing(t *testing.T) {
	cache := filepath.Join(t.TempDir(), "cache-bloqueado")
	blockWithAFile(t, cache)

	s := &Service{CacheDir: cache}
	_, err := s.keep(writeJPEG(t), itemWithBranch("prdash/pr-7"), KindMerge)
	if err == nil {
		t.Fatal("a cache that cannot be created gave nil")
	}
	if !strings.Contains(err.Error(), "simulation cache") {
		t.Errorf("the error %q does not say the cache failed", err)
	}
	info, statErr := os.Stat(cache)
	if statErr == nil && info.IsDir() {
		t.Error("the cache was created halfway: the next attempt would find it existing")
	}
}

// A failure that slips through easily.
func TestGitSimThatProducesNoImageSaysSo(t *testing.T) {
	mute := writeScript(t, t.TempDir(), "git-sim", "#!/bin/sh\nexit 0\n")

	r := &Runner{Bin: mute}
	_, err := r.Render(context.Background(), t.TempDir(), t.TempDir(), Spec{Kind: KindMerge, Ref: "main"})
	if err == nil {
		t.Fatal("a git-sim that produces no image gave nil")
	}
	if !strings.Contains(err.Error(), "no image") {
		t.Errorf("the error %q does not say git-sim produced no image", err)
	}
	// The binary's name is in there: an error without the command that caused it sends you looking.
	if !strings.Contains(err.Error(), "git-sim") {
		t.Errorf("the error %q does not name git-sim", err)
	}
}

func TestSimulatePropagatesStageFailureAndSaysWhereItCameFrom(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "repo-that-does-not-exist")
	s := newService(t, fakeLocator{ok: true, place: Place{Repo: missing, Branch: "prdash/pr-7"}},
		fakeSim(t, writeJPEG(t)))

	res, err := s.Simulate(context.Background(), itemWithBranch("prdash/pr-7"), KindMerge)
	if err == nil {
		t.Fatal("Simulate with a missing repo gave nil")
	}
	// And the empty result: a Result with Path set and a missing image would be opened.
	if res.Path != "" {
		t.Errorf("it returned an image %q despite the failure", res.Path)
	}
	if !strings.Contains(err.Error(), missing) {
		t.Errorf("the error %q does not name the repo that failed", err)
	}
}

func (s *Service) locatorPlace() Place {
	if l, ok := s.Locator.(fakeLocator); ok {
		return l.place
	}
	return Place{}
}

func itemWithBranch(branch string) model.Item {
	it := model.NewItem(model.RepoRef{
		Forge: "github", Host: "github.com", Project: "acme/widget",
	}, 7)
	it.SourceBranch = branch
	it.TargetBranch = "main"
	return it
}
