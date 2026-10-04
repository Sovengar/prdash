package inbox

import (
	"testing"
	"time"

	"prdash/internal/forge/model"
)

// Los dos de inbox.go que sobrevivían son el orden y la autoridad, y los dos son
// comparaciones cuyo borde es un empate.
//
// Y un empate no es un caso raro aquí: dos PRs del mismo autor en el mismo estado tienen
// la misma atención, y eso pasa en cada ejecución.

// TestElOrdenEsAtencionFechaYNumero: el orden tiene tres desempates y el último es el
// número.
//
// Y el tercero es el que hace falta para que el comparador sea un ORDEN TOTAL, que es lo
// que necesita `sort.SliceStable` para converger. Con solo atención y fecha, dos PRs del
// mismo autor en el mismo estado y del mismo día empatan, y `SliceStable` los deja en el
// orden de llegada —que depende de qué forge respondió antes, y eso cambia entre
// ejecuciones—. Con el número, el orden es el mismo siempre.
//
// Y ese es el motivo por el que la comparación de atención tiene que ser ESTRICTA: con
// `>=` en vez de `>`, dos ítems con la misma atención se declaran ordenados en los dos
// sentidos a la vez, y `sort.SliceStable` deja de tener un orden parcial que cumplir. El
// resultado no es un fallo visible, es un orden distinto cada vez según el pivote que el
// sort elija.
//
// Por eso los tres casos van en el test, y el primero es el que más cuesta ver: con la
// atención IGUAL y la fecha IGUAL, el `>=` y el `>` se distinguen, y sin los tres
// desempates probados no se sabe cuál de ellos manda.
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

	// EL TERCER DESEMPATE, que es el que se ve sin pensar. Cuatro PRs con la misma
	// atención y la misma fecha, en orden inverso al número: tienen que salir por
	// número, no como entraron.
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

	// Y al revés, para que no sea un caso que sale solo por casualidad.
	items = []model.Item{
		item(1, "approved", fecha), item(3, "approved", fecha),
		item(2, "approved", fecha), item(4, "approved", fecha),
	}
	sortItems(items)
	if got := numeros(items); !igual(got, []int{1, 2, 3, 4}) {
		t.Errorf("con atención y fecha iguales pero en otro orden de entrada salió %v, "+
			"want [1 2 3 4]", got)
	}

	// EL SEGUNDO: misma atención, fechas distintas. Va antes el más reciente.
	items = []model.Item{
		item(1, "approved", fecha), item(2, "approved", fecha.Add(time.Hour)),
		item(3, "approved", fecha.Add(48*time.Hour)),
	}
	sortItems(items)
	if got := numeros(items); !igual(got, []int{3, 2, 1}) {
		t.Errorf("con la atención igual y fechas distintas salió %v, want [3 2 1]: manda "+
			"la actualización más reciente", got)
	}

	// Y con el número al revés para que la fecha no compense: el 1 es el más antiguo
	// pero si la fecha mandara sobre el número dentro de la atención igual, esto
	// distinguiría las dos cosas.
	items = []model.Item{
		item(9, "approved", fecha), item(1, "approved", fecha.Add(time.Hour)),
	}
	sortItems(items)
	if got := numeros(items); !igual(got, []int{1, 9}) {
		t.Errorf("con el número al revés y la fecha al revés salió %v, want [1 9]", got)
	}

	// EL PRIMERO, que es el que mata al `>=`: la atención manda sobre todo lo demás.
	// Un PR con cambios solicitados va primero aunque sea el más viejo y el de número más
	// alto, porque es el único de los cuatro que necesita una acción.
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

	// Y el estado que de verdad se ve arriba del todo, para que el orden entero se lea
	// de una vez: checks rotos, cambios pedidos, review pendiente, aprobado y
	// pendiente, de más nuevo a más viejo.
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

// TestLaSeccionConMasAutoridadSeQuedaConElItem: un ítem que aparece en varias secciones
// se queda en la de más autoridad, y el criterio es el ORDEN DECLARADO.
//
// Y la autoridad es lo que decide si el usuario lo ve: un PR que escribiste tú está en
// `authored` y en `mentions`, y aparecer en las dos es verlo dos veces. El criterio es el
// orden de `sectionOrder` y no el orden de llegada de los datos, que depende de qué forge
// respondió antes —y eso cambia entre ejecuciones—.
//
// Y hay dos cosas que este test afirma porque son las que hacen que la comparación sea
// fiable, y ninguna es lo que parece:
//
//   - Solo se miran las secciones DECLARADAS. `assignAuthority` recorre `sectionOrder`, no
//     las claves del mapa, así que una sección que prdash no conoce no se mira. La
//     primera versión de este test construía un empate de rango con dos secciones
//     desconocidas y medía "" en las diez vueltas: no era un empate, era que nunca se
//     miran. Y eso no es un accidente: es lo que hace que la autoridad no dependa de qué
//     trae cada adapter.
//
//   - La autoridad es un RANGO, y con el rango solo hay dos casos: una sección que gana o
//     una que pierde. No hay empate entre dos secciones declaradas, porque `rank` es la
//     posición en `sectionOrder` y son todas distintas. El `!ok` cubre la primera vez que
//     se ve el ítem, y el `rank` cubre las siguientes.
func TestLaSeccionConMasAutoridadSeQuedaConElItem(t *testing.T) {
	nuevo := func(n int) model.Item {
		it := model.NewItem(model.RepoRef{Forge: "github", Host: "github.com",
			Project: "acme/widget", Owner: "acme", Name: "widget"}, n)
		it.Title = "pr"
		return it
	}

	// En las TRES secciones declaradas: se queda en la primera, que es la de más
	// autoridad. Y se repite veinte veces porque el mapa de Go no tiene orden: con un
	// `<=` en la comparación de rango el resultado sería el de la última que se
	// procesa, con lo que el mismo dato daría respuestas distintas entre ejecuciones.
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

	// Y cada sección gana cuando es la única: el caso normal.
	for _, s := range []model.Section{
		model.SectionAuthored, model.SectionReview, model.SectionMentions,
	} {
		best := assignAuthority(map[model.Section][]model.Item{s: {nuevo(2)}})
		if got := best[nuevo(2).ID()]; got != s {
			t.Errorf("un ítem solo en %q quedó en %q", s, got)
		}
	}

	// Y una sección que prdash NO declara no se mira, ni ganas ni pierdes: el ítem no
	// entra en el mapa de autoridad. Lo que se afirma es que sale vacío, no que salga
	// con la sección desconocida.
	futura := model.Section("futura")
	best := assignAuthority(map[model.Section][]model.Item{futura: {nuevo(3)}})
	if got, ok := best[nuevo(3).ID()]; ok {
		t.Errorf("una sección no declarada dio autoridad %q: `assignAuthority` recorre "+
			"las secciones declaradas, y si mirara las claves del mapa la autoridad "+
			"dependería de qué trae cada adapter", got)
	}

	// Y el mismo ítem en una declarada y una no declarada: gana la declarada, porque la
	// no declarada ni se mira.
	best = assignAuthority(map[model.Section][]model.Item{
		futura:                {nuevo(4)},
		model.SectionMentions: {nuevo(4)},
	})
	if got := best[nuevo(4).ID()]; got != model.SectionMentions {
		t.Errorf("con una declarada y otra no quedó en %q, want %q", got, model.SectionMentions)
	}
}
