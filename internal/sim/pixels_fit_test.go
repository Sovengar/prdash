package sim

import (
	"image"
	"image/color"
	"strconv"
	"testing"
)

type medidas struct{ w, h int }

// Two invariants and both matter: the box is NEVER exceeded, and the aspect ratio is respected.
func TestFitCellsRespetaLaCajaYUsaLaDimensionQueManda(t *testing.T) {
	formas := []medidas{{16, 9}, {9, 16}, {10, 10}, {100, 3}, {3, 100}, {1, 1}, {1920, 1080}}
	celdas := []medidas{{1, 2}, {9, 19}, {1, 1}, {2, 1}, {8, 16}}
	areas := []medidas{{40, 10}, {10, 40}, {20, 20}, {1, 1}, {200, 60}, {5, 3}, {3, 5}}

	for _, f := range formas {
		img := solid(f.w, f.h, color.RGBA{R: 1, G: 2, B: 3, A: 255})
		for _, c := range celdas {
			for _, a := range areas {
				cols, rows := FitCells(img, c.w, c.h, a.w, a.h)
				nombre := "img " + itoa(f.w) + "x" + itoa(f.h) + ", celda " + itoa(c.w) + "x" + itoa(c.h) +
					", area " + itoa(a.w) + "x" + itoa(a.h)

				if cols > a.w || rows > a.h {
					t.Errorf("%s: dio %dx%d, want <= %dx%d: se sale del popup",
						nombre, cols, rows, a.w, a.h)
					continue
				}
				if cols < 1 || rows < 1 {
					t.Errorf("%s: dio %dx%d, want al menos 1x1: un popup de cero celdas no enseña nada",
						nombre, cols, rows)
					continue
				}
				if cols < a.w && rows < a.h {
					t.Errorf("%s: dio %dx%d y el área es %dx%d: sobró sitio en las dos dimensiones, "+
						"y se eligió el mayor tamaño que cabe", nombre, cols, rows, a.w, a.h)
				}
			}
		}
	}
}

// The ratio that decides how many columns per row the image needs is the image's OVER the cell's,
// not the reverse.
func TestFitCellsConservaLaRelacionDeAspecto(t *testing.T) {
	cuadrada := solid(100, 100, black)
	cols, rows := FitCells(cuadrada, 1, 2, 100, 10)
	if cols != 20 || rows != 10 {
		t.Errorf("cuadrada en celdas 1x2 dio %dx%d, want 20x10: el alto manda (10 líneas) y cada línea necesita 2 columnas", cols, rows)
	}
	// Square image, 9x19 cells: one cell is much taller than wide, so it takes 19/9 = 2.11 columns per
	//row for the image to come out square.
	cols, rows = FitCells(cuadrada, 9, 19, 100, 10)
	if cols != 21 || rows != 10 {
		t.Errorf("cuadrada en celdas 9x19 dio %dx%d, want 21x10: 19/9 = 2,11 columnas por línea", cols, rows)
	}
	panoramica := solid(160, 90, black)
	cols, rows = FitCells(panoramica, 1, 2, 80, 20)
	if cols != 71 || rows != 20 {
		t.Errorf("16:9 en celdas 1x2 dio %dx%d, want 71x20: 3,56 columnas por línea x 20 líneas = 71, que cabe en 80", cols, rows)
	}
	// With fewer rows the WIDTH takes over: the same calculation from the other side, which is why both
	//cuts have to be right.
	cols, rows = FitCells(panoramica, 1, 2, 80, 30)
	if cols != 80 || rows != 22 {
		t.Errorf("16:9 en celdas 1x2 con 30 filas dio %dx%d, want 80x22: ya no caben las 107 columnas y manda el ancho", cols, rows)
	}
	vertical := solid(90, 160, black)
	cols, rows = FitCells(vertical, 1, 2, 80, 20)
	if rows != 20 || cols != 22 {
		t.Errorf("9:16 en celdas 1x2 dio %dx%d, want 22x20: manda el alto y cada línea necesita 1,125 columnas", cols, rows)
	}
}

