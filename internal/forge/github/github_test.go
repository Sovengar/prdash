package github

import (
	"context"
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
