package tui

import (
	"context"
	"errors"
	"image"
	"image/color"
	"image/jpeg"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"

	"prdash/internal/forge/model"
	"prdash/internal/herdr"
	"prdash/internal/sim"
)

// A real JPEG, because applySim loads it with sim.Load: an invented path would only measure the
// discard, not the good path loading the image.
func fakeRender(t *testing.T) sim.Result {
	t.Helper()
	path := filepath.Join(t.TempDir(), "render.jpg")
	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("creating the fake render: %v", err)
	}
	defer func() { _ = f.Close() }()
	img := image.NewRGBA(image.Rect(0, 0, 64, 64))
	for y := range 64 {
		for x := range 64 {
			img.Set(x, y, color.RGBA{R: 200, G: 100, B: 50, A: 255})
		}
	}
	if err := jpeg.Encode(f, img, nil); err != nil {
		t.Fatalf("encoding the fake render: %v", err)
	}
	return sim.Result{Kind: sim.KindMerge, Path: path, Ref: "HEAD", Base: "main"}
}

func simKindForTest(_ bool) sim.Kind { return sim.KindMerge }

func otherKind() sim.Kind { return sim.KindRebase }

// The counter is incremented in startSim BEFORE leaving to the goroutine, so what is slow here does
// not have to do with what is invalidated.
type silentSimulator struct{}

func (silentSimulator) Available() bool { return true }

func (silentSimulator) Simulate(context.Context, model.Item, sim.Kind) (sim.Result, error) {
	return sim.Result{}, errors.New("silent simulator: this test does not render")
}

// The 64x64 size is not arbitrary: with a 4x4 image it fitted in ONE cell at any cell size, so
// changing the cell changed nothing and every geometry assertion passed without looking.
func imageForGeometry() image.Image {
	const side = 64
	img := image.NewRGBA(image.Rect(0, 0, side, side))
	for y := range side {
		for x := range side {
			img.Set(x, y, image.White)
		}
	}
	return img
}

func modelWithSimAtCells(t *testing.T, w, h int) Model {
	t.Helper()
	m := newTestModel(t)
	m.width, m.height = 120, 40
	m.sim.state = simShowing
	m.sim.img = imageForGeometry()
	m.sim.viaGraphics = false
	m.sim.cellW_px, m.sim.cellH_px = w, h
	return m
}

func TestRenderSimCellsComposesAndAnnotatesTheGeometry(t *testing.T) {
	m := modelWithSimAtCells(t, 1, 2)
	m.renderSimCells()

	if len(m.sim.cells) == 0 {
		t.Fatal("with the popup visible and the image set it composed no cells")
	}
	wantW, wantH := m.simBox()
	wantW, wantH = wantW-2, wantH-simChrome
	if m.sim.cellW != wantW || m.sim.cellH != wantH {
		t.Errorf("it recorded %dx%d, want %dx%d (the boxes inner geometry)",
			m.sim.cellW, m.sim.cellH, wantW, wantH)
	}
}

func TestRenderSimCellsDoesNotRecomposeWithTheSameGeometry(t *testing.T) {
	m := modelWithSimAtCells(t, 1, 2)
	m.renderSimCells()
	primeras := len(m.sim.cells)
	if primeras == 0 {
		t.Fatal("it composed nothing the first time")
	}

	for range 4 {
		m.renderSimCells()
	}
	if len(m.sim.cells) != primeras {
		t.Errorf("after four renders with the same geometry there are %d cells, want %d: "+
			"the cache is not cutting, and rescaling the whole image on every resize is "+
			"exactly what makes the popup freeze", len(m.sim.cells), primeras)
	}
	if m.sim.cellW == 0 || m.sim.cellH == 0 {
		t.Error("the recorded geometry was lost: without it the cache cannot be validated")
	}
}

