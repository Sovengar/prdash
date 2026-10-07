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

func TestAuthLoginEmptyWhenUnknown(t *testing.T) {
	dir := t.TempDir()
	script := writeScript(t, dir, "gh", "#!/bin/sh\necho 'github.com'\n")
	if login := New("github.com", script).Auth(context.Background()).Login; login != "" {
		t.Errorf("Login = %q, want empty", login)
	}
}

func TestSearchQueryPagination(t *testing.T) {
	first := searchQuery("author:@me", "")
	if !strings.Contains(first, "is:pr is:open author:@me") {
		t.Errorf("searchQuery does not contain the qualifier:\n%s", first)
	}
	if strings.Contains(first, "after:") {
		t.Errorf("the first page should not carry a cursor:\n%s", first)
	}
	if !strings.Contains(first, "pageInfo") || !strings.Contains(first, "reviewDecision") {
		t.Errorf("searchQuery should ask for pageInfo and reviewDecision:\n%s", first)
	}

	next := searchQuery("mentions:@me", "CURSOR9")
	if !strings.Contains(next, `after: "CURSOR9"`) {
		t.Errorf("the next page should carry the cursor:\n%s", next)
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
		t.Error("an unknown section should not have a qualifier")
	}
}

func TestPrQuery(t *testing.T) {
	q := prQuery("acme", "widget", 42)
	for _, want := range []string{`repository(owner: "acme", name: "widget")`, "pullRequest(number: 42)", "reviewDecision"} {
		if !strings.Contains(q, want) {
			t.Errorf("prQuery does not contain %q:\n%s", want, q)
		}
	}
}

func TestSplitProject(t *testing.T) {
	owner, name := splitProject("acme/widget")
	if owner != "acme" || name != "widget" {
		t.Errorf("splitProject = %q, %q", owner, name)
	}
}

func TestConformanceMissingBinary(t *testing.T) {
	bin := filepath.Join(t.TempDir(), "no-gh")
	testutil.RunConformance(t, New("github.com", bin), testutil.ConformanceOptions{MissingBinary: true})
}

func TestListUnknownSectionReportsUnsupported(t *testing.T) {
	a := New("github.com", filepath.Join(t.TempDir(), "no-gh"))
	_, warns := a.List(context.Background(), forge.Query{Section: "unknown"})
	if len(warns) == 0 || warns[0].Kind != "unsupported" {
		t.Fatalf("warnings = %+v", warns)
	}
}

func TestSearchQueryUsesUnionFragments(t *testing.T) {
	q := searchQuery("author:@me", "")
	for _, want := range []string{"... on PullRequest", "... on CheckRun", "... on StatusContext", "reviewDecision", "statusCheckRollup"} {
		if !strings.Contains(q, want) {
			t.Errorf("searchQuery does not contain %q:\n%s", want, q)
		}
	}
	if p := prQuery("o", "r", 1); !strings.Contains(p, "statusCheckRollup") {
		t.Errorf("prQuery should ask for the checks:\n%s", p)
	}
}

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
		t.Errorf("the REST fallback does not bring branches: %+v", page.Items[0])
	}
	if len(warns) == 0 || warns[0].Kind != "degraded" {
		t.Fatalf("warnings = %+v", warns)
	}
}

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

func writeScript(t *testing.T, dir, name, body string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

// The conversation is asked apart from the inbox and with `last`, because the card shows the end.
func TestCommentsQueryShape(t *testing.T) {
	q := commentsQuery("acme", "widget", 42, commentFetch)
	for _, want := range []string{
		`repository(owner: "acme", name: "widget")`,
		"pullRequest(number: 42)",
		"comments(last: 15)",
		"totalCount",
		"author { login }",
		"body",
		"createdAt",
	} {
		if !strings.Contains(q, want) {
			t.Errorf("commentsQuery does not contain %q:\n%s", want, q)
		}
	}
	if strings.Contains(q, "first:") {
		t.Errorf("commentsQuery should not ask for the start of the conversation:\n%s", q)
	}
	// The margin over the card's limit is what keeps a PR full of bot boilerplate out.
	if commentFetch <= forge.CommentLimit {
		t.Errorf("commentFetch = %d, want > %d to absorb the noise", commentFetch, forge.CommentLimit)
	}
}

func TestCommentsEscapesRepoNames(t *testing.T) {
	if q := commentsQuery(`ac"me`, "wid\\get", 1, 5); !strings.Contains(q, `owner: "ac\"me"`) {
		t.Errorf("the owner was not escaped:\n%s", q)
	}
	if q := commentsQuery("acme", "wid\\get", 1, 5); !strings.Contains(q, `name: "wid\\get"`) {
		t.Errorf("the name was not escaped:\n%s", q)
	}
}

// The query brings more than the card shows and it is trimmed to CommentLimit, by the TAIL.
func TestCommentsCapsAtCardLimitKeepingTotal(t *testing.T) {
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
		t.Errorf("total = %d, want 23 (it is not trimmed)", page.Total)
	}
	want := []string{"c2", "c3", "c4", "c5", "c6"}
	for i, w := range want {
		if page.Comments[i].Body != w {
			t.Errorf("comments[%d] = %q, want %q (the last ones, in order)", i, page.Comments[i].Body, w)
		}
	}
}

func TestCommentsInvalidRepo(t *testing.T) {
	_, warns := New("github.com", writeScript(t, t.TempDir(), "gh", "#!/bin/sh\nexit 1\n")).
		Comments(context.Background(), model.RepoRef{Project: "widget"}, 42)
	if len(warns) == 0 || warns[0].Kind != "notfound" {
		t.Errorf("warnings = %v, want notfound", warns)
	}
}

func TestCommentsFailureIsWarning(t *testing.T) {
	script := writeScript(t, t.TempDir(), "gh", "#!/bin/sh\necho 'boom' >&2\nexit 1\n")
	page, warns := New("github.com", script).Comments(context.Background(),
		model.RepoRef{Project: "acme/widget"}, 42)
	if len(page.Comments) != 0 {
		t.Errorf("a failure must not return comments: %+v", page.Comments)
	}
	if len(warns) == 0 {
		t.Fatal("a failure should warn")
	}
}

// commentFetch is precomputed (a const decl carries no coverage, ADR 0011): this pins the formula against a change of forge.CommentLimit.
func TestCommentFetchKeepsTheMargin(t *testing.T) {
	if commentFetch != 3*forge.CommentLimit {
		t.Errorf("commentFetch = %d, want 3 * forge.CommentLimit = %d", commentFetch, 3*forge.CommentLimit)
	}
}
