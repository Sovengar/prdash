package parse

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"prdash/internal/forge/model"
)

// Where a forge's datum becomes a signal the TUI uses to decide whether to block a merge.

// A real case, not an edge: GitHub's classic status checks use the global state.
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
		// Lowercased, because GitHub's global state arrives in upper case but not in every field.
		{"success en minusculas", "success", model.ChecksPassing},
		// What is not recognised stays unknown: a rare global state is not turned into "passing".
		{"estado raro", "NEUTRAL", model.ChecksUnknown},
		{"vacio", "", model.ChecksUnknown},
	}
	for _, c := range casos {
		if got := checksStateFromRollup(c.estado); got != c.want {
			t.Errorf("%s: checksStateFromRollup(%q) dio %q, want %q", c.nombre, c.estado, got, c.want)
		}
	}
	// The whole aggregator: a rollup with no contexts and a global state gives the same state, with the
	//total at zero because there are none.
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

// The contexts win over the global state for a concrete reason.
func TestLosContextosMandanSobreElEstadoGlobal(t *testing.T) {
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

	it = checksDeUnSearch(t, commitConRollup(map[string]any{
		"state": "SUCCESS",
		"contexts": map[string]any{"nodes": []any{
			map[string]any{"status": "IN_PROGRESS", "conclusion": "", "state": "IN_PROGRESS"},
		}},
	}))
	if it.State != model.ChecksPending {
		t.Errorf("un contexto corriendo dio %q, want pending", it.State)
	}

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

// Not obvious that a failure should beat a pending.
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
	if it.Failing != 1 || it.Pending != 1 || it.Total != 2 {
		t.Errorf("los recuentos quedaron %+v", it)
	}
}

// Both give UNKNOWN checks, not green ones.
func TestSinCommitsOSinRollupNoHayChecks(t *testing.T) {
	it := checksDeUnSearch(t, commitConRollup(nil))
	if it.State != model.ChecksUnknown {
		t.Errorf("sin rollup dio %q, want desconocido", it.State)
	}
	if it.Total != 0 || it.Failing != 0 || it.Pending != 0 {
		t.Errorf("sin rollup dio recuentos %+v", it)
	}
}

// isGHFailure and isGHPending, the two predicates.
func TestLosConclusionsDeGitHubCabenEnUnaListaYLosQueFaltanNoCuentan(t *testing.T) {
	fallos := []string{"FAILURE", "TIMED_OUT", "CANCELLED", "ACTION_REQUIRED",
		"STARTUP_FAILURE", "STALE", "failure", "Cancelled"}
	for _, c := range fallos {
		if !isGHFailure(c, "") {
			t.Errorf("isGHFailure(%q) dio false: un check cancelado o expirado no pasa", c)
		}
	}
	if !isGHFailure("", "ERROR") || !isGHFailure("", "FAILURE") {
		t.Error("isGHFailure no mira el state cuando el conclusion viene vacío")
	}
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
	// A finished check that passes is NOT pending, not even when the status is still queued.
	for _, s := range []string{"COMPLETED", "completed", ""} {
		if isGHPending(s, "SUCCESS") {
			t.Errorf("isGHPending(%q, SUCCESS) dio true: un check verde está terminado", s)
		}
	}
	// A pending state with an empty status, which is what arrives when the API does not fill it in.
	if !isGHPending("", "PENDING") {
		t.Error("isGHPending con status vacío y state PENDING dio false")
	}
	if isGHPending("COMPLETED", "COMPLETED") {
		t.Error("isGHPending de un check completado dio true")
	}
	if isGHPending("", "NEUTRAL") {
		t.Error("isGHPending de un NEUTRAL dio true: neutral no es pendiente")
	}
}

func TestElErrorDeParseDesenvuelveLaCausa(t *testing.T) {
	causa := errors.New("boom")
	e := &Error{Tool: "gh", Msg: "no se pudo leer", Err: causa}
	if !errors.Is(e, causa) {
		t.Error("errors.Is no llega a la causa")
	}
	if !strings.Contains(e.Error(), "boom") {
		t.Errorf("el texto %q no incluye la causa", e.Error())
	}
	if !strings.HasPrefix(e.Error(), "parse gh") {
		t.Errorf("el texto %q no empieza por la herramienta", e.Error())
	}

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

func commitConRollup(rollup any) map[string]any {
	return map[string]any{"statusCheckRollup": rollup}
}