func TestRenderSimCellsRecomposesWhenTheGeometryChanges(t *testing.T) {
	cases := []struct {
		name   string
		width  int
		height int
	}{
		{"more width", 2, 2},           // only w changes
		{"more height", 1, 4},          // only h changes
		{"the other of the two", 2, 3}, // both change
		{"narrower and shorter", 1, 1},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			m := modelWithSimAtCells(t, 1, 2)
			m.renderSimCells()
			if len(m.sim.cells) == 0 {
				t.Fatal("it composed nothing the first time")
			}
			beforeW, beforeH := m.sim.cellW, m.sim.cellH

			m.sim.cellW_px, m.sim.cellH_px = c.width, c.height
			m.renderSimCells()

			wantW, wantH := m.simBox()
			wantW, wantH = wantW-2, wantH-simChrome

			if m.sim.cellW != wantW || m.sim.cellH != wantH {
				t.Errorf("after changing the cell to %dx%d the recorded geometry ended at %dx%d, "+
					"want %dx%d: it did not recompose with the new one, and the image stays out of sync",
					c.width, c.height, m.sim.cellW, m.sim.cellH, wantW, wantH)
			}
			// The new geometry has to DIFFER from the old one, or the case would not tell a recomposed from
			// a no-op and the assertion above would pass with nothing happening.
			if m.sim.cellW == beforeW && m.sim.cellH == beforeH {
				t.Errorf("the recorded geometry is still %dx%d after changing the cell to "+
					"%dx%d: the case would not test the change, Compose and no-op would give the same",
					beforeW, beforeH, c.width, c.height)
			}
		})
	}
}

// The stale cells belong to the PREVIOUS image with a different geometry; leaving them would paint
// the old image inside the new frame, so the user sees a review that is not the one in front of them.
func TestRenderSimCellsWithoutAnImageLeavesNoOldCells(t *testing.T) {
	m := modelWithSimAtCells(t, 1, 2)
	m.renderSimCells()
	if len(m.sim.cells) == 0 {
		t.Fatal("it composed nothing the first time")
	}

	for _, c := range []struct {
		name    string
		prepara func(*Model)
	}{
		{"the image disappears", func(m *Model) { m.sim.img = nil }},
		{"the popup is no longer visible", func(m *Model) { m.sim.state = simRendering }},
		{"the image goes through the graphics layer", func(m *Model) { m.sim.viaGraphics = true }},
	} {
		t.Run(c.name, func(t *testing.T) {
			m := modelWithSimAtCells(t, 1, 2)
			m.renderSimCells()
			if len(m.sim.cells) == 0 {
				t.Fatal("it composed nothing the first time")
			}
			c.prepara(&m)
			m.renderSimCells()

			if len(m.sim.cells) != 0 {
				t.Errorf("%d cells of the previous image were left: the old review "+
					"would be seen with the new frame, and with nothing saying it is out of sync",
					len(m.sim.cells))
			}
			if m.sim.cellW != 0 || m.sim.cellH != 0 {
				t.Errorf("the recorded geometry ended at %dx%d, want 0x0: with no cells no "+
					"geometry can remain, or the next round would compare against it",
					m.sim.cellW, m.sim.cellH)
			}
		})
	}
}

// Not a tautology: it CALLS simBox's formula, and `cols-2 > 0` alone is not enough. This
// REPLACES a test whose t.Fatalf never ran: it swept 4,500 combinations and always passed.
func TestThePopupsBoxAlwaysHasAnInteriorGap(t *testing.T) {
	images := []image.Image{
		imageForGeometry(), // cuadrada
		imageOf(4, 512),    // extreme vertical: the one of one column
		imageOf(2, 512),    // even more vertical
		imageOf(512, 4),    // apaisada extrema
		imageOf(64, 64),
	}

	for vi, img := range images {
		for _, cell := range [][2]int{{1, 2}, {2, 1}, {9, 19}, {1, 19}} {
			for w := 40; w <= 200; w += 13 {
				for h := 6; h <= 120; h += 11 {
					m := modelWithSimAtCells(t, cell[0], cell[1])
					m.width, m.height = w, h
					m.sim.state = simShowing
					m.sim.img = img
					cols, rows := m.simBox()
					if cols-2 <= 0 || rows-simChrome <= 0 {
						t.Fatalf("image %d, cell %dx%d, terminal %dx%d: the inner room "+
							"is %dx%d, not even one cell fits. simBox returned %dx%d, "+
							"and FitCells cannot return less than 1 in any dimension",
							vi, cell[0], cell[1], w, h,
							cols-2, rows-simChrome, cols, rows)
					}
				}
			}
		}
	}
}

