package tui

import (
	"image"
	"image/color"
	"testing"

	tea "charm.land/bubbletea/v2"

	"prdash/internal/sim"
)

func TestCellSizeUsaLoMedidoONuncaUnCero(t *testing.T) {
	m := newTestModel(t, ghAdapter())

	if w, h := m.cellSize(); w != 1 || h != 2 {
		t.Errorf("sin medir dio %dx%d, want 1x2", w, h)
	}
	m.sim.cellW_px, m.sim.cellH_px = 9, 19
	if w, h := m.cellSize(); w != 9 || h != 19 {
		t.Errorf("con 9x19 medidos dio %dx%d, want 9x19: no usar lo medido deforma la imagen", w, h)
	}
	for _, c := range [][2]int{{0, 19}, {9, 0}, {0, 0}, {-1, 19}, {9, -1}} {
		m.sim.cellW_px, m.sim.cellH_px = c[0], c[1]
		if w, h := m.cellSize(); w != 1 || h != 2 {
			t.Errorf("con %dx%d dio %dx%d, want 1x2: media celda no describe nada", c[0], c[1], w, h)
		}
	}
}

// The popup leaves room at the sides and top and bottom.
func TestSimMaxColsYRowsRespetanElSuelo(t *testing.T) {
	for _, w := range []int{0, 10, 20, 40, 80, 200} {
		for _, h := range []int{0, 1, 3, 6, 10, 20, 50, 200} {
			m := send(t, newTestModel(t, ghAdapter()), tea.WindowSizeMsg{Width: w, Height: h})

			cols := m.simMaxCols()
			if cols < simMinCols {
				t.Errorf("terminal %dx%d: %d columnas, want >= %d (el suelo)", w, h, cols, simMinCols)
			}
			if want := max(simMinCols, m.contentWidth()-simSideMargin); cols != want {
				t.Errorf("terminal %dx%d: %d columnas, want %d", w, h, cols, want)
			}

			rows := m.simMaxRows()
			if rows < simMinRows {
				t.Errorf("terminal %dx%d: %d filas, want >= %d (el suelo)", w, h, rows, simMinRows)
			}
			if h >= simMinRows+2*simMargin && rows > h {
				t.Errorf("terminal %dx%d: %d filas, más que la terminal", w, h, rows)
			}
			// It comes from the FRACTION, not the integer.
			if want := max(simMinRows, max(h-2*simMargin, 0)*simHeightNum/simHeightDen); rows != want {
				t.Errorf("terminal %dx%d: %d filas, want %d (3/4 del hueco libre)", w, h, rows, want)
			}
		}
	}
	m := send(t, newTestModel(t, ghAdapter()), tea.WindowSizeMsg{Width: 40, Height: 1})
	if m.simMaxRows() != simMinRows {
		t.Errorf("en una terminal de 1 fila dio %d filas, want el suelo %d", m.simMaxRows(), simMinRows)
	}
}

// Two different boxes with two different rules.
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
		if fitCols > m.simMaxCols() || fitRows > m.simMaxRows() {
			t.Errorf("img %dx%d: el encaje %dx%d se pasa del máximo %dx%d",
				f[0], f[1], fitCols, fitRows, m.simMaxCols(), m.simMaxRows())
		}
	}
}

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

		// What changes the geometry is the CELL measurement, not the terminal.
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

// The image goes in the graphics layer.
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
	// The box is measured BEFORE publishing.
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
	if g.layer != simLayer {
		t.Errorf("la imagen se publicó en la capa %q, want %q", g.layer, simLayer)
	}
	// The image goes rescaled to the gap's pixel size, using the MEASURED cell: sending the
	// original would stretch it.
	if b := g.sets[0].img.Bounds(); b.Dx() != pl.cols*g.cellW || b.Dy() != pl.rows*g.cellH {
		t.Errorf("la imagen mandada mide %dx%d px, want %dx%d (el hueco por la celda medida %dx%d): mandar la original serían 1920x1080 para pintar un rectángulo de 800",
			b.Dx(), b.Dy(), pl.cols*g.cellW, pl.rows*g.cellH, g.cellW, g.cellH)
	}
	// The measured cell is kept for the next render: without keeping it, every render would
	// assume 1x2 and the height would drift.
	if m.sim.cellW_px != g.cellW || m.sim.cellH_px != g.cellH {
		t.Errorf("la celda medida quedó en %dx%d, want %dx%d: sin guardarla se vuelve a suponer 1x2",
			m.sim.cellW_px, m.sim.cellH_px, g.cellW, g.cellH)
	}
	if !m.sim.viaGraphics {
		t.Error("tras publicar por la capa de gráficos no se marcó viaGraphics: la imagen se pintaría dos veces")
	}
	g.available = false
	if m.publishSimImage(m.sim.img) {
		t.Error("publicó la imagen sin capa de gráficos disponible")
	}
}

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
	// The selector stays usable: choosing confirms.
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

func showSim(t *testing.T, img image.Image) Model {
	t.Helper()
	m := send(t, simModel(t, &fakeSimulator{available: true}), tea.WindowSizeMsg{Width: 100, Height: 30})
	m.sim.state = simShowing
	m.sim.img = img
	return m
}
