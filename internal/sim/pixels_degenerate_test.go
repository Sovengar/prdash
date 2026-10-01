package sim

import (
	"image"
	"image/color"
	"testing"
)

// TestRgbaDevuelveNilParaLoQueNoSePuedePromediar: rgba es pura y su contrato es
// "nil o una imagen del tamaño de la original".
//
// Lo que protege son tres casos distintos, y cada uno con su motivo:
//
//   - nil: pedirle los bounds a una imagen inexistente es un PANIC, no un error. Un
//     panic al promediar se lleva la TUI entera, y el camino lo llega un simulator
//     que no devolvió imagen.
//   - tamaño cero en las dos dimensiones: no hay nada que dibujar. Y devolver una
//     imagen de 0 en vez de nil hace que el que la usa entre en `shrink` con un
//     origen sin píxeles, y de ahí sale una imagen con ruido en vez de ninguna.
//   - tamaño cero en UNA sola: igual, y es el caso que se confunde con el otro. Una
//     imagen de 4×0 tiene área cero, pero `Dx() > 0`, así que un "< 0" en vez de un
//     "<= 0" la deja pasar y construye un rectángulo degenerado.
func TestRgbaDevuelveNilParaLoQueNoSePuedePromediar(t *testing.T) {
	// Nil primero: es el que revienta si no se comprueba.
	if got := rgba(nil); got != nil {
		t.Errorf("rgba(nil) dio una imagen de %v: pedirle los bounds a nil es un panic", got.Bounds())
	}

	// Las tres formas de "no hay imagen", y el borde de UNA dimensión.
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

	// Y la que sí se puede: devuelve una del MISMO tamaño, con los píxeles puestos.
	origen := image.NewRGBA(image.Rect(0, 0, 3, 2))
	origen.Set(1, 0, color.RGBA{R: 10, G: 20, B: 30, A: 255})
	got := rgba(origen)
	if got == nil {
		t.Fatal("rgba de una imagen de verdad dio nil")
	}
	if b := got.Bounds(); b.Dx() != 3 || b.Dy() != 2 {
		t.Errorf("rgba devolvió %v, want 3x2: el tamaño es la mitad del contrato", b)
	}
	// Los bounds en el origen, que es lo que hace que `shrink` pueda indexar sin
	// sumar el Min. Está dicho en el comentario de shrink, y es una/aridad del
	// contrato: un RGBA con el Min distinto obligaría a sumar en cada píxel.
	if got.Bounds().Min != (image.Point{}) {
		t.Errorf("rgba devolvió los bounds en %v, want el origen: shrink indexa sin sumar el Min", got.Bounds().Min)
	}
	// Y el píxel está donde tocaba, o sea que se copió de verdad y no hay un relleno
	// negro por el camino.
	r, g, b, _ := got.At(1, 0).RGBA()
	if r>>8 != 10 || g>>8 != 20 || b>>8 != 30 {
		t.Errorf("el píxel (1,0) quedó en (%d,%d,%d), want (10,20,30)", r>>8, g>>8, b>>8)
	}

	// Y una imagen que no es RGBA se normaliza: es lo que hace la función existir,
	// porque promediar necesita leer por índice.
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

// TestFitCellsConAreaDeCeroNoInventaColumnas: un área de cero filas no da una
// columna.
//
// Es la diferencia entre las dos ramas del guard de entrada, y merece un caso propio:
//
//   - con imagen y maxRows == 0, la función devuelve (0, 0): no hay filas, no hay
//     nada que colocar, y una columna sola sería una imagen de una celda en un popup
//     de cero filas.
//   - si la guarda de maxRows desapareciera, la aritmética de abajo daría 0 filas ×
//     columnas por fila, y el suelo de "mínimo una columna" la convertiría en 1. Una
//     columna de más en un área de cero, que es exactamente lo que no debe pasar.
//
// El suelo de una columna existe para el caso normal (una imagen alta y estrecha en un
// área enana da menos de una columna por fila, y una columna es lo mínimo dibujable),
// no para el área vacía.
func TestFitCellsConAreaDeCeroNoInventaColumnas(t *testing.T) {
	img := solid(16, 9, black)

	// Área de cero filas: nada de nada.
	for _, a := range []medidas{{40, 0}, {1, 0}, {0, 0}, {40, -3}} {
		cols, rows := FitCells(img, 1, 2, a.w, a.h)
		if cols != max(a.w, 0) || rows != max(a.h, 0) {
			t.Errorf("con un área de %dx%d dio %dx%d, want %dx%d: sin filas no hay nada que colocar",
				a.w, a.h, cols, rows, max(a.w, 0), max(a.h, 0))
		}
	}
	// Y con las dos dimensiones no positivas, tampoco: un popup de 0×0 es un popup
	// que no se pinta.
	if cols, rows := FitCells(img, 1, 2, -5, -7); cols != 0 || rows != 0 {
		t.Errorf("con un área negativa dio %dx%d, want 0x0: un tamaño negativo no es un tamaño", cols, rows)
	}

	// Y el caso bueno: con filas de verdad, la relación manda y salen las dos.
	cols, rows := FitCells(img, 1, 2, 40, 10)
	if cols <= 0 || rows <= 0 {
		t.Errorf("con un área de verdad dio %dx%d, want algo en las dos dimensiones", cols, rows)
	}
}

// TestFitCellsConUnaDimensionCeroDaElAreaEntera: una imagen sin una de sus
// dimensiones no se puede relacionar con nada.
//
// El caso de 4×0 es el que separa las dos guardas: tiene área cero, pero `Dx() > 0`,
// así que un `Dx() <= 0` con un "menor" en vez de un "menor o igual" la deja pasar. Y
// lo que sale de ahí no es un descuadre pequeño: `Dx/Dy` con Dy en cero es infinito, y
// la aritmética de abajo acabaría devolviendo una fila y muchas columnas, es decir
// una imagen estirada en un popup que tiene sitio de sobra.
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

// TestFitCellsLasDosRamasCoincidenEnLaFrontera: en la frontera, las dos ramas dan la
// MISMA pareja, y un paso a cada lado divergen en una fila.
//
// El guard es `<=`, y en la frontera da igual cuál de las dos ramas se tome: por la de
// arriba sale `(maxRows*perRow, maxRows)`, y por la de abajo `(maxCols, maxCols/perRow)`,
// y con `maxRows*perRow == maxCols` las dos parejas son la misma. Eso es lo que hace
// que un `<` en vez de un `<=` no cambie nada, y aquí está demostrado con números en vez
// deständido en el allowlist.
//
// Y lo de al lado también importa, porque es lo que dice que la frontera es una
// frontera y no un tramo ancho donde da igual: con una columna MENOS ya manda el
// ancho, y el alto baja en uno. Con una fila MÁS pasa lo mismo. La coincidencia es de
// un punto, no de un rango.
func TestFitCellsLasDosRamasCoincidenEnLaFrontera(t *testing.T) {
	// Relación de columnas por fila ENTERA, para que la frontera caiga exacta y no
	// por redondeo: una imagen de 4x1 con celdas 1x2 necesita 8 columnas por fila.
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

		// Y la pareja que daría la otra rama, calculada a mano, es la misma. Eso es
		// el mutante de `<=` a `<` en persona: las dos ramas coinciden aquí.
		if otra := max(int(float64(maxCols)/perRow), 1); otra != rows {
			t.Errorf("en la frontera la otra rama daría %d filas y la elegida %d: "+
				"no coinciden y el mutante del guard se vería", otra, rows)
		}

		// Una columna menos: ya manda el ancho, y el alto baja en uno. Es el lado
		// izquierdo de la frontera.
		//
		// Con una sola fila, en cambio, se queda en 1: el suelo de una fila es lo
		// que impide que la imagen desaparezca, y una imagen de 0 filas no se
		// dibuja. Por eso el caso empieza en 2 filas, y no es unaгодня: es que en
		// una fila la frontera no está donde parece.
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

		// Una fila más con el mismo ancho: también manda el ancho, y el alto son
		// las mismas filas que antes. Es el lado derecho de la frontera.
		if c2, r2 := FitCells(img, 1, 2, maxCols, maxRows+1); r2 != maxRows {
			t.Errorf("con %d filas y %d columnas dio %dx%d, want %d filas: "+
				"una fila más y ya no cabe en el ancho",
				maxRows+1, maxCols, c2, r2, maxRows)
		}
	}
}
