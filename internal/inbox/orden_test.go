package inbox

import (
	"testing"
	"time"

	"prdash/internal/forge/model"
)

// The two survivors are the order and the authority, and both are comparisons whose boundary
//is a tie.

// The order has three tie-breaks and the last one is the number.
func TestElOrdenEsAtencionFechaYNumero(t *testing.T) {
	fecha := time.Date(2026, 3, 17, 10, 0, 0, 0, time.UTC)

	item := func(n int, score string, cuando time.Time) model.Item {
		it := model.NewItem(model.RepoRef{Forge: "github", Host: "github.com",
			Project: "acme/widget", Owner: "acme", Name: "widget"}, n)
		it.Title, it.UpdatedAt, it.ReviewDecision = "pr", cuando, score
		return it
	}
	numeros := func(items []model.Item) []int {
		out := make([]int, len(items))
		for i, it := range items {
			out[i] = it.Number
		}
		return out
	}
	igual := func(got, want []int) bool {
		if len(got) != len(want) {
			return false
		}
		for i := range got {
			if got[i] != want[i] {
				return false
			}
		}
		return true
	}

	// The THIRD tie-break, the one you can see without thinking.
	items := []model.Item{
		item(4, "approved", fecha), item(2, "approved", fecha),
		item(3, "approved", fecha), item(1, "approved", fecha),
	}
	sortItems(items)
	if got := numeros(items); !igual(got, []int{1, 2, 3, 4}) {
		t.Errorf("con atención y fecha iguales salió %v, want [1 2 3 4]. El tercer "+
			"desempate es el número: sin él el orden sería el de llegada, que depende "+
			"de qué forge respondió antes y cambia entre ejecuciones", got)
	}

	items = []model.Item{
		item(1, "approved", fecha), item(3, "approved", fecha),
		item(2, "approved", fecha), item(4, "approved", fecha),
	}
	sortItems(items)
	if got := numeros(items); !igual(got, []int{1, 2, 3, 4}) {
		t.Errorf("con atención y fecha iguales pero en otro orden de entrada salió %v, "+
			"want [1 2 3 4]", got)
	}

	items = []model.Item{
		item(1, "approved", fecha), item(2, "approved", fecha.Add(time.Hour)),
		item(3, "approved", fecha.Add(48*time.Hour)),
	}
	sortItems(items)
	if got := numeros(items); !igual(got, []int{3, 2, 1}) {
		t.Errorf("con la atención igual y fechas distintas salió %v, want [3 2 1]: manda "+
			"la actualización más reciente", got)
	}

	items = []model.Item{
		item(9, "approved", fecha), item(1, "approved", fecha.Add(time.Hour)),
	}
	sortItems(items)
	if got := numeros(items); !igual(got, []int{1, 9}) {
		t.Errorf("con el número al revés y la fecha al revés salió %v, want [1 9]", got)
	}

	// The FIRST tie-break, the one that kills a `>=`: attention beats everything else.
	items = []model.Item{
		item(1, "approved", fecha),
		item(2, "approved", fecha.Add(time.Hour)),
		item(9, "changes_requested", fecha.Add(-72*time.Hour)),
		item(3, "approved", fecha.Add(48*time.Hour)),
	}
	sortItems(items)
	if items[0].Number != 9 {
		t.Errorf("con cambios solicitados quedó primero el %d, want el 9. La atención "+
			"manda sobre la fecha y sobre el número: es el único que necesita una acción",
			items[0].Number)
	}

	fallos := item(1, "approved", fecha)
	fallos.State = "OPEN"
	fallos.Checks.State = model.ChecksFailing

	items = []model.Item{
		item(5, "", fecha),                  // pendiente
		item(4, "approved", fecha),          // aprobado
		item(3, "review_required", fecha),   // review pendiente
		item(2, "changes_requested", fecha), // cambios pedidos
		fallos,                              // checks rotos
	}
	sortItems(items)
	if got := numeros(items); !igual(got, []int{1, 2, 3, 4, 5}) {
		t.Errorf("el orden entero de estados salió %v, want [1 2 3 4 5]", got)
	}
}

// An item in several sections stays in the most authoritative one.
func TestLaSeccionConMasAutoridadSeQuedaConElItem(t *testing.T) {
	nuevo := func(n int) model.Item {
		it := model.NewItem(model.RepoRef{Forge: "github", Host: "github.com",
			Project: "acme/widget", Owner: "acme", Name: "widget"}, n)
		it.Title = "pr"
		return it
	}

	// All THREE declared sections: it stays in the first. Repeated twenty times because Go's map
	//order is not guaranteed.
	for i := range 20 {
		best := assignAuthority(map[model.Section][]model.Item{
			model.SectionMentions: {nuevo(1)},
			model.SectionAuthored: {nuevo(1)},
			model.SectionReview:   {nuevo(1)},
		})
		if got := best[nuevo(1).ID()]; got != model.SectionAuthored {
			t.Fatalf("pasada %d: un ítem en las tres secciones quedó en %q, want %q. La "+
				"autoridad es el orden declarado, no el recorrido del mapa",
				i, got, model.SectionAuthored)
		}
	}

	for _, s := range []model.Section{
		model.SectionAuthored, model.SectionReview, model.SectionMentions,
	} {
		best := assignAuthority(map[model.Section][]model.Item{s: {nuevo(2)}})
		if got := best[nuevo(2).ID()]; got != s {
			t.Errorf("un ítem solo en %q quedó en %q", s, got)
		}
	}

	// A section prdash does NOT declare is not consulted at all: the item does not enter.
	futura := model.Section("futura")
	best := assignAuthority(map[model.Section][]model.Item{futura: {nuevo(3)}})
	if got, ok := best[nuevo(3).ID()]; ok {
		t.Errorf("una sección no declarada dio autoridad %q: `assignAuthority` recorre "+
			"las secciones declaradas, y si mirara las claves del mapa la autoridad "+
			"dependería de qué trae cada adapter", got)
	}

	// The same item in a declared and an undeclared one: the declared wins.
	best = assignAuthority(map[model.Section][]model.Item{
		futura:                {nuevo(4)},
		model.SectionMentions: {nuevo(4)},
	})
	if got := best[nuevo(4).ID()]; got != model.SectionMentions {
		t.Errorf("con una declarada y otra no quedó en %q, want %q", got, model.SectionMentions)
	}
}
