package tui

import (
	"strings"
	"testing"
	"time"

	"prdash/internal/forge"
	"prdash/internal/forge/model"
	"prdash/internal/testutil"
	"prdash/internal/worktree"
)

func retargetFixture(t *testing.T, branches ...string) (Model, *testutil.FakeAdapter) {
	t.Helper()
	m, a := openRetargetFixture(t, branches...)
	// The listing is awaited from the channel rather than injected, because the goroutine is the point.
	return send(t, m, waitMount(t, m).(branchesMsg)), a
}

// openRetargetFixture leaves the popup open with the listing not yet arrived.
func openRetargetFixture(t *testing.T, branches ...string) (Model, *testutil.FakeAdapter) {
	t.Helper()
	a := ghAdapter()
	a.BranchLists = map[string][]string{"acme/widget": branches}
	it := mkItem("github", "github.com", "acme/widget", "Add widget", 7, "REVIEW_REQUIRED")
	a.ItemStates = map[string]model.Item{stateKey(it): it}

	m := newTestModel(t, a)
	m = send(t, m, page(1, "github", "github.com", model.SectionReview, model.ReviewRequested,
		[]model.Item{it}, false))

	return press(t, m, "e"), a
}

// The filter is typed letter by letter, as the terminal would.
func pressFilter(t *testing.T, m Model, word string) Model {
	t.Helper()
	for _, r := range word {
		m = press(t, m, string(r))
	}
	return m
}

// `e` opens the popup and what appears are the repository's branches.
func TestEOpensTheSearcherAndBringsTheForgesBranches(t *testing.T) {
	m, a := retargetFixture(t, "main", "develop", "release/2.0")

	if m.retarget.state != retargetChoosing {
		t.Fatalf("popup state = %v, want the searcher", m.retarget.state)
	}
	if a.BranchCallCount("acme/widget") != 1 {
		t.Errorf("Branches = %d calls, want 1", a.BranchCallCount("acme/widget"))
	}
	if m.retarget.view[0] != "main" {
		t.Errorf("the first row is %q, want the current base", m.retarget.view[0])
	}
	if got := strings.Join(m.retarget.view, ","); got != "main,develop,release/2.0" {
		t.Errorf("view = %q, want the three branches", got)
	}
}

// The filter runs on the whole name, not the last segment.
func TestTheFilterKeepsTheBranchesThatContainIt(t *testing.T) {
	m, _ := retargetFixture(t, "main", "fix/hunk-pane-argv", "release/2.0")

	m = pressFilter(t, m, "hunk")
	if got := strings.Join(m.retarget.view, ","); got != "fix/hunk-pane-argv" {
		t.Errorf("view = %q, want only the branch that contains the filter", got)
	}

	m = press(t, m, "backspace")
	if m.retarget.query != "hun" {
		t.Errorf("query = %q, want backspace to remove one character", m.retarget.query)
	}
	m = press(t, m, "ctrl+u")
	if m.retarget.query != "" {
		t.Errorf("query = %q, want ctrl+u to clear it entirely", m.retarget.query)
	}

	m = pressFilter(t, m, "HUNK")
	if got := strings.Join(m.retarget.view, ","); got != "fix/hunk-pane-argv" {
		t.Errorf("view = %q, want the filter ignoring case", got)
	}
}

// What stops the two halves stepping on each other.
func TestJAndKWithAFilterWriteAndNavigate(t *testing.T) {
	m, _ := retargetFixture(t, "main", "develop", "release/2.0", "fix/jj-one")

	m = press(t, m, "j")
	if m.retarget.cursor != 1 {
		t.Fatalf("with an empty filter, `j` did not move the cursor (cursor=%d)", m.retarget.cursor)
	}

	// As soon as the filter has something, `j` types: otherwise `fix/jj-one` is impossible to
	// write.
	m = pressFilter(t, m, "re")
	m = press(t, m, "j")
	if m.retarget.query != "rej" {
		t.Errorf("query = %q, want `j` to type with the filter active", m.retarget.query)
	}
	if m.retarget.cursor != 0 {
		t.Errorf("cursor = %d, want back to the top as soon as the filter changes", m.retarget.cursor)
	}

	m = press(t, m, "ctrl+u")
	m = press(t, m, "down")
	if m.retarget.cursor != 1 {
		t.Errorf("cursor = %d, want `down` to move with an empty filter", m.retarget.cursor)
	}
}

func TestChoosingTheBaseItAlreadyHasSpendsNoCall(t *testing.T) {
	m, a := retargetFixture(t, "main", "develop")

	m = press(t, m, "enter") // main is the current base and the first one out
	if m.retarget.state != retargetClosed {
		t.Errorf("state = %v, want the popup closed", m.retarget.state)
	}
	if a.RetargetCount() != 0 {
		t.Errorf("it called the forge (%v) to change nothing", a.Retargets)
	}
	assertToast(t, m, "already main")
}

