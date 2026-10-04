package inbox

import (
	"testing"

	"prdash/internal/forge/model"
)

// Estas dos funciones son las que consulta la TUI para decidir qué pintar, y las dos tienen una
// respuesta para "no hay nada" que no es un valor cero:
//
//   - `Section` devuelve una sección VACÍA con la clase pedida puesta, no la sección cero.
//   - `Empty` devuelve un bool, no una colección.
//
// Y las dos se probaron solo por el camino bueno —la sección que existe y el inbox
// vacío—, que es donde el resultado es el mismo que el de la guarda. Un test que comprobara
// "preguntó por review y hay review" no distinguiría un `return Section{Kind: kind}` de un
// `return Section{}`.
//
// Y la diferencia importa en el render. `Section` con la clase puesta es lo que permite que
// el encabezado de una columna que no tiene ítems siga diciendo "Review / assigned" en vez de
// desaparecer: la columna vacía es información —dice que se consultó y no hay nada—, y una
// columna sin nombre es un hueco que el usuario no sabe interpretar.

// TestPedirUnaSeccionQueNoEstaLaDevuelveVaciaPeroConSuClase: la guarda de `Section`.
//
// Y el caso que hay que mirar es el de la clase DESCONOCIDA, que no es lo mismo que la clase
// conocida pero vacía. Una clase que no existe no debería aparecer en ninguna columna, y sin
// embargo `Section` la devuelve con su nombre: el nombre es lo que permite a la TUI decir
// "no hay nada en esto" en vez de no decir nada.
func TestPedirUnaSeccionQueNoEstaLaDevuelveVaciaPeroConSuClase(t *testing.T) {
	in := Inbox{Sections: []Section{
		{Kind: model.SectionAuthored},
		{Kind: model.SectionReview},
	}}

	// Una clase presente y vacía: se devuelve tal cual, con sus ítems vacíos.
	review := in.Section(model.SectionReview)
	if review.Kind != model.SectionReview {
		t.Errorf("Kind = %q, want %q", review.Kind, model.SectionReview)
	}
	if len(review.Items) != 0 {
		t.Errorf("la sección de review trae %d ítems, y el fixture no pone ninguno", len(review.Items))
	}

	// Una clase ausente: vacía PERO con su nombre. Este es el caso que la tabla de "secciones
	// que existen" no cubre, y es el que hace que el encabezado siga siendo legible.
	ausente := in.Section("una-seccion-inventada")
	if ausente.Kind != "una-seccion-inventada" {
		t.Errorf("la sección ausente volvió con Kind %q: se perdería el nombre de la columna", ausente.Kind)
	}
	if len(ausente.Items) != 0 {
		t.Errorf("la sección ausente trae %d ítems", len(ausente.Items))
	}

	// Y la devuelta no está conectada al inbox: modificarla no lo cambia. Sin esto, un
	// `append` sobre los ítems de la sección devuelta cuando el inbox la tenía vacía
	// escribiría en una copia y el cambio se perdería —o, peor, en un slice compartido.
	ausente.Items = append(ausente.Items, model.Item{Number: 99})
	if len(in.Section("una-seccion-inventada").Items) != 0 {
		t.Error("modificar la sección devuelta cambió el inbox")
	}

	// Y con un inbox sin secciones: la misma respuesta. Un inbox recién construido no tiene
	// ninguna, y preguntar por review en ese punto es lo que hace la TUI en el primer render
	// antes de que llegue el primer refresco.
	vacio := Inbox{}
	if s := vacio.Section(model.SectionReview); s.Kind != model.SectionReview || len(s.Items) != 0 {
		t.Errorf("un inbox sin secciones devolvió %+v", s)
	}
}

// TestEmptyEsFalsoEnCuantoHayUnItemEnCualquierSeccion: la guarda de `Empty`.
//
// Y el recorrido es sobre SECCIONES y no sobre ítems, que es la diferencia real: lo que
// importa es que un ítem en la última sección —la que nadie mira— ya hace que el inbox no esté
// vacío. Un `Empty` que mirara solo la primera sección diría que el inbox está vacío con
// veinte PRs authored y ninguna mención, que es la forma exacta de que la TUI muestre un
// "no hay nada" con veinte líneas debajo.
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

// TestEmptyYCuentanYTienenQueCoincidir: la consistencia entre las dos.
//
// Y no es una comprobación de补课: son las dos funciones que la TUI usa para decidir si pinta
// un "no hay nada" o las filas, y si una dice que hay y la otra que no, el render muestra
// mensaje y lista a la vez. El caso que hace visible el desacuerdo es un ítem en una sección
// que no se está pintando —`Section` la devuelve vacía y `Empty` dice que no está vacío—,
// que es el estado normal durante un refresco parcial.
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
	// Y la sección visible tiene los dos: es la coherencia que se está comprobando, la de
	// que el total y lo que se pinta salen del mismo sitio.
	if len(in.Section(model.SectionReview).Items) != total {
		t.Errorf("la sección visible tiene %d ítems y el total es %d",
			len(in.Section(model.SectionReview).Items), total)
	}
}
