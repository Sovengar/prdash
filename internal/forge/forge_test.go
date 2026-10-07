package forge_test

import (
	"context"
	"strings"
	"testing"

	"prdash/internal/forge"
	"prdash/internal/forge/model"
	"prdash/internal/state"
	"prdash/internal/testutil"
)

func mkItem(forgeName, host, project string, number int) model.Item {
	return model.NewItem(model.RepoRef{Forge: forgeName, Host: host, Project: project, Owner: "acme", Name: "widget"}, number)
}

func TestCollectPagesThroughAllStreams(t *testing.T) {
	key := testutil.FakeKey{Section: model.SectionAuthored}
	fake := &testutil.FakeAdapter{
		ForgeName: "github",
		HostName:  "github.com",
		Pages: map[testutil.FakeKey][]forge.Page{
			key: {
				{Items: []model.Item{mkItem("github", "github.com", "acme/widget", 1)}, Next: "c1", More: true},
				{Items: []model.Item{mkItem("github", "github.com", "acme/widget", 2)}, More: false},
			},
		},
	}

	res := forge.Collect(context.Background(), fake)

	if len(res.Authored) != 2 {
		t.Fatalf("authored = %d, want 2 (two pages)", len(res.Authored))
	}
	if fake.ListCallCount() < len(forge.Streams) {
		t.Errorf("List was called %d times, want >= %d", fake.ListCallCount(), len(forge.Streams))
	}
}

func TestCollectAuthFailureDoesNotDropData(t *testing.T) {
	fake := &testutil.FakeAdapter{
		ForgeName: "gitlab",
		HostName:  "gitlab.example.com",
		AuthState: model.AuthState{Forge: "gitlab", OK: false, Reason: "401"},
		Pages: map[testutil.FakeKey][]forge.Page{
			{Section: model.SectionAuthored}: {{Items: []model.Item{mkItem("gitlab", "gitlab.example.com", "grp/proj", 4)}}},
		},
	}

	res := forge.Collect(context.Background(), fake)

	if len(res.Authored) != 1 {
		t.Errorf("authored = %d, want 1", len(res.Authored))
	}
	assertKind(t, res.Warnings, "auth")
}

func TestStreamEmitsPages(t *testing.T) {
	key := testutil.FakeKey{Section: model.SectionReview, Kind: model.ReviewRequested}
	fake := &testutil.FakeAdapter{
		ForgeName: "github",
		HostName:  "github.com",
		Pages: map[testutil.FakeKey][]forge.Page{
			key: {
				{Items: []model.Item{mkItem("github", "github.com", "acme/widget", 1)}, Next: "c1", More: true},
				{Items: []model.Item{mkItem("github", "github.com", "acme/widget", 2)}, More: false},
			},
		},
	}

	var firsts, nexts int
	forge.Stream(context.Background(), fake, func(p forge.PageResult) bool {
		if p.Query.Section != model.SectionReview || p.Query.ReviewKind != model.ReviewRequested {
			return true
		}
		if len(p.Items) == 0 {
			return true
		}
		if p.First {
			firsts++
		} else {
			nexts++
		}
		return true
	})

	if firsts != 1 || nexts != 1 {
		t.Fatalf("firsts=%d nexts=%d, want 1 and 1", firsts, nexts)
	}
}

func TestStreamStopsWhenEmitReturnsFalse(t *testing.T) {
	key := testutil.FakeKey{Section: model.SectionAuthored}
	fake := &testutil.FakeAdapter{
		ForgeName: "github",
		HostName:  "github.com",
		Pages: map[testutil.FakeKey][]forge.Page{
			key: {
				{Items: []model.Item{mkItem("github", "github.com", "acme/widget", 1)}, Next: "c1", More: true},
				{Items: []model.Item{mkItem("github", "github.com", "acme/widget", 2)}, More: false},
			},
		},
	}

	forge.Stream(context.Background(), fake, func(p forge.PageResult) bool {
		return p.Query.Section != model.SectionAuthored
	})

	if got := fake.ListCallCount(); got > len(forge.Streams) {
		t.Fatalf("List was called %d times; the cut should prevent the 2nd authored page", got)
	}
}

