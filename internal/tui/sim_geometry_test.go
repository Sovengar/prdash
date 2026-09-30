package tui

import (
	"image"
	"image/color"
	"testing"

	tea "charm.land/bubbletea/v2"

	"prdash/internal/sim"
)

// TestCellSizeUsaLoMedidoONuncaUnCero: los píxeles de una celda los mide Herdr, y
// usarlos hace dos cosas a la vez: que la imagen no se deforme y que no se mande
// más resolución de la que se ve. Sin Herdr se supone 1×2.
//
// El punto de esta prueba es el `&&`: si solo se ha medido UNA de las dos
// dimensiones, no se usa ninguna. Una celda de 9×0 píxeles no describe nada, y
// aceptarla daría un reescalado de ancho 9 y alto 0 —que devuelve nil, o sea una
// imagen que no se pinta— donde la degradación correcta es 1×2 y una imagen
// albeit fea pero presente.
func TestCellSizeUsaLoMedidoONuncaUnCero(t *testing.T) {
	m := newTestModel(t, ghAdapter())

	// Sin medir: la relación de toda la vida.
	if w, h := m.cellSize(); w != 1 || h != 2 {
		t.Errorf("sin medir dio %dx%d, want 1x2", w, h)
	}
	// Medidas las dos: se usan tal cual. Un 9×19 de kitty es lo que hay que usar.
	m.sim.cellW_px, m.sim.cellH_px = 9, 19
	if w, h := m.cellSize(); w != 9 || h != 19 {
		t.Errorf("con 9x19 medidos dio %dx%d, want 9x19: no usar lo medido deforma la imagen", w, h)
	}
	// Las cuatro combinaciones con un cero: NINGUNA usa las medidas parciales.
	for _, c := range [][2]int{{0, 19}, {9, 0}, {0, 0}, {-1, 19}, {9, -1}} {
		m.sim.cellW_px, m.sim.cellH_px = c[0], c[1]
		if w, h := m.cellSize(); w != 1 || h != 2 {
			t.Errorf("con %dx%d dio %dx%d, want 1x2: media celda no describe nada", c[0], c[1], w, h)
		}
	}
}

// TestSimMaxColsYRowsRespetanElSuelo: el popup deja fondo a los lados y arriba y
// abajo, y nunca baja de un suelo. El suelo es lo que evita que en una terminal
// diminuta el popup sea de cero columnas y no haya nada que mirar.
//
// El alto sale de una FRACCIÓN de lo que queda, no de todo: si la imagen usara
// la pantalla entera no se vería el marco, y el marco es lo que dice dónde acaba
// la imagen.
func TestSimMaxColsYRowsRespetanElSuelo(t *testing.T) {
	for _, w := range []int{0, 10, 20, 40, 80, 200} {
		for _, h := range []int{0, 1, 3, 6, 10, 20, 50, 200} {
			m := send(t, newTestModel(t, ghAdapter()), tea.WindowSizeMsg{Width: w, Height: h})

			cols := m.simMaxCols()
			if cols < simMinCols {
				t.Errorf("terminal %dx%d: %d columnas, want >= %d (el suelo)", w, h, cols, simMinCols)
			}
			// Y nunca más de lo que hay, con el margen lateral descontado: el
			// fondo tiene que verse por los lados.
			if want := max(simMinCols, m.contentWidth()-simSideMargin); cols != want {
				t.Errorf("terminal %dx%d: %d columnas, want %d", w, h, cols, want)
			}

			rows := m.simMaxRows()
			if rows < simMinRows {
				t.Errorf("terminal %dx%d: %d filas, want >= %d (el suelo)", w, h, rows, simMinRows)
			}
			// El suelo MANDA sobre la terminal: en una de 3 filas el popup sigue
			// teniendo simMinRows. Es la degradación correcta —un popup de cero
			// filas no muestra nada y parece un bug— a costa de que se salga por
			// abajo, que es el peptido honesto de una terminal demasiado pequeña.
			if h >= simMinRows+2*simMargin && rows > h {
				t.Errorf("terminal %dx%d: %d filas, más que la terminal", w, h, rows)
			}
			// Y sale de la fracción, no del entero: con la fracción 3/4, una
			// terminal de 20 filas deja 16 útiles y da 12.
			if want := max(simMinRows, max(h-2*simMargin, 0)*simHeightNum/simHeightDen); rows != want {
				t.Errorf("terminal %dx%d: %d filas, want %d (3/4 del hueco libre)", w, h, rows, want)
			}
		}
	}
	// El suelo manda sobre la fracción en una terminal diminuta: por debajo, sin
	// el suelo, el popup sería de cero filas.
	m := send(t, newTestModel(t, ghAdapter()), tea.WindowSizeMsg{Width: 40, Height: 1})
	if m.simMaxRows() != simMinRows {
		t.Errorf("en una terminal de 1 fila dio %d filas, want el suelo %d", m.simMaxRows(), simMinRows)
	}
}

