package tui

import (
	"strconv"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/charmbracelet/x/ansi"

	"prdash/internal/forge/model"
	"prdash/internal/inbox"
	"prdash/internal/testutil"
)

func manyItems(n int) []model.Item {
	items := make([]model.Item, 0, n)
	for i := 1; i <= n; i++ {
		items = append(items, mkItem("github", "github.com", "acme/widget", "Item "+strconv.Itoa(i), i, ""))
	}
	return items
}

func longModel(t *testing.T, n int) Model {
	t.Helper()
	m := newTestModel(t, ghAdapter())
	return send(t, m, page(1, "github", "github.com", model.SectionReview, model.ReviewRequested, manyItems(n), false))
}

func visibleLines(view string) []string {
	return strings.Split(stripANSI(view), "\n")
}

func TestViewBoxesEverySection(t *testing.T) {
	m := newTestModel(t, ghAdapter())
	m = send(t, m, page(1, "github", "github.com", model.SectionReview, model.ReviewRequested, []model.Item{
		mkItem("github", "github.com", "acme/widget", "Add widget", 1, ""),
	}, false))

	lines := visibleLines(m.View().Content)
	if len(lines) != m.height {
		t.Fatalf("la vista tiene %d líneas, want %d (la altura del terminal)", len(lines), m.height)
	}

	for _, want := range []string{"PRDash", "Assigned (1)", "Keybinds"} {
		if !hasBoxTitle(lines, want) {
			t.Errorf("falta la caja %q:\n%s", want, strings.Join(lines, "\n"))
		}
	}
	if hasBoxTitle(lines, "Inbox") {
		t.Errorf("el título Inbox ya no debería existir: lo sustituye la leyenda:\n%s", strings.Join(lines, "\n"))
	}
	if !hasBoxTitle(lines, "acme/widget#1") {
		t.Errorf("la caja del detalle no se titula con la referencia del ítem:\n%s", strings.Join(lines, "\n"))
	}

	tops, bottoms := 0, 0
	for _, l := range lines {
		if strings.HasPrefix(l, "╭") {
			tops++
		}
		if strings.HasPrefix(l, "╰") {
			bottoms++
		}
	}
	if tops != bottoms || tops == 0 {
		t.Errorf("cajas descompensadas: %d aperturas ╭ y %d cierres ╰", tops, bottoms)
	}
}

func hasBoxTitle(lines []string, title string) bool {
	for _, l := range lines {
		if strings.HasPrefix(l, "╭") && strings.Contains(l, " "+title+" ") {
			return true
		}
	}
	return false
}

func TestViewFitsTerminalHeight(t *testing.T) {
	m := longModel(t, 60)
	lines := visibleLines(m.View().Content)
	if len(lines) != m.height {
		t.Errorf("la vista tiene %d líneas, want %d (la altura del terminal)", len(lines), m.height)
	}
	if !hasBoxTitle(lines, "PRDash") {
		t.Errorf("la caja de cabecera no titula la app: %q", lines[0])
	}
	if !strings.Contains(lines[len(lines)-2], "quit") {
		t.Errorf("los hints no están en la última línea de contenido: %q", lines[len(lines)-2])
	}
	if !strings.HasPrefix(lines[len(lines)-1], "╰") {
		t.Errorf("la vista no acaba en el borde inferior de la caja de atajos: %q", lines[len(lines)-1])
	}
}

func TestViewBoxesEveryLineAtTerminalWidth(t *testing.T) {
	m := longModel(t, 30)
	for i, l := range visibleLines(m.View().Content) {
		if w := ansi.StringWidth(l); w != m.width {
			t.Errorf("línea %d mide %d columnas, want %d:\n%q", i, w, m.width, l)
		}
	}
}

