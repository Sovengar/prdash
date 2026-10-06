package sim

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"prdash/internal/forge/model"
)

// The case that proves it is a failure midway: a half-written destination would offer the popup a broken
// image under the right name.
func TestCopyFileToTempLeavesTheDestinationIntactAndNotTheTemp(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "origen.jpg")
	if err := os.WriteFile(src, []byte("some random image"), 0o644); err != nil {
		t.Fatal(err)
	}
	dst := filepath.Join(dir, "destino.jpg")

	if err := copyFile(src, dst); err != nil {
		t.Fatalf("copyFile: %v", err)
	}
	got, err := os.ReadFile(dst)
	if err != nil {
		t.Fatalf("reading the destination: %v", err)
	}
	if string(got) != "some random image" {
		t.Errorf("the destination has %q, want the source's content", got)
	}
	if _, err := os.Stat(dst + ".part"); !os.IsNotExist(err) {
		t.Errorf("the temp %s.part was left behind", dst)
	}

	if err := os.WriteFile(src, []byte("second version"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := copyFile(src, dst); err != nil {
		t.Fatalf("overwriting the destination: %v", err)
	}
	if got, _ := os.ReadFile(dst); string(got) != "second version" {
		t.Errorf("after the second copy there is %q, want the second version", got)
	}
}

// No simulated cut needed: copying a DIRECTORY as if it were an image fails with EISDIR on the read.
func TestCopyFileCleansTheTempWhenTheCopyIsCut(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "this-is-a-directory.jpg")
	if err := os.MkdirAll(src, 0o755); err != nil {
		t.Fatal(err)
	}
	dst := filepath.Join(dir, "destino.jpg")

	err := copyFile(src, dst)
	if err == nil {
		t.Fatal("copying a directory gave nil")
	}
	if !errors.Is(err, fs.ErrInvalid) && !strings.Contains(err.Error(), "is a directory") {
		t.Logf("the OS error does not mention EISDIR: %v", err)
	}
	if _, err := os.Stat(dst); !os.IsNotExist(err) {
		t.Error("the failed copy left the destination: the popup would open a broken image")
	}
	if _, err := os.Stat(dst + ".part"); !os.IsNotExist(err) {
		t.Error("the failed copy left the temp: it would pile up in the cache with nothing deleting it")
	}
}

func TestCopyFileNamesTheFailureAtEachPoint(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "origen.jpg")
	if err := os.WriteFile(src, []byte("content"), 0o644); err != nil {
		t.Fatal(err)
	}

	for _, c := range []struct {
		name     string
		src, dst string
	}{
		{"source does not exist", filepath.Join(dir, "no-existe.jpg"),
			filepath.Join(dir, "d1.jpg")},
		{"destination's parent missing", src, filepath.Join(dir, "no-existe", "d2.jpg")},
		{"destination is a directory", src, dir},
	} {
		err := copyFile(c.src, c.dst)
		if err == nil {
			t.Errorf("%s: copyFile gave nil", c.name)
			continue
		}
		// The message names the file involved. os already puts the path in its error, so this checks that
		//keep's wrap does not swallow it.
		base := c.dst
		if c.name == "source does not exist" {
			base = c.src
		}
		if !strings.Contains(err.Error(), filepath.Base(base)) {
			t.Errorf("%s: the error %q does not name the file involved", c.name, err)
		}
		if _, err := os.Stat(c.dst + ".part"); !os.IsNotExist(err) {
			t.Errorf("%s: it left the temp", c.name)
		}
	}
}

// "By date" is what a table test would miss: the filename carries a UnixNano and makes it look like
// name order is date order.
func TestPruneKeepsTheNewestByDateNotByName(t *testing.T) {
	dir := t.TempDir()
	base := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)

	// The names run opposite to the dates on purpose, so a prune sorting by name would keep exactly the
	//three it must not.
	for i := 9; i >= 1; i-- {
		name := itoa(i) + ".jpg"
		if err := os.WriteFile(filepath.Join(dir, name), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
		when := base.AddDate(0, 0, i)
		if err := os.Chtimes(filepath.Join(dir, name), when, when); err != nil {
			t.Fatal(err)
		}
	}

	prune(dir, 3)

	remaining := namesIn(t, dir)
	want := []string{"9.jpg", "8.jpg", "7.jpg"}
	if len(remaining) != len(want) {
		t.Fatalf("%v left, want %v", remaining, want)
	}
	for _, n := range want {
		if !contains(remaining, n) {
			t.Errorf("%s was not kept: there are %v", n, remaining)
		}
	}
	for _, n := range []string{"1.jpg", "2.jpg", "3.jpg", "4.jpg", "5.jpg", "6.jpg"} {
		if contains(remaining, n) {
			t.Errorf("%s was kept and it is one of the oldest: there are %v", n, remaining)
		}
	}
}

