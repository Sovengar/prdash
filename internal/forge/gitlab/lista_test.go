package gitlab

import (
	"context"
	"strings"
	"testing"

	"prdash/internal/forge"
	"prdash/internal/forge/model"
)

const mrJSON = `[{"iid":12,"title":"MR","description":"desc",
"web_url":"u","state":"opened","source_branch":"feat/a","target_branch":"main",
"updated_at":"2026-09-21T07:00:00Z","created_at":"2026-09-20T07:00:00Z",
"author":{"username":"me"},"labels":["uno"],
"references":{"full":"grp/sub/proy!12","short":"!12"},
"assignees":[{"username":"otro"}],
"reviewers":[{"username":"me"}],
"detailed_merge_status":"mergeable","merge_status":"can_be_merged",
"diff_stats":{"additions":10,"deletions":2,"changes":3},
"pipeline":{"status":"success"},
"upvotes":1,"downvotes":0,"user_notes_count":2}]`

const graphqlJSON = `{"data":{"currentUser":{"reviewRequestedMergeRequests":{"nodes":[{
	"iid":7,"title":"PR","description":"d","webUrl":"u","state":"opened",
	"sourceBranch":"feat/b","targetBranch":"main",
	"updatedAt":"2026-09-21T07:00:00Z","createdAt":"2026-09-20T07:00:00Z",
	"author":{"username":"me"},"labels":{"nodes":[{"name":"uno"}]},
	"assignees":{"nodes":[{"username":"otro"}]},
	"reviewers":{"nodes":[{"username":"otro"}]},
	"mergeStatus":"MERGEABLE","headRefOid":"deadbeef",
	"diffStats":[{"additions":5,"deletions":1,"changeCount":2}],
	"headPipeline":{"status":"SUCCESS"},"project":{"fullPath":"grp/sub/proy","name":"proy","group":{"fullPath":"grp/sub"}}
}],"pageInfo":{"hasNextPage":true,"endCursor":"Y3Vyc29yOjI="}}}}}`

// The cursor is what stops the TUI from asking for the first page again.
func TestElListadoPorGraphQLTraeElCursorDelForge(t *testing.T) {
	script, _ := glabQueRegistra(t, "cat <<'JSON'\n"+graphqlJSON+"\nJSON\n")
	a := New("h.example", script)

	page, warns := a.List(context.Background(), forge.Query{Section: model.SectionReview})

	if len(warns) != 0 {
		t.Errorf("el camino bueno dio avisos %+v", warns)
	}
	if len(page.Items) != 1 {
		t.Fatalf("salieron %d items, want 1: %+v", len(page.Items), page.Items)
	}
	if !page.More {
		t.Error("pageInfo.hasNextPage no llegó a More")
	}
	if page.Next != "Y3Vyc29yOjI=" {
		t.Errorf("el cursor es %q, want el endCursor del forge", page.Next)
	}

	it := page.Items[0]
	// The stamp: GraphQL items carry no section and no review kind, the query does not ask for them.
	if it.Section != model.SectionReview {
		t.Errorf("el item quedó en la sección %q: sin stamp no aparece en el inbox", it.Section)
	}
	if it.Forge != ForgeName || it.Host != "h.example" {
		t.Errorf("el item quedó como %s@%s, y el host sale de la config", it.Forge, it.Host)
	}
	if it.Ref.Project == "" {
		t.Error("el item no trae proyecto")
	}
}

// The forge's cursor is passed through as given.
func TestLaPrimeraPaginaUsaElCursorYLaSiguienteLoPasaTalCual(t *testing.T) {
	script, argsFile := glabQueRegistra(t, "cat <<'JSON'\n"+graphqlJSON+"\nJSON\n")
	a := New("h.example", script)

	cursor := "Y3Vyc29yOjI="
	a.List(context.Background(), forge.Query{Section: model.SectionReview, Cursor: cursor})

	args := argsRegistrados(t, argsFile)
	if !strings.Contains(args, cursor) {
		t.Errorf("el cursor no llegó a la query: %s", args)
	}
	if strings.Contains(args, "after: null") {
		t.Errorf("la query mandó un cursor nulo explícito: %s", args)
	}

	// On the last page: More false and Next empty. Without an empty Next the TUI would keep paging.
	fin := strings.Replace(graphqlJSON, `"hasNextPage":true,"endCursor":"Y3Vyc29yOjI="`,
		`"hasNextPage":false,"endCursor":null`, 1)
	script, _ = glabQueRegistra(t, "cat <<'JSON'\n"+fin+"\nJSON\n")
	a = New("h.example", script)
	page, _ := a.List(context.Background(), forge.Query{Section: model.SectionReview})
	if page.More {
		t.Error("la última página salió con More=true")
	}
	if page.Next != "" {
		t.Errorf("la última página trajo cursor %q", page.Next)
	}
}