// Not a counter: a mechanism for INVALIDATING, which works only because the number always moves
// up. With `--` instead, closing with `++` lands back on 0 and an old render paints over the new.
func TestSimSeqIncrementsAndNeverDecrements(t *testing.T) {
	m := newTestModel(t)
	m.SetSimulator(silentSimulator{})
	split := m.simSeq
	vistos := map[int]bool{}

	for i := range 20 {
		m.startSim(simKindForTest(false))
		// An increment of ONE, not just "it went up": if open and close summed more, two opens could land
		// on the same number and the discard would break unnoticed.
		if m.simSeq != split+1 {
			t.Fatalf("opening the popup (round %d) left simSeq at %d, want %d: "+
				"it has to grow by one at a time", i, m.simSeq, split+1)
		}
		split = m.simSeq
		if vistos[m.simSeq] {
			t.Fatalf("simSeq=%d had already been seen: a repeated number makes an "+
				"old result pass the applySim comparison", m.simSeq)
		}
		vistos[m.simSeq] = true

		m.closeSim()
		if m.simSeq != split+1 {
			t.Fatalf("closing the popup for the %d time did not raise simSeq: it stayed at %d, it had "+
				"to go from %d. A close that does not invalidate lets the render in flight "+
				"paint its image in a popup that no longer exists", i, m.simSeq, split)
		}
		split = m.simSeq
		if vistos[m.simSeq] {
			t.Fatalf("simSeq=%d had already been seen after closing", m.simSeq)
		}
		vistos[m.simSeq] = true
	}
}

// The case that matters is the CHANGED strategy with the popup open: the first result arrives
// while the second is still rendering and would paint over it.
func TestApplySimDiscardsTheStaleOne(t *testing.T) {
	for _, c := range []struct {
		name    string
		prepara func(*Model)
		wants   string
	}{
		{
			"the popup closed while it ran",
			func(m *Model) { m.closeSim() },
			"being left with no image",
		},
		{
			"the popup reopened with another strategy",
			func(m *Model) {
				m.sim.state = simRendering
				m.sim.kind = otherKind()
				m.simSeq++
			},
			"keeping the new strategy and no image",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			m := modelWithSimAtCells(t, 1, 2)
			m.SetSimulator(silentSimulator{})
			m.startSim(simKindForTest(false))
			stale := simMsg{seq: m.simSeq, kind: m.sim.kind}

			c.prepara(&m)
			beforeSeq := m.simSeq

			m.applySim(stale)

			if m.simSeq != beforeSeq {
				t.Errorf("applySim moved simSeq from %d to %d when discarding a stale "+
					"result: discarding is not advancing, and advancing invalidates the request "+
					"that IS in flight", beforeSeq, m.simSeq)
			}
			if len(m.sim.cells) != 0 {
				t.Errorf("a stale result left %d cells: the render of the "+
					"previous request was painted, and nothing says it is out of sync",
					len(m.sim.cells))
			}
		})
	}

	t.Run("the result that IS current is applied", func(t *testing.T) {
		m := modelWithSimAtCells(t, 1, 2)
		m.SetSimulator(silentSimulator{})
		m.startSim(simKindForTest(false))
		current := simMsg{seq: m.simSeq, kind: m.sim.kind, res: fakeRender(t)}

		m.applySim(current)

		if m.sim.state != simShowing {
			t.Errorf("state=%v, want simShowing: a current result has to open "+
				"the popup, or discarding the stale ones discards nothing",
				m.sim.state)
		}
		if m.sim.img == nil {
			t.Error("the current results image was not loaded")
		}
	})
}

// Storing the context is what makes the delete's timeout testable: "how long does this wait" is
// answered by the deadline the call received, not by the clock.
type graphicsCapture struct {
	cellW, cellH int

	mu       sync.Mutex
	clearCtx []clearCall
	layers   []string
}

// The moment matters: releaseSimLayer does `defer cancel()`, so a double that stored the context
// would always read "cancelled" and conclude the delete inherits the app's context.
type clearCall struct {
	errOnCall     error
	timeoutOnCall time.Duration
	hasTimeout    bool
}

