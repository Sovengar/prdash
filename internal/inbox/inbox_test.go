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