// The second press is the confirmation.
func TestTheChangeIsConfirmedBeforeLeaving(t *testing.T) {
	m, a := retargetFixture(t, "main", "release/2.0")

	m = press(t, m, "down")
	m = press(t, m, "enter")
	if m.retarget.state != retargetConfirm {
		t.Fatalf("state = %v, want the confirmation", m.retarget.state)
	}
	if a.RetargetCount() != 0 {
		t.Fatalf("the confirmation already called the forge (%v)", a.Retargets)
	}

	box := stripANSI(m.retargetOverlay2())
	for _, want := range []string{"main", "→", "release/2.0", "enter apply"} {
		if !strings.Contains(box, want) {
			t.Errorf("the confirmation does not say %q:\n%s", want, box)
		}
	}

	m = press(t, m, "enter")
	out := waitOutcome(t, m)
	if !out.OK {
		t.Errorf("the action came out = %+v, want OK", out)
	}
	if out.Base != "release/2.0" {
		t.Errorf("Outcome.Base = %q, want the branch applied for the notice", out.Base)
	}
	if a.RetargetCount() != 1 || a.Retargets[0] != "release/2.0" {
		t.Errorf("the adapter received %v, want release/2.0", a.Retargets)
	}
	if m.retarget.state != retargetClosed {
		t.Errorf("the popup is still open after applying")
	}
	if !m.actionBusy {
		t.Error("the action was not marked as in flight")
	}
}

func (m Model) retargetOverlay2() string {
	box, _ := m.retargetOverlay()
	return box
}

// The likeliest mistake when confirming is picking the wrong row.
func TestEscInTheConfirmationGoesBackToTheList(t *testing.T) {
	m, a := retargetFixture(t, "main", "release/2.0")

	m = press(t, m, "down")
	m = press(t, m, "enter")
	m = press(t, m, "esc")

	if m.retarget.state != retargetChoosing {
		t.Errorf("state = %v, want back to the searcher", m.retarget.state)
	}
	if a.BranchCallCount("acme/widget") != 1 {
		t.Errorf("it asked for the branches again: %d calls", a.BranchCallCount("acme/widget"))
	}
}

// Open, it takes the whole keyboard: a key that leaked through would have `j` approve the
// PR.
func TestThePopupDoesNotLetKeysThroughToTheView(t *testing.T) {
	m, a := retargetFixture(t, "main", "release/2.0")

	m = press(t, m, "a")
	if a.ActionCallCount("approve", mkRef(), 7) != 0 {
		t.Error("a popup key fired an approve on the view")
	}
	if m.retarget.state != retargetChoosing {
		t.Errorf("state = %v, want the popup open", m.retarget.state)
	}
}

func TestTheListingIsCachedPerRepository(t *testing.T) {
	m, a := retargetFixture(t, "main", "release/2.0")
	if a.BranchCallCount("acme/widget") != 1 {
		t.Fatalf("first open = %d calls, want 1", a.BranchCallCount("acme/widget"))
	}

	m = press(t, m, "esc")
	m = press(t, m, "e")
	if m.retarget.state != retargetChoosing {
		t.Fatalf("state = %v, want the searcher open instantly from the cache", m.retarget.state)
	}
	if a.BranchCallCount("acme/widget") != 1 {
		t.Errorf("the second open asked for the branches again: %d calls", a.BranchCallCount("acme/widget"))
	}
}

// A branch created a minute ago has to appear, or the picker lies.
func TestTheCacheExpires(t *testing.T) {
	m, a := retargetFixture(t, "main")
	key := keyOf(m.retargetItemForTest())
	m.branchCache[key] = branchCache{names: []string{"main"}, fetchedAt: time.Now().Add(-branchCacheTTL - time.Second)}

	m = press(t, m, "esc")
	m = press(t, m, "e")
	if m.retarget.state != retargetListing {
		t.Errorf("state = %v, want it to ask again for an expired cache", m.retarget.state)
	}
	waitMount(t, m) // the second request comes out on its own, like the first
	if a.BranchCallCount("acme/widget") != 2 {
		t.Errorf("calls = %d, want 2: an expired cache is requested again", a.BranchCallCount("acme/widget"))
	}
}

