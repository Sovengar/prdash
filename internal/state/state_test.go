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

func TestActionable(t *testing.T) {
	cases := []struct {
		name string
		item model.Item
		want bool
	}{
		{"abierto", item("OPEN", "APPROVED", model.Checks{}), true},
		{"mergeado", item("MERGED", "", model.Checks{}), false},
		{"cerrado", item("CLOSED", "", model.Checks{}), false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			ok, reason := Actionable(c.item)
			if ok != c.want {
				t.Fatalf("Actionable = %v (%s), want %v", ok, reason, c.want)
			}
			if !ok && reason == "" {
				t.Error("una acción no permitida debería traer motivo")
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

// TestCanApprove cubre el veto de aprobar lo propio: con login conocido decide
// por identidad, y si falta alguno cae a la sección sin bloquear de más.
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
		{"login coincide", own(model.SectionReview, "Sovengar"), "Sovengar", false, SelfReviewReason},
		{"login con otra caja", own(model.SectionReview, "sovengar"), "Sovengar", false, SelfReviewReason},
		{"otro autor", own(model.SectionReview, "otra"), "Sovengar", true, ""},
		{"seccion propia sin login", own(model.SectionAuthored, "quien sea"), "", false, SelfReviewReason},
		{"seccion de review sin login", own(model.SectionReview, "quien sea"), "", true, ""},
		{"login conocido sin autor", own(model.SectionAuthored, ""), "Sovengar", false, SelfReviewReason},
		{"menciones sin login", own(model.SectionMentions, "otro"), "", true, ""},
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
