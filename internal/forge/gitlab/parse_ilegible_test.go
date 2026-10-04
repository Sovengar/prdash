package gitlab

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"prdash/internal/forge"
	"prdash/internal/forge/model"
)

// GitLab tiene una salida más que GitHub y es la que importa: **una consulta que devuelve
// una lista vacía no es lo mismo que una consulta que no se pudo entender**.
//
// Y aquí no es una sutileza. `ItemState` se llama tras cada acción sobre un MR, y devuelve
// `(Item, []Warning)`. Si el MR ya no existe —fusionado por otra persona mientras se miraba,
// borrado, o un número que nunca existió— la respuesta de GraphQL es una lista VACÍA y
// legítima, no un error. Reconocerla como "no encontrado" es lo que evita que la TUI siga
// enseñando una ficha que ya no existe y que un approve se reintente contra un MR cerrado.
//
// Y el otro lado: una lista vacía cuando el proyecto no existe o no se tiene permiso es
// TAMBIÉN una lista vacía, y por eso la respuesta tiene que ser un aviso de "no encontrado"
// con el proyecto en el mensaje. La diferencia entre los dos casos no está en los datos que
// llegan sino en lo que el usuario necesita leer, y por eso el mensaje nombra el proyecto y
// el número.

// glabQueDevuelve construye un `glab` falso que contesta lo que se le pase. Sin registro de
// argumentos, porque en estos casos lo que importa es la salida.
func glabQueDevuelve(t *testing.T, cuerpo string) string {
	t.Helper()
	dir := t.TempDir()
	return writeScript(t, dir, "glab", "#!/bin/sh\n"+cuerpo+"\n")
}

// glabQueImprime construye el cuerpo de un `glab` falso que escribe el texto dado y sale con
// 0.
//
// Y va por `printf` y no por un heredoc porque el heredoc depende de que el terminador quede al
// principio de una línea, y con un JSON partido en varios literales de Go es fácil que no pase.
// El síntoma —"unexpected end of JSON input" con el JSON perfectamente válido—apunta al shell, no
// al parser, y el aserto de "clase parse" lo confirma sin decir por qué.
func glabQueImprime(salida string) string {
	// Comillas SIMPLES alrededor: con dobles, las del propio JSON las cierran y el shell
	// entrega el texto sin ellas —el síntoma es "invalid character 'd'", que apunta al
	// shell y no al parser—. El JSON de la API no lleva comillas simples, así que no hay
	// que escapar nada.
	return "printf '%s\\n' '" + salida + "'"
}

func avisoUnico(t *testing.T, warns []model.Warning, donde string) model.Warning {
	t.Helper()
	if len(warns) != 1 {
		t.Fatalf("%s: %d avisos, want 1: %+v", donde, len(warns), warns)
	}
	return warns[0]
}

// graphqlVacio es la respuesta de GraphQL que devuelve `ItemState` cuando el MR no existe:
// la forma EXACTA que pide `glMRQuery`, con `mergeRequest` a null.
//
// Y el detalle de por qué va por esa forma y no por la del listado —`currentUser` con una
// lista de `reviewRequestedMergeRequests`— es que `ParseGLGraphQL` lee el primer `mergeRequest`
// que encuentra y el parser de un listado no es el mismo camino. La respuesta del listado no
// servía, y el test pasaba sin cubrir la rama que quería: el aviso salía de la falta de parseo
// en vez de la de "no encontrado". Un fixture que no es el de la consulta bajo prueba es un
// fixture que prueba otra cosa.
const graphqlVacio = `{"data":{"project":{"mergeRequest":null}}}`

