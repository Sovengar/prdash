package sim

import (
	"image"
	"image/color"
	"strconv"
	"testing"
)

type measures struct{ w, h int }

// Two invariants and both matter: the box is NEVER exceeded, and the aspect ratio is respected.
func TestFitCellsRespectsTheBoxAndUsesTheDimensionThatRules(t *testing.T) {
	shapes := []measures{{16, 9}, {9, 16}, {10, 10}, {100, 3}, {3, 100}, {1, 1}, {1920, 1080}}
	cells := []measures{{1, 2}, {9, 19}, {1, 1}, {2, 1}, {8, 16}}
	areas := []measures{{40, 10}, {10, 40}, {20, 20}, {1, 1}, {200, 60}, {5, 3}, {3, 5}}

	for _, f := range shapes {
		img := solid(f.w, f.h, color.RGBA{R: 1, G: 2, B: 3, A: 255})
		for _, c := range cells {
			for _, a := range areas {
				cols, rows := FitCells(img, c.w, c.h, a.w, a.h)
				label := "img " + itoa(f.w) + "x" + itoa(f.h) + ", cell " + itoa(c.w) + "x" + itoa(c.h) +
					", area " + itoa(a.w) + "x" + itoa(a.h)

				if cols > a.w || rows > a.h {
					t.Errorf("%s: it gave %dx%d, want <= %dx%d: it goes out of the popup",
						label, cols, rows, a.w, a.h)
					continue
				}
				if cols < 1 || rows < 1 {
					t.Errorf("%s: it gave %dx%d, want at least 1x1: a zero-cell popup shows nothing",
						label, cols, rows)
					continue
				}
				if cols < a.w && rows < a.h {
					t.Errorf("%s: it gave %dx%d and the area is %dx%d: space was left in both "+
						"dimensions, and the largest size that fits was to be chosen", label, cols, rows, a.w, a.h)
				}
			}
		}
	}
}

// The ratio is the image's OVER the cell's, not the reverse.
func TestFitCellsPreservesTheAspectRatio(t *testing.T) {
	square := solid(100, 100, black)
	cols, rows := FitCells(square, 1, 2, 100, 10)
	if cols != 20 || rows != 10 {
		t.Errorf("square in 1x2 cells gave %dx%d, want 20x10: the height rules (10 lines) and each line needs 2 columns", cols, rows)
	}
	cols, rows = FitCells(square, 9, 19, 100, 10)
	if cols != 21 || rows != 10 {
		t.Errorf("square in 9x19 cells gave %dx%d, want 21x10: 19/9 = 2.11 columns per line", cols, rows)
	}
	wide := solid(160, 90, black)
	cols, rows = FitCells(wide, 1, 2, 80, 20)
	if cols != 71 || rows != 20 {
		t.Errorf("16:9 in 1x2 cells gave %dx%d, want 71x20: 3.56 columns per line x 20 lines = 71, which fits in 80", cols, rows)
	}
	// With fewer rows the WIDTH takes over: both cuts have to be right.
	cols, rows = FitCells(wide, 1, 2, 80, 30)
	if cols != 80 || rows != 22 {
		t.Errorf("16:9 in 1x2 cells with 30 rows gave %dx%d, want 80x22: the 107 columns no longer fit and the width rules", cols, rows)
	}
	tall := solid(90, 160, black)
	cols, rows = FitCells(tall, 1, 2, 80, 20)
	if rows != 20 || cols != 22 {
		t.Errorf("9:16 in 1x2 cells gave %dx%d, want 22x20: the height rules and each line needs 1.125 columns", cols, rows)
	}
}

