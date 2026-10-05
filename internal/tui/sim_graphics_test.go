package tui

import (
	"context"
	"errors"
	"image"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"prdash/internal/herdr"
	"prdash/internal/sim"
)

type fakeGraphics struct {
	available bool
	cellW     int
	cellH     int
	err       error

	cleared chan string

	sets   []placement
	clears int
	layer  string
}

type placement struct {
	col, row, cols, rows int
	img                  image.Image
}

func (f *fakeGraphics) Available() bool { return f.available }

func (f *fakeGraphics) CellSize(context.Context) (int, int) {
	if f.cellW <= 0 || f.cellH <= 0 {
		return 1, 2
	}
	return f.cellW, f.cellH
}

func (f *fakeGraphics) SetImage(_ context.Context, layer string, img image.Image, p herdr.Placement) error {
	f.layer = layer
	if f.err != nil {
		return f.err
	}
	f.sets = append(f.sets, placement{col: p.Col, row: p.Row, cols: p.Cols, rows: p.Rows, img: img})
	return nil
}

func (f *fakeGraphics) Clear(_ context.Context, layer string) error {
	f.clears++
	f.layer = layer
	if f.cleared != nil {
		f.cleared <- layer
	}
	return nil
}

func waitClear(t *testing.T, f *fakeGraphics) {
	t.Helper()
	if f.cleared == nil {
		if f.clears == 0 {
			t.Fatal("closing the popup did not ask to remove the layer: the image would stay over the TUI")
		}
		return
	}
	select {
	case <-f.cleared:
	case <-time.After(2 * time.Second):
		t.Fatal("the layer was not removed")
	}
}

func simModelWith(t *testing.T, g Graphics) Model {
	t.Helper()
	m := simModel(t, &fakeSimulator{available: true})
	m.SetGraphics(g)
	return m
}

func TestTheImageGoesToThePaneLayer(t *testing.T) {
	path := writeJPEG(t)
	f := &fakeSimulator{available: true, res: sim.Result{Kind: sim.KindMerge, Path: path}}
	g := &fakeGraphics{available: true, cellW: 9, cellH: 19}
	m := simModel(t, f)
	m.SetGraphics(g)

	m = press(t, m, "v")
	m = press(t, m, "enter")
	m = send(t, m, simMsg{seq: m.simSeq, kind: sim.KindMerge, res: f.res})

	if !m.sim.viaGraphics {
		t.Fatal("the image was not published on the pane layer")
	}
	if len(g.sets) != 1 {
		t.Fatalf("SetImage was called %d times, want 1", len(g.sets))
	}
	if len(m.sim.cells) != 0 {
		t.Errorf("the box painted %d lines of cells with the image on the layer", len(m.sim.cells))
	}
	set := g.sets[0]
	if set.cols <= 0 || set.rows <= 0 {
		t.Errorf("empty rectangle: %+v", set)
	}
	if set.col < 1 || set.row < 1 {
		t.Errorf("the image starts at (%d,%d): it would leave the frame", set.col, set.row)
	}
	b := set.img.Bounds()
	const cellPx = 9 // the cell width Herdr reports in kitty
	if want := set.cols * cellPx; b.Dx() > want {
		t.Errorf("image of %d px for a rectangle of %d: more resolution is sent than is seen", b.Dx(), want)
	}
	if b.Dx() < set.cols*4 {
		t.Errorf("image of %d px, too little for %d columns", b.Dx(), set.cols)
	}
	if text := viewText(m); !strings.Contains(text, "simulate: merge") {
		t.Errorf("the popup lost its title:\n%s", text)
	}
}

func TestWithoutTheLayerTheCellsTakeOver(t *testing.T) {
	path := writeJPEG(t)
	f := &fakeSimulator{available: true, res: sim.Result{Kind: sim.KindMerge, Path: path}}

	for name, g := range map[string]Graphics{
		"no layer":      nil,
		"missing layer": &fakeGraphics{available: false},
		"failing layer": &fakeGraphics{available: true, err: errors.New("pane graphics are disabled")},
	} {
		m := simModel(t, f)
		if g != nil {
			m.SetGraphics(g)
		}
		m = press(t, m, "v")
		m = press(t, m, "enter")
		m = send(t, m, simMsg{seq: m.simSeq, kind: sim.KindMerge, res: f.res})

		if m.sim.viaGraphics {
			t.Errorf("%s: the layer was used without power", name)
		}
		if len(m.sim.cells) == 0 {
			t.Errorf("%s: no cell was painted", name)
		}
	}
}