// The inbox sorts newest first, so "Item 60" is the first row and "Item 1" the last.
func TestScrollFollowsCursor(t *testing.T) {
	m := longModel(t, 60)
	if m.scroll != 0 {
		t.Fatalf("scroll inicial = %d, want 0", m.scroll)
	}

	m = press(t, m, "end")
	if it, ok := m.selected(); !ok || it.Title != "Item 1" {
		t.Fatalf("con end el cursor debería estar en la última fila, no en %q", it.Title)
	}
	view := stripANSI(m.View().Content)
	if !strings.Contains(view, "Item 1") {
		t.Errorf("la fila del cursor no se ve:\n%s", view)
	}
	if strings.Contains(view, "Item 60") {
		t.Errorf("la lista se desplazó de más: la primera fila ya no debería verse:\n%s", view)
	}
	if m.scroll == 0 {
		t.Error("la lista no se desplazó con el cursor al final")
	}

	m = press(t, m, "home")
	view = stripANSI(m.View().Content)
	if !strings.Contains(view, "Item 60") {
		t.Errorf("con el cursor al principio debe verse la primera fila:\n%s", view)
	}
	if strings.Contains(view, "Item 1") {
		t.Errorf("la lista no volvió arriba:\n%s", view)
	}
	if m.scroll != 0 {
		t.Errorf("scroll tras home = %d, want 0", m.scroll)
	}
}

func TestPageKeysMoveOneWindow(t *testing.T) {
	m := longModel(t, 60)
	page := m.pageRows()
	if page < 2 {
		t.Fatalf("pageRows = %d, want >= 2", page)
	}

	m = press(t, m, "pgdown")
	if m.cursor != page {
		t.Errorf("cursor tras pgdown = %d, want %d", m.cursor, page)
	}
	if m.scroll != page {
		t.Errorf("la ventana tras pgdown = %d, want %d (debe avanzar una página entera)", m.scroll, page)
	}
	m = press(t, m, "pgup")
	if m.cursor != 0 {
		t.Errorf("cursor tras pgup = %d, want 0", m.cursor)
	}
	if m.scroll != 0 {
		t.Errorf("la ventana tras pgup = %d, want 0", m.scroll)
	}

	m = press(t, m, "pgup")
	if m.cursor != 0 {
		t.Errorf("cursor tras pgup en el tope = %d, want 0", m.cursor)
	}
	for range 5 {
		m = press(t, m, "pgdown")
	}
	if m.cursor != len(m.rows())-1 {
		t.Errorf("cursor tras varios pgdown = %d, want %d", m.cursor, len(m.rows())-1)
	}
}

// These three cases are the ones that break if the condition is inverted or the boundary moves.
func TestScrollForAcomodaLaVentanaAlCursor(t *testing.T) {
	cases := []struct {
		name                         string
		current, target, total, view int
		want                         int
	}{
		{"el cursor ya se ve", 0, 3, 60, 10, 0},
		{"la última fila de la ventana cabe", 0, 9, 60, 10, 0},
		{"una por encima de la ventana sí desplaza", 0, 10, 60, 10, 1},
		{"el cursor está por encima", 5, 1, 60, 10, 1},
		{"el cursor es la primera línea", 5, 0, 60, 10, 0},
		{"la última fila se pega al borde", 0, 59, 60, 10, 50},
		{"el contenido cabe entero", 0, 5, 8, 10, 0},
		{"sin fila que seguir deja el scroll", 7, -1, 60, 10, 7},
		{"target negativo no arrastra el scroll a cero", 0, -1, 60, 10, 0},
		{"la ventana se acota al contenido", 40, 40, 12, 10, 2},
		{"una ventana mayor que el contenido deja arriba", 0, 0, 4, 10, 0},
		{"el scroll nunca queda negativo", -5, 0, 60, 10, 0},
		{"el scroll nunca se pasa del final", 99, 59, 60, 10, 50},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := scrollFor(c.current, c.target, c.total, c.view); got != c.want {
				t.Errorf("scrollFor(%d, %d, %d, %d) = %d, want %d",
					c.current, c.target, c.total, c.view, got, c.want)
			}
		})
	}
}

