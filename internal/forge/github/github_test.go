package github

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

// TestAuthExposesLogin: `gh auth status` ya se lanzaba para comprobar la sesión
// y su salida se descartaba; esa línea trae la cuenta, así que el viewer se
// conoce sin llamadas extra.
func TestAuthExposesLogin(t *testing.T) {
	dir := t.TempDir()
	script := writeScript(t, dir, "gh", `#!/bin/sh
cat <<'OUT'
github.com
  ✓ Logged in to github.com account Sovengar (/home/u/.config/gh/hosts.yml)
  - Active account: true
  - Token: gho_****
OUT
`)
	a := New("github.com", script)
	auth := a.Auth(context.Background())
	if !auth.OK {
		t.Fatalf("Auth = %+v", auth)
	}
	if auth.Login != "Sovengar" {
		t.Errorf("Login = %q, want %q", auth.Login, "Sovengar")
	}
}

// TestAuthLoginEmptyWhenUnknown: sin login en la salida, AuthState.Login queda
// vacío para que la decisión la tome la sección y no una suposición.
func TestAuthLoginEmptyWhenUnknown(t *testing.T) {
	dir := t.TempDir()
	script := writeScript(t, dir, "gh", "#!/bin/sh\necho 'github.com'\n")
	if login := New("github.com", script).Auth(context.Background()).Login; login != "" {
		t.Errorf("Login = %q, want vacío", login)
	}
}

func TestSearchQueryPagination(t *testing.T) {
	first := searchQuery("author:@me", "")
	if !strings.Contains(first, "is:pr is:open author:@me") {
		t.Errorf("searchQuery no contiene el qualifier:\n%s", first)
	}
	if strings.Contains(first, "after:") {
		t.Errorf("la primera página no debería llevar cursor:\n%s", first)
	}
	if !strings.Contains(first, "pageInfo") || !strings.Contains(first, "reviewDecision") {
		t.Errorf("searchQuery debería pedir pageInfo y reviewDecision:\n%s", first)
	}

	next := searchQuery("mentions:@me", "CURSOR9")
	if !strings.Contains(next, `after: "CURSOR9"`) {
		t.Errorf("la página siguiente debería llevar el cursor:\n%s", next)
	}
}

func TestQualifierFor(t *testing.T) {
	cases := []struct {
		q    forge.Query
		want string
	}{
		{forge.Query{Section: model.SectionAuthored}, "author:@me"},
		{forge.Query{Section: model.SectionReview, ReviewKind: model.ReviewRequested}, "review-requested:@me"},
		{forge.Query{Section: model.SectionReview, ReviewKind: model.ReviewAssigned}, "assignee:@me"},
		{forge.Query{Section: model.SectionMentions}, "mentions:@me"},
	}
	for _, c := range cases {
		got, ok := qualifierFor(c.q)
		if !ok || got != c.want {
			t.Errorf("qualifierFor(%+v) = %q, %v", c.q, got, ok)
		}
	}
	if _, ok := qualifierFor(forge.Query{Section: "nope"}); ok {
		t.Error("una sección desconocida no debería tener qualifier")
	}
}

func TestPrQuery(t *testing.T) {
	q := prQuery("acme", "widget", 42)
	for _, want := range []string{`repository(owner: "acme", name: "widget")`, "pullRequest(number: 42)", "reviewDecision"} {
		if !strings.Contains(q, want) {
			t.Errorf("prQuery no contiene %q:\n%s", want, q)
		}
	}
}

func TestSplitProject(t *testing.T) {
	owner, name := splitProject("acme/widget")
	if owner != "acme" || name != "widget" {
		t.Errorf("splitProject = %q, %q", owner, name)
	}
}

// TestConformanceMissingBinary pasa la suite de contrato con un binario
// inexistente: no hay ninguna llamada de red.
func TestConformanceMissingBinary(t *testing.T) {
	bin := filepath.Join(t.TempDir(), "no-gh")
	testutil.RunConformance(t, New("github.com", bin), testutil.ConformanceOptions{MissingBinary: true})
}

