// Tests del modelo TUI con adapters falsos: se construye el Model, se le
// envían mensajes y se inspecciona el estado y la vista, sin teatest y sin
// tocar ninguna CLI.
package tui

import (
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"prdash/internal/config"
	"prdash/internal/forge"
	"prdash/internal/forge/model"
	"prdash/internal/inbox"
	"prdash/internal/testutil"
)

func newTestModel(adapters ...forge.Adapter) Model {
	m := New(config.Defaults(), adapters)
	m.width, m.height = 140, 40
	m.loading = false
	return m
}

func mkItem(forge, host, project, title string, number int, decision string) model.Item {
	it := model.NewItem(model.RepoRef{Forge: forge, Host: host, Project: project, Owner: "acme", Name: "widget"}, number)
	it.Title = title
	it.ReviewDecision = decision
	it.State = "OPEN"
	it.UpdatedAt = time.Date(2026, 9, 24, 10, 0, 0, 0, time.UTC)
	return it
}

func applyResults(t *testing.T, m Model, results ...inbox.ForgeResult) Model {
	t.Helper()
	for _, r := range results {
		out, _ := m.Update(forgeResultMsg{result: r})
		m = out.(Model)
	}
	return m
}

func press(m Model, key string) Model {
	km := tea.KeyPressMsg{Code: []rune(key)[0], Text: key}
	switch key {
	case "tab":
		km = tea.KeyPressMsg{Code: tea.KeyTab}
	case "up":
		km = tea.KeyPressMsg{Code: tea.KeyUp}
	case "down":
		km = tea.KeyPressMsg{Code: tea.KeyDown}
	}
	out, _ := m.Update(km)
	return out.(Model)
}

// TestViewShowsThreeSectionsWithBothForges cubre el escenario "el inbox
// muestra las tres secciones con datos de ambos forges".
func TestViewShowsThreeSectionsWithBothForges(t *testing.T) {
	gh := &testutil.FakeAdapter{ForgeName: "github", HostName: "github.com"}
	gl := &testutil.FakeAdapter{ForgeName: "gitlab", HostName: "gitlab.example.com"}
	m := newTestModel(gh, gl)

	reviewReq := mkItem("github", "github.com", "acme/lib", "Review me", 2, "REVIEW_REQUIRED")
	reviewReq.ReviewKind = model.ReviewRequested
	assigned := mkItem("gitlab", "gitlab.example.com", "grp/proj", "Asignado", 4, "REVIEW_REQUIRED")
	assigned.ReviewKind = model.ReviewAssigned

	m = applyResults(t, m,
		inbox.ForgeResult{
			Forge:    "github",
			Host:     "github.com",
			Authored: []model.Item{mkItem("github", "github.com", "acme/widget", "Add widget", 1, "APPROVED")},
			Review:   []model.Item{reviewReq},
		},
		inbox.ForgeResult{
			Forge:    "gitlab",
			Host:     "gitlab.example.com",
			Authored: []model.Item{mkItem("gitlab", "gitlab.example.com", "grp/proj", "MR propio", 3, "APPROVED")},
			Review:   []model.Item{assigned},
			Mentions: []model.Item{mkItem("gitlab", "gitlab.example.com", "grp/proj", "Mención", 5, "")},
		},
	)

	view := stripANSI(m.View().Content)
	for _, want := range []string{
		"Creados por mí", "Review / asignados", "Menciones",
		"github@github.com", "gitlab@gitlab.example.com",
		"Add widget", "Review me", "MR propio", "Mención",
		"review req", "assigned",
	} {
		if !strings.Contains(view, want) {
			t.Errorf("la vista no contiene %q\n%s", want, view)
		}
	}
}

// TestSectionEmptyVsError cubre el escenario "distinguir sección vacía de no se
// pudo consultar": nunca se muestra "vacío" cuando hubo error.
func TestSectionEmptyVsError(t *testing.T) {
	gh := &testutil.FakeAdapter{ForgeName: "github", HostName: "github.com"}
	m := newTestModel(gh)
	m = applyResults(t, m, inbox.ForgeResult{
		Forge: "github",
		Host:  "github.com",
		Warnings: []model.Warning{
			{Forge: "github", Section: model.SectionReview, Kind: "network", Msg: "boom"},
		},
	})

	view := stripANSI(m.View().Content)
	if !strings.Contains(view, "no se pudo consultar") {
		t.Errorf("la sección fallida debería decirlo\n%s", view)
	}
	// authored y mentions están vacías de verdad: dos veces "(vacío)".
	if got := strings.Count(view, "(vacío)"); got != 2 {
		t.Errorf("(vacío) aparece %d veces, want 2 (review no debe decir vacío)\n%s", got, view)
	}
}

