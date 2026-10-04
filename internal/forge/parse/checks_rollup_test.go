package parse

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"prdash/internal/forge/model"
)

// La agregación de checks de GitHub es el punto donde un dato de la forge se convierte en
// una señal que la TUI usa para decidir si un PR está bloqueado. Y tiene tres estados que
// compiten entre sí —falla, pendiente, pasa— y una precedencia entre ellos que es una
// decisión, no un detalle: si hay uno que falla, el PR está bloqueado, y da igual que otros
// sigan corriendo.
//
// Y el cuarto camino, el que casi nadie mira, es el que este fichero fija: cuando el
// rollup no trae NINGÚN contexto, pero sí trae un estado global. Sin él, un PR cuyo único
// check es un status check antiguo —sin contexto asociado— saldría como "no se sabe", que
// es indistinguible de "el forge no lo ha dicho".

// TestUnRollupSinContextosUsaElEstadoGlobal: el cuarto camino.
//
// Y es un caso real, no un borde: los status checks clásicos de GitHub —los que aparecían antes de las GitHub Actions— no tienen contexto asociado. El rollup llega con
// `Contexts.Nodes` vacío y con un `state` global que si dice algo.
func TestUnRollupSinContextosUsaElEstadoGlobal(t *testing.T) {
	casos := []struct {
		nombre string
		estado string
		want   model.CheckState
	}{
		{"success", "SUCCESS", model.ChecksPassing},
		{"failure", "FAILURE", model.ChecksFailing},
		{"error", "ERROR", model.ChecksFailing},
		{"pending", "PENDING", model.ChecksPending},
		{"expected", "EXPECTED", model.ChecksPending},
		// Y en minúsculas, porque el estado global de GitHub lo manda en mayúsculas pero
		// no en todas las rutas de la API.
		{"success en minusculas", "success", model.ChecksPassing},
		// Lo que no se reconoce: desconocido. Un estado global raro no se traduce a
		// "pasando", que es lo que haría un default permisivo y lo que dejaría mergear un
		// PR cuyo CI no se sabe.
		{"estado raro", "NEUTRAL", model.ChecksUnknown},
		{"vacio", "", model.ChecksUnknown},
	}
	for _, c := range casos {
		if got := checksStateFromRollup(c.estado); got != c.want {
			t.Errorf("%s: checksStateFromRollup(%q) dio %q, want %q", c.nombre, c.estado, got, c.want)
		}
	}
	// Y el agregador entero: un rollup sin contextos con estado global se traduce al mismo
	// estado, con el total a cero —porque no hay contextos que contar, y un total inventado
	// saldría como "+3 checks" de nada—.
	// Y el agregado entero: un rollup sin contextos con estado global se traduce al mismo
	// estado, con el total a cero —porque no hay contextos que contar, y un total inventado
	// saldría como "2 checks" de nada—.
	for _, c := range casos {
		got := checksDeUnSearch(t, commitConRollup(map[string]any{"state": c.estado}))
		if got.State != c.want {
			t.Errorf("%s: dio %q, want %q", c.nombre, got.State, c.want)
		}
		if got.Total != 0 {
			t.Errorf("%s: dio Total=%d sin contextos, want 0", c.nombre, got.Total)
		}
	}
}

// TestLosContextosMandanSobreElEstadoGlobal: la precedencia.
//
// Y es la precedencia correcta por una razón concreta: el estado global es un resumen que
// GitHub calcula, y los contextos son los datos. Si un contexto dice que uno falla y el
// global dice SUCCESS, el que manda es el contexto —porque es el que se puede citar— y el
// PR está bloqueado.
func TestLosContextosMandanSobreElEstadoGlobal(t *testing.T) {
	// Un contexto que falla, con el global diciendo que todo está bien.
	it := checksDeUnSearch(t, commitConRollup(map[string]any{
		"state": "SUCCESS",
		"contexts": map[string]any{"nodes": []any{
			map[string]any{"status": "COMPLETED", "conclusion": "FAILURE", "state": "FAILURE"},
		}},
	}))
	if it.State != model.ChecksFailing {
		t.Errorf("un contexto que falla dio %q, want failing aunque el global diga SUCCESS", it.State)
	}
	if it.Failing != 1 || it.Total != 1 {
		t.Errorf("los recuentos quedaron %+v", it)
	}

	// Y un pendiente con el global diciendo SUCCESS: también manda el contexto, y por un
	// motivo distinto. Un PR con un check corriendo no se mergea, y el estado global que
	// dice SUCCESS es del commit anterior.
	it = checksDeUnSearch(t, commitConRollup(map[string]any{
		"state": "SUCCESS",
		"contexts": map[string]any{"nodes": []any{
			map[string]any{"status": "IN_PROGRESS", "conclusion": "", "state": "IN_PROGRESS"},
		}},
	}))
	if it.State != model.ChecksPending {
		t.Errorf("un contexto corriendo dio %q, want pending", it.State)
	}

	// Y cuando todos pasan, el global no hace falta: el total cuenta.
	it = checksDeUnSearch(t, commitConRollup(map[string]any{
		"contexts": map[string]any{"nodes": []any{
			map[string]any{"status": "COMPLETED", "conclusion": "SUCCESS", "state": "SUCCESS"},
			map[string]any{"status": "COMPLETED", "conclusion": "SUCCESS", "state": "SUCCESS"},
		}},
	}))
	if it.State != model.ChecksPassing || it.Total != 2 {
		t.Errorf("dos checks en verde dio %+v", it)
	}
}

