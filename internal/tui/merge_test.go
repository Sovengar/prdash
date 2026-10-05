package tui

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"prdash/internal/forge"
	"prdash/internal/forge/model"
	"prdash/internal/testutil"
	"prdash/internal/worktree"
)

type mergeFixture struct {
	m     Model
	adp   *testutil.FakeAdapter
	items []model.Item
}

func mergeItems() []model.Item {
	return []model.Item{
		mkItem("github", "github.com", "acme/widget", "Add widget", 1, ""),
		mkItem("github", "github.com", "acme/widget", "Another change", 2, ""),
	}
}

func newMergeFixture(t *testing.T, items ...model.Item) mergeFixture {
	t.Helper()
	adp := ghAdapter()
	adp.ItemStates = map[string]model.Item{}
	adp.StateWarnings = map[string][]model.Warning{}
	for _, it := range items {
		adp.ItemStates[stateKey(it)] = it
	}
	m := newTestModel(t, adp)
	m = send(t, m, page(1, "github", "github.com", model.SectionReview, model.ReviewRequested, items, false))
	return mergeFixture{m: m, adp: adp, items: items}
}

func stateKey(it model.Item) string {
	return it.Ref.Project + "#" + strconv.Itoa(it.Number)
}

func waitOutcome(t *testing.T, m Model) forge.Outcome {
	t.Helper()
	ev := waitMount(t, m)
	msg, ok := ev.(actionMsg)
	if !ok {
		t.Fatalf("event %T, want actionMsg", ev)
	}
	return msg.outcome
}

func toasts(m Model) string { return strings.Join(toastTexts(m), " | ") }

func TestMergeArmsOnFirstPress(t *testing.T) {
	f := newMergeFixture(t, mergeItems()...)
	// The cursor need not be on the first row of the page: the rows are ordered.
	armed, ok := f.m.selected()
	if !ok {
		t.Fatal("the fixture needs a selection")
	}

	m := press(t, f.m, "m")
	if !m.mergeArmed {
		t.Fatal("the first keypress should arm the merge")
	}
	if m.actionBusy {
		t.Error("arming should not launch the action yet")
	}
	if m.mergeArmedID != armed.ID() {
		t.Errorf("arming set %+v, want %+v", m.mergeArmedID, armed.ID())
	}
}

func TestMergeArmedShowsConfirmInKeybinds(t *testing.T) {
	f := newMergeFixture(t, mergeItems()...)
	m := press(t, f.m, "m")

	view := stripANSI(m.View().Content)
	for _, want := range []string{"press the mode", "m merge commit", "r rebase", "s squash", "esc cancel"} {
		if !strings.Contains(view, want) {
			t.Errorf("the confirmation does not mention %q\n%s", want, view)
		}
	}
	if strings.Contains(view, "pgup/dn page") {
		t.Errorf("with the merge armed the keybind bar should give way\n%s", view)
	}
}

func TestMergeSecondKeyPicksMode(t *testing.T) {
	for _, tc := range []struct {
		key  string
		want forge.MergeMode
	}{
		{"m", forge.MergeCommit},
		{"r", forge.Rebase},
		{"s", forge.Squash},
	} {
		t.Run(string(tc.want), func(t *testing.T) {
			f := newMergeFixture(t, mergeItems()...)
			m := press(t, f.m, "m")
			m = press(t, m, tc.key)

			if m.mergeArmed {
				t.Error("after the second keypress it should not stay armed")
			}
			out := waitOutcome(t, m)
			if !out.OK {
				t.Fatalf("merge %s no ok: %+v", tc.want, out)
			}
			if out.Mode != tc.want {
				t.Errorf("mode = %q, want %q", out.Mode, tc.want)
			}
			if got := f.adp.MergeModeCount(tc.want); got != 1 {
				t.Errorf("the forge received %d merge(s) as %q, want 1", got, tc.want)
			}
		})
	}
}

func TestMergeSecondKeyOnlyPicksItsOwnMode(t *testing.T) {
	f := newMergeFixture(t, mergeItems()...)
	m := press(t, f.m, "m")
	m = press(t, m, "s")
	_ = waitOutcome(t, m)

	for _, other := range []forge.MergeMode{forge.MergeCommit, forge.Rebase} {
		if n := f.adp.MergeModeCount(other); n != 0 {
			t.Errorf("squash launched %d merge(s) as %q, want 0", n, other)
		}
	}
}

