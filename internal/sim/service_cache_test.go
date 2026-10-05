package sim

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"prdash/internal/forge/model"
	"prdash/internal/testutil"
)

// keep and prune are the two that touch the user's disk, and they were the two untested.

// The ORDER of the timestamps is checked, not just how many files are left: a count alone would pass
// with the newest pruned and the oldest kept.
func TestPruneKeepsTheRecentAndDeletesTheOld(t *testing.T) {
	dir := t.TempDir()
	base := time.Now().Add(-time.Hour)

	names := []string{"a.jpg", "b.jpg", "c.jpg", "d.jpg", "e.jpg"}
	for i, n := range names {
		path := filepath.Join(dir, n)
		if err := os.WriteFile(path, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
		mod := base.Add(time.Duration(i) * time.Minute)
		if err := os.Chtimes(path, mod, mod); err != nil {
			t.Fatal(err)
		}
	}

	prune(dir, 2)

	for _, n := range names {
		_, err := os.Stat(filepath.Join(dir, n))
		survives := err == nil
		wanted := n == "d.jpg" || n == "e.jpg"
		if survives != wanted {
			if survives {
				t.Errorf("%s survives, but it had to be deleted for being older", n)
			} else {
				t.Errorf("%s was deleted, but it had to survive for being more recent", n)
			}
		}
	}
	alive := 0
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".jpg") {
			alive++
		}
	}
	if alive != 2 {
		t.Errorf("%d images are left, want 2", alive)
	}
}

