package state

import (
	"testing"

	"prdash/internal/forge/model"
)

func item(state, decision string, checks model.Checks) model.Item {
	it := model.NewItem(model.RepoRef{Forge: "github", Host: "github.com", Project: "o/r"}, 1)
	it.State = state
	it.ReviewDecision = decision
	it.Checks = checks
	return it
}

// A draft built as a real adapter builds it: State is the forge's enum ("OPEN").
func draft(decision string, checks model.Checks) model.Item {
	it := item("OPEN", decision, checks)
	it.IsDraft = true
	return it
}

func TestDerive(t *testing.T) {
	cases := []struct {
		name string
		item model.Item
		want State
	}{
		{"merged", item("MERGED", "", model.Checks{}), StateMerged},
		{"closed", item("CLOSED", "", model.Checks{}), StateClosed},
		{"failing checks win", item("OPEN", "APPROVED", model.Checks{State: model.ChecksFailing}), StateError},
		{"changes requested", item("OPEN", "CHANGES_REQUESTED", model.Checks{}), StateChangesRequested},
		{"review required", item("OPEN", "REVIEW_REQUIRED", model.Checks{}), StateReviewRequired},
		{"approved", item("OPEN", "APPROVED", model.Checks{}), StateApproved},
		{"draft", draft("", model.Checks{}), StateDraft},
		// Derive's precedence beats the draft on purpose: the column orders by attention.
		{"draft with changes requested", draft("CHANGES_REQUESTED", model.Checks{}), StateChangesRequested},
		{"open without decision", item("OPEN", "", model.Checks{}), StatePending},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := Derive(c.item); got != c.want {
				t.Fatalf("Derive = %s, want %s", got, c.want)
			}
		})
	}
}

func TestScorePrecedence(t *testing.T) {
	order := []State{
		StateError,
		StateChangesRequested,
		StateReviewRequired,
		StateApproved,
		StatePending,
		StateMerged,
		StateDraft,
	}
	for i := 0; i < len(order)-1; i++ {
		if order[i].Score() <= order[i+1].Score() {
			t.Errorf("Score(%s)=%d does not beat Score(%s)=%d",
				order[i], order[i].Score(), order[i+1], order[i+1].Score())
		}
	}
}

func TestActionable(t *testing.T) {
	cases := []struct {
		name string
		item model.Item
		want bool
	}{
		{"open", item("OPEN", "APPROVED", model.Checks{}), true},
		{"merged", item("MERGED", "", model.Checks{}), false},
		{"closed", item("CLOSED", "", model.Checks{}), false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			ok, reason := Actionable(c.item)
			if ok != c.want {
				t.Fatalf("Actionable = %v (%s), want %v", ok, reason, c.want)
			}
			if !ok && reason == "" {
				t.Error("a disallowed action should carry a reason")
			}
		})
	}
}

func TestStateString(t *testing.T) {
	cases := map[State]string{
		StateDraft:            "draft",
		StatePending:          "pending",
		StateApproved:         "approved",
		StateReviewRequired:   "review required",
		StateChangesRequested: "changes requested",
		StateMerged:           "merged",
		StateClosed:           "closed",
		StateError:            "error",
	}
	for st, want := range cases {
		if got := st.String(); got != want {
			t.Errorf("String(%d) = %q, want %q", st, got, want)
		}
	}
}

func TestCanApprove(t *testing.T) {
	own := func(section model.Section, author string) model.Item {
		it := model.NewItem(model.RepoRef{Forge: "github", Host: "github.com", Project: "acme/widget"}, 4)
		it.Section = section
		it.Author = author
		return it
	}
	cases := []struct {
		name    string
		it      model.Item
		viewer  string
		wantOK  bool
		wantMsg string
	}{
		{"matching login", own(model.SectionReview, "Sovengar"), "Sovengar", false, SelfReviewReason},
		{"login with different case", own(model.SectionReview, "sovengar"), "Sovengar", false, SelfReviewReason},
		{"another author", own(model.SectionReview, "someone"), "Sovengar", true, ""},
		{"own section without login", own(model.SectionAuthored, "whoever"), "", false, SelfReviewReason},
		{"review section without login", own(model.SectionReview, "whoever"), "", true, ""},
		{"known login without author", own(model.SectionAuthored, ""), "Sovengar", false, SelfReviewReason},
		{"mentions without login", own(model.SectionMentions, "other"), "", true, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ok, reason := CanApprove(tc.it, tc.viewer)
			if ok != tc.wantOK || reason != tc.wantMsg {
				t.Errorf("CanApprove = (%v, %q), want (%v, %q)", ok, reason, tc.wantOK, tc.wantMsg)
			}
		})
	}
}
