package tui

import (
	"strings"
	"testing"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"prdash/internal/forge/model"
	"prdash/internal/state"
)

func toastTestModel(t *testing.T) (Model, *time.Time) {
	t.Helper()
	m := newTestModel(t, ghAdapter())
	now := time.Date(2026, 9, 26, 10, 0, 0, 0, time.UTC)
	m.toast.now = func() time.Time { return now }
	return m, &now
}

func TestToastAppearsAndExpires(t *testing.T) {
	m, now := toastTestModel(t)
	m = press(t, m, "a") // sin selección: lanza un aviso

	if len(toastTexts(m)) == 0 {
		t.Fatal("debería haber un aviso tras la pulsación")
	}
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

func TestToastIgnoresEmptyMessage(t *testing.T) {
	m, _ := toastTestModel(t)
	m.toast.show("", toastInfo)
	if got := m.toast.texts(); len(got) != 0 {
		t.Errorf("un aviso vacío no debería apilarse: %q", got)
	}
}

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

// The delicate invariant: compositing the overlay over lines must not widen them, or the
// whole table misaligns.
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
		if w := ansi.StringWidth(l); w > m.outerWidth() {
			t.Fatalf("línea %d mide %d columnas, excede %d:\n%q", i, w, m.outerWidth(), stripANSI(l))
		}
	}
	if stripANSI(after[0]) != stripANSI(before[0]) {
		t.Errorf("el overlay movió la cabecera:\n%q\n%q", stripANSI(before[0]), stripANSI(after[0]))
	}
}

func marcoDe(view string) []string {
	out := make([]string, 0)
	for _, l := range strings.Split(stripANSI(view), "\n") {
		if l == "" {
			out = append(out, "")
			continue
		}
		out = append(out, string([]rune(l)[0])+string([]rune(l)[len([]rune(l))-1]))
	}
	return out
}

func TestToastNoPisaBordes(t *testing.T) {
	m, _ := toastTestModel(t)
	item := mkItem("github", "github.com", "acme/widget", "Add widget", 1, "")
	item.Author = "otra"
	m = send(t, m, page(1, "github", "github.com", model.SectionReview, model.ReviewRequested, []model.Item{item}, false))

	before := marcoDe(m.View().Content)
	m.toast.show("you cannot approve your own PR/MR", toastWarning)
	after := marcoDe(m.View().Content)

	if len(after) != len(before) {
		t.Fatalf("el overlay cambió el número de líneas: %d -> %d", len(before), len(after))
	}
	for i := range before {
		if after[i] != before[i] {
			t.Errorf("línea %d: el marco pasó de %q a %q; el aviso ha pisado un borde:\n%s",
				i, before[i], after[i], stripANSI(m.View().Content))
		}
	}
}

// The hints box does not give up its interior: it is the help the user has to read.
func TestToastNoTapaLaAyuda(t *testing.T) {
	m, _ := toastTestModel(t)
	m = send(t, m, page(1, "github", "github.com", model.SectionReview, model.ReviewRequested, []model.Item{
		mkItem("github", "github.com", "acme/widget", "Add widget", 1, ""),
	}, false))

	m.toast.show("merged !42 into main", toastSuccess)
	view := stripANSI(m.View().Content)
	for _, l := range strings.Split(view, "\n") {
		if !strings.Contains(l, "q quit") {
			continue
		}
		if !strings.Contains(l, "j/k move") {
			t.Errorf("la línea de atajos quedó tapada por un aviso: %q", l)
		}
	}
	if !strings.Contains(view, "j/k move") {
		t.Errorf("los atajos desaparecieron con el aviso:\n%s", view)
	}
}

func TestToastApilaVariosAvisos(t *testing.T) {
	m, _ := toastTestModel(t)
	m = send(t, m, page(1, "github", "github.com", model.SectionReview, model.ReviewRequested, []model.Item{
		mkItem("github", "github.com", "acme/widget", "Add widget", 1, ""),
	}, false))
	before := marcoDe(m.View().Content)

	m.toast.show("primer aviso", toastInfo)
	m.toast.show("segundo aviso", toastError)
	m.toast.show("tercer aviso", toastSuccess)
	view := stripANSI(m.View().Content)

	for _, want := range []string{"primer aviso", "segundo aviso", "tercer aviso"} {
		if !strings.Contains(view, want) {
			t.Errorf("falta el aviso %q:\n%s", want, view)
		}
	}
	after := marcoDe(m.View().Content)
	for i := range before {
		if after[i] != before[i] {
			t.Errorf("línea %d: el marco pasó de %q a %q con 3 avisos vivos", i, before[i], after[i])
		}
	}
}