func (g *graphicsCapture) Available() bool { return true }

func (g *graphicsCapture) CellSize(context.Context) (int, int) { return g.cellW, g.cellH }

func (g *graphicsCapture) SetImage(context.Context, string, image.Image, herdr.Placement) error {
	return nil
}

func (g *graphicsCapture) Clear(ctx context.Context, layer string) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	dl, exists := ctx.Deadline()
	var plazo time.Duration
	if exists {
		// The deadline is read HERE, at call time, not when the test looks later: the test's own time passes
		// in between, and with a millisecond deadline you would be measuring the test's clock.
		plazo = time.Until(dl)
	}
	g.clearCtx = append(g.clearCtx, clearCall{
		errOnCall:     ctx.Err(),
		timeoutOnCall: plazo,
		hasTimeout:    exists,
	})
	g.layers = append(g.layers, layer)
	return nil
}

func (g *graphicsCapture) ctxs() []clearCall {
	g.mu.Lock()
	defer g.mu.Unlock()
	return append([]clearCall(nil), g.clearCtx...)
}

func (g *graphicsCapture) visibleLayers() []string {
	g.mu.Lock()
	defer g.mu.Unlock()
	return append([]string(nil), g.layers...)
}

func TestTheUnmeasuredCellAssumes1x2(t *testing.T) {
	for _, c := range []struct {
		name   string
		medida [2]int
	}{
		{"Herdr knows nothing", [2]int{0, 0}},
		{"Herdr measures width but not height", [2]int{9, 0}},
		{"Herdr measures height but not width", [2]int{0, 19}},
		{"Herdr returns negatives, which are not measures", [2]int{-9, -19}},
	} {
		t.Run(c.name, func(t *testing.T) {
			m := modelWithSimAtCells(t, 1, 2)
			m.sim.state = simShowing
			m.sim.img = imageForGeometry()
			m.graphics = &graphicsCapture{cellW: c.medida[0], cellH: c.medida[1]}

			w, h := m.cellSize()
			if w != 1 || h != 2 {
				t.Errorf("with CellSize returning %v, cellSize gave %dx%d, want 1x2: "+
					"a terminal cell is one character of width by two dots of height",
					c.medida, w, h)
			}
		})
	}

	t.Run("Herdr wins the measure", func(t *testing.T) {
		m := modelWithSimAtCells(t, 9, 19)
		w, h := m.cellSize()
		if w != 9 || h != 19 {
			t.Errorf("with Herdr measuring 9x19, cellSize gave %dx%d, want 9x19", w, h)
		}
	})
}

func TestTheCellFloorChangesTheImageGeometry(t *testing.T) {
	m := modelWithSimAtCells(t, 0, 0) // Herdr does not measure: the floor kicks in
	m.sim.state = simShowing
	m.sim.img = imageForGeometry()
	m.renderSimCells()
	if m.sim.cellW == 0 {
		t.Fatal("with the 1x2 floor it composed nothing")
	}
	width1x2 := m.sim.cellW

	m2 := modelWithSimAtCells(t, 2, 1)
	m2.sim.state = simShowing
	m2.sim.img = imageForGeometry()
	m2.renderSimCells()
	width2x1 := m2.sim.cellW

	if width2x1 >= width1x2 {
		t.Errorf("with a cell of 2px width there are %d columns and with 1px %d: a cell at "+
			"double the width has to give fewer columns for the same image",
			width2x1, width1x2)
	}
	m4 := modelWithSimAtCells(t, 4, 1)
	m4.sim.state = simShowing
	m4.sim.img = imageForGeometry()
	m4.renderSimCells()
	if m4.sim.cellW >= width2x1 {
		t.Errorf("with a cell of 4px there are %d columns and with 2px %d: the number of columns "+
			"has to go down with the cells width", m4.sim.cellW, width2x1)
	}
}

