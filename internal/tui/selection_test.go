// Tests de la persistencia de la selección: la TUI deja el ítem seleccionado en
// un fichero de estado para que la acción `prdash.mount-review` sin URL lo monte.
package tui

import (
	"path/filepath"
	"testing"

	"prdash/internal/forge/model"
	"prdash/internal/selection"
)

// TestSelectionPersistedOnChange comprueba que la TUI persiste el ítem
// seleccionado (identidad completa) y lo actualiza al mover el cursor.
func TestSelectionPersistedOnChange(t *testing.T) {
	path := filepath.Join(t.TempDir(), "selection.json")
	m := newTestModel(t, ghAdapter())
	m.SetSelectionPath(path)

	first := mkItem("github", "github.com", "acme/widget", "Uno", 1, "")
	second := mkItem("github", "github.com", "acme/widget", "Dos", 2, "")
	m = send(t, m, page(1, "github", "github.com", model.SectionAuthored, "", []model.Item{first, second}, false))

	rows := m.rows()
	if len(rows) != 2 {
		t.Fatalf("filas = %d, quiero 2", len(rows))
	}
	sel, ok := selection.Load(path)
	if !ok {
		t.Fatal("la selección debería persistirse al cargar la página")
	}
	if sel.Number != rows[0].Number {
		t.Fatalf("selección = %+v, quiero la fila del cursor %+v", sel, rows[0].ID())
	}

	m = press(t, m, "down")
	sel, ok = selection.Load(path)
	if !ok || sel.Number != rows[1].Number {
		t.Fatalf("la selección debería seguir al cursor: %+v ok=%v, quiero %+v", sel, ok, rows[1].ID())
	}
	if sel.Forge != "github" || sel.Host != "github.com" || sel.Project != "acme/widget" {
		t.Fatalf("identidad incompleta: %+v", sel)
	}
	if sel.URL == "" {
		t.Fatal("la URL debería persistirse")
	}
}

// TestSelectionNotPersistedWithoutPath comprueba que sin ruta configurada la TUI
// no persiste nada (tests y arranque degradado).
func TestSelectionNotPersistedWithoutPath(t *testing.T) {
	m := newTestModel(t, ghAdapter())
	if m.selectionPath != "" {
		t.Fatalf("selectionPath = %q, quiero vacío", m.selectionPath)
	}
	m = send(t, m, page(1, "github", "github.com", model.SectionAuthored, "", []model.Item{mkItem("github", "github.com", "acme/widget", "X", 1, "")}, false))
	if m.selectionID != (model.ID{}) {
		t.Fatal("sin ruta no debería registrarse selección persistida")
	}
}
