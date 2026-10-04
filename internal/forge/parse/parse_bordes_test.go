package parse

import (
	"fmt"
	"strings"
	"testing"

	"prdash/internal/forge/model"
)

// The parsing branches that only show up with data the forge NEVER sends, which is why they are the
//ones most likely to rot.

// The hierarchy of the parse.
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

		// The `default`: with no known bucket the raw state is read, and the counting branch follows it.
		{"estado FAILURE sin bucket", `{"name":"ci","state":"FAILURE"}`, 1, 0, 1},
		{"estado error sin bucket", `{"name":"ci","state":"ERROR"}`, 1, 0, 1},
		{"estado FAILURE en minúsculas", `{"name":"ci","state":"failure"}`, 1, 0, 1},
		{"estado in_progress sin bucket", `{"name":"ci","state":"IN_PROGRESS"}`, 0, 1, 1},
		{"estado queued sin bucket", `{"name":"ci","state":"QUEUED"}`, 0, 1, 1},

		// And the case the default must leave at zero: neither failure nor pending.
		{"estado inventado", `{"name":"ci","state":"timed_out_wtf"}`, 0, 0, 1},
		{"bucket inventado y estado vacío", `{"name":"ci","bucket":"raro","state":""}`, 0, 0, 1},
		{"bucket raro y estado raro", `{"name":"ci","bucket":"raro","state":"raro"}`, 0, 0, 1},

		// The asymmetry to watch: the bucket wins over the state even when they contradict each other.
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

// The commonest "no checks" case.
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

	st, err := ParseGHChecks(`[{"name":"a","bucket":"pass"},{"name":"b","bucket":"pass"}]`)
	if err != nil {
		t.Fatal(err)
	}
	if st.State != model.ChecksPassing {
		t.Errorf("solo checks en verde dio %v, want passing", st.State)
	}
}

// A different case from an empty page.
func TestGLGraphQLConErroresDelServidorNoSeConfundeConUnaPaginaVacia(t *testing.T) {
	// The server's message comes through verbatim, and it is the only thing that tells "my query
	// already ran" from a real failure.
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

	// The control: broken JSON IS a parse error, and both are errors but of different classes.
	if _, _, err := ParseGLGraphQL("no soy json"); err == nil ||
		!strings.Contains(err.Error(), "invalid JSON") {
		t.Errorf("un JSON roto dio %v, want un error de JSON inválido", err)
	}
}

// The response carries neither data nor errors.
func TestGLGraphQLConUnaConexionAusenteNoSeFallaNiSePierdeLoDemas(t *testing.T) {
	const mr = `{"iid":12,"title":"MR","webUrl":"u","state":"opened",` +
		`"sourceBranch":"feat/a","targetBranch":"main",` +
		`"updatedAt":"2026-09-21T07:00:00Z","author":{"username":"me"}}`
	const conn = `{"pageInfo":{"hasNextPage":false,"endCursor":null},"nodes":[%s]}`

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

	vacias := `{"data":{"currentUser":{"authoredMergeRequests":null,` +
		`"reviewRequestedMergeRequests":null,"assignedMergeRequests":null}}}`
	if items, _, err = ParseGLGraphQL(vacias); err != nil || len(items) != 0 {
		t.Errorf("tres conexiones a null dieron %d ítems y %v", len(items), err)
	}

	sinData := `{"data":{}}`
	if items, _, err = ParseGLGraphQL(sinData); err != nil || len(items) != 0 {
		t.Errorf("sin `data` dio %d ítems y %v", len(items), err)
	}

	// Pagination is taken from the FIRST connection there is, which stops a second one from
	// overwriting it.
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

// The Todos API brings EVERY event a user has, so the action filter is what makes it a
// mentions list.
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
		// The iid is inside the target: the parser requires it to build the item.
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
		// The total the forge reports is the RESPONSE's, not the filtered mentions': a count that
		// contradicts the column is not a bug.
		if total != 1 {
			t.Errorf("%s: total = %d, want 1 (los eventos de la respuesta)", c.nombre, total)
		}
	}

	items, total, err := ParseGLTodos("[]")
	if err != nil || len(items) != 0 || total != 0 {
		t.Errorf("una lista vacía dio %d ítems, total %d y %v", len(items), total, err)
	}
}

// A real case, not invented data.
func TestUnNodoDeGitLabSinIidSeDescartaYLosDemosSiguen(t *testing.T) {
	sinIID := `{"title":"MR borrado","webUrl":"u","state":"opened",` +
		`"sourceBranch":"feat/a","targetBranch":"main","author":{"username":"alguien"}}`
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
		// None of the survivors carries number zero, which is what would collide with the others.
		for _, it := range items {
			if it.Number == 0 {
				t.Errorf("%s: salió un ítem con número 0: %+v", c.nombre, it)
			}
		}
		// The survivor is the good one, not the first: an invalid node at the front would win otherwise.
		if c.want == 1 && items[0].Number != 12 {
			t.Errorf("%s: el ítem que sobrevive es el %d, want el 12", c.nombre, items[0].Number)
		}
	}
}