func TestTheLayerWipeWaitsBrieflyAndInTheBackground(t *testing.T) {
	g := &graphicsCapture{}
	m := newTestModel(t)
	m.graphics = g
	m.sim.viaGraphics = true

	m.releaseSimLayer()

	calls := waitForCalls(t, g, 200, 5*time.Millisecond)
	if len(calls) != 1 {
		t.Fatalf("Clear was called %d times, want 1: a retry on a layer that is no "+
			"longer there does not remove it faster", len(calls))
	}
	call := calls[0]

	if !call.hasTimeout {
		t.Fatal("the deletion context has no deadline: if Herdr does not answer, the " +
			"Clear stays waiting and what freezes is the process on exit")
	}
	const (
		want = 2 * time.Second
		tol  = 20 * time.Millisecond
	)
	if d := call.timeoutOnCall - want; d > tol || d < -tol {
		t.Errorf("the deletion deadline is %v when calling, want %v±%v (it departs %v): "+
			"below it the Clear has no time to answer Herdr, and above "+
			"what freezes is the process on exit",
			call.timeoutOnCall.Round(time.Millisecond), want, tol,
			d.Round(time.Millisecond))
	}
	if call.errOnCall != nil {
		t.Errorf("the deletion context was already cancelled (%v) when calling: if "+
			"it inherited the apps one, the Clear would not get to do anything and the image "+
			"would stay stuck when leaving the TUI", call.errOnCall)
	}
}

func TestTheWipeIsNotAttemptedWhenTheLayerHasNoImage(t *testing.T) {
	for _, c := range []struct {
		name    string
		prepara func(*Model)
	}{
		{"nothing was published", func(m *Model) { m.sim.viaGraphics = false }},
		{"there is no graphics", func(m *Model) { m.graphics = nil }},
	} {
		t.Run(c.name, func(t *testing.T) {
			g := &graphicsCapture{}
			m := newTestModel(t)
			m.graphics = g
			m.sim.viaGraphics = true
			c.prepara(&m)

			m.releaseSimLayer()
			waitForCalls(t, g, 40, 5*time.Millisecond)

			if len(g.ctxs()) != 0 {
				t.Errorf("Clear was called %d times when there was nothing on the layer: "+
					"it is a call to a whole process, and having nothing is the normal case",
					len(g.ctxs()))
			}
		})
	}
}

func TestTheWipeGoesOnTopOfTheSimulatorsLayer(t *testing.T) {
	g := &graphicsCapture{}
	m := newTestModel(t)
	m.graphics = g
	m.sim.viaGraphics = true

	m.releaseSimLayer()
	waitForCalls(t, g, 200, 5*time.Millisecond)

	layers := g.visibleLayers()
	if len(layers) != 1 {
		t.Fatalf("Clear was called %d times, want 1", len(layers))
	}
	if layers[0] != simLayer {
		t.Errorf("it erased layer %q, want %q: a Clear on another layer is a silent "+
			"no-op, and the image stays stuck with nothing relating it to the popup",
			layers[0], simLayer)
	}
}

func waitForCalls(t *testing.T, g *graphicsCapture, attempts int, wait time.Duration) []clearCall {
	t.Helper()
	for range attempts {
		if c := g.ctxs(); len(c) > 0 {
			return c
		}
		time.Sleep(wait)
	}
	return g.ctxs()
}

type graphicsWithErrors struct {
	cellW, cellH int
	cellErr      error
	setErr       error

	mu        sync.Mutex
	setCalls  []herdr.Placement
	setCtxErr []error
	setImage  []image.Image
}

func (g *graphicsWithErrors) Available() bool { return true }

func (g *graphicsWithErrors) CellSize(context.Context) (int, int) {
	if g.cellErr != nil {
		return 0, 0
	}
	return g.cellW, g.cellH
}

func (g *graphicsWithErrors) SetImage(ctx context.Context, _ string, img image.Image, p herdr.Placement) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.setCalls = append(g.setCalls, p)
	g.setCtxErr = append(g.setCtxErr, ctx.Err())
	g.setImage = append(g.setImage, img)
	return g.setErr
}

func (g *graphicsWithErrors) Clear(context.Context, string) error { return nil }

func (g *graphicsWithErrors) placements() []herdr.Placement {
	g.mu.Lock()
	defer g.mu.Unlock()
	return append([]herdr.Placement(nil), g.setCalls...)
}