func TestRunActionApproveOK(t *testing.T) {
	item := mkItem("github", "github.com", "acme/widget", 1)
	item.State = "OPEN"
	fake := &testutil.FakeAdapter{
		ForgeName:  "github",
		HostName:   "github.com",
		ItemStates: map[string]model.Item{testutil.ItemKey("acme/widget", 1): item},
	}

	out := forge.RunAction(context.Background(), fake, forge.ActionApprove, item.Ref, 1, forge.MergeRequest{Mode: forge.Squash})
	if !out.OK || out.Conflict || out.Perm {
		t.Fatalf("outcome = %+v", out)
	}
	if !out.HasItem {
		t.Error("it should bring back the re-read state")
	}
}

func TestRunActionConflictWhenMerged(t *testing.T) {
	item := mkItem("github", "github.com", "acme/widget", 2)
	item.State = "MERGED"
	fake := &testutil.FakeAdapter{
		ForgeName:  "github",
		HostName:   "github.com",
		ItemStates: map[string]model.Item{testutil.ItemKey("acme/widget", 2): item},
	}

	out := forge.RunAction(context.Background(), fake, forge.ActionMerge, item.Ref, 2, forge.MergeRequest{Mode: forge.Squash})
	if !out.Conflict || out.OK {
		t.Fatalf("outcome = %+v", out)
	}
	if !out.HasItem || out.Item.State != "MERGED" {
		t.Errorf("it should bring back the re-read state: %+v", out.Item)
	}
}

func TestRunActionConflictWhenNotFound(t *testing.T) {
	item := mkItem("gitlab", "gitlab.example.com", "grp/proj", 3)
	fake := &testutil.FakeAdapter{
		ForgeName:     "gitlab",
		HostName:      "gitlab.example.com",
		StateWarnings: map[string][]model.Warning{testutil.ItemKey("grp/proj", 3): {{Forge: "gitlab", Kind: "notfound", Msg: "404"}}},
	}

	out := forge.RunAction(context.Background(), fake, forge.ActionApprove, item.Ref, 3, forge.MergeRequest{Mode: forge.Squash})
	if !out.Conflict {
		t.Fatalf("outcome = %+v", out)
	}
}

func TestRunActionPermissionDisabled(t *testing.T) {
	item := mkItem("gitlab", "gitlab.example.com", "grp/proj", 4)
	fake := &testutil.FakeAdapter{
		ForgeName:      "gitlab",
		HostName:       "gitlab.example.com",
		ItemStates:     map[string]model.Item{testutil.ItemKey("grp/proj", 4): item},
		ActionWarnings: map[string][]model.Warning{"approve:grp/proj#4": {{Forge: "gitlab", Kind: "permission", Msg: "you do not have permission"}}},
	}

	out := forge.RunAction(context.Background(), fake, forge.ActionApprove, item.Ref, 4, forge.MergeRequest{Mode: forge.Squash})
	if !out.Perm || out.OK {
		t.Fatalf("outcome = %+v", out)
	}
}

func TestRunActionUnsupportedDisabled(t *testing.T) {
	item := mkItem("bitbucket", "bitbucket.org", "acme/widget", 5)
	fake := &testutil.FakeAdapter{
		ForgeName:      "bitbucket",
		HostName:       "bitbucket.org",
		ItemStates:     map[string]model.Item{testutil.ItemKey("acme/widget", 5): item},
		ActionWarnings: map[string][]model.Warning{"merge:acme/widget#5": {{Forge: "bitbucket", Kind: "unsupported", Msg: "not supported"}}},
	}

	out := forge.RunAction(context.Background(), fake, forge.ActionMerge, item.Ref, 5, forge.MergeRequest{Mode: forge.Squash})
	if !out.Perm {
		t.Fatalf("outcome = %+v", out)
	}
}

// The safety net for when the local veto is not visible, e.g. on items the user did not author.
func TestRunActionSelfReviewDenied(t *testing.T) {
	item := mkItem("github", "github.com", "acme/widget", 6)
	fake := &testutil.FakeAdapter{
		ForgeName:  "github",
		HostName:   "github.com",
		ItemStates: map[string]model.Item{testutil.ItemKey("acme/widget", 6): item},
		ActionWarnings: map[string][]model.Warning{"approve:acme/widget#6": {{
			Forge: "github", Kind: "selfreview",
			Msg: "gh pr review 6 --approve: failed to create review: GraphQL: Review Can not approve your own pull request (exit 1)",
		}}},
	}

	out := forge.RunAction(context.Background(), fake, forge.ActionApprove, item.Ref, 6, forge.MergeRequest{Mode: forge.Squash})
	if !out.Perm || out.OK || out.Conflict {
		t.Fatalf("outcome = %+v", out)
	}
	if out.Msg != state.SelfReviewReason {
		t.Errorf("Msg = %q, want %q (not the CLI stderr)", out.Msg, state.SelfReviewReason)
	}
}

