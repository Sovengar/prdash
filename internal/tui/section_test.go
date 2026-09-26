// Tests del inbox de una sola sección: sección activa y su ciclo, leyenda de
// conteos del borde, prefijo de la activa, posición recordada por sección y los
// estados de la activa. Cada test deriva de un escenario de behavior.feature.
package tui

import (
	"strconv"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"prdash/internal/forge/model"
)

// ghItems crea ítems de GitHub con rutas concretas, todos en el mismo forge.
func ghItems(projects ...string) []model.Item {
	items := make([]model.Item, 0, len(projects))
	for i, p := range projects {
		items = append(items, mkItem("github", "github.com", p, "T", 100+i, ""))
	}
	return items
}

// numberedItems crea n ítems de una misma sección con proyecto propio, para que
// la leyenda cuente exactamente n sin que la deduplicación los colapse con los
// de otra sección.
func numberedItems(n int, project string) []model.Item {
	items := make([]model.Item, 0, n)
	for i := 1; i <= n; i++ {
		items = append(items, mkItem("github", "github.com", project, "Item "+strconv.Itoa(i), i, ""))
	}
	return items
}

// listText es el texto plano de la lista de la sección activa.
func listText(m Model) string {
	return stripANSI(strings.Join(textOf(m.listLines(m.contentWidth())), "\n"))
}

// TestDefaultSectionIsAssigned cubre "al abrir, la sección activa es Assigned" y
// que solo se ven sus ítems.
func TestDefaultSectionIsAssigned(t *testing.T) {
	m := newTestModel(t, ghAdapter())
	m = send(t, m, page(1, "github", "github.com", model.SectionAuthored, "", []model.Item{
		mkItem("github", "github.com", "acme/mine", "Mine item", 1, ""),
	}, false))
	m = send(t, m, page(1, "github", "github.com", model.SectionReview, model.ReviewRequested, []model.Item{
		mkItem("github", "github.com", "acme/assigned", "Assigned item", 2, ""),
	}, false))
	m = send(t, m, page(1, "github", "github.com", model.SectionMentions, "", []model.Item{
		mkItem("github", "github.com", "acme/mentioned", "Mentioned item", 3, ""),
	}, false))

	if m.activeSection != model.SectionReview {
		t.Fatalf("activeSection = %q, want %q (Assigned)", m.activeSection, model.SectionReview)
	}
	view := stripANSI(m.View().Content)
	if !strings.Contains(view, "Assigned item") {
		t.Errorf("la lista debería mostrar los ítems de Assigned:\n%s", view)
	}
	for _, hidden := range []string{"Mine item", "Mentioned item"} {
		if strings.Contains(view, hidden) {
			t.Errorf("la lista muestra %q, que es de otra sección:\n%s", hidden, view)
		}
	}
}

// TestCycleThroughEmptySection cubre "tab cicla también cuando una sección está
// vacía": la activa pasa a la vacía y muestra su estado y su conteo.
func TestCycleThroughEmptySection(t *testing.T) {
	m := newTestModel(t, ghAdapter())
	m = send(t, m, page(1, "github", "github.com", model.SectionReview, model.ReviewRequested, []model.Item{
		mkItem("github", "github.com", "acme/widget", "Assigned", 1, ""),
	}, false))

	m = press(t, m, "tab")
	if m.activeSection != model.SectionMentions {
		t.Fatalf("activeSection = %q, want %q", m.activeSection, model.SectionMentions)
	}
	view := stripANSI(m.View().Content)
	if !strings.Contains(view, "(empty)") {
		t.Errorf("la sección vacía debería marcar (empty):\n%s", view)
	}
	if !strings.Contains(view, "Mentioned (0)") {
		t.Errorf("la leyenda debería mostrar 0 para la vacía:\n%s", view)
	}

	// El ciclo sigue aunque la sección esté vacía.
	m = press(t, m, "tab")
	if m.activeSection != model.SectionAuthored {
		t.Fatalf("activeSection = %q, want %q (sigue ciclando)", m.activeSection, model.SectionAuthored)
	}
}

