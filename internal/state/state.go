// Package state derives one state with explicit precedence, plus an attention score, from each forge's
// raw output. The TUI and the print mode share it so the order is the same.
package state

import (
	"fmt"
	"strings"

	"prdash/internal/forge/model"
)

type State int

const (
	StateDraft State = iota
	StatePending
	StateApproved
	StateReviewRequired
	StateChangesRequested
	StateMerged
	StateClosed
	StateError
)

func (s State) String() string {
	switch s {
	case StateDraft:
		return "draft"
	case StatePending:
		return "pending"
	case StateApproved:
		return "approved"
	case StateReviewRequired:
		return "review required"
	case StateChangesRequested:
		return "changes requested"
	case StateMerged:
		return "merged"
	case StateClosed:
		return "closed"
	case StateError:
		return "error"
	default:
		return "unknown"
	}
}

func (s State) Score() int {
	switch s {
	case StateError:
		return 8
	case StateChangesRequested:
		return 7
	case StateReviewRequired:
		return 6
	case StateApproved:
		return 5
	case StatePending:
		return 4
	case StateMerged, StateClosed:
		return 2
	case StateDraft:
		return 1
	default:
		return 0
	}
}

// Precedence: closed/merged > failing checks > changes requested > review requested > approved > draft >
// open.
func Derive(it model.Item) State {
	switch normalize(it.State) {
	case "merged":
		return StateMerged
	case "closed":
		return StateClosed
	}

	if it.Checks.State == model.ChecksFailing {
		return StateError
	}

	switch normalize(it.ReviewDecision) {
	case "changes_requested":
		return StateChangesRequested
	case "review_required":
		return StateReviewRequired
	case "approved":
		return StateApproved
	}

	if it.IsDraft {
		return StateDraft
	}
	return StatePending
}

func Actionable(it model.Item) (bool, string) {
	switch Derive(it) {
	case StateMerged:
		return false, "item is already merged"
	case StateClosed:
		return false, "item is already closed"
	default:
		return true, ""
	}
}

// Hard versus soft is what makes the gate worth having. A hard block is a property of the forge (a draft
// PR cannot be merged because GitHub forbids it, and no confirmation changes that). A soft one is a
// policy: CI can be red on a flaky check, and requested changes are sometimes ignored by whoever owns the
// repo. Those are not forbidden —that would turn the tool into a wall— but they are not silent either:
// they are announced and asked for twice.
type Block struct {
	Reason string
	Hard   bool
}

// The precedence mirrors Derive's, which already decides the attention order: an item with red CI and
// requested changes is blocked for the CI, which is what the operator wants to know first. Pending checks
// count as a SOFT block rather than as ignored: merging while CI runs is exactly the race the head SHA
// pin does not close, because the CI can pass after the merge.
func MergeBlock(it model.Item) Block {
	// Draft, merged and closed are read from the item's own fields and not from Derive: Derive orders by
	// attention to the operator, so a draft that is also approved comes back StateApproved and the draft
	// is lost. The question here is a different one — can this forge integrate this? — and its answer
	// does not depend on the review decision.
	switch normalize(it.State) {
	case "merged":
		return Block{Reason: "item is already merged", Hard: true}
	case "closed":
		return Block{Reason: "item is already closed", Hard: true}
	}
	if it.IsDraft {
		return Block{Reason: "item is a draft", Hard: true}
	}
	// A branch collision is SOFT, and that is the design decision that matters: GitHub will not integrate
	// it as it stands, but a rebase fixes it in one command and the gate cannot know whether the user
	// already did. Forbidding it would leave the PR with no way out from here, so it is announced and the
	// second keypress decides.
	// Before the CI on purpose: a colliding PR is not going to pass CI, and saying "CI is failing" when
	// what is needed is a rebase sends the operator to the wrong place.
	if it.Mergeable.Known && it.Mergeable.Conflicted {
		return Block{Reason: conflictedReason(it.TargetBranch)}
	}
	switch {
	case it.Checks.State == model.ChecksFailing:
		return Block{Reason: checksFailingReason(it.Checks)}
	case it.Checks.State == model.ChecksPending:
		return Block{Reason: fmt.Sprintf("CI is still running (%d pending)", it.Checks.Pending)}
	// Normalised because the same datum arrives in different conventions per forge, and comparing it raw
	// made the gate miss requested changes in both.
	case normalize(it.ReviewDecision) == "changes_requested":
		return Block{Reason: "changes were requested on this item"}
	}
	return Block{}
}

// The difference from a conflict —the item changed while you were looking at it, which a refresh
// fixes— is the one that matters: here the right action is rebase and push, and a warning that says
// "refresh" sends the operator to the wrong place.
const UnmergeableReason = "the forge will not merge it as it is: rebase the branch onto the target and push"

func conflictedReason(target string) string {
	if strings.TrimSpace(target) == "" {
		return "the branch conflicts with the target branch"
	}
	return "the branch conflicts with " + target
}

func checksFailingReason(c model.Checks) string {
	if c.Total > 0 {
		return fmt.Sprintf("CI is failing (%d of %d checks)", c.Failing, c.Total)
	}
	return "CI is failing"
}

const SelfReviewReason = "you cannot approve your own PR/MR"

// No forge allows approving your own: GitHub rejects it in the API with no option to enable it, and
// the ones that allow it by configuration still have the repository decide, not the client.
// Identity uses the viewer's login when both are known, since that is a fact; when either is missing it
// falls back to the section, which for a forge means "the user wrote it".
func CanApprove(it model.Item, viewer string) (bool, string) {
	if viewer != "" && it.Author != "" {
		if strings.EqualFold(viewer, it.Author) {
			return false, SelfReviewReason
		}
		return true, ""
	}
	if it.Section == model.SectionAuthored {
		return false, SelfReviewReason
	}
	return true, ""
}

func normalize(s string) string {
	out := make([]rune, 0, len(s))
	for _, r := range s {
		switch r {
		case '-', ' ', '\t':
			out = append(out, '_')
		default:
			if r >= 'A' && r <= 'Z' {
				r += 'a' - 'A'
			}
			out = append(out, r)
		}
	}
	return string(out)
}
