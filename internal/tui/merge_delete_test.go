package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"prdash/internal/forge"
	"prdash/internal/forge/model"
	"prdash/internal/testutil"
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

// A repo that auto-deletes head branches ignores the toggle: the confirmation
// must say so instead of letting "no" read as "the branch survives".
func TestConfirmSaysWhenTheRepoAutoDeletes(t *testing.T) {
	it := mkItem("github", "github.com", "acme/widget", "Add widget", 1, "")
	it.AutoDeleteBranch = true
	it.AutoDeleteKnown = true
	f := newMergeFixture(t, it)

	m := press(t, f.m, "m")
	if view := stripANSI(m.View().Content); strings.Contains(view, "auto-deletes") {
		t.Errorf("with the request on, the policy note is noise\n%s", view)
	}

	m = press(t, m, "tab")
	view := stripANSI(m.View().Content)
	if !strings.Contains(view, "delete branch: no") {
		t.Errorf("the box does not reflect the new value\n%s", view)
	}
	if !strings.Contains(view, "auto-deletes") {
		t.Errorf("with the request off the repo policy must be named\n%s", view)
	}
}

// The caveat must fire ONLY where the policy is on: with it explicitly off the
// branch really survives, so "no" is the truth and warning would be a fresh lie.
func TestConfirmStaysSilentWhenThePolicyIsOff(t *testing.T) {
	it := mkItem("github", "github.com", "acme/widget", "Add widget", 1, "")
	it.AutoDeleteBranch = false
	it.AutoDeleteKnown = true
	f := newMergeFixture(t, it)

	m := press(t, f.m, "m")
	m = press(t, m, "tab")
	view := stripANSI(m.View().Content)
	if !strings.Contains(view, "delete branch: no") {
		t.Errorf("the box does not reflect the new value\n%s", view)
	}
	if strings.Contains(view, "auto-deletes") {
		t.Errorf("the policy is off, the branch survives: a caveat would be a new lie\n%s", view)
	}
}

// Not knowing the policy is not the same as it being off: no warning is invented.
func TestConfirmStaysSilentWhenThePolicyIsUnknown(t *testing.T) {
	f := newMergeFixture(t, mergeItems()...) // mkItem leaves both flags false
	m := press(t, f.m, "m")
	m = press(t, m, "tab")
	view := stripANSI(m.View().Content)
	if !strings.Contains(view, "delete branch: no") {
		t.Errorf("the box does not reflect the new value\n%s", view)
	}
	if strings.Contains(view, "auto-deletes") {
		t.Errorf("an unknown policy is not an off policy: no caveat\n%s", view)
	}
}

// GitLab's per-MR remove-source-branch flag is honoured, so its item never
// carries the policy and its confirmation never gets the caveat.
func TestGitLabKeepsItsDeleteRequestWithoutCaveat(t *testing.T) {
	gl := &testutil.FakeAdapter{ForgeName: "gitlab", HostName: "gitlab.example.com"}
	m := newTestModel(t, gl)
	it := mkItem("gitlab", "gitlab.example.com", "grp/proj", "Add widget", 1, "")
	m = send(t, m, page(1, "gitlab", "gitlab.example.com", model.SectionReview, model.ReviewRequested, []model.Item{it}, false))

	m = press(t, m, "m")
	if view := stripANSI(m.View().Content); strings.Contains(view, "auto-deletes") {
		t.Errorf("GitLab reports no such policy: a caveat would be invented\n%s", view)
	}
	m = press(t, m, "tab")
	view := stripANSI(m.View().Content)
	if !strings.Contains(view, "delete branch: no") {
		t.Errorf("the GitLab confirmation does not reflect the request\n%s", view)
	}
	if strings.Contains(view, "auto-deletes") {
		t.Errorf("GitLab's per-MR flag is honoured: no caveat\n%s", view)
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

// The caveat lengthens the confirmation head. On a narrow terminal the hint is
// clipped all-or-nothing and must never paint a box halfway or overflow the
// terminal, so the longer label cannot corrupt the layout.
func TestTheAutoDeleteCaveatDegradesOnANarrowTerminal(t *testing.T) {
	it := mkItem("github", "github.com", "acme/widget", "Add widget", 1, "")
	it.AutoDeleteBranch = true
	it.AutoDeleteKnown = true
	f := newMergeFixture(t, it)
	m := press(t, f.m, "m")
	m = press(t, m, "tab")

	for _, width := range []int{160, 80, 50, 40} {
		m.width = width
		view := stripANSI(m.View().Content)
		tops, bottoms := 0, 0
		for _, line := range strings.Split(view, "\n") {
			if w := ansi.StringWidth(line); w > width {
				t.Errorf("width %d: a line paints %d columns, past the terminal\n%q", width, w, line)
			}
			trimmed := strings.TrimRight(line, " ")
			if strings.HasPrefix(trimmed, "╭") {
				tops++
			}
			if strings.HasPrefix(trimmed, "╰") {
				bottoms++
			}
		}
		if tops == 0 || tops != bottoms {
			t.Errorf("width %d: %d box(es) opened and %d closed: one was painted halfway\n%s",
				width, tops, bottoms, view)
		}
		if got := len(m.hintLines()); got > maxHintLines {
			t.Errorf("width %d: %d hint lines, past the cap %d", width, got, maxHintLines)
		}
	}

	m.width = 160
	if view := stripANSI(m.View().Content); !strings.Contains(view, "auto-deletes") {
		t.Errorf("at a wide terminal the caveat must be visible\n%s", view)
	}
}
