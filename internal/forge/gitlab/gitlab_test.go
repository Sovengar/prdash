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

// TestNotesQueryShape: las notas se piden aparte del inbox, con `last` (la ficha
// enseña el final de la conversación) y con `system`, que es lo único que distingue
// una nota escrita de una que dejó el MR al abrirse. El iid va como literal de string
// porque el schema lo declara `String!`.
func TestNotesQueryShape(t *testing.T) {
	q := glNotesQuery("grupo/sub/proy", 42, commentFetch)
	for _, want := range []string{
		`project(fullPath: "grupo/sub/proy")`,
		`mergeRequest(iid: "42")`,
		"notes(last: 15)",
		"author { username }",
		"system",
		"body",
		"createdAt",
	} {
		if !strings.Contains(q, want) {
			t.Errorf("glNotesQuery no contiene %q:\n%s", want, q)
		}
	}
	if strings.Contains(q, `iid: 42`) {
		t.Errorf("el iid no puede ir como Int (String! lo rechaza):\n%s", q)
	}
	if commentFetch <= forge.CommentLimit {
		t.Errorf("commentFetch = %d, want > %d", commentFetch, forge.CommentLimit)
	}
}

// TestCommentsDropsSystemNotesAndCaps: las notas de sistema se descartan y el resto
// se recorta por la cola al tope de la ficha. El total es el de las leídas: GitLab
// no expone recuento, así que nunca hay "5 de N" y la línea del total no engaña.
func TestCommentsDropsSystemNotesAndCaps(t *testing.T) {
	dir := t.TempDir()
	script := writeScript(t, dir, "glab", `#!/bin/sh
cat <<'OUT'
{"data":{"project":{"mergeRequest":{"notes":{"nodes":[
{"author":{"username":null},"body":"added 56 commits","createdAt":"2026-09-20T10:00:00Z","system":true},
{"author":{"username":"alice"},"body":"first human","createdAt":"2026-09-20T10:01:00Z","system":false},
{"author":{"username":"bob"},"body":"second human","createdAt":"2026-09-20T10:02:00Z","system":false},
{"author":{"username":"carol"},"body":"third human","createdAt":"2026-09-20T10:03:00Z","system":false},
{"author":{"username":"dave"},"body":"fourth human","createdAt":"2026-09-20T10:04:00Z","system":false},
{"author":{"username":"erin"},"body":"fifth human","createdAt":"2026-09-20T10:05:00Z","system":false},
{"author":{"username":"frank"},"body":"sixth human","createdAt":"2026-09-20T10:06:00Z","system":false},
{"author":{"username":"gina"},"body":"seventh human","createdAt":"2026-09-20T10:07:00Z","system":false}
]}}}}}
OUT
`)
	page, warns := New("gitlab.example.com", script).Comments(context.Background(),
		model.RepoRef{Project: "grupo/sub/proy"}, 42)
	if len(warns) != 0 {
		t.Fatalf("warnings = %v", warns)
	}
	if len(page.Comments) != forge.CommentLimit {
		t.Fatalf("comments = %d, want %d", len(page.Comments), forge.CommentLimit)
	}
	// Ni la nota de sistema ni las humanas más antiguas se colan: la conexión
	// pedía las 15 últimas notas y el recorte se queda con las 5 últimas humanas.
	for _, c := range page.Comments {
		if c.Body == "added 56 commits" {
			t.Errorf("coló una nota de sistema: %q", c.Body)
		}
	}
	want := []string{"third human", "fourth human", "fifth human", "sixth human", "seventh human"}
	for i, w := range want {
		if page.Comments[i].Body != w {
			t.Errorf("comments[%d] = %q, want %q (los últimos, en orden)", i, page.Comments[i].Body, w)
		}
	}
	// GitLab no expone recuento: el total es lo leído (7), no lo pintado (5). Es
	// una cota inferior, que es justo lo que hace falta para insinuar que hay más
	// conversación. Ver CommentPage.Total.
	if page.Total != 7 {
		t.Errorf("total = %d, want 7 (las notas leídas, no las pintadas)", page.Total)
	}
}

// TestCommentsEmptyRepo: sin proyecto no hay query que componer, así que avisa en
// vez de golpear la API con un literal vacío.
func TestCommentsEmptyRepo(t *testing.T) {
	script := writeScript(t, t.TempDir(), "glab", "#!/bin/sh\nexit 1\n")
	_, warns := New("gitlab.example.com", script).Comments(context.Background(), model.RepoRef{}, 42)
	if len(warns) == 0 || warns[0].Kind != "notfound" {
		t.Errorf("warnings = %v, want notfound", warns)
	}
}

// TestCommentsFailureIsWarning: un fallo del forge avisa y no lanza.
func TestCommentsFailureIsWarning(t *testing.T) {
	script := writeScript(t, t.TempDir(), "glab", "#!/bin/sh\necho boom >&2\nexit 1\n")
	page, warns := New("gitlab.example.com", script).Comments(context.Background(),
		model.RepoRef{Project: "grupo/proy"}, 42)
	if len(page.Comments) != 0 {
		t.Errorf("un fallo no debe devolver comentarios: %+v", page.Comments)
	}
	if len(warns) == 0 {
		t.Fatal("un fallo debería avisar")
	}
}
