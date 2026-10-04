package github

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"prdash/internal/forge/model"
)

// `gh pr checks` exits 8 (pending) or 1 (failing) and still prints the JSON; a runner that looked
//at the exit code would discard it and classify an error.

func ghQueRegistra(t *testing.T, cuerpo string) (script, argsFile string) {
	t.Helper()
	dir := t.TempDir()
	argsFile = filepath.Join(dir, "args.log")
	ruta := filepath.Join(dir, "gh")
	script = "#!/bin/sh\necho \"$@\" >> \"" + argsFile + "\"\n" + cuerpo
	if err := os.WriteFile(ruta, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return ruta, argsFile
}

func argsRegistrados(t *testing.T, path string) string {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("no se registró ninguna llamada: %v", err)
	}
	return string(raw)
}

// checksJSON son checks de `gh pr checks --json name,state,bucket`.
const checksJSON = `[{"name":"build","state":"FAILURE","bucket":"fail"},
{"name":"lint","state":"SUCCESS","bucket":"pass"},
{"name":"e2e","state":"PENDING","bucket":"pending"}]`

const searchJSON = `{"total_count":1,"incomplete_results":false,"items":[
{"number":42,"title":"PR","html_url":"https://github.com/acme/proy/pull/42",
 "state":"open","created_at":"2026-09-20T07:00:00Z","updated_at":"2026-09-21T07:00:00Z",
 "user":{"login":"me"},
 "repository_url":"https://api.github.com/repos/acme/proy",
 "pull_request":{"url":"https://api.github.com/repos/acme/proy/pulls/42",
   "head":{"ref":"feat/x","sha":"deadbeef"},"base":{"ref":"main"}},
 "labels":[{"name":"uno"}]}
]}`

// This case's finding, and the reason checks parses BEFORE the exit code.
func TestLosChecksSeLeenAunQueElCodigoDeSalitaSeaDeError(t *testing.T) {
	script, argsFile := ghQueRegistra(t,
		"cat <<'JSON'\n"+checksJSON+"\nJSON\nexit 8\n")
	a := New("github.com", script)

	checks, warns := a.checks(context.Background(), "acme/proy", 42)

	if len(warns) != 0 {
		t.Fatalf("checks dio avisos %+v con el JSON parseable: el codigo de salida de "+
			"`gh pr checks` no es el veredicto", warns)
	}
	if checks.State != model.ChecksFailing {
		t.Errorf("estado = %q, want failing: hay un check en fail", checks.State)
	}
	if checks.Failing != 1 || checks.Pending != 1 || checks.Total != 3 {
		t.Errorf("recuentos = %+v, want 1/1/3", checks)
	}
	// The command carries the repo and the number, which is how it avoids depending on the cwd.
	args := argsRegistrados(t, argsFile)
	for _, quiere := range []string{"pr checks 42", "--repo acme/proy", "--json"} {
		if !strings.Contains(args, quiere) {
			t.Errorf("los args no traen %q: %s", quiere, args)
		}
	}
}

// Two cases, not one, and the difference is which class the warning carries.
func TestLosChecksSinJSONSonUnAvisoConSuClase(t *testing.T) {
	// El comando falla sin JSON.
	script, _ := ghQueRegistra(t, "echo 'HTTP 403: Forbidden' >&2\nexit 1\n")
	checks, warns := New("github.com", script).checks(context.Background(), "acme/proy", 42)
	if len(warns) != 1 {
		t.Fatalf("sin JSON y con fallo dio %d avisos, want 1", len(warns))
	}
	if warns[0].Kind != "permission" {
		t.Errorf("un 403 dio la clase %q, want permission", warns[0].Kind)
	}
	if checks.State != model.ChecksUnknown {
		t.Errorf("un fallo dio estado %q, want desconocido: no se puede pintar un CI "+
			"que no se ha preguntado", checks.State)
	}

	script, _ = ghQueRegistra(t, "echo 'esto no es json'\nexit 0\n")
	_, warns = New("github.com", script).checks(context.Background(), "acme/proy", 42)
	if len(warns) != 1 || warns[0].Kind != "parse" {
		t.Fatalf("aviso = %+v, want kind parse", warns)
	}
	if !strings.Contains(warns[0].Msg, "unreadable") {
		t.Errorf("el mensaje %q no dice que la salida no se entendió", warns[0].Msg)
	}
}

