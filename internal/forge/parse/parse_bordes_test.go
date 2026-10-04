package parse

import (
	"fmt"
	"strings"
	"testing"

	"prdash/internal/forge/model"
)

// Estas son las ramas del parseo que solo se ven con datos que el forge NO manda nunca, y que
// por eso son las que más se cuelan: un estado de check que no es ninguno de los conocidos, un
// `errors` de GraphQL con código de salida 0, una conexión ausente, y un evento de Todos que no
// es una mención.
//
// Y el patrón común es que todas devuelven un valor NEUTRO en vez de un error: se descarta el
// dato raro y se sigue. Y eso es lo correcto —un check con un estado desconocido no puede
// contarse ni como bueno ni como malo—, pero por ser silencioso necesita un test que fije QUÉ
// se descarta, porque "se descartó" y "se contó mal" se ven igual en el resumen de la cabecera.

// TestUnCheckConBucketDesconocidoCaeAlEstadoCrudoYNoSeInventaUnRecuento: `ParseGHChecks`.
//
// Y la jerarquía del parser es lo que hay que fijar: el `bucket` que normaliza `gh` MANDA, y si
// falta se cae al estado crudo. Y el caso que hay que provocar es un bucket que `gh` no emite
// —una acción personalizada, un runner nuevo— con un estado que sí se puede leer.
//
// Y el aserto es sobre el RECUENTO, no sobre que no rompa: un check con estado desconocido que
// se contara como fallido inflaría el número de fallos de la cabecera y el usuario vería un PR
// con checks en rojo que están en verde. Y al revés: se aprobaría un PR con un check roto sin
// que nadie lo viera.
func TestUnCheckConBucketDesconocidoCaeAlEstadoCrudoYNoSeInventaUnRecuento(t *testing.T) {
	for _, c := range []struct {
		nombre      string
		check       string
		wantFail    int
		wantPending int
		wantTotal   int
	}{
		{"bucket pass", `{"name":"ci","bucket":"pass","state":""}`, 0, 0, 1},
		{"bucket skip", `{"name":"ci","bucket":"skipping","state":""}`, 0, 0, 1},
		{"bucket fail", `{"name":"ci","bucket":"fail","state":""}`, 1, 0, 1},
		{"bucket cancel cuenta como fallo", `{"name":"ci","bucket":"cancel","state":""}`, 1, 0, 1},
		{"bucket pending", `{"name":"ci","bucket":"pending","state":""}`, 0, 1, 1},

		// Y aquí el `default`: sin bucket conocido se lee el estado crudo, y lo que cuenta
		// como fallo es `state` a FAILURE o ERROR.
		{"estado FAILURE sin bucket", `{"name":"ci","state":"FAILURE"}`, 1, 0, 1},
		{"estado error sin bucket", `{"name":"ci","state":"ERROR"}`, 1, 0, 1},
		{"estado FAILURE en minúsculas", `{"name":"ci","state":"failure"}`, 1, 0, 1},
		{"estado in_progress sin bucket", `{"name":"ci","state":"IN_PROGRESS"}`, 0, 1, 1},
		{"estado queued sin bucket", `{"name":"ci","state":"QUEUED"}`, 0, 1, 1},

		// Y el caso que el `default` tiene que dejar en cero: ni fallo ni pendiente. Un
		// estado inventado no puede contarse como bueno —eso sería mentir— ni como malo.
		{"estado inventado", `{"name":"ci","state":"timed_out_wtf"}`, 0, 0, 1},
		{"bucket inventado y estado vacío", `{"name":"ci","bucket":"raro","state":""}`, 0, 0, 1},
		{"bucket raro y estado raro", `{"name":"ci","bucket":"raro","state":"raro"}`, 0, 0, 1},

		// Y la asimetría que hay que mirar: el bucket manda sobre el estado aunque se
		// contradigan. Es lo que hace que un check marcado como `skipping` con un `state`
		// heredado de una versión vieja de `gh` no accounted como fallo.
		{"bucket pass con state FAILURE", `{"name":"ci","bucket":"pass","state":"FAILURE"}`, 0, 0, 1},
	} {
		raw := "[" + c.check + "]"
		st, err := ParseGHChecks(raw)
		if err != nil {
			t.Errorf("%s: %v", c.nombre, err)
			continue
		}
		if st.Total != c.wantTotal {
			t.Errorf("%s: Total = %d, want %d", c.nombre, st.Total, c.wantTotal)
		}
		if st.Failing != c.wantFail {
			t.Errorf("%s: Failing = %d, want %d", c.nombre, st.Failing, c.wantFail)
		}
		if st.Pending != c.wantPending {
			t.Errorf("%s: Pending = %d, want %d", c.nombre, st.Pending, c.wantPending)
		}
	}

	// Y el resumen de todos a la vez, que es lo que se pinta: con un check fallido el estado
	// global es "failing" aunque haya muchos en verde, porque el usuario tiene que verlo.
	mezcla := `[{"name":"a","bucket":"pass"},{"name":"b","bucket":"fail"},` +
		`{"name":"c","bucket":"pass"}]`
	st, err := ParseGHChecks(mezcla)
	if err != nil {
		t.Fatal(err)
	}
	if st.State != model.ChecksFailing {
		t.Errorf("con un check fallido el estado global es %v, want failing", st.State)
	}
	if st.Total != 3 || st.Failing != 1 {
		t.Errorf("el resumen es %+v: 3 checks y 1 fallido", st)
	}
}

