package github

import (
	"context"
	"testing"

	"prdash/internal/forge"
	"prdash/internal/forge/model"
)

const busquedaConPRs = `{"data":{"search":{"issueCount":2,"pageInfo":` +
	`{"hasNextPage":true,"endCursor":"CUR2"},"nodes":[` +
	`{"__typename":"PullRequest","number":7,"title":"uno","state":"OPEN",` +
	`"updatedAt":"2026-03-17T10:00:00Z","url":"https://github.com/acme/widget/pull/7",` +
	`"author":{"login":"alice"},"headRefName":"feat/x","baseRefName":"main",` +
	`"mergeable":"MERGEABLE","isDraft":false,` +
	`"repository":{"nameWithOwner":"acme/widget","name":"widget",` +
	`"owner":{"login":"acme"},"mergeCommitAllowed":true,"rebaseMergeAllowed":true,` +
	`"squashMergeAllowed":true}},` +
	`{"__typename":"PullRequest","number":8,"title":"dos","state":"OPEN",` +
	`"updatedAt":"2026-03-18T10:00:00Z","url":"https://github.com/acme/widget/pull/8",` +
	`"author":{"login":"bob"},"headRefName":"fix/y","baseRefName":"main",` +
	`"mergeable":"MERGEABLE","isDraft":false,` +
	`"repository":{"nameWithOwner":"acme/widget","name":"widget",` +
	`"owner":{"login":"acme"},"mergeCommitAllowed":true,"rebaseMergeAllowed":true,` +
	`"squashMergeAllowed":true}}]}}}`

// It goes through printf with SINGLE quotes and not a heredoc.
func ghQueImprime(salida string) string {
	return `printf '%s\n' '` + salida + `'`
}

func TestListarConUnaRespuestaValidaDevuelveLosItemsYLaPaginacion(t *testing.T) {
	a := New("github.com", ghQueDevuelve(t, ghQueImprime(busquedaConPRs)))
	q := forge.Query{Section: model.SectionReview, ReviewKind: model.ReviewRequested}

	page, warns := a.List(context.Background(), q)
	if len(warns) != 0 {
		t.Fatalf("una respuesta válida dio %d avisos: %+v", len(warns), warns)
	}
	if len(page.Items) != 2 {
		t.Fatalf("salieron %d ítems de una respuesta con 2, want 2", len(page.Items))
	}

	primero := page.Items[0]
	if primero.Number != 7 || primero.Title != "uno" {
		t.Errorf("el primer ítem es %+v", primero)
	}
	if primero.Forge != "github" || primero.Host != "github.com" {
		t.Errorf("la identidad del ítem es %s/%s", primero.Forge, primero.Host)
	}
	if primero.Ref.Project != "acme/widget" || primero.Ref.Owner != "acme" ||
		primero.Ref.Name != "widget" {
		t.Errorf("el repo del ítem es %q (owner %q, name %q): sin él no se puede clonar",
			primero.Ref.Project, primero.Ref.Owner, primero.Ref.Name)
	}
	if primero.SourceBranch != "feat/x" || primero.TargetBranch != "main" {
		t.Errorf("las ramas del ítem son %q -> %q", primero.SourceBranch, primero.TargetBranch)
	}
	if id := primero.ID(); id.Forge != "github" || id.Host != "github.com" ||
		id.Project != "acme/widget" {
		t.Errorf("el ID del ítem es %+v", id)
	}

	if !page.More {
		t.Error("More = false con hasNextPage=true: el inbox se trunca sin avisar y el " +
			"usuario ve cinco PRs de cuarenta sin ninguna señal")
	}
	if page.Next != "CUR2" {
		t.Errorf("Next = %q, want CUR2: sin el cursor la paginación no puede continuar", page.Next)
	}
	// Both together: More with no cursor, or a cursor with no More, are states that do not exist.
	if page.More && page.Next == "" {
		t.Error("More = true con Next vacío: la siguiente página no se puede pedir")
	}
}

