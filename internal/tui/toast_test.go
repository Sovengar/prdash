package tui

import (
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"prdash/internal/forge/model"
	"prdash/internal/state"
)

// toastTestModel devuelve un modelo con reloj fijo: la caducidad se prueba
// advancing the clock, not sleeping.
func toastTestModel(t *testing.T) (Model, *time.Time) {
	t.Helper()
	m := newTestModel(t, ghAdapter())
	now := time.Date(2026, 9, 26, 10, 0, 0, 0, time.UTC)
	m.toast.now = func() time.Time { return now }
	return m, &now
}

// TestToastAppearsAndExpires: un aviso se ve al salir y desaparece solo pasado su
// TTL, sin que ningún evento de la app lo empuje.
func TestToastAppearsAndExpires(t *testing.T) {
	m, now := toastTestModel(t)
	m = press(t, m, "a") // sin selección: lanza un aviso

	if len(toastTexts(m)) == 0 {
		t.Fatal("debería haber un aviso tras la pulsación")
	}
	// El tick poda lo caducado; antes del TTL sigue ahí.
	m = send(t, m, toastTickMsg{})
	if len(toastTexts(m)) != 1 {
		t.Fatalf("el aviso shouldn't caducar antes de tiempo: %q", toastTexts(m))
	}
	*now = now.Add(toastDuration + time.Second)
	m = send(t, m, toastTickMsg{})
	if got := toastTexts(m); len(got) != 0 {
		t.Fatalf("el aviso debería haber caducado: %q", got)
	}
}

// TestToastStacksInOrder cubre la pila: el último Lanzado es el que se lee y
// todos se dibujan.
func TestToastStacksInOrder(t *testing.T) {
	m, _ := toastTestModel(t)
	m.toast.show("first", toastInfo)
	m.toast.show("second", toastError)
	if got := m.toast.last(); got != "second" {
		t.Errorf("last() = %q, want %q", got, "second")
	}
	if len(m.toast.texts()) != 2 {
		t.Errorf("texts() = %q, want 2 avisos", m.toast.texts())
	}
}

// TestToastIgnoresEmptyMessage: un aviso vacío no se apila (evita cajas en
// blanco).
func TestToastIgnoresEmptyMessage(t *testing.T) {
	m, _ := toastTestModel(t)
	m.toast.show("", toastInfo)
	if got := m.toast.texts(); len(got) != 0 {
		t.Errorf("un aviso vacío no debería apilarse: %q", got)
	}
}

// TestToastOverlaysView: el aviso se dibuja encima de la vista, en su propia
// caja, y la vista sigue detrás.
func TestToastOverlaysView(t *testing.T) {
	m, _ := toastTestModel(t)
	item := mkItem("github", "github.com", "acme/widget", "Add widget", 1, "")
	item.Author = "otra"
	m = send(t, m, page(1, "github", "github.com", model.SectionReview, model.ReviewRequested, []model.Item{item}, false))

	view := stripANSI(m.View().Content)
	if !strings.Contains(view, "Add widget") {
		t.Fatalf("la vista base debería seguir ahí:\n%s", view)
	}

	m.toast.show("you cannot approve your own PR/MR", toastWarning)
	overlaid := stripANSI(m.View().Content)
	if !strings.Contains(overlaid, "you cannot approve your own PR/MR") {
		t.Fatalf("el aviso debería superponerse:\n%s", overlaid)
	}
	if !strings.Contains(overlaid, "Add widget") {
		t.Fatalf("el overlay no debe borrar la vista de fondo:\n%s", overlaid)
	}
}

// TestToastDoesNotBreakColumnWidths es la invariante delicate: componer el
// overlay sobre líneas con ANSI no puede desplazar el texto de debajo, desbordar
// el ancho útil ni añadir líneas.
func TestToastDoesNotBreakColumnWidths(t *testing.T) {
	m, _ := toastTestModel(t)
	item := mkItem("github", "github.com", "acme/widget", "Add widget", 1, "")
	item.Author = "otra"
	m = send(t, m, page(1, "github", "github.com", model.SectionReview, model.ReviewRequested, []model.Item{item}, false))
	before := strings.Split(m.View().Content, "\n")

	m.toast.show("you cannot approve your own PR/MR", toastWarning)
	after := strings.Split(m.View().Content, "\n")
	if len(after) != len(before) {
		t.Fatalf("el overlay añadió líneas: %d -> %d", len(before), len(after))
	}
	for i, l := range after {
		if w := ansi.StringWidth(l); w > m.contentWidth() {
			t.Fatalf("línea %d mide %d columnas, excede %d:\n%q", i, w, m.contentWidth(), stripANSI(l))
		}
	}
	// La tabla no se ha movido: el encabezado de columna sigue en su sitio.
	if stripANSI(after[0]) != stripANSI(before[0]) {
		t.Errorf("el overlay movió la cabecera:\n%q\n%q", stripANSI(before[0]), stripANSI(after[0]))
	}
}

