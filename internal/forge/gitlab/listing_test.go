package gitlab

import (
	"context"
	"strings"
	"testing"

	"prdash/internal/forge"
	"prdash/internal/forge/model"
)

const mrJSON = `[{"iid":12,"title":"MR","description":"desc",
"web_url":"u","state":"opened","source_branch":"feat/a","target_branch":"main",
"updated_at":"2026-09-21T07:00:00Z","created_at":"2026-09-20T07:00:00Z",
"author":{"username":"me"},"labels":["one"],
"references":{"full":"grp/sub/proj!12","short":"!12"},
"assignees":[{"username":"other"}],
"reviewers":[{"username":"me"}],
"detailed_merge_status":"mergeable","merge_status":"can_be_merged",
"diff_stats":{"additions":10,"deletions":2,"changes":3},
"pipeline":{"status":"success"},
"upvotes":1,"downvotes":0,"user_notes_count":2}]`

const graphqlJSON = `{"data":{"currentUser":{"reviewRequestedMergeRequests":{"nodes":[{
	"iid":7,"title":"PR","description":"d","webUrl":"u","state":"opened",
	"sourceBranch":"feat/b","targetBranch":"main",
	"updatedAt":"2026-09-21T07:00:00Z","createdAt":"2026-09-20T07:00:00Z",
	"author":{"username":"me"},"labels":{"nodes":[{"name":"one"}]},
	"assignees":{"nodes":[{"username":"other"}]},
	"reviewers":{"nodes":[{"username":"other"}]},
	"mergeStatus":"MERGEABLE","headRefOid":"deadbeef",
	"diffStats":[{"additions":5,"deletions":1,"changeCount":2}],
	"headPipeline":{"status":"SUCCESS"},"project":{"fullPath":"grp/sub/proj","name":"proj","group":{"fullPath":"grp/sub"}}
}],"pageInfo":{"hasNextPage":true,"endCursor":"Y3Vyc29yOjI="}}}}}`

// The cursor is what stops the TUI from asking for the first page again.
func TestTheGraphQLListingBringsTheForgesCursor(t *testing.T) {
	script, _ := glabThatLogs(t, "cat <<'JSON'\n"+graphqlJSON+"\nJSON\n")
	a := New("h.example", script)

	page, warns := a.List(context.Background(), forge.Query{Section: model.SectionReview})

	if len(warns) != 0 {
		t.Errorf("the good path gave warnings %+v", warns)
	}
	if len(page.Items) != 1 {
		t.Fatalf("%d items came out, want 1: %+v", len(page.Items), page.Items)
	}
	if !page.More {
		t.Error("pageInfo.hasNextPage did not reach More")
	}
	if page.Next != "Y3Vyc29yOjI=" {
		t.Errorf("the cursor is %q, want the forge's endCursor", page.Next)
	}

	it := page.Items[0]
	// The stamp: GraphQL items carry no section and no review kind, the query does not ask for them.
	if it.Section != model.SectionReview {
		t.Errorf("the item ended up in section %q: without a stamp it does not appear in the inbox", it.Section)
	}
	if it.Forge != ForgeName || it.Host != "h.example" {
		t.Errorf("the item ended up as %s@%s, and the host comes from the config", it.Forge, it.Host)
	}
	if it.Ref.Project == "" {
		t.Error("the item brings no project")
	}
}

