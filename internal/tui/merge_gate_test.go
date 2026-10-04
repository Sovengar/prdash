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
		t.Error("un ítem en borrador no debería armar el merge")
	}
	if !strings.Contains(toasts(m), "draft") {
		t.Errorf("el aviso no explica el bloqueo: %q", toasts(m))
	}
}

func TestMergeBlockedOnAMergedItemNoArms(t *testing.T) {
	items := gatedItems(model.MergeRulesAll())
	items[0].State = "merged"
	f := newMergeFixture(t, items...)

	m := press(t, f.m, "m")
	if m.mergeArmed {
		t.Error("un ítem ya mergeado no debería armar el merge")
	}
}

// Red CI cannot be forbidden: an unstable check would otherwise leave the user stuck.
func TestMergeOnFailingCIStillArmsButSays(t *testing.T) {
	items := gatedItems(model.MergeRulesAll())
	items[0].Checks = model.Checks{State: model.ChecksFailing, Total: 5, Failing: 2}
	f := newMergeFixture(t, items...)

	m := press(t, f.m, "m")
	if !m.mergeArmed {
		t.Fatal("con el CI en rojo el merge debería armar: el bloqueo es blando")
	}
	view := stripANSI(m.View().Content)
	if !strings.Contains(view, "CI is failing") {
		t.Errorf("la confirmación no menciona el CI en rojo:\n%s", view)
	}
	if !strings.Contains(view, "anyway") {
		t.Errorf("la confirmación no dice que elegir el modo es seguir adelante:\n%s", view)
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
		t.Fatal("con las ramas en conflicto el merge debería armar: un rebase lo arregla")
	}
	view := stripANSI(m.View().Content)
	if !strings.Contains(view, "conflicts with main") {
		t.Errorf("la confirmación no dice contra qué choca:\n%s", view)
	}
	if !strings.Contains(view, "anyway") {
		t.Errorf("la confirmación no dice que elegir el modo es seguir adelante:\n%s", view)
	}
}

// When the forge does not say (GitHub UNKNOWN) the result stays quiet.
func TestMergeWithoutMergeabilityDataStaysQuiet(t *testing.T) {
	for _, tc := range []struct {
		name string
		m    model.Mergeability
	}{
		{"sin dato", model.Mergeability{}},
		{"integrable", model.Mergeability{Known: true}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			items := gatedItems(model.MergeRulesAll())
			items[0].TargetBranch = "main"
			items[0].Mergeable = tc.m
			f := newMergeFixture(t, items...)

			m := press(t, f.m, "m")
			if !m.mergeArmed {
				t.Fatal("el merge debería armar")
			}
			if view := stripANSI(m.View().Content); strings.Contains(view, "conflicts") {
				t.Errorf("sin dato no debe anunciarse un conflicto:\n%s", view)
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
		t.Errorf("el aviso debería decir que el forge lo rechazó, no que hubo un conflicto: %q", notice)
	}
	if strings.Contains(notice, "forge conflict") {
		t.Errorf("un refresco no arregla un rebase: %q", notice)
	}
	if !strings.Contains(notice, "rebase") {
		t.Errorf("el aviso debería decir qué hacer: %q", notice)
	}
	// And it must not stay recorded as denied: a rebase makes it integrable again.
	if len(m.denied) != 0 {
		t.Errorf("el ítem no debería quedar denegado para siempre: %+v", m.denied)
	}
}

func TestMergeConfirmationOnlyOffersAllowedModes(t *testing.T) {
	f := newMergeFixture(t, gatedItems(model.MergeRules{Known: true, Rebase: true, MergeCommit: true})...)

	m := press(t, f.m, "m")
	view := stripANSI(m.View().Content)
	if strings.Contains(view, "s squash") {
		t.Errorf("la confirmación ofrece squash y el repositorio lo tiene desactivado:\n%s", view)
	}
	if !strings.Contains(view, "r rebase") {
		t.Errorf("la confirmación no ofrece rebase, que sí está permitido:\n%s", view)
	}
}

func TestMergeRefusesADisallowedMode(t *testing.T) {
	f := newMergeFixture(t, gatedItems(model.MergeRules{Known: true, Rebase: true})...)

	m := press(t, f.m, "m")
	m = press(t, m, "s")
	if m.actionBusy {
		t.Error("un modo no permitido no debería lanzar el merge")
	}
	if !strings.Contains(toasts(m), "does not allow") {
		t.Errorf("el aviso no explica el rechazo: %q", toasts(m))
	}
}

func TestMergeOnUnknownRulesOffersEveryMode(t *testing.T) {
	f := newMergeFixture(t, gatedItems(model.MergeRules{})...)

	m := press(t, f.m, "m")
	view := stripANSI(m.View().Content)
	for _, want := range []string{"r rebase", "m merge commit", "s squash"} {
		if !strings.Contains(view, want) {
			t.Errorf("sin reglas conocidas la confirmación debería ofrecer %q:\n%s", want, view)
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
		t.Error("un merge sin pin no debería salir como ok")
	}
	if n := f.adp.MergeModeCount(forge.Rebase); n != 0 {
		t.Errorf("hubo %d llamada(s) a Merge, want 0: no hay pin posible", n)
	}
}
