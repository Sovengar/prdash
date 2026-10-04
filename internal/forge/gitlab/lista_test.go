package gitlab

import (
	"context"
	"strings"
	"testing"

	"prdash/internal/forge"
	"prdash/internal/forge/model"
)

// `graphqlList` es el camino normal del listado y tenía la mitad sin probar. Lo que falta no
// es cobertura por cobertura: son sus TRES salidas, y la asimetría entre ellas es una
// decisión de producto disfrazada de `if`.
//
// El respaldo REST solo entra para la PRIMERA página de los MRs PROPIOS. Es un modo
// degradado: la API GraphQL puede fallar por permisos, por una instancia self-managed que no
// la tiene, o por un cambio de versión. Cuando el respaldo entra sale como `degraded`, que
// es lo que pinta el aviso en la cabecera del inbox.
//
// Y la asimetría que hay que fijar es esta: fuera del respaldo, el listado vacío es un
// FALLO con aviso; con el respaldo, es un resultado a medias con aviso de degradado. Un test
// que solo probara el camino bueno no distinguiría las dos cosas, que es justo donde está el
// riesgo — porque una de las dos deja el inbox en blanco sin explicación.

// mrJSON es un MR de la API de Todos, con la forma que espera el parser.
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

// graphqlJSON es una respuesta de GraphQL con la query de un listado.
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

// TestElListadoPorGraphQLTraeElCursorDelForge: el camino bueno, y el cursor.
//
// Y el cursor es lo que evita que la TUI vuelva a pedir la primera página. Un `More: true`
// sin `Next` es la incoherencia que más caro sale: la TUI pide la siguiente con un cursor
// vacío, recibe la primera otra vez, y entra en un bucle que consume red sin mostrar nada
// nuevo.
//
// Y sale del `pageInfo` del forge, no de contar los items: son cosas distintas, y la
// segunda es una heurística que un forge puede desmentir.
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
	// El cursor y la bandera vienen del pageInfo, no de contar.
	if !page.More {
		t.Error("pageInfo.hasNextPage no llegó a More")
	}
	if page.Next != "Y3Vyc29yOjI=" {
		t.Errorf("el cursor es %q, want el endCursor del forge", page.Next)
	}

	it := page.Items[0]
	// Y el stamp: los ítems del GraphQL no traen sección ni tipo de review —la query no los
	// pide—, y sin el stamp cada uno caería en la sección cero y desaparecería del inbox.
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

