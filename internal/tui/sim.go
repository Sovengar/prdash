package tui

import (
	"context"
	"image"
	"os/exec"
	"path/filepath"
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

const simTimeout = time.Duration(90e9)

const simLayer = herdr.GraphicsLayer

const (
	simChooserWidth = 64
	simChrome       = 3
	simHeightNum    = 3
	simHeightDen    = 4
	simMargin       = 2
	simSideMargin   = 4
	simMinCols      = 20
	simMinRows      = 6
)

type simState int

const (
	simClosed simState = iota
	simChoosing
	simRendering
	simShowing
)

var simKinds = []sim.Kind{sim.KindMerge}

type simPanel struct {
	state       simState
	item        model.Item
	cursor      int
	kind        sim.Kind
	kinds       []sim.Kind
	img         image.Image
	cells       []string
	cellW       int
	cellH       int
	image       string
	viaGraphics bool
	// Zero means unknown, and then 1x2 is assumed.
	cellW_px int
	cellH_px int
}

type Simulator interface {
	Available() bool
	Simulate(ctx context.Context, it model.Item, kind sim.Kind) (sim.Result, error)
}

// KindLister is optional: only the real service knows which strategies its git-sim can draw.
type KindLister interface {
	Kinds() []sim.Kind
}

type Graphics interface {
	Available() bool
	// (0, 0) when unknown.
	CellSize(ctx context.Context) (cellW, cellH int)
	// Placement is Herdr's type because a pane is a grid: the image is placed in cells, not pixels.
	SetImage(ctx context.Context, layer string, img image.Image, p herdr.Placement) error
	Clear(ctx context.Context, layer string) error
}

type simMsg struct {
	seq  int
	kind sim.Kind
	res  sim.Result
	err  error
}

func (m *Model) SetSimulator(s Simulator) { m.simulator = s }

func (m *Model) SetGraphics(g Graphics) { m.graphics = g }

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
	m.disarmMerge()
	m.sim = simPanel{state: simChoosing, item: it, kinds: m.availableSimKinds()}
	return m, nil
}

func (m Model) availableSimKinds() []sim.Kind {
	if lister, ok := m.simulator.(KindLister); ok {
		if kinds := lister.Kinds(); len(kinds) > 0 {
			return kinds
		}
	}
	return simKinds
}

// simKindsNow tolerates a panel opened directly, as tests do, without a strategy list.
func (m Model) simKindsNow() []sim.Kind {
	if len(m.sim.kinds) > 0 {
		return m.sim.kinds
	}
	return simKinds
}