func TestPruneIgnoresWhatIsNotAnImageAndDoesNotWipeTheWholeCache(t *testing.T) {
	dir := t.TempDir()
	base := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)

	touchable := []string{"a.jpg", "b.jpg", "c.jpg", "d.jpg"}
	for _, n := range touchable {
		p := filepath.Join(dir, n)
		if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.Chtimes(p, base, base); err != nil {
			t.Fatal(err)
		}
	}
	untouchable := map[string]string{
		"backup.jpg":     "directory",
		"notas.txt":      "text",
		"HEAD":           "file",
		"candidata.JPEG": "image in uppercase",
	}
	for n, class := range untouchable {
		p := filepath.Join(dir, n)
		var err error
		if class == "directory" {
			err = os.MkdirAll(filepath.Join(p, "contenido"), 0o755)
		} else {
			err = os.WriteFile(p, []byte("x"), 0o644)
		}
		if err != nil {
			t.Fatalf("preparing %s: %v", n, err)
		}
		if err := os.Chtimes(p, base, base); err != nil {
			t.Fatal(err)
		}
	}

	// A file named exactly `.jpg` IS pruned, because prune decides by suffix and HasSuffix(".jpg",".jpg")
	//is true. It was on the untouchable list, which is what broke the count below.
	extensionOnlyName := filepath.Join(dir, ".jpg")
	if err := os.WriteFile(extensionOnlyName, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	oldest := base.AddDate(0, 0, -1)
	if err := os.Chtimes(extensionOnlyName, oldest, oldest); err != nil {
		t.Fatal(err)
	}

	prune(dir, 1)

	if _, err := os.Stat(extensionOnlyName); err == nil {
		t.Errorf("the file %q survived the prune: prune decides by suffix", ".jpg")
	}
	for n := range untouchable {
		if _, err := os.Stat(filepath.Join(dir, n)); err != nil {
			t.Errorf("prune deleted %s (%s): %v", n, untouchable[n], err)
		}
	}
	remain := 0
	for _, n := range touchable {
		if _, err := os.Stat(filepath.Join(dir, n)); err == nil {
			remain++
		}
	}
	if remain != 1 {
		t.Errorf("%d images are left of 4, want 1", remain)
	}

	if _, err := os.Stat(filepath.Join(dir, "backup.jpg", "contenido")); err != nil {
		t.Errorf("prune emptied the subdirectory: %v", err)
	}
}

func TestPruneWithFewerThanToKeepDoesNothingAndWithZeroDeletesAll(t *testing.T) {
	dir := t.TempDir()
	for _, n := range []string{"a.jpg", "b.jpg"} {
		if err := os.WriteFile(filepath.Join(dir, n), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	prune(dir, 10)
	if len(namesIn(t, dir)) != 2 {
		t.Errorf("prune with keep=10 removed files: %v", namesIn(t, dir))
	}
	// A NEGATIVE keep is clamped to zero instead of panicking: `files[-1:]` gave "slice bounds out
	// of range [-1:]". My first version asserted it did not panic; now I check it the other way.
	prune(dir, -1)
	if remain := len(namesIn(t, dir)); remain != 0 {
		t.Errorf("with keep=-1 %d files are left, want 0 (negative reads as zero)", remain)
	}

	prune(dir, 0)
	if remain := namesIn(t, dir); len(remain) != 0 {
		t.Errorf("prune with keep=0 left %v", remain)
	}
}

func TestPruneOnMissingDirectoryDoesNotBlowUp(t *testing.T) {
	prune(filepath.Join(t.TempDir(), "no-existe"), 3)

	file := filepath.Join(t.TempDir(), "soy-a-file")
	if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	prune(file, 3)
	if _, err := os.Stat(file); err != nil {
		t.Errorf("prune took a file that was not a directory: %v", err)
	}
}

func TestKeepCopiesToCacheWithTheItemNameAndPrunesTheRest(t *testing.T) {
	cache := t.TempDir()
	source := writeJPEG(t)

	s := &Service{CacheDir: cache}
	it := testItem()
	it.Number = 42

	dst, err := s.keep(source, it, KindMerge)
	if err != nil {
		t.Fatalf("keep: %v", err)
	}
	a, err := os.ReadFile(source)
	if err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(dst)
	if err != nil {
		t.Fatalf("reading the cache copy: %v", err)
	}
	if len(a) != len(b) {
		t.Fatalf("the copy weighs %d and the original %d", len(b), len(a))
	}
	for i := range a {
		if a[i] != b[i] {
			t.Fatalf("the copy differs from the original at byte %d", i)
		}
	}
	name := filepath.Base(dst)
	if !strings.HasPrefix(name, "github-") {
		t.Errorf("the name %q does not start with the forge", name)
	}
	if !strings.Contains(name, "-42-") {
		t.Errorf("the name %q does not carry the item's number", name)
	}
	if strings.ContainsAny(name, "/ #") {
		t.Errorf("the name %q carries characters a viewer cannot handle", name)
	}

	for i := 0; i < keepImages+1; i++ {
		p := filepath.Join(cache, "github-extra-"+itoa(i)+"-1.jpg")
		if err := os.WriteFile(p, []byte("garbage"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	last, err := s.keep(source, it, KindMerge)
	if err != nil {
		t.Fatal(err)
	}
	// The image just saved cannot be deleted. My first version looked at `dst`, the image from the FIRST
	//keep call, which is the oldest, and failed intermittently.
	if _, err := os.Stat(last); err != nil {
		t.Errorf("the prune deleted the image that was just saved: %v", err)
	}
	if last == dst {
		t.Error("the two keep calls gave the same name: the name's UnixNano repeats and " +
			"one would overwrite the other")
	}
	remain := 0
	for _, n := range namesIn(t, cache) {
		if strings.HasSuffix(n, ".jpg") {
			remain++
		}
	}
	if remain != keepImages {
		t.Errorf("%d images are left after the prune, want %d", remain, keepImages)
	}
}

func testItem() model.Item {
	return model.NewItem(model.RepoRef{
		Forge: "github", Host: "github.com", Project: "acme/widget",
	}, 7)
}

func namesIn(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("listing %s: %v", dir, err)
	}
	out := make([]string, 0, len(entries))
	for _, e := range entries {
		out = append(out, e.Name())
	}
	return out
}

func contains(list []string, s string) bool {
	for _, e := range list {
		if e == s {
			return true
		}
	}
	return false
}