// TestUnMRQueNoExisteSeDiceQueNoExisteYNoDevuelveUnItemFalso: la salida que no tiene GitHub.
//
// Y el caso es el que pasa en producción: el usuario mira un MR, alguien lo fusiona desde el
// navegador, y la TUI refresca ese ítem. El MR ya no está en la búsqueda, la lista viene vacía,
// y lo que hay que devolver es "no encontrado" —no un parseo fallido y no un MR en blanco.
//
// Y el mensaje tiene que nombrar el proyecto Y el número. "MR no encontrado" a secas deja a
// quien lee sin saber si el problema es ese MR o la sección entera, que es la duda real
// cuando hay veinte líneas en pantalla.
func TestUnMRQueNoExisteSeDiceQueNoExisteYNoDevuelveUnItemFalso(t *testing.T) {
	a := New("gitlab.example.com", glabQueDevuelve(t, "cat <<'JSON'\n"+graphqlVacio+"\nJSON"))

	ref := model.RepoRef{Forge: "gitlab", Host: "gitlab.example.com", Project: "grupo/proyecto"}
	it, warns := a.ItemState(context.Background(), ref, 404)

	w := avisoUnico(t, warns, "MR inexistente")
	if w.Kind != "notfound" {
		t.Errorf("clase %q, want notfound: un MR que no existe no es un error de parseo ni "+
			"un rate limit", w.Kind)
	}
	// Y el mensaje dice cuál: sin el número, el aviso podría ser de cualquiera de los MRs
	// de la lista y el usuario no sabría cuál desapareció.
	for _, quiere := range []string{"404", "grupo/proyecto"} {
		if !strings.Contains(w.Msg, quiere) {
			t.Errorf("el aviso %q no menciona %q", w.Msg, quiere)
		}
	}
	// Y el ítem es el valor cero. Un `Item` con el número puesto y el resto vacío se
	// pintaría como una línea casi normal, y un approve contra él iría al forge con un
	// HeadSHA vacío.
	if it.ID() != (model.ID{}) || it.Number != 0 || it.Title != "" || it.HeadSHA != "" {
		t.Errorf("devolvió un ítem a medias en vez del valor cero: %+v", it)
	}
	// Y el aviso no lleva sección: `ItemState` no pertenece a ninguna columna del inbox, se
	// atribuye al ítem que se está refrescando.
	if w.Section != "" {
		t.Errorf("el aviso lleva sección %q y aparecería además en el inbox", w.Section)
	}
}

// TestUnaSalidaQueNoEsJSONEnGitLabAvisaYNoSeRompe: el mismo contrato que en GitHub.
//
// Y el detalle del heredoc no es cosmético: con `echo` y un JSON truncado el script queda con
// una comilla suelta, lo que falla es el SHELL y no el parseo, y el aviso sale de clase
// "network" —que es correcta— en lugar de "parse". Un aserto de "una salida ilegible da
// parse" no distingue los dos fallos, que son fixtures distintas.
func TestUnaSalidaQueNoEsJSONEnGitLabAvisaYNoSeRompe(t *testing.T) {
	for _, c := range []struct {
		nombre string
		cuerpo string
	}{
		{"html de un proxy", "echo '<html>Sign in to continue</html>'"},
		{"json truncado", "cat <<'JSON'\n{\"data\":{\"currentUser\":\nJSON"},
		{"json vacio", "printf ''"},
		{"lista en vez de objeto", "echo '[1,2,3]'"},
	} {
		a := New("gitlab.example.com", glabQueDevuelve(t, c.cuerpo))
		ref := model.RepoRef{Forge: "gitlab", Host: "gitlab.example.com", Project: "grupo/proy"}
		it, warns := a.ItemState(context.Background(), ref, 7)

		if w := avisoUnico(t, warns, c.nombre); w.Kind != "parse" {
			t.Errorf("%s: clase %q, want parse", c.nombre, w.Kind)
		}
		if it.ID() != (model.ID{}) || it.Number != 0 {
			t.Errorf("%s: devolvió un ítem a medias", c.nombre)
		}
	}
}

// TestListarConSalidaIlegibleAvisaYNoInventaItems: el listado de GraphQL y el de Todos.
//
// Y son DOS funciones con el mismo contrato y por eso van en tabla: `graphqlList` es el
// camino normal y `todosList` es el de las menciones, que usa la API de Todos con un cursor
// de página. El riesgo de que una de las dos se quede sin probar es alto porque el código es
// casi idéntico y leerlo da la impresión de que está cubierto.
func TestListarConSalidaIlegibleAvisaYNoInventaItems(t *testing.T) {
	a := New("gitlab.example.com", glabQueDevuelve(t, "echo 'no soy json'"))

	for _, c := range []struct {
		nombre string
		q      forge.Query
	}{
		{"graphql de review", forge.Query{Section: model.SectionReview, ReviewKind: model.ReviewRequested}},
		{"graphql de menciones", forge.Query{Section: model.SectionMentions}},
		{"graphql de authored", forge.Query{Section: model.SectionAuthored}},
	} {
		page, warns := a.List(context.Background(), c.q)
		if len(page.Items) != 0 {
			t.Errorf("%s: %d ítems de una salida ilegible", c.nombre, len(page.Items))
		}
		w := avisoUnico(t, warns, c.nombre)
		if w.Kind != "parse" {
			t.Errorf("%s: clase %q, want parse", c.nombre, w.Kind)
		}
		if w.Section != c.q.Section {
			t.Errorf("%s: el aviso no lleva la sección consultada (%q)", c.nombre, w.Section)
		}
	}
}

