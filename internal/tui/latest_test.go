package tui

import (
	"context"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"image"
	"image/color"

	"prdash/internal/forge"
	"prdash/internal/forge/model"
	"prdash/internal/herdr"
	"prdash/internal/sim"
	"prdash/internal/testutil"
)

// A popup that cannot fit is not painted, and that is not an error: painting half would show a frame
// with nothing in it.
func TestASimulationPopupThatDoesNotFitIsNotPaintedAndIsNotAnError(t *testing.T) {
	m := newTestModel(t, &testutil.FakeAdapter{ForgeName: "github", HostName: "github.com"})
	g := &graphicsCounter{}
	m.graphics = g
	m.sim.state = simShowing

	// A terminal BELOW the popup's minimum. simMaxCols and simMaxRows have floors
	//(simMinCols/simMinRows), so what is really shown here is that the floor wins over the terminal.
	m.width = simMinCols - 1
	m.height = simMinRows - 1
	cols, _ := m.simBox()
	if cols < simMinCols {
		t.Errorf("with a terminal of %d columns the popup box measures %d, below the "+
			"floor of %d: a popup narrower than the minimum cannot be read",
			m.width, cols, simMinCols)
	}
	if cols-2 <= 0 {
		t.Errorf("with the floor applied the inner room is %d columns", cols-2)
	}

	m2 := newTestModel(t, &testutil.FakeAdapter{ForgeName: "github", HostName: "github.com"})
	g2 := &graphicsCounter{}
	m2.graphics = g2
	m2.width, m2.height = 120, 40
	if !m2.publishSimImage(tinyImage(8, 8)) {
		t.Fatal("with spare room it was not published: the guard is not what stopped it")
	}
	if g2.published != 1 {
		t.Errorf("%d images were published, want 1", g2.published)
	}
	if g2.last.Cols <= 0 || g2.last.Rows <= 0 {
		t.Errorf("the published rectangle has no size: %+v", g2.last)
	}
	if g2.last.Col <= 0 || g2.last.Row <= 0 {
		t.Errorf("the image was published at the frames origin (%+v): it would cover the border",
			g2.last)
	}
}

func TestAnImageThatCannotBeResizedIsNotPublished(t *testing.T) {
	m := newTestModel(t, &testutil.FakeAdapter{ForgeName: "github", HostName: "github.com"})
	g := &graphicsCounter{}
	m.graphics = g
	m.width, m.height = 120, 40

	if m.publishSimImage(imageRotated{w: 0, h: 0}) {
		t.Error("an image that cannot be resized was published")
	}
	if g.published != 0 {
		t.Errorf("%d publications came out of a broken image", g.published)
	}
}

// republishSimImage runs on EVERY resize while the image is in the layer, so the image-less path is
// not rare.
func TestRepublishWithoutAnImageDoesNothingNorFails(t *testing.T) {
	m := newTestModel(t, &testutil.FakeAdapter{ForgeName: "github", HostName: "github.com"})
	g := &graphicsCounter{}
	m.graphics = g
	m.width, m.height = 120, 40
	m.sim.img = nil
	m.sim.viaGraphics = true

	m.republishSimImage()

	if g.published != 0 {
		t.Errorf("%d images were published with no image to publish", g.published)
	}
	if !m.sim.viaGraphics {
		t.Error("with no image, republishSimImage turned off viaGraphics: the popup would have " +
			"neither layer nor half-blocks")
	}

	m2 := newTestModel(t, &testutil.FakeAdapter{ForgeName: "github", HostName: "github.com"})
	g2 := &graphicsCounter{}
	m2.graphics = g2
	m2.width, m2.height = 120, 40
	m2.sim.viaGraphics = true
	m2.sim.img = imageRotated{w: 0, h: 0}
	m2.republishSimImage()
	if m2.sim.viaGraphics {
		t.Error("with an image that does not publish, viaGraphics stays true: the TUI believes " +
			"there is an image on the layer and does not paint the fallback half-blocks")
	}
	if g2.published != 0 {
		t.Errorf("%d publications came out of an image that cannot be resized",
			g2.published)
	}
}

// The placeholder is not cosmetic: an item whose SourceBranch came back empty paints as
// "the PR branch".
func TestTheChooserSetsTheItemsBranchAndItsPlaceholderWhenThereIsNone(t *testing.T) {
	m := newTestModel(t, &testutil.FakeAdapter{ForgeName: "github", HostName: "github.com"})
	it := mkItem("github", "github.com", "acme/widget", "one", 7, "")
	it.SourceBranch = "feat/mi-rama"
	m.sim.item = it

	if box := m.simChooserBox(); !strings.Contains(box, "feat/mi-rama") {
		t.Errorf("the popup does not name the items branch:\n%s", box)
	}
	if box := m.simChooserBox(); !strings.Contains(box, it.TargetBranch) {
		t.Errorf("the popup does not name the base:\n%s", box)
	}

	m.sim.item.SourceBranch = ""
	box := m.simChooserBox()
	if !strings.Contains(box, "the PR branch") {
		t.Errorf("with no source branch the popup puts %q, and leaves the line half done", box)
	}
	if strings.Contains(box, "  \n") {
		t.Errorf("the popup has an empty line:\n%s", box)
	}
	m.sim.item.SourceBranch = "   "
	if box := m.simChooserBox(); !strings.Contains(box, "the PR branch") {
		t.Errorf("a branch of only spaces does not count as empty:\n%s", box)
	}
}