func (m *Model) moveSimCursor(key string) bool {
	kinds := m.simKindsNow()
	if len(kinds) < 2 {
		return false
	}
	switch key {
	case "up", "k":
		m.sim.cursor = (m.sim.cursor - 1 + len(kinds)) % len(kinds)
	case "down", "j", "right", "tab":
		m.sim.cursor = (m.sim.cursor + 1) % len(kinds)
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

func (m Model) handleSimKey(msg tea.KeyPressMsg, key string) (tea.Model, tea.Cmd) {
	switch key {
	case "q", "ctrl+c":
		m.closeSim()
		m.cancel()
		return m, tea.Quit
	case "o":
		if m.sim.state == simShowing && m.sim.image != "" {
			return m, m.openSimPageCmd(m.sim.image)
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
			return m, m.startSim(m.simKindsNow()[m.sim.cursor])
		}
		m.closeSim()
		return m, nil

	default:
		m.closeSim()
		return m, nil
	}
}

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

func (m *Model) applySim(msg simMsg) tea.Cmd {
	if msg.seq != m.simSeq {
		return nil
	}
	if msg.err != nil {
		m.closeSim()
		m.setNotice("simulate "+string(msg.kind)+": "+msg.err.Error(), levelError)
		return nil
	}
	if strings.HasSuffix(msg.res.Path, ".html") {
		m.sim.state = simShowing
		m.sim.image = msg.res.Path
		m.sim.img = nil
		return m.openSimPageCmd(msg.res.Path)
	}
	img, err := sim.Load(msg.res.Path)
	if err != nil {
		m.closeSim()
		m.setNotice("simulate: "+err.Error(), levelError)
		return nil
	}
	m.sim.state = simShowing
	m.sim.image = msg.res.Path
	m.sim.img = img
	if !m.publishSimImage(img) {
		m.renderSimCells()
	}
	return nil
}

func (m *Model) openSimPageCmd(path string) tea.Cmd {
	return func() tea.Msg {
		if err := openSimPage(path); err != nil {
			return notifyMsg{text: "simulate: " + err.Error(), level: levelWarn}
		}
		return notifyMsg{text: "opened " + filepath.Base(path), level: levelInfo}
	}
}

// The HTML page opens beside the TUI; without terminal-browser the desktop browser is the honest fallback.
func openSimPage(path string) error {
	if _, err := exec.LookPath("terminal-browser"); err == nil {
		cmd := exec.Command("terminal-browser", "open", path, "--split", "right")
		if err := cmd.Start(); err != nil {
			return err
		}
		return cmd.Process.Release()
	}
	cmd := exec.Command("xdg-open", path)
	if err := cmd.Start(); err != nil {
		return err
	}
	return cmd.Process.Release()
}

func (m *Model) publishSimImage(img image.Image) bool {
	if m.graphics == nil || !m.graphics.Available() {
		return false
	}
	cols, rows := m.simBox()
	col, row := m.simBoxOrigin(cols, rows)
	innerCols, innerRows := cols-2, rows-simChrome
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cellW, cellH := m.graphics.CellSize(ctx)
	if cellW <= 0 || cellH <= 0 {
		cellW, cellH = 1, 2
	}
	m.sim.cellW_px, m.sim.cellH_px = cellW, cellH
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

func (m *Model) republishSimImage() {
	img := m.sim.img
	if img == nil {
		return
	}
	m.sim.viaGraphics = false
	if !m.publishSimImage(img) {
		m.renderSimCells()
	}
}

func (m Model) simBox() (w, h int) {
	switch m.sim.state {
	case simChoosing:
		return min(m.contentWidth(), simChooserWidth), simChrome + 2
	case simRendering:
		return min(m.contentWidth(), simChooserWidth), simChrome + 2
	}

	if m.sim.img == nil {
		return min(m.contentWidth(), simChooserWidth), simChrome + 2
	}
	cellW, cellH := m.cellSize()
	cols, rows := sim.FitCells(m.sim.img, cellW, cellH, m.simMaxCols(), m.simMaxRows())
	return cols + 2, rows + simChrome
}

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
	w, h := cols-2, rows-simChrome
	if m.sim.cells != nil && m.sim.cellW == w && m.sim.cellH == h {
		return
	}
	m.sim.cells = sim.Cells(m.sim.img, w, h)
	m.sim.cellW, m.sim.cellH = w, h
}

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

func (m Model) simChooserBox() string {
	width, _ := m.simBox()
	it := m.sim.item
	branch := strings.TrimSpace(it.SourceBranch)
	if branch == "" {
		branch = "the PR branch"
	}
	base := it.TargetBranch

	lines := make([]string, 0, 4)
	for i, kind := range m.simKindsNow() {
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
	if m.sim.img == nil {
		body := padLines([]string{
			styleInfo.Render("opened in terminal-browser"),
			styleDim.Render(filepath.Base(m.sim.image)),
		}, height-simChrome)
		body = append(body, styleDim.Render("esc close · o reopen"))
		return borderedBox(" simulate: "+string(m.sim.kind)+" "+refLabel(m.sim.item), strings.Join(body, "\n"), width)
	}
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
	return s + strings.Repeat(" ", max(0, n-ansi.StringWidth(s)))
}
