// Tests de la ventana de lista: reparto vertical, scroll que sigue al cursor y
// panel de detalle del ítem seleccionado.
package tui

import (
	"strconv"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

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

// TestViewBoxesEverySection es el motivo del refactor: cada región de pantalla
// es una caja con borde redondeado y título embebido en su línea superior. Sin
// esto la vista se lee como un bloque plano y no se distingue dónde acaba la
// lista y empieza el detalle.
func TestViewBoxesEverySection(t *testing.T) {
	m := newTestModel(t, ghAdapter())
	m = send(t, m, page(1, "github", "github.com", model.SectionAuthored, "", []model.Item{
		mkItem("github", "github.com", "acme/widget", "Add widget", 1, ""),
	}, false))

	lines := visibleLines(m.View().Content)
	if len(lines) != m.height {
		t.Fatalf("la vista tiene %d líneas, want %d (la altura del terminal)", len(lines), m.height)
	}

	// Los cuatro títulos van embebidos en la línea superior de su caja, y cada
	// caja cierra con su borde inferior.
	for _, want := range []string{"PRDash", "Inbox", "Keybinds"} {
		if !hasBoxTitle(lines, want) {
			t.Errorf("falta la caja %q:\n%s", want, strings.Join(lines, "\n"))
		}
	}
	// La caja del detalle se titula con la referencia del ítem: es lo que dice de
	// un vistazo sobre qué ficha se trata.
	if !hasBoxTitle(lines, "acme/widget#1") {
		t.Errorf("la caja del detalle no se titula con la referencia del ítem:\n%s", strings.Join(lines, "\n"))
	}

	// Las esquinas de las cajas: cada ╭ abre y su ╯ correspondiente cierra.
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

// hasBoxTitle busca una caja cuyo borde superior embeba title.
func hasBoxTitle(lines []string, title string) bool {
	for _, l := range lines {
		if strings.HasPrefix(l, "╭") && strings.Contains(l, " "+title+" ") {
			return true
		}
	}
	return false
}

// TestViewFitsTerminalHeight es el motivo del scroll: con más ítems que líneas
// la vista ya no cabe en el terminal, así que hay que recortar en vez de
// desbordar (si no, la cabecera y la caja de atajos se van de pantalla).
func TestViewFitsTerminalHeight(t *testing.T) {
	m := longModel(t, 60)
	lines := visibleLines(m.View().Content)
	if len(lines) != m.height {
		t.Errorf("la vista tiene %d líneas, want %d (la altura del terminal)", len(lines), m.height)
	}
	// La caja de cabecera es la primera y lleva el nombre de la app embebido en
	// la línea de borde, como las demás.
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

// TestViewBoxesEveryLineAtTerminalWidth comprueba que la caja no se descuadra en
// horizontal: el borde se come 2 columnas del ancho de la terminal, y una línea
// más ancha que el interior se recortaría y rompería el borde.
func TestViewBoxesEveryLineAtTerminalWidth(t *testing.T) {
	m := longModel(t, 30)
	for i, l := range visibleLines(m.View().Content) {
		if w := ansi.StringWidth(l); w != m.width {
			t.Errorf("línea %d mide %d columnas, want %d:\n%q", i, w, m.width, l)
		}
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
// vacío: el panel lo dice, y su caja se titula "Detail" porque no hay ninguna
// referencia que poner.
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

// TestDetailPaneClipsOnlyWhenTooSmall comprueba el último recurso: si ni
// siquiera en dos columnas cabe, se recorta por arriba lo que falta. El final
// (estado, review y rol) es lo que dice si la acción procede.
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

// TestDetailShowsDiffStat: el detalle es donde el diffstat se ve siempre, con
// las cifras sin compactar y el recuento de ficheros, porque la columna de la
// lista abrevia y puede no estar.
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

// TestDetailDiffUnknownAndEmpty: los dos ceros significan cosas distintas y el
// detalle las distingue. Desconocido lo dice, para que un "-" no se lea como un
// MR que no toca nada.
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

// TestDetailDropsDiffBeforeLosingTheTitle: al añadir el diffstat la rejilla pasó
// de 6 a 7 filas. Cuando el panel no cabe, el diffstat es lo primero que se cae
// —nunca se vio, no se pierde— y el título se conserva.
func TestDetailDropsDiffBeforeLosingTheTitle(t *testing.T) {
	detail := func(rows int) string {
		m := newTestModel(t, ghAdapter())
		m.width, m.height = 120, 24
		it := mkItem("github", "github.com", "acme/widget", "Add widget", 1, "")
		it.Diff = model.DiffStat{Additions: 42, Deletions: 1, Files: 2, Known: true}
		m = send(t, m, page(1, "github", "github.com", model.SectionReview, model.ReviewRequested, []model.Item{it}, false))
		// El aviso de acción deshabilitada es el del forge (`denied`), el único que la
		// ficha sigue pintando: ocupa dos filas (blanco + texto) y es lo que obliga a
		// la rejilla a ceder.
		m.denied[it.ID()] = "the forge refused the action"
		return stripANSI(strings.Join(m.detailPane(rows), "\n"))
	}

	// 9 filas es el 40% de un terminal de 24: no cabe la rejilla de 7 con el
	// título y el aviso de acción, así que el diffstat se va.
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

	// Con sitio, el diffstat sale: el detalle es la vista que no pierde campos.
	if roomy := detail(11); !strings.Contains(roomy, "+42 -1 (2 files)") {
		t.Errorf("a 11 filas el diffstat debería estar:\n%s", roomy)
	}
}