func TestListUnknownSectionReportsUnsupported(t *testing.T) {
	a := New("github.com", filepath.Join(t.TempDir(), "no-gh"))
	_, warns := a.List(context.Background(), forge.Query{Section: "desconocida"})
	if len(warns) == 0 || warns[0].Kind != "unsupported" {
		t.Fatalf("warnings = %+v", warns)
	}
}

// TestSearchQueryUsesUnionFragments cubre C1: search.nodes es la unión
// SearchResultItem y contexts.nodes la unión StatusCheckRollupContext.
func TestSearchQueryUsesUnionFragments(t *testing.T) {
	q := searchQuery("author:@me", "")
	for _, want := range []string{"... on PullRequest", "... on CheckRun", "... on StatusContext", "reviewDecision", "statusCheckRollup"} {
		if !strings.Contains(q, want) {
			t.Errorf("searchQuery no contiene %q:\n%s", want, q)
		}
	}
	if p := prQuery("o", "r", 1); !strings.Contains(p, "statusCheckRollup") {
		t.Errorf("prQuery debería pedir los checks:\n%s", p)
	}
}

// TestListAuthoredFallsBackToRESTDegraded cubre C7/M3: el respaldo REST real no
// trae ramas; debe avisarse como degradado.
func TestListAuthoredFallsBackToRESTDegraded(t *testing.T) {
	dir := t.TempDir()
	script := writeScript(t, dir, "gh", `#!/bin/sh
case "$*" in
  *graphql*) echo "HTTP 500: server error" >&2; exit 1;;
  *search/issues*) cat <<'JSON'
{"total_count":1,"items":[{"number":7,"title":"Fix","html_url":"u","state":"open","updated_at":"2026-09-20T08:00:00Z","user":{"login":"me"},"repository_url":"https://api.github.com/repos/acme/lib"}]}
JSON
    exit 0;;
esac
exit 1
`)
	a := New("github.com", script)
	page, warns := a.List(context.Background(), forge.Query{Section: model.SectionAuthored})
	if len(page.Items) != 1 || page.Items[0].Ref.Project != "acme/lib" {
		t.Fatalf("items = %+v", page.Items)
	}
	if page.Items[0].SourceBranch != "" {
		t.Errorf("el fallback REST no trae ramas: %+v", page.Items[0])
	}
	if len(warns) == 0 || warns[0].Kind != "degraded" {
		t.Fatalf("warnings = %+v", warns)
	}
}

