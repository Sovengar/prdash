package tui

import (
	"strings"
	"testing"

	"prdash/internal/forge"
)

func TestDeleteBranchShowsInTheArmedConfirm(t *testing.T) {
	f := newMergeFixture(t, mergeItems()...)

	if view := stripANSI(f.m.View().Content); strings.Contains(view, "delete branch") {
		t.Errorf("with no merge armed the delete should not appear\n%s", view)
	}

	m := press(t, f.m, "m")
	view := stripANSI(m.View().Content)
	if !strings.Contains(view, "delete branch: yes") {
		t.Errorf("the confirmation does not show the default value\n%s", view)
	}
	if !strings.Contains(view, "tab") {
		t.Errorf("the confirmation does not say how to change it\n%s", view)
	}
}

// The merge deletes the branch unless turned off, which is what the forges do on their own.
func TestDeleteBranchIsOnByDefault(t *testing.T) {
	f := newMergeFixture(t, mergeItems()...)
	m := press(t, f.m, "m")
	m = press(t, m, "s")
	out := waitOutcome(t, m)

	if !out.OK {
		t.Fatalf("outcome = %+v", out)
	}
	if !out.DeleteBranch {
		t.Error("the merge should ask to delete the branch by default")
	}
	if got := f.adp.MergeDeleteCount(true); got != 1 {
		t.Errorf("the adapter received %d merge(s) with delete, want 1", got)
	}
}

// tab changes the value and executes NOTHING. Disarming would make the keypress pointless.
func TestTabTogglesTheDeleteWithoutDisarming(t *testing.T) {
	f := newMergeFixture(t, mergeItems()...)
	m := press(t, f.m, "m")
	m = press(t, m, "tab")

	if !m.mergeArmed {
		t.Fatal("`tab` should not disarm the merge")
	}
	if m.actionBusy {
		t.Fatal("`tab` should not fire the action")
	}
	if m.deleteBranch {
		t.Error("`tab` should have turned the delete off")
	}
	if view := stripANSI(m.View().Content); !strings.Contains(view, "delete branch: no") {
		t.Errorf("the box does not reflect the new value\n%s", view)
	}
}

func TestTabToggleReachesTheAdapter(t *testing.T) {
	f := newMergeFixture(t, mergeItems()...)
	m := press(t, f.m, "m")
	m = press(t, m, "tab")
	m = press(t, m, "s")
	out := waitOutcome(t, m)

	if out.DeleteBranch {
		t.Error("the merge should not ask to delete the branch after turning it off")
	}
	if got := f.adp.MergeDeleteCount(false); got != 1 {
		t.Errorf("the adapter received %d merge(s) without delete, want 1", got)
	}
	if got := f.adp.MergeDeleteCount(true); got != 0 {
		t.Errorf("the adapter received %d merge(s) with delete, want 0", got)
	}
}

func TestDeleteToggleStaysForTheSession(t *testing.T) {
	f := newMergeFixture(t, mergeItems()...)

	m := press(t, f.m, "m")
	m = press(t, m, "tab")
	m = press(t, m, "s")
	// The result reaches the model: without it the action stays marked as in progress.
	m = send(t, m, actionMsg{cycle: m.cycle, outcome: waitOutcome(t, m)})

	if got := f.adp.MergeDeleteCount(false); got != 1 {
		t.Fatalf("the first merge: %d without delete, want 1", got)
	}

	m = press(t, m, "m")
	if m.deleteBranch {
		t.Error("the value should stay off on the next merge")
	}
	m = press(t, m, "s")
	_ = waitOutcome(t, m)

	if got := f.adp.MergeDeleteCount(false); got != 2 {
		t.Errorf("the adapter received %d merge(s) without delete, want 2", got)
	}
}

func TestTabOutsideTheArmedMergeCyclesSections(t *testing.T) {
	f := newMergeFixture(t, mergeItems()...)
	before := f.m.activeSection

	m := press(t, f.m, "tab")

	if m.activeSection == before {
		t.Error("`tab` with no merge armed should change section")
	}
	if m.deleteBranch != f.m.deleteBranch {
		t.Error("`tab` outside the armed state should not touch the delete")
	}
}

func TestDeleteOnlyTravelsWithAMerge(t *testing.T) {
	f := newMergeFixture(t, mergeItems()...)
	m := press(t, f.m, "a")
	out := waitOutcome(t, m)

	if out.Kind != forge.ActionApprove {
		t.Fatalf("Kind = %q, want approve", out.Kind)
	}
	if out.DeleteBranch {
		t.Error("approve should not ask to delete the branch")
	}
}

func TestMergeNoticeNamesTheBranch(t *testing.T) {
	f := newMergeFixture(t, mergeItems()...)
	m := press(t, f.m, "m")
	m = press(t, m, "s")
	out := waitOutcome(t, m)
	m = send(t, m, actionMsg{cycle: m.cycle, outcome: out})

	notice := lastToast(m)
	if !strings.Contains(notice, "branch deleted") {
		t.Errorf("the notice should confirm the delete: %q", notice)
	}
}

func TestMergeNoticeSaysWhenTheBranchSurvived(t *testing.T) {
	f := newMergeFixture(t, mergeItems()...)
	m := press(t, f.m, "m")
	m = press(t, m, "s")
	out := waitOutcome(t, m)
	out.DeleteMsg = "Resource not accessible by integration"
	m = send(t, m, actionMsg{cycle: m.cycle, outcome: out})

	notice := lastToast(m)
	if !strings.Contains(notice, "branch not deleted") {
		t.Errorf("the notice should say the branch is still there: %q", notice)
	}
	if !strings.Contains(notice, "Resource not accessible") {
		t.Errorf("the notice should give the forges reason: %q", notice)
	}
	if strings.Contains(notice, "error:") {
		t.Errorf("a merge that walked away is not an error: %q", notice)
	}
}