// It ALSO eats the key. It used to delegate to handleKey, which turned a mis-armed merge into an
// approve: `m` then `a` approved the PR.
func TestMergeArmedConsumesOtherKey(t *testing.T) {
	f := newMergeFixture(t, mergeItems()...)
	start := f.m.cursor
	m := press(t, f.m, "m")

	m = press(t, m, "down")
	if m.mergeArmed {
		t.Error("a navigation key should disarm the merge")
	}
	if m.cursor != start {
		t.Errorf("cursor = %d, want %d: the filtered key must not be redispatched", m.cursor, start)
	}
	if m.actionBusy {
		t.Error("disarming should not leave an action in flight")
	}
}

func TestMergeArmedApproveKeyDoesNotApprove(t *testing.T) {
	f := newMergeFixture(t, mergeItems()...)
	m := press(t, f.m, "m")

	m = press(t, m, "a")
	if m.actionBusy {
		t.Error("`a` with the merge armed should not launch any action")
	}
	if n := f.adp.MergeModeCount(forge.Squash) + f.adp.MergeModeCount(forge.Rebase) +
		f.adp.MergeModeCount(forge.MergeCommit); n != 0 {
		t.Errorf("`a` with the merge armed launched %d merge(s), want 0", n)
	}
}

func TestMergeEscCancels(t *testing.T) {
	f := newMergeFixture(t, mergeItems()...)
	m := press(t, f.m, "m")

	m = press(t, m, "esc")
	if m.mergeArmed {
		t.Error("esc should cancel the merge")
	}
	if m.actionBusy {
		t.Error("canceling should not leave an action in flight")
	}
	if !strings.Contains(toasts(m), "cancelled") {
		t.Errorf("canceling should say so, notices = %q", toasts(m))
	}
}

func TestMergeQuittingStillQuits(t *testing.T) {
	f := newMergeFixture(t, mergeItems()...)
	m := press(t, f.m, "m")

	out, _ := m.Update(tea.KeyPressMsg{Code: 'q', Text: "q"})
	if out == nil {
		t.Error("q should keep closing the TUI with the merge armed")
	}
}

func TestMergeArmedGuardsBeforeArming(t *testing.T) {
	m := newTestModel(t, ghAdapter()) // no selection

	m = press(t, m, "m")
	if m.mergeArmed {
		t.Error("with no selection it should not arm")
	}
	if !strings.Contains(toasts(m), "select an item") {
		t.Errorf("it should warn that a selection is missing, notices = %q", toasts(m))
	}
}

func TestMergeArmedRefusesAlreadyDenied(t *testing.T) {
	f := newMergeFixture(t, mergeItems()...)
	m := f.m
	sel, ok := m.selected()
	if !ok {
		t.Fatal("the fixture needs a selection")
	}
	m.denied[sel.ID()] = "the branch is protected"

	m = press(t, m, "m")
	if m.mergeArmed {
		t.Error("an already denied merge should not arm")
	}
	if !strings.Contains(toasts(m), "protected") {
		t.Errorf("it should explain the denial, notices = %q", toasts(m))
	}
}

func TestMergeOnChangedItemAborts(t *testing.T) {
	f := newMergeFixture(t, mergeItems()...)
	m := press(t, f.m, "m")
	armed, _ := m.selected()

	m.cursor = (m.cursor + 1) % len(m.rows())
	if moved, _ := m.selected(); moved.ID() == armed.ID() {
		t.Skip("the fixture has no second item to move to")
	}

	m = press(t, m, "m")
	if m.actionBusy {
		t.Error("it should not merge an item that is no longer where it was armed")
	}
	if m.mergeArmed {
		t.Error("it should disarm after detecting the change")
	}
	if !strings.Contains(toasts(m), "changed") {
		t.Errorf("it should explain the change, notices = %q", toasts(m))
	}
	total := f.adp.MergeModeCount(forge.MergeCommit) +
		f.adp.MergeModeCount(forge.Squash) + f.adp.MergeModeCount(forge.Rebase)
	if total != 0 {
		t.Errorf("no merge should have reached the forge, there are %d", total)
	}
}

