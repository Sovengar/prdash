package forge

import (
	"testing"

	"prdash/internal/forge/model"
)

// TestElSelladoNoPisaLaSeccionQueYaLaTiene: un warning que ya trae sección se queda con
// la suya, y solo se sella el que no la trae.
//
// Y la razón de que la comprobación exista es que los warnings viajan por la cadena sin
// saber de dónde vienen. Uno lo pone el adapter al parsear y sabe si es de list, de item o
// de comentarios; otro lo pone el TUI, que ya sabe en qué sección está mirando. Sellar los
// dos por igual significa que un warning de comentarios acaba con la sección de list pegada
// encima, y el usuario ve el aviso junto a una sección donde no ocurrió.
//
// Y el caso que distingue es el de los dos: un warning CON sección y otro SIN ella en la
// misma llamada. Si solo se probara el que no la tiene, la condición podría ir al revés y
// nada lo notaría —porque sellar uno sin sección da el mismo resultado con las dos
// condiciones—.
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

	// Y la mezcla que es el caso de verdad: sellar una lista donde uno ya trae sección y
	// otro no. Solo el que no la trae cambia, y el otro se queda como estaba.
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

	// Y una lista vacía no inventa nada, que es el caso de un adapter que no avisó.
	if got := StampSection(nil, listado); len(got) != 0 {
		t.Errorf("una lista vacía devolvió %d warnings", len(got))
	}
}
