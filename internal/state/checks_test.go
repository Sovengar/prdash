package state

import (
	"strings"
	"testing"

	"prdash/internal/forge/model"
)

func TestChecksFailingReasonDistinguishesCountFromNoCount(t *testing.T) {
	cases := []struct {
		name   string
		checks model.Checks
		want   string
	}{
		{"with count", model.Checks{Total: 5, Failing: 2, State: model.ChecksFailing}, "CI is failing (2 of 5 checks)"},
		{"one of one", model.Checks{Total: 1, Failing: 1, State: model.ChecksFailing}, "CI is failing (1 of 1 checks)"},
		{"no data", model.Checks{}, "CI is failing"},
		{"zero total with failure", model.Checks{Total: 0, Failing: 0, State: model.ChecksFailing}, "CI is failing"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := checksFailingReason(c.checks); got != c.want {
				t.Errorf("checksFailingReason(%+v) = %q, want %q", c.checks, got, c.want)
			}
		})
	}
}

// That the count survives the trip: MergeBlock carries it to the warning.
func TestChecksFailingReasonReachesTheGateWarning(t *testing.T) {
	it := model.NewItem(model.RepoRef{Forge: "github", Host: "github.com", Project: "acme/widget"}, 1)
	it.State = "OPEN"
	it.Checks = model.Checks{Total: 12, Failing: 3, State: model.ChecksFailing}

	block := MergeBlock(it)
	if block.Reason == "" {
		t.Fatal("an item with red CI should have a block")
	}
	if !strings.Contains(block.Reason, "3 of 12") {
		t.Errorf("the reason = %q, want the check count", block.Reason)
	}

	noChecks := it
	noChecks.Checks = model.Checks{State: model.ChecksFailing}
	block = MergeBlock(noChecks)
	if !strings.Contains(block.Reason, "CI is failing") {
		t.Errorf("the reason = %q, want the CI warning", block.Reason)
	}
	if strings.Contains(block.Reason, "of 0 checks") {
		t.Errorf("with no check data there is no count to show: %q", block.Reason)
	}
}

// normalize's contract: the forges' enums arrive in different conventions.
func TestNormalizeComparesWithoutCaseOrSeparators(t *testing.T) {
	cases := map[string]string{
		"OPEN":              "open",
		"open":              "open",
		"CHANGES_REQUESTED": "changes_requested",
		"changes-requested": "changes_requested",
		"changes requested": "changes_requested",
		"  OPEN\t":          "__open_",
		"":                  "",
		// The Z is the last uppercase of the range, the one a misplaced `<=` leaves out.
		"BUZZ":   "buzz",
		"Z":      "z",
		"AZ":     "az",
		"ZZ":     "zz",
		"año2":   "año2",
		"9lives": "9lives",
	}
	for in, want := range cases {
		if got := normalize(in); got != want {
			t.Errorf("normalize(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestDeriveNormalizesStatesWithZ(t *testing.T) {
	it := model.NewItem(model.RepoRef{Forge: "github", Host: "github.com", Project: "acme/widget"}, 1)
	it.State = "BUZZ"
	if got := Derive(it); got != StatePending {
		t.Errorf("Derive(state=%q) = %v, want StatePending (an unknown state does not block)", it.State, got)
	}
	for _, s := range []string{"merged", "MERGED", "Merged"} {
		it.State = s
		if got := Derive(it); got != StateMerged {
			t.Errorf("Derive(%q) = %v, want StateMerged", s, got)
		}
	}
	for _, s := range []string{"closed", "CLOSED"} {
		it.State = s
		if got := MergeBlock(it).Reason; got != "item is already closed" {
			t.Errorf("MergeBlock(%q) = %q, want the closed-block reason", s, got)
		}
	}
}
