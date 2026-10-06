package sim

import (
	"context"
	"image"
	"image/color"
	"image/jpeg"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"prdash/internal/forge/model"
	"prdash/internal/testutil"
)

type stubLocator struct {
	place Place
	ok    bool
}

func (l stubLocator) Locate(model.Item) (Place, bool) { return l.place, l.ok }

func fixture(t *testing.T) (repo, review string, it model.Item) {
	t.Helper()
	dir := t.TempDir()

	src := filepath.Join(dir, "src")
	testutil.InitRepo(t, src)
	testutil.CommitFile(t, src, "f.txt", "base", "base")
	testutil.RunGit(t, src, "branch", "-M", "main")
	testutil.RunGit(t, src, "checkout", "-b", "prdash/pr-7")
	testutil.CommitFile(t, src, "f.txt", "pr", "pr")

	repo = filepath.Join(dir, "remote.git")
	testutil.InitBare(t, repo)
	testutil.RunGit(t, src, "remote", "add", "origin", repo)
	testutil.Push(t, src, "origin", "main", "prdash/pr-7")

	review = filepath.Join(dir, "review")
	testutil.RunGit(t, src, "checkout", "--quiet", "main")
	testutil.RunGit(t, src, "worktree", "add", "--quiet", review, "prdash/pr-7")

	it = model.NewItem(model.RepoRef{Forge: "github", Host: "github.com", Project: "acme/widget"}, 7)
	it.SourceBranch = "feat/x"
	it.TargetBranch = "main"
	return repo, review, it
}

func fakeSim(t *testing.T, srcJPEG string) string {
	t.Helper()
	dir := t.TempDir()
	script := "#!/bin/sh\n" +
		"media=\"\"\n" +
		"while [ $# -gt 0 ]; do\n" +
		"  if [ \"$1\" = \"--media-dir\" ]; then media=\"$2\"; fi\n" +
		"  shift\n" +
		"done\n" +
		"mkdir -p \"$media/images\"\n" +
		"cp " + srcJPEG + " \"$media/images/out.jpg\"\n" +
		"echo \"$media/images/out.jpg\"\n"
	return writeScript(t, dir, "git-sim", script)
}

func writeJPEG(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "src.jpg")
	img := image.NewRGBA(image.Rect(0, 0, 8, 8))
	for y := range 8 {
		for x := range 8 {
			img.SetRGBA(x, y, color.RGBA{R: uint8(x * 30), G: uint8(y * 30), B: 200, A: 255})
		}
	}
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	if err := jpeg.Encode(f, img, nil); err != nil {
		t.Fatal(err)
	}
	return path
}

func newService(t *testing.T, loc Locator, bin string) *Service {
	t.Helper()
	svc := New(loc)
	svc.Runner = &Runner{Bin: bin}
	svc.CacheDir = filepath.Join(t.TempDir(), "cache")
	return svc
}

func TestSimulateLeavesNoTraceInTheLocalRepo(t *testing.T) {
	repo, review, it := fixture(t)
	before := testutil.RunGit(t, repo, "branch", "--format=%(refname)")
	svc := newService(t, stubLocator{place: Place{Repo: repo, Branch: "prdash/pr-7"}, ok: true}, fakeSim(t, writeJPEG(t)))

	res, err := svc.Simulate(context.Background(), it, KindMerge)
	if err != nil {
		t.Fatalf("Simulate: %v", err)
	}
	if res.Kind != KindMerge || res.Ref != "prdash/pr-7" || res.Base != "main" {
		t.Errorf("Result = %+v", res)
	}
	if _, err := os.Stat(res.Path); err != nil {
		t.Errorf("the image was not kept at %s: %v", res.Path, err)
	}

	if after := testutil.RunGit(t, repo, "branch", "--format=%(refname)"); after != before {
		t.Errorf("the repo gained or lost branches:\n%s\nwant\n%s", after, before)
	}
	if n := strings.Count(testutil.RunGit(t, repo, "worktree", "list"), "\n"); n != 0 {
		t.Errorf("%d worktrees are left in the repo", n)
	}
	if got := strings.TrimSpace(testutil.RunGit(t, review, "rev-parse", "--abbrev-ref", "HEAD")); got != "prdash/pr-7" {
		t.Errorf("the review's worktree ended up at %q", got)
	}
	if out := testutil.RunGit(t, review, "status", "--porcelain"); strings.TrimSpace(out) != "" {
		t.Errorf("the render left uncommitted changes in the review's worktree:\n%s", out)
	}
}

func TestRebaseRunsFromTheItemBranch(t *testing.T) {
	repo, _, it := fixture(t)
	dir := t.TempDir()
	bin := writeScript(t, dir, "git-sim", "#!/bin/sh\n"+
		"echo \"$PWD|$*|$(git rev-parse --abbrev-ref HEAD)\" > "+filepath.Join(dir, "cwd")+"\n"+
		"media=\"\"\n"+
		"while [ $# -gt 0 ]; do if [ \"$1\" = \"--media-dir\" ]; then media=\"$2\"; fi; shift; done\n"+
		"mkdir -p \"$media/images\" && cp "+writeJPEG(t)+" \"$media/images/out.jpg\" && echo \"$media/images/out.jpg\"\n")

	svc := newService(t, stubLocator{place: Place{Repo: repo, Branch: "prdash/pr-7"}, ok: true}, bin)
	res, err := svc.Simulate(context.Background(), it, KindRebase)
	if err != nil {
		t.Fatalf("Simulate: %v", err)
	}
	if res.Ref != "main" {
		t.Errorf("Ref = %q, want main: the rebase is done against the base", res.Ref)
	}

	raw, err := os.ReadFile(filepath.Join(dir, "cwd"))
	if err != nil {
		t.Fatal(err)
	}
	cwd, rest, _ := strings.Cut(strings.TrimSpace(string(raw)), "|")
	if cwd == repo {
		t.Fatal("the render ran in the source repo instead of the temporary clone")
	}
	args, head, _ := strings.Cut(rest, "|")
	if !strings.HasSuffix(args, " rebase main") {
		t.Errorf("argv = %q, want a rebase against main", args)
	}
	// The active branch has to be the item's: that is what is rebased from.
	if head != "prdash/pr-7" {
		t.Errorf("HEAD = %q, want prdash/pr-7", head)
	}
}