// TestLegendExactFormatReplacesInbox cubre el formato exacto de la leyenda y que
// el título "Inbox" ya no existe.
func TestLegendExactFormatReplacesInbox(t *testing.T) {
	m := newTestModel(t, ghAdapter())
	m = send(t, m, page(1, "github", "github.com", model.SectionAuthored, "", numberedItems(9, "mine/repo"), false))
	m = send(t, m, page(1, "github", "github.com", model.SectionReview, model.ReviewRequested, numberedItems(4, "assigned/repo"), false))

	lines := visibleLines(m.View().Content)
	if !hasBoxTitle(lines, "Mine (9) · Assigned (4) · Mentioned (0)") {
		t.Errorf("la leyenda no tiene el formato exacto:\n%s", strings.Join(lines, "\n"))
	}
	if hasBoxTitle(lines, "Inbox") {
		t.Errorf("el título Inbox ya no debería existir:\n%s", strings.Join(lines, "\n"))
	}
}

// TestLegendHighlightsActiveSection cubre que la activa va resaltada y las demás
// atenuadas.
func TestLegendHighlightsActiveSection(t *testing.T) {
	m := newTestModel(t, ghAdapter())
	m = send(t, m, page(1, "github", "github.com", model.SectionReview, model.ReviewRequested, []model.Item{
		mkItem("github", "github.com", "acme/widget", "Assigned", 1, ""),
	}, false))

	raw := m.View().Content
	if !strings.Contains(raw, styleLegendActive.Render("Assigned (1)")) {
		t.Errorf("la sección activa debería ir resaltada en la leyenda")
	}
	for _, dim := range []string{"Mine (0)", "Mentioned (0)"} {
		if !strings.Contains(raw, styleDim.Render(dim)) {
			t.Errorf("%q debería ir atenuada en la leyenda", dim)
		}
	}
}

// TestLegendCountsDedupedSection cubre que los conteos son los de la sección
// deduplicada: un ítem que aparecería en dos secciones cuenta solo en la de
// mayor autoridad.
func TestLegendCountsDedupedSection(t *testing.T) {
	m := newTestModel(t, ghAdapter())
	dup := mkItem("github", "github.com", "acme/widget", "Dup", 1, "")
	m = send(t, m, page(1, "github", "github.com", model.SectionAuthored, "", []model.Item{dup}, false))
	m = send(t, m, page(1, "github", "github.com", model.SectionReview, model.ReviewRequested, []model.Item{dup}, false))

	view := stripANSI(m.View().Content)
	if !strings.Contains(view, "Mine (1) · Assigned (0) · Mentioned (0)") {
		t.Errorf("la leyenda no refleja la deduplicación por autoridad:\n%s", view)
	}
}

// TestPrefixFollowsActiveSection cubre que la vista muestra el prefijo de la
// activa y no el de otra sección.
func TestPrefixFollowsActiveSection(t *testing.T) {
	m := newTestModel(t, ghAdapter())
	m = send(t, m, page(1, "github", "github.com", model.SectionAuthored, "", ghItems("mine/group/api-a", "mine/group/api-b"), false))
	m = send(t, m, page(1, "github", "github.com", model.SectionReview, model.ReviewRequested, ghItems("app/vsocial/backend/api-gateway", "app/vsocial/backend/web-app"), false))

	if got := listText(m); !strings.Contains(got, "app/vsocial/backend/") || strings.Contains(got, "mine/group/") {
		t.Errorf("con Assigned activa la lista debería mostrar su prefijo, no el de Mine:\n%s", got)
	}

	// tab hasta Mine (Assigned → Mentioned → Mine), saltando la vacía.
	m = press(t, m, "tab")
	m = press(t, m, "tab")
	if m.activeSection != model.SectionAuthored {
		t.Fatalf("activeSection = %q, want %q", m.activeSection, model.SectionAuthored)
	}
	if got := listText(m); !strings.Contains(got, "mine/group/") || strings.Contains(got, "app/vsocial/backend/") {
		t.Errorf("con Mine activa la lista debería mostrar su prefijo:\n%s", got)
	}
}

// TestPrefixKeepsTableWidth cubre que el prefijo no desalinea la tabla a ningún
// ancho y que las celdas ITEM conservan la hoja y el "#número".
func TestPrefixKeepsTableWidth(t *testing.T) {
	for _, width := range []int{70, 124, 200} {
		m := newTestModel(t, ghAdapter())
		m.width, m.height = width, 40
		m = send(t, m, page(1, "github", "github.com", model.SectionReview, model.ReviewRequested, ghItems(
			"APPCITTI/vsocial/backend/api-gateway",
			"APPCITTI/vsocial/backend/web-app",
		), false))

		view := m.View().Content
		for i, l := range visibleLines(view) {
			if w := ansi.StringWidth(l); w != m.width {
				t.Errorf("ancho %d: línea %d mide %d columnas:\n%q", width, i, w, l)
			}
		}
		if !strings.Contains(stripANSI(view), "api-gateway#100") {
			t.Errorf("ancho %d: la celda ITEM perdió la hoja y el número:\n%s", width, stripANSI(view))
		}
	}
}