func TestMergeNoticeNamesTheMode(t *testing.T) {
	f := newMergeFixture(t, mergeItems()...)
	m := press(t, f.m, "m")
	m = press(t, m, "r")
	out := waitOutcome(t, m)
	m = send(t, m, actionMsg{cycle: m.cycle, outcome: out})

	if !strings.Contains(lastToast(m), "rebase") {
		t.Errorf("the final notice should name the mode, notice = %q", lastToast(m))
	}
}

type fakeRemover struct {
	removed bool
	reason  string
	err     error
	got     model.Item
	calls   int
}

func (f *fakeRemover) RemoveReview(_ context.Context, it model.Item) (bool, string, error) {
	f.got = it
	f.calls++
	return f.removed, f.reason, f.err
}

func mergeOutcome(t *testing.T, remove ReviewRemover) (Model, forge.Outcome) {
	t.Helper()
	f := newMergeFixture(t, mergeItems()...)
	m := f.m
	if remove != nil {
		m.SetReviewRemover(remove)
	}
	m = press(t, m, "m")
	m = press(t, m, "r")
	return m, waitOutcome(t, m)
}

func applyMerge(t *testing.T, remove ReviewRemover) (Model, tea.Cmd) {
	t.Helper()
	m, out := mergeOutcome(t, remove)
	return m, m.applyAction(out, m.cycle)
}

func runCleanup(t *testing.T, m Model, cmd tea.Cmd) (Model, bool) {
	t.Helper()
	if cmd == nil {
		return m, false
	}
	msg, ok := cmd().(reviewCleanupMsg)
	if !ok {
		t.Fatalf("the cleanup command returned %T, want reviewCleanupMsg", cmd())
	}
	return send(t, m, msg), true
}

func TestMergeOKRemovesCleanWorktree(t *testing.T) {
	remove := &fakeRemover{removed: true}
	m, out := mergeOutcome(t, remove)
	cmd := m.applyAction(out, m.cycle)
	if cmd == nil {
		t.Fatal("a successful merge should return the cleanup command")
	}
	m, _ = runCleanup(t, m, cmd)

	if remove.calls != 1 || remove.got.ID() != out.Item.ID() {
		t.Fatalf("remove calls=%d item=%v, want the merged item %v", remove.calls, remove.got.ID(), out.Item.ID())
	}
	toast := lastToast(m)
	if !strings.Contains(toast, "worktree removed") || !strings.Contains(toast, "ok") {
		t.Errorf("notice = %q, want merge ok + worktree removed", toast)
	}
}

func TestMergeOKKeepsDirtyWorktree(t *testing.T) {
	remove := &fakeRemover{reason: worktree.KeptUncommitted}
	m, cmd := applyMerge(t, remove)
	m, _ = runCleanup(t, m, cmd)

	if !strings.Contains(lastToast(m), "merged, but the worktree has uncommitted changes — kept") {
		t.Errorf("notice = %q, want the exact text of the dirty case", lastToast(m))
	}
}

func TestMergeOKKeepsUnreadableWorktree(t *testing.T) {
	remove := &fakeRemover{reason: worktree.KeptUnreadable}
	m, cmd := applyMerge(t, remove)
	m, _ = runCleanup(t, m, cmd)

	if !strings.Contains(lastToast(m), "could not read the worktree status") {
		t.Errorf("notice = %q, want it to say the state could not be checked", lastToast(m))
	}
}

func TestMergeOKWithoutReviewReportsNoError(t *testing.T) {
	remove := &fakeRemover{} // (false, "", nil): no exists review montado
	m, cmd := applyMerge(t, remove)
	if cmd == nil {
		t.Fatal("with an injected remover a successful merge should return a command")
	}
	m, _ = runCleanup(t, m, cmd)

	toast := lastToast(m)
	if strings.Contains(toast, "worktree") || strings.Contains(toast, "could not") {
		t.Errorf("notice = %q, with no review there should be no cleanup notice", toast)
	}
	if !strings.Contains(toast, "ok") {
		t.Errorf("notice = %q, the merge should keep saying it went well", toast)
	}
}

