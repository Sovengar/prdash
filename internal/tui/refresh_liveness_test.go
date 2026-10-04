package tui

import (
	"testing"

	"prdash/internal/forge/model"
)

// If the page that closed the pagination is discarded, the `more` flag survives the end of the
// cycle as residue, and consulting it would freeze the tick for good.
func TestAutoRefreshSurvivesStalePagination(t *testing.T) {
	m := newTestModel(t, ghAdapter())
	m = send(t, m, page(m.cycle, "github", "github.com", model.SectionAuthored, "",
		[]model.Item{mkItem("github", "github.com", "acme/widget", "Uno", 1, "")}, true))

	// The cycle ends with `more` still pending: the page that closed it never arrived.
	m = send(t, m, refreshDoneMsg{cycle: m.cycle})
	if m.loading {
		t.Fatal("el ciclo debería haber terminado")
	}
	if !m.sectionLoadingMore(model.SectionAuthored) {
		t.Fatal("precondición: se esperaba un `more` colgado tras el ciclo")
	}

	before := m.cycle
	m = send(t, m, tickMsg{})
	if m.cycle == before {
		t.Fatal("el tick quedó bloqueado por una paginación colgada")
	}
}

func TestBeginRefreshResetsStalePagination(t *testing.T) {
	m := newTestModel(t, ghAdapter())
	m = send(t, m, page(m.cycle, "github", "github.com", model.SectionAuthored, "",
		[]model.Item{mkItem("github", "github.com", "acme/widget", "Uno", 1, "")}, true))
	if !m.sectionLoadingMore(model.SectionAuthored) {
		t.Fatal("precondición: debería haber paginación pendiente")
	}

	updated, _ := m.beginRefresh()
	if updated.sectionLoadingMore(model.SectionAuthored) {
		t.Fatal("un ciclo nuevo no debe arrastrar la paginación del anterior")
	}
}