func TestFitCellsFallsBackToDefaultsWithoutBreaking(t *testing.T) {
	img := solid(16, 9, black)

	for _, a := range []measures{{40, 10}, {0, 10}, {40, 0}, {0, 0}, {-5, -7}} {
		cols, rows := FitCells(nil, 1, 2, a.w, a.h)
		if want, wantR := max(a.w, 0), max(a.h, 0); cols != want || rows != wantR {
			t.Errorf("without an image and a %dx%d area it gave %dx%d, want %dx%d: a negative size sneaks into the layout",
				a.w, a.h, cols, rows, want, wantR)
		}
	}

	empty := image.NewRGBA(image.Rect(0, 0, 0, 0))
	for _, a := range []measures{{40, 10}, {1, 1}} {
		if cols, rows := FitCells(empty, 1, 2, a.w, a.h); cols != a.w || rows != a.h {
			t.Errorf("a 0x0 image with a %dx%d area gave %dx%d, want the whole area", a.w, a.h, cols, rows)
		}
	}

	wantCols, wantRows := FitCells(img, 1, 2, 40, 10)
	for _, c := range []measures{{0, 0}, {0, 2}, {1, 0}, {-1, -1}} {
		cols, rows := FitCells(img, c.w, c.h, 40, 10)
		if cols != wantCols || rows != wantRows {
			t.Errorf("cell %dx%d gave %dx%d, want %dx%d (the 1x2 default): without a cell size there is no dividing",
				c.w, c.h, cols, rows, wantCols, wantRows)
		}
	}
	// A zero in ONE of the two does not drag the other: 0x5 is 1x5, not 1x2.
	wantC5, wantR5 := FitCells(img, 1, 5, 40, 10)
	if c0, r0 := FitCells(img, 0, 5, 40, 10); c0 != wantC5 || r0 != wantR5 {
		t.Errorf("cell 0x5 gave %dx%d, want %dx%d (1x5): a zero in the width does not touch the declared height", c0, r0, wantC5, wantR5)
	}
	if wantC5 == wantCols && wantR5 == wantRows {
		t.Error("1x5 and 1x2 gave the same size: the cell height does not enter the ratio, which is exactly what is asserted")
	}
}

func TestFitIsFitCellsWithTheUsualCell(t *testing.T) {
	for _, f := range []measures{{16, 9}, {9, 16}, {10, 10}, {1920, 1080}} {
		img := solid(f.w, f.h, black)
		for _, a := range []measures{{40, 10}, {10, 40}, {1, 1}} {
			gotC, gotR := Fit(img, a.w, a.h)
			wantC, wantR := FitCells(img, 1, 2, a.w, a.h)
			if gotC != wantC || gotR != wantR {
				t.Errorf("Fit and FitCells(1,2) differ with img %dx%d and area %dx%d: %dx%d vs %dx%d",
					f.w, f.h, a.w, a.h, gotC, gotR, wantC, wantR)
			}
		}
	}
}

func TestResizePromisesTheColorOfASingleColor(t *testing.T) {
	red := color.RGBA{R: 200, G: 10, B: 20, A: 255}
	for _, f := range []measures{{1, 1}, {2, 2}, {64, 64}, {640, 480}} {
		for _, d := range []measures{{1, 1}, {2, 3}, {16, 9}, {100, 100}, {3, 1}} {
			out := Resize(solid(f.w, f.h, red), d.w, d.h)
			if out == nil {
				t.Fatalf("Resize(%dx%d -> %dx%d) returned nil", f.w, f.h, d.w, d.h)
			}
			if got := out.Bounds(); got.Dx() != d.w || got.Dy() != d.h {
				t.Errorf("Resize(%dx%d -> %dx%d) gave a map of %dx%d", f.w, f.h, d.w, d.h, got.Dx(), got.Dy())
				continue
			}
			for y := range d.h {
				for x := range d.w {
					if got := out.RGBAAt(x, y); got != red {
						t.Fatalf("Resize(%dx%d -> %dx%d): pixel (%d,%d) = %+v, want %+v",
							f.w, f.h, d.w, d.h, x, y, got, red)
					}
				}
			}
		}
	}
}