func TestTheRealCellRatioMakesTheImageBigger(t *testing.T) {
	path := writeJPEG(t)
	f := &fakeSimulator{available: true, res: sim.Result{Kind: sim.KindMerge, Path: path}}

	assumed := simModel(t, f)
	assumed = press(t, assumed, "v")
	assumed = press(t, assumed, "enter")
	assumed = send(t, assumed, simMsg{seq: assumed.simSeq, kind: sim.KindMerge, res: f.res})
	assumedW, _ := assumed.simBox()

	measured := simModel(t, f)
	measured.SetGraphics(&fakeGraphics{available: true, cellW: 9, cellH: 19})
	measured = press(t, measured, "v")
	measured = press(t, measured, "enter")
	measured = send(t, measured, simMsg{seq: measured.simSeq, kind: sim.KindMerge, res: f.res})

	if measured.sim.cellW_px != 9 || measured.sim.cellH_px != 19 {
		t.Fatalf("the measured cell was not stored: %dx%d", measured.sim.cellW_px, measured.sim.cellH_px)
	}
	if got, _ := measured.simBox(); got <= assumedW {
		t.Errorf("with the measured cell (9x19) the box measures %d columns, no more than the %d of the assumed 1x2",
			got, assumedW)
	}
}

// The layer lives above the pane's content, so not clearing it on close leaves the image
// over the TUI.
func TestClosingThePopupClearsTheLayer(t *testing.T) {
	path := writeJPEG(t)
	f := &fakeSimulator{available: true, res: sim.Result{Kind: sim.KindMerge, Path: path}}
	g := &fakeGraphics{available: true, cellW: 9, cellH: 19, cleared: make(chan string, 4)}
	m := simModel(t, f)
	m.SetGraphics(g)

	m = press(t, m, "v")
	m = press(t, m, "enter")
	m = send(t, m, simMsg{seq: m.simSeq, kind: sim.KindMerge, res: f.res})
	if g.clears != 0 {
		t.Fatalf("the layer was cleared %d times with the popup open", g.clears)
	}

	m = press(t, m, "esc")
	waitClear(t, g)
	if g.layer != simLayer {
		t.Errorf("the layer %q was removed, want %q", g.layer, simLayer)
	}
}

func TestResizeRepositionsTheLayer(t *testing.T) {
	path := writeJPEG(t)
	f := &fakeSimulator{available: true, res: sim.Result{Kind: sim.KindMerge, Path: path}}
	g := &fakeGraphics{available: true, cellW: 9, cellH: 19}
	m := simModel(t, f)
	m.SetGraphics(g)
	m = press(t, m, "v")
	m = press(t, m, "enter")
	m = send(t, m, simMsg{seq: m.simSeq, kind: sim.KindMerge, res: f.res})
	before := g.sets[len(g.sets)-1]

	m = send(t, m, tea.WindowSizeMsg{Width: 100, Height: 30})
	if len(g.sets) < 2 {
		t.Fatalf("the resize did not relocate the image (%d publications)", len(g.sets))
	}
	after := g.sets[len(g.sets)-1]
	if after == before {
		t.Error("the placement did not change with the resize: the image stayed in the old rectangle")
	}
	if after.col > 100 || after.col+after.cols > 100 {
		t.Errorf("the image leaves the terminal: %+v", after)
	}
}

func TestResizeFallsBackWhenTheLayerGoesAway(t *testing.T) {
	path := writeJPEG(t)
	f := &fakeSimulator{available: true, res: sim.Result{Kind: sim.KindMerge, Path: path}}
	g := &fakeGraphics{available: true, cellW: 9, cellH: 19}
	m := simModel(t, f)
	m.SetGraphics(g)
	m = press(t, m, "v")
	m = press(t, m, "enter")
	m = send(t, m, simMsg{seq: m.simSeq, kind: sim.KindMerge, res: f.res})

	g.err = errors.New("pane not found")
	m = send(t, m, tea.WindowSizeMsg{Width: 100, Height: 30})

	if m.sim.viaGraphics {
		t.Error("it kept believing the image is on the layer")
	}
	if len(m.sim.cells) == 0 {
		t.Error("it did not fall back to half-blocks: the popup was left with a gap where the image went")
	}
}

func TestSimulateDoesNotLeaveGraphicsOn(t *testing.T) {
	path := writeJPEG(t)
	f := &fakeSimulator{available: true, res: sim.Result{Kind: sim.KindMerge, Path: path}}
	m := simModelWith(t, nil)
	m = press(t, m, "v")
	m = press(t, m, "enter")
	m = send(t, m, simMsg{seq: m.simSeq, kind: sim.KindMerge, res: f.res})
	m = press(t, m, "esc")

	if m.sim.state != simClosed {
		t.Errorf("state = %v, want cerrado", m.sim.state)
	}
}