// TestGHChecksConUnaSalidaVaciaNoInventaChecksNiFalla: el caso de "no hay checks".
//
// Y es el caso que más se da en la práctica: un PR recién abierto no tiene checks todavía y
// `gh pr checks` sale con una lista vacía. El parser tiene que devolver un estado vacío SIN
// error, porque "no hay checks" y "no se pudo consultar" son cosas distintas y el usuario las
// lee distinto.
//
// Y con el estado vacío, que es lo que se pinta: no es "falling" ni "passing" sino lo
// tercero, para que el render no diga que todo está bien cuando en realidad no se sabe.
func TestGHChecksConUnaSalidaVaciaNoInventaChecksNiFalla(t *testing.T) {
	for _, raw := range []string{"[]", "null"} {
		st, err := ParseGHChecks(raw)
		if err != nil {
			t.Errorf("%q dio error: %v", raw, err)
			continue
		}
		if st.Failing != 0 || st.Pending != 0 || st.Total != 0 {
			t.Errorf("%q inventó checks: %+v", raw, st)
		}
	}

	// Y solo con checks en verde, que es el otro extremo.
	st, err := ParseGHChecks(`[{"name":"a","bucket":"pass"},{"name":"b","bucket":"pass"}]`)
	if err != nil {
		t.Fatal(err)
	}
	if st.State != model.ChecksPassing {
		t.Errorf("solo checks en verde dio %v, want passing", st.State)
	}
}

