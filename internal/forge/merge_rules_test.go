package forge

import (
	"slices"
	"testing"

	"prdash/internal/forge/model"
)

func TestAllowedModesReadsTheRepository(t *testing.T) {
	for _, tc := range []struct {
		name  string
		rules model.MergeRules
		want  []MergeMode
	}{
		{"solo rebase", model.MergeRules{Known: true, Rebase: true}, []MergeMode{Rebase}},
		{"rebase y squash", model.MergeRules{Known: true, Rebase: true, Squash: true}, []MergeMode{Rebase, Squash}},
		{"los tres", model.MergeRulesAll(), []MergeMode{Rebase, MergeCommit, Squash}},
		{"ninguno", model.MergeRules{Known: true}, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := AllowedModes(tc.rules); !slices.Equal(got, tc.want) {
				t.Errorf("AllowedModes = %v, want %v", got, tc.want)
			}
		})
	}
}

// GitLab does not publish the strategies over GraphQL; filtering with no data would be worse than
// not filtering.
func TestAllowedModesDoesNotRestrictWhatItDoesNotKnow(t *testing.T) {
	got := AllowedModes(model.MergeRules{})
	want := []MergeMode{Rebase, MergeCommit, Squash}
	if !slices.Equal(got, want) {
		t.Errorf("AllowedModes(sin conocer) = %v, want %v", got, want)
	}
}

// The list's order is the order they are offered in, and rebase comes first.
func TestAllowedModesPutsRebaseFirst(t *testing.T) {
	got := AllowedModes(model.MergeRulesAll())
	if len(got) == 0 || got[0] != Rebase {
		t.Errorf("AllowedModes = %v, want rebase el primero", got)
	}
}

func TestAllowsModeIsConsistentWithAllowedModes(t *testing.T) {
	rules := model.MergeRules{Known: true, Rebase: true, MergeCommit: true}
	if !AllowsMode(rules, Rebase) {
		t.Error("rebase está permitido por las reglas y AllowsMode lo niega")
	}
	if AllowsMode(rules, Squash) {
		t.Error("squash no está permitido por las reglas y AllowsMode lo admite")
	}
}