// It is what lets the same PR appear in two columns with the right review kind in each.
func TestListarMarcaCadaItemConLaSeccionQueSeConsulta(t *testing.T) {
	a := New("github.com", ghQueDevuelve(t, ghQueImprime(busquedaConPRs)))

	for _, c := range []struct {
		nombre string
		q      forge.Query
	}{
		{"review solicitada", forge.Query{
			Section: model.SectionReview, ReviewKind: model.ReviewRequested}},
		{"review asignada", forge.Query{
			Section: model.SectionReview, ReviewKind: model.ReviewAssigned}},
		{"propios", forge.Query{Section: model.SectionAuthored}},
	} {
		page, warns := a.List(context.Background(), c.q)
		if len(warns) != 0 {
			t.Errorf("%s: %d avisos", c.nombre, len(warns))
			continue
		}
		for _, it := range page.Items {
			if it.Section != c.q.Section {
				t.Errorf("%s: el ítem %d salió con sección %q, want %q",
					c.nombre, it.Number, it.Section, c.q.Section)
			}
		}
	}
}

// A different case from broken JSON, and the difference matters: GraphQL answers 200 with an
// errors array.
func TestListarConGraphQLQueContestaErroresDaUnAvisoYNoItems(t *testing.T) {
	const conErrores = `{"errors":[{"message":"Field 'reviewDecision' doesn't exist on ` +
		`type 'PullRequest'","type":"INTERNAL"}]}`

	a := New("github.com", ghQueDevuelve(t, ghQueImprime(conErrores)))
	page, warns := a.List(context.Background(), forge.Query{
		Section: model.SectionReview, ReviewKind: model.ReviewRequested,
	})

	if len(page.Items) != 0 {
		t.Errorf("salieron %d ítems de una respuesta que solo trae errores", len(page.Items))
	}
	if len(warns) != 1 {
		t.Fatalf("%d avisos, want 1: %+v", len(warns), warns)
	}
	if !contains(warns[0].Msg, "reviewDecision") {
		t.Errorf("el aviso %q no trae el motivo del servidor", warns[0].Msg)
	}
}

// A deliberate limit: when GraphQL fails, one's own PRs are asked over REST and the rest are not.
func TestElFallbackRESTSeUsaSoloEnLaPrimeraPaginaDeLosPropios(t *testing.T) {
	caido := New("github.com", ghQueDevuelve(t, "echo 'gh: could not resolve host' >&2\nexit 1"))

	primera, warns := caido.List(context.Background(), forge.Query{Section: model.SectionAuthored})
	if len(primera.Items) != 0 {
		t.Errorf("con `gh` caído salieron %d ítems de la primera página", len(primera.Items))
	}
	if len(warns) == 0 {
		t.Error("con `gh` caído no hay aviso: el inbox parecería vacío sin explicación")
	}
	segunda, warns2 := caido.List(context.Background(),
		forge.Query{Section: model.SectionAuthored, Cursor: "CUR2"})
	if len(segunda.Items) != 0 {
		t.Errorf("con `gh` caído la segunda página trajo %d ítems", len(segunda.Items))
	}
	if len(warns2) == 0 {
		t.Error("la segunda página sin cursor no ha dado aviso")
	}
}

// A section with no search does not query the forge on purpose.
func TestUnaSeccionNoSoportadaNiSeConsultaElForge(t *testing.T) {
	a := New("github.com", ghQueDevuelve(t, "echo 'no debería haberme llamado' >&2\nexit 1"))

	page, warns := a.List(context.Background(), forge.Query{Section: "una-seccion-inventada"})
	if len(page.Items) != 0 {
		t.Errorf("una sección no soportada trajo %d ítems", len(page.Items))
	}
	if len(warns) != 1 {
		t.Fatalf("%d avisos, want 1: %+v", len(warns), warns)
	}
	if warns[0].Kind != "unsupported" {
		t.Errorf("clase %q, want unsupported: no es un fallo de red ni de parseo, es que "+
			"esta combinación no existe en este forge", warns[0].Kind)
	}
	if !contains(warns[0].Msg, "una-seccion-inventada") {
		t.Errorf("el aviso %q no nombra la sección que no se soporta", warns[0].Msg)
	}
	if warns[0].Section != "una-seccion-inventada" {
		t.Errorf("el aviso no lleva la sección (lleva %q)", warns[0].Section)
	}
}

func contains(haystack, needle string) bool {
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return true
		}
	}
	return len(needle) == 0
}