func TestSimulateNeedsAMountedReview(t *testing.T) {
	_, _, it := fixture(t)
	svc := newService(t, stubLocator{}, fakeSim(t, writeJPEG(t)))

	_, err := svc.Simulate(context.Background(), it, KindMerge)
	if err == nil || !strings.Contains(err.Error(), "mounted") {
		t.Fatalf("err = %v, want a warning that the review is not mounted", err)
	}
}

func TestSimulateNeedsATargetBranch(t *testing.T) {
	repo, _, it := fixture(t)
	it.TargetBranch = ""
	svc := newService(t, stubLocator{place: Place{Repo: repo, Branch: "prdash/pr-7"}, ok: true}, fakeSim(t, writeJPEG(t)))

	if _, err := svc.Simulate(context.Background(), it, KindMerge); err == nil {
		t.Fatal("Simulate accepted an item with no base branch")
	}
}

// A base that is not in the clone cannot be compared, and saying so is the point.
func TestSimulateFailsWhenTheBaseIsNotInTheClone(t *testing.T) {
	repo, _, it := fixture(t)
	it.TargetBranch = "release/9"
	svc := newService(t, stubLocator{place: Place{Repo: repo, Branch: "prdash/pr-7"}, ok: true}, fakeSim(t, writeJPEG(t)))

	_, err := svc.Simulate(context.Background(), it, KindMerge)
	if err == nil || !strings.Contains(err.Error(), "release/9") {
		t.Fatalf("err = %v, want it to name the base it cannot find", err)
	}
}

// A clone with the base only as a remote ref is still usable.
func TestMaterializeFallsBackToTheRemoteRef(t *testing.T) {
	repo, _, it := fixture(t)
	clone := filepath.Join(t.TempDir(), "clone")
	testutil.RunGit(t, t.TempDir(), "clone", "--quiet", repo, clone)
	testutil.RunGit(t, clone, "update-ref", "-d", "refs/heads/main")

	svc := newService(t, stubLocator{place: Place{Repo: clone}, ok: true}, fakeSim(t, writeJPEG(t)))
	if err := svc.materialize(context.Background(), clone, it.TargetBranch); err != nil {
		t.Fatalf("materialize: %v", err)
	}
	if !testutil.RefExists(t, clone, "refs/heads/main") {
		t.Error("materialize did not create the local branch from the remote ref")
	}
}

func TestMaterializePrefersTheLocalBranch(t *testing.T) {
	repo, _, it := fixture(t)
	clone := filepath.Join(t.TempDir(), "clone")
	testutil.RunGit(t, t.TempDir(), "clone", "--quiet", repo, clone)
	testutil.RunGit(t, clone, "checkout", "--quiet", "main")
	testutil.CommitFile(t, clone, "local-only.txt", "x", "local progress")

	localSHA := strings.TrimSpace(testutil.RunGit(t, clone, "rev-parse", "main"))
	remoteSHA := strings.TrimSpace(testutil.RunGit(t, repo, "rev-parse", "main"))
	if localSHA == remoteSHA {
		t.Skip("the clone and the remote point at the same commit; there is no divergence to test")
	}

	svc := newService(t, stubLocator{place: Place{Repo: clone, Branch: "prdash/pr-7"}, ok: true}, fakeSim(t, writeJPEG(t)))
	if err := svc.materialize(context.Background(), clone, it.TargetBranch); err != nil {
		t.Fatalf("materialize: %v", err)
	}
	if got := strings.TrimSpace(testutil.RunGit(t, clone, "rev-parse", "main")); got != localSHA {
		t.Errorf("main = %s, want the local %s (the remote is %s)", got, localSHA, remoteSHA)
	}
}

func TestPruneKeepsTheNewest(t *testing.T) {
	dir := t.TempDir()
	for i, name := range []string{"a.jpg", "b.jpg", "c.jpg"} {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
		stamp := timeAt(int64(i))
		_ = os.Chtimes(path, stamp, stamp)
	}
	if err := os.WriteFile(filepath.Join(dir, "notimage.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	prune(dir, 2)

	if _, err := os.Stat(filepath.Join(dir, "a.jpg")); !os.IsNotExist(err) {
		t.Error("it did not prune the oldest image")
	}
	if _, err := os.Stat(filepath.Join(dir, "c.jpg")); err != nil {
		t.Errorf("it pruned a recent image: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "notimage.txt")); err != nil {
		t.Errorf("it touched a file that is not an image: %v", err)
	}
}

func timeAt(offset int64) time.Time {
	return time.Unix(1_700_000_000+offset*60, 0)
}
