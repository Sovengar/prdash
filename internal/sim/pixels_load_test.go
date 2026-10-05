package sim

import (
	"bytes"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const halfGlyph = '▀'

// The case that names the function: a PNG with the wrong suffix.
func TestLoadOpensByContentNotByExtension(t *testing.T) {
	dir := t.TempDir()
	content := testJPEG(t, 5, 2, color.RGBA{R: 200, G: 100, B: 0, A: 255})

	misleading := filepath.Join(dir, "render.png")
	if err := os.WriteFile(misleading, content, 0o644); err != nil {
		t.Fatal(err)
	}
	img, err := Load(misleading)
	if err != nil {
		t.Fatalf("a JPEG named .png did not load: %v", err)
	}
	if img.Bounds().Dx() != 5 || img.Bounds().Dy() != 2 {
		t.Errorf("it loaded %v, want 5x2", img.Bounds())
	}

	for _, name := range []string{"render.jpg", "imagen"} {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, content, 0o644); err != nil {
			t.Fatal(err)
		}
		if _, err := Load(path); err != nil {
			t.Errorf("%s did not load: %v", name, err)
		}
	}

	pngPath := filepath.Join(dir, "otro.png")
	if err := os.WriteFile(pngPath, testPNG(t, 3, 3, color.RGBA{G: 9, A: 255}), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(pngPath); err != nil {
		t.Errorf("a real PNG did not load: %v", err)
	}
}

// Two failures and what each says.
func TestLoadTellsMissingFromNotAnImage(t *testing.T) {
	dir := t.TempDir()

	// Missing: the error has to be distinguishable from the format one, so it leaves the suffix out.
	_, err := Load(filepath.Join(dir, "no-existe.jpg"))
	if err == nil {
		t.Fatal("loading a missing file gave nil")
	}
	if strings.Contains(err.Error(), "decode") {
		t.Errorf("a missing file gave %q, which looks like a format problem", err)
	}
	// os's error is preserved wrapped, so os.IsNotExist still works through it.
	if !os.IsNotExist(err) {
		t.Errorf("os.IsNotExist gave false with %v: os.Open's cause does not come through", err)
	}

	garbage := filepath.Join(dir, "basura.png")
	if err := os.WriteFile(garbage, []byte("this is not an image"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err = Load(garbage)
	if err == nil {
		t.Fatal("loading garbage gave nil")
	}
	if !strings.Contains(err.Error(), "decode") {
		t.Errorf("the error %q does not say it is a decoding one", err)
	}
	if !strings.Contains(err.Error(), "basura.png") {
		t.Errorf("the error %q does not name the file", err)
	}
	empty := filepath.Join(dir, "vacio.jpg")
	if err := os.WriteFile(empty, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(empty); err == nil {
		t.Error("an empty file gave nil")
	}

	if _, err := Load(dir); err == nil {
		t.Error("loading a directory gave nil")
	}
}

// Not defensive: average divides by n, and the only way n is zero is an empty block.
func TestAverageOfAnEmptyBlockIsTheLiteralZero(t *testing.T) {
	src := solid(2, 2, color.RGBA{R: 200, G: 200, B: 200, A: 255})

	blocks := [][4]int{
		{0, 0, 0, 1},
		{0, 0, 1, 0},
		{0, 0, 0, 0},
		{1, 1, 0, 1},
	}
	for _, b := range blocks {
		got := average(src, b[0], b[1], b[2], b[3])
		if got != (color.RGBA{}) {
			t.Errorf("the empty block %v gave %+v, want the literal zero", b, got)
		}
	}

	if got := average(src, 0, 0, 1, 1); got != (color.RGBA{R: 200, G: 200, B: 200, A: 255}) {
		t.Errorf("a one-pixel block gave %+v", got)
	}
	if got := average(src, 0, 0, 2, 2); got != (color.RGBA{R: 200, G: 200, B: 200, A: 255}) {
		t.Errorf("a uniform block gave %+v", got)
	}
}

// The reason slug exists: the project goes into a filename.
func TestSlugLeavesAFileNameNotAPath(t *testing.T) {
	inputs := []string{
		"group/project", "a//b", "my project", "project2026",
		"my-project", "my_project", "project.git",
		"/project/", "///", "", "project+1", "GROUP/PROJECT",
		"col:on:colon", "with\ttab", "with\u00f1",
	}
	seen := map[string]bool{}
	for _, in := range inputs {
		got := slug(in)
		// The property: no path separators, no spaces, no tabs, which is what keeps the name usable.
		if strings.ContainsAny(got, "/ \t\\:*?\"<>|") {
			t.Errorf("slug(%q) gave %q, which is not a file name", in, got)
		}
		// And no leading dashes, which is what keeps the cache listing readable.
		if strings.HasPrefix(got, "-") || strings.HasSuffix(got, "-") {
			t.Errorf("slug(%q) gave %q, with dashes on the edges", in, got)
		}
		seen[got] = true
	}

	if got := slug("group/project"); got != "group-project" {
		t.Errorf("a slash gave %q", got)
	}
	if got := slug("///"); got != "" {
		t.Errorf("only slashes gave %q, want empty: a name of dashes is not a name", got)
	}
	for _, in := range []string{"my-project", "my_project", "project.git"} {
		if slug(in) != in {
			t.Errorf("slug(%q) changed it to %q, and those characters are kept", in, slug(in))
		}
	}
}

// The missing path: Cells assumes RGBA.
func TestPalettedImageIsConvertedBeforeAveraging(t *testing.T) {
	pal := color.Palette{color.RGBA{R: 0, G: 0, B: 0, A: 0}, color.RGBA{R: 77, G: 0, B: 0, A: 255}}
	img := image.NewPaletted(image.Rect(0, 0, 2, 2), pal)
	for y := range 2 {
		for x := range 2 {
			img.SetColorIndex(x, y, 1)
		}
	}

	cells := Cells(img, 2, 2)
	if len(cells) != 2 {
		t.Fatalf("%d cells came out, want 2", len(cells))
	}
	for i, c := range cells {
		if !strings.Contains(c, "38;2;77;0;0") {
			t.Errorf("cell %d does not bring the Paletted's color: %q", i, c)
		}
	}
	if n := strings.Count(cells[0], string(halfGlyph)); n != 2 {
		t.Errorf("the cell brings %d half blocks, want 2: %q", n, cells[0])
	}
}

func testJPEG(t *testing.T, w, h int, c color.RGBA) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, solid(w, h, c), nil); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func testPNG(t *testing.T, w, h int, c color.RGBA) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := png.Encode(&buf, solid(w, h, c)); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}
