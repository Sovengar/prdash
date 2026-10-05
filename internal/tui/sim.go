// Simulation overlay: a popup that picks the command, renders it with git-sim and shows the image
// over the inbox. In the same view, because hiding it hides what you should be looking at.
package tui

import (
	"context"
	"image"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"prdash/internal/forge/model"
	"prdash/internal/herdr"
	"prdash/internal/sim"
	"prdash/internal/tui/bordered"
)

const simTimeout = 90 * time.Second

const simLayer = herdr.GraphicsLayer

const (
	simChooserWidth = 64
	simChrome       = 3
	// Numerator and denominator rather than a division: in a Go constant 3/4 is 0 and the popup would
	// fall back to its floor. Not 100%, because losing the inbox loses the context.
	simHeightNum  = 3
	simHeightDen  = 4
	simMargin     = 2
	simSideMargin = 4
	simMinCols    = 20
	simMinRows    = 6
)

type simState int

const (
	simClosed simState = iota
	simChoosing
	simRendering
	simShowing
)

// Rebase is excluded on purpose, not out of caution: git-sim 0.3.5 cannot draw it. Merge works in
// all three cases, and this list is the only thing to touch when the project fixes it.
var simKinds = []sim.Kind{sim.KindMerge}

// In the Model rather than apart, because it shares its lifecycle: opened by a key, alive while the
// render runs, closed by another key.
type simPanel struct {
	state  simState
	item   model.Item
	cursor int
	kind   sim.Kind
	// Kept so a resize does not re-read the JPEG, and the size is all a resize changes.
	img   image.Image
	cells []string
	cellW int
	cellH int
	image string
	// The layer sits above the pane's content: if the popup closes without removing it, the image stays on
	// top of the UI.
	viaGraphics bool
	// Zero means unknown, and then 1x2 is assumed.
	cellW_px int
	cellH_px int
}

type Simulator interface {
	Available() bool
	Simulate(ctx context.Context, it model.Item, kind sim.Kind) (sim.Result, error)
}

// The port that decides quality: without it the image is drawn with half-blocks and looks
// pixelated, because it quantises to the cell grid. With it the terminal scales it.
type Graphics interface {
	Available() bool
	// (0, 0) when unknown.
	CellSize(ctx context.Context) (cellW, cellH int)
	// Placement is Herdr's type because it is Herdr's concept: a pane is a grid and the image is placed
	// in cells, not pixels.
	SetImage(ctx context.Context, layer string, img image.Image, p herdr.Placement) error
	Clear(ctx context.Context, layer string) error
}

// seq identifies the request, so a render that arrives late with the popup closed or reopened is
// discarded instead of jumping to the screen.
type simMsg struct {
	seq  int
	kind sim.Kind
	res  sim.Result
	err  error
}

func (m *Model) SetSimulator(s Simulator) { m.simulator = s }

func (m *Model) SetGraphics(g Graphics) { m.graphics = g }

// With a merge armed this key does not get here: the second press of a merge can only be a mode, so the
// `v` disarms and is consumed, which avoids an out-of-time press opening a modal popup.
func (m Model) openSimulator() (tea.Model, tea.Cmd) {
	if m.simulator == nil || !m.simulator.Available() {
		m.setNotice("simulate: git-sim is not installed", levelWarn)
		return m, nil
	}
	it, ok := m.selected()
	if !ok {
		m.setNotice("select an item first", levelWarn)
		return m, nil
	}
	if strings.TrimSpace(it.TargetBranch) == "" {
		m.setNotice("simulate: the forge reports no target branch for "+refLabel(it), levelWarn)
		return m, nil
	}
	// Disarming is explicit even though an armed merge is already resolved here: if the action is
	// ever routed from elsewhere, the popup must not coexist with a merge confirmation.
	m.disarmMerge()
	m.sim = simPanel{state: simChoosing, item: it}
	return m, nil
}

// Navigates only when there is something to walk: with a single strategy an arrow has to fall
// through to the default instead of being swallowed.
func (m *Model) moveSimCursor(key string) bool {
	if len(simKinds) < 2 {
		return false
	}
	switch key {
	case "up", "k":
		m.sim.cursor = (m.sim.cursor - 1 + len(simKinds)) % len(simKinds)
	case "down", "j", "right", "tab":
		m.sim.cursor = (m.sim.cursor + 1) % len(simKinds)
	default:
		return false
	}
	return true
}

func (m *Model) closeSim() {
	m.simSeq++
	m.releaseSimLayer()
	m.sim = simPanel{}
}

// In the background with its own context: if the popup closes on TUI exit, the app's context is already
// cancelled and the image would stay stuck.
func (m *Model) releaseSimLayer() {
	if !m.sim.viaGraphics || m.graphics == nil {
		return
	}
	g := m.graphics
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = g.Clear(ctx, simLayer)
	}()
}