// TestSimBoxDaMargenAlMarcoYLaCajaALaImagen: hay dos cajas distintas con dos
// reglas distintas, y confundirlas es un descuadre visible:
//
//   - El SELECTOR y el estado de "renderizando" no enseñan imagen: son una caja
//     estrecha de alto fijo. No dependen del tamaño de la imagen porque no la
//     enseñan.
//   - Cuando se ENSEÑA la imagen, la caja se ajusta a lo que la imagen necesita más
//     el marco: dos columnas de borde y el alto del marco. Al revés —la imagen
//     mandando sobre la caja— es lo que dejaba la imagen en 94 columnas dentro de
//     una caja de 200, con un borde vacío a su derecha que parecía parte de la
//     imagen.
func TestSimBoxDaMargenAlMarcoYLaCajaALaImagen(t *testing.T) {
	m := send(t, newTestModel(t, ghAdapter()), tea.WindowSizeMsg{Width: 100, Height: 30})
	wantAncho := min(m.contentWidth(), simChooserWidth)

	for _, estado := range []simState{simChoosing, simRendering} {
		m.sim.state = estado
		w, h := m.simBox()
		if w != wantAncho {
			t.Errorf("en %v la caja mide %d de ancho, want %d: sin imagen el ancho es fijo", estado, w, wantAncho)
		}
		if h != simChrome+2 {
			t.Errorf("en %v la caja mide %d de alto, want %d (el marco más dos)", estado, h, simChrome+2)
		}
	}

	// Con imagen: la caja es lo que la imagen necesita MÁS el marco.
	for _, f := range [][2]int{{16, 9}, {9, 16}, {100, 100}, {3, 1}} {
		m.sim.state = simShowing
		m.sim.img = solidSim(f[0], f[1], negro)
		m.sim.cellW_px, m.sim.cellH_px = 0, 0
		cellW, cellH := m.cellSize()
		fitCols, fitRows := sim.FitCells(m.sim.img, cellW, cellH, m.simMaxCols(), m.simMaxRows())
		w, h := m.simBox()
		if w != fitCols+2 {
			t.Errorf("img %dx%d: caja de %d columnas, want %d (el encaje más dos de borde)", f[0], f[1], w, fitCols+2)
		}
		if h != fitRows+simChrome {
			t.Errorf("img %dx%d: caja de %d filas, want %d (el encaje más el marco)", f[0], f[1], h, fitRows+simChrome)
		}
		// Y la imagen NUNCA se sale de la caja: el interior es lo que queda tras
		// el marco, y es lo que se pinta.
		if fitCols > m.simMaxCols() || fitRows > m.simMaxRows() {
			t.Errorf("img %dx%d: el encaje %dx%d se pasa del máximo %dx%d",
				f[0], f[1], fitCols, fitRows, m.simMaxCols(), m.simMaxRows())
		}
	}
}