// The fallback does NOT ask for checks, and that is not an omission.
func TestElRespaldoRESTTraeLosPropiosYNoTraeLosChecks(t *testing.T) {
	script, argsFile := ghQueRegistra(t, "cat <<'JSON'\n"+searchJSON+"\nJSON\n")
	a := New("github.com", script)

	page, ok := a.restAuthored(context.Background())
	if !ok {
		t.Fatal("el respaldo REST devolvió ok=false con una respuesta válida")
	}
	if len(page.Items) != 1 {
		t.Fatalf("trajo %d items, want 1", len(page.Items))
	}
	it := page.Items[0]
	if it.Number != 42 || it.Ref.Project == "" {
		t.Errorf("el item no se leyó bien: %+v", it)
	}
	// The stamp: section and forge identity. Without it the fallback returns items that belong
	// nowhere.
	if it.Section != model.SectionAuthored {
		t.Errorf("sección = %q, want la de propios", it.Section)
	}
	if it.Forge != ForgeName || it.Host != "github.com" {
		t.Errorf("identidad = %s@%s", it.Forge, it.Host)
	}
	if it.Checks.State != model.ChecksUnknown {
		t.Errorf("el respaldo trajo checks %q, want desconocido", it.Checks.State)
	}

	args := argsRegistrados(t, argsFile)
	for _, quiere := range []string{"search/issues", "is:pr is:open author:@me", "per_page"} {
		if !strings.Contains(args, quiere) {
			t.Errorf("los args no traen %q: %s", quiere, args)
		}
	}
}

// Both return false instead of an error, which is what lets the caller fall through.
func TestElRespaldoRESTNoEntraSiFallaONoSeEntiende(t *testing.T) {
	// Falla.
	script, _ := ghQueRegistra(t, "echo 'boom' >&2\nexit 1\n")
	if _, ok := New("github.com", script).restAuthored(context.Background()); ok {
		t.Error("un respaldo que falla dio ok=true")
	}

	script, _ = ghQueRegistra(t, "echo 'esto no es json'\nexit 0\n")
	if _, ok := New("github.com", script).restAuthored(context.Background()); ok {
		t.Error("un respaldo ilegible dio ok=true: no es un respaldo, es un fallo")
	}

	script, _ = ghQueRegistra(t,
		`echo '{"total_count":0,"incomplete_results":false,"items":[]}'`)
	page, ok := New("github.com", script).restAuthored(context.Background())
	if !ok {
		t.Error("una respuesta vacía dio ok=false: no tienes PRs y no es un fallo")
	}
	if len(page.Items) != 0 {
		t.Errorf("una respuesta vacia trajo %d items", len(page.Items))
	}
}

// It warns WITHOUT calling the forge: a reference with no owner is not something to ask about.
func TestRelecturarUnItemSinDueñoAvisaSinLlamarAlForge(t *testing.T) {
	script, argsFile := ghQueRegistra(t, "echo '{\"data\":{}}'\n")
	a := New("github.com", script)

	_, warns := a.ItemState(context.Background(),
		model.RepoRef{Project: "proy"}, 1)
	if len(warns) != 1 || warns[0].Kind != "notfound" {
		t.Fatalf("aviso = %+v, want un notfound", warns)
	}
	if !strings.Contains(warns[0].Msg, "invalid repo reference") {
		t.Errorf("el mensaje %q no dice que la referencia es inválida", warns[0].Msg)
	}
	if _, err := os.Stat(argsFile); err == nil {
		t.Error("se llamó al forge con una referencia sin dueño: la query necesita los " +
			"dos nombres y habría pedido el PR #0 de un repositorio vacío")
	}

	script, argsFile = ghQueRegistra(t, "echo '{\"data\":{\"search\":{\"nodes\":[]}}}'\n")
	a = New("github.com", script)
	_, warns = a.ItemState(context.Background(), model.RepoRef{Project: "acme/proy"}, 1)
	if _, err := os.Stat(argsFile); err != nil {
		t.Error("con dueño no se llamó al forge")
	}
	// A PR that does not exist in a repo that does: also notfound, and the message says so.
	if len(warns) != 1 || warns[0].Kind != "notfound" {
		t.Errorf("un PR inexistente dio %+v, want notfound", warns)
	}
	if !strings.Contains(warns[0].Msg, "no encontrado") {
		t.Errorf("el mensaje %q no dice que el PR no se encontró", warns[0].Msg)
	}

	_, warns = New("github.com", "/no-debe-ejecutarse").ItemState(context.Background(),
		model.RepoRef{}, 1)
	if len(warns) != 1 || warns[0].Kind != "notfound" {
		t.Errorf("una referencia vacía dio %+v, want notfound", warns)
	}
}