// The cache lives under the user's home and prune deletes by directory, not by list.
func TestPruneIgnoresWhatAreNotImagesAndTheDirectories(t *testing.T) {
	dir := t.TempDir()

	for _, n := range []string{"vieja.jpg", "nueva.jpg"} {
		path := filepath.Join(dir, n)
		if err := os.WriteFile(path, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	extras := map[string]string{
		"a-medias.jpg.part": "copyFile's temp",
		"nota.txt":          "not an image",
		"sin-extension":     "has no extension",
	}
	for n := range extras {
		if err := os.WriteFile(filepath.Join(dir, n), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	sub := filepath.Join(dir, "sub.jpg") // directory with an image name
	if err := os.Mkdir(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-time.Hour)
	mod := time.Now()
	if err := os.Chtimes(filepath.Join(dir, "vieja.jpg"), old, old); err != nil {
		t.Fatal(err)
	}
	for _, n := range append(keys(extras), "sub.jpg") {
		p := filepath.Join(dir, n)
		if err := os.Chtimes(p, old, old); err != nil {
			t.Fatal(err)
		}
	}
	_ = mod

	prune(dir, 1)

	if _, err := os.Stat(filepath.Join(dir, "nueva.jpg")); err != nil {
		t.Error("the most recent image did not survive the prune")
	}
	if _, err := os.Stat(filepath.Join(dir, "vieja.jpg")); err == nil {
		t.Error("the oldest image survived the prune")
	}
	for n := range extras {
		if _, err := os.Stat(filepath.Join(dir, n)); err != nil {
			t.Errorf("%s (%s) did not survive the prune", n, extras[n])
		}
	}
	if _, err := os.Stat(sub); err != nil {
		t.Error("the prune ate a directory with an image name")
	}
}

func TestPruneWithLessThanTheLimitTouchesNothing(t *testing.T) {
	empty := t.TempDir()
	prune(empty, 5)
	entries, err := os.ReadDir(empty)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Errorf("pruning an empty directory gave %d entries", len(entries))
	}

	one := t.TempDir()
	path := filepath.Join(one, "a.jpg")
	if err := os.WriteFile(path, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	prune(one, 5)
	if _, err := os.Stat(path); err != nil {
		t.Error("an image below the limit was deleted")
	}

	prune(filepath.Join(t.TempDir(), "no-existe"), 5)
}

// The name is the deduplication key: forge, project, number, kind and a UnixNano.
func TestKeepCopiesTheImageAndPrunesAfterwards(t *testing.T) {
	dir := t.TempDir()
	source := filepath.Join(dir, "render.jpg")
	if err := os.WriteFile(source, []byte("content"), 0o644); err != nil {
		t.Fatal(err)
	}

	s := &Service{CacheDir: filepath.Join(dir, "cache")}
	it := model.Item{Forge: "github", Number: 42, Ref: model.RepoRef{Project: "grupo/proyecto"}}

	dst, err := s.keep(source, it, KindMerge)
	if err != nil {
		t.Fatalf("keep: %v", err)
	}

	content, err := os.ReadFile(dst)
	if err != nil {
		t.Fatalf("the saved image is not there: %v", err)
	}
	if string(content) != "content" {
		t.Errorf("the copy has %q", content)
	}
	if _, err := os.Stat(dst + ".part"); err == nil {
		t.Error("the copy's temp is still in the cache")
	}
	name := filepath.Base(dst)
	if !strings.HasPrefix(name, "github-") || !strings.Contains(name, "proyecto") {
		t.Errorf("the name %q carries neither forge nor project", name)
	}
	if !strings.Contains(name, "-42-") {
		t.Errorf("the name %q does not carry the number", name)
	}
	if !strings.HasSuffix(name, ".jpg") {
		t.Errorf("the name %q does not end in .jpg", name)
	}
	if _, err := os.Stat(s.CacheDir); err != nil {
		t.Errorf("keep did not create the cache directory: %v", err)
	}

	if _, err := os.Stat(source); err != nil {
		t.Error("keep deleted the original, which the viewer needs")
	}
}

// Both must surface as errors and not as an empty dst: an empty dst with the error ignored would
// have the popup open nothing.
func TestKeepFailsWithMissingSourceAndImpossibleDestination(t *testing.T) {
	dir := t.TempDir()
	it := model.Item{Forge: "github", Number: 1, Ref: model.RepoRef{Project: "p"}}

	s := &Service{CacheDir: filepath.Join(dir, "c1")}
	dst, err := s.keep(filepath.Join(dir, "no-existe.jpg"), it, KindMerge)
	if err == nil {
		t.Fatal("keep with a missing source gave nil")
	}
	if dst != "" {
		t.Errorf("keep returned %q with an error", dst)
	}

	block := filepath.Join(dir, "bloque")
	if err := os.WriteFile(block, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	s = &Service{CacheDir: block}
	if _, err := s.keep(testSource(t), it, KindMerge); err == nil {
		t.Error("keep with a CacheDir that is a file gave nil")
	}
}

// The memo matters: cacheDir stores the result so os.UserCacheDir is not asked on every image.
func TestCacheDirFallsBackToDefaultAndIsMemoized(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", dir)

	path, err := DefaultCacheDir()
	if err != nil {
		t.Fatalf("DefaultCacheDir: %v", err)
	}
	if filepath.Dir(path) != filepath.Join(dir, "prdash") || filepath.Base(path) != "sim" {
		t.Errorf("DefaultCacheDir gave %q", path)
	}

	s := &Service{}
	obtained, err := s.cacheDir()
	if err != nil {
		t.Fatalf("cacheDir: %v", err)
	}
	if obtained != path {
		t.Errorf("cacheDir gave %q, want the default %q", obtained, path)
	}
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	if other, _ := s.cacheDir(); other != path {
		t.Errorf("the second call gave %q, want the memoized %q", other, path)
	}

	explicit := filepath.Join(dir, "otro")
	withField := &Service{CacheDir: explicit}
	if g, _ := withField.cacheDir(); g != explicit {
		t.Errorf("with CacheDir set it gave %q, want %q", g, explicit)
	}
	if withField.CacheDir != explicit {
		t.Errorf("cacheDir altered the CacheDir: %q", withField.CacheDir)
	}
}

// The order matters: without a Locator there is nowhere to look, and with a Locator but no clone
// there is nothing to take refs from.
func TestSimulateRefusesWithoutLocatorAndWithoutClone(t *testing.T) {
	it := model.Item{Forge: "github", Number: 1, Ref: model.RepoRef{Project: "p"}}

	s := &Service{}
	if _, err := s.Simulate(context.Background(), it, KindMerge); err == nil {
		t.Error("without a Locator it gave nil")
	} else if !strings.Contains(err.Error(), "local repository") {
		t.Errorf("the error %q does not say the local repo is missing", err)
	}

	s = &Service{Locator: fakeLocator{ok: false}}
	if _, err := s.Simulate(context.Background(), it, KindMerge); err == nil {
		t.Error("with a locator that finds nothing it gave nil")
	} else if !strings.Contains(err.Error(), "mounted") {
		t.Errorf("the error %q does not say the review is not mounted", err)
	}

	s = &Service{Locator: fakeLocator{ok: true, place: Place{}}}
	if _, err := s.Simulate(context.Background(), it, KindMerge); err == nil {
		t.Error("with a Place without Repo it gave nil")
	}

	s = &Service{Locator: fakeLocator{ok: true, place: Place{Repo: testDir(t), Branch: "b"}}}
	_, err := s.Simulate(context.Background(), it, KindMerge)
	if err == nil {
		t.Error("with a repo present it gave nil without getting to render")
	}
}

// A Service with Runner nil recovers by itself, which happens when someone composes one by hand.
func TestAvailableAndRunnerDoNotBreakWithAnEmptyService(t *testing.T) {
	s := &Service{}
	_ = s.Available()
	if s.Runner == nil {
		t.Error("runner() did not fill in the Runner of a hand-built service")
	}
	fresh := New(fakeLocator{})
	if fresh == nil || fresh.Runner == nil || fresh.Git == nil {
		t.Errorf("New returned %+v with nil parts", fresh)
	}
	manual := &Service{Locator: fakeLocator{}}
	if manual.gitRunner() == nil || manual.Git == nil {
		t.Error("gitRunner() did not fill in the Git of a hand-built service")
	}
}

// The decision is whether the process complained or hung, which is what separates two completely
// different diagnoses.
func TestSimErrorMessageTellsComplainingFromHanging(t *testing.T) {
	got := message("first\nsecond\nthird", errors.New("exit status 1"))
	if !strings.Contains(got, "first") {
		t.Errorf("the message %q does not bring stderr's first line", got)
	}
	if strings.Contains(got, "second") {
		t.Errorf("the message %q brings more than one line", got)
	}

	got = message("   \n  ", errors.New("exit status 137"))
	if strings.TrimSpace(got) == "" {
		t.Error("without stderr the message ended up empty")
	}
	if !strings.Contains(got, "137") {
		t.Errorf("the message %q lost the exit code", got)
	}

	if got := message("", nil); strings.TrimSpace(got) == "" {
		t.Error("without stderr nor error the message ended up empty")
	}
}

// My first version assumed bin read an environment variable so a test could point it elsewhere:
// it does not, and the test now checks the field.
func TestSimBinaryIsTheFieldOrTheCanonicalOneAndNothingElse(t *testing.T) {
	if got := (&Runner{}).bin(); got != DefaultBin {
		t.Errorf("with Bin empty it gave %q, want %q", got, DefaultBin)
	}
	if DefaultBin != "git-sim" {
		t.Errorf("DefaultBin is %q, want git-sim", DefaultBin)
	}
	if got := (&Runner{Bin: "/opt/git-sim-mio"}).bin(); got != "/opt/git-sim-mio" {
		t.Errorf("with Bin set it gave %q", got)
	}
	t.Setenv("GIT_SIM_BIN", "/opt/otro")
	if got := (&Runner{}).bin(); got != DefaultBin {
		t.Errorf("with GIT_SIM_BIN in the environment it gave %q, want the canonical one", got)
	}
	if got := NewRunner().Bin; got != DefaultBin {
		t.Errorf("NewRunner().Bin = %q", got)
	}
	_ = (&Runner{}).Available()
}

// The path and arguments are what make the failure reproducible without guessing which invocation
// hit it.
func TestSimErrorBringsWhatWasExecuted(t *testing.T) {
	e := &Error{Args: []string{"config", "user.email"}, Dir: "/repos/proy", Msg: "no such repository", ExitCode: 128}
	msg := e.Error()
	for _, want := range []string{"config", "user.email", "/repos/proy", "no such repository", "128"} {
		if !strings.Contains(msg, want) {
			t.Errorf("the message %q does not bring %q", msg, want)
		}
	}
	if strings.Contains((&Error{Args: []string{"a"}, Msg: "m"}).Error(), "exit") {
		t.Error("an error without an exit code puts it in parentheses")
	}
	if e.Unwrap() != nil {
		_ = e // Unwrap without a cause returns nil; here it only checks it does not blow up
	}
}

type fakeLocator struct {
	ok    bool
	place Place
}

func (l fakeLocator) Locate(model.Item) (Place, bool) { return l.place, l.ok }

func testSource(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "render.jpg")
	if err := os.WriteFile(path, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func testDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	testutil.InitRepo(t, dir)
	return dir
}

func keys(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