// TestDegradationKeepsOtherForges cubre el escenario "una forge caída o sin
// auth no vacía el inbox".
func TestDegradationKeepsOtherForges(t *testing.T) {
	gh := &testutil.FakeAdapter{ForgeName: "github", HostName: "github.com"}
	gl := &testutil.FakeAdapter{ForgeName: "gitlab", HostName: "gitlab.example.com"}
	m := newTestModel(gh, gl)

	m = applyResults(t, m,
		inbox.ForgeResult{
			Forge:    "github",
			Host:     "github.com",
			Authored: []model.Item{mkItem("github", "github.com", "acme/widget", "Sigue visible", 1, "")},
		},
		inbox.ForgeResult{
			Forge: "gitlab",
			Host:  "gitlab.example.com",
			Warnings: []model.Warning{
				{Forge: "gitlab", Kind: "auth", Msg: "401"},
			},
		},
	)

	view := stripANSI(m.View().Content)
	if !strings.Contains(view, "Sigue visible") {
		t.Errorf("los ítems de GitHub deberían seguir visibles\n%s", view)
	}
	if !strings.Contains(view, "gitlab ✗") {
		t.Errorf("gitlab debería reportarse caído\n%s", view)
	}
}

// TestAuthoredOnlyShowsOwnItems comprueba que la sección authored solo contiene
// lo que abrí yo (los ítems de review no se cuelan).
func TestAuthoredOnlyShowsOwnItems(t *testing.T) {
	gh := &testutil.FakeAdapter{ForgeName: "github", HostName: "github.com"}
	m := newTestModel(gh)
	m = applyResults(t, m, inbox.ForgeResult{
		Forge:    "github",
		Host:     "github.com",
		Authored: []model.Item{mkItem("github", "github.com", "acme/widget", "Mío", 1, "")},
		Review:   []model.Item{mkItem("github", "github.com", "acme/lib", "Ajeno", 2, "")},
	})

	authored := m.sectionItems(model.SectionAuthored)
	if len(authored) != 1 || authored[0].Title != "Mío" {
		t.Fatalf("authored = %+v", authored)
	}
	view := stripANSI(m.View().Content)
	if idxMio, idxAjeno := strings.Index(view, "Mío"), strings.Index(view, "Ajeno"); idxAjeno < idxMio {
		t.Errorf("los ítems ajenos no deberían ir antes que los míos")
	}
}

// TestNavigationMovesCursor comprueba la navegación entre filas y secciones.
func TestNavigationMovesCursor(t *testing.T) {
	gh := &testutil.FakeAdapter{ForgeName: "github", HostName: "github.com"}
	m := newTestModel(gh)
	m = applyResults(t, m, inbox.ForgeResult{
		Forge: "github",
		Host:  "github.com",
		Authored: []model.Item{
			mkItem("github", "github.com", "acme/widget", "A", 1, ""),
			mkItem("github", "github.com", "acme/widget", "B", 2, ""),
		},
		Review: []model.Item{mkItem("github", "github.com", "acme/widget", "C", 3, "")},
	})

	if m.cursor != 0 {
		t.Fatalf("cursor inicial = %d", m.cursor)
	}
	m = press(m, "down")
	if m.cursor != 1 {
		t.Fatalf("cursor tras down = %d", m.cursor)
	}
	m = press(m, "tab")
	if m.cursor != 2 {
		t.Fatalf("cursor tras tab = %d, want 2 (primer ítem de review)", m.cursor)
	}
	if it, ok := m.selected(); !ok || it.Title != "C" {
		t.Fatalf("seleccionado = %+v", it)
	}
}

func TestInitReturnsCmd(t *testing.T) {
	gh := &testutil.FakeAdapter{ForgeName: "github", HostName: "github.com"}
	m := newTestModel(gh)
	if m.Init() == nil {
		t.Fatal("Init debería devolver un Cmd")
	}
}

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
