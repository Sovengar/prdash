package tui

import (
	"context"
	"image"
	"image/color"
	"image/jpeg"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"prdash/internal/forge/model"
	"prdash/internal/sim"
)

type fakeSimulator struct {
	available bool
	res       sim.Result
	err       error
	mu        sync.Mutex
	kinds     []sim.Kind
	items     []model.Item
	delay     time.Duration
}

func (f *fakeSimulator) Available() bool { return f.available }

func (f *fakeSimulator) Simulate(_ context.Context, it model.Item, kind sim.Kind) (sim.Result, error) {
	f.mu.Lock()
	f.kinds = append(f.kinds, kind)
	f.items = append(f.items, it)
	delay := f.delay
	f.mu.Unlock()
	if delay > 0 {
		time.Sleep(delay)
	}
	return f.res, f.err
}

func simModel(t *testing.T, s Simulator) Model {
	t.Helper()
	ad := ghAdapter()
	m := newTestModel(t, ad)
	m.SetSimulator(s)
	it := mkItem("github", "github.com", "acme/widget", "Fix the thing", 7, "")
	m = send(t, m, page(1, "github", "github.com", model.SectionReview, model.ReviewRequested, []model.Item{it}, false))
	return m
}

func writeJPEG(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "sim.jpg")
	img := image.NewRGBA(image.Rect(0, 0, 16, 16))
	for y := range 16 {
		for x := range 16 {
			img.SetRGBA(x, y, color.RGBA{R: uint8(x * 16), G: 120, B: uint8(y * 16), A: 255})
		}
	}
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	if err := jpeg.Encode(f, img, nil); err != nil {
		t.Fatal(err)
	}
	return path
}

func viewText(m Model) string { return stripANSI(m.View().Content) }

func TestSimulateWithoutGitSimInforms(t *testing.T) {
	m := simModel(t, nil)
	m = press(t, m, "v")

	if m.sim.state != simClosed {
		t.Errorf("state = %v, want cerrado", m.sim.state)
	}
	if !strings.Contains(lastToast(m), "git-sim") {
		t.Errorf("notice = %q, want un aviso sobre git-sim", lastToast(m))
	}
}

func TestSimulateOpensTheChooser(t *testing.T) {
	m := simModel(t, &fakeSimulator{available: true})
	m = press(t, m, "v")

	if m.sim.state != simChoosing {
		t.Fatalf("state = %v, want simChoosing", m.sim.state)
	}
	if m.sim.item.Number != 7 {
		t.Errorf("ítem = %d, want 7", m.sim.item.Number)
	}
	text := viewText(m)
	for _, want := range []string{"simulate", "merge", "main ← feat/x", "enter render"} {
		if !strings.Contains(text, want) {
			t.Errorf("la vista no menciona %q:\n%s", want, text)
		}
	}
}

// Rebase is not offered because git-sim 0.3.5 cannot draw it.
func TestChooserOnlyOffersWhatGitSimCanRender(t *testing.T) {
	m := simModel(t, &fakeSimulator{available: true})
	m = press(t, m, "v")

	if len(simKinds) != 1 || simKinds[0] != sim.KindMerge {
		t.Fatalf("simKinds = %v, want solo merge", simKinds)
	}
	if text := viewText(m); strings.Contains(text, "rebase") {
		t.Errorf("el selector ofrece rebase, que git-sim no sabe dibujar:\n%s", text)
	}
}

func TestChooserLeavesNothingToAccidentallyConfirm(t *testing.T) {
	f := &fakeSimulator{available: true}
	m := simModel(t, f)
	m = press(t, m, "v")

	if m.sim.cursor != 0 {
		t.Errorf("cursor = %d, want el primero", m.sim.cursor)
	}
	if m.sim.kind != "" {
		t.Errorf("kind = %q, want ninguno hasta que se confirme", m.sim.kind)
	}
	if len(f.kinds) != 0 {
		t.Errorf("abriendo el selector ya se renderizó: %v", f.kinds)
	}
}

