package sim

import (
	"image"
	"image/color"
	"testing"
)

// rgba is pure and its contract is "nil or an image of at least one dimension".
func TestRgbaDevuelveNilParaLoQueNoSePuedePromediar(t *testing.T) {
	if got := rgba(nil); got != nil {
		t.Errorf("rgba(nil) dio una imagen de %v: pedirle los bounds a nil es un panic", got.Bounds())
	}

	for _, c := range []struct {
		nombre string
		img    image.Image
	}{
		{"0x0", image.NewRGBA(image.Rect(0, 0, 0, 0))},
		{"0x5", image.NewRGBA(image.Rect(0, 0, 0, 5))},
		{"5x0", image.NewRGBA(image.Rect(0, 0, 5, 0))},
		{"rejilla vacía", image.NewRGBA(image.Rect(4, 4, 4, 4))},
	} {
		if got := rgba(c.img); got != nil {
			t.Errorf("rgba de una imagen %s dio una imagen de %v, want nil: no hay píxeles que promediar",
				c.nombre, got.Bounds())
		}
	}

	origen := image.NewRGBA(image.Rect(0, 0, 3, 2))
	origen.Set(1, 0, color.RGBA{R: 10, G: 20, B: 30, A: 255})
	got := rgba(origen)
	if got == nil {
		t.Fatal("rgba de una imagen de verdad dio nil")
	}
	if b := got.Bounds(); b.Dx() != 3 || b.Dy() != 2 {
		t.Errorf("rgba devolvió %v, want 3x2: el tamaño es la mitad del contrato", b)
	}
	if got.Bounds().Min != (image.Point{}) {
		t.Errorf("rgba devolvió los bounds en %v, want el origen: shrink indexa sin sumar el Min", got.Bounds().Min)
	}
	// And the pixel is where it should be, so it really was copied and not filled with black.
	r, g, b, _ := got.At(1, 0).RGBA()
	if r>>8 != 10 || g>>8 != 20 || b>>8 != 30 {
		t.Errorf("el píxel (1,0) quedó en (%d,%d,%d), want (10,20,30)", r>>8, g>>8, b>>8)
	}

	// A non-RGBA image is normalised, which is what makes the function exist.
	other := image.NewGray(image.Rect(0, 0, 2, 2))
	other.Set(0, 0, color.Gray{Y: 200})
	got = rgba(other)
	if got == nil {
		t.Fatal("rgba de una imagen Gray dio nil: no hay motivo para no poder leerla")
	}
	if _, _, _, a := got.At(0, 0).RGBA(); a == 0 {
		t.Error("el píxel copiado es transparente: la conversión no tocó el contenido")
	}
}

// An area of zero rows does not yield a column.
func TestFitCellsConAreaDeCeroNoInventaColumnas(t *testing.T) {
	img := solid(16, 9, black)

	for _, a := range []medidas{{40, 0}, {1, 0}, {0, 0}, {40, -3}} {
		cols, rows := FitCells(img, 1, 2, a.w, a.h)
		if cols != max(a.w, 0) || rows != max(a.h, 0) {
			t.Errorf("con un área de %dx%d dio %dx%d, want %dx%d: sin filas no hay nada que colocar",
				a.w, a.h, cols, rows, max(a.w, 0), max(a.h, 0))
		}
	}
	// Neither dimension positive, then neither: a 0x0 popup is a popup that cannot be drawn.
	if cols, rows := FitCells(img, 1, 2, -5, -7); cols != 0 || rows != 0 {
		t.Errorf("con un área negativa dio %dx%d, want 0x0: un tamaño negativo no es un tamaño", cols, rows)
	}

	cols, rows := FitCells(img, 1, 2, 40, 10)
	if cols <= 0 || rows <= 0 {
		t.Errorf("con un área de verdad dio %dx%d, want algo en las dos dimensiones", cols, rows)
	}
}

// An image without one of its dimensions cannot be related to anything.
func TestFitCellsConUnaDimensionCeroDaElAreaEntera(t *testing.T) {
	casos := []struct {
		nombre string
		img    image.Image
	}{
		{"4x0", image.NewRGBA(image.Rect(0, 0, 4, 0))},
		{"0x4", image.NewRGBA(image.Rect(0, 0, 0, 4))},
		{"0x0", image.NewRGBA(image.Rect(0, 0, 0, 0))},
	}
	for _, c := range casos {
		for _, a := range []medidas{{40, 10}, {10, 40}, {1, 1}} {
			cols, rows := FitCells(c.img, 1, 2, a.w, a.h)
			if cols != a.w || rows != a.h {
				t.Errorf("imagen %s con área %dx%d dio %dx%d, want el área entera: "+
					"sin una dimensión no hay relación que calcular",
					c.nombre, a.w, a.h, cols, rows)
			}
		}
	}
}

// At the boundary both branches give the SAME pair, and a step either way gives the other.
func TestFitCellsLasDosRamasCoincidenEnLaFrontera(t *testing.T) {
	img := solid(4, 1, black)
	const perRow = 8

	for maxRows := 1; maxRows <= 12; maxRows++ {
		maxCols := maxRows * perRow

		// En la frontera: alto*perRow == maxCols, y el alto manda.
		cols, rows := FitCells(img, 1, 2, maxCols, maxRows)
		if cols != maxCols || rows != maxRows {
			t.Errorf("en la frontera (%d filas, %d columnas) dio %dx%d, want %dx%d",
				maxRows, maxCols, cols, rows, maxCols, maxRows)
		}

		// The pair the other branch would give, computed by hand, is the same: that is what kills the
		// mutation.
		if otra := max(int(float64(maxCols)/perRow), 1); otra != rows {
			t.Errorf("en la frontera la otra rama daría %d filas y la elegida %d: "+
				"no coinciden y el mutante del guard se vería", otra, rows)
		}

		// One column less: the width takes over and the height drops by one.
		if maxRows >= 2 {
			if c1, r1 := FitCells(img, 1, 2, maxCols-1, maxRows); r1 != maxRows-1 {
				t.Errorf("con %d filas y %d columnas dio %dx%d, want %d filas: "+
					"una columna menos y ya manda el ancho",
					maxRows, maxCols-1, c1, r1, maxRows-1)
			}
		}
		if _, r1 := FitCells(img, 1, 2, maxCols-1, 1); r1 != 1 {
			t.Errorf("con una sola fila y %d columnas dio %d filas, want 1: "+
				"el suelo de una fila impide que la imagen desaparezca", maxCols-1, r1)
		}

		if c2, r2 := FitCells(img, 1, 2, maxCols, maxRows+1); r2 != maxRows {
			t.Errorf("con %d filas y %d columnas dio %dx%d, want %d filas: "+
				"una fila más y ya no cabe en el ancho",
				maxRows+1, maxCols, c2, r2, maxRows)
		}
	}
}