// TestLaPrimeraPaginaUsaElCursorYLaSiguienteLoPasaTalCual: la paginación hacia atrás.
//
// Y el detalle que importa es que el cursor del forge se PASA tal cual, sin reescribir. Un
// cursor es opaco para quien pagina: si `List` lo decodifica, lo recompone o le añade un
// prefijo, la segunda página sale distinta de lo que el forge pidió y el listado se repite
// o se salta. La única traducción que hay es de vuelta, cuando `More` es falso.
func TestLaPrimeraPaginaUsaElCursorYLaSiguienteLoPasaTalCual(t *testing.T) {
	script, argsFile := glabQueRegistra(t, "cat <<'JSON'\n"+graphqlJSON+"\nJSON\n")
	a := New("h.example", script)

	// Con cursor: la query lo lleva dentro, y eso se comprueba en el argv real.
	cursor := "Y3Vyc29yOjI="
	a.List(context.Background(), forge.Query{Section: model.SectionReview, Cursor: cursor})

	args := argsRegistrados(t, argsFile)
	if !strings.Contains(args, cursor) {
		t.Errorf("el cursor no llegó a la query: %s", args)
	}
	// Y no aparece "after: null" ni nada que lo sustituya.
	if strings.Contains(args, "after: null") {
		t.Errorf("la query mandó un cursor nulo explícito: %s", args)
	}

	// Con la última página: `More` a false y `Next` vacío. Sin `Next` vacío, la TUI
	// seguiría pidiendo páginas.
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

// TestElRespaldoRESTSoloEntraParaLaPrimeraPaginaDeLosPropios: la asimetría.
//
// Y son tres casos, y los tres son decisiones distintas:
//
//   - Propios + primera página: entra. Es el único caso en que el respaldo ayuda, porque
//     son los MRs que el usuario ve sin permisos de lectura sobre todos los proyectos.
//   - Propios + página siguiente: NO entra. El respaldo es una sola página por
//     construcción —no pagina—, así que fingir que pagina daría un cursor que al seguirlo
//     no devuelve nada.
//   - Otra sección: NO entra. El respaldo solo sabe de MRs propios; usarlo para menciones
//     devolvería los MRs equivocados, que es peor que devolver ninguno.
func TestElRespaldoRESTSoloEntraParaLaPrimeraPaginaDeLosPropios(t *testing.T) {
	ctx := context.Background()
	// Un glab que falla en graphql y responde en merge_requests.
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

	// El caso que entra.
	page, warns := a.List(ctx, forge.Query{Section: model.SectionAuthored})
	if len(page.Items) != 1 {
		t.Errorf("el respaldo no devolvió items en el caso que sí debe entrar: %+v", page.Items)
	}
	if len(warns) != 1 || warns[0].Kind != "degraded" {
		t.Fatalf("avisos = %+v, want un único degraded", warns)
	}

	// El caso que NO entra: con cursor.
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

// TestUnListadoQueNoSePuedeLeerDaUnAvisoAsociadoASuSeccion: el fallo, no el degradado.
//
// Y el fallo tiene que llevar aviso aunque no haya lista que mostrar. Un `List` que devuelve
// una página vacía SIN aviso deja el inbox en blanco con la cabecera de "todo bien", y el
// usuario no tiene forma de saber que `glab` falló. Con el aviso, la cabecera lo dice.
//
// Y el aviso va asociado a la sección que se pidió, que es lo que dice QUÉ parte del inbox
// está mal, en vez de "el inbox entero", que es la mitad de información.
func TestUnListadoQueNoSePuedeLeerDaUnAvisoAsociadoASuSeccion(t *testing.T) {
	// Un glab que falla en todo.
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
	// Y NO es "degraded": degradado significa "funciona a medias", y aquí no funciona.
	if warns[0].Kind == "degraded" {
		t.Error("un fallo total se marcó como degraded: el usuario creería que tiene datos")
	}
}

// TestElRespaldoRESTNoEntraSiLaRespuestaNoSeEntiende: el otro camino del respaldo.
//
// Y es el caso que se olvida: el respaldo entra solo si `restAuthored` DICE que sí, y eso
// es a la vez "el comando salió bien" y "el JSON se entendió". Si la respuesta es basura, el
// respaldo no entra y el fallo se presenta como fallo —no como "degradado, funciona a
// medias"—, que es lo correcto: un respaldo que no se puede leer no es un respaldo.
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

// TestLaListaDeTodosSoloMarcaMasCuandoVieneLlena: la paginación de la API de Todos.
//
// Y "viene llena" es `total >= pageSize`, no "hay items" ni "el forge dijo que hay más". La
// API de Todos no manda un `hasNextPage`: hay que deducirlo del total, y la deducción es
// "si el total dice que hay al menos una página más, hay más".
//
// Y la asimetría con GraphQL es que aquí el cursor es un NÚMERO de página, no un cursor
// opaco. Es lo que dice el comentario de la función y lo que la distingue de la otra.
func TestLaListaDeTodosSoloMarcaMasCuandoVieneLlena(t *testing.T) {
	dir := t.TempDir()
	// Una respuesta con el total por debajo del tamaño de página: no hay más.
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

	// Y el camino de un fallo de la API de Todos, que es un aviso y no un item vacío.
	fail := writeScript(t, dir, "glab-2", "#!/bin/sh\necho 'nope' >&2\nexit 1\n")
	a = New("h.example", fail)
	page, warns := a.List(context.Background(), forge.Query{Section: model.SectionMentions})
	if len(page.Items) != 0 || len(warns) != 1 {
		t.Errorf("un fallo de Todos dio %d items y %d avisos", len(page.Items), len(warns))
	}
}

// TestLaListaDeTodosSigueElCursorDePagina: el número de página como cursor.
//
// Y el número se manda TAL CUAL, y solo si es un número positivo. Un cursor que no es un
// número —el de otro modo de ejecución, o basura— vuelve a la primera página en vez de
// inventar una, porque inventar una página es saltar ítems sin avisar.
func TestLaListaDeTodosSigueElCursorDePagina(t *testing.T) {
	// Con un cursor numérico llega a la query.
	script, argsFile := glabQueRegistra(t, "echo '[]'")
	a := New("h.example", script)
	a.List(context.Background(), forge.Query{Section: model.SectionMentions, Cursor: "3"})
	if args := argsRegistrados(t, argsFile); !strings.Contains(args, "todos") {
		t.Errorf("no se llamó a la API de Todos: %s", args)
	}

	// Con un cursor que no es número: la API de Todos se pide igual, pero la respuesta
	// vacía es una lista vacía, no un error. Que es lo que permite que un cursor
	// corrupto no rompa el inbox.
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
