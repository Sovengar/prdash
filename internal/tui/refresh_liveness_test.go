// Tests de liveness del auto-refresco: una paginación que se cuelga al cerrar
// el ciclo no puede congelar la cadena de ticks. Recoge el fallo detectado en
// `sendEvent`/`streamForge` (eventos críticos descartables) y en `beginRefresh`
// (no reinicia `more`).
package tui

import (
	"testing"

	"prdash/internal/forge/model"
)

// TestAutoRefreshSurvivesStalePagination: si la página que cerraba la
// paginación se descarta, `more` queda colgado al terminar el ciclo; el tick
// no debe quedar bloqueado por ese residuo (antes: paused() == true para
// siempre y el inbox no volvía a refrescar hasta pulsar "r").
func TestAutoRefreshSurvivesStalePagination(t *testing.T) {
	m := newTestModel(t, ghAdapter())
	m = send(t, m, page(m.cycle, "github", "github.com", model.SectionAuthored, "",
		[]model.Item{mkItem("github", "github.com", "acme/widget", "Uno", 1, "")}, true))

	// El ciclo termina con `more` aún pendiente: la página que lo cerraba no
	// llegó a aplicarse.
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

// TestBeginRefreshResetsStalePagination: arrancar un ciclo nuevo no debe
// arrastrar la paginación del ciclo anterior (`more`/`complete` frescos), o el
// indicador "cargando más…" quedaría pegado indefinidamente.
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