func TestCursorLineLocalizaLaFilaDelCursor(t *testing.T) {
	lines := []listLine{
		{text: "prefijo", row: -1},
		{text: "aviso", row: -1},
		{text: "header", row: -1},
		{text: "fila 0", row: 0},
		{text: "fila 1", row: 1},
		{text: "loading more", row: -1},
	}
	cases := map[int]int{0: 3, 1: 4, 2: -1, 99: -1}
	for cursor, want := range cases {
		if got := cursorLine(lines, cursor); got != want {
			t.Errorf("cursorLine(cursor=%d) = %d, want %d", cursor, got, want)
		}
	}
	if got := cursorLine([]listLine{{text: "vacío", row: -1}}, 0); got != -1 {
		t.Errorf("cursorLine sobre una lista sin filas = %d, want -1", got)
	}
	if got := cursorLine(nil, 0); got != -1 {
		t.Errorf("cursorLine(nil, 0) = %d, want -1", got)
	}
}

func TestVisibleListAcotaLaVentanaAlContenido(t *testing.T) {
	lines := make([]listLine, 10)
	for i := range lines {
		lines[i] = listLine{row: i}
	}

	got := visibleList(lines, 3, 4)
	if len(got) != 4 || got[0].row != 3 || got[3].row != 6 {
		t.Errorf("visibleList(3, 4) = %d líneas empezando en %d", len(got), got[0].row)
	}
	got = visibleList(lines, 99, 4)
	if len(got) != 4 || got[0].row != 6 {
		t.Errorf("visibleList(99, 4) = %d líneas empezando en %d, want 4 desde 6", len(got), got[0].row)
	}
	seis := lines[:6]
	got = visibleList(seis, 0, 8)
	if len(got) != 6 || got[0].row != 0 {
		t.Errorf("visibleList(6 líneas, view 8) = %d líneas, want 6 desde 0", len(got))
	}
	got = visibleList(seis, 3, 3)
	if len(got) != 3 || got[0].row != 3 {
		t.Errorf("visibleList(6 líneas, 3, 3) = %d líneas desde %d, want 3 desde 3", len(got), got[0].row)
	}
	got = visibleList(lines, -5, 3)
	if len(got) != 3 || got[0].row != 0 {
		t.Errorf("visibleList(-5, 3) = %d líneas empezando en %d, want 3 desde 0", len(got), got[0].row)
	}
	got = visibleList(lines, 99, 3)
	if len(got) != 3 || got[0].row != 7 {
		t.Errorf("visibleList(99, 3) = %d líneas empezando en %d, want 3 desde 7", len(got), got[0].row)
	}
	if visibleList(lines, 0, 0) != nil || visibleList(nil, 0, 5) != nil {
		t.Error("sin ventana o sin líneas visibleList debería devolver nil")
	}
}

// Rows are numbered with `row` so the cursor and the scroll find them, and the scroll depends on it
// matching the real position.
func TestListLinesComponeElCuerpoEnOrden(t *testing.T) {
	m := newTestModel(t, ghAdapter())
	m = send(t, m, page(1, "github", "github.com", model.SectionReview, model.ReviewRequested, []model.Item{
		mkItem("github", "github.com", "acme/widget", "Uno", 1, ""),
		mkItem("github", "github.com", "acme/widget", "Dos", 2, ""),
	}, false))

	lines := m.listLines(m.contentWidth())
	var rows []int
	for _, l := range lines {
		rows = append(rows, l.row)
	}
	want := []int{-1, -1, 0, 1}
	if len(rows) != len(want) {
		t.Fatalf("líneas = %v, want %v", rows, want)
	}
	for i := range want {
		if rows[i] != want[i] {
			t.Fatalf("rows = %v, want %v (la fila %d debe ser row %d)", rows, want, i, want[i])
		}
	}

	i0 := cursorLine(lines, 0)
	if i0 < 0 || lines[i0].row != 0 {
		t.Fatalf("cursorLine(0) = %d", i0)
	}
	conCursor := stripANSI(lines[i0].text)
	sinCursor := stripANSI(lines[cursorLine(lines, 1)].text)
	if conCursor == sinCursor {
		t.Errorf("la fila del cursor no se distingue de la otra:\n%s", conCursor)
	}

	// row: -1, so neither the cursor nor the scroll can mistake it for an item.
	m2 := newTestModel(t, ghAdapter())
	m2 = send(t, m2, page(1, "github", "github.com", model.SectionReview, model.ReviewRequested, []model.Item{
		mkItem("github", "github.com", "acme/widget", "Uno", 1, ""),
	}, true))
	lines = m2.listLines(m2.contentWidth())
	last := lines[len(lines)-1]
	if last.row != -1 || !strings.Contains(stripANSI(last.text), "loading more") {
		t.Errorf("la última línea = %+v, want el indicador de paginación con row -1", last)
	}
}

