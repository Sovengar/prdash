package tui

import (
	"strings"
	"testing"

	"prdash/internal/forge"
	"prdash/internal/forge/model"
)

func gatedItems(rules model.MergeRules) []model.Item {
	it := mkItem("github", "github.com", "acme/widget", "Add widget", 1, "")
	it.State = "open"
	it.Merge = rules
	return []model.Item{it}
}

func TestMergeBlockedOnADraftNoArms(t *testing.T) {
	items := gatedItems(model.MergeRulesAll())
	items[0].IsDraft = true
	f := newMergeFixture(t, items...)

	m := press(t, f.m, "m")
	if m.mergeArmed {
		t.Error("a draft item should not arm the merge")
	}
	if !strings.Contains(toasts(m), "draft") {
		t.Errorf("the notice does not explain the block: %q", toasts(m))
	}
}

func TestMergeBlockedOnAMergedItemNoArms(t *testing.T) {
	items := gatedItems(model.MergeRulesAll())
	items[0].State = "merged"
	f := newMergeFixture(t, items...)

	m := press(t, f.m, "m")
	if m.mergeArmed {
		t.Error("an already merged item should not arm the merge")
	}
}

// Red CI cannot be forbidden: an unstable check would otherwise leave the user stuck.
func TestMergeOnFailingCIStillArmsButSays(t *testing.T) {
	items := gatedItems(model.MergeRulesAll())
	items[0].Checks = model.Checks{State: model.ChecksFailing, Total: 5, Failing: 2}
	f := newMergeFixture(t, items...)

	m := press(t, f.m, "m")
	if !m.mergeArmed {
		t.Fatal("with red CI the merge should arm: the block is soft")
	}
	view := stripANSI(m.View().Content)
	if !strings.Contains(view, "CI is failing") {
		t.Errorf("the confirmation does not mention the red CI:\n%s", view)
	}
	if !strings.Contains(view, "anyway") {
		t.Errorf("the confirmation does not say that choosing the mode goes ahead:\n%s", view)
	}
}

// A PR whose branches collide is not going to be integrated anyway.
func TestMergeOnConflictingBranchesStillArmsButSays(t *testing.T) {
	items := gatedItems(model.MergeRulesAll())
	items[0].TargetBranch = "main"
	items[0].Mergeable = model.Mergeability{Known: true, Conflicted: true}
	f := newMergeFixture(t, items...)

	m := press(t, f.m, "m")
	if !m.mergeArmed {
		t.Fatal("with conflicting branches the merge should arm: a rebase fixes it")
	}
	view := stripANSI(m.View().Content)
	if !strings.Contains(view, "conflicts with main") {
		t.Errorf("the confirmation does not say what it clashes with:\n%s", view)
	}
	if !strings.Contains(view, "anyway") {
		t.Errorf("the confirmation does not say that choosing the mode goes ahead:\n%s", view)
	}
}

// When the forge does not say (GitHub UNKNOWN) the result stays quiet.
func TestMergeWithoutMergeabilityDataStaysQuiet(t *testing.T) {
	for _, tc := range []struct {
		name string
		m    model.Mergeability
	}{
		{"no data", model.Mergeability{}},
		{"integrable", model.Mergeability{Known: true}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			items := gatedItems(model.MergeRulesAll())
			items[0].TargetBranch = "main"
			items[0].Mergeable = tc.m
			f := newMergeFixture(t, items...)

			m := press(t, f.m, "m")
			if !m.mergeArmed {
				t.Fatal("the merge should arm")
			}
			if view := stripANSI(m.View().Content); strings.Contains(view, "conflicts") {
				t.Errorf("with no data a conflict must not be announced:\n%s", view)
			}
		})
	}
}

// The refusal's warning has to say what to do.
func TestMergeRefusedForConflictingBranchesSaysRebase(t *testing.T) {
	it := mergeItems()[1]
	f := newMergeFixture(t, it)
	f.adp.ActionWarnings = map[string][]model.Warning{"merge:acme/widget#2": {{
		Forge: "github", Kind: "unmergeable",
		Msg: "gh pr merge 2: × Pull request acme/widget#2 is not mergeable: the merge commit cannot be cleanly created. (exit 1)",
	}}}

	m := press(t, f.m, "m")
	m = press(t, m, "r")
	out := waitOutcome(t, m)
	m = send(t, m, actionMsg{cycle: m.cycle, outcome: out})

	notice := lastToast(m)
	if !strings.Contains(notice, "refused") {
		t.Errorf("the notice should say the forge rejected it, not that there was a conflict: %q", notice)
	}
	if strings.Contains(notice, "forge conflict") {
		t.Errorf("a refresh does not fix a rebase: %q", notice)
	}
	if !strings.Contains(notice, "rebase") {
		t.Errorf("the notice should say what to do: %q", notice)
	}
	// And it must not stay recorded as denied: a rebase makes it integrable again.
	if len(m.denied) != 0 {
		t.Errorf("the item should not stay denied forever: %+v", m.denied)
	}
}

func TestMergeConfirmationOnlyOffersAllowedModes(t *testing.T) {
	f := newMergeFixture(t, gatedItems(model.MergeRules{Known: true, Rebase: true, MergeCommit: true})...)

	m := press(t, f.m, "m")
	view := stripANSI(m.View().Content)
	if strings.Contains(view, "s squash") {
		t.Errorf("the confirmation offers squash and the repository has it disabled:\n%s", view)
	}
	if !strings.Contains(view, "r rebase") {
		t.Errorf("the confirmation does not offer rebase, which is allowed:\n%s", view)
	}
}

func TestMergeRefusesADisallowedMode(t *testing.T) {
	f := newMergeFixture(t, gatedItems(model.MergeRules{Known: true, Rebase: true})...)

	m := press(t, f.m, "m")
	m = press(t, m, "s")
	if m.actionBusy {
		t.Error("a mode that is not allowed should not fire the merge")
	}
	if !strings.Contains(toasts(m), "does not allow") {
		t.Errorf("the notice does not explain the rejection: %q", toasts(m))
	}
}

func TestMergeOnUnknownRulesOffersEveryMode(t *testing.T) {
	f := newMergeFixture(t, gatedItems(model.MergeRules{})...)

	m := press(t, f.m, "m")
	view := stripANSI(m.View().Content)
	for _, want := range []string{"r rebase", "m merge commit", "s squash"} {
		if !strings.Contains(view, want) {
			t.Errorf("with no known rules the confirmation should offer %q:\n%s", want, view)
		}
	}
}

func TestMergePinsToTheHeadCommitItSaw(t *testing.T) {
	items := gatedItems(model.MergeRulesAll())
	items[0].HeadSHA = "abc1234"
	f := newMergeFixture(t, items...)

	m := press(t, f.m, "m")
	m = press(t, m, "r")
	_ = waitOutcome(t, m)

	if n := f.adp.MergeModeCount(forge.Rebase); n != 1 {
		t.Errorf("MergeModeCount(rebase) = %d, want 1", n)
	}
}

func TestMergeWithoutAHeadSHARefuses(t *testing.T) {
	items := gatedItems(model.MergeRulesAll())
	items[0].HeadSHA = ""
	f := newMergeFixture(t, items...)

	m := press(t, f.m, "m")
	m = press(t, m, "r")
	out := waitOutcome(t, m)

	if out.OK {
		t.Error("a merge with no pin should not come out as ok")
	}
	if n := f.adp.MergeModeCount(forge.Rebase); n != 0 {
		t.Errorf("there were %d call(s) to Merge, want 0: no pin is possible", n)
	}
}