// TestUnFalloMandaSobreUnPendiente: la precedencia DENTRO del agregado.
//
// Y no es obvio que deba ser así. Con un check fallido y otro corriendo, ¿está el PR
// bloqueado o se está esperando? Bloqueado, porque el que falla no va a pasar solo, y
// esperar a que termine el otro solo retrasa el aviso. Es lo que hace que el `switch` de
// `checksFromRollup` mire primero `Failing`.
func TestUnFalloMandaSobreUnPendiente(t *testing.T) {
	it := checksDeUnSearch(t, commitConRollup(map[string]any{
		"contexts": map[string]any{"nodes": []any{
			map[string]any{"status": "IN_PROGRESS", "conclusion": "", "state": "IN_PROGRESS"},
			map[string]any{"status": "COMPLETED", "conclusion": "FAILURE", "state": "FAILURE"},
		}},
	}))
	if it.State != model.ChecksFailing {
		t.Errorf("con un fallo y un pendiente dio %q, want failing", it.State)
	}
	// Y los dos recuentos se cuentan, no solo el que manda: el usuario ve "1 de 2".
	if it.Failing != 1 || it.Pending != 1 || it.Total != 2 {
		t.Errorf("los recuentos quedaron %+v", it)
	}
}

// TestSinCommitsOSinRollupNoHayChecks: los dos "no hay nada" del agregado.
//
// Y ambos dan checks desconocidos, no checks en verde. Un PR recién abierto sin commits no
// tiene CI porque no tiene código que compilar, y eso no es "el CI pasa".
func TestSinCommitsOSinRollupNoHayChecks(t *testing.T) {
	// Sin commits: un rollup `null`, que es lo que llega cuando el PR está vacío.
	it := checksDeUnSearch(t, commitConRollup(nil))
	if it.State != model.ChecksUnknown {
		t.Errorf("sin rollup dio %q, want desconocido", it.State)
	}
	if it.Total != 0 || it.Failing != 0 || it.Pending != 0 {
		t.Errorf("sin rollup dio recuentos %+v", it)
	}
}

// TestLosConclusionsDeGitHubCabenEnUnaListaYLosQueFaltanNoCuentan:
// `isGHFailure` y `isGHPending`, los dos predicados que alimentan el agregado.
//
// Y la lista de `isGHFailure` tiene seis valores que no son `FAILURE`, y todos cuentan como
// fallo: `TIMED_OUT` y `CANCELLED` son fallos a todos los efectos —el PR no va a pasar— y
// `STALE` también. Un `CANCELLED` que no contara dejaría el PR con el CI en verde.
//
// Y el caso al revés, que es el que de verdad se cuela: un check que pasó y cuyo `state`
// es `SUCCESS` no es pendiente. `isGHPending` tiene que distinguir "status vacío" de "status
// que ya terminó", porque si tratara el vacío como no-terminado, todo check en verde sería
// pendiente.
func TestLosConclusionsDeGitHubCabenEnUnaListaYLosQueFaltanNoCuentan(t *testing.T) {
	fallos := []string{"FAILURE", "TIMED_OUT", "CANCELLED", "ACTION_REQUIRED",
		"STARTUP_FAILURE", "STALE", "failure", "Cancelled"}
	for _, c := range fallos {
		if !isGHFailure(c, "") {
			t.Errorf("isGHFailure(%q) dio false: un check cancelado o expirado no pasa", c)
		}
	}
	// Y por `state`, que es donde GitLab-shaped llega desde el otro lado.
	if !isGHFailure("", "ERROR") || !isGHFailure("", "FAILURE") {
		t.Error("isGHFailure no mira el state cuando el conclusion viene vacío")
	}
	// Y lo que NO es fallo, que es la mitad que importa para no bloquear de más.
	for _, c := range []string{"", "SUCCESS", "NEUTRAL", "SKIPPED", "NEUTRAL_OLD"} {
		if isGHFailure(c, "") {
			t.Errorf("isGHFailure(%q) dio true", c)
		}
	}

	pendientes := []string{"IN_PROGRESS", "QUEUED", "PENDING", "WAITING", "EXPECTED",
		"in_progress", "queued"}
	for _, s := range pendientes {
		if !isGHPending(s, "") {
			t.Errorf("isGHPending(%q) dio false", s)
		}
	}
	// Y el caso de siempre: un check terminado que pasa NO está pendiente, ni aunque el
	// status venga vacío.
	for _, s := range []string{"COMPLETED", "completed", ""} {
		if isGHPending(s, "SUCCESS") {
			t.Errorf("isGHPending(%q, SUCCESS) dio true: un check verde está terminado", s)
		}
	}
	// Y un estado pendiente con el status vacío, que es lo que llega cuando la API no
	// rellena el status del contexto.
	if !isGHPending("", "PENDING") {
		t.Error("isGHPending con status vacío y state PENDING dio false")
	}
	// Y lo que no es pendiente de ninguna manera.
	if isGHPending("COMPLETED", "COMPLETED") {
		t.Error("isGHPending de un check completado dio true")
	}
	if isGHPending("", "NEUTRAL") {
		t.Error("isGHPending de un NEUTRAL dio true: neutral no es pendiente")
	}
}

