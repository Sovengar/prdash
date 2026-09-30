package inbox

import (
	"testing"
	"time"

	"prdash/internal/forge/model"
)

func mkItem(forge, host, project string, number int, decision string) model.Item {
	it := model.NewItem(model.RepoRef{
		Forge:   forge,
		Host:    host,
		Project: project,
		Owner:   "acme",
		Name:    "widget",
	}, number)
	it.ReviewDecision = decision
	it.State = "OPEN"
	it.UpdatedAt = time.Date(2026, 9, 24, 10, 0, 0, 0, time.UTC)
	return it
}

// TestBuildShowsThreeSectionsWithBothForges cubre el escenario "el inbox
// muestra las tres secciones con datos de ambos forges".
func TestBuildShowsThreeSectionsWithBothForges(t *testing.T) {
	gh := ForgeResult{
		Forge:    "github",
		Host:     "github.com",
		Authored: []model.Item{mkItem("github", "github.com", "acme/widget", 1, "APPROVED")},
		Review:   []model.Item{mkItem("github", "github.com", "acme/widget", 2, "REVIEW_REQUIRED")},
		Mentions: []model.Item{mkItem("github", "github.com", "acme/widget", 3, "")},
	}
	gl := ForgeResult{
		Forge:    "gitlab",
		Host:     "gitlab.example.com",
		Authored: []model.Item{mkItem("gitlab", "gitlab.example.com", "grp/proj", 4, "APPROVED")},
		Review:   []model.Item{mkItem("gitlab", "gitlab.example.com", "grp/proj", 5, "REVIEW_REQUIRED")},
		Mentions: []model.Item{mkItem("gitlab", "gitlab.example.com", "grp/proj", 6, "")},
	}

	box := Build([]ForgeResult{gh, gl})

	if len(box.Sections) != 3 {
		t.Fatalf("secciones = %d, want 3", len(box.Sections))
	}
	if got := box.Section(model.SectionAuthored); len(got.Items) != 2 {
		t.Errorf("authored = %d items, want 2", len(got.Items))
	}
	if got := box.Section(model.SectionReview); len(got.Items) != 2 {
		t.Errorf("review = %d items, want 2", len(got.Items))
	}
	if got := box.Section(model.SectionMentions); len(got.Items) != 2 {
		t.Errorf("mentions = %d items, want 2", len(got.Items))
	}

	// Cada ítem indica su forge y su host.
	for _, sec := range box.Sections {
		for _, it := range sec.Items {
			if it.Forge == "" || it.Host == "" {
				t.Errorf("ítem sin forge/host: %+v", it)
			}
			if it.Section != sec.Kind {
				t.Errorf("ítem %v marcado como %v", it.ID(), it.Section)
			}
		}
	}
}

// TestAuthoredOnlyOpenByMe comprueba que "creados por mí" solo recibe los ítems
// de authored, nunca los de review o menciones.
func TestAuthoredOnlyOpenByMe(t *testing.T) {
	gh := ForgeResult{
		Forge:    "github",
		Host:     "github.com",
		Authored: []model.Item{mkItem("github", "github.com", "acme/widget", 1, "")},
		Review:   []model.Item{mkItem("github", "github.com", "acme/widget", 2, "")},
	}
	box := Build([]ForgeResult{gh})

	authored := box.Section(model.SectionAuthored)
	if len(authored.Items) != 1 || authored.Items[0].Number != 1 {
		t.Fatalf("authored = %+v", authored.Items)
	}
}

// TestReviewIncludesRequestedAndAssigned cubre el escenario "review / asignados
// incluye review pedido y asignaciones".
func TestReviewIncludesRequestedAndAssigned(t *testing.T) {
	requested := mkItem("github", "github.com", "acme/widget", 10, "REVIEW_REQUIRED")
	requested.ReviewKind = model.ReviewRequested
	assigned := mkItem("gitlab", "gitlab.example.com", "grp/proj", 11, "REVIEW_REQUIRED")
	assigned.ReviewKind = model.ReviewAssigned

	box := Build([]ForgeResult{{
		Forge:  "github",
		Host:   "github.com",
		Review: []model.Item{requested, assigned},
	}})

	review := box.Section(model.SectionReview)
	if len(review.Items) != 2 {
		t.Fatalf("review = %d items, want 2", len(review.Items))
	}
	kinds := map[int]model.ReviewKind{}
	for _, it := range review.Items {
		kinds[it.Number] = it.ReviewKind
	}
	if kinds[10] != model.ReviewRequested {
		t.Errorf("kind de #10 = %q", kinds[10])
	}
	if kinds[11] != model.ReviewAssigned {
		t.Errorf("kind de #11 = %q", kinds[11])
	}
}