func TestAnUnreadableListingExplainsItself(t *testing.T) {
	a := ghAdapter()
	it := mkItem("github", "github.com", "acme/widget", "Add widget", 7, "REVIEW_REQUIRED")
	m := newTestModel(t, a)
	m = send(t, m, page(1, "github", "github.com", model.SectionReview, model.ReviewRequested,
		[]model.Item{it}, false))
	m = press(t, m, "e")

	m = send(t, m, branchesMsg{seq: m.branchSeq, errMsg: "gh: Not Found (HTTP 404)"})

	box := stripANSI(m.retargetOverlay2())
	if !strings.Contains(box, "404") {
		t.Errorf("the popup does not explain the forges failure:\n%s", box)
	}
	if m.retarget.state != retargetListing {
		t.Errorf("state = %v, want to stay waiting with the reason on the view", m.retarget.state)
	}
}

func TestThePopupAppliesToTheItemThatWasConfirmed(t *testing.T) {
	m, a := retargetFixture(t, "main", "release/2.0")
	other := mkItem("github", "github.com", "acme/other", "Other PR", 9, "REVIEW_REQUIRED")
	m = send(t, m, page(1, "github", "github.com", model.SectionReview, model.ReviewRequested,
		[]model.Item{other}, false))
	if !m.selectedIs(other) {
		t.Fatalf("the cursor is not on the other item: the test would prove nothing")
	}

	m = press(t, m, "down") // the popup keeps its keyboard
	m = press(t, m, "enter")
	m = press(t, m, "enter")
	waitOutcome(t, m)

	if a.RetargetCount() != 1 || a.Retargets[0] != "release/2.0" {
		t.Errorf("the adapter received %v, want the change of the confirmed item", a.Retargets)
	}
	if n := a.ActionCallCount("retarget", mkRef(), 7); n != 1 {
		t.Errorf("retarget on acme/widget = %d, want 1 (the popups item, not the cursors)", n)
	}
}

func TestALateListingIsNotPainted(t *testing.T) {
	m, _ := retargetFixture(t, "main", "release/2.0")
	m = press(t, m, "esc") // closed: the in-flight request goes stale

	m = send(t, m, branchesMsg{seq: m.branchSeq, names: []string{"otra/cosa"}})

	if m.retarget.state != retargetClosed {
		t.Errorf("a late listing reopened the popup: state = %v", m.retarget.state)
	}
}

func TestTheNoticeSaysFromWhichBaseToWhich(t *testing.T) {
	m, a := retargetFixture(t, "main", "release/2.0")
	m, out := applyBranchAndMove(t, m, a)
	it, _ := m.selected()
	it.TargetBranch = out.Base

	m = applyActionResult(t, m, forge.Outcome{
		Kind: forge.ActionRetarget, OK: true, HasItem: true, Item: it,
		Base: out.Base, FromBase: "main",
	})
	if !strings.Contains(lastToast(m), "main") || !strings.Contains(lastToast(m), "release/2.0") {
		t.Errorf("the notice = %q, want the two branches", lastToast(m))
	}
}

// Changing the base does not touch the worktree.
func TestTheNoticeWarnsAboutTheStaleWorktree(t *testing.T) {
	m, a := retargetFixture(t, "main", "release/2.0")
	m.SetReviewLookup(fakeLookup{path: "/tmp/wt/prdash-pr-7"})

	m, out := applyBranchAndMove(t, m, a)
	it, _ := m.selected()
	m = applyActionResult(t, m, forge.Outcome{
		Kind: forge.ActionRetarget, OK: true, HasItem: true, Item: it,
		Base: out.Base, FromBase: "main",
	})

	if !strings.Contains(lastToast(m), "mounted review") {
		t.Errorf("the notice = %q, want it to name the out of sync review", lastToast(m))
	}
}

// The warning is for the real case, not decoration on every item.
func TestWithNoReviewMountedThereIsNothingToWarn(t *testing.T) {
	m, a := retargetFixture(t, "main", "release/2.0")
	m.SetReviewLookup(fakeLookup{}) // there is a record, but no review mounted

	m, out := applyBranchAndMove(t, m, a)
	it, _ := m.selected()
	m = applyActionResult(t, m, forge.Outcome{
		Kind: forge.ActionRetarget, OK: true, HasItem: true, Item: it,
		Base: out.Base, FromBase: "main",
	})

	if strings.Contains(lastToast(m), "mounted review") {
		t.Errorf("the notice = %q, want it not to speak of a review that does not exist", lastToast(m))
	}
}

// Without the port injected the action works and only the warning is lost.
func TestWithNoRecordedReviewsThereIsNothingToWarn(t *testing.T) {
	m, a := retargetFixture(t, "main", "release/2.0")
	if m.reviewLookup != nil {
		t.Fatal("the test needs the model without a record: New does not inject it")
	}
	m, out := applyBranchAndMove(t, m, a)
	it, _ := m.selected()
	m = applyActionResult(t, m, forge.Outcome{
		Kind: forge.ActionRetarget, OK: true, HasItem: true, Item: it,
		Base: out.Base, FromBase: "main",
	})

	if !strings.Contains(lastToast(m), "release/2.0") {
		t.Errorf("the notice = %q, want the result of the change", lastToast(m))
	}
}

