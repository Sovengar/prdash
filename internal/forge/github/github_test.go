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