func (g *graphicsWithErrors) contexts() []error {
	g.mu.Lock()
	defer g.mu.Unlock()
	return append([]error(nil), g.setCtxErr...)
}

func TestPublishSimImageMeasuresTheCellAndStoresIt(t *testing.T) {
	const cellW, cellH = 9, 19
	g := &graphicsWithErrors{cellW: cellW, cellH: cellH}

	m := modelWithSimAtCells(t, 0, 0)
	m.graphics = g
	m.sim.state = simShowing
	m.sim.img = imageForGeometry()

	cols, rows := m.simBox()
	col, row := m.simBoxOrigin(cols, rows)
	want := herdr.Placement{
		Col:  col + 1,
		Row:  row + 1,
		Cols: cols - 2,
		Rows: rows - simChrome,
	}

	if !m.publishSimImage(m.sim.img) {
		t.Fatal("publishSimImage said it could not publish, and the double never fails")
	}

	if m.sim.cellW_px != cellW || m.sim.cellH_px != cellH {
		t.Errorf("it stored the cell as %dx%d, want %dx%d: without storing it, if the layer stops "+
			"being available the popup draws at 1x2 an image that was sized for "+
			"%dx%d", m.sim.cellW_px, m.sim.cellH_px, cellW, cellH, cellW, cellH)
	}

	placements := g.placements()
	if len(placements) != 1 {
		t.Fatalf("sent %d images, want 1", len(placements))
	}
	if placements[0] != want {
		t.Errorf("placed at %+v, want %+v: the image goes in the popups inner room, "+
			"one cell inside the frame", placements[0], want)
	}
}

func TestPublishSimImageWithoutAMeasuredCellAssumes1x2(t *testing.T) {
	for _, c := range []struct {
		name   string
		medida [2]int
	}{
		{"Herdr returns (0,0)", [2]int{0, 0}},
		{"Herdr measures only the width", [2]int{9, 0}},
		{"Herdr measures only the height", [2]int{0, 19}},
	} {
		t.Run(c.name, func(t *testing.T) {
			g := &graphicsWithErrors{cellW: c.medida[0], cellH: c.medida[1]}
			m := modelWithSimAtCells(t, 0, 0)
			m.graphics = g
			m.sim.state = simShowing
			m.sim.img = imageForGeometry()

			if !m.publishSimImage(m.sim.img) {
				t.Fatal("with no measured cell it could not publish: the 1x2 floor has to " +
					"leave the graphics layer path available")
			}
			if m.sim.cellW_px != 1 || m.sim.cellH_px != 2 {
				t.Errorf("it stored the cell as %dx%d, want 1x2",
					m.sim.cellW_px, m.sim.cellH_px)
			}
		})
	}
}

func TestPublishUsesItsOwnContextAndNotTheApplications(t *testing.T) {
	g := &graphicsWithErrors{cellW: 9, cellH: 19}
	m := modelWithSimAtCells(t, 0, 0)
	m.graphics = g
	m.sim.state = simShowing
	m.sim.img = imageForGeometry()

	ctx, cancel := context.WithCancel(m.ctx)
	cancel()
	m.ctx = ctx

	if !m.publishSimImage(m.sim.img) {
		t.Fatal("it could not publish with the apps context cancelled: the publication " +
			"has to carry its own, or when leaving the TUI the image stays stuck")
	}

	ctxs := g.contexts()
	if len(ctxs) != 1 {
		t.Fatalf("sent %d images, want 1", len(ctxs))
	}
	if ctxs[0] != nil {
		t.Errorf("the publication context was cancelled (%v) when sending the "+
			"image: it inherited the apps one, which is already dead when leaving the TUI",
			ctxs[0])
	}
}

func TestPublishSimImageDoesNotPublishIfTheSetFails(t *testing.T) {
	g := &graphicsWithErrors{cellW: 9, cellH: 19, setErr: errors.New("layer busy")}
	m := modelWithSimAtCells(t, 0, 0)
	m.graphics = g
	m.sim.state = simShowing
	m.sim.img = imageForGeometry()

	if m.publishSimImage(m.sim.img) {
		t.Fatal("it said it published when Herdr rejected it")
	}
	if m.sim.viaGraphics {
		t.Error("it marked viaGraphics with the failed publication: the popup would paint nothing " +
			"and would not know it is waiting, that is an empty frame")
	}
}

