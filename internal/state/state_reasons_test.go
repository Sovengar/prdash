package state

import (
	"strings"
	"testing"

	"prdash/internal/forge/model"
)

// Score orders the inbox and the order is a product decision, not an accident.
func TestTheAttentionOrderIsTheDocumentedOne(t *testing.T) {
	order := []State{StateError, StateChangesRequested, StateReviewRequired, StateApproved,
		StatePending, StateMerged, StateDraft}
	for i := 0; i < len(order)-1; i++ {
		a, b := order[i], order[i+1]
		if order[i].Score() <= order[i+1].Score() {
			t.Errorf("%q (score %d) does not sort above %q (score %d)",
				a, a.Score(), b, b.Score())
		}
	}

	// Merged and closed tie. Deliberately: both mean "this is no longer pending".
	if StateMerged.Score() != StateClosed.Score() {
		t.Errorf("merged scores %d and closed %d: the code puts them together on purpose, so "+
			"if they diverge it is a decision that has to be written down",
			StateMerged.Score(), StateClosed.Score())
	}

	// An unknown state scores zero, below draft: unknown is not "urgent".
	if got := State(99).Score(); got != 0 {
		t.Errorf("an unknown state scores %d, want 0", got)
	}
	if got := State(99).String(); got == "" {
		t.Error("an unknown state gave an empty label")
	}
}

// String goes in the state column, one of the narrowest.
func TestTheStateLabelIsTheOneForTheNarrowColumn(t *testing.T) {
	cases := []struct {
		state State
		want  string
	}{
		{StateDraft, "draft"},
		{StatePending, "pending"},
		{StateApproved, "approved"},
		{StateReviewRequired, "review required"},
		{StateChangesRequested, "changes requested"},
		{StateMerged, "merged"},
		{StateClosed, "closed"},
		{StateError, "error"},
	}
	for _, c := range cases {
		if got := c.state.String(); got != c.want {
			t.Errorf("State(%q).String() gave %q, want %q", c.state, got, c.want)
		}
		// An empty label is worse than a long one: the column goes blank and there is nothing to read.
		if strings.TrimSpace(c.state.String()) == "" {
			t.Errorf("State(%q) has an empty label", c.state)
		}
		if c.state.String() != strings.TrimSpace(c.state.String()) {
			t.Errorf("the label of %q has spaces at the edges", c.state)
		}
	}
	// An unknown state invents no label: it prints what can be extracted from an int.
	if got := State(99).String(); strings.TrimSpace(got) == "" {
		t.Error("an unknown state gave an empty label")
	}
}

// The conflict warning has to name the branch you have to rebase against.
func TestTheConflictReasonNamesTheBranch(t *testing.T) {
	cases := []struct {
		target string
		want   string
	}{
		{"main", "the branch conflicts with main"},
		{"release/2026-04", "the branch conflicts with release/2026-04"},
		{"", "the branch conflicts with the target branch"},
		{"   ", "the branch conflicts with the target branch"},
		{"\t\n", "the branch conflicts with the target branch"},
	}
	for _, c := range cases {
		got := conflictedReason(c.target)
		if got != c.want {
			t.Errorf("conflictedReason(%q) gave %q, want %q", c.target, got, c.want)
		}
		if strings.TrimSpace(got) == "" {
			t.Errorf("conflictedReason(%q) returned empty text", c.target)
		}
	}
	if strings.HasSuffix(conflictedReason(""), "with ") {
		t.Error("the generic warning ends in \"with \": it looks like a branch got lost")
	}
}

// The failing-check count is what tells a broken CI from a pending one.
func TestTheCIReasonStartsWithTheCount(t *testing.T) {
	cases := []struct {
		checks model.Checks
		want   string
	}{
		{model.Checks{Total: 10, Failing: 1}, "CI is failing (1 of 10 checks)"},
		{model.Checks{Total: 3, Failing: 3}, "CI is failing (3 of 3 checks)"},
		{model.Checks{Total: 1, Failing: 1}, "CI is failing (1 of 1 checks)"},
		{model.Checks{}, "CI is failing"},
	}
	for _, c := range cases {
		if got := checksFailingReason(c.checks); got != c.want {
			t.Errorf("checksFailingReason(%+v) gave %q, want %q", c.checks, got, c.want)
		}
	}
	// A known total never prints without counts: Failing zero and total non-zero must still say so.
	if got := checksFailingReason(model.Checks{Total: 5, Failing: 0}); got == "CI is failing" {
		t.Error("with 5 known checks the warning came out without figures: the data is lost")
	}
}

func TestTheReasonsAreNotEmpty(t *testing.T) {
	reasons := []string{UnmergeableReason}
	for _, m := range reasons {
		if strings.TrimSpace(m) == "" {
			t.Errorf("a reason is empty: %q", m)
		}
		if m != strings.TrimSpace(m) {
			t.Errorf("the reason %q has spaces at the edges", m)
		}
	}
	// The merge reason reads as something to DO, not as a "no".
	if !strings.Contains(UnmergeableReason, "push") {
		t.Errorf("the merge reason does not say what to do: %q", UnmergeableReason)
	}
}