// TestChecksKeepsPendingOnExit8 cubre C8: exit 8 (pendiente) trae JSON válido.
func TestChecksKeepsPendingOnExit8(t *testing.T) {
	dir := t.TempDir()
	script := writeScript(t, dir, "gh", `#!/bin/sh
echo '[{"name":"ci","state":"PENDING","bucket":"pending"}]'
exit 8
`)
	a := New("github.com", script)
	c, warns := a.checks(context.Background(), "acme/widget", 1)
	if len(warns) != 0 {
		t.Fatalf("warnings = %+v", warns)
	}
	if c.State != model.ChecksPending || c.Pending != 1 {
		t.Fatalf("checks = %+v", c)
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

// TestCommentsQueryShape: la conversación se pide aparte del inbox y con `first`,
// no `last`, porque la ficha enseña el principio de la conversación (dónde está
// el contexto de qué se pidió) y no las últimas respuestas peleándose por el
// sitio. `totalCount` va en la misma conexión para no gastar una segunda consulta.
func TestCommentsQueryShape(t *testing.T) {
	q := commentsQuery("acme", "widget", 42, commentFetch)
	for _, want := range []string{
		`repository(owner: "acme", name: "widget")`,
		"pullRequest(number: 42)",
		"comments(first: 15)",
		"totalCount",
		"author { login }",
		"body",
		"createdAt",
	} {
		if !strings.Contains(q, want) {
			t.Errorf("commentsQuery no contiene %q:\n%s", want, q)
		}
	}
	if strings.Contains(q, "last:") {
		t.Errorf("commentsQuery no debería pedir los últimos comentarios:\n%s", q)
	}
	// El margen sobre el tope de la ficha es lo que hace que un PR con boilerplate
	// de bots no llegue corto tras filtrar.
	if commentFetch <= forge.CommentLimit {
		t.Errorf("commentFetch = %d, want > %d para absorber el ruido", commentFetch, forge.CommentLimit)
	}
}

// TestCommentsEscapesRepoNames: owner y name van como literales, así que un
// nombre con comillas rompería la query entera.
func TestCommentsEscapesRepoNames(t *testing.T) {
	if q := commentsQuery(`ac"me`, "wid\\get", 1, 5); !strings.Contains(q, `owner: "ac\"me"`) {
		t.Errorf("no escapó el owner:\n%s", q)
	}
	if q := commentsQuery("acme", "wid\\get", 1, 5); !strings.Contains(q, `name: "wid\\get"`) {
		t.Errorf("no escapó el name:\n%s", q)
	}
}

// TestCommentsCapsAtFichaLimitKeepingTotal: la consulta trae más de los que la
// ficha enseña, así que se recorta a CommentLimit. El total se conserva entero:
// es lo que permite decir "5 de 23" y saber que la ficha se está perdiendo
// conversación, que es justo lo que decide abrir el PR.
func TestCommentsCapsAtFichaLimitKeepingTotal(t *testing.T) {
	dir := t.TempDir()
	script := writeScript(t, dir, "gh", `#!/bin/sh
cat <<'OUT'
{"data":{"repository":{"pullRequest":{"comments":{"totalCount":23,"nodes":[
{"author":{"login":"u0"},"body":"c0","createdAt":"2026-09-20T10:00:00Z"},
{"author":{"login":"u1"},"body":"c1","createdAt":"2026-09-20T10:01:00Z"},
{"author":{"login":"u2"},"body":"c2","createdAt":"2026-09-20T10:02:00Z"},
{"author":{"login":"u3"},"body":"c3","createdAt":"2026-09-20T10:03:00Z"},
{"author":{"login":"u4"},"body":"c4","createdAt":"2026-09-20T10:04:00Z"},
{"author":{"login":"u5"},"body":"c5","createdAt":"2026-09-20T10:05:00Z"},
{"author":{"login":"u6"},"body":"c6","createdAt":"2026-09-20T10:06:00Z"}
]}}}}}
OUT
`)
	page, warns := New("github.com", script).Comments(context.Background(),
		model.RepoRef{Project: "acme/widget"}, 42)
	if len(warns) != 0 {
		t.Fatalf("warnings = %v", warns)
	}
	if len(page.Comments) != forge.CommentLimit {
		t.Errorf("comments = %d, want %d", len(page.Comments), forge.CommentLimit)
	}
	if page.Total != 23 {
		t.Errorf("total = %d, want 23 (no se recorta)", page.Total)
	}
	if page.Comments[0].Body != "c0" {
		t.Errorf("el primero debería ser el más antiguo, no %q", page.Comments[0].Body)
	}
}

// TestCommentsInvalidRepo: sin owner/repo no hay query que componer, así que se
// avisa en vez de golpear la API con un literal vacío.
func TestCommentsInvalidRepo(t *testing.T) {
	_, warns := New("github.com", writeScript(t, t.TempDir(), "gh", "#!/bin/sh\nexit 1\n")).
		Comments(context.Background(), model.RepoRef{Project: "widget"}, 42)
	if len(warns) == 0 || warns[0].Kind != "notfound" {
		t.Errorf("warnings = %v, want notfound", warns)
	}
}

// TestCommentsFailureIsWarning: un fallo del forge avisa y no lanza. La ficha
// muestra el aviso y el resto del panel sigue siendo cierto.
func TestCommentsFailureIsWarning(t *testing.T) {
	script := writeScript(t, t.TempDir(), "gh", "#!/bin/sh\necho 'boom' >&2\nexit 1\n")
	page, warns := New("github.com", script).Comments(context.Background(),
		model.RepoRef{Project: "acme/widget"}, 42)
	if len(page.Comments) != 0 {
		t.Errorf("un fallo no debe devolver comentarios: %+v", page.Comments)
	}
	if len(warns) == 0 {
		t.Fatal("un fallo debería avisar")
	}
}