// My first version asserted the checks stayed and they did not.
func TestElRollupDelGraphQLSobreviveSiLosChecksNoSePudenLeer(t *testing.T) {
	script, _ := ghQueRegistra(t, cuerpoChecksQueFallan())
	it, warns := New("github.com", script).ItemState(context.Background(),
		model.RepoRef{Project: "acme/proy"}, 42)

	if len(warns) != 0 {
		t.Fatalf("ItemState dio avisos %+v: los checks que fallan no deben tumbar la "+
			"lectura del item", warns)
	}
	if it.Number != 42 || it.Title != "PR" {
		t.Fatalf("el item no se leyo: %+v", it)
	}
	if it.Checks.State != model.ChecksPassing {
		t.Errorf("los checks = %q, want el del rollup: una segunda fuente que falla no "+
			"puede borrar lo que ya se sabia", it.Checks.State)
	}
}

// This is why `gh pr checks` exists at all: the rollup and the command can disagree.
func TestLosChecksDeLaSegundaFuenteSobrescribenAlRollup(t *testing.T) {
	cuerpo := `case "$*" in
  *"pr checks"*) cat <<'JSON'
` + checksJSON + `
JSON
    exit 8;;
  *) cat <<'JSON'
` + graphqlJSONDeUnItem() + `
JSON
    exit 0;;
esac
exit 1
`
	script, _ := ghQueRegistra(t, cuerpo)
	it, warns := New("github.com", script).ItemState(context.Background(),
		model.RepoRef{Project: "acme/proy"}, 42)

	if len(warns) != 0 {
		t.Fatalf("avisos = %+v", warns)
	}
	if it.Checks.State != model.ChecksFailing {
		t.Errorf("los checks = %q, want failing: la segunda fuente manda cuando discrepa "+
			"del rollup", it.Checks.State)
	}
	if it.Checks.Failing != 1 {
		t.Errorf("recuentos = %+v, want un fallando", it.Checks)
	}
}

func cuerpoChecksQueFallan() string {
	return `case "$*" in
  *"pr checks"*) echo 'nope' >&2; exit 1;;
  *) cat <<'JSON'
` + graphqlJSONDeUnItem() + `
JSON
    exit 0;;
esac
exit 1
`
}

func graphqlJSONDeUnItem() string {
	return `{"data":{"search":{"nodes":[
{"number":42,"title":"PR","state":"OPEN","url":"https://github.com/acme/proy/pull/42",
 "headRefName":"feat/x","baseRefName":"main","isDraft":false,
 "updatedAt":"2026-09-21T07:00:00Z","author":{"login":"me"},
 "headRefOid":"deadbeef","mergeable":"MERGEABLE","reviewDecision":"REVIEW_REQUIRED",
 "additions":5,"deletions":1,"changedFiles":2,
 "repository":{"nameWithOwner":"acme/proy","name":"proy","owner":{"login":"acme"},
   "viewerPermission":"WRITE","mergeCommitAllowed":true,"rebaseMergeAllowed":true,
   "squashMergeAllowed":true},
 "commits":{"nodes":[{"commit":{"statusCheckRollup":{"state":"SUCCESS",
   "contexts":{"nodes":[]}}}}]}}
],"pageInfo":{"hasNextPage":false,"endCursor":null}}}}`
}
