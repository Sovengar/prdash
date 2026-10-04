package tool

import (
	"errors"
	"testing"
)

// The exact error `gh api` produces when the base branch does not exist.
const ghValidationErr = `gh api -X PATCH repos/acme/widget/pulls/7 -f base=main2: ` +
	`gh: Validation Failed (HTTP 422) (exit 1)`

const ghValidationBody = `{"message":"Validation Failed","errors":[{"message":` +
	`"Proposed base branch 'main2' was not found","resource":"PullRequest","field":"base","code":"invalid"}],` +
	`"documentation_url":"https://docs.github.com/rest/pulls/pulls#update-a-pull-request","status":"422"}`

func TestKindDe422EsValidationYNoNetwork(t *testing.T) {
	if got := Kind(errors.New(ghValidationErr)); got != "validation" {
		t.Errorf("Kind = %q, want validation (no network: no hay conflicto que refrescar)", got)
	}
}

// The 422 also has to come out through the code path, which is the one checked first.
func TestKindPorCodigoHTTP(t *testing.T) {
	if got := kindForHTTP(422); got != "validation" {
		t.Errorf("kindForHTTP(422) = %q, want validation", got)
	}
}

// GitHub puts "Validation Failed" in `message` and the offending field in `errors`.
func TestAPIMessagePrefiereElDetalle(t *testing.T) {
	want := "Proposed base branch 'main2' was not found"
	if got := APIMessage(ghValidationBody); got != want {
		t.Errorf("APIMessage = %q, want %q", got, want)
	}
}

func TestAPIMessageCaeAMessage(t *testing.T) {
	if got := APIMessage(`{"message":"404 Not found"}`); got != "404 Not found" {
		t.Errorf("APIMessage = %q, want el message de GitLab", got)
	}
	if got := APIMessage(`{"message":{"base":["is invalid"]}}`); got != "" {
		t.Errorf("APIMessage = %q, want vacío con un message que no es texto", got)
	}
}

// A body that is not JSON, or empty, gives "", which is what lets the caller fall back.
func TestAPIMessageNoSeInventa(t *testing.T) {
	for _, body := range []string{"", "not json", "[]", "{}"} {
		if got := APIMessage(body); got != "" {
			t.Errorf("APIMessage(%q) = %q, want vacío", body, got)
		}
	}
}
