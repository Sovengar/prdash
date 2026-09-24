package gitlab

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"prdash/internal/forge"
	"prdash/internal/testutil"
)

func TestRestEndpointUsesSubfolder(t *testing.T) {
	a := New("gitlab.example.com", "glab", "/git/api/v4/")
	if got := a.restEndpoint("todos"); got != "/git/api/v4/todos" {
		t.Errorf("restEndpoint(todos) = %q", got)
	}
	if got := a.restEndpoint("/merge_requests"); got != "/git/api/v4/merge_requests" {
		t.Errorf("restEndpoint(/merge_requests) = %q", got)
	}
}

func TestRestEndpointWithoutBase(t *testing.T) {
	a := New("gitlab.com", "glab", "")
	if got := a.restEndpoint("todos"); got != "todos" {
		t.Errorf("restEndpoint(todos) = %q", got)
	}
}

func TestNormalizeBase(t *testing.T) {
	if got := normalizeBase("/git/api/v4/"); got != "git/api/v4" {
		t.Errorf("normalizeBase = %q", got)
	}
	if got := normalizeBase("git/api/v4"); got != "git/api/v4" {
		t.Errorf("normalizeBase = %q", got)
	}
}

func TestGraphQLQueries(t *testing.T) {
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
	if q := glMRQuery("grp/proj", 7); !strings.Contains(q, `project(fullPath: "grp/proj")`) || !strings.Contains(q, "mergeRequest(iid: 7)") {
		t.Errorf("glMRQuery = %s", q)
	}
}

func TestConformanceMissingBinary(t *testing.T) {
	bin := filepath.Join(t.TempDir(), "no-glab")
	testutil.RunConformance(t, New("gitlab.example.com", bin, "/git/api/v4/"), testutil.ConformanceOptions{MissingBinary: true})
}

func TestListUnknownSectionReportsUnsupported(t *testing.T) {
	a := New("gitlab.example.com", filepath.Join(t.TempDir(), "no-glab"), "")
	_, warns := a.List(context.Background(), forge.Query{Section: "desconocida"})
	if len(warns) == 0 || warns[0].Kind != "unsupported" {
		t.Fatalf("warnings = %+v", warns)
	}
}
