package parse

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"prdash/internal/forge/model"
)

// A real case, not an edge: GitHub's classic status checks use the global state.
func TestARollupWithoutContextsUsesTheGlobalState(t *testing.T) {
	cases := []struct {
		name  string
		state string
		want  model.CheckState
	}{
		{"success", "SUCCESS", model.ChecksPassing},
		{"failure", "FAILURE", model.ChecksFailing},
		{"error", "ERROR", model.ChecksFailing},
		{"pending", "PENDING", model.ChecksPending},
		{"expected", "EXPECTED", model.ChecksPending},
		// Lowercased, because GitHub's global state arrives in upper case but not in every field.
		{"success lowercased", "success", model.ChecksPassing},
		// What is not recognised stays unknown: a rare global state is not turned into "passing".
		{"strange state", "NEUTRAL", model.ChecksUnknown},
		{"empty", "", model.ChecksUnknown},
	}
	for _, c := range cases {
		if got := checksStateFromRollup(c.state); got != c.want {
			t.Errorf("%s: checksStateFromRollup(%q) gave %q, want %q", c.name, c.state, got, c.want)
		}
	}
	// The whole aggregator: a rollup with no contexts and a global state gives the same state, with Total at zero.
	for _, c := range cases {
		got := checksFromSearch(t, commitWithRollup(map[string]any{"state": c.state}))
		if got.State != c.want {
			t.Errorf("%s: gave %q, want %q", c.name, got.State, c.want)
		}
		if got.Total != 0 {
			t.Errorf("%s: gave Total=%d with no contexts, want 0", c.name, got.Total)
		}
	}
}

func TestTheContextsWinOverTheGlobalState(t *testing.T) {
	it := checksFromSearch(t, commitWithRollup(map[string]any{
		"state": "SUCCESS",
		"contexts": map[string]any{"nodes": []any{
			map[string]any{"status": "COMPLETED", "conclusion": "FAILURE", "state": "FAILURE"},
		}},
	}))
	if it.State != model.ChecksFailing {
		t.Errorf("a failing context gave %q, want failing even though the global says SUCCESS", it.State)
	}
	if it.Failing != 1 || it.Total != 1 {
		t.Errorf("the counts ended up %+v", it)
	}

	it = checksFromSearch(t, commitWithRollup(map[string]any{
		"state": "SUCCESS",
		"contexts": map[string]any{"nodes": []any{
			map[string]any{"status": "IN_PROGRESS", "conclusion": "", "state": "IN_PROGRESS"},
		}},
	}))
	if it.State != model.ChecksPending {
		t.Errorf("a running context gave %q, want pending", it.State)
	}

	it = checksFromSearch(t, commitWithRollup(map[string]any{
		"contexts": map[string]any{"nodes": []any{
			map[string]any{"status": "COMPLETED", "conclusion": "SUCCESS", "state": "SUCCESS"},
			map[string]any{"status": "COMPLETED", "conclusion": "SUCCESS", "state": "SUCCESS"},
		}},
	}))
	if it.State != model.ChecksPassing || it.Total != 2 {
		t.Errorf("two green checks gave %+v", it)
	}
}

// Not obvious that a failure should beat a pending.
func TestAFailureWinsOverAPending(t *testing.T) {
	it := checksFromSearch(t, commitWithRollup(map[string]any{
		"contexts": map[string]any{"nodes": []any{
			map[string]any{"status": "IN_PROGRESS", "conclusion": "", "state": "IN_PROGRESS"},
			map[string]any{"status": "COMPLETED", "conclusion": "FAILURE", "state": "FAILURE"},
		}},
	}))
	if it.State != model.ChecksFailing {
		t.Errorf("with a failure and a pending it gave %q, want failing", it.State)
	}
	if it.Failing != 1 || it.Pending != 1 || it.Total != 2 {
		t.Errorf("the counts ended up %+v", it)
	}
}

// Both give UNKNOWN checks, not green ones.
func TestWithoutCommitsOrWithoutRollupThereAreNoChecks(t *testing.T) {
	it := checksFromSearch(t, commitWithRollup(nil))
	if it.State != model.ChecksUnknown {
		t.Errorf("with no rollup it gave %q, want unknown", it.State)
	}
	if it.Total != 0 || it.Failing != 0 || it.Pending != 0 {
		t.Errorf("with no rollup it gave counts %+v", it)
	}
}