// TestDedupeKeepsHighestAuthority cubre el escenario "el mismo ítem no se
// duplica entre secciones o queries".
func TestDedupeKeepsHighestAuthority(t *testing.T) {
	shared := mkItem("github", "github.com", "acme/widget", 1, "")
	box := Build([]ForgeResult{{
		Forge:    "github",
		Host:     "github.com",
		Authored: []model.Item{shared},
		Mentions: []model.Item{shared},
	}})

	if got := box.Section(model.SectionAuthored); len(got.Items) != 1 {
		t.Fatalf("authored = %d, want 1", len(got.Items))
	}
	if got := box.Section(model.SectionMentions); len(got.Items) != 0 {
		t.Fatalf("mentions = %d, want 0 (deduplicado)", len(got.Items))
	}

	total := 0
	for _, sec := range box.Sections {
		total += len(sec.Items)
	}
	if total != 1 {
		t.Fatalf("total = %d, want 1", total)
	}
}

// TestDedupeWithinSection comprueba que la misma query repetida no duplica.
func TestDedupeWithinSection(t *testing.T) {
	it := mkItem("github", "github.com", "acme/widget", 1, "")
	box := Build([]ForgeResult{{
		Forge:    "github",
		Host:     "github.com",
		Authored: []model.Item{it, it},
	}})
	if got := box.Section(model.SectionAuthored); len(got.Items) != 1 {
		t.Fatalf("authored = %d, want 1", len(got.Items))
	}
}

// TestSortItemsRompeLosEmpatesEnCascada: el orden de la lista es a tres niveles
// y el desempate importa porque la lista es lo que el usuario recorre. Sin
// probarlos por separado, quitar cualquiera de los tres deja la lista igual en
// los tests que hay, que es exactamente cómo un empate mal resuelto llega a
// producción sin que nada se entere.
//
//  1. atención (score) — lo que requiere atención va primero.
//  2. fecha de actualización — a igual atención, lo más reciente.
//  3. número — a igual de todo, el más bajo; es lo que hace determinista la
//     lista cuando dos PR se movieron en el mismo segundo, que pasa cada vez que
//     seImportan en lote.
func TestSortItemsRompeLosEmpatesEnCascada(t *testing.T) {
	// Dos ítems con la misma atención y distinta fecha: manda la fecha.
	reciente := mkItem("github", "github.com", "acme/widget", 9, "APPROVED")
	antiguo := mkItem("github", "github.com", "acme/widget", 1, "APPROVED")
	reciente.UpdatedAt = time.Unix(2000, 0)
	antiguo.UpdatedAt = time.Unix(1000, 0)

	items := []model.Item{antiguo, reciente}
	sortItems(items)
	if items[0].Number != 9 {
		t.Errorf("a igual atención manda la fecha más reciente, primero = #%d", items[0].Number)
	}

	// Misma atención y misma fecha: manda el número más bajo, para que el orden
	// no dependa de cómo llegó la página.
	menor := mkItem("github", "github.com", "acme/widget", 3, "APPROVED")
	mayor := mkItem("github", "github.com", "acme/widget", 7, "APPROVED")
	menor.UpdatedAt = time.Unix(1000, 0)
	mayor.UpdatedAt = time.Unix(1000, 0)

	items = []model.Item{mayor, menor}
	sortItems(items)
	if items[0].Number != 3 {
		t.Errorf("a igual de todo manda el número más bajo, primero = #%d", items[0].Number)
	}

	// El mismo empate con la entrada YA en el orden correcto. Es el caso que
	// separa la comparación de una que siempre dice "sí": con dos ítems empatados
	// un `>=` en vez de `>` devuelve verdadero siempre, y un ordenamiento estable
	// con eso invierte la entrada en vez de mantenerla. Con la entrada en orden,
	// el original no la toca y el mutante la da vuelta.
	items = []model.Item{reciente, antiguo}
	sortItems(items)
	if items[0].Number != 9 || items[1].Number != 1 {
		t.Errorf("con la entrada ya en orden, un empate no debe reordenarla: %d, %d", items[0].Number, items[1].Number)
	}

	// Dos ítems con el MISMO número (repos distintos, así que los dos sobreviven
	// al dedupe), misma atención y misma fecha. Con dos números distintos el
	// desempate por número da el mismo resultado con `<` y con `<=`, así que el
	// único caso que distingue la comparación estricta es la igualdad: un `<=`
	// haría que cualquier orden se considerara "menor" y el estável acabaría
	// invirtiendo la entrada.
	igualA := mkItem("github", "github.com", "acme/widget", 5, "APPROVED")
	igualB := mkItem("gitlab", "gitlab.example.com", "grp/proj", 5, "APPROVED")
	igualA.UpdatedAt = time.Unix(1000, 0)
	igualB.UpdatedAt = time.Unix(1000, 0)
	items = []model.Item{igualA, igualB}
	sortItems(items)
	if items[0].Ref.Project != "acme/widget" || items[1].Ref.Project != "grp/proj" {
		t.Errorf("con empate total la lista debe conservar el orden de entrada, dio %s, %s",
			items[0].Ref.Project, items[1].Ref.Project)
	}

	// Y la cascada entera: changes requested gana a aprobado aunque sea más viejo.
	changes := mkItem("github", "github.com", "acme/widget", 1, "CHANGES_REQUESTED")
	changes.UpdatedAt = time.Unix(500, 0)
	approved := mkItem("github", "github.com", "acme/widget", 99, "APPROVED")
	approved.UpdatedAt = time.Unix(3000, 0)
	items = []model.Item{approved, changes}
	sortItems(items)
	if items[0].Number != 1 {
		t.Errorf("la atención manda sobre la fecha, primero = #%d", items[0].Number)
	}
}