// TestRememberCursorAndScrollPerSection cubre que cada sección recuerda su
// cursor y su scroll al volver a ella.
func TestRememberCursorAndScrollPerSection(t *testing.T) {
	m := newTestModel(t, ghAdapter())
	m.width, m.height = 160, 24
	m = send(t, m, page(1, "github", "github.com", model.SectionReview, model.ReviewRequested, manyItems(60), false))
	m = send(t, m, page(1, "github", "github.com", model.SectionAuthored, "", []model.Item{
		mkItem("github", "github.com", "acme/mine-a", "Mine A", 1, ""),
		mkItem("github", "github.com", "acme/mine-b", "Mine B", 2, ""),
	}, false))

	m = press(t, m, "end")
	assigned := sectionPos{cursor: m.cursor, scroll: m.scroll}
	if assigned.cursor == 0 || assigned.scroll == 0 {
		t.Fatalf("el caso necesita cursor y scroll movidos en Assigned: %+v", assigned)
	}

	// Assigned → Mentioned (vacía) → Mine.
	m = press(t, m, "tab")
	m = press(t, m, "tab")
	if m.activeSection != model.SectionAuthored {
		t.Fatalf("activeSection = %q, want %q", m.activeSection, model.SectionAuthored)
	}
	if m.cursor != 0 || m.scroll != 0 {
		t.Fatalf("Mine debe abrirse en su posición por defecto: cursor=%d scroll=%d", m.cursor, m.scroll)
	}
	m = press(t, m, "down")
	mine := sectionPos{cursor: m.cursor, scroll: m.scroll}

	// Volver a Assigned recupera su posición.
	m = press(t, m, "tab")
	if m.activeSection != model.SectionReview {
		t.Fatalf("activeSection = %q, want %q", m.activeSection, model.SectionReview)
	}
	if m.cursor != assigned.cursor || m.scroll != assigned.scroll {
		t.Errorf("Assigned no recuperó su posición: cursor=%d scroll=%d, want %+v", m.cursor, m.scroll, assigned)
	}
	// Y volver a Mine recupera la suya.
	m = press(t, m, "tab")
	m = press(t, m, "tab")
	if m.cursor != mine.cursor || m.scroll != mine.scroll {
		t.Errorf("Mine no recuperó su posición: cursor=%d scroll=%d, want %+v", m.cursor, m.scroll, mine)
	}
}

// TestRefreshKeepsSectionPosition cubre que un refresco conserva la posición de
// la activa, acotada al nuevo contenido.
func TestRefreshKeepsSectionPosition(t *testing.T) {
	m := newTestModel(t, ghAdapter())
	m.width, m.height = 160, 24
	m = send(t, m, page(m.cycle, "github", "github.com", model.SectionReview, model.ReviewRequested, manyItems(60), false))
	m = press(t, m, "end")
	before := sectionPos{cursor: m.cursor, scroll: m.scroll}

	// Un refresco con el mismo contenido no mueve la posición.
	m = send(t, m, page(m.cycle, "github", "github.com", model.SectionReview, model.ReviewRequested, manyItems(60), false))
	if m.cursor != before.cursor || m.scroll != before.scroll {
		t.Errorf("el refresco movió la posición: cursor=%d scroll=%d, want %+v", m.cursor, m.scroll, before)
	}

	// Con menos contenido, la posición se acota.
	m = send(t, m, page(m.cycle, "github", "github.com", model.SectionReview, model.ReviewRequested, manyItems(3), false))
	if m.cursor > len(m.rows())-1 || m.cursor < 0 {
		t.Errorf("cursor = %d fuera de las %d filas nuevas", m.cursor, len(m.rows()))
	}
	if m.scroll != 0 {
		t.Errorf("scroll = %d, want 0: el contenido nuevo cabe entero", m.scroll)
	}
}

// TestActiveEmptyShowsEmptyAndZero cubre la sección activa vacía sin fallo.
func TestActiveEmptyShowsEmptyAndZero(t *testing.T) {
	m := newTestModel(t, ghAdapter())
	view := stripANSI(m.View().Content)
	if !strings.Contains(view, "(empty)") {
		t.Errorf("la activa vacía debería marcar (empty):\n%s", view)
	}
	if !strings.Contains(view, "Assigned (0)") {
		t.Errorf("la leyenda debería mostrar 0 en la activa:\n%s", view)
	}
}