func TestEnterStartsTheChosenStrategy(t *testing.T) {
	f := &fakeSimulator{available: true}
	m := simModel(t, f)
	m = press(t, m, "v")
	m = press(t, m, "enter")

	if m.sim.state != simRendering {
		t.Errorf("state = %v, want simRendering", m.sim.state)
	}
	if m.sim.kind != sim.KindMerge {
		t.Errorf("kind = %q, want merge", m.sim.kind)
	}
	if !strings.Contains(viewText(m), "rendering") {
		t.Errorf("la vista no dice que está renderizando:\n%s", viewText(m))
	}
}

func TestAPressedWhileChoosingDoesNotFallThrough(t *testing.T) {
	f := &fakeSimulator{available: true}
	m := simModel(t, f)
	m = press(t, m, "v")
	m = press(t, m, "a")

	if m.sim.state != simClosed {
		t.Errorf("state = %v, want cerrado", m.sim.state)
	}
	if lastToast(m) != "" {
		t.Errorf("notice = %q, want vacío: la `a` no debe haber aprobado nada", lastToast(m))
	}
	if m.actionBusy {
		t.Error("la `a` lanzó una acción del forge")
	}
}

func TestEscClosesThePopup(t *testing.T) {
	m := simModel(t, &fakeSimulator{available: true})
	m = press(t, m, "v")
	m = press(t, m, "esc")

	if m.sim.state != simClosed {
		t.Errorf("state = %v, want cerrado", m.sim.state)
	}
	if strings.Contains(viewText(m), "simulate acme/widget") {
		t.Error("el popup sigue en la vista tras cerrarlo")
	}
}

func TestQuittingStillQuitsWithThePopupOpen(t *testing.T) {
	m := simModel(t, &fakeSimulator{available: true})
	m = press(t, m, "v")
	m = press(t, m, "v")
	if m.sim.state != simClosed {
		t.Errorf("state = %v, want cerrado: la segunda `v` no es una elección", m.sim.state)
	}
}

func TestResultShowsTheImage(t *testing.T) {
	path := writeJPEG(t)
	f := &fakeSimulator{available: true, res: sim.Result{Kind: sim.KindMerge, Path: path}}
	m := simModel(t, f)
	m = press(t, m, "v")
	m = press(t, m, "enter")
	m = send(t, m, simMsg{seq: m.simSeq, kind: sim.KindMerge, res: f.res})

	if m.sim.state != simShowing {
		t.Fatalf("state = %v, want simShowing", m.sim.state)
	}
	if len(m.sim.cells) == 0 {
		t.Fatal("el popup no pintó ninguna celda de la imagen")
	}
	if m.sim.cellW <= 0 || m.sim.cellH <= 0 {
		t.Errorf("geometría de la imagen = %dx%d", m.sim.cellW, m.sim.cellH)
	}
	text := viewText(m)
	if !strings.Contains(text, "o open image") {
		t.Errorf("la vista no ofrece abrir la imagen:\n%s", text)
	}
	if !strings.Contains(text, "Assigned (1)") {
		t.Error("el popup tapó la vista de fondo entera")
	}
}

func TestTheImageFillsTheBox(t *testing.T) {
	path := writeJPEG(t)
	f := &fakeSimulator{available: true, res: sim.Result{Kind: sim.KindMerge, Path: path}}
	m := simModel(t, f)
	m = press(t, m, "v")
	m = press(t, m, "enter")
	m = send(t, m, simMsg{seq: m.simSeq, kind: sim.KindMerge, res: f.res})

	boxW, boxH := m.simBox()
	if m.sim.cellW != boxW-2 {
		t.Errorf("celdas de %d columnas en una caja de %d: la imagen no llena la caja", m.sim.cellW, boxW)
	}
	if len(m.sim.cells) != boxH-simChrome {
		t.Errorf("celdas en %d líneas para una caja de %d", len(m.sim.cells), boxH)
	}
	if boxW > m.contentWidth() {
		t.Errorf("caja de %d columnas en una vista de %d", boxW, m.contentWidth())
	}
	if boxH > m.height {
		t.Errorf("caja de %d líneas en una terminal de %d", boxH, m.height)
	}
}