// Three cases and three different decisions.
func TestElRespaldoRESTSoloEntraParaLaPrimeraPaginaDeLosPropios(t *testing.T) {
	ctx := context.Background()
	cuerpo := `case "$*" in
  *graphql*) echo "boom" >&2; exit 1;;
  *merge_requests*) cat <<'JSON'
` + mrJSON + `
JSON
    exit 0;;
esac
exit 1
`
	script, _ := glabQueRegistra(t, cuerpo)
	a := New("h.example", script)

	page, warns := a.List(ctx, forge.Query{Section: model.SectionAuthored})
	if len(page.Items) != 1 {
		t.Errorf("el respaldo no devolvió items en el caso que sí debe entrar: %+v", page.Items)
	}
	if len(warns) != 1 || warns[0].Kind != "degraded" {
		t.Fatalf("avisos = %+v, want un único degraded", warns)
	}

	script, _ = glabQueRegistra(t, cuerpo)
	a = New("h.example", script)
	page, warns = a.List(ctx, forge.Query{Section: model.SectionAuthored, Cursor: "2"})
	if len(page.Items) != 0 {
		t.Errorf("con cursor el respaldo devolvió %d items: no pagina", len(page.Items))
	}
	if len(warns) != 1 || warns[0].Kind == "degraded" {
		t.Errorf("avisos = %+v: con cursor no debería entrar el respaldo", warns)
	}

	// Y otra sección: tampoco.
	script, _ = glabQueRegistra(t, cuerpo)
	a = New("h.example", script)
	page, warns = a.List(ctx, forge.Query{Section: model.SectionMentions})
	if len(page.Items) != 0 {
		t.Errorf("en menciones el respaldo devolvió %d items de los propios", len(page.Items))
	}
	if len(warns) != 1 || warns[0].Kind == "degraded" {
		t.Errorf("avisos = %+v: en menciones no debería entrar", warns)
	}
}

// The failure has to carry a warning even when there is no list to show.
func TestUnListadoQueNoSePuedeLeerDaUnAvisoAsociadoASuSeccion(t *testing.T) {
	script, _ := glabQueRegistra(t, "echo 'se rompio' >&2\nexit 1\n")
	a := New("h.example", script)

	page, warns := a.List(context.Background(), forge.Query{Section: model.SectionReview})

	if len(page.Items) != 0 {
		t.Errorf("un listado que falla devolvió %d items", len(page.Items))
	}
	if len(warns) != 1 {
		t.Fatalf("avisos = %+v, want 1: un fallo sin aviso deja el inbox en blanco", warns)
	}
	if warns[0].Section != model.SectionReview {
		t.Errorf("el aviso quedó en la sección %q, want la que se pidió", warns[0].Section)
	}
	if warns[0].Forge != ForgeName {
		t.Errorf("el aviso no trae el nombre del forge: %+v", warns[0])
	}
	if strings.TrimSpace(warns[0].Msg) == "" {
		t.Error("el aviso llegó sin texto")
	}
	// NOT "degraded": degraded means "works halfway", and this does not work.
	if warns[0].Kind == "degraded" {
		t.Error("un fallo total se marcó como degraded: el usuario creería que tiene datos")
	}
}

// The case nobody remembers: the fallback only enters if restAuthored actually parses.
func TestElRespaldoRESTNoEntraSiLaRespuestaNoSeEntiende(t *testing.T) {
	cuerpo := `case "$*" in
  *graphql*) echo "boom" >&2; exit 1;;
  *merge_requests*) echo 'esto no es json'; exit 0;;
esac
exit 1
`
	script, _ := glabQueRegistra(t, cuerpo)
	a := New("h.example", script)

	page, warns := a.List(context.Background(), forge.Query{Section: model.SectionAuthored})

	if len(page.Items) != 0 {
		t.Errorf("con basura en el respaldo salieron %d items", len(page.Items))
	}
	if len(warns) != 1 {
		t.Fatalf("avisos = %+v, want 1", warns)
	}
	if warns[0].Kind == "degraded" {
		t.Error("un respaldo ilegible se'annonceó como degradado: no es un respaldo, es un fallo")
	}
}

// "Comes full" is `total >= pageSize`, not "has items".
func TestLaListaDeTodosSoloMarcaMasCuandoVieneLlena(t *testing.T) {
	dir := t.TempDir()
	short := `[{"iid":1,"title":"a","web_url":"u","state":"opened",
"source_branch":"s","target_branch":"main","updated_at":"2026-09-21T07:00:00Z",
"author":{"username":"me"},"references":{"full":"p!1","short":"!1"}}]`
	script := writeScript(t, dir, "glab-1", "#!/bin/sh\ncat <<'JSON'\n"+short+"\nJSON\n")
	a := New("h.example", script)
	page, _ := a.List(context.Background(), forge.Query{Section: model.SectionMentions})
	if page.More {
		t.Error("una respuesta corta se marcó como More")
	}
	if page.Next != "" {
		t.Errorf("una respuesta corta trajo cursor %q", page.Next)
	}

	fail := writeScript(t, dir, "glab-2", "#!/bin/sh\necho 'nope' >&2\nexit 1\n")
	a = New("h.example", fail)
	page, warns := a.List(context.Background(), forge.Query{Section: model.SectionMentions})
	if len(page.Items) != 0 || len(warns) != 1 {
		t.Errorf("un fallo de Todos dio %d items y %d avisos", len(page.Items), len(warns))
	}
}

// The number is passed AS GIVEN and only if it is a positive number.
func TestLaListaDeTodosSigueElCursorDePagina(t *testing.T) {
	script, argsFile := glabQueRegistra(t, "echo '[]'")
	a := New("h.example", script)
	a.List(context.Background(), forge.Query{Section: model.SectionMentions, Cursor: "3"})
	if args := argsRegistrados(t, argsFile); !strings.Contains(args, "todos") {
		t.Errorf("no se llamó a la API de Todos: %s", args)
	}

	for _, cursor := range []string{"", "abc", "-1", "0"} {
		script, _ = glabQueRegistra(t, "echo '[]'")
		a = New("h.example", script)
		page, warns := a.List(context.Background(),
			forge.Query{Section: model.SectionMentions, Cursor: cursor})
		if len(warns) != 0 {
			t.Errorf("con cursor %q dio avisos %+v", cursor, warns)
		}
		if len(page.Items) != 0 {
			t.Errorf("con cursor %q devolvió items de una respuesta vacía", cursor)
		}
	}
}
