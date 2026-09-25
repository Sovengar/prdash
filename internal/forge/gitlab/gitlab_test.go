package gitlab

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"prdash/internal/forge"
	"prdash/internal/forge/model"
	"prdash/internal/testutil"
)

func TestAuthArgsScopedByHost(t *testing.T) {
	a := New("umane.emeal.nttdata.com", "glab")
	joined := strings.Join(a.authArgs(), " ")
	if !strings.Contains(joined, "auth status") || !strings.Contains(joined, "--hostname umane.emeal.nttdata.com") {
		t.Fatalf("authArgs = %q", joined)
	}
}

func TestGraphQLArgsPinHost(t *testing.T) {
	a := New("h.example", "glab")
	joined := strings.Join(a.graphqlArgs("query { x }"), " ")
	for _, want := range []string{"api", "--hostname h.example", "graphql", "-f query=query { x }"} {
		if !strings.Contains(joined, want) {
			t.Errorf("graphqlArgs no contiene %q: %q", want, joined)
		}
	}
}

// TestRESTArgsUseGetAndRelativeEndpoint cubre C3 y C4: recurso relativo y GET
// explícito (con campos, glab haría POST).
func TestRESTArgsUseGetAndRelativeEndpoint(t *testing.T) {
	a := New("h.example", "glab")
	joined := strings.Join(a.getArgs("todos", "action=mentioned"), " ")
	for _, want := range []string{"-X GET", " todos", "-f action=mentioned"} {
		if !strings.Contains(joined, want) {
			t.Errorf("getArgs no contiene %q: %q", want, joined)
		}
	}
	if strings.Contains(joined, "/git/api/v4") || strings.Contains(joined, "/api/v4") {
		t.Errorf("no debe construir la ruta absoluta: %q", joined)
	}
}

func TestMRActionArgs(t *testing.T) {
	a := New("h.example", "glab")
	if got := strings.Join(a.mrArgs("merge", 7, "grp/proj", "--yes"), " "); !strings.Contains(got, "mr merge 7 -R grp/proj --yes") {
		t.Errorf("mrArgs = %q", got)
	}
}

// TestRunnerEnvPinsHost cubre C2 para `glab mr`, que no acepta --hostname.
func TestRunnerEnvPinsHost(t *testing.T) {
	a := New("h.example", "glab")
	if joined := strings.Join(a.runner.Extra, " "); !strings.Contains(joined, "GITLAB_HOST=h.example") {
		t.Fatalf("runner.Extra = %q", joined)
	}
}

func TestRESTEndpointRelative(t *testing.T) {
	if got := restEndpoint("/merge_requests"); got != "merge_requests" {
		t.Errorf("restEndpoint = %q", got)
	}
	if got := restEndpoint("todos"); got != "todos" {
		t.Errorf("restEndpoint = %q", got)
	}
}

// TestQueryBuilders cubre C5: la instancia CE rechaza approvalsLeft, así que no
// se pide.
func TestQueryBuilders(t *testing.T) {
	if q := glAuthoredQuery(""); !strings.Contains(q, "authoredMergeRequests") || !strings.Contains(q, "pageInfo") {
		t.Errorf("glAuthoredQuery = %s", q)
	}
	if q := glReviewQuery(""); !strings.Contains(q, "reviewRequestedMergeRequests") {
		t.Errorf("glReviewQuery = %s", q)
	}
	if q := glAssignedQuery(""); !strings.Contains(q, "assignedMergeRequests") {
		t.Errorf("glAssignedQuery = %s", q)
	}
	if q := glAuthoredQuery("CUR"); !strings.Contains(q, `after: "CUR"`) {
		t.Errorf("la query paginada debería llevar el cursor: %s", q)
	}
	// El iid es un literal de cadena: el schema lo declara `String!` y GraphQL no
	// coacciona Int -> String, así que sin comillas la query entera se rechaza.
	if q := glMRQuery("grp/proj", 7); !strings.Contains(q, `project(fullPath: "grp/proj")`) || !strings.Contains(q, `mergeRequest(iid: "7")`) {
		t.Errorf("glMRQuery = %s", q)
	}
	if q := glMRQuery("grp/proj", 7); strings.Contains(q, "mergeRequest(iid: 7)") {
		t.Errorf("glMRQuery no debería pasar el iid como Int: %s", q)
	}
	if !strings.Contains(mrFields, "approved") || strings.Contains(mrFields, "approvalsLeft") {
		t.Errorf("mrFields debería usar approved y no approvalsLeft: %s", mrFields)
	}
}

