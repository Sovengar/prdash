package testutil

import (
	"context"
	"strings"
	"testing"

	"prdash/internal/forge"
	"prdash/internal/forge/model"
)

// This file tests the test double, which is the opposite of the usual —doubles are not normally
//tested— and the reason is that a double that lies does not fail, it weakens.

func testRef() model.RepoRef {
	return model.RepoRef{Forge: "github", Host: "github.com", Project: "o/r", Owner: "o", Name: "r"}
}

func TestTheDoubleIdentifiesForgeAndHostAndIsAuthenticatedByDefault(t *testing.T) {
	f := &FakeAdapter{ForgeName: "github", HostName: "github.com"}
	if f.Forge() != "github" || f.Host() != "github.com" {
		t.Errorf("Forge/Host gave %q/%q", f.Forge(), f.Host())
	}

	auth := f.Auth(context.Background())
	if !auth.OK {
		t.Error("with no config, Auth gave OK=false: tests that are not about auth " +
			"would show up as degraded")
	}
	if auth.Forge != "github" {
		t.Errorf("the Auth default does not bring the forge name: %q", auth.Forge)
	}

	f.AuthState = model.AuthState{Forge: "gitlab", OK: false, Reason: "expired token"}
	auth = f.Auth(context.Background())
	if auth.OK || auth.Forge != "gitlab" || auth.Reason != "expired token" {
		t.Errorf("the configured state came out altered: %+v", auth)
	}
}

// The two halves matter for different reasons: advancing a page is what makes list pagination
// testable, and repeating the last is what makes a test that does not care stop mid-stream.
func TestListAdvancesPageAndTheLastOneRepeats(t *testing.T) {
	key := FakeKey{Section: model.SectionReview, Kind: model.ReviewRequested}
	first := forge.Page{Items: []model.Item{{Number: 1}}, More: true, Next: "cursor-1"}
	second := forge.Page{Items: []model.Item{{Number: 2}}, More: false}
	f := &FakeAdapter{ForgeName: "github", Pages: map[FakeKey][]forge.Page{key: {first, second}}}
	q := forge.Query{Section: model.SectionReview, ReviewKind: model.ReviewRequested}

	page, _ := f.List(context.Background(), q)
	if len(page.Items) != 1 || page.Items[0].Number != 1 || !page.More || page.Next != "cursor-1" {
		t.Fatalf("the first page gave %+v", page)
	}
	page, _ = f.List(context.Background(), q)
	if len(page.Items) != 1 || page.Items[0].Number != 2 || page.More {
		t.Fatalf("the second page gave %+v", page)
	}
	for i := 0; i < 3; i++ {
		page, _ = f.List(context.Background(), q)
		if len(page.Items) != 1 || page.Items[0].Number != 2 {
			t.Fatalf("page %d after running out gave %+v, want the last one repeated", i+3, page)
		}
	}
	if got := f.ListCallCount(); got != 5 {
		t.Errorf("ListCallCount gave %d, want 5", got)
	}

	other := forge.Query{Section: model.SectionMentions}
	page, warns := f.List(context.Background(), other)
	if len(page.Items) != 0 || len(warns) != 0 || page.More {
		t.Errorf("an unconfigured list gave %+v / %v", page, warns)
	}
}

// The difference between "once" and "always": the TUI refreshes every six seconds.
func TestTheWarningsComeOutOnEveryCall(t *testing.T) {
	key := FakeKey{Section: model.SectionReview, Kind: model.ReviewRequested}
	warns := []model.Warning{{Forge: "github", Kind: "ratelimit", Msg: "wait"}}
	f := &FakeAdapter{
		ForgeName:    "github",
		Pages:        map[FakeKey][]forge.Page{key: {{Items: []model.Item{{Number: 1}}}}},
		ListWarnings: map[FakeKey][]model.Warning{key: warns},
	}
	q := forge.Query{Section: model.SectionReview, ReviewKind: model.ReviewRequested}

	for i := 1; i <= 3; i++ {
		_, got := f.List(context.Background(), q)
		if len(got) != 1 || got[0].Kind != "ratelimit" {
			t.Fatalf("call %d gave %v, want the ratelimit on all of them", i, got)
		}
	}
}

