package tui

import (
	"context"
	"image"
	"image/color"
	"image/jpeg"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"prdash/internal/forge/model"
	"prdash/internal/sim"
)

// fakeSimulator devuelve un resultado fijo y recuerda qué se le pidió.
type fakeSimulator struct {
	available bool
	res       sim.Result
	err       error
	kinds     []sim.Kind
	items     []model.Item
	// delay hace que la llamada espere, para poder comprobar el estado
	// intermediario sin depender del orden del planificador.
	delay time.Duration
}

func (f *fakeSimulator) Available() bool { return f.available }

func (f *fakeSimulator) Simulate(_ context.Context, it model.Item, kind sim.Kind) (sim.Result, error) {
	f.kinds = append(f.kinds, kind)
	f.items = append(f.items, it)
	if f.delay > 0 {
		time.Sleep(f.delay)
	}
	return f.res, f.err
}

// simModel monta un modelo con un ítem seleccionado y un simulador inyectado.
func simModel(t *testing.T, s Simulator) Model {
	t.Helper()
	ad := ghAdapter()
	m := newTestModel(t, ad)
	m.SetSimulator(s)
	it := mkItem("github", "github.com", "acme/widget", "Fix the thing", 7, "")
	m = send(t, m, page(1, "github", "github.com", model.SectionReview, model.ReviewRequested, []model.Item{it}, false))
	return m
}

// writeJPEG crea un JPEG diminuto y devuelve su ruta.
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
	defer f.Close()
	if err := jpeg.Encode(f, img, nil); err != nil {
		t.Fatal(err)
	}
	return path
}

// viewText es el texto plano de la vista, sin los códigos de color.
func viewText(m Model) string { return stripANSI(m.View().Content) }

// TestSimulateWithoutGitSimInforms: sin el binario la acción avisa en la cabecera
// en vez de abrir un popup que no puede hacer nada.
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

// TestSimulateOpensTheChooser: la tecla de simulación abre el selector sobre el
// ítem seleccionado, sin cambiar de vista.
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

// TestChooserOnlyOffersWhatGitSimCanRender: rebase no se ofrece porque git-sim
// 0.3.5 no lo dibuja (se equivoca de mensaje si la rama ya está basada en la base
// y revienta con IndexError si divergen). Ofrecerlo sería una opción que solo
// puede fallar.
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

// TestChooserLeavesNothingToAccidentallyConfirm: el popup no lanza nada hasta que
// se pulse enter, así que abrirlo ygolpear otra tecla no cuesta un render.
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

// TestEnterStartsTheChosenStrategy: confirmar lanza el render de la estrategia
// nombrada, y solo de esa.
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

// TestAPressedWhileChoosingDoesNotFallThrough: una tecla que no nombra ninguna
// estrategia cierra el popup, pero no dispara la acción que tiene esa tecla en el
// resto de la vista. Es la misma trampa que ya tenía el merge armado.
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

// TestEscClosesThePopup: cerrar el popup es una intención distinta de cerrar la
// TUI, y `q` tiene que seguir funcionando con el popup abierto.
func TestEscClosesThePopup(t *testing.T) {
	m := simModel(t, &fakeSimulator{available: true})
	m = press(t, m, "v")
	m = press(t, m, "esc")

	if m.sim.state != simClosed {
		t.Errorf("state = %v, want cerrado", m.sim.state)
	}
	// El popup se Comprobaba por su título, no por la palabra "simulate": esa
	// también está en la barra de atajos, que sobrevive al cierre.
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

// TestResultShowsTheImage: con el render terminado, el popup enseña la imagen ya
// traducida a celdas, y el fondo sigue detrás.
func TestResultShowsTheImage(t *testing.T) {
	path := writeJPEG(t)
	f := &fakeSimulator{available: true, res: sim.Result{Kind: sim.KindMerge, Path: path}}
	m := simModel(t, f)
	m = press(t, m, "v")
	m = press(t, m, "enter")
	// El resultado llega por el canal, con la misma secuencia que la petición.
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
	// El fondo sobrevive: la caja del inbox sigue debajo del popup.
	if !strings.Contains(text, "Inbox") {
		t.Error("el popup tapó la vista de fondo entera")
	}
}

// TestStaleResultIsDiscarded: un render que llega tarde, con el popup ya cerrado,
// no puede resucitarlo ni saltar a la pantalla de arriba.
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

// TestFailureClosesThePopupAndNotices: un fallo se cuenta en la cabecera, que es
// donde viven los avisos. Un popup explicando su propio error encima de la vista
// sería un popup más difícil de leer que el aviso.
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

// TestUnreadableImageNotices: una imagen que no se puede decodificar es un fallo
// del render, no de la vista, y se cuenta como tal.
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

// TestResizeRescalesTheImage: el ancho del popup depende del terminal, y una
// imagen calculada para el tamaño anterior se quedaría descentrada o desbordada.
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

// TestSimulateNeedsATargetBranch: sin rama base no hay nada que comparar, y el
// popup solo podría ofrecer un menú de estrategias que no significan nada.
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

// TestAnArmedMergeSwallowsTheSimulateKey: con el merge armado, la segunda
// pulsación solo puede ser un modo. La `v` no lo es, así que desarma y se
// consume: abrir un popup modal desde una pulsación a destiempo sería la misma
// trampa que el merge armado ya evita (un `m` solo nunca degenera en otra
// acción), y la segunda `v` sí abre la simulación.
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

// errNotMounted es el fallo típico: el review no está montado y no hay refs.
type errNotMounted struct{}

func (errNotMounted) Error() string { return "the review must be mounted first" }