// The forge's cursor is passed through as given.
func TestTheFirstPageUsesTheCursorAndTheNextOnePassesItThroughAsIs(t *testing.T) {
	script, argsFile := glabThatLogs(t, "cat <<'JSON'\n"+graphqlJSON+"\nJSON\n")
	a := New("h.example", script)

	cursor := "Y3Vyc29yOjI="
	a.List(context.Background(), forge.Query{Section: model.SectionReview, Cursor: cursor})

	args := loggedArgs(t, argsFile)
	if !strings.Contains(args, cursor) {
		t.Errorf("the cursor did not reach the query: %s", args)
	}
	if strings.Contains(args, "after: null") {
		t.Errorf("the query sent an explicit null cursor: %s", args)
	}

	// On the last page: More false and Next empty. Without an empty Next the TUI would keep paging.
	last := strings.Replace(graphqlJSON, `"hasNextPage":true,"endCursor":"Y3Vyc29yOjI="`,
		`"hasNextPage":false,"endCursor":null`, 1)
	script, _ = glabThatLogs(t, "cat <<'JSON'\n"+last+"\nJSON\n")
	a = New("h.example", script)
	page, _ := a.List(context.Background(), forge.Query{Section: model.SectionReview})
	if page.More {
		t.Error("the last page came out with More=true")
	}
	if page.Next != "" {
		t.Errorf("the last page brought cursor %q", page.Next)
	}
}

// Three cases and three different decisions.
func TestTheRESTFallbackOnlyKicksInForTheFirstAuthoredPage(t *testing.T) {
	ctx := context.Background()
	body := `case "$*" in
  *graphql*) echo "boom" >&2; exit 1;;
  *merge_requests*) cat <<'JSON'
` + mrJSON + `
JSON
    exit 0;;
esac
exit 1
`
	script, _ := glabThatLogs(t, body)
	a := New("h.example", script)

	page, warns := a.List(ctx, forge.Query{Section: model.SectionAuthored})
	if len(page.Items) != 1 {
		t.Errorf("the fallback did not return items in the case where it must kick in: %+v", page.Items)
	}
	if len(warns) != 1 || warns[0].Kind != "degraded" {
		t.Fatalf("warnings = %+v, want a single degraded", warns)
	}

	script, _ = glabThatLogs(t, body)
	a = New("h.example", script)
	page, warns = a.List(ctx, forge.Query{Section: model.SectionAuthored, Cursor: "2"})
	if len(page.Items) != 0 {
		t.Errorf("with a cursor the fallback returned %d items: it does not page", len(page.Items))
	}
	if len(warns) != 1 || warns[0].Kind == "degraded" {
		t.Errorf("warnings = %+v: with a cursor the fallback should not kick in", warns)
	}

	script, _ = glabThatLogs(t, body)
	a = New("h.example", script)
	page, warns = a.List(ctx, forge.Query{Section: model.SectionMentions})
	if len(page.Items) != 0 {
		t.Errorf("in mentions the fallback returned %d items from the authored ones", len(page.Items))
	}
	if len(warns) != 1 || warns[0].Kind == "degraded" {
		t.Errorf("warnings = %+v: in mentions it should not kick in", warns)
	}
}

// The failure has to carry a warning even when there is no list to show.
func TestAListingThatCannotBeReadGivesAWarningAttachedToItsSection(t *testing.T) {
	script, _ := glabThatLogs(t, "echo 'it broke' >&2\nexit 1\n")
	a := New("h.example", script)

	page, warns := a.List(context.Background(), forge.Query{Section: model.SectionReview})

	if len(page.Items) != 0 {
		t.Errorf("a failing listing returned %d items", len(page.Items))
	}
	if len(warns) != 1 {
		t.Fatalf("warnings = %+v, want 1: a failure with no warning leaves the inbox blank", warns)
	}
	if warns[0].Section != model.SectionReview {
		t.Errorf("the warning ended up in section %q, want the requested one", warns[0].Section)
	}
	if warns[0].Forge != ForgeName {
		t.Errorf("the warning does not bring the forge name: %+v", warns[0])
	}
	if strings.TrimSpace(warns[0].Msg) == "" {
		t.Error("the warning arrived with no text")
	}
	// NOT "degraded": degraded means "works halfway", and this does not work.
	if warns[0].Kind == "degraded" {
		t.Error("a total failure was marked as degraded: the user would believe they have data")
	}
}