func TestListLinesVaciaPintaElEstadoVacio(t *testing.T) {
	m := newTestModel(t, ghAdapter())
	m = send(t, m, page(1, "github", "github.com", model.SectionReview, model.ReviewRequested, nil, false))

	lines := m.listLines(m.contentWidth())
	if len(lines) != 1 {
		t.Fatalf("líneas = %d (%v), want solo la del estado vacío", len(lines), lines)
	}
	if lines[0].row != -1 || !strings.Contains(stripANSI(lines[0].text), "(empty)") {
		t.Errorf("línea = %+v, want el estado vacío con row -1", lines[0])
	}
	m = send(t, m, pageMsg{
		cycle:    1,
		key:      streamKey{forge: "github", section: model.SectionReview, kind: model.ReviewRequested},
		warnings: []model.Warning{{Forge: "github", Section: model.SectionReview, Kind: "network", Msg: "dial tcp: timeout"}},
	})
	lines = m.listLines(m.contentWidth())
	joined := ""
	for _, l := range lines {
		joined += stripANSI(l.text)
	}
	if !strings.Contains(joined, "github: could not be queried") {
		t.Errorf("con un aviso debería pintarse el aviso del forge:\n%s", joined)
	}
	if strings.Contains(joined, "(empty)") {
		t.Errorf("con un aviso no debería pintarse también el estado vacío:\n%s", joined)
	}
	if lines[0].row != -1 {
		t.Errorf("el aviso debería ser la primera línea con row -1, es %+v", lines[0])
	}
}