// TestLaConversacionDeGitLabConSalidaIlegibleNoInventaNotas: `Comments`.
//
// Y el riesgo aquí es el mismo que en GitHub pero con una diferencia: una nota de sistema
// mal parseada se pintaría en la conversación. La ficha de un MR se lee para decidir si se
// aprueba, y un comentario inventado con el nombre de otra persona es el tipo de cosa que no
// se detecta leyendo.
func TestLaConversacionDeGitLabConSalidaIlegibleNoInventaNotas(t *testing.T) {
	a := New("gitlab.example.com", glabQueDevuelve(t, "echo 'rompido'"))
	ref := model.RepoRef{Forge: "gitlab", Host: "gitlab.example.com", Project: "grupo/proyecto"}

	page, warns := a.Comments(context.Background(), ref, 12)

	if w := avisoUnico(t, warns, "Comments"); w.Kind != "parse" {
		t.Errorf("clase %q, want parse", w.Kind)
	}
	if len(page.Comments) != 0 {
		t.Errorf("%d notas inventadas de una salida ilegible", len(page.Comments))
	}
	if page.Total != 0 {
		t.Errorf("Total = %d con una salida ilegible", page.Total)
	}
}

// TestUnProyectoVacioSe DiceQueNoYNoSaleAConsultarElForge: la guarda de entrada.
//
// Y las tres operaciones la tienen —`ItemState`, `Comments` y la de notas— porque un proyecto
// vacío es un ítem con datos a medias: el forge devuelve una URL sin repo y `glab` contesta
// con un error de sintaxis en lugar de un 404, que es un error mucho más difícil de leer que
// un aviso que diga "no hay repositorio".
func TestUnProyectoVacioSeDiceQueNoYNoSaleAConsultarElForge(t *testing.T) {
	// Un `glab` que FALLA si se le llama: la guarda tiene que cortarlo antes, y no por
	// suerte. Con un `glab` que contesta cualquier cosa, un `ItemState` de un proyecto vacío
	// devolvería "no encontrado" y el test pasaría sin haber comprobado la guarda.
	a := New("gitlab.example.com", glabQueDevuelve(t,
		"echo 'glab no debería haberme llamado' >&2\nexit 1"))
	ref := model.RepoRef{Forge: "gitlab", Host: "gitlab.example.com", Project: ""}

	it, warns := a.ItemState(context.Background(), ref, 12)
	if w := avisoUnico(t, warns, "ItemState sin proyecto"); w.Kind != "notfound" {
		t.Errorf("clase %q, want notfound", w.Kind)
	}
	if it.ID() != (model.ID{}) {
		t.Errorf("devolvió un ítem sin proyecto: %+v", it)
	}

	page, warns := a.Comments(context.Background(), ref, 12)
	if w := avisoUnico(t, warns, "Comments sin proyecto"); w.Kind != "notfound" {
		t.Errorf("clase %q, want notfound", w.Kind)
	}
	if len(page.Comments) != 0 || page.Total != 0 {
		t.Errorf("devolvió comentarios sin proyecto: %+v", page)
	}

	// Y el aviso no lleva la URL del binario, que es ruido de una máquina concreta en un
	// mensaje que se lee en la cabecera del inbox.
	for _, w := range warns {
		if strings.Contains(w.Msg, "glab") {
			t.Errorf("el aviso menciona el binario, que no le dice nada al usuario: %q", w.Msg)
		}
	}
}

// TestUnBinarioInexistenteSeDiceQueNoYNoSeIntentaEjecutar: la degradación del entorno.
//
// Y la comprobación es que el aviso NO menciona el binario, por la misma razón que arriba: el
// mensaje se enseña al usuario y "fork/exec /home/tu/.local/bin/glab: no such file" es un
// mensaje de una máquina, no una explicación. Lo que el usuario necesita es "no hay glab".
func TestUnBinarioInexistenteSeDiceQueNoYNoSeIntentaEjecutar(t *testing.T) {
	ausente := filepath.Join(t.TempDir(), "no-hay-glab")
	a := New("gitlab.example.com", ausente)
	ref := model.RepoRef{Forge: "gitlab", Host: "gitlab.example.com", Project: "grupo/proyecto"}

	_, warns := a.ItemState(context.Background(), ref, 12)
	w := avisoUnico(t, warns, "sin glab")
	if w.Kind == "" {
		t.Error("sin binario no hay aviso: el inbox parecería vacío sin explicación")
	}
	if !strings.Contains(w.Msg, "glab") {
		t.Errorf("el aviso %q no dice que falta glab, que es lo accionable", w.Msg)
	}
}

