// Tests de la ventana de lista: reparto vertical, scroll que sigue al cursor y
// panel de detalle del ítem seleccionado.
package tui

import (
	"strconv"
	"strings"
	"testing"

	"prdash/internal/forge/model"
	"prdash/internal/testutil"
)

// manyItems compone n ítems en la sección de creados, con título indexado para
// poder localizar una fila concreta en el texto pintado.
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
	return send(t, m, page(1, "github", "github.com", model.SectionAuthored, "", manyItems(n), false))
}

// visibleLines cuenta las líneas de la vista ya sin códigos ANSI.
func visibleLines(view string) []string {
	return strings.Split(stripANSI(view), "\n")
}

// TestSplitRowsReservesFortyPercent comprueba el reparto: el detalle se queda
// con el 40% de la pantalla y la lista con el hueco que queda tras el chrome.
func TestSplitRowsReservesFortyPercent(t *testing.T) {
	for _, tc := range []struct{ height, chrome, list, detail int }{
		{40, 4, 20, 16}, // 16/40 = 40%
		{30, 4, 14, 12}, // 12/30 = 40%
		{24, 4, 11, 9},  // 9/24 = 37%, redondeo a la baja por entero
	} {
		list, detail := splitRows(tc.height, tc.chrome)
		if list != tc.list || detail != tc.detail {
			t.Errorf("splitRows(%d, %d) = %d, %d; want %d, %d", tc.height, tc.chrome, list, detail, tc.list, tc.detail)
		}
		if got := list + detail + tc.chrome; got != tc.height {
			t.Errorf("splitRows(%d, %d) suma %d líneas, want %d", tc.height, tc.chrome, got, tc.height)
		}
	}

	// Sin altura conocida no se recorta nada: es el primer render, antes del
	// primer WindowSizeMsg.
	if list, detail := splitRows(0, 4); list != 0 || detail != 0 {
		t.Errorf("splitRows(0, 4) = %d, %d; want 0, 0", list, detail)
	}

	// Pantalla diminuta: la lista conserva su mínimo y nada sale negativo.
	list, detail := splitRows(12, 4)
	if list < minListRows || detail < 0 || list+detail+4 > 12 {
		t.Errorf("splitRows(12, 4) = %d, %d; la lista debe conservar %d y todo sumar 12", list, detail, minListRows)
	}
}

// TestViewFitsTerminalHeight es el motivo del scroll: con más ítems que líneas
// la vista ya no cabe en el terminal, así que hay que recortar en vez de
// desbordar (si no, la cabecera y los hints se van de pantalla).
func TestViewFitsTerminalHeight(t *testing.T) {
	m := longModel(t, 60)
	lines := visibleLines(m.View().Content)
	if len(lines) != m.height {
		t.Errorf("la vista tiene %d líneas, want %d (la altura del terminal)", len(lines), m.height)
	}
	if !strings.HasPrefix(lines[0], "prdash") {
		t.Errorf("la cabecera no está en la primera línea: %q", lines[0])
	}
	if !strings.Contains(lines[len(lines)-1], "quit") {
		t.Errorf("los hints no están en la última línea: %q", lines[len(lines)-1])
	}
}

// TestScrollFollowsCursor comprueba el auto-scroll: al mover el cursor fuera de
// la ventana, la lista se desplaza lo justo para que la fila siga a la vista.
// El inbox ordena de más reciente a más antiguo, así que "Item 60" es la primera
// fila y "Item 1" la última.
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

// TestPageKeysMoveOneWindow comprueba que pgup/pgdn mueven el cursor una
// ventana entera, que es lo que el usuario ve avanzar.
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

	// Salir por encima o por debajo no rompe nada.
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

// TestDetailPaneShowsSelectedItem es el panel inferior: describe el ítem bajo el
// cursor y cambia al moverlo, con la ruta completa del forge.
func TestDetailPaneShowsSelectedItem(t *testing.T) {
	m := newTestModel(t, ghAdapter(), &testutil.FakeAdapter{ForgeName: "gitlab", HostName: "gitlab.example.com"})
	m = send(t, m, page(1, "github", "github.com", model.SectionAuthored, "", []model.Item{
		mkItem("github", "github.com", "acme/widget", "PR de github", 1, ""),
	}, false))
	m = send(t, m, page(1, "gitlab", "gitlab.example.com", model.SectionReview, model.ReviewRequested, []model.Item{
		mkItem("gitlab", "gitlab.example.com", "grp/proj", "MR de gitlab", 2, "REVIEW_REQUIRED"),
	}, false))

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

// TestDetailPaneEmptyWithoutSelection evita inventar datos cuando el inbox está
// vacío: el panel lo dice.
func TestDetailPaneEmptyWithoutSelection(t *testing.T) {
	m := newTestModel(t, ghAdapter())
	view := stripANSI(m.View().Content)
	if !strings.Contains(view, "no selection") {
		t.Errorf("con el inbox vacío el panel debería avisar:\n%s", view)
	}
}

// TestDetailPaneFitsShortScreenInTwoColumns comprueba el fallback: en una
// pantalla de 24 filas el 40% son 9 líneas y el detalle en una columna no cabe,
// así que se reparte en dos columnas antes que recortar campos. Forge y Autor
// solo existen en el panel, así que perderlos sería la peor opción.
func TestDetailPaneFitsShortScreenInTwoColumns(t *testing.T) {
	m := newTestModel(t, ghAdapter())
	m.width, m.height = 120, 24
	m = send(t, m, page(1, "github", "github.com", model.SectionAuthored, "", []model.Item{
		mkItem("github", "github.com", "acme/widget", "Add widget", 1, ""),
	}, false))

	_, detail := splitRows(m.height, m.chromeLines())
	if detail != 9 {
		t.Fatalf("panel de %d líneas, want 9 (el 40%% de 24)", detail)
	}
	lines := m.detailPane(detail)
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

// TestDetailPaneClipsOnlyWhenTooSmall comprueba el último recurso: si ni
// siquiera en dos columnas cabe, se recorta por arriba lo que falta. El final
// (estado, review y rol) es lo que dice si la acción procede.
func TestDetailPaneClipsOnlyWhenTooSmall(t *testing.T) {
	m := longModel(t, 1)
	clipped := m.detailPane(4)
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
