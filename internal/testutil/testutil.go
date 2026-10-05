package testutil

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"sync"

	"prdash/internal/forge"
	"prdash/internal/forge/model"
)

type FakeKey struct {
	Section model.Section
	Kind    model.ReviewKind
}

type FakeAdapter struct {
	ForgeName string
	HostName  string
	AuthState model.AuthState

	Pages        map[FakeKey][]forge.Page
	ListWarnings map[FakeKey][]model.Warning

	ItemStates    map[string]model.Item
	StateWarnings map[string][]model.Warning

	// Unconfigured, Comments returns an already-resolved EMPTY conversation, so a test that does not deal
	// with comments does not meet a "loading…" that never ends.
	Conversations   map[string]forge.CommentPage
	CommentWarnings map[string][]model.Warning

	ActionWarnings map[string][]model.Warning

	// Unconfigured, Branches returns an already-resolved empty list, for the same reason: a test that does
	// not talk about branches should not have to invent them to keep the picker from hanging.
	BranchLists    map[string][]string
	BranchWarnings map[string][]model.Warning

	mu           sync.Mutex
	calls        map[FakeKey]int
	listCalls    int
	actionCalls  map[string]int
	commentCalls int
	MergeModes   map[forge.MergeMode]int
	// A list and not a counter because the test's question is "which base did it move to?", which only
	// the order answers.
	Retargets   []string
	branchCalls map[string]int
	// A list and not a counter because the question is "this merge, with or without the delete?", and the
	// list answers it without depending on the order the test fired the calls in.
	MergeDeletes []bool
	// For what a fixed-state fake cannot express: a merge that succeeds AND changes the item, which is
	// the branch-delete case (merged even though the command exits with an error).
	OnMerge func(*model.Item)
	merged  bool
}

var _ forge.Adapter = (*FakeAdapter)(nil)

func (f *FakeAdapter) Forge() string { return f.ForgeName }

func (f *FakeAdapter) Host() string { return f.HostName }

// Authenticated unless configured otherwise.
func (f *FakeAdapter) Auth(_ context.Context) model.AuthState {
	if f.AuthState.Forge == "" && !f.AuthState.OK && f.AuthState.Reason == "" {
		return model.AuthState{Forge: f.ForgeName, OK: true}
	}
	return f.AuthState
}

func (f *FakeAdapter) List(_ context.Context, q forge.Query) (forge.Page, []model.Warning) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.calls == nil {
		f.calls = map[FakeKey]int{}
	}
	key := FakeKey{Section: q.Section, Kind: q.ReviewKind}
	f.listCalls++

	pages := f.Pages[key]
	idx := f.calls[key]
	f.calls[key]++
	if len(pages) == 0 {
		return forge.Page{}, f.ListWarnings[key]
	}
	if idx >= len(pages) {
		idx = len(pages) - 1
	}
	return pages[idx], f.ListWarnings[key]
}

func (f *FakeAdapter) ListCallCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.listCalls
}

// OnMerge applies only to reads AFTER a merge, which is what keeps the pre-merge re-read honest: that
// re-read is the one that decides whether the merge goes.
func (f *FakeAdapter) ItemState(_ context.Context, ref model.RepoRef, number int) (model.Item, []model.Warning) {
	key := ref.Project + "#" + strconv.Itoa(number)
	it := f.ItemStates[key]
	f.mu.Lock()
	after := f.merged
	f.mu.Unlock()
	if after && f.OnMerge != nil {
		f.OnMerge(&it)
	}
	return it, f.StateWarnings[key]
}

func (f *FakeAdapter) Comments(_ context.Context, ref model.RepoRef, number int) (forge.CommentPage, []model.Warning) {
	key := ref.Project + "#" + strconv.Itoa(number)
	f.mu.Lock()
	f.commentCalls++
	f.mu.Unlock()
	if page, ok := f.Conversations[key]; ok {
		return page, f.CommentWarnings[key]
	}
	return forge.CommentPage{}, f.CommentWarnings[key]
}

func (f *FakeAdapter) CommentCallCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.commentCalls
}

func (f *FakeAdapter) Approve(_ context.Context, ref model.RepoRef, number int) []model.Warning {
	return f.record("approve", ref, number)
}

func (f *FakeAdapter) Merge(_ context.Context, ref model.RepoRef, number int, req forge.MergeRequest) []model.Warning {
	// The fake refuses to merge without a pin, like both real adapters, and BEFORE recording anything: a
	// fake that accepted what production rejects would leave the real failure where no test reaches.
	if req.HeadSHA == "" {
		return []model.Warning{{Forge: f.ForgeName, Kind: "unsupported", Msg: forge.ErrMissingHeadSHA.Error()}}
	}
	// Mode and delete recorded separately so a test can assert with which strategy and with which
	// housekeeping the merge was asked for, not only that it was asked for.
	f.mu.Lock()
	if f.MergeModes == nil {
		f.MergeModes = map[forge.MergeMode]int{}
	}
	f.MergeModes[req.Mode]++
	f.MergeDeletes = append(f.MergeDeletes, req.DeleteBranch)
	f.merged = true
	f.mu.Unlock()
	return f.record("merge", ref, number)
}

func (f *FakeAdapter) Retarget(_ context.Context, ref model.RepoRef, number int, branch string) []model.Warning {
	if strings.TrimSpace(branch) == "" {
		return []model.Warning{{Forge: f.ForgeName, Kind: "unsupported", Msg: forge.ErrMissingBaseBranch.Error()}}
	}
	f.mu.Lock()
	f.Retargets = append(f.Retargets, branch)
	f.mu.Unlock()
	return f.record("retarget", ref, number)
}

