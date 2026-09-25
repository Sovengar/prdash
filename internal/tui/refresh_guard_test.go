// Test del guard de ordenación entre páginas de refresco y resultados de
// acciones rápidas: una página capturada ANTES de una acción no puede revertir
// el estado releído del ítem accionado, pero el resto de la página sí aplica y
// una página de un ciclo posterior manda.
package tui

import (
	"testing"

	"prdash/internal/forge"
	"prdash/internal/forge/model"
)

// TestStalePageDoesNotRevertAction cubre "un refresco no pisa una acción en
// curso": la página capturada antes de la acción no debe revertir el ítem
// accionado, aunque el resto de ítems sí se actualice.
func TestStalePageDoesNotRevertAction(t *testing.T) {
	open := mkItem("github", "github.com", "acme/widget", "Add widget", 1, "")
	other := mkItem("github", "github.com", "acme/widget", "Otro", 2, "")
	m := newTestModel(t, ghAdapter())
	m = send(t, m, page(m.cycle, "github", "github.com", model.SectionAuthored, "", []model.Item{open, other}, false))

	merged := open
	merged.State = "MERGED"
	m = send(t, m, actionMsg{cycle: m.cycle, outcome: forge.Outcome{
		Kind: forge.ActionMerge, ID: open.ID(), OK: true, Item: merged, HasItem: true,
	}})

	// Página del mismo ciclo, capturada antes de la acción.
	otherUpdated := other
	otherUpdated.Title = "Otro actualizado"
	m = send(t, m, page(m.cycle, "github", "github.com", model.SectionAuthored, "", []model.Item{open, otherUpdated}, false))

	byID := map[model.ID]model.Item{}
	for _, it := range m.sectionItems(model.SectionAuthored) {
		byID[it.ID()] = it
	}
	if got := byID[open.ID()].State; got != "MERGED" {
		t.Fatalf("la página vieja revirtió la acción: state=%q, want MERGED", got)
	}
	if got := byID[other.ID()].Title; got != "Otro actualizado" {
		t.Fatalf("el resto de ítems debería actualizarse: title=%q", got)
	}
}

// TestNewerPageOverridesActionReread: el guard no congela el ítem; una página
// de un ciclo posterior a la acción vuelve a mandar.
func TestNewerPageOverridesActionReread(t *testing.T) {
	open := mkItem("github", "github.com", "acme/widget", "Add widget", 1, "")
	m := newTestModel(t, ghAdapter())
	m = send(t, m, page(m.cycle, "github", "github.com", model.SectionAuthored, "", []model.Item{open}, false))

	merged := open
	merged.State = "MERGED"
	m = send(t, m, actionMsg{cycle: m.cycle, outcome: forge.Outcome{
		Kind: forge.ActionMerge, ID: open.ID(), OK: true, Item: merged, HasItem: true,
	}})

	// Ciclo nuevo: la página refleja el estado actual del forge y manda.
	m, _ = m.beginRefresh()
	fromForge := open
	fromForge.State = "CLOSED"
	m = send(t, m, page(m.cycle, "github", "github.com", model.SectionAuthored, "", []model.Item{fromForge}, false))

	items := m.sectionItems(model.SectionAuthored)
	if len(items) != 1 || items[0].State != "CLOSED" {
		t.Fatalf("una página de un ciclo posterior debe mandar: %+v", items)
	}
}