// The three degradations. None may break the fit, and each has a reason.
func TestFitCellsCaeADefaultsSinRomper(t *testing.T) {
	img := solid(16, 9, black)

	for _, a := range []medidas{{40, 10}, {0, 10}, {40, 0}, {0, 0}, {-5, -7}} {
		cols, rows := FitCells(nil, 1, 2, a.w, a.h)
		if want, wantR := max(a.w, 0), max(a.h, 0); cols != want || rows != wantR {
			t.Errorf("sin imagen y área %dx%d dio %dx%d, want %dx%d: un tamaño negativo se cuela en el layout",
				a.w, a.h, cols, rows, want, wantR)
		}
	}

	vacia := image.NewRGBA(image.Rect(0, 0, 0, 0))
	for _, a := range []medidas{{40, 10}, {1, 1}} {
		if cols, rows := FitCells(vacia, 1, 2, a.w, a.h); cols != a.w || rows != a.h {
			t.Errorf("imagen de 0x0 con área %dx%d dio %dx%d, want el área entera", a.w, a.h, cols, rows)
		}
	}

	wantCols, wantRows := FitCells(img, 1, 2, 40, 10)
	for _, c := range []medidas{{0, 0}, {0, 2}, {1, 0}, {-1, -1}} {
		cols, rows := FitCells(img, c.w, c.h, 40, 10)
		if cols != wantCols || rows != wantRows {
			t.Errorf("celda %dx%d dio %dx%d, want %dx%d (el default 1x2): sin medida de celda no se puede dividir",
				c.w, c.h, cols, rows, wantCols, wantRows)
		}
	}
	// A zero in ONE of the two does not drag the other: 0x5 is 1x5, not 1x2.
	wantC5, wantR5 := FitCells(img, 1, 5, 40, 10)
	if c0, r0 := FitCells(img, 0, 5, 40, 10); c0 != wantC5 || r0 != wantR5 {
		t.Errorf("celda 0x5 dio %dx%d, want %dx%d (1x5): un cero en el ancho no toca el alto declarado", c0, r0, wantC5, wantR5)
	}
	if wantC5 == wantCols && wantR5 == wantRows {
		t.Error("1x5 y 1x2 dieron el mismo tamaño: el alto de celda no entra en la relación, que es justo lo que se afirma")
	}
}

func TestFitEsFitCellsConLaCeldaDeSiempre(t *testing.T) {
	for _, f := range []medidas{{16, 9}, {9, 16}, {10, 10}, {1920, 1080}} {
		img := solid(f.w, f.h, black)
		for _, a := range []medidas{{40, 10}, {10, 40}, {1, 1}} {
			gotC, gotR := Fit(img, a.w, a.h)
			wantC, wantR := FitCells(img, 1, 2, a.w, a.h)
			if gotC != wantC || gotR != wantR {
				t.Errorf("Fit y FitCells(1,2) difieren con img %dx%d y área %dx%d: %dx%d vs %dx%d",
					f.w, f.h, a.w, a.h, gotC, gotR, wantC, wantR)
			}
		}
	}
}

// Box average, and in the one case where the truth is known without arithmetic the result equals it.
func TestResizePrometeElColorDeUnColorUnico(t *testing.T) {
	rojo := color.RGBA{R: 200, G: 10, B: 20, A: 255}
	for _, f := range []medidas{{1, 1}, {2, 2}, {64, 64}, {640, 480}} {
		for _, d := range []medidas{{1, 1}, {2, 3}, {16, 9}, {100, 100}, {3, 1}} {
			out := Resize(solid(f.w, f.h, rojo), d.w, d.h)
			if out == nil {
				t.Fatalf("Resize(%dx%d -> %dx%d) devolvió nil", f.w, f.h, d.w, d.h)
			}
			if got := out.Bounds(); got.Dx() != d.w || got.Dy() != d.h {
				t.Errorf("Resize(%dx%d -> %dx%d) dio un mapa de %dx%d", f.w, f.h, d.w, d.h, got.Dx(), got.Dy())
				continue
			}
			for y := range d.h {
				for x := range d.w {
					if got := out.RGBAAt(x, y); got != rojo {
						t.Fatalf("Resize(%dx%d -> %dx%d): el píxel (%d,%d) = %+v, want %+v",
							f.w, f.h, d.w, d.h, x, y, got, rojo)
					}
				}
			}
		}
	}
}