func TestThePopupGrowsWithTheTerminal(t *testing.T) {
	path := writeJPEG(t)
	f := &fakeSimulator{available: true, res: sim.Result{Kind: sim.KindMerge, Path: path}}
	m := simModel(t, f)
	m = send(t, m, tea.WindowSizeMsg{Width: 240, Height: 70})
	m = press(t, m, "v")
	m = press(t, m, "enter")
	m = send(t, m, simMsg{seq: m.simSeq, kind: sim.KindMerge, res: f.res})

	wide, rows := m.simBox()
	if wide < 100 {
		t.Errorf("caja de %d columnas en una terminal de 240: no aprovecha el ancho", wide)
	}
	if rows < 20 {
		t.Errorf("caja de %d líneas en una terminal de 70: no aprovecha el alto", rows)
	}
	if rows >= m.height {
		t.Errorf("la caja (%d) tapa la terminal entera (%d)", rows, m.height)
	}
	if wide >= m.contentWidth() {
		t.Errorf("la caja (%d) ocupa la vista entera (%d)", wide, m.contentWidth())
	}
}

// A cell is twice as tall as wide, so drawing a 16:9 image unscaled would stretch it.
func TestTheImageIsNotStretched(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 1920, 1080))
	for y := range 1080 {
		for x := range 1920 {
			img.SetRGBA(x, y, color.RGBA{R: uint8(x % 256), G: uint8(y % 256), B: 90, A: 255})
		}
	}
	big := filepath.Join(t.TempDir(), "wide.jpg")
	f, err := os.Create(big)
	if err != nil {
		t.Fatal(err)
	}
	if err := jpeg.Encode(f, img, nil); err != nil {
		t.Fatal(err)
	}
	_ = f.Close()

	sim1 := &fakeSimulator{available: true, res: sim.Result{Kind: sim.KindMerge, Path: big}}
	m := simModel(t, sim1)
	m = send(t, m, tea.WindowSizeMsg{Width: 200, Height: 60})
	m = press(t, m, "v")
	m = press(t, m, "enter")
	m = send(t, m, simMsg{seq: m.simSeq, kind: sim.KindMerge, res: sim1.res})

	if ratio := float64(m.sim.cellW) / float64(m.sim.cellH); ratio < 3.0 || ratio > 4.1 {
		t.Errorf("la imagen ocupa %d×%d celdas (ratio %.2f), want ~3,56: está deformada",
			m.sim.cellW, m.sim.cellH, ratio)
	}
}

func TestTheChooserNavigatesWhenThereIsSomethingToNavigate(t *testing.T) {
	restore := simKinds
	simKinds = []sim.Kind{sim.KindMerge, sim.KindRebase}
	t.Cleanup(func() { simKinds = restore })

	f := &fakeSimulator{available: true}
	m := simModel(t, f)
	m = press(t, m, "v")

	m = press(t, m, "j")
	if m.sim.cursor != 1 {
		t.Fatalf("cursor = %d, want 1", m.sim.cursor)
	}
	if len(f.kinds) != 0 {
		t.Fatalf("una flecha confirmó el render: %v", f.kinds)
	}
	m = press(t, m, "j")
	if m.sim.cursor != 0 {
		t.Errorf("cursor = %d, want 0 al dar la vuelta", m.sim.cursor)
	}
	m = press(t, m, "k")
	if m.sim.cursor != 1 {
		t.Errorf("cursor = %d, want 1 al llegar por arriba", m.sim.cursor)
	}
	m = press(t, m, "enter")
	if m.sim.kind != sim.KindRebase {
		t.Errorf("kind = %q, want la opción elegida (rebase)", m.sim.kind)
	}
}

func TestLeftArrowDoesNotStartTheRender(t *testing.T) {
	f := &fakeSimulator{available: true}
	m := simModel(t, f)
	m = press(t, m, "v")

	for _, key := range []string{"left", "right", "up", "down", "h", "l"} {
		m = press(t, m, "v")
		m = press(t, m, key)
		if m.sim.state != simClosed {
			t.Errorf("la tecla %q dejó el popup en %v; cualquier flecha debe cerrarlo", key, m.sim.state)
		}
		if len(f.kinds) != 0 {
			t.Fatalf("la tecla %q lanzó un render: %v", key, f.kinds)
		}
	}
}