// Not a test of syncScroll's `view <= 0` guard: that case cannot be reached, and the reason is here
// so nobody re-adds it.
func TestSyncScrollNoSeRompeEnTerminalesDiminutos(t *testing.T) {
	m := longModel(t, 60)
	m = press(t, m, "end")
	if it, ok := m.selected(); !ok || it.Title != "Item 1" {
		t.Fatalf("con end el cursor debería estar en la última fila, no en %q", it.Title)
	}

	for _, h := range []int{40, 12, 8, 5, 3, 1} {
		m.height = h
		m.syncScroll()
		lay := m.layout()
		if lay.bodyLines < 1 {
			t.Fatalf("height=%d: bodyLines = %d, pero el layout garantiza >= 1", h, lay.bodyLines)
		}
		lines := m.listLines(m.contentWidth())
		if m.scroll < 0 || m.scroll > max(0, len(lines)-1) {
			t.Errorf("height=%d: scroll = %d fuera de rango con %d líneas", h, m.scroll, len(lines))
		}
		if cl := cursorLine(lines, m.cursor); cl >= 0 {
			vis := visibleList(lines, m.scroll, lay.bodyLines)
			found := false
			for _, v := range vis {
				if v.row == m.cursor {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("height=%d: la fila del cursor no está en la ventana (línea %d de %d, %d visibles)",
					h, cl, len(lines), len(vis))
			}
		}
	}
}

// pad on a string that ALREADY fits must add nothing: adding would push the cell past its column and
// the table would dance as text is written.
func TestPadYTruncateMidenEnRunesYWEnBordes(t *testing.T) {
	if got := pad("ab", 4); got != "ab  " {
		t.Errorf("pad(ab,4) = %q, want \"ab  \"", got)
	}
	if got := pad("ab", 2); got != "ab" {
		t.Errorf("pad con la cadena justa = %q, want %q (no rellena de más)", got, "ab")
	}
	if got := pad("abcd", 2); got != "abcd" {
		t.Errorf("pad de una cadena más larga = %q, want sin cambios (queda al truncado)", got)
	}
	if got := utf8.RuneCountInString(pad("áé", 4)); got != 4 {
		t.Errorf("pad con acentos = %d runes, want 4", got)
	}
	if got := pad("👍", 3); utf8.RuneCountInString(got) != 3 {
		t.Errorf("pad con un emoji = %d runes, want 3", utf8.RuneCountInString(got))
	}

	if got := truncate("abcdef", 4); got != "abc…" {
		t.Errorf("truncate(abcdef,4) = %q, want \"abc…\"", got)
	}
	if got := utf8.RuneCountInString(truncate("abcdefgh", 5)); got != 5 {
		t.Errorf("truncate debe dar EXACTAMENTE 5 runes, dio %d", got)
	}
	if got := truncate("abc", 3); got != "abc" {
		t.Errorf("truncate de lo que cabe justo = %q, want sin cambios", got)
	}
	if got := truncate("abc", 10); got != "abc" {
		t.Errorf("truncate de lo que sobra sitio = %q, want sin cambios", got)
	}
	if got := truncate("abc", 0); got != "" {
		t.Errorf("truncate(abc,0) = %q, want vacío", got)
	}
	if got := truncate("abc", 1); got != "…" {
		t.Errorf("truncate(abc,1) = %q, want solo la elipsis (dos columnas en una)", got)
	}
	if got := truncate("abc", -1); got != "" {
		t.Errorf("truncate con ancho negativo = %q, want vacío", got)
	}
	if got := truncate("áéíóú", 3); got != "áé…" {
		t.Errorf("truncate con acentos = %q, want \"áé…\"", got)
	}
}

// Shared with the graphics layer on purpose (see overlay.go): if the frame and the image computed
// their own position they would land in different rectangles, and only when they happened to agree.
func TestCenteredOriginColocaLaCaja(t *testing.T) {
	cases := []struct {
		name                      string
		width, height, boxW, boxH int
		wantX, wantY              int
	}{
		{"caja centrada en un área mayor", 20, 10, 4, 4, 8, 3},
		{"caja que llena el área", 10, 10, 10, 10, 0, 0},
		{"más alta que el área", 20, 3, 4, 8, 8, 0},
		{"más ancha que el área", 3, 10, 10, 2, 0, 4},
		{"una fila de diferencia", 20, 11, 4, 4, 8, 3},
		{"impar por arriba", 21, 11, 4, 4, 8, 3},
		{"área de una línea", 20, 1, 4, 1, 8, 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			x, y := centeredOrigin(c.width, c.height, c.boxW, c.boxH)
			if x != c.wantX || y != c.wantY {
				t.Errorf("centeredOrigin(%d,%d,%d,%d) = %d,%d, want %d,%d",
					c.width, c.height, c.boxW, c.boxH, x, y, c.wantX, c.wantY)
			}
			if x < 0 || y < 0 {
				t.Errorf("origen negativo: x=%d y=%d", x, y)
			}
		})
	}
}