// TestGLGraphQLConErroresDelServidorNoSeConfundeConUnaPaginaVacia: el `errors` de GraphQL.
//
// Y es un caso distinto del JSON inválido, y la diferencia importa porque los dos llegan igual:
// GraphQL responde con CÓDIGO DE SALIDA 0 y un array `errors` en el cuerpo cuando la consulta
// está bien formada pero el schema la rechaza, cuando falta un permiso de campo o cuando hay
// un error de validación en el servidor.
//
// Y el efecto de no distinguirlo es el peor de los dos: el parser leería `data` vacío, no
// inventaría ítems y devolvería una página vacía SIN ERROR, que el adapter pinta como "no hay
// PRs que revisar". El usuario cerraría la sesión creyendo que le han mergeado todo.
func TestGLGraphQLConErroresDelServidorNoSeConfundeConUnaPaginaVacia(t *testing.T) {
	// El mensaje del servidor llega tal cual, que es lo único que distingue "mi consulta ya
	// no vale" de "el forge va lento" —y lo único que dice si hay que esperar o rehacer la
	// consulta—.
	raw := `{"errors":[{"message":"Field 'mergeRequest' doesn't exist on type 'Project'",` +
		`"type":"undefinedField","path":["project"]}]}`
	items, page, err := ParseGLGraphQL(raw)
	if err == nil {
		t.Fatal("unos errores de GraphQL dieron nil: el adapter los trataría como un inbox vacío")
	}
	if len(items) != 0 {
		t.Errorf("salieron %d ítems de una respuesta que solo trae errores", len(items))
	}
	if page.More || page.Next != "" {
		t.Errorf("la paginación salió con datos: %+v", page)
	}
	if !strings.Contains(err.Error(), "mergeRequest") {
		t.Errorf("el error %q no trae el mensaje del servidor", err)
	}

	// Y el caso de control: JSON roto SÍ es un error de parseo, y los dos son errores pero
	// de clases distintas. Un aserto de "da error" para los dos no distinguiría nada.
	if _, _, err := ParseGLGraphQL("no soy json"); err == nil ||
		!strings.Contains(err.Error(), "invalid JSON") {
		t.Errorf("un JSON roto dio %v, want un error de JSON inválido", err)
	}
}

