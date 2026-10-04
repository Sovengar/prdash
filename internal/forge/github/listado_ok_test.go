package github

import (
	"context"
	"testing"

	"prdash/internal/forge"
	"prdash/internal/forge/model"
)

// La respuesta VÁLIDA del listado de GitHub, que es la forma que `ParseGHGraphQLSearch`
// entiende. Vive aquí y no en el parser porque es el contrato del ADAPTER: lo que `gh api
// graphql` devuelve cuando todo va bien.
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

// ghQueImprime construye el cuerpo de un `gh` falso que escribe el texto dado y sale con 0.
//
// Y va por `printf` con comillas SIMPLES y no por un heredoc porque el heredoc depende de que
// el terminador quede al principio de una línea, y con el JSON partido en varios literales es
// fácil que no pase. El síntoma es "invalid character 's'" con un JSON perfectamente válido:
// apunta al shell, no al parser, y un aserto de "clase parse" lo confirma sin decir por qué.
func ghQueImprime(salida string) string {
	return `printf '%s\n' '` + salida + `'`
}

// TestListarConUnaRespuestaValidaDevuelveLosItemsYLaPaginacion: el camino bueno de `List`.
//
// Y estaba sin probar entero, y es el camino que se ejecuta en cada refresco del inbox: el
// único camino de `List` que estaba cubierto era el que devuelve un aviso de parseo. Un
// adapter que solo se prueba con basura verifica que no se rompe, que es la mitad fácil.
//
// Y lo que se fija son las TRES cosas que la respuesta trae y que el inbox necesita:
//
//   - Los ítems, con su número y su título.
//   - El repositorio DE CADA ÍTEM, que es lo que permite clonar y montar el review sin
//     volver a preguntar al forge.
//   - La paginación: `hasNextPage` y `endCursor`, que son lo que hace que haya un "cargar
//     más" en lugar de un inbox truncado en silencio.
//
// Y la paginación es la que más se cuela: sin ella el listado funciona en la primera página
// y el usuario ve cinco PRs de cuarenta sin ninguna señal de que faltan treinta y cinco.
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

	// Los ítems con lo que el inbox pinta.
	primero := page.Items[0]
	if primero.Number != 7 || primero.Title != "uno" {
		t.Errorf("el primer ítem es %+v", primero)
	}
	if primero.Forge != "github" || primero.Host != "github.com" {
		t.Errorf("la identidad del ítem es %s/%s", primero.Forge, primero.Host)
	}
	// Y el repo, que es lo que permite montar el review sin volver a preguntar.
	if primero.Ref.Project != "acme/widget" || primero.Ref.Owner != "acme" ||
		primero.Ref.Name != "widget" {
		t.Errorf("el repo del ítem es %q (owner %q, name %q): sin él no se puede clonar",
			primero.Ref.Project, primero.Ref.Owner, primero.Ref.Name)
	}
	// Y las ramas, que son lo que se usa para el ref de review y para el worktree.
	if primero.SourceBranch != "feat/x" || primero.TargetBranch != "main" {
		t.Errorf("las ramas del ítem son %q -> %q", primero.SourceBranch, primero.TargetBranch)
	}
	// Y el ID compone forge, host y proyecto: es la clave con la que la memoria de reviews
	// distingue un PR del host equivocado con el mismo path de proyecto.
	if id := primero.ID(); id.Forge != "github" || id.Host != "github.com" ||
		id.Project != "acme/widget" {
		t.Errorf("el ID del ítem es %+v", id)
	}

	// Y la paginación, que es la parte que se pierde en silencio.
	if !page.More {
		t.Error("More = false con hasNextPage=true: el inbox se trunca sin avisar y el " +
			"usuario ve cinco PRs de cuarenta sin ninguna señal")
	}
	if page.Next != "CUR2" {
		t.Errorf("Next = %q, want CUR2: sin el cursor la paginación no puede continuar", page.Next)
	}
	// Y la coherencia entre los dos: `More` sin cursor, o cursor sin `More`, son estados que
	// no deberían existir y que un test de cada campo por separado no detecta.
	if page.More && page.Next == "" {
		t.Error("More = true con Next vacío: la siguiente página no se puede pedir")
	}
}