func (f *FakeAdapter) RetargetCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.Retargets)
}

func (f *FakeAdapter) Branches(_ context.Context, ref model.RepoRef) ([]string, []model.Warning) {
	f.mu.Lock()
	if f.branchCalls == nil {
		f.branchCalls = map[string]int{}
	}
	f.branchCalls[ref.Project]++
	f.mu.Unlock()
	if names, ok := f.BranchLists[ref.Project]; ok {
		return names, f.BranchWarnings[ref.Project]
	}
	return nil, f.BranchWarnings[ref.Project]
}

func (f *FakeAdapter) BranchCallCount(project string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.branchCalls[project]
}

func (f *FakeAdapter) MergeDeleteCount(delete bool) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := 0
	for _, d := range f.MergeDeletes {
		if d == delete {
			n++
		}
	}
	return n
}

func (f *FakeAdapter) MergeModeCount(mode forge.MergeMode) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.MergeModes[mode]
}

func (f *FakeAdapter) record(kind string, ref model.RepoRef, number int) []model.Warning {
	key := kind + ":" + ref.Project + "#" + strconv.Itoa(number)
	f.mu.Lock()
	if f.actionCalls == nil {
		f.actionCalls = map[string]int{}
	}
	f.actionCalls[key]++
	f.mu.Unlock()
	return f.ActionWarnings[key]
}

func (f *FakeAdapter) ActionCallCount(kind string, ref model.RepoRef, number int) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.actionCalls[kind+":"+ref.Project+"#"+strconv.Itoa(number)]
}

func ItemKey(project string, number int) string {
	return project + "#" + strconv.Itoa(number)
}

type ConformanceOptions struct {
	Unsupported   bool
	MissingBinary bool
}

// A gate that cannot be tested does not distinguish a gate from a sign: with the checks in a list
// and not touching the test, a test can hand it a broken adapter and see what comes out.
func RunConformance(t testReport, a forge.Adapter, opts ConformanceOptions) {
	t.Helper()
	reportViolations(a, opts, func(v string) { t.Error(v) })
}

func reportViolations(a forge.Adapter, opts ConformanceOptions, report func(string)) {
	for _, v := range ConformanceViolations(a, opts) {
		report(v)
	}
}

// A list and not an error because there are several independent violations, and reporting only the
// first forces a fix-and-rerun to find the next. A broken adapter has five or six.
func ConformanceViolations(a forge.Adapter, opts ConformanceOptions) []string {
	var violations []string
	if a.Forge() == "" {
		violations = append(violations, "Forge() is empty")
	}
	if a.Host() == "" {
		violations = append(violations, "Host() is empty")
	}
	ctx := context.Background()

	for _, q := range forge.Streams {
		page, warns := a.List(ctx, q)
		if page.More && page.Next == "" {
			violations = append(violations, fmt.Sprintf("%s: List(%v) More=true without Next", a.Forge(), q))
		}
		if opts.Unsupported {
			if !hasKind(warns, "unsupported") {
				violations = append(violations, fmt.Sprintf("%s: List(%v) should report unsupported", a.Forge(), q))
			}
			if len(page.Items) != 0 {
				violations = append(violations, fmt.Sprintf("%s: List(%v) should not return items", a.Forge(), q))
			}
		}
		if opts.MissingBinary {
			if len(page.Items) != 0 {
				violations = append(violations, fmt.Sprintf("%s: List(%v) with no binary should not return items", a.Forge(), q))
			}
			if len(warns) == 0 {
				violations = append(violations, fmt.Sprintf("%s: List(%v) with no binary should return a warning", a.Forge(), q))
			}
		}
	}

	ref := model.RepoRef{Forge: a.Forge(), Host: a.Host(), Project: "o/r", Owner: "o", Name: "r"}
	if _, warns := a.ItemState(ctx, ref, 1); opts.Unsupported && !hasKind(warns, "unsupported") {
		violations = append(violations, fmt.Sprintf("%s: ItemState should report unsupported", a.Forge()))
	}
	if _, warns := a.Comments(ctx, ref, 1); opts.Unsupported && !hasKind(warns, "unsupported") {
		violations = append(violations, fmt.Sprintf("%s: Comments should report unsupported", a.Forge()))
	}
	if warns := a.Approve(ctx, ref, 1); opts.Unsupported && !hasKind(warns, "unsupported") {
		violations = append(violations, fmt.Sprintf("%s: Approve should report unsupported", a.Forge()))
	}
	if warns := a.Merge(ctx, ref, 1, forge.MergeRequest{Mode: forge.Squash, HeadSHA: "deadbeef"}); opts.Unsupported && !hasKind(warns, "unsupported") {
		violations = append(violations, fmt.Sprintf("%s: Merge should report unsupported", a.Forge()))
	}
	if warns := a.Retarget(ctx, ref, 1, "release/2.0"); opts.Unsupported && !hasKind(warns, "unsupported") {
		violations = append(violations, fmt.Sprintf("%s: Retarget should report unsupported", a.Forge()))
	}
	// The picker opens even when the listing came back empty, so what is checked is the warning and
	// that there are no branches: an empty list silently would be a different diagnosis.
	if names, warns := a.Branches(ctx, ref); opts.Unsupported && !hasKind(warns, "unsupported") {
		violations = append(violations, fmt.Sprintf("%s: Branches should report unsupported", a.Forge()))
	} else if len(names) != 0 {
		violations = append(violations, fmt.Sprintf("%s: Branches should not return branches (%v)", a.Forge(), names))
	}
	return violations
}

func hasKind(warns []model.Warning, kind string) bool {
	for _, w := range warns {
		if w.Kind == kind {
			return true
		}
	}
	return false
}