func TestGitHubsConclusionsFitInAListAndTheMissingOnesDoNotCount(t *testing.T) {
	failures := []string{"FAILURE", "TIMED_OUT", "CANCELLED", "ACTION_REQUIRED",
		"STARTUP_FAILURE", "STALE", "failure", "Cancelled"}
	for _, c := range failures {
		if !isGHFailure(c, "") {
			t.Errorf("isGHFailure(%q) gave false: a cancelled or timed-out check does not pass", c)
		}
	}
	if !isGHFailure("", "ERROR") || !isGHFailure("", "FAILURE") {
		t.Error("isGHFailure does not look at the state when the conclusion comes empty")
	}
	for _, c := range []string{"", "SUCCESS", "NEUTRAL", "SKIPPED", "NEUTRAL_OLD"} {
		if isGHFailure(c, "") {
			t.Errorf("isGHFailure(%q) gave true", c)
		}
	}

	pendings := []string{"IN_PROGRESS", "QUEUED", "PENDING", "WAITING", "EXPECTED",
		"in_progress", "queued"}
	for _, s := range pendings {
		if !isGHPending(s, "") {
			t.Errorf("isGHPending(%q) gave false", s)
		}
	}
	// A finished check that passes is NOT pending, not even when the status is still queued.
	for _, s := range []string{"COMPLETED", "completed", ""} {
		if isGHPending(s, "SUCCESS") {
			t.Errorf("isGHPending(%q, SUCCESS) gave true: a green check is finished", s)
		}
	}
	// A pending state with an empty status, which is what arrives when the API does not fill it in.
	if !isGHPending("", "PENDING") {
		t.Error("isGHPending with empty status and state PENDING gave false")
	}
	if isGHPending("COMPLETED", "COMPLETED") {
		t.Error("isGHPending of a completed check gave true")
	}
	if isGHPending("", "NEUTRAL") {
		t.Error("isGHPending of a NEUTRAL gave true: neutral is not pending")
	}
}

func TestTheParseErrorUnwrapsTheCause(t *testing.T) {
	cause := errors.New("boom")
	e := &Error{Tool: "gh", Msg: "could not be read", Err: cause}
	if !errors.Is(e, cause) {
		t.Error("errors.Is does not reach the cause")
	}
	if !strings.Contains(e.Error(), "boom") {
		t.Errorf("the text %q does not include the cause", e.Error())
	}
	if !strings.HasPrefix(e.Error(), "parse gh") {
		t.Errorf("the text %q does not start with the tool", e.Error())
	}

	noCause := &Error{Tool: "gl", Msg: "missing field"}
	if noCause.Unwrap() != nil {
		t.Error("Unwrap returned something without a cause")
	}
	text := noCause.Error()
	if strings.Contains(text, "nil") {
		t.Errorf("an error without a cause brings nil in the text: %q", text)
	}
	if text != "parse gl: missing field" {
		t.Errorf("the text gave %q", text)
	}

	var syntaxErr *json.SyntaxError
	_, _, err := ParseGHGraphQLSearch(`this is not json`)
	if err == nil {
		t.Fatal("ParseGHGraphQLSearch with garbage gave nil")
	}
	pe := &Error{}
	if !errors.As(err, &pe) {
		t.Errorf("ParseGH returned %T, not a *parse.Error", err)
	}
	if errors.As(err, &syntaxErr) {
		t.Log("the cause is a SyntaxError, reachable with errors.As")
	} else if !strings.Contains(err.Error(), "unreadable") && !strings.Contains(err.Error(), "parse") {
		t.Errorf("a JSON error does not look like a parse one: %q", err)
	}
}

func checksFromSearch(t *testing.T, commit map[string]any) model.Checks {
	t.Helper()
	node := map[string]any{
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
			"search": map[string]any{"nodes": []any{node}},
		},
	})
	if err != nil {
		t.Fatalf("composing the fixture: %v", err)
	}
	items, _, err := ParseGHGraphQLSearch(string(raw))
	if err != nil {
		t.Fatalf("ParseGHGraphQLSearch(%s): %v", raw, err)
	}
	if len(items) != 1 {
		t.Fatalf("%d items came out, want 1", len(items))
	}
	return items[0].Checks
}

func commitWithRollup(rollup any) map[string]any {
	return map[string]any{"statusCheckRollup": rollup}
}
