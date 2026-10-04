package forge

import (
	"testing"

	"prdash/internal/forge/model"
)

func TestElSelladoNoPisaLaSeccionQueYaLaTiene(t *testing.T) {
	const (
		listado    = model.SectionReview
		menciones  = model.SectionMentions
		propia     = model.SectionAuthored
		sinSeccion = model.Section("")
	)

	casos := []struct {
		nombre string
		entra  model.Section
		sellar model.Section
		want   model.Section
	}{
		{"sin sección, se sella", sinSeccion, listado, listado},
		{"con sección propia, no se toca", listado, menciones, listado},
		{"con sección de menciones, no se toca", menciones, listado, menciones},
		{"con sección de authorship, no se toca", propia, listado, propia},
	}
	for _, c := range casos {
		warns := StampSection([]model.Warning{{Msg: "algo", Section: c.entra}}, c.sellar)
		if len(warns) != 1 {
			t.Fatalf("caso %q: %d warnings, want 1", c.nombre, len(warns))
		}
		if warns[0].Section != c.want {
			t.Errorf("caso %q: la sección quedó %q, want %q", c.nombre, warns[0].Section, c.want)
		}
	}

	warns := StampSection([]model.Warning{
		{Msg: "el primero ya sabe de donde viene", Section: menciones},
		{Msg: "el segundo no"},
	}, listado)
	if len(warns) != 2 {
		t.Fatalf("quedaron %d warnings, want 2", len(warns))
	}
	if warns[0].Section != menciones {
		t.Errorf("el warning que ya traía sección quedó en %q: sellar por igual hace que "+
			"un aviso de comentarios acabe con la sección de listado pegada encima",
			warns[0].Section)
	}
	if warns[1].Section != listado {
		t.Errorf("el warning sin sección quedó en %q, want %q: este sí se sella",
			warns[1].Section, listado)
	}

	if got := StampSection(nil, listado); len(got) != 0 {
		t.Errorf("una lista vacía devolvió %d warnings", len(got))
	}
}
