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

func TestDerive(t *testing.T) {
	cases := []struct {
		name string
		item model.Item
		want State
	}{
		{"merged", item("MERGED", "", model.Checks{}), StateMerged},
		{"closed", item("CLOSED", "", model.Checks{}), StateClosed},
		{"failing checks ganan", item("OPEN", "APPROVED", model.Checks{State: model.ChecksFailing}), StateError},
		{"changes requested", item("OPEN", "CHANGES_REQUESTED", model.Checks{}), StateChangesRequested},
		{"review required", item("OPEN", "REVIEW_REQUIRED", model.Checks{}), StateReviewRequired},
		{"approved", item("OPEN", "APPROVED", model.Checks{}), StateApproved},
		{"draft", item("draft", "", model.Checks{}), StateDraft},
		{"abierto sin decisión", item("OPEN", "", model.Checks{}), StatePending},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := Derive(c.item); got != c.want {
				t.Fatalf("Derive = %s, want %s", got, c.want)
			}
		})
	}
}

// TestScorePrecedence fija el orden de atención: error > changes-requested >
// review-required > approved > pending > merged/closed > draft.
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
			t.Errorf("Score(%s)=%d no supera a Score(%s)=%d",
				order[i], order[i].Score(), order[i+1], order[i+1].Score())
		}
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
