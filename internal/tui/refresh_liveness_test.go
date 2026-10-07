package tui

import (
	"testing"

	"prdash/internal/forge/model"
)

// If the page that closed the pagination is discarded, the `more` flag survives as residue and would freeze the tick.
func TestAutoRefreshSurvivesStalePagination(t *testing.T) {
	m := newTestModel(t, ghAdapter())
	m = send(t, m, page(m.cycle, "github", "github.com", model.SectionAuthored, "",
		[]model.Item{mkItem("github", "github.com", "acme/widget", "One", 1, "")}, true))

	m = send(t, m, refreshDoneMsg{cycle: m.cycle})
	if m.loading {
		t.Fatal("the cycle should have finished")
	}
	if !m.sectionLoadingMore(model.SectionAuthored) {
		t.Fatal("precondition: a hung `more` was expected after the cycle")
	}

	before := m.cycle
	m = send(t, m, tickMsg{})
	if m.cycle == before {
		t.Fatal("the tick was blocked by a hung pagination")
	}
}

func TestBeginRefreshResetsStalePagination(t *testing.T) {
	m := newTestModel(t, ghAdapter())
	m = send(t, m, page(m.cycle, "github", "github.com", model.SectionAuthored, "",
		[]model.Item{mkItem("github", "github.com", "acme/widget", "One", 1, "")}, true))
	if !m.sectionLoadingMore(model.SectionAuthored) {
		t.Fatal("precondition: there should be pending pagination")
	}

	updated, _ := m.beginRefresh()
	if updated.sectionLoadingMore(model.SectionAuthored) {
		t.Fatal("a new cycle must not drag the previous cycle pagination")
	}
}
