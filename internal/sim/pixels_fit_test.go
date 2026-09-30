package sim

import (
	"image"
	"image/color"
	"strconv"
	"testing"
)

// medidas es un par de dimensiones (ancho x alto), que en este fichero son siempre
// lo mismo: un píxel, una celda o un área de celdas.
type medidas struct{ w, h int }

// TestFitCellsRespetaLaCajaYUsaLaDimensionQueManda: el encaje de una imagen en el
// popup tiene dos invariantes, y las dos importan:
//
//   - NUNCA se pasa del área disponible. Un cols/rows que se pase dibuja fuera
//     del popup, encima del inbox, y es el borde del popup lo que deja ver que la
//     imagen termina.
//   - La dimensión que manda se USA ENTERA. Se elige el mayor tamaño que cabe en
//     lugar de rellenar el área, pero "mayor" significa que la que manda llega al
//     tope: si el alto es el que manda, rows == maxRows; si es el ancho,
//     cols == maxCols. Quedarse corto sin motivo deja una celda vacía al lado de
//     la imagen, que es justo la señal de que la imagen acaba ahí, y se pierde.
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

				// Nunca por encima del área, y nunca en cero: un popup de cero
				// celdas es un popup que no enseña la imagen.
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
				// La dimensión que manda llega al tope. Con la relación de
				// aspecto preservada, la que se queda corta es la que no llenaba.
				if cols < a.w && rows < a.h {
					t.Errorf("%s: dio %dx%d y el área es %dx%d: sobró sitio en las dos dimensiones, "+
						"y se eligió el mayor tamaño que cabe", nombre, cols, rows, a.w, a.h)
				}
			}
		}
	}
}

// TestFitCellsConservaLaRelacionDeAspecto: la relación que decide cuántas columnas
// por línea necesita la imagen es la de la imagen POR LA de la celda, no la de la
// celda sola. Por eso existe FitCells: suponer 1×2 cuando el terminal tiene celdas
// de 9×19 px introduce un error de un 5% en el tamaño, que es justo el tipo de
// error que hace que algo "casi cuadre" y no se sepa por qué.
//
// El caso que delata una relación mal puesta es el CUADRADO con celdas altas: una
// imagen cuadrada en celdas de 1×2 necesita el doble de columnas que de filas.
func TestFitCellsConservaLaRelacionDeAspecto(t *testing.T) {
	// Cuadrada, celdas 1×2 (el default de toda la vida).
	cuadrada := solid(100, 100, black)
	cols, rows := FitCells(cuadrada, 1, 2, 100, 10)
	if cols != 20 || rows != 10 {
		t.Errorf("cuadrada en celdas 1x2 dio %dx%d, want 20x10: el alto manda (10 líneas) y cada línea necesita 2 columnas", cols, rows)
	}
	// Cuadrada, celdas 9×19: una celda es mucho más alta que ancha, así que hacen
	// falta 19/9 = 2,11 columnas por línea para que la imagen quede cuadrada. Con
	// 10 líneas son 21 columnas de 100: el alto sigue mandando, pero el ancho
	// usado baja de 20 a 21. Si se olvidara el factor de la celda, saldrían 10
	// columnas y la imagen se vería estirada.
	cols, rows = FitCells(cuadrada, 9, 19, 100, 10)
	if cols != 21 || rows != 10 {
		t.Errorf("cuadrada en celdas 9x19 dio %dx%d, want 21x10: 19/9 = 2,11 columnas por línea", cols, rows)
	}
	// Panorámica 16:9 con celdas 1×2: pocas líneas, muchas columnas.
	panoramica := solid(160, 90, black)
	cols, rows = FitCells(panoramica, 1, 2, 80, 20)
	if cols != 71 || rows != 20 {
		t.Errorf("16:9 en celdas 1x2 dio %dx%d, want 71x20: 3,56 columnas por línea x 20 líneas = 71, que cabe en 80", cols, rows)
	}
	// Y con menos filas, el ANCHO pasa a mandar: es el mismo cálculo del otro
	// lado, y por eso los dos cortes tienen que estar bien.
	// Con 30 filas, 3,56 columnas por línea pedirían 107 columnas de las 80
	// disponibles: ya no caben y manda el ancho, que se llena entero y el alto
	// sale de la relación.
	cols, rows = FitCells(panoramica, 1, 2, 80, 30)
	if cols != 80 || rows != 22 {
		t.Errorf("16:9 en celdas 1x2 con 30 filas dio %dx%d, want 80x22: ya no caben las 107 columnas y manda el ancho", cols, rows)
	}
	// Y vertical 9:16: al revés, el alto manda con muy pocas columnas.
	vertical := solid(90, 160, black)
	cols, rows = FitCells(vertical, 1, 2, 80, 20)
	if rows != 20 || cols != 22 {
		t.Errorf("9:16 en celdas 1x2 dio %dx%d, want 22x20: manda el alto y cada línea necesita 1,125 columnas", cols, rows)
	}
}

