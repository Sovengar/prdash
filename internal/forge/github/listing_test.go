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

func ghThatLogs(t *testing.T, body string) (script, argsFile string) {
	t.Helper()
	dir := t.TempDir()
	argsFile = filepath.Join(dir, "args.log")
	path := filepath.Join(dir, "gh")
	script = "#!/bin/sh\necho \"$@\" >> \"" + argsFile + "\"\n" + body
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return path, argsFile
}

func loggedArgs(t *testing.T, path string) string {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("no call was logged: %v", err)
	}
	return string(raw)
}

// checksJSON are the checks from `gh pr checks --json name,state,bucket`.
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
func TestChecksAreReadEvenWhenTheExitCodeIsAnError(t *testing.T) {
	script, argsFile := ghThatLogs(t,
		"cat <<'JSON'\n"+checksJSON+"\nJSON\nexit 8\n")
	a := New("github.com", script)

	checks, warns := a.checks(context.Background(), "acme/proy", 42)

	if len(warns) != 0 {
		t.Fatalf("checks gave warnings %+v with parseable JSON: the exit code of "+
			"`gh pr checks` is not the verdict", warns)
	}
	if checks.State != model.ChecksFailing {
		t.Errorf("state = %q, want failing: there is a check in fail", checks.State)
	}
	if checks.Failing != 1 || checks.Pending != 1 || checks.Total != 3 {
		t.Errorf("counts = %+v, want 1/1/3", checks)
	}
	// The command carries the repo and the number, which is how it avoids depending on the cwd.
	args := loggedArgs(t, argsFile)
	for _, want := range []string{"pr checks 42", "--repo acme/proy", "--json"} {
		if !strings.Contains(args, want) {
			t.Errorf("the args do not carry %q: %s", want, args)
		}
	}
}

// Two cases, not one, and the difference is which class the warning carries.
func TestChecksWithoutJSONAreAWarningWithTheirKind(t *testing.T) {
	// The command fails without JSON.
	script, _ := ghThatLogs(t, "echo 'HTTP 403: Forbidden' >&2\nexit 1\n")
	checks, warns := New("github.com", script).checks(context.Background(), "acme/proy", 42)
	if len(warns) != 1 {
		t.Fatalf("without JSON and with a failure it gave %d warnings, want 1", len(warns))
	}
	if warns[0].Kind != "permission" {
		t.Errorf("a 403 gave kind %q, want permission", warns[0].Kind)
	}
	if checks.State != model.ChecksUnknown {
		t.Errorf("a failure gave state %q, want unknown: a CI that was never asked about "+
			"cannot be painted", checks.State)
	}

	script, _ = ghThatLogs(t, "echo 'this is not json'\nexit 0\n")
	_, warns = New("github.com", script).checks(context.Background(), "acme/proy", 42)
	if len(warns) != 1 || warns[0].Kind != "parse" {
		t.Fatalf("warning = %+v, want kind parse", warns)
	}
	if !strings.Contains(warns[0].Msg, "unreadable") {
		t.Errorf("the message %q does not say that the output could not be understood", warns[0].Msg)
	}
}

// The fallback does NOT ask for checks, and that is not an omission.
func TestTheRESTFallbackBringsAuthoredAndNotChecks(t *testing.T) {
	script, argsFile := ghThatLogs(t, "cat <<'JSON'\n"+searchJSON+"\nJSON\n")
	a := New("github.com", script)

	page, ok := a.restAuthored(context.Background())
	if !ok {
		t.Fatal("the REST fallback returned ok=false with a valid response")
	}
	if len(page.Items) != 1 {
		t.Fatalf("brought %d items, want 1", len(page.Items))
	}
	it := page.Items[0]
	if it.Number != 42 || it.Ref.Project == "" {
		t.Errorf("the item was not read well: %+v", it)
	}
	// The stamp: section and forge identity. Without it the fallback returns items that belong
	// nowhere.
	if it.Section != model.SectionAuthored {
		t.Errorf("section = %q, want the authored one", it.Section)
	}
	if it.Forge != ForgeName || it.Host != "github.com" {
		t.Errorf("identity = %s@%s", it.Forge, it.Host)
	}
	if it.Checks.State != model.ChecksUnknown {
		t.Errorf("the fallback brought checks %q, want unknown", it.Checks.State)
	}

	args := loggedArgs(t, argsFile)
	for _, want := range []string{"search/issues", "is:pr is:open author:@me", "per_page"} {
		if !strings.Contains(args, want) {
			t.Errorf("the args do not carry %q: %s", want, args)
		}
	}
}