func TestViewRowsCoincidenConLasLineas(t *testing.T) {
	m, _ := toastTestModel(t)
	v := m.compose(m.layout(), m.listSection(m.layout()), m.detailSection(model.Item{}, false, m.layout().detailLines))
	if got, want := len(v.rows), len(strings.Split(v.text, "\n")); got != want {
		t.Errorf("rows = %d, want %d (una por línea)", got, want)
	}
	for i, ok := range v.rows {
		l := stripANSI(strings.Split(v.text, "\n")[i])
		borde := strings.HasPrefix(l, "╭") || strings.HasPrefix(l, "╰")
		if borde && ok {
			t.Errorf("la línea %d es un borde y no debería admitir aviso: %q", i, l)
		}
	}
}

func TestLandRowBuscaElHuecoMasBajo(t *testing.T) {
	rows := []bool{false, false, false, true, true, true}
	if base, ok := landRow(rows, 5, 3); !ok || base != 5 {
		t.Errorf("landRow = (%d, %v), want (5, true): el hueco más bajo es 3..5", base, ok)
	}
	if _, ok := landRow(rows, 5, 4); ok {
		t.Error("landRow encontró hueco para 4 filas en 3 libres")
	}
	arriba := []bool{true, true, true, false, false, false}
	if base, ok := landRow(arriba, 2, 3); !ok || base != 2 {
		t.Errorf("landRow(arriba, 2, 3) = (%d, %v), want (2, true): el anchor limita la búsqueda", base, ok)
	}
	if _, ok := landRow(arriba, 1, 3); ok {
		t.Error("landRow subió por encima del anchor")
	}
	if _, ok := landRow([]bool{false, false, false}, 2, 2); ok {
		t.Error("landRow pintó sobre filas que no admiten aviso")
	}
	if _, ok := landRow([]bool{true, true, true}, -5, 3); ok {
		t.Error("landRow aceptó un anchor negativo")
	}
}

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
	if w := ansi.StringWidth(strings.Split(m.toast.blocks(30)[0], "\n")[0]); w > 30 {
		t.Errorf("la caja no respetó el hueco disponible: %d columnas", w)
	}
}

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

// The tick comes from the clock, not the channel, so it cannot re-arm a reader.
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

func TestToastInViewOfDetail(t *testing.T) {
	m, _ := toastTestModel(t)
	item := mkItem("github", "github.com", "acme/widget", "Add widget", 1, "")
	m = send(t, m, page(1, "github", "github.com", model.SectionAuthored, "", []model.Item{item}, false))
	m.toast.show(state.SelfReviewReason, toastWarning)
	if !strings.Contains(stripANSI(m.View().Content), "you cannot approve your own") {
		t.Fatal("el aviso debería verse sobre el panel de detalle")
	}
}

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

// replace must find its target even when it is not the newest.
func TestToastReplaceFindsAnOlderToast(t *testing.T) {
	m, now := toastTestModel(t)
	m.toast.show("merge ok", toastSuccess)
	m.toast.show("an action is running", toastInfo) // más nuevo: el merge no es el último

	*now = now.Add(time.Second)
	m.toast.replace("merge ok", "merge ok · worktree removed", toastSuccess)

	got := m.toast.texts()
	if len(got) != 2 {
		t.Fatalf("texts = %q, quiero 2 (el viejo sustituido, el nuevo intacto)", got)
	}
	if got[0] != "merge ok · worktree removed" {
		t.Errorf("el aviso viejo debería sustituirse en su sitio: %q", got)
	}
	if got[1] != "an action is running" {
		t.Errorf("el aviso más nuevo no debería tocarse: %q", got)
	}
	if !m.toast.toasts[0].created.Equal(*now) {
		t.Errorf("el TTL del aviso sustituido debería reiniciarse: created=%v, now=%v", m.toast.toasts[0].created, *now)
	}
}

func TestToastReplaceAppendsWhenTheTargetIsGone(t *testing.T) {
	m, now := toastTestModel(t)
	m.toast.show("merge ok", toastSuccess)

	*now = now.Add(toastDuration + time.Second)
	m.toast.update()
	m.toast.show("refreshed", toastInfo)
	if got := m.toast.texts(); len(got) != 1 || got[0] != "refreshed" {
		t.Fatalf("preparación: texts = %q, quiero solo el aviso nuevo", got)
	}

	m.toast.replace("merge ok", "merge ok · worktree removed", toastSuccess)
	got := m.toast.texts()
	if len(got) != 2 || got[0] != "refreshed" || got[1] != "merge ok · worktree removed" {
		t.Fatalf("texts = %q, quiero el aviso nuevo apilado al final", got)
	}
}