// A rescale that averages red well and blue badly yields a colour that is not in the source image.
func TestResizePromediaLosCanalesYNoSoloUno(t *testing.T) {
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
		t.Fatal("Resize devolvió nil")
	}
	got := out.RGBAAt(0, 0)
	want := color.RGBA{
		R: uint8((0 + 100 + 0 + 200) / 4),
		G: uint8((0 + 0 + 0 + 200) / 4),
		B: uint8((0 + 0 + 100 + 200) / 4),
		A: 255,
	}
	if got != want {
		t.Errorf("promedio de 2x2 a 1x1 = %+v, want %+v (cada canal entre el mismo número de píxeles)", got, want)
	}

	out = Resize(img, 2, 1)
	if out == nil {
		t.Fatal("Resize devolvió nil")
	}
	if izq := out.RGBAAt(0, 0); izq != (color.RGBA{R: 0, G: 0, B: 50, A: 255}) {
		t.Errorf("la mitad izquierda = %+v, want el promedio de los dos píxeles de la columna 0 (negro y azul)", izq)
	}
	if der := out.RGBAAt(1, 0); der != (color.RGBA{R: 150, G: 100, B: 100, A: 255}) {
		t.Errorf("la mitad derecha = %+v, want el promedio de los dos píxeles de la columna 1 (rojo y gris)", der)
	}
}

func TestResizeSinSalidaNiSinImagenDevuelveNil(t *testing.T) {
	img := solid(4, 4, color.RGBA{R: 1, A: 255})
	for _, d := range []medidas{{0, 4}, {4, 0}, {0, 0}, {-1, 4}, {4, -1}} {
		if out := Resize(img, d.w, d.h); out != nil {
			t.Errorf("Resize a %dx%d devolvió una imagen de %v, want nil", d.w, d.h, out.Bounds())
		}
	}
	for _, d := range []medidas{{4, 4}, {1, 1}} {
		if out := Resize(nil, d.w, d.h); out != nil {
			t.Errorf("Resize(nil, %dx%d) devolvió una imagen, want nil", d.w, d.h)
		}
	}
}

// Bounds does not only say the size, it says WHERE the image starts: a cropped PNG is half an
// image returned by git-sim.
func TestResizeRespetaElOrigenDeLosBounds(t *testing.T) {
	// A big image with two very different halves and a crop that falls EXACTLY between them: an
	//absolute-coordinate reader would take only one.
	rojo := color.RGBA{R: 250, G: 0, B: 0, A: 255}
	verde := color.RGBA{R: 0, G: 250, B: 0, A: 255}
	fondo := image.NewRGBA(image.Rect(0, 0, 100, 100))
	for y := range 100 {
		for x := range 100 {
			if x < 50 {
				fondo.SetRGBA(x, y, rojo)
			} else {
				fondo.SetRGBA(x, y, verde)
			}
		}
	}
	recorte := fondo.SubImage(image.Rect(30, 30, 70, 70))
	if got := recorte.Bounds().Min; got.X != 30 || got.Y != 30 {
		t.Fatalf("el recorte debería tener Min = (30,30), dio %v", got)
	}

	want := color.RGBA{R: 125, G: 125, B: 0, A: 255}
	if got := Resize(recorte, 1, 1).RGBAAt(0, 0); got != want {
		t.Errorf("el recorte a caballo a 1x1 dio %+v, want %+v: leyó la imagen original en vez del recorte", got, want)
	}
	out := Resize(recorte, 40, 10)
	if out == nil {
		t.Fatal("Resize devolvió nil")
	}
	if got := out.RGBAAt(0, 0); got.R < got.G {
		t.Errorf("la izquierda del recorte dio %+v, want más rojo que verde", got)
	}
	if got := out.RGBAAt(39, 0); got.G < got.R {
		t.Errorf("la derecha del recorte dio %+v, want más verde que rojo", got)
	}

	plano := solid(100, 100, verde)
	sub := plano.SubImage(image.Rect(30, 30, 70, 70))
	for _, d := range []medidas{{1, 1}, {8, 6}, {40, 10}} {
		got := Resize(sub, d.w, d.h)
		if got == nil {
			t.Fatalf("Resize devolvió nil")
		}
		for y := range d.h {
			for x := range d.w {
				if c := got.RGBAAt(x, y); c != verde {
					t.Fatalf("el recorte sólido a %dx%d dio %+v en (%d,%d), want %+v: leyó la imagen original en vez del recorte",
						d.w, d.h, c, x, y, verde)
				}
			}
		}
	}
}

func itoa(n int) string { return strconv.Itoa(n) }