// TestFitCellsCaeADefaultsSinRomper: los tres cortes de degradación. Ninguno puede
// romper el encaje, y cada uno tiene un motivo:
//
//   - sin imagen, o sin sitio: se devuelve el área, acotada a cero. Un cols/rows
//     negativo se cuela en el layout y descuadra todo lo de abajo.
//   - una imagen de tamaño cero no se puede encajar: se devuelve el área entera.
//   - sin medida de celda (una terminal que no la ha dicho todavía) se usan 1×2,
//     que es la relación de toda la vida. Confiar en un 0 sin defecto es dividir
//     entre cero y devolver un tamaño infinito o NaN que el render no sabe pintar.
func TestFitCellsCaeADefaultsSinRomper(t *testing.T) {
	img := solid(16, 9, black)

	// Sin imagen: el área, acotada a cero. Un máximo negativo NO se propaga.
	for _, a := range []medidas{{40, 10}, {0, 10}, {40, 0}, {0, 0}, {-5, -7}} {
		cols, rows := FitCells(nil, 1, 2, a.w, a.h)
		if want, wantR := max(a.w, 0), max(a.h, 0); cols != want || rows != wantR {
			t.Errorf("sin imagen y área %dx%d dio %dx%d, want %dx%d: un tamaño negativo se cuela en el layout",
				a.w, a.h, cols, rows, want, wantR)
		}
	}

	// Una imagen sin píxeles: el área entera, que es lo que cabe de una imagen que
	// no tiene nada que enseñar.
	vacia := image.NewRGBA(image.Rect(0, 0, 0, 0))
	for _, a := range []medidas{{40, 10}, {1, 1}} {
		if cols, rows := FitCells(vacia, 1, 2, a.w, a.h); cols != a.w || rows != a.h {
			t.Errorf("imagen de 0x0 con área %dx%d dio %dx%d, want el área entera", a.w, a.h, cols, rows)
		}
	}

	// Sin medida de celda: 1×2, la relación de siempre. Esto NO es lo mismo que
	// usar un 0, que daría un tamaño sin sentido.
	wantCols, wantRows := FitCells(img, 1, 2, 40, 10)
	for _, c := range []medidas{{0, 0}, {0, 2}, {1, 0}, {-1, -1}} {
		cols, rows := FitCells(img, c.w, c.h, 40, 10)
		if cols != wantCols || rows != wantRows {
			t.Errorf("celda %dx%d dio %dx%d, want %dx%d (el default 1x2): sin medida de celda no se puede dividir",
				c.w, c.h, cols, rows, wantCols, wantRows)
		}
	}
	// Y un cero en UNA de las dos no arrastra a la otra: 0×5 es 1×5, no 1×2. El
	// alto declarado se respeta tal cual, que es el caso donde un default
	// equivocado se nota: con celdas 1×5 una imagen de 16:9 necesita 5,56
	// columnas por línea y con celdas 1×2 solo 3,56.
	wantC5, wantR5 := FitCells(img, 1, 5, 40, 10)
	if c0, r0 := FitCells(img, 0, 5, 40, 10); c0 != wantC5 || r0 != wantR5 {
		t.Errorf("celda 0x5 dio %dx%d, want %dx%d (1x5): un cero en el ancho no toca el alto declarado", c0, r0, wantC5, wantR5)
	}
	if wantC5 == wantCols && wantR5 == wantRows {
		t.Error("1x5 y 1x2 dieron el mismo tamaño: el alto de celda no entra en la relación, que es justo lo que se afirma")
	}
}

// TestFitEsFitCellsConLaCeldaDeSiempre: Fit es la versión con la relación 1×2, y no
// un atajo con otro comportamiento. Es lo que llama el resto del código.
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

// TestResizePrometeElColorDeUnColorUnico: el reescalado es promedio de caja, y en
// el caso en el que se puede saber la verdad sin hacer cuentas, el resultado es la
// verdad: una imagen de un solo color, reescalada a lo que sea, sigue siendo ese
// color.
//
// Es lo que distingue un promedio de un "agarra el píxel de la esquina": con un
// degradado, agarrar una esquina produce una imagen con bandas, que es
// exactamente el defecto que hace que un reescalado parezca roto aunque compile.
// Y se comprueba TODOS los píxeles, no solo el primero: un promedio mal calculado
// se nota en el interior, que es donde está la imagen entera.
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