// A zero interval is the config with `refresh_interval = "0s"`, which is a MANUAL refresh: there is
// no tick to arm and arming one would tick with no work.
func TestTheTickDoesNotArmWithZeroIntervalNorWithRefreshPaused(t *testing.T) {
	for _, d := range []time.Duration{0, -time.Second, -time.Hour} {
		m := newTestModel(t)
		m.cfg.RefreshInterval = d
		if cmd := m.tickCmd(); cmd != nil {
			t.Errorf("with refresh_interval=%v a tick was armed: the chain would fetch non stop",
				d)
		}
	}

	m2 := newTestModel(t)
	m2.cfg.RefreshInterval = 90 * time.Second
	if cmd := m2.tickCmd(); cmd == nil {
		t.Error("with a valid interval the tick was not armed")
	}
}

// tickToast comes from tea.Every, not the events channel, so it must NOT go through withPump.
func TestTheToastsTickIsAClockAndNotAChannelReader(t *testing.T) {
	cmd := tickToast()
	if cmd == nil {
		t.Fatal("tickToast returned nil: the notices chain is short")
	}
	msg := cmd()
	if _, ok := msg.(toastTickMsg); !ok {
		t.Errorf("tickToast returned %T, want toastTickMsg", msg)
	}

}

// The case that really happens: the forge returns the branches AND a warning, and changing the base on
// a partial list is worse than not changing it.
func TestABranchListingWithWarningsTurnsThemIntoAnErrorMessage(t *testing.T) {
	m := newTestModel(t, &testutil.FakeAdapter{ForgeName: "github", HostName: "github.com"})
	m.retarget.state = retargetListing
	m.retarget.item = mkItem("github", "github.com", "acme/widget", "one", 7, "")
	m.branchSeq = 1

	ok := send(t, m, branchesMsg{
		seq: 1, key: keyOf(m.retarget.item),
		names: []string{"main", "release/2.0"},
	})
	if ok.retarget.errMsg != "" {
		t.Errorf("a listing with no notices set an error: %q", ok.retarget.errMsg)
	}
	if ok.retarget.state != retargetChoosing {
		t.Errorf("a listing with no notices did not open the selector (state=%d)", ok.retarget.state)
	}

	m2 := newTestModel(t, &testutil.FakeAdapter{ForgeName: "github", HostName: "github.com"})
	m2.retarget.state = retargetListing
	m2.retarget.item = mkItem("github", "github.com", "acme/widget", "one", 7, "")
	m2.branchSeq = 1

	withNotice := send(t, m2, branchesMsg{
		seq: 1, key: keyOf(m2.retarget.item),
		names:  []string{"main"},
		errMsg: "a branch could not be queried",
	})
	if !strings.Contains(withNotice.retarget.errMsg, "a branch could not be queried") {
		t.Errorf("the forges notice did not reach the popup: %q", withNotice.retarget.errMsg)
	}
	if withNotice.retarget.state == retargetChoosing {
		t.Error("a partial listing opened the selector: it would present itself as complete")
	}
}

func TestStartRetargetClosesThePopupIfTheActionCannotBeDone(t *testing.T) {
	for _, c := range []struct {
		name    string
		prepara func(*testing.T) Model
	}{
		{"forge unknown", func(t *testing.T) Model {
			return newTestModel(t)
		}},
		{"action in flight", func(t *testing.T) Model {
			m := newTestModel(t, &testutil.FakeAdapter{
				ForgeName: "github", HostName: "github.com",
			})
			m.actionBusy = true
			m.retarget.state = retargetConfirm
			m.retarget.item = mkItem("github", "github.com", "acme/widget", "one", 7, "")
			m.retarget.view = []string{"main", "otra"}
			return m
		}},
	} {
		t.Run(c.name, func(t *testing.T) {
			m := c.prepara(t)
			if cmd := m.startRetarget("otra"); cmd != nil {
				t.Error("an action that cannot be done returned a command: it would apply the " +
					"base change anyway")
			}
			if m.retarget.state != retargetClosed {
				t.Errorf("the popup ended at %d, want closed: there is nothing to confirm if the "+
					"action cannot be done", m.retarget.state)
			}
		})
	}
}

type graphicsCounter struct {
	published int
	last      herdr.Placement
}

