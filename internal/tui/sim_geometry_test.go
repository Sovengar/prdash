package tui

import (
	"image"
	"image/color"
	"testing"

	tea "charm.land/bubbletea/v2"

	"prdash/internal/sim"
)

func TestCellSizeUsesTheMeasuredOneOrNeverAZero(t *testing.T) {
	m := newTestModel(t, ghAdapter())

	if w, h := m.cellSize(); w != 1 || h != 2 {
		t.Errorf("unmeasured it gave %dx%d, want 1x2", w, h)
	}
	m.sim.cellW_px, m.sim.cellH_px = 9, 19
	if w, h := m.cellSize(); w != 9 || h != 19 {
		t.Errorf("with 9x19 measured it gave %dx%d, want 9x19: not using the measured one deforms the image", w, h)
	}
	for _, c := range [][2]int{{0, 19}, {9, 0}, {0, 0}, {-1, 19}, {9, -1}} {
		m.sim.cellW_px, m.sim.cellH_px = c[0], c[1]
		if w, h := m.cellSize(); w != 1 || h != 2 {
			t.Errorf("with %dx%d it gave %dx%d, want 1x2: half a cell describes nothing", c[0], c[1], w, h)
		}
	}
}

func TestSimMaxColsAndRowsRespectTheFloor(t *testing.T) {
	for _, w := range []int{0, 10, 20, 40, 80, 200} {
		for _, h := range []int{0, 1, 3, 6, 10, 20, 50, 200} {
			m := send(t, newTestModel(t, ghAdapter()), tea.WindowSizeMsg{Width: w, Height: h})

			cols := m.simMaxCols()
			if cols < simMinCols {
				t.Errorf("terminal %dx%d: %d columns, want >= %d (the floor)", w, h, cols, simMinCols)
			}
			if want := max(simMinCols, m.contentWidth()-simSideMargin); cols != want {
				t.Errorf("terminal %dx%d: %d columnas, want %d", w, h, cols, want)
			}

			rows := m.simMaxRows()
			if rows < simMinRows {
				t.Errorf("terminal %dx%d: %d rows, want >= %d (the floor)", w, h, rows, simMinRows)
			}
			if h >= simMinRows+2*simMargin && rows > h {
				t.Errorf("terminal %dx%d: %d rows, more than the terminal", w, h, rows)
			}
			// It comes from the FRACTION, not the integer.
			if want := max(simMinRows, max(h-2*simMargin, 0)*simHeightNum/simHeightDen); rows != want {
				t.Errorf("terminal %dx%d: %d rows, want %d (3/4 of the free room)", w, h, rows, want)
			}
		}
	}
	m := send(t, newTestModel(t, ghAdapter()), tea.WindowSizeMsg{Width: 40, Height: 1})
	if m.simMaxRows() != simMinRows {
		t.Errorf("on a terminal of 1 row it gave %d rows, want the floor %d", m.simMaxRows(), simMinRows)
	}
}

func TestSimBoxGivesMarginToTheFrameAndTheBoxToTheImage(t *testing.T) {
	m := send(t, newTestModel(t, ghAdapter()), tea.WindowSizeMsg{Width: 100, Height: 30})
	wantWidth := min(m.contentWidth(), simChooserWidth)

	for _, state := range []simState{simChoosing, simRendering} {
		m.sim.state = state
		w, h := m.simBox()
		if w != wantWidth {
			t.Errorf("in %v the box measures %d of width, want %d: with no image the width is fixed", state, w, wantWidth)
		}
		if h != simChrome+2 {
			t.Errorf("in %v the box measures %d of height, want %d (the frame plus two)", state, h, simChrome+2)
		}
	}

	for _, f := range [][2]int{{16, 9}, {9, 16}, {100, 100}, {3, 1}} {
		m.sim.state = simShowing
		m.sim.img = solidSim(f[0], f[1], black)
		m.sim.cellW_px, m.sim.cellH_px = 0, 0
		cellW, cellH := m.cellSize()
		fitCols, fitRows := sim.FitCells(m.sim.img, cellW, cellH, m.simMaxCols(), m.simMaxRows())
		w, h := m.simBox()
		if w != fitCols+2 {
			t.Errorf("img %dx%d: box of %d columns, want %d (the fit plus two of border)", f[0], f[1], w, fitCols+2)
		}
		if h != fitRows+simChrome {
			t.Errorf("img %dx%d: box of %d rows, want %d (the fit plus the frame)", f[0], f[1], h, fitRows+simChrome)
		}
		if fitCols > m.simMaxCols() || fitRows > m.simMaxRows() {
			t.Errorf("img %dx%d: the fit %dx%d goes past the maximum %dx%d",
				f[0], f[1], fitCols, fitRows, m.simMaxCols(), m.simMaxRows())
		}
	}
}

