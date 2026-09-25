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
	// El overlay no puede ensuciar el borde de las cajas: toda la vista mide el
	// ancho exterior de la terminal (el del borde), y el aviso se recorta al
	// interior.
	for i, l := range after {
		if w := ansi.StringWidth(l); w > m.outerWidth() {
			t.Fatalf("línea %d mide %d columnas, excede %d:\n%q", i, w, m.outerWidth(), stripANSI(l))
		}
	}
	// La tabla no se ha movido: el encabezado de columna sigue en su sitio.
	if stripANSI(after[0]) != stripANSI(before[0]) {
		t.Errorf("el overlay movió la cabecera:\n%q\n%q", stripANSI(before[0]), stripANSI(after[0]))
	}
}

// marcoDe extrae el esqueleto de la vista: el primer y el último carácter de
// cada línea. Es lo que dice si los marcos siguen en su sitio: si el overlay
// pisa una línea de borde, el esqueleto cambia.
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

// TestToastNoPisaBordes es el motivo de anclar el aviso al interior: la vista
// es una pila de cajas y un aviso encima del borde inferior del detalle lo
// destrozaba, dejando un marco partido. Ahora el aviso solo aterriza en interior
// de caja, así que todos los marcos sobreviven intactos.
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

// TestToastNoTapaLaAyuda: la caja de atajos no cede su interior. Es la ayuda que
// hay que leer cuando no se entiende una tecla, así que un aviso encima la
// volvería ilegible justo cuando hace falta.
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
		// La línea de atajos tiene que seguir siendo la caja de atajos, no un
		// aviso superpuesto: su contenido intacto y sin glifos de aviso.
		if !strings.Contains(l, "j/k move") {
			t.Errorf("la línea de atajos quedó tapada por un aviso: %q", l)
		}
	}
	if !strings.Contains(view, "j/k move") {
		t.Errorf("los atajos desaparecieron con el aviso:\n%s", view)
	}
}

// TestToastApilaVariosAvisos: los avisos se apilan hacia arriba sin pisar bordes
// ni solaparse, y cada uno se ve entero.
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

// TestViewRowsCoincidenConLasLineas: la lista de filas que usa el overlay tiene
// una entrada por línea de la vista. Si se desincronizara, el aviso aterrizaría
// en la fila que no es.
func TestViewRowsCoincidenConLasLineas(t *testing.T) {
	m, _ := toastTestModel(t)
	v := m.compose(m.layout(), m.listSection(m.layout()), m.detailSection(model.Item{}, false, m.layout().detailLines))
	if got, want := len(v.rows), len(strings.Split(v.text, "\n")); got != want {
		t.Errorf("rows = %d, want %d (una por línea)", got, want)
	}
	// Los bordes de las cajas no admiten aviso; el interior de la lista y del
	// detalle, sí.
	for i, ok := range v.rows {
		l := stripANSI(strings.Split(v.text, "\n")[i])
		borde := strings.HasPrefix(l, "╭") || strings.HasPrefix(l, "╰")
		if borde && ok {
			t.Errorf("la línea %d es un borde y no debería admitir aviso: %q", i, l)
		}
	}
}

// TestLandRowBuscaElHuecoMasBajo: la búsqueda del hueco es pura y su contrato es
// exacto — la fila más baja que cabe, y solo sobre filas que lo admiten.
func TestLandRowBuscaElHuecoMasBajo(t *testing.T) {
	// 6 filas: las 3 últimas admiten aviso, las anteriores no.
	rows := []bool{false, false, false, true, true, true}
	if base, ok := landRow(rows, 5, 3); !ok || base != 5 {
		t.Errorf("landRow = (%d, %v), want (5, true): el hueco más bajo es 3..5", base, ok)
	}
	// Un bloque de 4 no cabe en las 3 filas libres: no hay hueco.
	if _, ok := landRow(rows, 5, 4); ok {
		t.Error("landRow encontró hueco para 4 filas en 3 libres")
	}
	// Con anchor más arriba solo se pinta lo que queda por debajo de él: aquí
	// las 3 primeras filas son las que admiten aviso, y el hueco es 0..2.
	arriba := []bool{true, true, true, false, false, false}
	if base, ok := landRow(arriba, 2, 3); !ok || base != 2 {
		t.Errorf("landRow(arriba, 2, 3) = (%d, %v), want (2, true): el anchor limita la búsqueda", base, ok)
	}
	// Con anchor por debajo del hueco no se sube a buscarlo.
	if _, ok := landRow(arriba, 1, 3); ok {
		t.Error("landRow subió por encima del anchor")
	}
	// Si no hay ninguna fila que admita aviso, no se pinta.
	if _, ok := landRow([]bool{false, false, false}, 2, 2); ok {
		t.Error("landRow pintó sobre filas que no admiten aviso")
	}
	// Un anchor por debajo de cero no inventa filas.
	if _, ok := landRow([]bool{true, true, true}, -5, 3); ok {
		t.Error("landRow aceptó un anchor negativo")
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