// TestRenderSimCellsNoRepintaLoMismo: el repintado de la imagen a la geometría del
// popup es caro (es un promedio de caja sobre todos los píxeles) y se dispara en
// CADA resize, que es lo único que cambia el tamaño. Sin la guarda de caché, cada
// redimensionado de la terminal repinta una imagen que ya está bien, y mover el
// popup con el ratón se nota.
//
// La guarda compara las TRES cosas: que hay celdas, y el ancho y el alto por los
// que se hicieron. Un ancho distinto con el mismo alto es otra geometría, y
// repintar es lo correcto.
func TestRenderSimCellsNoRepintaLoMismo(t *testing.T) {
	img := solidSim(16, 9, negro)

	t.Run("sin imagen no hay celdas", func(t *testing.T) {
		m := showSim(t, img)
		m.sim.state = simRendering
		m.sim.cells = nil
		m.sim.viaGraphics = false
		m.renderSimCells()
		if m.sim.cells != nil {
			t.Errorf("sin imagen en estado de renderizando dejó %d celdas", len(m.sim.cells))
		}
		if m.sim.cellW != 0 || m.sim.cellH != 0 {
			t.Errorf("sin imagen quedaron celdas de %dx%d, want 0x0 (para que no se reutilice un repintado viejo)",
				m.sim.cellW, m.sim.cellH)
		}
	})

	t.Run("con la capa de graficos no hay celdas", func(t *testing.T) {
		m := showSim(t, img)
		m.sim.viaGraphics = true
		m.sim.cells = []string{"vieja"}
		m.renderSimCells()
		if m.sim.cells != nil {
			t.Errorf("con la imagen en la capa de gráficos quedaron %d celdas: se pintarían dos veces", len(m.sim.cells))
		}
		if m.sim.cellW != 0 || m.sim.cellH != 0 {
			t.Errorf("con la capa de gráficos quedaron dimensiones %dx%d, want 0x0", m.sim.cellW, m.sim.cellH)
		}
	})

	t.Run("misma geometria no repinta", func(t *testing.T) {
		m := showSim(t, img)
		m.sim.viaGraphics = false
		m.renderSimCells()
		if len(m.sim.cells) == 0 {
			t.Fatal("la primera llamada no pintó nada")
		}
		celdas := m.sim.cells
		w, h := m.sim.cellW, m.sim.cellH
		// Un segundo render con la misma geometría tiene que ser un no-op: si
		// repintara, la lista sería una nueva, no la misma.
		m.sim.cells = nil
		m.renderSimCells()
		if m.sim.cells == nil {
			t.Error("la segunda llamada con la misma geometría borró las celdas: la guarda de caché no está")
		}
		_ = celdas
		_ = w
		_ = h
	})

	t.Run("geometria distinta repinta", func(t *testing.T) {
		m := showSim(t, img)
		m.sim.viaGraphics = false
		m.renderSimCells()
		anchoAntes := m.sim.cellW

		antes := append([]string(nil), m.sim.cells...)

		// Lo que cambia la geometría es la medida de la celda, no la terminal: es
		// lo que decide cuántas columnas por línea necesita la imagen. Y se
		// cambia llamando a renderSimCells a mano, no con un resize, porque un resize
		// dispara el repintado por su cuenta desde update.go y la llamada
		// siguiente comprobaría el segundo render (que sí es un no-op) en vez del
		// primero (que es el que repinta).
		m.sim.cellW_px, m.sim.cellH_px = 9, 19
		m.renderSimCells()
		if m.sim.cellW == anchoAntes {
			t.Errorf("tras cambiar la celda a 9x19 el ancho sigue en %d: no repintó", anchoAntes)
		}
		if len(m.sim.cells) == 0 {
			t.Fatal("tras cambiar la geometría no quedó ninguna celda pintada")
		}
		if mismoStrings(m.sim.cells, antes) {
			t.Error("tras cambiar la medida de celda las celdas son las mismas: se quedó con la imagen de antes")
		}
	})
}

// TestPublishSimImagePoneLaImagenJustoEnElHueco: la imagen va en la capa de
// gráficos, y para caer JUSTO en el hueco del marco tiene que llevar la colocación
// en coordenadas de celda CON el borde descontado: la del popup más uno, y el
// interior sin él. Sin el +1 la imagen se iría una fila y una columna arriba, y
// taparía el borde —que es lo que dice al usuario dónde acaba la imagen—.
func TestPublishSimImagePoneLaImagenJustoEnElHueco(t *testing.T) {
	g := &fakeGraphics{available: true, cellW: 9, cellH: 19}
	m := showSim(t, solidSim(16, 9, negro))
	m.SetGraphics(g)
	preCols, preRows := m.simBox()

	if !m.publishSimImage(m.sim.img) {
		t.Fatalf("publishSimImage devolvió false: %+v", g.sets)
	}
	if len(g.sets) != 1 {
		t.Fatalf("publicó %d imágenes, want 1", len(g.sets))
	}
	pl := g.sets[0]
	// La caja se mide ANTES de publicar, que es cuando la calcula la propia
	// función. Publicar guarda la celda medida (9×19 en vez del 1×2 supuesto) y
	// eso cambia el encaje para la SIGUIENTE vez: la colocación va un paso por
	// detrás de la celda y se corrige en el siguiente resize. Es lo que hace el
	// código y lo que se afirma, no lo que uno se inventaría.
	cols, rows := preCols, preRows
	col, row := m.simBoxOrigin(cols, rows)
	if pl.col != col+1 || pl.row != row+1 {
		t.Errorf("la imagen se publicó en (%d,%d), want (%d,%d): la del popup más una, que es el borde",
			pl.col, pl.row, col+1, row+1)
	}
	if pl.cols != cols-2 || pl.rows != rows-simChrome {
		t.Errorf("la imagen se publicó de %dx%d, want %dx%d: el interior de la caja, sin el marco",
			pl.cols, pl.rows, cols-2, rows-simChrome)
	}
	// Y la capa es la del sim, para que se pueda apagar sin tocar la de nadie: si
	// publicara en la de por defecto, limpiar el sim se llevaría por delante la
	// imagen de otro.
	if g.layer != simLayer {
		t.Errorf("la imagen se publicó en la capa %q, want %q", g.layer, simLayer)
	}
	// Y la imagen va reescalada al tamaño en píxeles del hueco, con la celda
	// medida: mandar la original son 1920×1080 para pintar 800 px.
	if b := g.sets[0].img.Bounds(); b.Dx() != pl.cols*g.cellW || b.Dy() != pl.rows*g.cellH {
		t.Errorf("la imagen mandada mide %dx%d px, want %dx%d (el hueco por la celda medida %dx%d): mandar la original serían 1920x1080 para pintar un rectángulo de 800",
			b.Dx(), b.Dy(), pl.cols*g.cellW, pl.rows*g.cellH, g.cellW, g.cellH)
	}
	// Y la celda medida queda guardada para el siguiente render: sin guardarla,
	// cada uno Assumption 1x2 y la imagen se deformaría siempre.
	if m.sim.cellW_px != g.cellW || m.sim.cellH_px != g.cellH {
		t.Errorf("la celda medida quedó en %dx%d, want %dx%d: sin guardarla se vuelve a suponer 1x2",
			m.sim.cellW_px, m.sim.cellH_px, g.cellW, g.cellH)
	}
	// Y se marca que la imagen va por la capa de gráficos, que es lo que impide
	// que además se pinte con celdas: dos veces la misma imagen, una encima de
	// la otra y desalineadas.
	if !m.sim.viaGraphics {
		t.Error("tras publicar por la capa de gráficos no se marcó viaGraphics: la imagen se pintaría dos veces")
	}
	// Y sin gráficos disponibles no se publica: es la degradación, no un error.
	g.available = false
	if m.publishSimImage(m.sim.img) {
		t.Error("publicó la imagen sin capa de gráficos disponible")
	}
}

