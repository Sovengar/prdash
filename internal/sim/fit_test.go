package sim

import (
	"image"
	"image/color"
	"testing"
)

// black es el color de fondo de los fixtures: da igual, el ratio no lo mira.
var black = color.RGBA{A: 255}

// TestFitKeepsTheAspectOfAWideImage: una celda es el doble de alta que de ancha,
// así que una imagen 16:9 necesita 3,56 columnas por línea. Sin ese factor, el
// grafo se estiraría a lo ancho y los dos commits de una fila se verían como una
// tira de elipses en vez de dos círculos.
func TestFitKeepsTheAspectOfAWideImage(t *testing.T) {
	img := solid(1920, 1080, black)

	cols, rows := Fit(img, 400, 40)
	if rows != 40 {
		t.Errorf("rows = %d, want 40: manda el alto", rows)
	}
	// 40 filas * 3,56 columnas por fila ≈ 142 columnas.
	if cols < 138 || cols > 146 {
		t.Errorf("cols = %d, want ~142 (3,56 columnas por fila para 16:9)", cols)
	}
}

// TestFitPrefersTheWidthWhenItBinds: con una terminal ancha y baja manda la
// anchura, y la imagen no debe desbordar hacia abajo.
func TestFitPrefersTheWidthWhenItBinds(t *testing.T) {
	img := solid(1920, 1080, black)

	cols, rows := Fit(img, 100, 400)
	if cols != 100 {
		t.Errorf("cols = %d, want 100: manda la anchura", cols)
	}
	if rows >= 400 {
		t.Errorf("rows = %d, want menos que el máximo disponible", rows)
	}
	// 100 columnas a 3,56 por fila ≈ 28 filas.
	if rows < 25 || rows > 31 {
		t.Errorf("rows = %d, want ~28", rows)
	}
}

// TestFitNeverExceedsTheArea: es la garantía que importa, porque si el ajuste
// devolviera más de lo que cabe, el popup se saldría de la terminal.
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

// TestFitDegradesOnAnUnusableInput: sin imagen no hay ratio que respetar, así que
// se devuelve el área tal cual y el popup sigue teniendo algo que dibujar.
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
