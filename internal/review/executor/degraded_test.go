package executor

import (
	"context"
	"errors"
	"strings"
	"testing"

	"prdash/internal/cache"
	"prdash/internal/forge/model"
	"prdash/internal/herdr"
	"prdash/internal/review/plan"
	"prdash/internal/worktree"
)

func TestThePlanIsBuiltWithThePlannerWhenThereIsOne(t *testing.T) {
	it := model.NewItem(model.RepoRef{Project: "o/r"}, 7)
	wt := worktree.Worktree{Path: "/wt/pr-7", Branch: "feat/x", Label: "prdash-pr-7"}

	calls := 0
	var receivedItem model.Item
	var receivedWt plan.Worktree
	e := &Executor{
		Planner: func(pr model.Item, pw plan.Worktree) plan.Plan {
			calls++
			receivedItem, receivedWt = pr, pw
			return plan.Plan{Tabs: []plan.Tab{{Label: "MINE"}}}
		},
	}
	got := e.buildPlan(it, wt)
	if calls != 1 {
		t.Errorf("the Planner was called %d times, want 1", calls)
	}
	if len(got.Tabs) != 1 || got.Tabs[0].Label != "MINE" {
		t.Errorf("buildPlan returned %+v, want the Planner's plan", got)
	}
	if receivedItem.Number != 7 {
		t.Errorf("the Planner received item %+v", receivedItem)
	}
	// The worktree arrives mapped to the three fields the plan uses to build the argv.
	for name, value := range map[string]string{
		"Path": receivedWt.Path, "Branch": receivedWt.Branch, "Label": receivedWt.Label,
	} {
		if value == "" {
			t.Errorf("the Planner received the worktree with %s empty: %+v", name, receivedWt)
		}
	}
	if receivedWt.Path != wt.Path || receivedWt.Branch != wt.Branch || receivedWt.Label != wt.Label {
		t.Errorf("the worktree did not arrive intact at the Planner: %+v", receivedWt)
	}

	withoutPlanner := &Executor{
		Tools: plan.Tools{Tuicr: plan.Tool{Argv: []string{"tuicr"}}},
		Env:   plan.Env{Available: map[string]bool{}},
	}
	if got := withoutPlanner.buildPlan(it, wt); len(got.Tabs) == 0 {
		t.Errorf("without a Planner, buildPlan returned a plan with no tabs: %+v", got)
	}
}

func TestWithoutHerdrTheWorktreeIsStillMountedAndWarned(t *testing.T) {
	wt := worktree.Worktree{Path: "/wt", Branch: "b", Label: "prdash-pr-1"}
	res := Result{}

	e := &Executor{}
	if e.mountLayout(context.Background(), wt, plan.Plan{}, &res) {
		t.Error("without Herdr the layout was reported mounted")
	}
	if len(res.Warnings) != 1 {
		t.Fatalf("warnings = %v, want 1", res.Warnings)
	}
	if !strings.Contains(res.Warnings[0], "Herdr") || !strings.Contains(res.Warnings[0], "worktree") {
		t.Errorf("the warning %q has to say that Herdr is missing and that the worktree stays", res.Warnings[0])
	}

	unavailable := &Executor{Herdr: &fakeHerdr{available: false}}
	res = Result{}
	if unavailable.mountLayout(context.Background(), wt, plan.Plan{}, &res) {
		t.Error("with Herdr unavailable the layout was reported mounted")
	}
	if len(res.Warnings) != 1 {
		t.Errorf("warnings = %v, want 1", res.Warnings)
	}
}

type failingHerdr struct {
	err   error
	warns []string
}

func (h *failingHerdr) Available() bool { return true }

func (h *failingHerdr) MountLayout(context.Context, herdr.Container, plan.Plan) ([]string, error) {
	return h.warns, h.err
}

func (h *failingHerdr) Notify(context.Context, string, herdr.NotifyOptions) error { return nil }

