package inbox

import (
	"testing"

	"prdash/internal/forge/model"
)

// The two functions the TUI asks what to paint, and both have a "nothing here" answer that is
//not a value but a state.

// The case to watch is the UNKNOWN section's class.
func TestPedirUnaSeccionQueNoEstaLaDevuelveVaciaPeroConSuClase(t *testing.T) {
	in := Inbox{Sections: []Section{
		{Kind: model.SectionAuthored},
		{Kind: model.SectionReview},
	}}

	review := in.Section(model.SectionReview)
	if review.Kind != model.SectionReview {
		t.Errorf("Kind = %q, want %q", review.Kind, model.SectionReview)
	}
	if len(review.Items) != 0 {
		t.Errorf("la sección de review trae %d ítems, y el fixture no pone ninguno", len(review.Items))
	}

	// An absent class: empty BUT with its name, which is the case the "sections" table needs.
	ausente := in.Section("una-seccion-inventada")
	if ausente.Kind != "una-seccion-inventada" {
		t.Errorf("la sección ausente volvió con Kind %q: se perdería el nombre de la columna", ausente.Kind)
	}
	if len(ausente.Items) != 0 {
		t.Errorf("la sección ausente trae %d ítems", len(ausente.Items))
	}

	// The returned section is not attached to the inbox: changing it does not change the inbox.
	ausente.Items = append(ausente.Items, model.Item{Number: 99})
	if len(in.Section("una-seccion-inventada").Items) != 0 {
		t.Error("modificar la sección devuelta cambió el inbox")
	}

	// And with an inbox with no sections: the same answer.
	vacio := Inbox{}
	if s := vacio.Section(model.SectionReview); s.Kind != model.SectionReview || len(s.Items) != 0 {
		t.Errorf("un inbox sin secciones devolvió %+v", s)
	}
}

// The walk is over SECTIONS and not over items.
func TestEmptyEsFalsoEnCuantoHayUnItemEnCualquierSeccion(t *testing.T) {
	for _, c := range []struct {
		nombre    string
		in        Inbox
		wantEmpty bool
	}{
		{"inbox recién construido", Inbox{}, true},
		{"secciones sin ítems", Inbox{Sections: []Section{
			{Kind: model.SectionReview}, {Kind: model.SectionMentions},
		}}, true},
		{"un item en la primera sección", Inbox{Sections: []Section{
			{Kind: model.SectionReview, Items: []model.Item{{Number: 1}}},
			{Kind: model.SectionMentions},
		}}, false},
		{"un item SOLO en la última sección", Inbox{Sections: []Section{
			{Kind: model.SectionReview},
			{Kind: model.SectionAuthored},
			{Kind: model.SectionMentions, Items: []model.Item{{Number: 7}}},
		}}, false},
		{"una entrada nil", Inbox{Sections: []Section{
			{Kind: model.SectionReview, Items: []model.Item{}},
		}}, true},
	} {
		if got := c.in.Empty(); got != c.wantEmpty {
			t.Errorf("%s: Empty() = %v, want %v", c.nombre, got, c.wantEmpty)
		}
	}
}

// Not a make-up check: they are the two functions the TUI uses to decide what to paint.
func TestEmptyYCuentanYTienenQueCoincidir(t *testing.T) {
	in := Inbox{Sections: []Section{
		{Kind: model.SectionReview, Items: []model.Item{{Number: 1}, {Number: 2}}},
		{Kind: model.SectionAuthored},
	}}

	total := 0
	for _, s := range in.Sections {
		total += len(s.Items)
	}
	if total != 2 {
		t.Fatalf("el fixture tiene %d ítems, no 2", total)
	}
	if in.Empty() {
		t.Error("un inbox con dos ítems dijo que estaba vacío")
	}
	if len(in.Section(model.SectionReview).Items) != total {
		t.Errorf("la sección visible tiene %d ítems y el total es %d",
			len(in.Section(model.SectionReview).Items), total)
	}
}