func TestPublishSimImageMarksTheFlagAndEmptiesTheCells(t *testing.T) {
	g := &graphicsWithErrors{cellW: 9, cellH: 19}
	m := modelWithSimAtCells(t, 1, 2)
	m.graphics = g
	m.sim.state = simShowing
	m.sim.img = imageForGeometry()

	m.renderSimCells()
	if len(m.sim.cells) == 0 {
		t.Fatal("it left no previous cells: the case needs cells that can remain")
	}

	if !m.publishSimImage(m.sim.img) {
		t.Fatal("no pudo publicar")
	}
	if !m.sim.viaGraphics {
		t.Error("it did not mark viaGraphics after publishing: the popup would paint over itself " +
			"an image the layer is already painting")
	}
	if len(m.sim.cells) != 0 {
		t.Errorf("%d cells were left after publishing on the layer: they are of the previous image, "+
			"which has another geometry, and they would be painted over the new one", len(m.sim.cells))
	}
}

func TestPublishSimImageWithoutGraphicsNorLayerAttemptsNothing(t *testing.T) {
	m := modelWithSimAtCells(t, 0, 0)
	m.graphics = nil
	m.sim.state = simShowing
	m.sim.img = imageForGeometry()
	if m.publishSimImage(m.sim.img) {
		t.Error("it published without having Herdr")
	}
	if m.sim.viaGraphics || m.sim.cellW_px != 0 {
		t.Error("without Herdr it must touch neither the flag nor the stored cell")
	}
}

func TestPublishSimImageUsesTheSimulatorsLayer(t *testing.T) {
	g := &graphicsWithErrors{cellW: 9, cellH: 19}
	m := modelWithSimAtCells(t, 0, 0)
	m.graphics = g
	m.sim.state = simShowing
	m.sim.img = imageForGeometry()

	m.publishSimImage(m.sim.img)
	if simLayer == "" {
		t.Error("the simulator layer is empty")
	}
}

func TestPublishSimImageDoesNotPublishWithABoxThatDoesNotFit(t *testing.T) {
	g := &graphicsWithErrors{cellW: 9, cellH: 19}

	m := modelWithSimAtCells(t, 0, 0)
	m.graphics = g
	m.sim.state = simShowing
	m.sim.img = imageForGeometry()

	for w := 20; w >= 4; w-- {
		for h := 8; h >= 4; h-- {
			m.width, m.height = w, h
			before := len(g.placements())
			m.publishSimImage(m.sim.img)
			cols, rows := m.simBox()
			if cols-2 > 0 && rows-simChrome > 0 {
				continue // the box still fits
			}
			if len(g.placements()) != before {
				t.Fatalf("with %dx%d the box does not fit (%dx%d) and something was published: an inner "+
					"room of 0 columns is an image of 0 pixels",
					w, h, cols-2, rows-simChrome)
			}
		}
	}
}

type caseGeom struct {
	img       image.Image
	col, rows int
}

