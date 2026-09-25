package tui

import (
	"strings"
	"testing"
	"unicode/utf8"

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
	out := stripANSI(renderCells(cells, newRefLayout(nil), 100))
	if !strings.HasPrefix(out, "ab   x  ") {
		t.Errorf("renderCells = %q", out)
	}
}

// TestForgeBadgeCoversStandardAndSelfHosted fija la etiqueta de la columna
// FORGE: abreviatura sola en el host estándar, abreviatura + primera etiqueta
// del host en uno self-hosted, sin perder nunca la identidad en el detalle.
func TestForgeBadgeCoversStandardAndSelfHosted(t *testing.T) {
	badge := func(forge, host string) string {
		it := model.NewItem(model.RepoRef{Forge: forge, Host: host, Project: "o/r"}, 1)
		return forgeBadge(it)
	}
	for _, tc := range []struct {
		forge, host, want string
	}{
		{"github", "github.com", "GH"},
		{"gitlab", "gitlab.com", "GLab"},
		{"bitbucket", "bitbucket.org", "BB"},
		{"gitlab", "umane.emeal.nttdata.com", "GLab@umane"},
		{"github", "github.corp.example.com", "GH@github"},
		{"gitlab", "gitserver", "GLab@gitserver"},
		{"github", "", "GH"},
		{"gerrit", "gerrit.example.com", "gerrit@gerrit"},
	} {
		if got := badge(tc.forge, tc.host); got != tc.want {
			t.Errorf("forgeBadge(%q, %q) = %q, want %q", tc.forge, tc.host, got, tc.want)
		}
	}

	// El detalle sigue mostrando la ruta completa.
	it := model.NewItem(model.RepoRef{Forge: "gitlab", Host: "umane.emeal.nttdata.com", Project: "g/p"}, 7)
	if got := forgeLabel(it); got != "gitlab@umane.emeal.nttdata.com" {
		t.Errorf("forgeLabel = %q", got)
	}
}

// TestForgeBadgeFitsColumn evita que un host self-hosted largo desalinee la
// tabla: la celda se recorta al ancho de columna.
func TestForgeBadgeFitsColumn(t *testing.T) {
	it := model.NewItem(model.RepoRef{Forge: "gitlab", Host: "empresaurbanisimaziyota.example.com", Project: "g/p"}, 1)
	if got := truncate(forgeBadge(it), colForge); utf8.RuneCountInString(got) != colForge {
		t.Errorf("celda forge = %q (%d runes), want %d", got, utf8.RuneCountInString(got), colForge)
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
