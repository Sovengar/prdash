package tui

import (
	"testing"

	"prdash/internal/forge"
	"prdash/internal/forge/model"
)

func TestStalePageDoesNotRevertAction(t *testing.T) {
	open := mkItem("github", "github.com", "acme/widget", "Add widget", 1, "")
	other := mkItem("github", "github.com", "acme/widget", "Other", 2, "")
	m := newTestModel(t, ghAdapter())
	m = send(t, m, page(m.cycle, "github", "github.com", model.SectionAuthored, "", []model.Item{open, other}, false))

	merged := open
	merged.State = "MERGED"
	m = send(t, m, actionMsg{cycle: m.cycle, outcome: forge.Outcome{
		Kind: forge.ActionMerge, ID: open.ID(), OK: true, Item: merged, HasItem: true,
	}})

	otherUpdated := other
	otherUpdated.Title = "Other updated"
	m = send(t, m, page(m.cycle, "github", "github.com", model.SectionAuthored, "", []model.Item{open, otherUpdated}, false))

	byID := map[model.ID]model.Item{}
	for _, it := range m.sectionItems(model.SectionAuthored) {
		byID[it.ID()] = it
	}
	if got := byID[open.ID()].State; got != "MERGED" {
		t.Fatalf("the old page reverted the action: state=%q, want MERGED", got)
	}
	if got := byID[other.ID()].Title; got != "Other updated" {
		t.Fatalf("the rest of the items should be updated: title=%q", got)
	}
}

func TestNewerPageOverridesActionReread(t *testing.T) {
	open := mkItem("github", "github.com", "acme/widget", "Add widget", 1, "")
	m := newTestModel(t, ghAdapter())
	m = send(t, m, page(m.cycle, "github", "github.com", model.SectionAuthored, "", []model.Item{open}, false))

	merged := open
	merged.State = "MERGED"
	m = send(t, m, actionMsg{cycle: m.cycle, outcome: forge.Outcome{
		Kind: forge.ActionMerge, ID: open.ID(), OK: true, Item: merged, HasItem: true,
	}})

	m, _ = m.beginRefresh()
	fromForge := open
	fromForge.State = "CLOSED"
	m = send(t, m, page(m.cycle, "github", "github.com", model.SectionAuthored, "", []model.Item{fromForge}, false))

	items := m.sectionItems(model.SectionAuthored)
	if len(items) != 1 || items[0].State != "CLOSED" {
		t.Fatalf("a page from a later cycle must win: %+v", items)
	}
}