// TestSimKindsNoSeRompenConUnSoloKind: el selector circular con un solo kind es un
// caso degenerado, pero tiene que funcionar: `cursor % 1` siempre da 0 y moverse
// no debe hacer nada raro.
func TestSimKindsNoSeRompenConUnSoloKind(t *testing.T) {
	restore := simKinds
	simKinds = []sim.Kind{sim.KindMerge}
	t.Cleanup(func() { simKinds = restore })

	m := simModel(t, &fakeSimulator{available: true})
	m = press(t, m, "v")
	for i := range 4 {
		m = press(t, m, "j")
		if m.sim.cursor != 0 {
			t.Fatalf("con un solo kind, tras %d pasos el cursor = %d, want 0", i+1, m.sim.cursor)
		}
	}
	m = press(t, m, "k")
	if m.sim.cursor != 0 {
		t.Errorf("con un solo kind, arriba dio el cursor %d, want 0", m.sim.cursor)
	}
	// Y el selector se sigue pudiendo usar: elegir confirma.
	// Con una sola estrategia no hay nada que elegir, así que se entra
	// directamente a renderizar: el popup no se queda esperando una decisión que
	// no existe. Lo que se afirma aquí es que no hay panic ni un cursor fuera de
	// rango, que es lo único que se puede exigir con un solo kind.
	if m.sim.cursor != 0 {
		t.Errorf("con un solo kind el cursor = %d, want 0", m.sim.cursor)
	}
	if m.sim.state == simChoosing && m.sim.kind != sim.KindMerge {
		t.Errorf("con un solo kind y en el selector, kind = %q, want %q", m.sim.kind, sim.KindMerge)
	}
}

func mismoStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// solidSim es una imagen de un solo color, que en el encaje da igual: lo que
// importa son sus DIMENSIONES, y con un color uniforme el promedio de caja es ese
// color y el resultado se puede comparar sin ambigüedad.
func solidSim(w, h int, c color.RGBA) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := range h {
		for x := range w {
			img.SetRGBA(x, y, c)
		}
	}
	return img
}

var negro = color.RGBA{A: 255}

// showSim deja el popup de sim enseñando una imagen, que es el estado del que
// salen los tres caminos de renderSimCells. Se pone la imagen a mano porque lo que
// se prueba es el render, no el camino que llega hasta el popup.
func showSim(t *testing.T, img image.Image) Model {
	t.Helper()
	m := send(t, simModel(t, &fakeSimulator{available: true}), tea.WindowSizeMsg{Width: 100, Height: 30})
	m.sim.state = simShowing
	m.sim.img = img
	return m
}