func TestTheLayoutContributesItsWarningsAndTheFailureAddsMore(t *testing.T) {
	wt := worktree.Worktree{Path: "/wt", Branch: "b", Label: "prdash-pr-1"}
	ctx := context.Background()

	fake := &fakeHerdr{available: true}
	e := &Executor{Herdr: fake}
	res := Result{}
	if !e.mountLayout(ctx, wt, plan.Plan{}, &res) {
		t.Error("with Herdr available it did not mount")
	}
	if len(fake.notified) != 1 {
		t.Errorf("notifications = %v, want 1", fake.notified)
	}
	if len(res.Warnings) != 0 {
		t.Errorf("the happy path gave warnings %v", res.Warnings)
	}
	// The container reaching the layout is the worktree's, which connects the pane to the right directory.
	if fake.container.WorkspaceID != wt.WorkspaceID || fake.container.PaneID != wt.RootPaneID {
		t.Errorf("the container reaching the layout is not the worktree's: %+v", fake.container)
	}

	withWarnings := &Executor{Herdr: &failingHerdr{warns: []string{"hunk is not installed"}}}
	res = Result{Warnings: []string{"previous warning"}}
	if !withWarnings.mountLayout(ctx, wt, plan.Plan{}, &res) {
		t.Error("a layout with non-fatal warnings must not be considered failed")
	}
	if len(res.Warnings) != 2 {
		t.Fatalf("warnings = %v, want both", res.Warnings)
	}
	if res.Warnings[0] != "previous warning" {
		t.Errorf("the previous warnings were lost: %v", res.Warnings)
	}
	if !strings.Contains(res.Warnings[1], "hunk") {
		t.Errorf("the layout's warning did not arrive: %v", res.Warnings)
	}

	// On failure: the layout's warnings plus the failure's, and the two texts have to be distinguishable.
	withFailure := &Executor{Herdr: &failingHerdr{err: errors.New("the socket does not respond")}}
	res = Result{}
	if withFailure.mountLayout(ctx, wt, plan.Plan{}, &res) {
		t.Error("a layout that failed was reported as mounted")
	}
	if len(res.Warnings) == 0 {
		t.Fatal("the layout failure left no warning")
	}
	joined := strings.Join(res.Warnings, " | ")
	if !strings.Contains(joined, "layout") {
		t.Errorf("the warning %q does not say the layout failed", joined)
	}
	if strings.Contains(joined, "requires HERDR_ENV") {
		t.Errorf("the warning %q says Herdr is missing, which it was not: it sends the user off "+
			"to look for a problem that does not exist", joined)
	}
}

// The condition is `created`, not whether the bare exists.
func TestCleanupBareOnlyIfCreatedInThisCall(t *testing.T) {
	it := model.NewItem(model.RepoRef{Project: "o/r"}, 1)

	r := &recordingResolver{}
	e := &Executor{Resolver: r}
	e.cleanupBare(false, it)
	if r.removed != 0 {
		t.Errorf("with created=false it removed %d clones: it would take down the clone of another "+
			"tab of the same PR", r.removed)
	}

	e.cleanupBare(true, it)
	if r.removed != 1 {
		t.Errorf("with created=true it removed %d clones, want 1", r.removed)
	}
}

type recordingResolver struct {
	fakeRepoResolver
	removed int
}

func (r *recordingResolver) RemoveBare(model.RepoRef) error {
	r.removed++
	return nil
}

type fakeRepoResolver struct{}

func (fakeRepoResolver) ResolveLocal(model.RepoRef) (string, bool) { return "", false }
func (fakeRepoResolver) HasBare(model.RepoRef) bool                { return false }
func (fakeRepoResolver) EnsureBare(context.Context, model.RepoRef) (string, error) {
	return "", nil
}
func (fakeRepoResolver) RemoveBare(model.RepoRef) error { return nil }
func (fakeRepoResolver) FetchReviewRef(context.Context, string, model.Item) (string, error) {
	return "", nil
}
func (fakeRepoResolver) WorktreePath(model.RepoRef, int) string            { return "" }
func (fakeRepoResolver) Remember(model.RepoRef, string)                    {}
func (fakeRepoResolver) RecordReview(model.Item, cache.ReviewRecord) error { return nil }
func (fakeRepoResolver) ActiveReview(model.ID) (cache.ReviewRecord, bool) {
	return cache.ReviewRecord{}, false
}
func (fakeRepoResolver) ForgetReview(model.Item) error { return nil }

func TestLabelCarriesTheNumberAndPrefixThatMarkOwnership(t *testing.T) {
	if got := Label(7); got != "prdash-pr-7" {
		t.Errorf("Label(7) gave %q", got)
	}
	// Two different numbers give two different labels, which stops two reviews from colliding.
	if Label(7) == Label(8) {
		t.Error("two numbers gave the same label")
	}
	// The label is what Owned recognises: the link between the name and the provisioning.
	if !strings.HasPrefix(Label(1), worktree.LabelPrefix) {
		t.Errorf("Label(1) = %q does not start with %q: Owned would not recognise it",
			Label(1), worktree.LabelPrefix)
	}
}
