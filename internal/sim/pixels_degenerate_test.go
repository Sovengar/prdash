package sim

import (
	"image"
	"image/color"
	"testing"
)

// rgba is pure and its contract is "nil or an image of at least one dimension".
func TestRgbaReturnsNilForWhatCannotBeAveraged(t *testing.T) {
	if got := rgba(nil); got != nil {
		t.Errorf("rgba(nil) gave an image of %v: asking nil for its bounds is a panic", got.Bounds())
	}

	for _, c := range []struct {
		name string
		img  image.Image
	}{
		{"0x0", image.NewRGBA(image.Rect(0, 0, 0, 0))},
		{"0x5", image.NewRGBA(image.Rect(0, 0, 0, 5))},
		{"5x0", image.NewRGBA(image.Rect(0, 0, 5, 0))},
		{"empty grid", image.NewRGBA(image.Rect(4, 4, 4, 4))},
	} {
		if got := rgba(c.img); got != nil {
			t.Errorf("rgba of a %s image gave an image of %v, want nil: there are no pixels to average",
				c.name, got.Bounds())
		}
	}

	source := image.NewRGBA(image.Rect(0, 0, 3, 2))
	source.Set(1, 0, color.RGBA{R: 10, G: 20, B: 30, A: 255})
	got := rgba(source)
	if got == nil {
		t.Fatal("rgba of a real image gave nil")
	}
	if b := got.Bounds(); b.Dx() != 3 || b.Dy() != 2 {
		t.Errorf("rgba returned %v, want 3x2: the size is half the contract", b)
	}
	if got.Bounds().Min != (image.Point{}) {
		t.Errorf("rgba returned the bounds at %v, want the origin: shrink indexes without adding the Min", got.Bounds().Min)
	}
	// And the pixel is where it should be, so it really was copied and not filled with black.
	r, g, b, _ := got.At(1, 0).RGBA()
	if r>>8 != 10 || g>>8 != 20 || b>>8 != 30 {
		t.Errorf("pixel (1,0) ended up as (%d,%d,%d), want (10,20,30)", r>>8, g>>8, b>>8)
	}

	// A non-RGBA image is normalised, which is what makes the function exist.
	other := image.NewGray(image.Rect(0, 0, 2, 2))
	other.Set(0, 0, color.Gray{Y: 200})
	got = rgba(other)
	if got == nil {
		t.Fatal("rgba of a Gray image gave nil: there is no reason it cannot be read")
	}
	if _, _, _, a := got.At(0, 0).RGBA(); a == 0 {
		t.Error("the copied pixel is transparent: the conversion did not touch the content")
	}
}

func TestFitCellsWithZeroAreaInventsNoColumns(t *testing.T) {
	img := solid(16, 9, black)

	for _, a := range []measures{{40, 0}, {1, 0}, {0, 0}, {40, -3}} {
		cols, rows := FitCells(img, 1, 2, a.w, a.h)
		if cols != max(a.w, 0) || rows != max(a.h, 0) {
			t.Errorf("with a %dx%d area it gave %dx%d, want %dx%d: with no rows there is nothing to place",
				a.w, a.h, cols, rows, max(a.w, 0), max(a.h, 0))
		}
	}
	// Neither dimension positive, then neither: a 0x0 popup is a popup that cannot be drawn.
	if cols, rows := FitCells(img, 1, 2, -5, -7); cols != 0 || rows != 0 {
		t.Errorf("with a negative area it gave %dx%d, want 0x0: a negative size is not a size", cols, rows)
	}

	cols, rows := FitCells(img, 1, 2, 40, 10)
	if cols <= 0 || rows <= 0 {
		t.Errorf("with a real area it gave %dx%d, want something in both dimensions", cols, rows)
	}
}

func TestFitCellsWithOneZeroDimensionGivesTheWholeArea(t *testing.T) {
	cases := []struct {
		name string
		img  image.Image
	}{
		{"4x0", image.NewRGBA(image.Rect(0, 0, 4, 0))},
		{"0x4", image.NewRGBA(image.Rect(0, 0, 0, 4))},
		{"0x0", image.NewRGBA(image.Rect(0, 0, 0, 0))},
	}
	for _, c := range cases {
		for _, a := range []measures{{40, 10}, {10, 40}, {1, 1}} {
			cols, rows := FitCells(c.img, 1, 2, a.w, a.h)
			if cols != a.w || rows != a.h {
				t.Errorf("%s image with a %dx%d area gave %dx%d, want the whole area: "+
					"without a dimension there is no relation to compute",
					c.name, a.w, a.h, cols, rows)
			}
		}
	}
}

// At the boundary both branches give the SAME pair, and a step either way gives the other.
func TestFitCellsBothBranchesAgreeAtTheBoundary(t *testing.T) {
	img := solid(4, 1, black)
	const perRow = 8

	for maxRows := 1; maxRows <= 12; maxRows++ {
		maxCols := maxRows * perRow

		// At the boundary: height*perRow == maxCols, and the height rules.
		cols, rows := FitCells(img, 1, 2, maxCols, maxRows)
		if cols != maxCols || rows != maxRows {
			t.Errorf("at the boundary (%d rows, %d columns) it gave %dx%d, want %dx%d",
				maxRows, maxCols, cols, rows, maxCols, maxRows)
		}

		// The other branch would give the same pair: that is what kills the mutation.
		if other := max(int(float64(maxCols)/perRow), 1); other != rows {
			t.Errorf("at the boundary the other branch would give %d rows and the chosen one %d: "+
				"they do not match and the guard mutant would show", other, rows)
		}

		if maxRows >= 2 {
			if c1, r1 := FitCells(img, 1, 2, maxCols-1, maxRows); r1 != maxRows-1 {
				t.Errorf("with %d rows and %d columns it gave %dx%d, want %d rows: "+
					"one column less and the width already rules",
					maxRows, maxCols-1, c1, r1, maxRows-1)
			}
		}
		if _, r1 := FitCells(img, 1, 2, maxCols-1, 1); r1 != 1 {
			t.Errorf("with a single row and %d columns it gave %d rows, want 1: "+
				"the one-row floor keeps the image from disappearing", maxCols-1, r1)
		}

		if c2, r2 := FitCells(img, 1, 2, maxCols, maxRows+1); r2 != maxRows {
			t.Errorf("with %d rows and %d columns it gave %dx%d, want %d rows: "+
				"one row more and it no longer fits in the width",
				maxRows+1, maxCols, c2, r2, maxRows)
		}
	}
}