func TestMergeCleanupNoopDoesNotDuplicateToast(t *testing.T) {
	remove := &fakeRemover{} // (false, "", nil)
	m, cmd := applyMerge(t, remove)
	before := len(toastTexts(m))
	m, _ = runCleanup(t, m, cmd)
	if got := len(toastTexts(m)); got != before {
		t.Fatalf("live notices = %d after the no-op, want %d (without duplicating): %q", got, before, toastTexts(m))
	}
}

// reviewCleanupMsg comes from a Cmd, not the events channel.
func TestReviewCleanupMsgDoesNotArmChannelReader(t *testing.T) {
	remove := &fakeRemover{removed: true}
	m, cmd := applyMerge(t, remove)
	msg, ok := cmd().(reviewCleanupMsg)
	if !ok {
		t.Fatalf("the cleanup command returned %T, want reviewCleanupMsg", cmd())
	}

	updated, follow := m.Update(msg)
	if follow != nil {
		t.Fatalf("a Cmd-delivered message must not return a cmd (withPump would arm an extra reader): %T", follow)
	}
	if got := updated.(Model).readers; got != 1 {
		t.Fatalf("readers = %d, quiero 1", got)
	}
}

func TestMergeCleanupRemovedDoesNotDuplicateToast(t *testing.T) {
	remove := &fakeRemover{removed: true}
	m, out := mergeOutcome(t, remove)
	cmd := m.applyAction(out, m.cycle)
	m, _ = runCleanup(t, m, cmd)

	base := actionDoneNotice(out)
	texts := toastTexts(m)
	stale, removed := 0, 0
	for _, txt := range texts {
		if txt == base {
			stale++
		}
		if strings.Contains(txt, "worktree removed") {
			removed++
		}
	}
	if stale != 0 {
		t.Fatalf("an outdated copy of the merge notice is left: %q", texts)
	}
	if removed != 1 {
		t.Fatalf("I want a single notice with 'worktree removed': %q", texts)
	}
}

func TestMergeOKCleanupErrorIsWarn(t *testing.T) {
	remove := &fakeRemover{err: errors.New("boom")}
	m, cmd := applyMerge(t, remove)
	m, _ = runCleanup(t, m, cmd)

	if !strings.Contains(lastToast(m), "could not remove the worktree: boom") {
		t.Errorf("notice = %q, want the reason of the delete failure", lastToast(m))
	}
}

func TestMergeCleanupComposesWithBranchNotDeleted(t *testing.T) {
	remove := &fakeRemover{reason: worktree.KeptUncommitted}
	m, out := mergeOutcome(t, remove)
	out.DeleteMsg = "the protected branch was kept"
	cmd := m.applyAction(out, m.cycle)
	m, _ = runCleanup(t, m, cmd)

	toast := lastToast(m)
	for _, want := range []string{"ok", "branch not deleted", "uncommitted changes — kept"} {
		if !strings.Contains(toast, want) {
			t.Errorf("notice = %q, missing %q", toast, want)
		}
	}
}

func TestReviewCleanupNoticeLevels(t *testing.T) {
	const base = "merge (squash) ok · branch not deleted: protected"
	cases := []struct {
		name      string
		baseLevel noticeLevel
		removed   bool
		reason    string
		err       error
		wantLevel noticeLevel
	}{
		{"deleted on base OK", levelOK, true, "", nil, levelOK},
		{"deleted on base warn", levelWarn, true, "", nil, levelWarn},
		{"kept because dirty", levelOK, false, worktree.KeptUncommitted, nil, levelWarn},
		{"kept because unreadable", levelOK, false, worktree.KeptUnreadable, nil, levelWarn},
		{"delete failure", levelOK, false, "", errors.New("boom"), levelWarn},
		{"no review mounted", levelOK, false, "", nil, levelOK},
		{"no review on base warn", levelWarn, false, "", nil, levelWarn},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, level := reviewCleanupNotice(base, tc.baseLevel, tc.removed, tc.reason, tc.err)
			if level != tc.wantLevel {
				t.Fatalf("level = %d, want %d", level, tc.wantLevel)
			}
		})
	}
}

