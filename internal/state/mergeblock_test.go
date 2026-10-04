package state

import (
	"strings"
	"testing"

	"prdash/internal/forge/model"
)

func base() model.Item {
	return model.Item{
		Number:         7,
		State:          "open",
		Ref:            model.RepoRef{Forge: "github", Host: "github.com", Project: "acme/widget"},
		HeadSHA:        "abc1234",
		Checks:         model.Checks{State: model.ChecksPassing, Total: 3},
		ReviewDecision: "APPROVED",
	}
}

func TestMergeBlockRefusesWhatTheForgeRefuses(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mutate func(*model.Item)
		want   string
	}{
		{"draft", func(it *model.Item) { it.IsDraft = true }, "draft"},
		{"merged", func(it *model.Item) { it.State = "merged" }, "merged"},
		{"closed", func(it *model.Item) { it.State = "closed" }, "closed"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			it := base()
			tc.mutate(&it)
			block := MergeBlock(it)
			if block.Reason == "" {
				t.Fatalf("MergeBlock = vacío, want un bloqueo por %s", tc.name)
			}
			if !block.Hard {
				t.Errorf("Hard = false, want true: %s no se puede forzar", tc.name)
			}
			if !strings.Contains(block.Reason, tc.want) {
				t.Errorf("Reason = %q, want que mencione %q", block.Reason, tc.want)
			}
		})
	}
}

// The case the gate missed when it read the draft off the review decision.
func TestMergeBlockSeesTheDraftUnderAnyReviewDecision(t *testing.T) {
	for _, decision := range []string{"", "APPROVED", "CHANGES_REQUESTED", "REVIEW_REQUIRED"} {
		t.Run("review="+decision, func(t *testing.T) {
			it := base()
			it.ReviewDecision = decision
			it.IsDraft = true

			block := MergeBlock(it)
			if !strings.Contains(block.Reason, "draft") {
				t.Errorf("MergeBlock = %q, want que mencione el borrador", block.Reason)
			}
			if !block.Hard {
				t.Error("Hard = false, want true: el borrador no se fuerza")
			}
		})
	}
}

func TestMergeBlockWarnsAboutConflictingBranches(t *testing.T) {
	it := base()
	it.TargetBranch = "main"
	it.Mergeable = model.Mergeability{Known: true, Conflicted: true}

	block := MergeBlock(it)
	if !strings.Contains(block.Reason, "conflicts") {
		t.Fatalf("Reason = %q, want que mencione el conflicto", block.Reason)
	}
	// The branch name first says what to rebase against, without vetoing: a rebase resolves it.
	if !strings.Contains(block.Reason, "main") {
		t.Errorf("Reason = %q, want que nombre la rama destino", block.Reason)
	}
	if block.Hard {
		t.Error("Hard = true, want false: un rebase lo arregla y el veto no tiene salida")
	}
}

func TestMergeBlockPrefersTheConflictOverTheCI(t *testing.T) {
	it := base()
	it.TargetBranch = "main"
	it.Mergeable = model.Mergeability{Known: true, Conflicted: true}
	it.Checks = model.Checks{State: model.ChecksFailing, Total: 4, Failing: 2}

	if reason := MergeBlock(it).Reason; !strings.Contains(reason, "conflicts") {
		t.Errorf("Reason = %q, want el conflicto antes que el CI", reason)
	}
}

func TestMergeBlockStaysQuietWithoutTheData(t *testing.T) {
	for _, tc := range []struct {
		name string
		m    model.Mergeability
	}{
		{"sin dato (GitHub UNKNOWN, la API de Todos)", model.Mergeability{}},
		{"integrable", model.Mergeability{Known: true}},
		{"integrable y sin conflicto", model.Mergeability{Known: true, Conflicted: false}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			it := base()
			it.TargetBranch = "main"
			it.Mergeable = tc.m
			if reason := MergeBlock(it).Reason; reason != "" {
				t.Errorf("Reason = %q, want silencio: no hay conflicto que anunciar", reason)
			}
		})
	}
}

// Red CI, running CI and requested changes are POLICY, not a veto.
func TestMergeBlockWarnsWithoutForbidding(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mutate func(*model.Item)
		want   string
	}{
		{"ci roja", func(it *model.Item) {
			it.Checks = model.Checks{State: model.ChecksFailing, Total: 5, Failing: 2}
		}, "2 of 5"},
		{"ci corriendo", func(it *model.Item) {
			it.Checks = model.Checks{State: model.ChecksPending, Total: 4, Pending: 1}
		}, "1 pending"},
		{"cambios pedidos", func(it *model.Item) {
			it.ReviewDecision = "CHANGES_REQUESTED"
		}, "changes were requested"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			it := base()
			tc.mutate(&it)
			block := MergeBlock(it)
			if block.Reason == "" {
				t.Fatal("MergeBlock = vacío, want un aviso")
			}
			if block.Hard {
				t.Errorf("Hard = true, want false: %s debe poder forzarse", tc.name)
			}
			if !strings.Contains(block.Reason, tc.want) {
				t.Errorf("Reason = %q, want que mencione %q", block.Reason, tc.want)
			}
		})
	}
}

func TestMergeBlockIsQuietOnAHealthyItem(t *testing.T) {
	if block := MergeBlock(base()); block.Reason != "" {
		t.Errorf("MergeBlock = %+v, want sin bloqueo ni aviso", block)
	}
}

func TestMergeBlockPrefersFailingCI(t *testing.T) {
	it := base()
	it.Checks = model.Checks{State: model.ChecksFailing, Total: 2, Failing: 1}
	it.ReviewDecision = "CHANGES_REQUESTED"

	block := MergeBlock(it)
	if !strings.Contains(block.Reason, "CI is failing") {
		t.Errorf("Reason = %q, want el motivo del CI", block.Reason)
	}
}
