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

type Block struct {
	Reason string
	Hard   bool
}

func MergeBlock(it model.Item) Block {
	switch normalize(it.State) {
	case "merged":
		return Block{Reason: "item is already merged", Hard: true}
	case "closed":
		return Block{Reason: "item is already closed", Hard: true}
	}
	if it.IsDraft {
		return Block{Reason: "item is a draft", Hard: true}
	}
	if it.Mergeable.Known && it.Mergeable.Conflicted {
		return Block{Reason: conflictedReason(it.TargetBranch)}
	}
	if it.Checks.State == model.ChecksFailing {
		return Block{Reason: checksFailingReason(it.Checks)}
	}
	if it.Checks.State == model.ChecksPending {
		return Block{Reason: fmt.Sprintf("CI is still running (%d pending)", it.Checks.Pending)}
	}
	if normalize(it.ReviewDecision) == "changes_requested" {
		return Block{Reason: "changes were requested on this item"}
	}
	return Block{}
}

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
