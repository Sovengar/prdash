package tui

import (
	"strings"
	"testing"

	"prdash/internal/forge/model"
)

func TestPadAndTruncate(t *testing.T) {
	if got := pad("ab", 5); got != "ab   " {
		t.Errorf("pad = %q", got)
	}
	if got := truncate("abcdef", 4); got != "abc…" {
		t.Errorf("truncate = %q", got)
	}
}

// TestRenderCellsPadsBeforeStyle verifica que el relleno va antes del estilo:
// el texto plano resultante conserva el ancho de columna.
func TestRenderCellsPadsBeforeStyle(t *testing.T) {
	cells := []cell{
		{"ab", styleForge, 5},
		{"x", styleRef, 3},
	}
	out := stripANSI(renderCells(cells, 100))
	if !strings.HasPrefix(out, "ab   x  ") {
		t.Errorf("renderCells = %q", out)
	}
}

// TestNavigationMovesCursor cubre la navegación entre filas y secciones.
func TestNavigationMovesCursor(t *testing.T) {
	m := newTestModel(t, ghAdapter())
	m = send(t, m, page(1, "github", "github.com", model.SectionAuthored, "", []model.Item{
		mkItem("github", "github.com", "acme/widget", "A", 1, ""),
		mkItem("github", "github.com", "acme/widget", "B", 2, ""),
	}, false))
	m = send(t, m, page(1, "github", "github.com", model.SectionReview, model.ReviewRequested, []model.Item{
		mkItem("github", "github.com", "acme/widget", "C", 3, ""),
	}, false))

	if m.cursor != 0 {
		t.Fatalf("cursor inicial = %d", m.cursor)
	}
	m = press(t, m, "down")
	if m.cursor != 1 {
		t.Fatalf("cursor tras down = %d", m.cursor)
	}
	m = press(t, m, "tab")
	if m.cursor != 2 {
		t.Fatalf("cursor tras tab = %d, want 2 (primer ítem de review)", m.cursor)
	}
	if it, ok := m.selected(); !ok || it.Title != "C" {
		t.Fatalf("seleccionado = %+v", it)
	}
}

// TestAuthoredOnlyShowsOwnItems comprueba que la sección authored solo contiene
// lo que abrí yo.
func TestAuthoredOnlyShowsOwnItems(t *testing.T) {
	m := newTestModel(t, ghAdapter())
	m = send(t, m, page(1, "github", "github.com", model.SectionAuthored, "", []model.Item{mkItem("github", "github.com", "acme/widget", "Mío", 1, "")}, false))
	m = send(t, m, page(1, "github", "github.com", model.SectionReview, model.ReviewRequested, []model.Item{mkItem("github", "github.com", "acme/lib", "Ajeno", 2, "")}, false))

	authored := m.sectionItems(model.SectionAuthored)
	if len(authored) != 1 || authored[0].Title != "Mío" {
		t.Fatalf("authored = %+v", authored)
	}
}

func TestInitReturnsCmd(t *testing.T) {
	m := newTestModel(t, ghAdapter())
	if m.Init() == nil {
		t.Fatal("Init debería devolver un Cmd")
	}
}