func TestOverlayCenteredRecortaLaCajaQueNoCabe(t *testing.T) {
	// A background WIDER than the scroll, because the overlay clips the line where the box needs room
	// rather than deleting the line: with a shorter background the box text would stick to the end of the line.
	fondo := "aaaa\naaaa\naaaa\naaaa"
	got := overlayCentered(fondo, "1\n2\n3\n4\n5\n6", 10)
	lines := strings.Split(got, "\n")
	if len(lines) != 4 {
		t.Fatalf("líneas = %d, want 4 (el fondo no crece):\n%s", len(lines), got)
	}
	for i, want := range []string{"aaaa1", "aaaa2", "aaaa3", "aaaa4"} {
		if lines[i] != want {
			t.Errorf("línea %d = %q, want %q (la caja se pinta encima del fondo)", i, lines[i], want)
		}
	}
	if got := overlayCentered(fondo, "", 10); got != fondo {
		t.Errorf("caja vacía = %q, want la vista intacta", got)
	}
	got = overlayCentered("aaaaaaaaaa", "0123456789", 6)
	linea := strings.Split(got, "\n")[0]
	if linea != "012345aaaa" {
		t.Errorf("línea = %q, want \"012345aaaa\" (caja recortada al ancho, fondo detrás)", linea)
	}
}

// Header and rows share the layout on purpose, so writing over the table does not move a column.
func TestLaFilaPintaLasCeldasQueCabenEsas(t *testing.T) {
	m := longModel(t, 3)
	lay := newRefLayout([]inbox.Section{{Kind: m.activeSection, Items: m.rows()}}, m.prefixMode)

	for width := 20; width <= 140; width++ {
		m.width = width
		inner := m.contentWidth()
		want := fitColumns(lay, inner-2)
		wantW := 2
		for _, c := range lay.cols[:want] {
			wantW += c.width
		}

		for _, l := range m.listLines(inner) {
			if l.row < 0 {
				continue
			}
			if got := ansi.StringWidth(stripANSI(l.text)); got != wantW {
				t.Fatalf("ancho %d: la fila %d mide %d columnas, want %d (las de %d celdas que caben)",
					width, l.row, got, wantW, want)
			}
		}
	}
}

// The `inner-2` listLines passes to the header is the list's quietest decision: it does not show in
// the width (the header is always shorter than the rows) but in WHICH columns appear.
func TestElHeaderMuestraLasColumnasQueCabenEsas(t *testing.T) {
	m := longModel(t, 6)
	for width := 20; width <= 140; width += 2 {
		m.width = width
		inner := m.contentWidth()
		lay := newRefLayout([]inbox.Section{{Kind: m.activeSection, Items: m.rows()}}, m.prefixMode)
		want := fitColumns(lay, inner-2)

		lines := m.listLines(inner)
		hi := -1
		for i, l := range lines {
			if l.row == 0 {
				hi = i
			}
		}
		if hi < 1 {
			t.Fatalf("ancho %d: no encuentro la primera fila en %d líneas", width, len(lines))
		}
		header := stripANSI(lines[hi-1].text)

		for i := range want {
			if !strings.Contains(header, lay.cols[i].title) {
				t.Fatalf("ancho %d: faltan las %d primeras columnas en el header, que dice %q", width, want, header)
			}
		}
		if want < len(lay.cols) && strings.Contains(header, lay.cols[want].title) {
			t.Fatalf("ancho %d: la columna %d (%q) no cabe y no debería estar en el header: %q",
				width, want, lay.cols[want].title, header)
		}
	}
}

func TestFitColumnsDejaSiempreForge(t *testing.T) {
	m := longModel(t, 3)
	lay := newRefLayout([]inbox.Section{{Kind: m.activeSection, Items: m.rows()}}, m.prefixMode)
	total := len(lay.cols)
	if total < 2 {
		t.Fatalf("la layout necesita al menos FORGE y otra columna, tiene %d", total)
	}

	if got := fitColumns(lay, 100_000); got != total {
		t.Errorf("con ancho de sobra caben %d columnas, dio %d", total, got)
	}
	usado := 0
	for k := 1; k <= total; k++ {
		borde := usado + lay.cols[k-1].width
		if got := fitColumns(lay, borde); got != k {
			t.Errorf("con el ancho justo de %d columnas caben %d, dio %d", k, k, got)
		}
		if k > 1 {
			if got := fitColumns(lay, borde-1); got != k-1 {
				t.Errorf("con el ancho de %d columnas caben %d y con %d caben %d, dio %d", k, k, k-1, k-1, got)
			}
		}
		usado = borde
	}
	for _, w := range []int{0, 1, -5, -100} {
		if got := fitColumns(lay, w); got != 1 {
			t.Errorf("fitColumns(%d) = %d, want 1 (FORGE siempre entra)", w, got)
		}
	}
}

