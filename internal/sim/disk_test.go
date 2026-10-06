package sim

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
	"testing"
	"time"

	"prdash/internal/forge/model"
)

// ENOSPC on the render directory is real —a small tmpfs swallows a big repo— and the warning must not
// talk about git.
func TestFullDiskIsWarnedAsWhatItIsAndNotAsAGitFailure(t *testing.T) {
	repo, _ := simRepoMount(t)
	it := model.NewItem(model.RepoRef{Forge: "github", Host: "github.com", Project: "acme/widget"}, 7)
	it.TargetBranch = "main"

	s := newService(t, fakeLocator{ok: true, place: Place{Repo: repo, Branch: "main-origin"}},
		fakeSim(t, writeJPEG(t)))

	// The mkdir is counted and inspected, because an injected Mkdir that is never called proves
	// nothing.
	calls := 0
	s.Mkdir = func(path string, perm fs.FileMode) error {
		calls++
		if filepath.Base(path) != "media" {
			t.Errorf("it created %q, which is not the render directory", path)
		}
		if perm != 0o755 {
			t.Errorf("permissions %o, want 755", perm)
		}
		return &os.PathError{Op: "mkdir", Path: path, Err: syscall.ENOSPC}
	}

	res, err := s.Simulate(context.Background(), it, KindMerge)
	if err == nil {
		t.Fatalf("no space gave nil and the image %q", res.Path)
	}
	if calls != 1 {
		t.Fatalf("the directory creation was attempted %d times, want 1", calls)
	}
	if res.Path != "" {
		t.Errorf("with a full disk it returned the path %q: the popup would offer opening an "+
			"image that was never generated", res.Path)
	}

	msg := err.Error()
	if !strings.Contains(msg, "prepare the render directory") {
		t.Errorf("the warning %q does not say what was being prepared", msg)
	}
	if !strings.Contains(msg, "no space left") {
		t.Errorf("the warning %q does not bring the ENOSPC cause, which is the half that says "+
			"how to fix it", msg)
	}
	// And it says nothing about git, which is the error it is most confused with.
	for _, gitWord := range []string{"clone", "check out", "branch --quiet"} {
		if strings.Contains(msg, gitWord) {
			t.Errorf("the warning %q talks about git (%q) and the failure was the disk's", msg, gitWord)
		}
	}

	clean := &Service{Locator: fakeLocator{}}
	if clean.mkdir() == nil {
		t.Error("mkdir() did not return a function for a hand-built service")
	}
}