// TestLeerElEstadoDeUnMRQueExisteLoDevuelveEstampadoConSuIdentidad: el camino bueno.
//
// Y estaba sin probar entero, que es un agujero raro: el resto del adapter sí lo estaba, y este
// es el camino que se ejecuta tras CADA acción sobre un MR. Lo que sale de aquí se aplica
// encima del ítem que ya está en pantalla, así que lo que tiene que traer es lo completo.
//
// Y la palabra "estampado" es lo que hay que mirar, porque `identity` es lo que hace que el
// ítem sepa de qué forge y de qué host viene. Sin ella el MR se pintaría pero un approve
// posterior iría al adapter equivocado en la 다루ной de la lista: el `ID` se compone del
// forge y del host, así que sin estampar el ID es de otro repo.
func TestLeerElEstadoDeUnMRQueExisteLoDevuelveEstampadoConSuIdentidad(t *testing.T) {
	const mrVivo = `{"data":{"project":{"mergeRequest":{"iid":12,"title":"Un MR",` +
		`"webUrl":"https://gitlab.acme.example/g/p/-/merge_requests/12","state":"opened",` +
		`"draft":false,"detailedMergeStatus":"mergeable","sourceBranch":"feat/a",` +
		`"targetBranch":"main","approved":false,"updatedAt":"2026-09-21T07:00:00Z",` +
		`"diffHeadSha":"abc123"}}}}`
	a := New("gitlab.acme.example", glabQueDevuelve(t, glabQueImprime(mrVivo)))
	ref := model.RepoRef{Forge: "gitlab", Host: "gitlab.acme.example", Project: "grupo/proyecto"}

	it, warns := a.ItemState(context.Background(), ref, 12)

	if len(warns) != 0 {
		t.Fatalf("un MR que existe dio %d avisos: %+v", len(warns), warns)
	}
	if it.Number != 12 {
		t.Errorf("Number = %d, want 12", it.Number)
	}
	if it.Title != "Un MR" || it.SourceBranch != "feat/a" || it.TargetBranch != "main" {
		t.Errorf("el MR llegó incompleto: %+v", it)
	}
	// Y el HeadSHA, que es lo que permite pinear el merge. Sin él, el merge saldría sin
	// `--sha` y podría integrar commits que nadie miró.
	if it.HeadSHA != "abc123" {
		t.Errorf("HeadSHA = %q, want abc123: sin él el merge no se puede pinear", it.HeadSHA)
	}
	// Y la identidad estampada en los cuatro sitios donde se mira: el ítem, y el RepoRef que
	// lo identifica. Es la conexión entre "este MR es de gitlab" y "este MR es de ESTE host".
	if it.Forge != "gitlab" || it.Host != "gitlab.acme.example" {
		t.Errorf("identidad en el Item = %s/%s, want gitlab/gitlab.acme.example",
			it.Forge, it.Host)
	}
	if it.Ref.Forge != "gitlab" || it.Ref.Host != "gitlab.acme.example" {
		t.Errorf("identidad en el Ref = %s/%s", it.Ref.Forge, it.Ref.Host)
	}
	// Y el ID lleva la identidad.
	if it.ID().Forge != "gitlab" || it.ID().Host != "gitlab.acme.example" {
		t.Errorf("el ID no lleva la identidad: %+v", it.ID())
	}

	// Y lo que `identity` NO hace es sellar el proyecto, y es lo mismo que hace GitHub —las
	// dos funciones `identity` son idénticas, línea por línea—. Mi primera versión afirmaba
	// que el Ref traía el proyecto y falló: no lo trae, y no es un olvido.
	//
	// El proyecto viene del ítem que ya está en pantalla, porque el resultado de `ItemState`
	// se APLICA encima de ese ítem y no lo sustituye. Un `ItemState` que sellara el proyecto
	// con el que se le llamó no cambiaría nada —es el mismo—, pero daría la impresión de que
	// el ítem es autónomo, y un ítem que se usa suelto llevaría el proyecto vacío y un ID
	// que no colisiona con nada.
	if it.Ref.Project != "" {
		t.Errorf("identity selló el proyecto (%q): el proyecto viene del ítem sobre el que se "+
			"aplica, y sellarlo aquí daría a entender que el Item es autónomo", it.Ref.Project)
	}

	// Y el inverso: un host DISTINTO da un ID distinto, que es lo que impide que dos
	// instancias self-managed con el mismo path de proyecto se pisen en la memoria.
	otro := New("otro.acme.example", glabQueDevuelve(t, glabQueImprime(
		`{"data":{"project":{"mergeRequest":{"iid":12,"title":"Otro host","webUrl":"u",`+
			`"state":"opened","sourceBranch":"feat/a","targetBranch":"main",`+
			`"updatedAt":"2026-09-21T07:00:00Z"}}}}`)))
	it2, warns := otro.ItemState(context.Background(), ref, 12)
	if len(warns) != 0 {
		t.Fatalf("el segundo adapter avisa: %+v", warns)
	}
	if it2.ID() == it.ID() {
		t.Error("el mismo MR en dos hosts dio el mismo ID: la memoria de reviews los confundiría")
	}
	if it2.HeadSHA != "" {
		t.Errorf("HeadSHA = %q sin diffHeadSha: debe quedar vacío para que el merge NO se "+
			"pinee con un SHA inventado", it2.HeadSHA)
	}
}