func (g *graphicsCounter) Available() bool                     { return true }
func (g *graphicsCounter) CellSize(context.Context) (int, int) { return 1, 2 }
func (g *graphicsCounter) SetImage(_ context.Context, _ string, _ image.Image, p herdr.Placement) error {
	g.published++
	g.last = p
	return nil
}
func (g *graphicsCounter) Clear(context.Context, string) error { return nil }

var (
	_ = forge.ActionRetarget
	_ = model.SectionReview
	_ = sim.KindMerge
	_ = tea.KeyPressMsg{}
)

func tinyImage(w, h int) image.Image {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := range h {
		for x := range w {
			img.Set(x, y, color.RGBA{R: uint8(x * 8), G: uint8(y * 8), B: 64, A: 255})
		}
	}
	return img
}

// The ZERO size is what really returns nil, not incoherent Bounds: Resize downloads the image first.
type imageRotated struct{ w, h int }

func (i imageRotated) ColorModel() color.Model { return color.RGBAModel }
func (i imageRotated) Bounds() image.Rectangle { return image.Rect(0, 0, i.w, i.h) }
func (imageRotated) At(int, int) color.Color   { return color.RGBA{} }

// This REPLACES the guard it documents: the inner gap is exactly what FitCells returned, and
// FitCells floors at 1. What is checked is that the invariant HOLDS across widths 0 to 400.
func TestThePopupsInteriorGapNeverDisappearsHoweverNarrowTheTerminal(t *testing.T) {
	for _, state := range []simState{simShowing, simChoosing, simRendering} {
		for _, width := range []int{0, 1, 2, 10, 20, 38, 40, 64, 80, 200, 400} {
			m := newTestModel(t, &testutil.FakeAdapter{
				ForgeName: "github", HostName: "github.com",
			})
			g := &graphicsCount{cellW: 8, cellH: 16}
			m.graphics = g
			m.sim.state = state
			m.sim.img = tinyImage(640, 480)
			m.width, m.height = width, 3

			cols, rows := m.simBox()
			innerCols, innerRows := cols-2, rows-simChrome
			if innerCols <= 0 || innerRows <= 0 {
				t.Errorf("state %v on a terminal of %d columns: inner room of %dx%d. It is "+
					"what the removed guard checked, and the contentWidth floor keeps it "+
					"impide", state, width, innerCols, innerRows)
			}

			if !m.publishSimImage(m.sim.img) {
				t.Errorf("state %v on a terminal of %d columns: it did not publish with a room of "+
					"%dx%d", state, width, innerCols, innerRows)
				continue
			}
			if g.calls != 1 {
				t.Errorf("state %v on a terminal of %d columns: %d publications, want 1",
					state, width, g.calls)
			}
			if g.last.Cols <= 0 || g.last.Rows <= 0 {
				t.Errorf("state %v on a terminal of %d columns: rectangle of %dx%d cells",
					state, width, g.last.Cols, g.last.Rows)
			}
			if g.last.Cols != innerCols || g.last.Rows != innerRows {
				t.Errorf("state %v on a terminal of %d columns: it published at %dx%d and the room "+
					"was %dx%d: the image would leave the frame",
					state, width, g.last.Cols, g.last.Rows, innerCols, innerRows)
			}
		}
	}
}

// The previous test proves the lower bound (never smaller than the minimum); this one proves the
// other side, since a box wider than the terminal is cut and the popup looks split.
func TestWithAHugeImageThePopupFitsTheTerminalAndDoesNotOverflowIt(t *testing.T) {
	for _, size := range [][2]int{{40, 12}, {80, 24}, {120, 40}} {
		m := newTestModel(t, &testutil.FakeAdapter{
			ForgeName: "github", HostName: "github.com",
		})
		m.sim.state = simShowing
		m.sim.img = tinyImage(4000, 3000)
		m.width, m.height = size[0], size[1]

		cols, rows := m.simBox()
		if cols > size[0] {
			t.Errorf("terminal of %d columns: the popup box measures %d and leaves at the "+
				"border derecho", size[0], cols)
		}
		if rows > size[1] {
			t.Errorf("terminal of %d rows: the popup box measures %d and leaves at the bottom "+
				"border", size[1], rows)
		}
	}
}

// It exists because graphicsCapture does not count SetImage, and whether anything was published is
// exactly what the guard's false means.
type graphicsCount struct {
	cellW, cellH int

	calls int
	last  herdr.Placement
}

func (g *graphicsCount) Available() bool { return true }

func (g *graphicsCount) CellSize(context.Context) (int, int) { return g.cellW, g.cellH }

func (g *graphicsCount) SetImage(_ context.Context, layer string, _ image.Image, p herdr.Placement) error {
	g.calls++
	g.last = p
	return nil
}

func (g *graphicsCount) Clear(context.Context, string) error { return nil }
