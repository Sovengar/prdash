package gitlab

import (
	"strings"
	"testing"
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
	if q := authoredQuery(); !strings.Contains(q, "authoredMergeRequests") {
		t.Errorf("authoredQuery = %s", q)
	}
	q := reviewQuery()
	for _, want := range []string{"reviewRequestedMergeRequests", "assignedMergeRequests"} {
		if !strings.Contains(q, want) {
			t.Errorf("reviewQuery no contiene %q", want)
		}
	}
}

func TestToolEnvIsNonInteractive(t *testing.T) {
	joined := strings.Join(toolEnv(), "\n")
	for _, want := range []string{"LC_ALL=C", "GIT_TERMINAL_PROMPT=0", "NO_COLOR=1"} {
		if !strings.Contains(joined, want) {
			t.Errorf("toolEnv no contiene %q", want)
		}
	}
}