// TestRankOrdenaLasSecciones por autoridad: la sección de mayor rank se queda
// con el ítem cuando aparece en varias. El orden es el de sectionOrder, así que
// rank() tiene que devolver el índice real y no un valor arbitrario.
func TestRankOrdenaLasSecciones(t *testing.T) {
	for i, kind := range sectionOrder {
		if got := rank(kind); got != i {
			t.Errorf("rank(%v) = %d, want %d (su posición en sectionOrder)", kind, got, i)
		}
	}
	// Una sección que no está en la lista va al final, no a un sitio arbitrario:
	// es lo que hace que un ítem de una sección desconocida no robe la
	// autoridad de una de verdad.
	if got := rank(model.Section("inventada")); got != len(sectionOrder) {
		t.Errorf("rank(inventada) = %d, want %d (al final)", got, len(sectionOrder))
	}
}

// TestOrderByAttention comprueba que lo que requiere atención va primero.
func TestOrderByAttention(t *testing.T) {
	approved := mkItem("github", "github.com", "acme/widget", 1, "APPROVED")
	changes := mkItem("github", "github.com", "acme/widget", 2, "CHANGES_REQUESTED")
	box := Build([]ForgeResult{{
		Forge:    "github",
		Host:     "github.com",
		Authored: []model.Item{approved, changes},
	}})

	items := box.Section(model.SectionAuthored).Items
	if len(items) != 2 {
		t.Fatalf("items = %d", len(items))
	}
	if items[0].Number != 2 {
		t.Fatalf("el primero debería ser el de changes requested, fue #%d", items[0].Number)
	}
}

func TestBuildAggregatesWarnings(t *testing.T) {
	w := model.Warning{Forge: "gitlab", Section: model.SectionReview, Kind: "auth", Msg: "401"}
	box := Build([]ForgeResult{{
		Forge:    "gitlab",
		Host:     "gitlab.example.com",
		Warnings: []model.Warning{w},
	}})

	if len(box.Warnings) != 1 || box.Warnings[0].Kind != "auth" {
		t.Fatalf("warnings = %+v", box.Warnings)
	}
	if !box.Empty() {
		t.Fatal("el inbox debería estar vacío")
	}
	// Aun sin ítems, siempre hay tres secciones para pintar.
	if len(box.Sections) != 3 {
		t.Fatalf("secciones = %d, want 3", len(box.Sections))
	}
}