// A rejection because the branches collide is not a state conflict; confusing them made the UI promise a refresh that fixes nothing.
func TestRunActionUnmergeableIsNotAConflict(t *testing.T) {
	item := mkItem("github", "github.com", "acme/widget", 7)
	item.HeadSHA = "abc1234" // without a pin the merge does not go through, and that is not what is being tested
	fake := &testutil.FakeAdapter{
		ForgeName:  "github",
		HostName:   "github.com",
		ItemStates: map[string]model.Item{testutil.ItemKey("acme/widget", 7): item},
		ActionWarnings: map[string][]model.Warning{"merge:acme/widget#7": {{
			Forge: "github", Kind: "unmergeable",
			Msg: "gh pr merge 7: × Pull request acme/widget#7 is not mergeable: the merge commit cannot be cleanly created. (exit 1)",
		}}},
	}

	out := forge.RunAction(context.Background(), fake, forge.ActionMerge, item.Ref, 7, forge.MergeRequest{Mode: forge.Squash})
	if !out.Unmergeable {
		t.Fatalf("Unmergeable = false, outcome = %+v", out)
	}
	if out.OK || out.Conflict {
		t.Errorf("a rejection because of branches is not a state conflict: %+v", out)
	}
	if out.Perm {
		t.Error("Perm = true: a rebase fixes it, the item is not denied forever")
	}
	if out.Msg != state.UnmergeableReason {
		t.Errorf("Msg = %q, want %q (not the CLI stderr)", out.Msg, state.UnmergeableReason)
	}
}

func TestRunActionConflictStaysAConflict(t *testing.T) {
	item := mkItem("github", "github.com", "acme/widget", 8)
	item.HeadSHA = "abc1234"
	fake := &testutil.FakeAdapter{
		ForgeName:  "github",
		HostName:   "github.com",
		ItemStates: map[string]model.Item{testutil.ItemKey("acme/widget", 8): item},
		ActionWarnings: map[string][]model.Warning{"merge:acme/widget#8": {{
			Forge: "github", Kind: "conflict", Msg: "gh pr merge 8: Pull request is already merged (exit 1)",
		}}},
	}

	out := forge.RunAction(context.Background(), fake, forge.ActionMerge, item.Ref, 8, forge.MergeRequest{Mode: forge.Squash})
	if !out.Conflict {
		t.Fatalf("Conflict = false, outcome = %+v", out)
	}
	if out.Unmergeable {
		t.Error("Unmergeable = true: this is resolved by refreshing")
	}
}

func assertKind(t *testing.T, warns []model.Warning, kind string) {
	t.Helper()
	for _, w := range warns {
		if w.Kind == kind {
			return
		}
	}
	t.Fatalf("no warning of kind %q in %+v", kind, warns)
}

func TestCollectStopsOnRateLimit(t *testing.T) {
	key := testutil.FakeKey{Section: model.SectionAuthored}
	fake := &testutil.FakeAdapter{
		ForgeName: "github",
		HostName:  "github.com",
		Pages: map[testutil.FakeKey][]forge.Page{
			key: {
				{Items: []model.Item{mkItem("github", "github.com", "acme/widget", 1)}, Next: "c1", More: true},
				{Items: []model.Item{mkItem("github", "github.com", "acme/widget", 2)}, More: false},
			},
		},
		ListWarnings: map[testutil.FakeKey][]model.Warning{
			key: {{Forge: "github", Kind: "ratelimit", Msg: "429"}},
		},
	}

	res := forge.Collect(context.Background(), fake)

	if fake.ListCallCount() != len(forge.Streams) {
		t.Fatalf("List was called %d times; with a rate limit it should stop at the first page", fake.ListCallCount())
	}
	assertKind(t, res.Warnings, "ratelimit")
}

func TestEscapeGraphQL(t *testing.T) {
	in := "x\"y\\z\nw\tv\ru"
	out := forge.EscapeGraphQL(in)
	if strings.ContainsAny(out, "\n\t\r") {
		t.Fatalf("no raw control may remain: %q", out)
	}
	for _, want := range []string{`\"`, `\\`, `\n`, `\t`, `\r`} {
		if !strings.Contains(out, want) {
			t.Errorf("escape %q missing in %q", want, out)
		}
	}
}
