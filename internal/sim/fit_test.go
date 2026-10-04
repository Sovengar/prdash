package sim

import (
	"image"
	"image/color"
	"testing"
)

var black = color.RGBA{A: 255}

// A cell is twice as tall as wide, so a 16:9 image needs 3.56 columns per row.
func TestFitKeepsTheAspectOfAWideImage(t *testing.T) {
	img := solid(1920, 1080, black)

	cols, rows := Fit(img, 400, 40)
	if rows != 40 {
		t.Errorf("rows = %d, want 40: manda el alto", rows)
	}
	if cols < 138 || cols > 146 {
		t.Errorf("cols = %d, want ~142 (3,56 columnas por fila para 16:9)", cols)
	}
}

func TestFitPrefersTheWidthWhenItBinds(t *testing.T) {
	img := solid(1920, 1080, black)

	cols, rows := Fit(img, 100, 400)
	if cols != 100 {
		t.Errorf("cols = %d, want 100: manda la anchura", cols)
	}
	if rows >= 400 {
		t.Errorf("rows = %d, want menos que el máximo disponible", rows)
	}
	if rows < 25 || rows > 31 {
		t.Errorf("rows = %d, want ~28", rows)
	}
}

func TestFitNeverExceedsTheArea(t *testing.T) {
	for _, size := range [][2]int{{1, 1}, {1920, 1080}, {4, 900}, {900, 4}, {3, 3}} {
		img := solid(size[0], size[1], black)
		for _, area := range [][2]int{{10, 10}, {200, 60}, {5, 100}, {100, 5}, {1, 1}} {
			cols, rows := Fit(img, area[0], area[1])
			if cols > area[0] || rows > area[1] {
				t.Errorf("Fit(%dx%d, %dx%d) = %dx%d, no cabe", size[0], size[1], area[0], area[1], cols, rows)
			}
			if cols < 1 || rows < 1 {
				t.Errorf("Fit(%dx%d, %dx%d) = %dx%d, no dibuja nada", size[0], size[1], area[0], area[1], cols, rows)
			}
		}
	}
}

func TestFitDegradesOnAnUnusableInput(t *testing.T) {
	if c, r := Fit(nil, 40, 10); c != 40 || r != 10 {
		t.Errorf("Fit(nil) = %dx%d, want 40x10", c, r)
	}
	if c, r := Fit(image.NewRGBA(image.Rect(0, 0, 0, 0)), 40, 10); c != 40 || r != 10 {
		t.Errorf("Fit(vacía) = %dx%d, want 40x10", c, r)
	}
	if c, r := Fit(solid(4, 4, black), 0, 10); c != 0 || r != 10 {
		t.Errorf("Fit(w=0) = %dx%d, want 0x10", c, r)
	}
}