// The base only changes on an item that is still something to integrate.
func TestANonActionableItemDoesNotOpenThePopup(t *testing.T) {
	a := ghAdapter()
	it := mkItem("github", "github.com", "acme/widget", "Add widget", 7, "APPROVED")
	it.State = "MERGED"
	m := newTestModel(t, a)
	m = send(t, m, page(1, "github", "github.com", model.SectionReview, model.ReviewRequested,
		[]model.Item{it}, false))

	m = press(t, m, "e")

	if m.retarget.state != retargetClosed {
		t.Errorf("state = %v, want it not to open over a merged item", m.retarget.state)
	}
	if a.BranchCallCount("acme/widget") != 0 {
		t.Error("it asked for the branches of an item that is not actionable")
	}
	assertToast(t, m, "already merged")
}

func TestThePopupDoesNotOpenWithTheMergeArmed(t *testing.T) {
	m, a := retargetFixture(t, "main", "release/2.0")
	m = press(t, m, "esc")

	m = press(t, m, "m") // arms the merge
	if !m.mergeArmed {
		t.Fatal("the merge did not arm: the test would prove nothing")
	}
	m = press(t, m, "e")

	if m.retarget.state != retargetClosed {
		t.Errorf("state = %v, want the popup not to open with the merge armed", m.retarget.state)
	}
	if a.BranchCallCount("acme/widget") != 1 {
		t.Errorf("`e` got to ask for branches: %d calls", a.BranchCallCount("acme/widget"))
	}
}

func TestTheKeyCanBeRebound(t *testing.T) {
	a := ghAdapter()
	a.BranchLists = map[string][]string{"acme/widget": {"main", "develop"}}
	m := newTestModel(t, a)
	m.cfg.Keybindings["retarget"] = "T"
	m = send(t, m, page(1, "github", "github.com", model.SectionReview, model.ReviewRequested,
		[]model.Item{mkItem("github", "github.com", "acme/widget", "Add widget", 7, "REVIEW_REQUIRED")}, false))

	m = press(t, m, "T")
	m = send(t, m, waitMount(t, m).(branchesMsg))

	if m.retarget.state != retargetChoosing {
		t.Errorf("state = %v, want the rebind to open the searcher", m.retarget.state)
	}
}

// The reason has to be the forge's, not the CLI's line.
func TestAForgesRejectionIsShownWithoutTheArgv(t *testing.T) {
	m, a := retargetFixture(t, "main", "release/2.0")
	a.ActionWarnings = map[string][]model.Warning{
		"retarget:" + stateKey(m.retarget.item): {{
			Forge: "github", Kind: "validation",
			Msg: "Proposed base branch 'release/2.0' was not found",
		}},
	}

	m, out := applyBranchAndMove(t, m, a)
	m = applyActionResult(t, m, out)

	if out.OK {
		t.Error("a forges rejection cannot come out as OK")
	}
	if out.Conflict {
		t.Errorf("it came out as a conflict: a refresh does not fix the branch name")
	}
	if strings.Contains(lastToast(m), "-X PATCH") {
		t.Errorf("the notice = %q, want the forges reason and not the argv", lastToast(m))
	}
	if !strings.Contains(lastToast(m), "was not found") {
		t.Errorf("the notice = %q, want the forges reason", lastToast(m))
	}
}

type fakeLookup struct{ path string }

func (f fakeLookup) ActiveReview(model.Item) (worktree.Worktree, bool) {
	if f.path == "" {
		return worktree.Worktree{}, false
	}
	return worktree.Worktree{Path: f.path}, true
}

func mkRef() model.RepoRef {
	return model.RepoRef{Forge: "github", Host: "github.com", Project: "acme/widget", Owner: "acme", Name: "widget"}
}

func applyBranchAndMove(t *testing.T, m Model, a *testutil.FakeAdapter) (Model, forge.Outcome) {
	t.Helper()
	m = press(t, m, "down")
	m = press(t, m, "enter")
	m = press(t, m, "enter")
	out := waitOutcome(t, m)
	if a.RetargetCount() != 1 {
		t.Fatalf("the popup never got to apply: %v", a.Retargets)
	}
	return m, out
}

func applyActionResult(t *testing.T, m Model, out forge.Outcome) Model {
	t.Helper()
	return send(t, m, actionMsg{cycle: m.cycle, outcome: out})
}

func (m Model) selectedIs(it model.Item) bool {
	got, ok := m.selected()
	return ok && got.ID() == it.ID()
}

func (m Model) retargetItemForTest() model.Item {
	it, _ := m.selected()
	return it
}
