package sim

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func gitSimFailing(t *testing.T, message string) string {
	t.Helper()
	return writeScript(t, t.TempDir(), "git-sim",
		"#!/bin/sh\necho '"+message+"' >&2\nexit 1\n")
}

func TestWhenGitSimFailsTheErrorComesFromItNotFromStageNorCache(t *testing.T) {
	repo, _, it := fixture(t)
	it.SourceBranch = "prdash/pr-7"
	bin := gitSimFailing(t, "IndexError: no such branch")

	s := newService(t, fakeLocator{ok: true, place: Place{Repo: repo, Branch: "prdash/pr-7"}}, bin)

	res, err := s.Simulate(context.Background(), it, KindMerge)
	if err == nil {
		t.Fatal("a failing git-sim gave nil: the popup would close without saying why")
	}
	if !strings.Contains(err.Error(), "no such branch") {
		t.Errorf("the error %q does not bring git-sim's message", err)
	}
	// And there is no image: a Result with Path set would be opened in the viewer.
	if res.Path != "" {
		t.Errorf("it returned an image %q despite the render's failure", res.Path)
	}
	// A cache message in a clone failure sends you the wrong way: that guard comes later.
	if strings.Contains(err.Error(), "cache") {
		t.Errorf("the error %q talks about the cache, which is a later stage", err)
	}
}

func TestIfRenderLeavesNoImageTheErrorSaysSoAndTheEmptyPathIsNotPropagated(t *testing.T) {
	repo, _, it := fixture(t)
	it.SourceBranch = "prdash/pr-7"
	mute := writeScript(t, t.TempDir(), "git-sim", "#!/bin/sh\nexit 0\n")

	s := newService(t, fakeLocator{ok: true, place: Place{Repo: repo, Branch: "prdash/pr-7"}}, mute)

	res, err := s.Simulate(context.Background(), it, KindMerge)
	if err == nil {
		t.Fatal("a git-sim that produces no image gave nil")
	}
	if !strings.Contains(err.Error(), "no image") {
		t.Errorf("the error %q does not say git-sim produced no image", err)
	}
	if res.Path != "" {
		t.Errorf("it returned Path=%q: an empty file would be copied to the cache", res.Path)
	}
	entries, err := os.ReadDir(s.CacheDir)
	if err == nil {
		for _, e := range entries {
			t.Errorf("%s was left in the cache despite the failure", e.Name())
		}
	}
}

func TestSimulationTempIsDeletedEvenIfTheRenderFails(t *testing.T) {
	repo, _, it := fixture(t)
	it.SourceBranch = "prdash/pr-7"
	bin := gitSimFailing(t, "boom")

	tempDir := t.TempDir()
	t.Setenv("TMPDIR", tempDir)

	s := newService(t, fakeLocator{ok: true, place: Place{Repo: repo, Branch: "prdash/pr-7"}}, bin)
	if _, err := s.Simulate(context.Background(), it, KindMerge); err == nil {
		t.Fatal("the render should have failed")
	}

	entries, err := os.ReadDir(tempDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		var names []string
		for _, e := range entries {
			names = append(names, e.Name())
		}
		t.Errorf("the simulation's temp had %d entries: %v. The repo's clone "+
			"stays on disk for every failed render", len(entries), names)
	}
}

func TestUncreatableCacheFailsAfterRenderingAndSaysSo(t *testing.T) {
	repo, _, it := fixture(t)
	it.SourceBranch = "prdash/pr-7"

	cache := filepath.Join(t.TempDir(), "cache-bloqueado")
	if err := os.WriteFile(cache, []byte("blocker"), 0o644); err != nil {
		t.Fatal(err)
	}
	s := newService(t, fakeLocator{ok: true, place: Place{Repo: repo, Branch: "prdash/pr-7"}},
		fakeSim(t, writeJPEG(t)))
	s.CacheDir = cache

	res, err := s.Simulate(context.Background(), it, KindMerge)
	if err == nil {
		t.Fatal("a cache that cannot be created gave nil")
	}
	if !strings.Contains(err.Error(), "simulation cache") {
		t.Errorf("the error %q does not say the cache failed, which is where the fault is", err)
	}
	// The message does NOT say git-sim failed, because it did not.
	if strings.Contains(err.Error(), "git-sim") {
		t.Errorf("the error %q blames git-sim, which did render", err)
	}
	if res.Path != "" {
		t.Errorf("it returned Path=%q without having saved anything", res.Path)
	}
}

func TestReviewBranchIsActivatedForRebaseAndBaseForMerge(t *testing.T) {
	repo, _ := simRepoMount(t)
	source, tmp := simRepoMount(t)
	_ = repo
	for _, c := range []struct {
		name       string
		kind       Kind
		base       string
		itemBranch string
	}{
		{"merge with a missing base", KindMerge, "base/that-does-not-exist", "feat/x"},
		{"rebase with a missing base", KindRebase, "base/that-does-not-exist", "feat/x"},
	} {
		_, _, err := New(locatorForStage()).stage(context.Background(),
			Place{Repo: source, Branch: c.itemBranch}, c.kind, c.base, t.TempDir())
		if err == nil {
			t.Errorf("%s: it passed without the base", c.name)
			continue
		}
		if !strings.Contains(err.Error(), c.base) {
			t.Errorf("%s: the error %q does not name the base that could not be activated",
				c.name, err)
		}
	}
	_ = tmp
}

// The case that uncovered the leak.
func TestCopyFileLeavesNoTempIfTheDestinationIsAnExistingDirectory(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "origen.jpg")
	if err := os.WriteFile(src, []byte("image"), 0o644); err != nil {
		t.Fatal(err)
	}
	dst := filepath.Join(dir, "destino.jpg")
	if err := os.MkdirAll(dst, 0o755); err != nil {
		t.Fatal(err)
	}

	if err := copyFile(src, dst); err == nil {
		t.Fatal("copying to a directory gave nil")
	}
	if _, err := os.Stat(dst + ".part"); !os.IsNotExist(err) {
		t.Errorf("the temp %s.part was left: every failed attempt would leave trash that prune "+
			"does not delete because it only looks at the .jpg", dst+".part")
	}
	info, err := os.Stat(dst)
	if err != nil || !info.IsDir() {
		t.Errorf("the destination stopped being a directory: %v", err)
	}
	// The real prune does not touch the `.part`: that is what makes a leftover harmless.
	leftovers := filepath.Join(dir, "other.part")
	if err := os.WriteFile(leftovers, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	prune(dir, 0)
	if _, err := os.Stat(leftovers); err != nil {
		t.Errorf("prune removed the .part, so copyFile's cleanup is not needed: %v", err)
	}
}

func TestPruneWalkToleratesAnEntryThatCannotBeStatted(t *testing.T) {
	dir := t.TempDir()
	dangling := filepath.Join(dir, "colgado.jpg")
	if err := os.Symlink(filepath.Join(dir, "no-existe.jpg"), dangling); err != nil {
		t.Fatal(err)
	}
	// A real image that must NOT be lost: a prune using directory order would take it.
	good := filepath.Join(dir, "buena.jpg")
	if err := os.WriteFile(good, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	prune(dir, 0)

	// The broken link stays: leaving it makes the state visible.
	if _, err := os.Stat(good); err == nil {
		t.Error("the good image is still there: prune's walk stopped at the broken link")
	}
}