// A rescale that averages red well and blue badly yields a colour that is not in the source image.
func TestResizeAveragesTheChannelsAndNotJustOne(t *testing.T) {
	cols := []color.RGBA{
		{R: 0, G: 0, B: 0, A: 255},
		{R: 100, G: 0, B: 0, A: 255},
		{G: 0, B: 100, A: 255},
		{R: 200, G: 200, B: 200, A: 255},
	}
	img := image.NewRGBA(image.Rect(0, 0, 2, 2))
	for i, c := range cols {
		img.SetRGBA(i%2, i/2, c)
	}

	out := Resize(img, 1, 1)
	if out == nil {
		t.Fatal("Resize returned nil")
	}
	got := out.RGBAAt(0, 0)
	want := color.RGBA{
		R: uint8((0 + 100 + 0 + 200) / 4),
		G: uint8((0 + 0 + 0 + 200) / 4),
		B: uint8((0 + 0 + 100 + 200) / 4),
		A: 255,
	}
	if got != want {
		t.Errorf("average of 2x2 to 1x1 = %+v, want %+v (each channel between the same number of pixels)", got, want)
	}

	out = Resize(img, 2, 1)
	if out == nil {
		t.Fatal("Resize returned nil")
	}
	if left := out.RGBAAt(0, 0); left != (color.RGBA{R: 0, G: 0, B: 50, A: 255}) {
		t.Errorf("the left half = %+v, want the average of the two pixels of column 0 (black and blue)", left)
	}
	if right := out.RGBAAt(1, 0); right != (color.RGBA{R: 150, G: 100, B: 100, A: 255}) {
		t.Errorf("the right half = %+v, want the average of the two pixels of column 1 (red and grey)", right)
	}
}

func TestResizeWithoutOutputOrWithoutImageReturnsNil(t *testing.T) {
	img := solid(4, 4, color.RGBA{R: 1, A: 255})
	for _, d := range []measures{{0, 4}, {4, 0}, {0, 0}, {-1, 4}, {4, -1}} {
		if out := Resize(img, d.w, d.h); out != nil {
			t.Errorf("Resize to %dx%d returned an image of %v, want nil", d.w, d.h, out.Bounds())
		}
	}
	for _, d := range []measures{{4, 4}, {1, 1}} {
		if out := Resize(nil, d.w, d.h); out != nil {
			t.Errorf("Resize(nil, %dx%d) returned an image, want nil", d.w, d.h)
		}
	}
}

// Bounds says WHERE the image starts: a cropped PNG is half an image returned by git-sim.
func TestResizeRespectsTheBoundsOrigin(t *testing.T) {
	// A crop that falls EXACTLY between two different halves: an absolute-coordinate reader would take only one.
	red := color.RGBA{R: 250, G: 0, B: 0, A: 255}
	green := color.RGBA{R: 0, G: 250, B: 0, A: 255}
	background := image.NewRGBA(image.Rect(0, 0, 100, 100))
	for y := range 100 {
		for x := range 100 {
			if x < 50 {
				background.SetRGBA(x, y, red)
			} else {
				background.SetRGBA(x, y, green)
			}
		}
	}
	crop := background.SubImage(image.Rect(30, 30, 70, 70))
	if got := crop.Bounds().Min; got.X != 30 || got.Y != 30 {
		t.Fatalf("the crop should have Min = (30,30), it gave %v", got)
	}

	want := color.RGBA{R: 125, G: 125, B: 0, A: 255}
	if got := Resize(crop, 1, 1).RGBAAt(0, 0); got != want {
		t.Errorf("the straddling crop to 1x1 gave %+v, want %+v: it read the original image instead of the crop", got, want)
	}
	out := Resize(crop, 40, 10)
	if out == nil {
		t.Fatal("Resize returned nil")
	}
	if got := out.RGBAAt(0, 0); got.R < got.G {
		t.Errorf("the crop's left gave %+v, want more red than green", got)
	}
	if got := out.RGBAAt(39, 0); got.G < got.R {
		t.Errorf("the crop's right gave %+v, want more green than red", got)
	}

	flat := solid(100, 100, green)
	sub := flat.SubImage(image.Rect(30, 30, 70, 70))
	for _, d := range []measures{{1, 1}, {8, 6}, {40, 10}} {
		got := Resize(sub, d.w, d.h)
		if got == nil {
			t.Fatalf("Resize returned nil")
		}
		for y := range d.h {
			for x := range d.w {
				if c := got.RGBAAt(x, y); c != green {
					t.Fatalf("the solid crop to %dx%d gave %+v at (%d,%d), want %+v: it read the original image instead of the crop",
						d.w, d.h, c, x, y, green)
				}
			}
		}
	}
}

func itoa(n int) string { return strconv.Itoa(n) }