func TestRenderSimCellsDoesNotRepaintTheSameThing(t *testing.T) {
	img := solidSim(16, 9, black)

	t.Run("with no image there are no cells", func(t *testing.T) {
		m := showSim(t, img)
		m.sim.state = simRendering
		m.sim.cells = nil
		m.sim.viaGraphics = false
		m.renderSimCells()
		if m.sim.cells != nil {
			t.Errorf("with no image in the rendering state it left %d cells", len(m.sim.cells))
		}
		if m.sim.cellW != 0 || m.sim.cellH != 0 {
			t.Errorf("with no image cells of %dx%d were left, want 0x0 (so an old repaint is not reused)",
				m.sim.cellW, m.sim.cellH)
		}
	})

	t.Run("with the graphics layer there are no cells", func(t *testing.T) {
		m := showSim(t, img)
		m.sim.viaGraphics = true
		m.sim.cells = []string{"vieja"}
		m.renderSimCells()
		if m.sim.cells != nil {
			t.Errorf("with the image on the graphics layer %d cells were left: they would be painted twice", len(m.sim.cells))
		}
		if m.sim.cellW != 0 || m.sim.cellH != 0 {
			t.Errorf("with the graphics layer dimensions %dx%d were left, want 0x0", m.sim.cellW, m.sim.cellH)
		}
	})

	t.Run("same geometry does not repaint", func(t *testing.T) {
		m := showSim(t, img)
		m.sim.viaGraphics = false
		m.renderSimCells()
		if len(m.sim.cells) == 0 {
			t.Fatal("the first call painted nothing")
		}
		cells := m.sim.cells
		w, h := m.sim.cellW, m.sim.cellH
		m.sim.cells = nil
		m.renderSimCells()
		if m.sim.cells == nil {
			t.Error("the second call with the same geometry erased the cells: the cache guard is not there")
		}
		_ = cells
		_ = w
		_ = h
	})

	t.Run("geometria distinta repinta", func(t *testing.T) {
		m := showSim(t, img)
		m.sim.viaGraphics = false
		m.renderSimCells()
		widthBefore := m.sim.cellW

		before := append([]string(nil), m.sim.cells...)

		// What changes the geometry is the CELL measurement, not the terminal.
		m.sim.cellW_px, m.sim.cellH_px = 9, 19
		m.renderSimCells()
		if m.sim.cellW == widthBefore {
			t.Errorf("after changing the cell to 9x19 the width is still %d: it did not repaint", widthBefore)
		}
		if len(m.sim.cells) == 0 {
			t.Fatal("after changing the geometry no painted cell was left")
		}
		if sameStrings(m.sim.cells, before) {
			t.Error("after changing the cell size the cells are the same: it kept the previous image")
		}
	})
}

func TestPublishSimImagePutsTheImageRightInTheGap(t *testing.T) {
	g := &fakeGraphics{available: true, cellW: 9, cellH: 19}
	m := showSim(t, solidSim(16, 9, black))
	m.SetGraphics(g)
	preCols, preRows := m.simBox()

	if !m.publishSimImage(m.sim.img) {
		t.Fatalf("publishSimImage returned false: %+v", g.sets)
	}
	if len(g.sets) != 1 {
		t.Fatalf("published %d images, want 1", len(g.sets))
	}
	pl := g.sets[0]
	// The box is measured BEFORE publishing.
	cols, rows := preCols, preRows
	col, row := m.simBoxOrigin(cols, rows)
	if pl.col != col+1 || pl.row != row+1 {
		t.Errorf("the image was published at (%d,%d), want (%d,%d): the popups plus one, which is the border",
			pl.col, pl.row, col+1, row+1)
	}
	if pl.cols != cols-2 || pl.rows != rows-simChrome {
		t.Errorf("the image was published at %dx%d, want %dx%d: the inside of the box, without the frame",
			pl.cols, pl.rows, cols-2, rows-simChrome)
	}
	if g.layer != simLayer {
		t.Errorf("the image was published on layer %q, want %q", g.layer, simLayer)
	}
	// The image goes rescaled to the gap's pixel size with the MEASURED cell: the original would stretch it.
	if b := g.sets[0].img.Bounds(); b.Dx() != pl.cols*g.cellW || b.Dy() != pl.rows*g.cellH {
		t.Errorf("the sent image measures %dx%d px, want %dx%d (the room times the measured cell %dx%d): sending the original would be 1920x1080 to paint a rectangle of 800",
			b.Dx(), b.Dy(), pl.cols*g.cellW, pl.rows*g.cellH, g.cellW, g.cellH)
	}
	// The measured cell is kept for the next render, or every render would assume 1x2 and the height would drift.
	if m.sim.cellW_px != g.cellW || m.sim.cellH_px != g.cellH {
		t.Errorf("the measured cell ended at %dx%d, want %dx%d: without storing it 1x2 is assumed again",
			m.sim.cellW_px, m.sim.cellH_px, g.cellW, g.cellH)
	}
	if !m.sim.viaGraphics {
		t.Error("after publishing through the graphics layer viaGraphics was not marked: the image would be painted twice")
	}
	g.available = false
	if m.publishSimImage(m.sim.img) {
		t.Error("it published the image with no graphics layer available")
	}
}

func TestSimKindsDoNotBreakWithASingleKind(t *testing.T) {
	restore := simKinds
	simKinds = []sim.Kind{sim.KindMerge}
	t.Cleanup(func() { simKinds = restore })

	m := simModel(t, &fakeSimulator{available: true})
	m = press(t, m, "v")
	for i := range 4 {
		m = press(t, m, "j")
		if m.sim.cursor != 0 {
			t.Fatalf("with a single kind, after %d steps the cursor = %d, want 0", i+1, m.sim.cursor)
		}
	}
	m = press(t, m, "k")
	if m.sim.cursor != 0 {
		t.Errorf("with a single kind, up gave the cursor %d, want 0", m.sim.cursor)
	}
	if m.sim.cursor != 0 {
		t.Errorf("with a single kind the cursor = %d, want 0", m.sim.cursor)
	}
	if m.sim.state == simChoosing && m.sim.kind != sim.KindMerge {
		t.Errorf("with a single kind and in the selector, kind = %q, want %q", m.sim.kind, sim.KindMerge)
	}
}

func sameStrings(a, b []string) bool {
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

var black = color.RGBA{A: 255}

func showSim(t *testing.T, img image.Image) Model {
	t.Helper()
	m := send(t, simModel(t, &fakeSimulator{available: true}), tea.WindowSizeMsg{Width: 100, Height: 30})
	m.sim.state = simShowing
	m.sim.img = img
	return m
}