// The most important of the three: its consequence is not a warning but a `.part` the size of the
// JPEG that `prune` never deletes.
func TestUnclosedTempIsDeletedAndNothingIsPublished(t *testing.T) {
	dir := t.TempDir()
	dst := filepath.Join(dir, "destino.jpg")
	content := []byte("a fake jpeg")

	broken := &fakeWrite{closeErr: errClosing}
	err := copyPublishing(bytes.NewReader(content), dst, broken.opensAt)
	if err == nil {
		t.Fatal("an unclosed temp gave nil: half a JPEG would be published")
	}
	if !errors.Is(err, errClosing) {
		t.Errorf("the error is %v, want the close one. A write failure would give the "+
			"`write` one and prove the other path", err)
	}
	if !bytes.Equal(broken.written, content) {
		t.Errorf("it wrote %q, want %q", broken.written, content)
	}

	// And the temporary is gone. This is the assertion that matters: a `.part` weighs as much as the
	// JPEG it was going to become.
	if _, err := os.Stat(dst + ".part"); !os.IsNotExist(err) {
		t.Errorf("the temp %s was left behind: it weighs what the image weighs and `prune` only "+
			"looks at the `.jpg`", dst+".part")
	}
	if _, err := os.Stat(dst); !os.IsNotExist(err) {
		t.Errorf("it published %s without closing the temp properly", dst)
	}

	good := &fakeWrite{closeErr: nil}
	if err := copyPublishing(bytes.NewReader([]byte("hello")), filepath.Join(dir, "bueno.jpg"),
		good.opensAt); err != nil {
		t.Fatalf("with a working close: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "bueno.jpg.part")); !os.IsNotExist(err) {
		t.Error("the temp was left after a well-done rename")
	}
	got, err := os.ReadFile(filepath.Join(dir, "bueno.jpg"))
	if err != nil || string(got) != "hello" {
		t.Errorf("what was published is %q (%v)", got, err)
	}

	src := filepath.Join(dir, "origen.jpg")
	if err := os.WriteFile(src, content, 0o644); err != nil {
		t.Fatal(err)
	}
	direct := filepath.Join(dir, "directo.jpg")
	if err := copyFile(src, direct); err != nil {
		t.Fatalf("copyFile without seam: %v", err)
	}
	if _, err := os.Stat(direct); err != nil {
		t.Errorf("copyFile did not publish: %v", err)
	}
}

var errClosing = errors.New("the filesystem could not close the temp")

// It writes a real file because an in-memory double would leave nothing to assert on.
type fakeWrite struct {
	closeErr error

	written []byte
	file    *os.File
}

func (e *fakeWrite) opensAt(path string) (writer, error) {
	f, err := os.Create(path)
	if err != nil {
		return nil, err
	}
	e.file = f
	return e, nil
}

func (e *fakeWrite) Write(p []byte) (int, error) {
	e.written = append(e.written, p...)
	if e.file == nil {
		return len(p), nil
	}
	return e.file.Write(p)
}

func (e *fakeWrite) Close() error {
	if e.file != nil {
		_ = e.file.Close()
		e.file = nil
	}
	return e.closeErr
}

var _ writer = &fakeWrite{}

// The race is real but cannot be forced, so an already-gone entry is handed over instead.
func TestImageDisappearingBeforeLstatDoesNotBreakThePrune(t *testing.T) {
	dir := t.TempDir()
	base := time.Now().Add(-24 * time.Hour)

	base0 := fakeImage(t, dir, "img-00.jpg", base)
	var survivors []string
	for i := 1; i <= keepImages; i++ {
		name := "img-" + fourDigits(i) + ".jpg"
		delta := time.Duration(i) * time.Hour
		fakeImage(t, dir, name, base.Add(delta))
		survivors = append(survivors, filepath.Join(dir, name))
	}

	listing := func(string) ([]os.DirEntry, error) {
		real, err := os.ReadDir(dir)
		if err != nil {
			return nil, err
		}
		return append(real,
			deadEntry{name: filepath.Base(base0), err: errors.New("lstat: no such file")},
		), nil
	}

	pruneWith(dir, keepImages, listing)

	if _, err := os.Stat(base0); !os.IsNotExist(err) {
		t.Error("the oldest image was not pruned")
	}
	left, err := filepath.Glob(filepath.Join(dir, "img-*.jpg"))
	if err != nil {
		t.Fatal(err)
	}
	if len(left) != keepImages {
		t.Errorf("%d images are left, want %d: an unreadable entry cannot cause over-pruning",
			len(left), keepImages)
	}
	for _, r := range left {
		if !slices.Contains(survivors, r) {
			t.Errorf("%s was left, and it was not on the should-survive list", r)
		}
	}

	// And the whole listing failing.
	calm := t.TempDir()
	fakeImage(t, calm, "a.jpg", base)
	pruneWith(calm, 0, func(string) ([]os.DirEntry, error) {
		return nil, &os.PathError{Op: "readdir", Path: calm, Err: fs.ErrPermission}
	})
	if _, err := os.Stat(filepath.Join(calm, "a.jpg")); err != nil {
		t.Errorf("an unreadable listing emptied the cache: %v", err)
	}

	real := t.TempDir()
	for i := 0; i <= keepImages; i++ {
		fakeImage(t, real, "img-"+fourDigits(i)+".jpg", base.Add(time.Duration(i)*time.Hour))
	}
	prune(real, keepImages)
	remaining, err := filepath.Glob(filepath.Join(real, "img-*.jpg"))
	if err != nil {
		t.Fatal(err)
	}
	if len(remaining) != keepImages {
		t.Errorf("prune without seam left %d images, want %d", len(remaining), keepImages)
	}
}

type deadEntry struct {
	name string
	err  error
}

func (e deadEntry) Name() string               { return e.name }
func (e deadEntry) IsDir() bool                { return false }
func (e deadEntry) Type() fs.FileMode          { return 0 }
func (e deadEntry) Info() (fs.FileInfo, error) { return nil, e.err }

var _ os.DirEntry = deadEntry{}

func fakeImage(t *testing.T, dir, name string, when time.Time) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(path, when, when); err != nil {
		t.Fatal(err)
	}
	return path
}

func fourDigits(i int) string {
	s := fmt.Sprintf("%04d", i)
	return s[len(s)-4:]
}

var _ io.Writer = &fakeWrite{}