// TestLoadingMoreOnlyOnActiveSection cubre que el indicador "loading more…" es
// de la activa y reaparece al volver a la sección que pagina.
func TestLoadingMoreOnlyOnActiveSection(t *testing.T) {
	m := newTestModel(t, ghAdapter())
	m = send(t, m, page(1, "github", "github.com", model.SectionAuthored, "", []model.Item{
		mkItem("github", "github.com", "acme/mine", "Mine", 1, ""),
	}, true))
	m = send(t, m, page(1, "github", "github.com", model.SectionReview, model.ReviewRequested, []model.Item{
		mkItem("github", "github.com", "acme/assigned", "Assigned", 2, ""),
	}, false))

	if view := stripANSI(m.View().Content); strings.Contains(view, "loading more…") {
		t.Errorf("el indicador de una sección no activa no debe pintarse:\n%s", view)
	}

	// Assigned → Mentioned → Mine: la que pagina pasa a ser la activa.
	m = press(t, m, "tab")
	m = press(t, m, "tab")
	if m.activeSection != model.SectionAuthored {
		t.Fatalf("activeSection = %q, want %q", m.activeSection, model.SectionAuthored)
	}
	if view := stripANSI(m.View().Content); !strings.Contains(view, "loading more…") {
		t.Errorf("al volver a la sección que pagina, el indicador reaparece:\n%s", view)
	}
}

// TestWarningsOnlyOnActiveSection cubre que los avisos de consulta son de la
// activa: los de otra sección no se pintan y su conteo sigue en la leyenda.
func TestWarningsOnlyOnActiveSection(t *testing.T) {
	m := newTestModel(t, ghAdapter())
	m = send(t, m, pageMsg{cycle: 1, key: streamKey{forge: "github", section: model.SectionAuthored}, warnings: []model.Warning{
		{Forge: "github", Section: model.SectionAuthored, Kind: "network", Msg: "boom"},
	}})
	m = send(t, m, page(1, "github", "github.com", model.SectionReview, model.ReviewRequested, []model.Item{
		mkItem("github", "github.com", "acme/assigned", "Assigned", 2, ""),
	}, false))

	view := stripANSI(m.View().Content)
	if strings.Contains(view, "could not be queried") {
		t.Errorf("el aviso de una sección no activa no debe pintarse:\n%s", view)
	}
	if !strings.Contains(view, "Mine (0)") {
		t.Errorf("la leyenda debería seguir contando la sección con aviso:\n%s", view)
	}

	// Assigned → Mentioned → Mine: al tabular a ella, su aviso aparece.
	m = press(t, m, "tab")
	m = press(t, m, "tab")
	if view := stripANSI(m.View().Content); !strings.Contains(view, "could not be queried") {
		t.Errorf("al tabular a la sección con aviso, debería pintarse:\n%s", view)
	}
}

// TestNarrowTerminalTruncatesLegendKeepsBorderWidth cubre que en un terminal
// estrecho la leyenda se recorta sin descuadrar la caja.
func TestNarrowTerminalTruncatesLegendKeepsBorderWidth(t *testing.T) {
	m := newTestModel(t, ghAdapter())
	m.width, m.height = 34, 20
	m = send(t, m, page(1, "github", "github.com", model.SectionAuthored, "", numberedItems(9, "mine/repo"), false))
	m = send(t, m, page(1, "github", "github.com", model.SectionReview, model.ReviewRequested, numberedItems(4, "assigned/repo"), false))

	lines := visibleLines(m.View().Content)
	for i, l := range lines {
		if w := ansi.StringWidth(l); w != m.width {
			t.Errorf("línea %d mide %d columnas, want %d:\n%q", i, w, m.width, l)
		}
	}
	// La leyenda completa no cabe: se recorta por la derecha.
	top := ""
	for _, l := range lines {
		if strings.HasPrefix(l, "╭") && strings.Contains(l, "Mine (") {
			top = l
		}
	}
	if top == "" {
		t.Fatalf("no se encontró la línea de la leyenda:\n%s", strings.Join(lines, "\n"))
	}
	if strings.Contains(top, "Mentioned (0)") {
		t.Errorf("a 34 columnas la leyenda debería recortarse:\n%q", top)
	}
}
