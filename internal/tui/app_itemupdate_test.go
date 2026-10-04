package tui

import (
	"testing"
	"time"

	"prdash/internal/forge/model"
)

// Without it, merging a PR would leave it looking unmerged until the next refresh.

// The failure mergeItem exists for.
func TestUnItemQueNoEstaSeAnadeYNoSePierdeLaSeccion(t *testing.T) {
	m := newTestModel(t)
	original := model.Item{
		Section: model.SectionReview, ReviewKind: model.ReviewRequested,
		Forge: "github", Host: "github.com", Number: 7,
		Ref: model.RepoRef{Project: "o/r"}, Title: "antes", UpdatedAt: time.Now(),
	}
	conItems(t, &m, original)
	m.rebuild()

	// The re-read: the forge answers the state but does not know which section the item came from.
	releida := original
	releida.Section = ""
	releida.ReviewKind = ""
	releida.Title = "después"
	m.applyItemUpdate(releida)

	it, ok := findItem(todosLosItems(m), original.ID())
	if !ok {
		t.Fatalf("el item desaparecio tras actualizarlo: %v", todosLosItems(m))
	}
	if it.Title != "después" {
		t.Errorf("el titulo no se actualizó: %q", it.Title)
	}
	if it.Section != model.SectionReview {
		t.Errorf("la seccion se perdio: %q", it.Section)
	}
	if it.ReviewKind != model.ReviewRequested {
		t.Errorf("el tipo de review se perdio: %q", it.ReviewKind)
	}
	// And it stays in the stream it was in, because the user had it in front of them.
	conItems := 0
	for _, s := range m.streams {
		conItems += len(s.items)
	}
	if conItems != 1 {
		t.Errorf("hay %d items en los streams tras actualizar, want 1", conItems)
	}
}

// The other side of that.
func TestUnItemQueNoEstaSeAnadeAlStreamQueToca(t *testing.T) {
	m := newTestModel(t)
	m.rebuild()

	nuevo := model.Item{
		Section: model.SectionMentions, ReviewKind: "",
		Forge: "gitlab", Host: "gitlab.com", Number: 3,
		Ref: model.RepoRef{Project: "o/r"}, Title: "nuevo",
	}
	m.applyItemUpdate(nuevo)

	it, ok := findItem(todosLosItems(m), nuevo.ID())
	if !ok {
		t.Fatalf("el item anadido no aparece: %v", todosLosItems(m))
	}
	if it.Title != "nuevo" || it.Forge != "gitlab" {
		t.Errorf("el item anadido no llego entero: %+v", it)
	}
	enReview := false
	for k, s := range m.streams {
		for _, si := range s.items {
			if si.ID() == nuevo.ID() && k.section != model.SectionMentions {
				enReview = true
			}
		}
	}
	if enReview {
		t.Error("el item se metio en un stream que no es el suyo")
	}

	otro := nuevo
	otro.Forge = "github"
	otro.Host = "github.com"
	m.applyItemUpdate(otro)
	total := 0
	for _, s := range m.streams {
		total += len(s.items)
	}
	if total != 2 {
		t.Errorf("dos forges con el mismo numero acabaron con %d items, want 2", total)
	}
}