// Two behaviours in one function and both matter: it refuses without a HeadSHA, and it records
// NOTHING, because a merge that did not go asked for no strategy.
func TestTheDoubleRefusesToMergeWithoutAPinAndDoesNotRecordIt(t *testing.T) {
	f := &FakeAdapter{ForgeName: "github"}
	ref := testRef()

	warns := f.Merge(context.Background(), ref, 1, forge.MergeRequest{Mode: forge.Squash})
	if len(warns) != 1 || warns[0].Kind != "unsupported" {
		t.Fatalf("a merge without a pin gave %v, want an unsupported", warns)
	}
	if !strings.Contains(warns[0].Msg, "head") {
		t.Errorf("the warning %q does not say that the head is missing", warns[0].Msg)
	}
	if got := f.ActionCallCount("merge", ref, 1); got != 0 {
		t.Errorf("a refused merge counted %d action calls, want 0", got)
	}
	if got := f.MergeModeCount(forge.Squash); got != 0 {
		t.Errorf("a refused merge counted %d modes, want 0", got)
	}
	if len(f.MergeDeletes) != 0 {
		t.Errorf("a refused merge recorded a delete: %v", f.MergeDeletes)
	}

	warns = f.Merge(context.Background(), ref, 1, forge.MergeRequest{
		Mode: forge.Squash, HeadSHA: "deadbeef", DeleteBranch: true,
	})
	if len(warns) != 0 {
		t.Errorf("a merge with a pin gave warnings %v", warns)
	}
	if got := f.ActionCallCount("merge", ref, 1); got != 1 {
		t.Errorf("ActionCallCount gave %d, want 1", got)
	}
	if got := f.MergeModeCount(forge.Squash); got != 1 {
		t.Errorf("MergeModeCount gave %d, want 1", got)
	}
	if f.MergeDeleteCount(true) != 1 || f.MergeDeleteCount(false) != 0 {
		t.Errorf("MergeDeleteCount gave true=%d false=%d, want 1 and 0",
			f.MergeDeleteCount(true), f.MergeDeleteCount(false))
	}
}

// The order is what makes Retargets a list rather than a counter: the test's question is "which base
// did it move to?".
func TestTheDoubleRefusesRetargetWithoutABranchAndRecordsItInOrder(t *testing.T) {
	f := &FakeAdapter{ForgeName: "github"}
	ref := testRef()

	for _, empty := range []string{"", "   ", "\t"} {
		warns := f.Retarget(context.Background(), ref, 1, empty)
		if len(warns) != 1 || warns[0].Kind != "unsupported" {
			t.Errorf("a retarget to %q gave %v, want an unsupported", empty, warns)
		}
	}
	if f.RetargetCount() != 0 {
		t.Errorf("a refused retarget counted %d, want 0", f.RetargetCount())
	}
	if got := f.ActionCallCount("retarget", ref, 1); got != 0 {
		t.Errorf("a refused retarget counted %d calls, want 0", got)
	}

	for _, branch := range []string{"main", "release/2.0", "develop"} {
		f.Retarget(context.Background(), ref, 1, branch)
	}
	want := []string{"main", "release/2.0", "develop"}
	if f.RetargetCount() != len(want) {
		t.Fatalf("RetargetCount gave %d, want %d", f.RetargetCount(), len(want))
	}
	for i := range want {
		if f.Retargets[i] != want[i] {
			t.Errorf("Retargets[%d] = %q, want %q (complete %v)", i, f.Retargets[i], want[i], f.Retargets)
		}
	}
}

// "Only after" is the half that matters: the merge does a re-read before deciding, and a hook that
// fired on it would corrupt the state being read.
func TestOnMergeOnlyAppliesAfterTheMerge(t *testing.T) {
	ref := testRef()
	key := ItemKey(ref.Project, 1)
	f := &FakeAdapter{
		ForgeName:  "github",
		ItemStates: map[string]model.Item{key: {Number: 1, State: "OPEN", Ref: ref}},
		OnMerge:    func(it *model.Item) { it.State = "MERGED" },
	}

	it, _ := f.ItemState(context.Background(), ref, 1)
	if it.State != "OPEN" {
		t.Fatalf("without a merge it gave %q, want OPEN: the hook is being applied too early", it.State)
	}

	f.Merge(context.Background(), ref, 1, forge.MergeRequest{Mode: forge.Squash, HeadSHA: "deadbeef"})

	it, _ = f.ItemState(context.Background(), ref, 1)
	if it.State != "MERGED" {
		t.Errorf("after the merge it gave %q, want MERGED: the hook is not being applied", it.State)
	}
	if it.Number != 1 {
		t.Errorf("the hook ate the rest of the item: %+v", it)
	}

	clean := &FakeAdapter{ForgeName: "github", ItemStates: map[string]model.Item{key: {Number: 1, State: "OPEN"}}}
	clean.Merge(context.Background(), ref, 1, forge.MergeRequest{Mode: forge.Squash, HeadSHA: "x"})
	it, _ = clean.ItemState(context.Background(), ref, 1)
	if it.State != "OPEN" {
		t.Errorf("without OnMerge it gave %q, want OPEN", it.State)
	}
}