func TestElMarcadorDelCursorVaEnSuFila(t *testing.T) {
	m := longModel(t, 4)

	for cursor := range 4 {
		m.cursor = cursor
		lines := m.listLines(m.contentWidth())
		for _, l := range lines {
			if l.row < 0 {
				continue
			}
			plano := stripANSI(l.text)
			marcado := strings.Contains(plano, "▸")
			if want := l.row == cursor; marcado != want {
				t.Errorf("fila %d con cursor en %d: marcada=%v, want %v (%q)", l.row, cursor, marcado, want, plano)
			}
		}
	}
}

// Both items sit in the active section with the same UpdatedAt, so the order between them is
// deterministic by number.
func TestDetailPaneShowsSelectedItem(t *testing.T) {
	m := newTestModel(t, ghAdapter(), &testutil.FakeAdapter{ForgeName: "gitlab", HostName: "gitlab.example.com"})
	first := mkItem("github", "github.com", "acme/widget", "PR de github", 1, "")
	second := mkItem("gitlab", "gitlab.example.com", "grp/proj", "MR de gitlab", 2, "")
	first.UpdatedAt = time.Unix(1000, 0)
	second.UpdatedAt = time.Unix(1000, 0)
	m = send(t, m, page(1, "github", "github.com", model.SectionReview, model.ReviewRequested, []model.Item{first}, false))
	m = send(t, m, page(1, "gitlab", "gitlab.example.com", model.SectionReview, model.ReviewRequested, []model.Item{second}, false))

	view := stripANSI(m.View().Content)
	for _, want := range []string{"PR de github", "github@github.com", "Item", "Author", "State"} {
		if !strings.Contains(view, want) {
			t.Errorf("el panel no contiene %q:\n%s", want, view)
		}
	}

	m = press(t, m, "down")
	view = stripANSI(m.View().Content)
	for _, want := range []string{"MR de gitlab", "gitlab@gitlab.example.com"} {
		if !strings.Contains(view, want) {
			t.Errorf("tras mover el cursor el panel no contiene %q:\n%s", want, view)
		}
	}
	if strings.Contains(view, "github@github.com") {
		t.Errorf("el panel sigue mostrando el forge del ítem anterior:\n%s", view)
	}
}

func TestDetailPaneEmptyWithoutSelection(t *testing.T) {
	m := newTestModel(t, ghAdapter())
	view := stripANSI(m.View().Content)
	if !strings.Contains(view, "no selection") {
		t.Errorf("con el inbox vacío el panel debería avisar:\n%s", view)
	}
	if !hasBoxTitle(visibleLines(m.View().Content), "Detail") {
		t.Errorf("la caja del panel debería titularse Detail sin selección:\n%s", view)
	}
}

func TestDetailPaneFitsShortScreenInTwoColumns(t *testing.T) {
	m := newTestModel(t, ghAdapter())
	m.width, m.height = 120, 24
	m = send(t, m, page(1, "github", "github.com", model.SectionReview, model.ReviewRequested, []model.Item{
		mkItem("github", "github.com", "acme/widget", "Add widget", 1, ""),
	}, false))

	detail := m.layout().detailLines
	if detail != 9 {
		t.Fatalf("panel de %d líneas, want 9 (el 40%% de 24)", detail)
	}
	it, ok := m.selected()
	lines := m.detailLines(it, ok, detail)
	if len(lines) != detail {
		t.Fatalf("el panel devolvió %d líneas, want %d", len(lines), detail)
	}
	joined := stripANSI(strings.Join(lines, "\n"))
	for _, want := range []string{"Add widget", "Item:", "Forge:", "github@github.com", "Author:", "State:", "Role:"} {
		if !strings.Contains(joined, want) {
			t.Errorf("el panel de dos columnas perdió %q:\n%s", want, joined)
		}
	}
}