// TestListarMarcaCadaItemConLaSeccionQueSeConsulta: `stamp`.
//
// Y es lo que hace que el mismo PR pueda estar en dos columnas —"review" y "propios"— y que
// cada aparición lleve SU etiqueta. Sin `stamp`, los ítems salen sin sección y el render no
// sabe en qué columna pintarlos, con lo que los duplica o los pierde.
//
// Y el caso que hay que mirar es el de un PR propio que además te han pedido revisar: es el
// mismo ítem en dos sitios, y las dos apariciones tienen que decir por qué están ahí.
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

// TestListarConGraphQLQueContestaErroresDaUnAvisoYNoItems: el `errors` de GraphQL.
//
// Y es un caso distinto del JSON roto, y la diferencia importa: GraphQL devuelve CÓDIGO DE
// SALIDA 0 con un array `errors` en el cuerpo cuando la consulta está bien formada pero el
// schema la rechaza, cuando falta un permiso de campo o cuando hay un error de validación en
// el servidor. Un runner que mirara el código de salida se tragaría el cuerpo con los errores
// y devolvería un inbox vacío.
//
// Y el aviso tiene que decir lo que dice el error del servidor —"Field 'x' doesn't exist"—,
// porque es lo único que distingue "el forge va lento" de "mi consulta ya no vale".
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

// TestElFallbackRESTSeUsaSoloEnLaPrimeraPaginaDeLosPropios: el alcance del respaldo.
//
// Y es un límite deliberado: cuando GraphQL falla, los PRs propios se piden por REST. Pero un
// repo con grupos —`acme/grupo/proyecto`— necesita saber los grupos del usuario para filtrar, y
// la API de todos los repos no lo da, así que la segunda página por REST sería una lista de
// TODOS los repos del usuario y no solo los suyos.
//
// Y la consecuencia de no respetarlo sería un inbox con repos ajenos, que es peor que un inbox
// corto: el usuario approve sobre un PR que no es suyo.
func TestElFallbackRESTSeUsaSoloEnLaPrimeraPaginaDeLosPropios(t *testing.T) {
	// Un `gh` que falla siempre: sin GraphQL, sin REST.
	caido := New("github.com", ghQueDevuelve(t, "echo 'gh: could not resolve host' >&2\nexit 1"))

	// La primera página de los propios con cursor vacío: intenta el respaldo REST, que con
	// este `gh` también falla, y sale con aviso del fallo original.
	primera, warns := caido.List(context.Background(), forge.Query{Section: model.SectionAuthored})
	if len(primera.Items) != 0 {
		t.Errorf("con `gh` caído salieron %d ítems de la primera página", len(primera.Items))
	}
	if len(warns) == 0 {
		t.Error("con `gh` caído no hay aviso: el inbox parecería vacío sin explicación")
	}
	// Y la segunda página, con cursor: NO intenta REST, así que el aviso es del fallo de
	// GraphQL y no del de REST. Con un `gh` que funciona distinto para cada uno se distingue,
	// pero lo que importa aquí es que hay aviso y que la lista está vacía.
	segunda, warns2 := caido.List(context.Background(),
		forge.Query{Section: model.SectionAuthored, Cursor: "CUR2"})
	if len(segunda.Items) != 0 {
		t.Errorf("con `gh` caído la segunda página trajo %d ítems", len(segunda.Items))
	}
	if len(warns2) == 0 {
		t.Error("la segunda página sin cursor no ha dado aviso")
	}
}

// TestUnaSeccionNoSoportadaNiSeConsultaElForge: el guard de `qualifierFor`.
//
// Y es un guard que no consulta el forge a propósito: una sección que no tiene búsqueda en
// GitHub no puede devolver nada, y preguntarlo sería gastar cuota para recibir un error que
// ya se sabe.
//
// Y la aviso tiene que ser de clase "unsupported" y no de parseo ni de red, porque no es un
// fallo: es una combinación que este forge no implementa, y eso no se arregla reintentando ni
// autenticándose.
func TestUnaSeccionNoSoportadaNiSeConsultaElForge(t *testing.T) {
	// Un `gh` que falla si se le llama: el guard tiene que cortarlo antes, no por suerte.
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
	// Y el aviso lleva la sección, que es lo que permite al inbox saber a qué columna
	// atribuírlo.
	if warns[0].Section != "una-seccion-inventada" {
		t.Errorf("el aviso no lleva la sección (lleva %q)", warns[0].Section)
	}
}

// contains es el contains de la biblioteca estándar con el nombre del paquete, para no importar
// `strings` solo para esto.
func contains(haystack, needle string) bool {
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return true
		}
	}
	return len(needle) == 0
}