// TestElErrorDeParseDesenvuelveLaCausa: `Error` y su cadena.
//
// Y lo que se fija es lo que permite diagnosticar un parseo roto: la causa original se puede
// alcanzar con `errors.As`, no solo leerla como texto. Sin `Unwrap`, un error de JSON de
// `encoding/json` llega envuelto en un texto y nadie puede distinguir "el forge mandó algo
// que no es JSON" de "el JSON es válido pero le falta un campo".
//
// Y el texto lleva el nombre de la herramienta, que es lo que hace falta para no perder una
// tarde con dos formatos.
func TestElErrorDeParseDesenvuelveLaCausa(t *testing.T) {
	// Con causa: el texto la incluye y `errors.As` la alcanza.
	causa := errors.New("boom")
	e := &Error{Tool: "gh", Msg: "no se pudo leer", Err: causa}
	if !errors.Is(e, causa) {
		t.Error("errors.Is no llega a la causa")
	}
	if !strings.Contains(e.Error(), "boom") {
		t.Errorf("el texto %q no incluye la causa", e.Error())
	}
	// Y el prefijo con la herramienta, que es lo que separa gh de gl en un log.
	if !strings.HasPrefix(e.Error(), "parse gh") {
		t.Errorf("el texto %q no empieza por la herramienta", e.Error())
	}

	// Sin causa: el texto no lleva un ": <nil>" colgando.
	sinCausa := &Error{Tool: "gl", Msg: "campo ausente"}
	if sinCausa.Unwrap() != nil {
		t.Error("Unwrap devolvió algo sin causa")
	}
	texto := sinCausa.Error()
	if strings.Contains(texto, "nil") {
		t.Errorf("un error sin causa trae nil en el texto: %q", texto)
	}
	if texto != "parse gl: campo ausente" {
		t.Errorf("el texto dio %q", texto)
	}

	// Y el caso real: un JSON que no se puede deserializar deja una causa de
	// `encoding/json` que se puede distinguir del error de prdash.
	var syntaxErr *json.SyntaxError
	_, _, err := ParseGHGraphQLSearch(`esto no es json`)
	if err == nil {
		t.Fatal("ParseGHGraphQLSearch con basura dio nil")
	}
	pe := &Error{}
	if !errors.As(err, &pe) {
		t.Errorf("ParseGH devolvió %T, no un *parse.Error", err)
	}
	if errors.As(err, &syntaxErr) {
		t.Log("la causa es un SyntaxError, alcanzable con errors.As")
	} else if !strings.Contains(err.Error(), "ilegible") && !strings.Contains(err.Error(), "parse") {
		t.Errorf("un error de JSON no se parece a uno de parse: %q", err)
	}
}

// checksDeUnSearch pasa un commit por el parser de verdad y devuelve los checks del primer
// item, para que el test llegue a `checksFromRollup` por la misma puerta que produccion.
//
// Y el JSON se compone con `json.Marshal` y no a mano. La primera version lo escribia como
// una cadena con llaves contadas, y faltaba una: tres de los cuatro tests de este fichero
// fallaban con "invalid character" sin llegar al código. Contar llaves a mano en un fixture
// anidado es una fuente de fallos que no prueba nada.
func checksDeUnSearch(t *testing.T, commit map[string]any) model.Checks {
	t.Helper()
	nodo := map[string]any{
		"number": 1,
		"title":  "x",
		"state":  "OPEN",
		"repository": map[string]any{
			"nameWithOwner": "o/r",
			"name":          "r",
			"owner":         map[string]any{"login": "o"},
		},
		"commits": map[string]any{
			"nodes": []any{map[string]any{"commit": commit}},
		},
	}
	raw, err := json.Marshal(map[string]any{
		"data": map[string]any{
			"search": map[string]any{"nodes": []any{nodo}},
		},
	})
	if err != nil {
		t.Fatalf("componer el fixture: %v", err)
	}
	items, _, err := ParseGHGraphQLSearch(string(raw))
	if err != nil {
		t.Fatalf("ParseGHGraphQLSearch(%s): %v", raw, err)
	}
	if len(items) != 1 {
		t.Fatalf("salieron %d items, want 1", len(items))
	}
	return items[0].Checks
}

// commitConRollup mete un statusCheckRollup en el commit que se le pase. El rollup puede
// ser nil, que es el `null` explícito de la API.
func commitConRollup(rollup any) map[string]any {
	return map[string]any{"statusCheckRollup": rollup}
}