func TestTheWidthDoesNotDetermineTheHeight(t *testing.T) {
	const (
		viewWidth, viewHeight = 60, 100
		cellW, cellH          = 1, 2
	)

	var first, second *caseGeom

	for side := 8; side <= 128 && second == nil; side += 8 {
		for heightPx := 8; heightPx <= 128 && second == nil; heightPx += 8 {
			m := modelWithSimAtCells(t, cellW, cellH)
			m.width, m.height = viewWidth, viewHeight
			m.sim.state = simShowing
			m.sim.img = imageOf(side, heightPx)
			m.renderSimCells()
			if m.sim.cellW == 0 || m.sim.cellH == 0 {
				continue
			}
			sample := &caseGeom{img: m.sim.img, col: m.sim.cellW, rows: m.sim.cellH}
			switch {
			case first == nil:
				first = sample
			case second == nil && sample.col == first.col && sample.rows != first.rows:
				second = sample
			}
		}
	}

	if second == nil {
		t.Fatal("no image was found that gives the same box width with " +
			"a different height. If it no longer exists, the width determines the height, the " +
			"cellH cache comparison can be removed, and this test is " +
			"saying something that is no longer true")
	}
	t.Logf("same %d columns: %d rows with the first image and %d with the second",
		first.col, first.rows, second.rows)

	m := modelWithSimAtCells(t, cellW, cellH)
	m.width, m.height = viewWidth, viewHeight
	m.sim.state = simShowing
	m.sim.img = first.img
	m.renderSimCells()
	if m.sim.cellW != first.col || m.sim.cellH != first.rows {
		t.Fatalf("the first image gave %dx%d, want %dx%d",
			m.sim.cellW, m.sim.cellH, first.col, first.rows)
	}
	firstCells := len(m.sim.cells)

	m.sim.img = second.img
	m.renderSimCells()

	if m.sim.cellH != second.rows {
		t.Errorf("with the second image (same width of %d, %d rows) it stayed at %d: "+
			"it did not recompose, so the cells are of the previous image —%d of them— and "+
			"the new one leaves at the bottom of the frame",
			second.col, second.rows, m.sim.cellH, firstCells)
	}
	if m.sim.cellW != first.col {
		t.Errorf("the width changed to %d with the second image, and the case was of the same "+
			"width (%d)", m.sim.cellW, first.col)
	}
}

func imageOf(widthPx, heightPx int) image.Image {
	img := image.NewRGBA(image.Rect(0, 0, widthPx, heightPx))
	for y := range heightPx {
		for x := range widthPx {
			img.Set(x, y, image.White)
		}
	}
	return img
}

func TestPadRightPadItOrLeaveIt(t *testing.T) {
	cases := []struct {
		name      string
		s         string
		n         int
		wantsCols int
	}{
		{"short, it pads", "ab", 6, 6},
		{"exact, it does not touch", "abcd", 4, 4},
		{"exact with one less", "abcd", 3, 4}, // it does not fit: it is left as is
		{"very long", "many letters", 3, 13},  // it does not touch it either
		{"empty", "", 5, 5},
		{"a zero", "abc", 0, 3},
		{"negative n", "abc", -4, 3},
		{"with accents", "áéí", 6, 3}, // 3 columns, 6 bytes
		{"with emoji", "🙂", 4, 2},     // 2 columns, 4 bytes
		{"emoji y text", "a🙂b", 6, 4},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := padRight(c.s, c.n)
			want := max(ansi.StringWidth(c.s), c.n)

			if w := ansi.StringWidth(got); w != want {
				t.Errorf("padRight(%q, %d) measures %d visible columns, want %d. The input "+
					"measures %d bytes and %d columns: padding by bytes misaligns the labels, "+
					"which is the only thing this function fixes",
					c.s, c.n, w, want, len(c.s), ansi.StringWidth(c.s))
			}
			if len(got) < len(c.s) || got[:len(c.s)] != c.s {
				t.Errorf("padRight(%q, %d) returned %q, which does not start with the original text: "+
					"it does not clip, it pads", c.s, c.n, got)
			}
			for i, r := range got[len(c.s):] {
				if r != ' ' {
					t.Errorf("the padding carries %q at position %d, want a space: "+
						"a different character reads as part of the label", r, i)
					break
				}
			}
		})
	}
}

func TestPadRightWithTheExactEdge(t *testing.T) {
	for _, s := range []string{"", "a", "ab", "áé", "🙂"} {
		width := ansi.StringWidth(s)
		got := padRight(s, width)
		if got != s {
			t.Errorf("with the exact width (%q takes %d) it returned %q, want the text unchanged",
				s, width, got)
		}
		if ansi.StringWidth(got) != width {
			t.Errorf("with the exact width it returned %d columns, want %d", ansi.StringWidth(got), width)
		}
		if p := padRight(s, width+1); ansi.StringWidth(p) != width+1 {
			t.Errorf("with one extra column it returned %d columns, want %d: the padding "+
				"does not happen, and the exact border assertion would pass anyway",
				ansi.StringWidth(p), width+1)
		}
	}
}