// TestGLGraphQLConUnaConexionAusenteNoSeFallaNiSePierdeLoDemas: `glItems` con la conexión a nil.
//
// Y la respuesta de una consulta de listado trae las TRES conexiones —authored, review
// requested, review assigned— porque la consulta las pide todas para no hacer tres llamadas.
// Pero una conexión puede no estar: un token sin el scope de un permiso concreto hace que el
// servidor devuelva `null` en ese campo y `data` en los demás.
//
// Y lo que hay que comprobar es que la conexión ausente aporta CERO ítems y no un error, y que
// las otras siguen dando los suyos: tirar la página entera por un campo que no se pudo leer
// dejaría al usuario con menos PRs de los que tiene, sin ninguna explicación.
func TestGLGraphQLConUnaConexionAusenteNoSeFallaNiSePierdeLoDemas(t *testing.T) {
	const mr = `{"iid":12,"title":"MR","webUrl":"u","state":"opened",` +
		`"sourceBranch":"feat/a","targetBranch":"main",` +
		`"updatedAt":"2026-09-21T07:00:00Z","author":{"username":"me"}}`
	const conn = `{"pageInfo":{"hasNextPage":false,"endCursor":null},"nodes":[%s]}`

	// Solo la de review solicitada; las otras dos a null.
	raw := fmt.Sprintf(
		`{"data":{"currentUser":{"authoredMergeRequests":null,`+
			`"reviewRequestedMergeRequests":%s,`+
			`"assignedMergeRequests":null}}}`,
		fmt.Sprintf(conn, mr))
	items, _, err := ParseGLGraphQL(raw)
	if err != nil {
		t.Fatalf("una conexión ausente dio error: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("salieron %d ítems con una conexión a null, want 1", len(items))
	}
	if items[0].Number != 12 {
		t.Errorf("el ítem es %+v", items[0])
	}

	// Y el caso de control: las TRES a null dan una página vacía y ningún error, que es lo
	// que el adapter traduce en "no hay nada".
	vacias := `{"data":{"currentUser":{"authoredMergeRequests":null,` +
		`"reviewRequestedMergeRequests":null,"assignedMergeRequests":null}}}`
	if items, _, err = ParseGLGraphQL(vacias); err != nil || len(items) != 0 {
		t.Errorf("tres conexiones a null dieron %d ítems y %v", len(items), err)
	}

	// Y sin `data` en absoluto, que es lo mismo: `currentUser` a nil es no traer el campo.
	sinData := `{"data":{}}`
	if items, _, err = ParseGLGraphQL(sinData); err != nil || len(items) != 0 {
		t.Errorf("sin `data` dio %d ítems y %v", len(items), err)
	}

	// Y la paginación se toma de la PRIMERA conexión que hay, que es lo que evita que una
	// conexión ausente deje la paginación en "no hay más" con contenido sin mostrar.
	conDos := fmt.Sprintf(
		`{"data":{"currentUser":{"authoredMergeRequests":null,`+
			`"reviewRequestedMergeRequests":%s,`+
			`"assignedMergeRequests":%s}}}`,
		fmt.Sprintf(`{"pageInfo":{"hasNextPage":true,"endCursor":"CUR2"},"nodes":[%s]}`, mr),
		`{"pageInfo":{"hasNextPage":false,"endCursor":null},"nodes":[]}`)
	_, page, err := ParseGLGraphQL(conDos)
	if err != nil {
		t.Fatal(err)
	}
	if !page.More || page.Next != "CUR2" {
		t.Errorf("con una conexión a null antes de la que trae items, la paginación es %+v: "+
			"se perdió el cursor y el inbox se trunca", page)
	}
}

// TestGLTodosIgnoraLoQueNoEsUnaMension: el filtro de acciones.
//
// Y la API de Todos trae TODOS los eventos de un usuario en un repo, no solo las menciones: la
// API incluye sus propios push, sus cambios de etiqueta, sus comentarios. Si el parser no
// filtrara, la columna de menciones enseñaría los push del propio usuario como si fueran
// review requests.
//
// Y los dos criterios son necesarios por motivos distintos: `TargetType` descarta los que
// apuntan a un issue —una mención en un issue no es un PR que revisar— y `ActionName` descarta
// las acciones sobre el propio repo. Y los dos juntos, porque hay acciones "mentioned" que
// apuntan a un issue y no sirven para nada en esta columna.
func TestGLTodosIgnoraLoQueNoEsUnaMension(t *testing.T) {
	for _, c := range []struct {
		nombre string
		accion string
		tipo   string
		iid    int
		want   int
	}{
		{"mención a un MR", "mentioned", "MergeRequest", 12, 1},
		{"mención directa a un MR", "directly_addressed", "MergeRequest", 13, 1},
		{"push propio", "pushed", "MergeRequest", 14, 0},
		{"cambio de etiqueta", "update", "MergeRequest", 15, 0},
		{"asignado a un MR", "assigned", "MergeRequest", 19, 0},
		{"mención a un ISSUE", "mentioned", "Issue", 16, 0},
		{"mención directa a un ISSUE", "directly_addressed", "Issue", 17, 0},
		{"acción vacía", "", "MergeRequest", 18, 0},
		{"tipo vacío", "mentioned", "", 20, 0},
		{"iid a cero", "mentioned", "MergeRequest", 0, 0},
	} {
		// Y el `iid` va dentro del target: el parser lo exige para construir el ítem, y
		// sin él la respuesta se descarta entera. Mi primera versión lo puso en la
		// referencia —que es donde GitLab lo escribe en la URL— y ninguna mención llegaba
		// al filtro de verdad.
		raw := fmt.Sprintf(
			`[{"action_name":%q,"target_type":%q,`+
				`"target":{"iid":%d,"references":{"full":"grp/sub/proy!%d"}}}]`,
			c.accion, c.tipo, c.iid, c.iid)

		items, total, err := ParseGLTodos(raw)
		if err != nil {
			t.Errorf("%s: %v", c.nombre, err)
			continue
		}
		if len(items) != c.want {
			t.Errorf("%s: %d ítems, want %d", c.nombre, len(items), c.want)
		}
		// Y el total que dice el forge es el de la RESPUESTA, no el de las menciones
		// filtradas: la columna enseña "3 de 27" y 27 es lo que hay en la API. Contar
		// después del filtro haría que la columna mintiera sobre el tamaño de la lista.
		if total != 1 {
			t.Errorf("%s: total = %d, want 1 (los eventos de la respuesta)", c.nombre, total)
		}
	}

	// Y una lista vacía: la columna queda en cero, sin error.
	items, total, err := ParseGLTodos("[]")
	if err != nil || len(items) != 0 || total != 0 {
		t.Errorf("una lista vacía dio %d ítems, total %d y %v", len(items), total, err)
	}
}

// TestUnNodoDeGitLabSinIidSeDescartaYLosDemasSiguen: el `continue` de `glItems`.
//
// Y es un caso real y no un dato inventado: la API de Todos devuelve eventos cuyo `target` no
// es un MR aunque el filtro lo diga —un evento de un recurso borrado, un `target` que el
// proyecto ya no expone— y el `iid` llega a cero o ausente. El `flexInt` del parser acepta las
// dos formas, así que el nodo entra en el bucle con `IID == 0`.
//
// Y lo que hay que comprobar es que el nodo se descarta SIN_THROW y sin arrastrar a los demás. Un
// `continue` que no existiera —o que fuera un `return`— daría dosBn后果 distintos: o un ítem con
// número cero, que en la columna es indistinguible del primer PR real; o una lista vacía, que es
// "no hay menciones" para un repo que sí las tiene.
//
// Y el 0 tiene que salir de verdad del parser: el número se compone en el `ID`, y un `ID` con
// número cero colisiona con el de otro ítem del mismo repo.
func TestUnNodoDeGitLabSinIidSeDescartaYLosDemosSiguen(t *testing.T) {
	// Un nodo con `iid` ausente del todo, que es como llega cuando el recurso ya no existe.
	sinIID := `{"title":"MR borrado","webUrl":"u","state":"opened",` +
		`"sourceBranch":"feat/a","targetBranch":"main","author":{"username":"alguien"}}`
	// Y uno con `iid` explícitamente a cero, que es la otra forma del mismo problema.
	conCero := `{"iid":0,"title":"otro borrado","webUrl":"u","state":"opened",` +
		`"sourceBranch":"feat/b","targetBranch":"main","author":{"username":"otro"}}`
	bueno := `{"iid":12,"title":"MR","webUrl":"u","state":"opened",` +
		`"sourceBranch":"feat/c","targetBranch":"main",` +
		`"updatedAt":"2026-09-21T07:00:00Z","author":{"username":"me"}}`

	for _, c := range []struct {
		nombre string
		nodos  string
		want   int
	}{
		{"los tres, uno válido", fmt.Sprintf(`[%s,%s,%s]`, sinIID, conCero, bueno), 1},
		{"solo el válido", "[" + bueno + "]", 1},
		{"todos inválidos", fmt.Sprintf(`[%s,%s]`, sinIID, conCero), 0},
		{"lista vacía", "[]", 0},
	} {
		raw := `{"data":{"currentUser":{"reviewRequestedMergeRequests":` +
			`{"pageInfo":{"hasNextPage":false,"endCursor":null},"nodes":` +
			c.nodos + `}}}}`
		items, _, err := ParseGLGraphQL(raw)
		if err != nil {
			t.Errorf("%s: %v", c.nombre, err)
			continue
		}
		if len(items) != c.want {
			t.Errorf("%s: %d ítems, want %d", c.nombre, len(items), c.want)
		}
		// Y ninguno de los que salen lleva número cero, que es lo que lo haría
		// colisionar con el primer ítem del repo en el `ID`.
		for _, it := range items {
			if it.Number == 0 {
				t.Errorf("%s: salió un ítem con número 0: %+v", c.nombre, it)
			}
		}
		// Y el que sobrevive es el bueno, no el primero de la lista: un nodo inválido al
		// principio no puede desplazar la selección del resto.
		if c.want == 1 && items[0].Number != 12 {
			t.Errorf("%s: el ítem que sobrevive es el %d, want el 12", c.nombre, items[0].Number)
		}
	}
}