func TestDetailPaneClipsOnlyWhenTooSmall(t *testing.T) {
	m := longModel(t, 1)
	it, ok := m.selected()
	clipped := m.detailLines(it, ok, 4)
	if len(clipped) != 4 {
		t.Fatalf("el recorte devolvió %d líneas, want 4", len(clipped))
	}
	joined := stripANSI(strings.Join(clipped, "\n"))
	for _, want := range []string{"Review:", "Role:"} {
		if !strings.Contains(joined, want) {
			t.Errorf("el recorte perdió %q:\n%s", want, joined)
		}
	}
	if strings.Contains(joined, "Forja:") || strings.Contains(joined, "Item:") {
		t.Errorf("el recorte debería haber dejado caer la cabecera del detalle:\n%s", joined)
	}
}

func TestDetailShowsDiffStat(t *testing.T) {
	it := mkItem("gitlab", "gitlab.example.com", "grp/proj", "MR", 1015, "")
	it.Diff = model.DiffStat{Additions: 42, Deletions: 1, Files: 2, Known: true}
	m := newTestModel(t, ghAdapter(), &testutil.FakeAdapter{ForgeName: "gitlab", HostName: "gitlab.example.com"})
	m = send(t, m, page(1, "gitlab", "gitlab.example.com", model.SectionReview, model.ReviewRequested, []model.Item{it}, false))

	view := stripANSI(m.View().Content)
	if !strings.Contains(view, "+42 -1 (2 files)") {
		t.Errorf("el detalle debería mostrar el diffstat agregado:\n%s", view)
	}
}

func TestDetailDiffUnknownAndEmpty(t *testing.T) {
	cases := []struct {
		name string
		d    model.DiffStat
		want string
	}{
		{"sin datos", model.DiffStat{}, "unknown (forge did not report it)"},
		{"sin cambios", model.DiffStat{Known: true}, "no changes"},
		{"un fichero", model.DiffStat{Additions: 3, Deletions: 0, Files: 1, Known: true}, "+3 -0 (1 file)"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			it := mkItem("github", "github.com", "acme/widget", "PR", 1, "")
			it.Diff = c.d
			m := newTestModel(t, ghAdapter())
			m = send(t, m, page(1, "github", "github.com", model.SectionReview, model.ReviewRequested, []model.Item{it}, false))
			view := stripANSI(m.View().Content)
			if !strings.Contains(view, c.want) {
				t.Errorf("el detalle debería mostrar %q:\n%s", c.want, view)
			}
		})
	}
}

func TestDetailDropsDiffBeforeLosingTheTitle(t *testing.T) {
	detail := func(rows int) string {
		m := newTestModel(t, ghAdapter())
		m.width, m.height = 120, 24
		it := mkItem("github", "github.com", "acme/widget", "Add widget", 1, "")
		it.Diff = model.DiffStat{Additions: 42, Deletions: 1, Files: 2, Known: true}
		m = send(t, m, page(1, "github", "github.com", model.SectionReview, model.ReviewRequested, []model.Item{it}, false))
		m.denied[it.ID()] = "the forge refused the action"
		return stripANSI(strings.Join(m.detailPane(rows), "\n"))
	}

	tight := detail(9)
	if strings.Contains(tight, "Diff:") {
		t.Errorf("a 9 filas el diffstat debería caerse antes que el título:\n%s", tight)
	}
	if !strings.Contains(tight, "Add widget") {
		t.Errorf("el título no debería caerse por culpa del diffstat:\n%s", tight)
	}
	if !strings.Contains(tight, "Role:") {
		t.Errorf("el rol dice si la acción procede y no puede caerse:\n%s", tight)
	}

	if roomy := detail(11); !strings.Contains(roomy, "+42 -1 (2 files)") {
		t.Errorf("a 11 filas el diffstat debería estar:\n%s", roomy)
	}
}