func TestTheStateAndPermissionWarningsComeConfigured(t *testing.T) {
	ref := testRef()
	key := ItemKey(ref.Project, 7)
	if key != "o/r#7" {
		t.Fatalf("ItemKey gave %q, want o/r#7", key)
	}

	f := &FakeAdapter{
		ForgeName: "github",
		ItemStates: map[string]model.Item{
			key: {Number: 7, Title: "one", Ref: ref},
		},
		StateWarnings: map[string][]model.Warning{
			key: {{Kind: "ratelimit", Msg: "wait"}},
		},
		Conversations: map[string]forge.CommentPage{
			key: {Comments: []model.Comment{{Body: "hello"}}, Total: 1},
		},
		CommentWarnings: map[string][]model.Warning{
			key: {{Kind: "network", Msg: "no network"}},
		},
		ActionWarnings: map[string][]model.Warning{
			"approve:o/r#7": {{Kind: "permission", Msg: "no"}},
		},
	}

	it, warns := f.ItemState(context.Background(), ref, 7)
	if it.Title != "one" || len(warns) != 1 || warns[0].Kind != "ratelimit" {
		t.Errorf("ItemState gave %+v / %v", it, warns)
	}
	page, warns := f.Comments(context.Background(), ref, 7)
	if len(page.Comments) != 1 || page.Comments[0].Body != "hello" || warns[0].Kind != "network" {
		t.Errorf("Comments gave %+v / %v", page, warns)
	}
	if warns := f.Approve(context.Background(), ref, 7); len(warns) != 1 || warns[0].Kind != "permission" {
		t.Errorf("Approve gave %v", warns)
	}

	it, warns = f.ItemState(context.Background(), ref, 999)
	if it.Number != 0 || len(warns) != 0 {
		t.Errorf("an unconfigured number gave %+v / %v", it, warns)
	}
}

// The empty page has to carry Total at zero, not a total claiming there are 30 comments.
func TestCommentsUnconfiguredComesResolved(t *testing.T) {
	f := &FakeAdapter{ForgeName: "github"}
	page, warns := f.Comments(context.Background(), testRef(), 1)
	if len(page.Comments) != 0 || len(warns) != 0 {
		t.Errorf("Comments unconfigured gave %+v / %v", page, warns)
	}
	if page.Total != 0 {
		t.Errorf("Comments unconfigured gave Total=%d, want 0", page.Total)
	}
	if got := f.CommentCallCount(); got != 1 {
		t.Errorf("CommentCallCount gave %d, want 1", got)
	}
	f.Comments(context.Background(), testRef(), 1)
	if got := f.CommentCallCount(); got != 2 {
		t.Errorf("CommentCallCount gave %d, want 2", got)
	}
}

func TestBranchesPerProjectAndWithCounter(t *testing.T) {
	ref := testRef()
	other := model.RepoRef{Forge: "github", Host: "github.com", Project: "o/other"}

	f := &FakeAdapter{
		ForgeName:      "github",
		BranchLists:    map[string][]string{"o/r": {"main", "feature"}},
		BranchWarnings: map[string][]model.Warning{"o/other": {{Kind: "unsupported", Msg: "no"}}},
	}

	names, warns := f.Branches(context.Background(), ref)
	if len(names) != 2 || names[0] != "main" || len(warns) != 0 {
		t.Errorf("Branches gave %v / %v", names, warns)
	}
	if got := f.BranchCallCount("o/r"); got != 1 {
		t.Errorf("BranchCallCount gave %d, want 1", got)
	}
	if got := f.BranchCallCount("o/other"); got != 0 {
		t.Errorf("BranchCallCount for a project that was not called gave %d, want 0", got)
	}

	names, warns = f.Branches(context.Background(), other)
	if len(names) != 0 || len(warns) != 1 || warns[0].Kind != "unsupported" {
		t.Errorf("an unconfigured project gave %v / %v", names, warns)
	}
	if got := f.BranchCallCount("o/other"); got != 1 {
		t.Errorf("BranchCallCount gave %d after the call, want 1", got)
	}
}

// Separate counters per action kind, not just per item: one counter would report "1" for three
// different actions.
func TestTheActionCounterSeparatesTheThree(t *testing.T) {
	ref := testRef()
	f := &FakeAdapter{ForgeName: "github"}

	f.Approve(context.Background(), ref, 1)
	f.Approve(context.Background(), ref, 1)
	f.Approve(context.Background(), ref, 2) // another item
	f.Merge(context.Background(), ref, 1, forge.MergeRequest{Mode: forge.Squash, HeadSHA: "a"})
	f.Retarget(context.Background(), ref, 1, "main")

	cases := []struct {
		kind   string
		number int
		want   int
	}{
		{"approve", 1, 2},
		{"approve", 2, 1},
		{"merge", 1, 1},
		{"retarget", 1, 1},
		{"approve", 3, 0},
		{"merge", 2, 0},
		{"invented", 1, 0},
	}
	for _, c := range cases {
		if got := f.ActionCallCount(c.kind, ref, c.number); got != c.want {
			t.Errorf("ActionCallCount(%q, %d) gave %d, want %d", c.kind, c.number, got, c.want)
		}
	}
	f.ActionWarnings = map[string][]model.Warning{
		"approve:o/r#2": {{Kind: "unsupported", Msg: "no"}},
	}
	if warns := f.Approve(context.Background(), ref, 2); len(warns) != 1 {
		t.Errorf("Approve gave %v", warns)
	}
	if warns := f.Merge(context.Background(), ref, 2, forge.MergeRequest{HeadSHA: "x"}); len(warns) != 0 {
		t.Errorf("Merge with someone else's warning on the same key gave %v", warns)
	}
}