// The three things an update must not do.
func TestActualizarNoDuplicaNiBorraLoDemas(t *testing.T) {
	m := newTestModel(t)
	base := model.Item{
		Section: model.SectionReview, ReviewKind: model.ReviewRequested,
		Forge: "github", Host: "github.com", Number: 1,
		Ref: model.RepoRef{Project: "o/r"}, Title: "uno",
	}
	dos := base
	dos.Number = 2
	dos.Title = "dos"
	tres := base
	tres.Number = 3
	tres.Title = "tres"
	conItems(t, &m, base, dos, tres)
	m.rebuild()
	antes := len(todosLosItems(m))

	actualizada := base
	actualizada.Title = "uno cambiado"
	actualizada.State = "MERGED"
	m.applyItemUpdate(actualizada)

	if len(todosLosItems(m)) != antes {
		t.Errorf("tras actualizar hay %d items y habia %d", len(todosLosItems(m)), antes)
	}
	vistos := 0
	for _, it := range todosLosItems(m) {
		if it.ID() == base.ID() {
			vistos++
			if it.Title != "uno cambiado" || it.State != "MERGED" {
				t.Errorf("el item no quedo actualizado: %+v", it)
			}
		}
	}
	if vistos != 1 {
		t.Errorf("el item aparece %d veces tras actualizarlo", vistos)
	}
	titulos := map[string]bool{}
	for _, it := range todosLosItems(m) {
		titulos[it.Title] = true
	}
	for _, quiere := range []string{"uno cambiado", "dos", "tres"} {
		if !titulos[quiere] {
			t.Errorf("tras actualizar falta %q; quedan %v", quiere, titulos)
		}
	}
}

// Both halves, and the default matters as much as the cases.
func TestMergeItemConservaLoQueElForgeNoSabeYCambiaLoQueSi(t *testing.T) {
	viejo := model.Item{Section: model.SectionReview, ReviewKind: model.ReviewRequested, Title: "viejo"}

	fresco := model.Item{Title: "fresco"}
	merged := mergeItem(viejo, fresco)
	if merged.Section != model.SectionReview || merged.ReviewKind != model.ReviewRequested {
		t.Errorf("no conservo lo que el releido no traia: %+v", merged)
	}
	if merged.Title != "fresco" {
		t.Errorf("el titulo no es el del releido: %q", merged.Title)
	}

	// With a section set: the re-read's wins, and that is the retarget.
	fresco = model.Item{Title: "fresco", Section: model.SectionAuthored, ReviewKind: ""}
	merged = mergeItem(viejo, fresco)
	if merged.Section != model.SectionAuthored {
		t.Errorf("con seccion propia se puso la vieja: %q", merged.Section)
	}
	if merged.ReviewKind != model.ReviewRequested {
		t.Errorf("conservar la seccion impidio conservar el kind: %q", merged.ReviewKind)
	}

	fresco = model.Item{Title: "fresco", ReviewKind: model.ReviewAssigned}
	merged = mergeItem(viejo, fresco)
	if merged.Section != model.SectionReview || merged.ReviewKind != model.ReviewAssigned {
		t.Errorf("no mezclo bien: %+v", merged)
	}

	merged = mergeItem(model.Item{}, model.Item{Title: "fresco"})
	if merged.Section != "" || merged.ReviewKind != "" {
		t.Errorf("con un viejo vacio se invento algo: %+v", merged)
	}
}

// The difference between "no" and "not known".
func TestSiNoEnElDetalleNoEsUnGuion(t *testing.T) {
	if got := yesNo(true); got != "yes" {
		t.Errorf("yesNo(true) dio %q", got)
	}
	if got := yesNo(false); got != "no" {
		t.Errorf("yesNo(false) dio %q, want \"no\": un false es respuesta, no ausencia", got)
	}
	// The difference with orDash is real: a false does NOT print as a dash.
	if yesNo(false) == orDash("") {
		t.Error("yesNo(false) sale igual que orDash de una cadena vacia: son cosas distintas")
	}
	if got := orDash(""); got != "-" {
		t.Errorf("orDash(\"\") dio %q", got)
	}
	if got := orDash("algo"); got != "algo" {
		t.Errorf("orDash con texto dio %q", got)
	}
}

func conItems(t *testing.T, m *Model, items ...model.Item) {
	t.Helper()
	for _, it := range items {
		k := streamKey{forge: it.Forge, section: it.Section, kind: it.ReviewKind}
		s := m.streams[k]
		if s == nil {
			s = &stream{}
			m.streams[k] = s
		}
		s.items = append(s.items, it)
	}
}

func todosLosItems(m Model) []model.Item {
	var out []model.Item
	for _, s := range m.streams {
		out = append(out, s.items...)
	}
	return out
}