func TestConformanceMissingBinary(t *testing.T) {
	bin := filepath.Join(t.TempDir(), "no-glab")
	testutil.RunConformance(t, New("gitlab.example.com", bin), testutil.ConformanceOptions{MissingBinary: true})
}

func TestListUnknownSectionReportsUnsupported(t *testing.T) {
	a := New("gitlab.example.com", filepath.Join(t.TempDir(), "no-glab"))
	_, warns := a.List(context.Background(), forge.Query{Section: "desconocida"})
	if len(warns) == 0 || warns[0].Kind != "unsupported" {
		t.Fatalf("warnings = %+v", warns)
	}
}

// TestListAuthoredFallsBackToREST cubre C7/M3: si GraphQL falla, el respaldo
// REST devuelve datos parciales marcados como degradados (sin red real: un
// `glab` falso en un script).
func TestListAuthoredFallsBackToREST(t *testing.T) {
	dir := t.TempDir()
	argsFile := filepath.Join(dir, "args.log")
	script := writeScript(t, dir, "glab", `#!/bin/sh
echo "$@" >> "`+argsFile+`"
case "$*" in
  *graphql*) echo "API rate limit exceeded" >&2; exit 1;;
  *merge_requests*) cat <<'JSON'
[{"iid":12,"title":"MR","web_url":"u","state":"opened","source_branch":"a","target_branch":"main","updated_at":"2026-09-21T07:00:00Z","author":{"username":"me"},"references":{"full":"grp/sub/proj!12","short":"!12"}}]
JSON
    exit 0;;
esac
exit 1
`)

	a := New("h.example", script)
	page, warns := a.List(context.Background(), forge.Query{Section: model.SectionAuthored})
	if len(page.Items) != 1 || page.Items[0].Ref.Project != "grp/sub/proj" {
		t.Fatalf("items = %+v", page.Items)
	}
	if len(warns) == 0 || warns[0].Kind != "degraded" {
		t.Fatalf("warnings = %+v", warns)
	}

	log, _ := os.ReadFile(argsFile)
	if !strings.Contains(string(log), "--hostname h.example") || !strings.Contains(string(log), "-X GET merge_requests") {
		t.Fatalf("los args reales no fijan host/GET:\n%s", string(log))
	}
}

// TestAuthExposesLogin: la salida de `glab auth status` que ya se pedía para
// comprobar la sesión trae el usuario; se reutiliza para reconocer los ítems
// propios sin ninguna llamada extra.
func TestAuthExposesLogin(t *testing.T) {
	dir := t.TempDir()
	script := writeScript(t, dir, "glab", `#!/bin/sh
cat <<'OUT'
umane.emeal.nttdata.com
  ✓ Logged in to umane.emeal.nttdata.com as jogarrui (/home/u/.config/glab-cli/config.yml)
  ✓ Git operations for umane.emeal.nttdata.com configured to use https protocol.
  - Active account: true
OUT
`)
	a := New("umane.emeal.nttdata.com", script)
	auth := a.Auth(context.Background())
	if !auth.OK {
		t.Fatalf("Auth = %+v", auth)
	}
	if auth.Login != "jogarrui" {
		t.Errorf("Login = %q, want %q", auth.Login, "jogarrui")
	}
}

// TestAuthLoginEmptyWhenUnknown: si la salida no trae login, se deja vacío y
// quien decide es la regla de sección, no una suposición.
func TestAuthLoginEmptyWhenUnknown(t *testing.T) {
	dir := t.TempDir()
	script := writeScript(t, dir, "glab", "#!/bin/sh\necho 'glab: logged in'\n")
	if login := New("h.example", script).Auth(context.Background()).Login; login != "" {
		t.Errorf("Login = %q, want vacío", login)
	}
}

// writeScript crea un binario falso ejecutable y devuelve su ruta.
func writeScript(t *testing.T, dir, name, body string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}