func TestMergeCleanupRemovedKeepsWarnLevel(t *testing.T) {
	remove := &fakeRemover{removed: true}
	m, out := mergeOutcome(t, remove)
	out.DeleteMsg = "the protected branch was kept"
	cmd := m.applyAction(out, m.cycle)
	m, _ = runCleanup(t, m, cmd)

	if !strings.Contains(lastToast(m), "worktree removed") {
		t.Fatalf("notice = %q, want it to say it was deleted", lastToast(m))
	}
	if got := lastToastLevel(m); got != toastWarning {
		t.Fatalf("level = %d, want warning: the branch was not deleted", got)
	}
}

func TestTriggersReviewCleanup(t *testing.T) {
	yes := []forge.Outcome{
		{Kind: forge.ActionMerge, OK: true},
	}
	no := []forge.Outcome{
		{Kind: forge.ActionMerge, OK: false},
		{Kind: forge.ActionApprove, OK: true},
		{Kind: forge.ActionRetarget, OK: true},
		{Kind: forge.ActionMerge, Conflict: true},
		{Kind: forge.ActionMerge, Unmergeable: true},
		{Kind: forge.ActionMerge, Perm: true},
	}
	for _, out := range yes {
		if !triggersReviewCleanup(out) {
			t.Errorf("a successful merge should trigger the cleanup: %+v", out)
		}
	}
	for _, out := range no {
		if triggersReviewCleanup(out) {
			t.Errorf("it should not trigger the cleanup: %+v", out)
		}
	}
}

func TestCleanupDoesNotTriggerOnOtherOutcomes(t *testing.T) {
	cases := []struct {
		name string
		out  forge.Outcome
	}{
		{"approve", forge.Outcome{Kind: forge.ActionApprove, OK: true}},
		{"retarget", forge.Outcome{Kind: forge.ActionRetarget, OK: true, Base: "main"}},
		{"merge fails", forge.Outcome{Kind: forge.ActionMerge, OK: false, Msg: "boom"}},
		{"conflicto", forge.Outcome{Kind: forge.ActionMerge, Conflict: true, Msg: "changed"}},
		{"no mergeable", forge.Outcome{Kind: forge.ActionMerge, Unmergeable: true, Msg: "rebase"}},
		{"permiso", forge.Outcome{Kind: forge.ActionMerge, Perm: true, Msg: "no"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newMergeFixture(t, mergeItems()...)
			remove := &fakeRemover{removed: true}
			m := f.m
			m.SetReviewRemover(remove)
			if cmd := m.applyAction(tc.out, m.cycle); cmd != nil {
				t.Fatalf("it should not return a cleanup command for %s", tc.name)
			}
			if remove.calls != 0 {
				t.Errorf("remove called %d times, want 0", remove.calls)
			}
		})
	}
}

func TestMergeOKWithoutRemoverStillWorks(t *testing.T) {
	m, cmd := applyMerge(t, nil)
	if cmd != nil {
		t.Fatal("with no remover there should be no cleanup command")
	}
	if !strings.Contains(lastToast(m), "ok") {
		t.Fatalf("notice = %q", lastToast(m))
	}
}

func TestQuitDoesNotRemoveWorktrees(t *testing.T) {
	f := newMergeFixture(t, mergeItems()...)
	remove := &fakeRemover{removed: true}
	m := f.m
	m.SetReviewRemover(remove)

	out, _ := m.Update(tea.KeyPressMsg{Code: 'q', Text: "q"})
	if out == nil {
		t.Fatal("q should keep closing the TUI")
	}
	if remove.calls != 0 {
		t.Errorf("closing the app must not delete anything (calls=%d)", remove.calls)
	}
}

func TestRefreshMergedItemDoesNotTriggerCleanup(t *testing.T) {
	f := newMergeFixture(t, mergeItems()...)
	remove := &fakeRemover{removed: true}
	m := f.m
	m.SetReviewRemover(remove)

	merged := f.items[0]
	merged.State = "MERGED"
	m = send(t, m, page(m.cycle, "github", "github.com", model.SectionReview, model.ReviewRequested, []model.Item{merged}, false))

	if remove.calls != 0 {
		t.Errorf("a refresh must not trigger the cleanup (calls=%d)", remove.calls)
	}
}