func TestStaleResultIsDiscarded(t *testing.T) {
	path := writeJPEG(t)
	f := &fakeSimulator{available: true, res: sim.Result{Kind: sim.KindMerge, Path: path}}
	m := simModel(t, f)
	m = press(t, m, "v")
	m = press(t, m, "enter")
	stale := m.simSeq

	m = press(t, m, "esc") // el popup se cierra
	m = send(t, m, simMsg{seq: stale, kind: sim.KindMerge, res: f.res})

	if m.sim.state != simClosed {
		t.Errorf("state = %v, want cerrado: un resultado obsoleto no debe abrir el popup", m.sim.state)
	}
}

func TestFailureClosesThePopupAndNotices(t *testing.T) {
	f := &fakeSimulator{available: true, err: errNotMounted{}}
	m := simModel(t, f)
	m = press(t, m, "v")
	m = press(t, m, "enter")
	m = send(t, m, simMsg{seq: m.simSeq, kind: sim.KindMerge, err: f.err})

	if m.sim.state != simClosed {
		t.Errorf("state = %v, want cerrado", m.sim.state)
	}
	if !strings.Contains(lastToast(m), "simulate merge") {
		t.Errorf("notice = %q, want el motivo del fallo", lastToast(m))
	}
}

func TestUnreadableImageNotices(t *testing.T) {
	broken := filepath.Join(t.TempDir(), "broken.jpg")
	if err := os.WriteFile(broken, []byte("no soy una imagen"), 0o644); err != nil {
		t.Fatal(err)
	}
	f := &fakeSimulator{available: true, res: sim.Result{Kind: sim.KindMerge, Path: broken}}
	m := simModel(t, f)
	m = press(t, m, "v")
	m = press(t, m, "enter")
	m = send(t, m, simMsg{seq: m.simSeq, kind: sim.KindMerge, res: f.res})

	if m.sim.state != simClosed {
		t.Errorf("state = %v, want cerrado", m.sim.state)
	}
	if !strings.Contains(lastToast(m), "simulate") {
		t.Errorf("notice = %q", lastToast(m))
	}
}

// The popup's width follows the terminal, so an image sized for the old width is rescaled.
func TestResizeRescalesTheImage(t *testing.T) {
	path := writeJPEG(t)
	f := &fakeSimulator{available: true, res: sim.Result{Kind: sim.KindMerge, Path: path}}
	m := simModel(t, f)
	m = press(t, m, "v")
	m = press(t, m, "enter")
	m = send(t, m, simMsg{seq: m.simSeq, kind: sim.KindMerge, res: f.res})
	before := m.sim.cellW

	m = send(t, m, tea.WindowSizeMsg{Width: 60, Height: 20})
	if m.sim.cellW == before {
		t.Errorf("la imagen no se reescaló al nuevo ancho (sigue en %d)", m.sim.cellW)
	}
	if m.sim.cellW >= 60 {
		t.Errorf("la imagen mide %d columnas en una terminal de 60", m.sim.cellW)
	}
}

func TestSimulateNeedsATargetBranch(t *testing.T) {
	f := &fakeSimulator{available: true}
	ad := ghAdapter()
	m := newTestModel(t, ad)
	m.SetSimulator(f)

	noBase := mkItem("github", "github.com", "acme/widget", "x", 8, "")
	noBase.TargetBranch = ""
	m = send(t, m, page(1, "github", "github.com", model.SectionReview, model.ReviewRequested, []model.Item{noBase}, false))
	m = press(t, m, "v")

	if m.sim.state != simClosed {
		t.Errorf("state = %v, want cerrado", m.sim.state)
	}
	if !strings.Contains(lastToast(m), "target branch") {
		t.Errorf("notice = %q", lastToast(m))
	}
}

func TestAnArmedMergeSwallowsTheSimulateKey(t *testing.T) {
	m := simModel(t, &fakeSimulator{available: true})
	m = press(t, m, "m") // arma el merge
	if !m.mergeArmed {
		t.Fatal("el merge no se armó")
	}
	m = press(t, m, "v")

	if m.mergeArmed {
		t.Error("el merge sigue armado")
	}
	if m.sim.state != simClosed {
		t.Errorf("state = %v, want cerrado: la `v` no abre nada con el merge armado", m.sim.state)
	}

	m = press(t, m, "v")
	if m.sim.state != simChoosing {
		t.Errorf("state = %v, want simChoosing en la segunda `v`", m.sim.state)
	}
}

type errNotMounted struct{}

func (errNotMounted) Error() string { return "the review must be mounted first" }
