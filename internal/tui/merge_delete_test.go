package tui

import (
	"strings"
	"testing"

	"prdash/internal/forge"
)

func TestDeleteBranchShowsInTheArmedConfirm(t *testing.T) {
	f := newMergeFixture(t, mergeItems()...)

	if view := stripANSI(f.m.View().Content); strings.Contains(view, "delete branch") {
		t.Errorf("sin merge armado el borrado no debería aparecer\n%s", view)
	}

	m := press(t, f.m, "m")
	view := stripANSI(m.View().Content)
	if !strings.Contains(view, "delete branch: yes") {
		t.Errorf("la confirmación no muestra el valor por defecto\n%s", view)
	}
	if !strings.Contains(view, "tab") {
		t.Errorf("la confirmación no dice cómo se cambia\n%s", view)
	}
}

// The merge deletes the branch unless turned off, which is what the forges do on their own.
func TestDeleteBranchIsOnByDefault(t *testing.T) {
	f := newMergeFixture(t, mergeItems()...)
	m := press(t, f.m, "m")
	m = press(t, m, "s")
	out := waitOutcome(t, m)

	if !out.OK {
		t.Fatalf("outcome = %+v", out)
	}
	if !out.DeleteBranch {
		t.Error("el merge debería pedir borrar la rama por defecto")
	}
	if got := f.adp.MergeDeleteCount(true); got != 1 {
		t.Errorf("el adapter recibió %d merge(s) con borrado, want 1", got)
	}
}

// tab changes the value and executes NOTHING. Disarming would make the keypress pointless.
func TestTabTogglesTheDeleteWithoutDisarming(t *testing.T) {
	f := newMergeFixture(t, mergeItems()...)
	m := press(t, f.m, "m")
	m = press(t, m, "tab")

	if !m.mergeArmed {
		t.Fatal("`tab` no debería desarmar el merge")
	}
	if m.actionBusy {
		t.Fatal("`tab` no debería lanzar la acción")
	}
	if m.deleteBranch {
		t.Error("`tab` debería haber apagado el borrado")
	}
	if view := stripANSI(m.View().Content); !strings.Contains(view, "delete branch: no") {
		t.Errorf("la caja no refleja el valor nuevo\n%s", view)
	}
}

func TestTabToggleReachesTheAdapter(t *testing.T) {
	f := newMergeFixture(t, mergeItems()...)
	m := press(t, f.m, "m")
	m = press(t, m, "tab")
	m = press(t, m, "s")
	out := waitOutcome(t, m)

	if out.DeleteBranch {
		t.Error("el merge no debería pedir borrar la rama tras apagarla")
	}
	if got := f.adp.MergeDeleteCount(false); got != 1 {
		t.Errorf("el adapter recibió %d merge(s) sin borrado, want 1", got)
	}
	if got := f.adp.MergeDeleteCount(true); got != 0 {
		t.Errorf("el adapter recibió %d merge(s) con borrado, want 0", got)
	}
}

func TestDeleteToggleStaysForTheSession(t *testing.T) {
	f := newMergeFixture(t, mergeItems()...)

	m := press(t, f.m, "m")
	m = press(t, m, "tab")
	m = press(t, m, "s")
	// The result reaches the model: without it the action stays marked as in progress.
	m = send(t, m, actionMsg{cycle: m.cycle, outcome: waitOutcome(t, m)})

	if got := f.adp.MergeDeleteCount(false); got != 1 {
		t.Fatalf("el primer merge: %d sin borrado, want 1", got)
	}

	m = press(t, m, "m")
	if m.deleteBranch {
		t.Error("el valor debería seguir apagado en el siguiente merge")
	}
	m = press(t, m, "s")
	_ = waitOutcome(t, m)

	if got := f.adp.MergeDeleteCount(false); got != 2 {
		t.Errorf("el adapter recibió %d merge(s) sin borrado, want 2", got)
	}
}

// With the merge armed tab is the toggle; OUTSIDE it cycles sections.
func TestTabOutsideTheArmedMergeCyclesSections(t *testing.T) {
	f := newMergeFixture(t, mergeItems()...)
	before := f.m.activeSection

	m := press(t, f.m, "tab")

	if m.activeSection == before {
		t.Error("`tab` sin merge armado debería cambiar de sección")
	}
	if m.deleteBranch != f.m.deleteBranch {
		t.Error("`tab` fuera del armado no debería tocar el borrado")
	}
}

func TestDeleteOnlyTravelsWithAMerge(t *testing.T) {
	f := newMergeFixture(t, mergeItems()...)
	m := press(t, f.m, "a")
	out := waitOutcome(t, m)

	if out.Kind != forge.ActionApprove {
		t.Fatalf("Kind = %q, want approve", out.Kind)
	}
	if out.DeleteBranch {
		t.Error("approve no debería pedir borrar la rama")
	}
}

// The result says what happened to the branch, not only that the merge worked.
func TestMergeNoticeNamesTheBranch(t *testing.T) {
	f := newMergeFixture(t, mergeItems()...)
	m := press(t, f.m, "m")
	m = press(t, m, "s")
	out := waitOutcome(t, m)
	m = send(t, m, actionMsg{cycle: m.cycle, outcome: out})

	notice := lastToast(m)
	if !strings.Contains(notice, "branch deleted") {
		t.Errorf("el aviso debería confirmar el borrado: %q", notice)
	}
}

func TestMergeNoticeSaysWhenTheBranchSurvived(t *testing.T) {
	f := newMergeFixture(t, mergeItems()...)
	m := press(t, f.m, "m")
	m = press(t, m, "s")
	out := waitOutcome(t, m)
	out.DeleteMsg = "Resource not accessible by integration"
	m = send(t, m, actionMsg{cycle: m.cycle, outcome: out})

	notice := lastToast(m)
	if !strings.Contains(notice, "branch not deleted") {
		t.Errorf("el aviso debería decir que la rama sigue ahí: %q", notice)
	}
	if !strings.Contains(notice, "Resource not accessible") {
		t.Errorf("el aviso debería dar el motivo del forge: %q", notice)
	}
	if strings.Contains(notice, "error:") {
		t.Errorf("un merge que salió no es un error: %q", notice)
	}
}