// `q` and `ctrl+c` quit, as in the rest of the view: closing with esc and quitting with q are two
// different intentions.

// Any other key closes the popup, and while a strategy is being picked only the ones naming one count. Same
// policy as the armed merge, for the same reason: a press that is not a choice must not land on an action.
func (m Model) handleSimKey(msg tea.KeyPressMsg, key string) (tea.Model, tea.Cmd) {
	switch key {
	case "q", "ctrl+c":
		m.closeSim()
		m.cancel()
		return m, tea.Quit
	case "o":
		if m.sim.state == simShowing && m.sim.image != "" {
			return m, m.openBrowserCmd(m.sim.image)
		}
		m.closeSim()
		return m, nil
	case "esc":
		m.closeSim()
		return m, nil
	}

	switch m.sim.state {
	case simChoosing:
		if m.moveSimCursor(key) {
			return m, nil
		}
		if key == "enter" {
			return m, m.startSim(simKinds[m.sim.cursor])
		}
		m.closeSim()
		return m, nil

	default:
		// While rendering or showing the image the popup waits for no particular key, so any press closes it.
		m.closeSim()
		return m, nil
	}
}

// In the background because blocking the update loop reads as a freeze, which is the one thing
// an overlay cannot do. The result goes to the events channel like any other work.
func (m *Model) startSim(kind sim.Kind) tea.Cmd {
	m.sim.state = simRendering
	m.sim.kind = kind
	m.simSeq++
	seq := m.simSeq

	appCtx, events, sim := m.ctx, m.events, m.simulator
	it := m.sim.item
	go func() {
		ctx, cancel := context.WithTimeout(appCtx, simTimeout)
		defer cancel()
		res, err := sim.Simulate(ctx, it, kind)
		sendEvent(appCtx, events, simMsg{seq: seq, kind: kind, res: res, err: err})
	}()
	return nil
}

// A failure closes the popup and warns in the header, where warnings live: a popup explaining its own
// error on top of the view is harder to read than the warning.
func (m *Model) applySim(msg simMsg) {
	if msg.seq != m.simSeq {
		return // stale request: the popup closed or reopened while it ran
	}
	if msg.err != nil {
		m.closeSim()
		m.setNotice("simulate "+string(msg.kind)+": "+msg.err.Error(), levelError)
		return
	}
	img, err := sim.Load(msg.res.Path)
	if err != nil {
		m.closeSim()
		m.setNotice("simulate: "+err.Error(), levelError)
		return
	}
	m.sim.state = simShowing
	m.sim.image = msg.res.Path
	m.sim.img = img
	if !m.publishSimImage(img) {
		m.renderSimCells()
	}
}

// Rescaled to the rectangle's pixel size before being sent, because that is the size the terminal
// draws it at. It also fits the cell's real height, not twice its width.
func (m *Model) publishSimImage(img image.Image) bool {
	if m.graphics == nil || !m.graphics.Available() {
		return false
	}
	cols, rows := m.simBox()
	col, row := m.simBoxOrigin(cols, rows)
	// The inner gap is not checked against zero because it is unreachable: `simBox` adds exactly
	// what is subtracted here, and the floor that closes the selector case is `contentWidth`'s.
	innerCols, innerRows := cols-2, rows-simChrome
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cellW, cellH := m.graphics.CellSize(ctx)
	if cellW <= 0 || cellH <= 0 {
		cellW, cellH = 1, 2
	}
	m.sim.cellW_px, m.sim.cellH_px = cellW, cellH
	// The rectangle in pixels, with the measured cell: sending the original image here would only add
	// bytes, since the terminal draws it at this size.
	resized := sim.Resize(img, innerCols*cellW, innerRows*cellH)
	if resized == nil {
		return false
	}
	if err := m.graphics.SetImage(ctx, simLayer, resized, herdr.Placement{
		Col: col + 1, Row: row + 1, Cols: innerCols, Rows: innerRows,
	}); err != nil {
		return false
	}
	m.sim.viaGraphics = true
	m.sim.cells, m.sim.cellW, m.sim.cellH = nil, 0, 0
	return true
}

// On every resize: the placement is in cells, so it changes with the terminal size exactly like the
// frame does.
func (m *Model) republishSimImage() {
	img := m.sim.img
	if img == nil {
		return
	}
	m.sim.viaGraphics = false
	if !m.publishSimImage(img) {
		// If the layer stops being available (hidden pane, unresponsive Herdr) it falls back to
		// half-blocks rather than leaving a hole.
		m.renderSimCells()
	}
}