// TestResizePromediaLosCanalesYNoSoloUno: un reescalado que promedia bien el rojo y
// mal el azul da un color que no existe en la imagen de partida, y se ve como una
// aberración cromática en el gráfico reescalado. El promedio es por canal, con el
// MISMO divisor para los cuatro: dividir cada canal por un número distinto es el
// error clásico, y solo se nota con una imagen de colores dispares.
func TestResizePromediaLosCanalesYNoSoloUno(t *testing.T) {
	// Cuatro colores en 2×2, muy separados, para que un promedio a la baja de
	// cada canal se note y no se esconda en el redondeo.
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

	// A 1×1: los cuatro píxeles promedian. El alfa es 255 en todos, así que el
	// promedio también lo es: un divisor distinto para el alfa dejaría el
	// resultado translúcido o saturado.
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

	// A 2×1: dos píxeles de entrada por uno de salida, uno por columna. La suma
	// es la misma pero el divisor es 2, y por eso importa que el divisor sea el
	// número de píxeles de la CAJA y no el de la imagen entera.
	out = Resize(img, 2, 1)
	if out == nil {
		t.Fatal("Resize devolvió nil")
	}
	// A 2×1 cada píxel de salida promedia una COLUMNA de entrada: la izquierda,
	// el negro y el azul; la derecha, el rojo y el gris.
	if izq := out.RGBAAt(0, 0); izq != (color.RGBA{R: 0, G: 0, B: 50, A: 255}) {
		t.Errorf("la mitad izquierda = %+v, want el promedio de los dos píxeles de la columna 0 (negro y azul)", izq)
	}
	if der := out.RGBAAt(1, 0); der != (color.RGBA{R: 150, G: 100, B: 100, A: 255}) {
		t.Errorf("la mitad derecha = %+v, want el promedio de los dos píxeles de la columna 1 (rojo y gris)", der)
	}
}

// TestResizeSinSalidaNiSinImagenDevuelveNil: un tamaño no positivo o una imagen
// inexistente no se pueden promediar. Devolver una imagen vacía en vez de nil
// revienta en el consumidor, que la pasa a Herdr sin mirar.
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

// TestResizeRespetaElOrigenDeLosBounds: Bounds no solo dice el tamaño, dice DÓNDE
// empieza la imagen. Un PNG recortado (que es media imagen que devuelve git-sim)
// llega con Min distinto de (0,0), y un reescalado que leyera los píxeles por
// coordenada absoluta en vez de relativa al origen leería una región de la imagen
// original que no es esta.
//
// El síntoma es un gráfico reescalado con la parte equivocada, y solo aparece con
// recortes: con una imagen que empieza en el origen los dos caminos coinciden y
// la prueba pasa sin mirar nada.
func TestResizeRespetaElOrigenDeLosBounds(t *testing.T) {
	// Una imagen grande con dos mitades muy distintas, y un recorte que CAE A
	// CABALLO de las dos. Un lector por coordenada absoluta se quedaría con una
	// sola mitad; el correcto devuelve la mezcla, en la proporción que ocupa cada
	// mitad en el recorte.
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
	// El recorte: x de 30 a 70 (20 columnas de rojo y 20 de verde, a caballo de
	// la frontera que está en 50), y de 30 a 70.
	recorte := fondo.SubImage(image.Rect(30, 30, 70, 70))
	if got := recorte.Bounds().Min; got.X != 30 || got.Y != 30 {
		t.Fatalf("el recorte debería tener Min = (30,30), dio %v", got)
	}

	// El promedio del recorte entero es mitad y mitad: eso es lo que un lector
	// por coordenada absoluta NO puede dar (leería solo el rojo o solo el verde,
	// según por dónde empezara a leer).
	want := color.RGBA{R: 125, G: 125, B: 0, A: 255}
	if got := Resize(recorte, 1, 1).RGBAAt(0, 0); got != want {
		t.Errorf("el recorte a caballo a 1x1 dio %+v, want %+v: leyó la imagen original en vez del recorte", got, want)
	}
	// Y a un tamaño con detalle, la proporción se mantiene: más roja la mitad
	// izquierda del recorte, más verde la derecha.
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

	// Y el caso limpio, que delata al lector por coordenada absoluta sin
	// ambigüedad: un recorte de un color sólido, con origen décalado, sale de
	// ese color. En la imagen original, (0,0) es del OTRO cuadrante.
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