// Both return false instead of an error, which is what lets the caller fall through.
func TestTheRESTFallbackDoesNotKickInIfItFailsOrIsUnreadable(t *testing.T) {
	// It fails.
	script, _ := ghThatLogs(t, "echo 'boom' >&2\nexit 1\n")
	if _, ok := New("github.com", script).restAuthored(context.Background()); ok {
		t.Error("a failing fallback gave ok=true")
	}

	script, _ = ghThatLogs(t, "echo 'this is not json'\nexit 0\n")
	if _, ok := New("github.com", script).restAuthored(context.Background()); ok {
		t.Error("an unreadable fallback gave ok=true: it is not a fallback, it is a failure")
	}

	script, _ = ghThatLogs(t,
		`echo '{"total_count":0,"incomplete_results":false,"items":[]}'`)
	page, ok := New("github.com", script).restAuthored(context.Background())
	if !ok {
		t.Error("an empty response gave ok=false: you have no PRs and it is not a failure")
	}
	if len(page.Items) != 0 {
		t.Errorf("an empty response brought %d items", len(page.Items))
	}
}

// It warns WITHOUT calling the forge: a reference with no owner is not something to ask about.
func TestReReadingAnItemWithoutAnOwnerWarnsWithoutCallingTheForge(t *testing.T) {
	script, argsFile := ghThatLogs(t, "echo '{\"data\":{}}'\n")
	a := New("github.com", script)

	_, warns := a.ItemState(context.Background(),
		model.RepoRef{Project: "proy"}, 1)
	if len(warns) != 1 || warns[0].Kind != "notfound" {
		t.Fatalf("warning = %+v, want a notfound", warns)
	}
	if !strings.Contains(warns[0].Msg, "invalid repo reference") {
		t.Errorf("the message %q does not say that the reference is invalid", warns[0].Msg)
	}
	if _, err := os.Stat(argsFile); err == nil {
		t.Error("the forge was called with a reference without an owner: the query needs both " +
			"names and would have asked for PR #0 of an empty repository")
	}

	script, argsFile = ghThatLogs(t, "echo '{\"data\":{\"search\":{\"nodes\":[]}}}'\n")
	a = New("github.com", script)
	_, warns = a.ItemState(context.Background(), model.RepoRef{Project: "acme/proy"}, 1)
	if _, err := os.Stat(argsFile); err != nil {
		t.Error("with an owner the forge was not called")
	}
	// A PR that does not exist in a repo that does: also notfound, and the message says so.
	if len(warns) != 1 || warns[0].Kind != "notfound" {
		t.Errorf("a nonexistent PR gave %+v, want notfound", warns)
	}
	if !strings.Contains(warns[0].Msg, "not found") {
		t.Errorf("the message %q does not say that the PR was not found", warns[0].Msg)
	}

	_, warns = New("github.com", "/must-not-be-executed").ItemState(context.Background(),
		model.RepoRef{}, 1)
	if len(warns) != 1 || warns[0].Kind != "notfound" {
		t.Errorf("an empty reference gave %+v, want notfound", warns)
	}
}

// My first version asserted the checks stayed and they did not.
func TestTheGraphQLRollupSurvivesWhenChecksCannotBeRead(t *testing.T) {
	script, _ := ghThatLogs(t, bodyOfFailingChecks())
	it, warns := New("github.com", script).ItemState(context.Background(),
		model.RepoRef{Project: "acme/proy"}, 42)

	if len(warns) != 0 {
		t.Fatalf("ItemState gave warnings %+v: failing checks must not take down the "+
			"reading of the item", warns)
	}
	if it.Number != 42 || it.Title != "PR" {
		t.Fatalf("the item was not read: %+v", it)
	}
	if it.Checks.State != model.ChecksPassing {
		t.Errorf("checks = %q, want the rollup's: a second source that fails cannot "+
			"erase what was already known", it.Checks.State)
	}
}

// This is why `gh pr checks` exists at all: the rollup and the command can disagree.
func TestChecksFromTheSecondSourceOverwriteTheRollup(t *testing.T) {
	body := `case "$*" in
  *"pr checks"*) cat <<'JSON'
` + checksJSON + `
JSON
    exit 8;;
  *) cat <<'JSON'
` + graphqlJSONOfAnItem() + `
JSON
    exit 0;;
esac
exit 1
`
	script, _ := ghThatLogs(t, body)
	it, warns := New("github.com", script).ItemState(context.Background(),
		model.RepoRef{Project: "acme/proy"}, 42)

	if len(warns) != 0 {
		t.Fatalf("warnings = %+v", warns)
	}
	if it.Checks.State != model.ChecksFailing {
		t.Errorf("checks = %q, want failing: the second source wins when it disagrees "+
			"with the rollup", it.Checks.State)
	}
	if it.Checks.Failing != 1 {
		t.Errorf("counts = %+v, want one failing", it.Checks)
	}
}

func bodyOfFailingChecks() string {
	return `case "$*" in
  *"pr checks"*) echo 'nope' >&2; exit 1;;
  *) cat <<'JSON'
` + graphqlJSONOfAnItem() + `
JSON
    exit 0;;
esac
exit 1
`
}

func graphqlJSONOfAnItem() string {
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
