package sim

import (
	"errors"
	"image"
	"image/color"
	"strings"
	"testing"
)

func solid(w, h int, c color.RGBA) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := range h {
		for x := range w {
			img.SetRGBA(x, y, c)
		}
	}
	return img
}

func TestCellsEmitsTwoPixelsPerCell(t *testing.T) {
	lines := Cells(solid(4, 4, color.RGBA{R: 10, G: 20, B: 30, A: 255}), 3, 2)
	if len(lines) != 2 {
		t.Fatalf("lines = %d, want 2", len(lines))
	}
	for i, l := range lines {
		if n := strings.Count(l, halfBlock); n != 3 {
			t.Errorf("línea %d = %d celdas, want 3", i, n)
		}
	}
}

// The cell's colour is the top pixel as foreground and the bottom as background.
func TestCellsEncodesBothHalves(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 1, 2))
	img.SetRGBA(0, 0, color.RGBA{R: 10, G: 20, B: 30, A: 255})   // arriba
	img.SetRGBA(0, 1, color.RGBA{R: 200, G: 100, B: 50, A: 255}) // abajo

	lines := Cells(img, 1, 1)
	if len(lines) != 1 {
		t.Fatalf("lines = %d, want 1", len(lines))
	}
	if !strings.Contains(lines[0], "38;2;10;20;30") {
		t.Errorf("no pinta el píxel de arriba como foreground:\n%q", lines[0])
	}
	if !strings.Contains(lines[0], "48;2;200;100;50") {
		t.Errorf("no pinta el píxel de abajo como background:\n%q", lines[0])
	}
}

func TestCellsAveragesInsteadOfSampling(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 2, 1))
	img.SetRGBA(0, 0, color.RGBA{R: 100, A: 255})
	img.SetRGBA(1, 0, color.RGBA{R: 200, A: 255})

	lines := Cells(img, 1, 1)
	if !strings.Contains(lines[0], "38;2;150;0;0") {
		t.Errorf("no promedia, no muestrea:\n%q", lines[0])
	}
}

func TestCellsGuardsBadGeometry(t *testing.T) {
	img := solid(2, 2, color.RGBA{A: 255})
	if Cells(nil, 2, 2) != nil {
		t.Error("Cells(nil) devolvió algo")
	}
	if Cells(img, 0, 2) != nil {
		t.Error("Cells(w=0) devolvió algo")
	}
	if Cells(img, 2, 0) != nil {
		t.Error("Cells(h=0) devolvió algo")
	}
	if got := Cells(image.NewRGBA(image.Rect(0, 0, 0, 0)), 2, 2); got != nil {
		t.Error("Cells de una imagen vacía devolvió algo")
	}
}

func TestCellsSurvivesUpscaling(t *testing.T) {
	lines := Cells(solid(1, 1, color.RGBA{G: 77, A: 255}), 8, 4)
	if len(lines) != 4 {
		t.Fatalf("lines = %d, want 4", len(lines))
	}
	if n := strings.Count(lines[0], "38;2;0;77;0"); n != 8 {
		t.Errorf("el color del único píxel aparece %d veces, want 8 (una por celda)", n)
	}
}

func TestErrorUnwrapsAndNamesTheCommand(t *testing.T) {
	cause := errors.New("boom")
	err := &Error{Args: []string{"merge", "x"}, Dir: "/wt", ExitCode: 3, Msg: "nope", Err: cause}

	msg := err.Error()
	for _, want := range []string{"git-sim", "/wt", "merge x", "nope", "exit 3"} {
		if !strings.Contains(msg, want) {
			t.Errorf("Error() = %q, no menciona %q", msg, want)
		}
	}
	if !errors.Is(err, cause) {
		t.Error("Error no preserva la causa con Unwrap")
	}
}

func asError(err error, target **Error) bool {
	for err != nil {
		if e, ok := err.(*Error); ok {
			*target = e
			return true
		}
		u, ok := err.(interface{ Unwrap() error })
		if !ok {
			return false
		}
		err = u.Unwrap()
	}
	return false
}