// The case nobody remembers: the fallback only enters if restAuthored actually parses.
func TestTheRESTFallbackDoesNotKickInIfTheResponseIsUnreadable(t *testing.T) {
	body := `case "$*" in
  *graphql*) echo "boom" >&2; exit 1;;
  *merge_requests*) echo 'this is not json'; exit 0;;
esac
exit 1
`
	script, _ := glabThatLogs(t, body)
	a := New("h.example", script)

	page, warns := a.List(context.Background(), forge.Query{Section: model.SectionAuthored})

	if len(page.Items) != 0 {
		t.Errorf("with garbage in the fallback %d items came out", len(page.Items))
	}
	if len(warns) != 1 {
		t.Fatalf("warnings = %+v, want 1", warns)
	}
	if warns[0].Kind == "degraded" {
		t.Error("an unreadable fallback announced itself as degraded: it is not a fallback, it is a failure")
	}
}

// "Comes full" is `total >= pageSize`, not "has items".
func TestTheTodosListOnlyMarksMoreWhenItComesFull(t *testing.T) {
	dir := t.TempDir()
	short := `[{"iid":1,"title":"a","web_url":"u","state":"opened",
"source_branch":"s","target_branch":"main","updated_at":"2026-09-21T07:00:00Z",
"author":{"username":"me"},"references":{"full":"p!1","short":"!1"}}]`
	script := writeScript(t, dir, "glab-1", "#!/bin/sh\ncat <<'JSON'\n"+short+"\nJSON\n")
	a := New("h.example", script)
	page, _ := a.List(context.Background(), forge.Query{Section: model.SectionMentions})
	if page.More {
		t.Error("a short response was marked as More")
	}
	if page.Next != "" {
		t.Errorf("a short response brought cursor %q", page.Next)
	}

	fail := writeScript(t, dir, "glab-2", "#!/bin/sh\necho 'nope' >&2\nexit 1\n")
	a = New("h.example", fail)
	page, warns := a.List(context.Background(), forge.Query{Section: model.SectionMentions})
	if len(page.Items) != 0 || len(warns) != 1 {
		t.Errorf("a Todos failure gave %d items and %d warnings", len(page.Items), len(warns))
	}
}

// The number is passed AS GIVEN and only if it is a positive number.
func TestTheTodosListFollowsThePageCursor(t *testing.T) {
	script, argsFile := glabThatLogs(t, "echo '[]'")
	a := New("h.example", script)
	a.List(context.Background(), forge.Query{Section: model.SectionMentions, Cursor: "3"})
	if args := loggedArgs(t, argsFile); !strings.Contains(args, "todos") {
		t.Errorf("the Todos API was not called: %s", args)
	}

	for _, cursor := range []string{"", "abc", "-1", "0"} {
		script, _ = glabThatLogs(t, "echo '[]'")
		a = New("h.example", script)
		page, warns := a.List(context.Background(),
			forge.Query{Section: model.SectionMentions, Cursor: cursor})
		if len(warns) != 0 {
			t.Errorf("with cursor %q it gave warnings %+v", cursor, warns)
		}
		if len(page.Items) != 0 {
			t.Errorf("with cursor %q it returned items from an empty response", cursor)
		}
	}
}

// The Assigned review listing is a different query from the plain Review one, and only a test
// that asks for it tells the two apart: a negated section or kind silently falls through to the
// other query and the page still parses.
func TestTheAssignedReviewListingAsksForTheAssignedQuery(t *testing.T) {
	script, argsFile := glabThatLogs(t, "cat <<'JSON'\n"+graphqlJSON+"\nJSON\n")
	a := New("h.example", script)

	_, warns := a.List(context.Background(), forge.Query{
		Section:    model.SectionReview,
		ReviewKind: model.ReviewAssigned,
	})

	if len(warns) != 0 {
		t.Errorf("the assigned listing gave warnings %+v", warns)
	}
	args := loggedArgs(t, argsFile)
	if !strings.Contains(args, "assignedMergeRequests") {
		t.Errorf("the assigned listing did not ask for assignedMergeRequests: %s", args)
	}
	if strings.Contains(args, "reviewRequests") && !strings.Contains(args, "assignedMergeRequests") {
		t.Errorf("the assigned listing asked for the plain review query instead: %s", args)
	}
}