// TestToastWrapsAndStaysBounded: un mensaje largo se envuelve y la caja nunca
// se sale del máximo.
func TestToastWrapsAndStaysBounded(t *testing.T) {
	m, _ := toastTestModel(t)
	long := strings.Repeat("very long toast message ", 12)
	m.toast.show(long, toastError)
	blocks := m.toast.blocks(80)
	if len(blocks) != 1 {
		t.Fatalf("un aviso es una caja, want 1: %d", len(blocks))
	}
	boxLines := strings.Split(blocks[0], "\n")
	if len(boxLines) < 2 {
		t.Fatalf("un mensaje largo debería ocupar varias líneas: %d", len(boxLines))
	}
	for _, l := range boxLines {
		if w := ansi.StringWidth(l); w > toastMaxWidth {
			t.Errorf("línea de caja con %d columnas, excede %d: %q", w, toastMaxWidth, l)
		}
	}
	// Con poco hueco disponible la caja se encoge.
	if w := ansi.StringWidth(strings.Split(m.toast.blocks(30)[0], "\n")[0]); w > 30 {
		t.Errorf("la caja no respetó el hueco disponible: %d columnas", w)
	}
}

// TestSetNoticeMapsLevels: el nivel interno se traduce al del toast y
// levelNone no lanza nada.
func TestSetNoticeMapsLevels(t *testing.T) {
	m, _ := toastTestModel(t)
	m.setNotice("hola", levelNone)
	if got := m.toast.texts(); len(got) != 0 {
		t.Errorf("levelNone no debería lanzar aviso: %q", got)
	}
	for _, lvl := range []noticeLevel{levelInfo, levelOK, levelWarn, levelError} {
		m.toast.toasts = nil
		m.setNotice("mensaje", lvl)
		if len(m.toast.texts()) != 1 {
			t.Errorf("nivel %v no lanzó aviso", lvl)
		}
	}
}

// TestToastTickDoesNotTouchTheEventChannel: el tick viene del reloj, no del
// canal: no puede rearmar un lector (el invariante de la bomba es 1).
func TestToastTickDoesNotTouchTheEventChannel(t *testing.T) {
	m, _ := toastTestModel(t)
	before := m.readers
	out, cmd := m.Update(toastTickMsg{})
	after := out.(Model)
	if after.readers != before {
		t.Errorf("el tick de toast tocó los lectores: %d -> %d", before, after.readers)
	}
	if cmd == nil {
		t.Error("el tick debería rearmarse")
	}
}

// TestToastInViewOfDetail: el overlay también se aplica con el detalle abierto.
func TestToastInViewOfDetail(t *testing.T) {
	m, _ := toastTestModel(t)
	item := mkItem("github", "github.com", "acme/widget", "Add widget", 1, "")
	m = send(t, m, page(1, "github", "github.com", model.SectionAuthored, "", []model.Item{item}, false))
	m = send(t, m, tea.KeyPressMsg{Code: []rune(m.cfg.KeyFor("detail"))[0], Text: m.cfg.KeyFor("detail")})
	if !m.detailOpen {
		t.Fatal("el detalle debería estar abierto")
	}
	m.toast.show(state.SelfReviewReason, toastWarning)
	if !strings.Contains(stripANSI(m.View().Content), "you cannot approve your own") {
		t.Fatal("el aviso debería verse también con el detalle abierto")
	}
}

// TestToastBorderMatchesLevel: cada nivel tiene su icono y su color, para que el
// aviso se distinga de un vistazo.
func TestToastBorderMatchesLevel(t *testing.T) {
	m, _ := toastTestModel(t)
	want := map[toastLevel]string{
		toastSuccess: "✓", toastError: "✗", toastInfo: "ℹ", toastWarning: "⚠",
	}
	for lvl, icon := range want {
		if got := toastIcon(lvl); got != icon {
			t.Errorf("toastIcon(%d) = %q, want %q", lvl, got, icon)
		}
		block := m.toast.render(toast{message: "x", level: lvl, created: time.Now(), duration: time.Second}, 80)
		if !strings.Contains(block, icon) {
			t.Errorf("la caja del nivel %d no lleva su icono: %q", lvl, block)
		}
		if !strings.Contains(block, lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).Render("x")) &&
			!strings.Contains(block, "│") {
			t.Errorf("la caja del nivel %d no parece una caja: %q", lvl, block)
		}
	}
}