// One source, not puritanism: when the two widths were decided separately the image drew at 94
// columns inside a 200-column box and looked like part of the image on its right.
func (m Model) simBox() (w, h int) {
	switch m.sim.state {
	case simChoosing:
		return min(m.contentWidth(), simChooserWidth), simChrome + 2
	case simRendering:
		return min(m.contentWidth(), simChooserWidth), simChrome + 2
	}

	// The image decides: the box fits what the image needs, not the other way round.
	cellW, cellH := m.cellSize()
	cols, rows := sim.FitCells(m.sim.img, cellW, cellH, m.simMaxCols(), m.simMaxRows())
	return cols + 2, rows + simChrome
}

// Herdr measures them (9x19 with the default kitty font, not 2x1) and using them does two things
// at once: the image does not distort and no invisible resolution is sent.
func (m Model) cellSize() (w, h int) {
	if m.sim.cellW_px > 0 && m.sim.cellH_px > 0 {
		return m.sim.cellW_px, m.sim.cellH_px
	}
	return 1, 2
}

func (m Model) simBoxOrigin(cols, rows int) (col, row int) {
	return centeredOrigin(m.contentWidth(), m.height, cols, rows)
}

func (m Model) simMaxCols() int {
	return max(simMinCols, m.contentWidth()-simSideMargin)
}

func (m Model) simMaxRows() int {
	free := m.height - 2*simMargin
	return max(simMinRows, free*simHeightNum/simHeightDen)
}

func (m *Model) renderSimCells() {
	if m.sim.state != simShowing || m.sim.img == nil || m.sim.viaGraphics {
		m.sim.cells, m.sim.cellW, m.sim.cellH = nil, 0, 0
		return
	}
	cols, rows := m.simBox()
	// The inner gap is not checked against zero and used to be: it is unreachable, `FitCells` floors
	// both dimensions at 1, and `sim.Cells` returns nil below 1 anyway.
	w, h := cols-2, rows-simChrome
	if m.sim.cells != nil && m.sim.cellW == w && m.sim.cellH == h {
		return
	}
	m.sim.cells = sim.Cells(m.sim.img, w, h)
	m.sim.cellW, m.sim.cellH = w, h
}

// The width it centres in is the VIEW's, not the box's: they are different things, and confusing them is
// what left the image in a corner.
func (m Model) simOverlay() (string, bool) {
	switch m.sim.state {
	case simChoosing:
		return m.simChooserBox(), true
	case simRendering:
		return m.simBusyBox(), true
	case simShowing:
		return m.simImageBox(), true
	default:
		return "", false
	}
}

// No default mode, like the merge: the simulation shown has to be the one the user wanted. With a
// single strategy the popup confirms instead of asking, which is what makes it worth having.
func (m Model) simChooserBox() string {
	width, _ := m.simBox()
	it := m.sim.item
	branch := strings.TrimSpace(it.SourceBranch)
	if branch == "" {
		branch = "the PR branch"
	}
	base := it.TargetBranch

	lines := make([]string, 0, 4)
	for i, kind := range simKinds {
		flow := base + " ← " + branch
		if kind == sim.KindRebase {
			flow = branch + " → " + base
		}
		marker := "  "
		if i == m.sim.cursor {
			marker = styleCursor.Render("▸ ")
		}
		lines = append(lines, marker+styleRef.Render(padRight(string(kind), 8))+styleDim.Render(flow))
	}
	lines = append(lines, styleDim.Render("enter render · esc close"))

	return borderedBox(" simulate "+refLabel(it), strings.Join(lines, "\n"), width)
}

func (m Model) simBusyBox() string {
	width, _ := m.simBox()
	body := m.spinner.View() + styleInfo.Render(" rendering "+string(m.sim.kind)+"…")
	body += "\n" + styleDim.Render("this takes a couple of seconds · esc close")
	return borderedBox(" simulate: "+string(m.sim.kind), body, width)
}

func (m Model) simImageBox() string {
	width, height := m.simBox()
	// Filled to the height MINUS the frame and the footer line, which is what simChrome measures. It
	// used to be one row less, and that row shows in the centring and the image placement.
	body := padLines(m.sim.cells, height-simChrome)
	body = append(body, styleDim.Render("esc close · o open image"))
	return borderedBox(" simulate: "+string(m.sim.kind)+" "+refLabel(m.sim.item), strings.Join(body, "\n"), width)
}

func padLines(lines []string, want int) []string {
	out := make([]string, 0, max(0, want))
	out = append(out, lines...)
	for len(out) < want {
		out = append(out, "")
	}
	return out
}

var simBorder = lipgloss.Color("39")

func borderedBox(title, content string, width int) string {
	return bordered.RenderWithTitle(bordered.Rounded(), simBorder, title, content, width)
}

func padRight(s string, n int) string {
	if pad := n - ansi.StringWidth(s); pad > 0 {
		return s + strings.Repeat(" ", pad)
	}
	return s
}
