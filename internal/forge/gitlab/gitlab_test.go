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
			t.Errorf("graphqlArgs does not contain %q: %q", want, joined)
		}
	}
}

func TestRESTArgsUseGetAndRelativeEndpoint(t *testing.T) {
	a := New("h.example", "glab")
	joined := strings.Join(a.getArgs("todos", "action=mentioned"), " ")
	for _, want := range []string{"-X GET", " todos", "-f action=mentioned"} {
		if !strings.Contains(joined, want) {
			t.Errorf("getArgs does not contain %q: %q", want, joined)
		}
	}
	if strings.Contains(joined, "/git/api/v4") || strings.Contains(joined, "/api/v4") {
		t.Errorf("it must not build the absolute path: %q", joined)
	}
}

func TestMRActionArgs(t *testing.T) {
	a := New("h.example", "glab")
	if got := strings.Join(a.mrArgs("merge", 7, "grp/proj", "--yes"), " "); !strings.Contains(got, "mr merge 7 -R grp/proj --yes") {
		t.Errorf("mrArgs = %q", got)
	}
}

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
		t.Errorf("the paginated query should carry the cursor: %s", q)
	}
	// The iid is a string literal: the schema declares String! and GraphQL does not coerce Int to
	// it.
	if q := glMRQuery("grp/proj", 7); !strings.Contains(q, `project(fullPath: "grp/proj")`) || !strings.Contains(q, `mergeRequest(iid: "7")`) {
		t.Errorf("glMRQuery = %s", q)
	}
	if q := glMRQuery("grp/proj", 7); strings.Contains(q, "mergeRequest(iid: 7)") {
		t.Errorf("glMRQuery must not pass the iid as an Int: %s", q)
	}
	if !strings.Contains(mrFields, "approved") || strings.Contains(mrFields, "approvalsLeft") {
		t.Errorf("mrFields should use approved and not approvalsLeft: %s", mrFields)
	}
}

func TestConformanceMissingBinary(t *testing.T) {
	bin := filepath.Join(t.TempDir(), "no-glab")
	testutil.RunConformance(t, New("gitlab.example.com", bin), testutil.ConformanceOptions{MissingBinary: true})
}

func TestListUnknownSectionReportsUnsupported(t *testing.T) {
	a := New("gitlab.example.com", filepath.Join(t.TempDir(), "no-glab"))
	_, warns := a.List(context.Background(), forge.Query{Section: "unknown"})
	if len(warns) == 0 || warns[0].Kind != "unsupported" {
		t.Fatalf("warnings = %+v", warns)
	}
}

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
		t.Fatalf("the real args do not pin host/GET:\n%s", string(log))
	}
}

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

func TestAuthLoginEmptyWhenUnknown(t *testing.T) {
	dir := t.TempDir()
	script := writeScript(t, dir, "glab", "#!/bin/sh\necho 'glab: logged in'\n")
	if login := New("h.example", script).Auth(context.Background()).Login; login != "" {
		t.Errorf("Login = %q, want empty", login)
	}
}

func writeScript(t *testing.T, dir, name, body string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

// The notes are asked apart from the inbox, with `last` and with `system`, which is the only thing
// telling a note written by a person from one the MR left when it opened.
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
			t.Errorf("glNotesQuery does not contain %q:\n%s", want, q)
		}
	}
	if strings.Contains(q, `iid: 42`) {
		t.Errorf("the iid cannot go as an Int (String! rejects it):\n%s", q)
	}
	if commentFetch <= forge.CommentLimit {
		t.Errorf("commentFetch = %d, want > %d", commentFetch, forge.CommentLimit)
	}
}

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
	for _, c := range page.Comments {
		if c.Body == "added 56 commits" {
			t.Errorf("it slipped in a system note: %q", c.Body)
		}
	}
	want := []string{"third human", "fourth human", "fifth human", "sixth human", "seventh human"}
	for i, w := range want {
		if page.Comments[i].Body != w {
			t.Errorf("comments[%d] = %q, want %q (the last ones, in order)", i, page.Comments[i].Body, w)
		}
	}
	// GitLab exposes no count: the total is what was read (7), not what was painted (5).
	if page.Total != 7 {
		t.Errorf("total = %d, want 7 (the notes read, not the painted ones)", page.Total)
	}
}

func TestCommentsEmptyRepo(t *testing.T) {
	script := writeScript(t, t.TempDir(), "glab", "#!/bin/sh\nexit 1\n")
	_, warns := New("gitlab.example.com", script).Comments(context.Background(), model.RepoRef{}, 42)
	if len(warns) == 0 || warns[0].Kind != "notfound" {
		t.Errorf("warnings = %v, want notfound", warns)
	}
}

func TestCommentsFailureIsWarning(t *testing.T) {
	script := writeScript(t, t.TempDir(), "glab", "#!/bin/sh\necho boom >&2\nexit 1\n")
	page, warns := New("gitlab.example.com", script).Comments(context.Background(),
		model.RepoRef{Project: "grupo/proy"}, 42)
	if len(page.Comments) != 0 {
		t.Errorf("a failure must not return comments: %+v", page.Comments)
	}
	if len(warns) == 0 {
		t.Fatal("a failure should warn")
	}
}