func TestTheConformanceSuiteCoversTheWholeContractSurface(t *testing.T) {
	a := &callCounter{FakeAdapter: &FakeAdapter{
		ForgeName: "github",
		HostName:  "github.com",
	}}
	a.Pages = map[FakeKey][]forge.Page{
		{Section: model.SectionAuthored}:                            {{Items: []model.Item{{Number: 1}}}},
		{Section: model.SectionReview, Kind: model.ReviewRequested}: {{Items: []model.Item{{Number: 2}}}},
		{Section: model.SectionReview, Kind: model.ReviewAssigned}:  {{Items: []model.Item{{Number: 3}}}},
		{Section: model.SectionMentions}:                            {{Items: []model.Item{{Number: 4}}}},
	}
	RunConformance(t, a, ConformanceOptions{})

	seen := a.seen()
	expected := []string{
		"List", "ItemState", "Comments", "Approve", "Merge", "Retarget", "Branches",
	}
	for _, m := range expected {
		if seen[m] == 0 {
			t.Errorf("RunConformance did not invoke %s: a method the suite does not touch is a "+
				"method nobody tests", m)
		}
	}
	for _, q := range forge.Streams {
		k := FakeKey{Section: q.Section, Kind: q.ReviewKind}
		if a.Pages[k] == nil && a.lists[k] == 0 {
			t.Errorf("the suite did not ask the stream %+v", q)
		}
	}
	if seen["Auth"] != 0 {
		t.Error("the suite invokes Auth and this test's list did not expect it: review this")
	}
}

type callCounter struct {
	*FakeAdapter
	lists   map[FakeKey]int
	seenMap map[string]int
}

func (c *callCounter) Forge() string { c.mark("Forge"); return c.FakeAdapter.Forge() }
func (c *callCounter) Host() string  { c.mark("Host"); return c.FakeAdapter.Host() }
func (c *callCounter) Auth(ctx context.Context) model.AuthState {
	c.mark("Auth")
	return c.FakeAdapter.Auth(ctx)
}
func (c *callCounter) List(ctx context.Context, q forge.Query) (forge.Page, []model.Warning) {
	c.mark("List")
	if c.lists == nil {
		c.lists = map[FakeKey]int{}
	}
	c.lists[FakeKey{Section: q.Section, Kind: q.ReviewKind}]++
	return c.FakeAdapter.List(ctx, q)
}
func (c *callCounter) ItemState(ctx context.Context, ref model.RepoRef, n int) (model.Item, []model.Warning) {
	c.mark("ItemState")
	return c.FakeAdapter.ItemState(ctx, ref, n)
}
func (c *callCounter) Comments(ctx context.Context, ref model.RepoRef, n int) (forge.CommentPage, []model.Warning) {
	c.mark("Comments")
	return c.FakeAdapter.Comments(ctx, ref, n)
}
func (c *callCounter) Approve(ctx context.Context, ref model.RepoRef, n int) []model.Warning {
	c.mark("Approve")
	return c.FakeAdapter.Approve(ctx, ref, n)
}
func (c *callCounter) Merge(ctx context.Context, ref model.RepoRef, n int, req forge.MergeRequest) []model.Warning {
	c.mark("Merge")
	return c.FakeAdapter.Merge(ctx, ref, n, req)
}
func (c *callCounter) Retarget(ctx context.Context, ref model.RepoRef, n int, b string) []model.Warning {
	c.mark("Retarget")
	return c.FakeAdapter.Retarget(ctx, ref, n, b)
}
func (c *callCounter) Branches(ctx context.Context, ref model.RepoRef) ([]string, []model.Warning) {
	c.mark("Branches")
	return c.FakeAdapter.Branches(ctx, ref)
}

func (c *callCounter) mark(m string) {
	if c.seenMap == nil {
		c.seenMap = map[string]int{}
	}
	c.seenMap[m]++
}

func (c *callCounter) seen() map[string]int { return c.seenMap }
