// Tests del toggle de borrado de rama en la Confirmación de merge: dónde se ve,
// quién lo cambia y qué llega al adapter.
//
// El borrado es la segunda de las dos cosas que el merge nombra, así que se
// comporta como el modo: solo se puede decidir en la Confirmación, se ve en la
// caja (no en un toast que caduca) y lo que se ve es lo que sale.
package tui

import (
	"strings"
	"testing"

	"prdash/internal/forge"
)

// TestDeleteBranchShowsInTheArmedConfirm: el valor se ve con el merge armado y
// solo entonces. Fuera de ahí no es una decisión que se pueda tomar, así que
// anunciarlo sería ruido.
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

// TestDeleteBranchIsOnByDefault: el merge borra la rama salvo que se apague. Es
// lo que hacen los forges por su cuenta y lo que espera quien limpia detrás de
// un PR; exigir un gesto para hallmark de más sería pedir confirmaciones de las
// que la gente se cansa.
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

// TestTabTogglesTheDeleteWithoutDisarming: `tab` cambia el valor y NO ejecuta
// nada. Si desarmara, apagar el borrado sería una forma de cancelar el merge, y
// la tecla que ejecuta pasaría a ser la tercera pulsación en un camino y la
// segunda en el otro.
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

// TestTabToggleReachesTheAdapter: el toggle no es decorativo. Lo que la caja
// dice es lo que el argv tiene que llevar.
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

// TestDeleteToggleStaysForTheSession: apagarlo una vez lo apaga para los merges
// siguientes, no solo para el que está en pantalla. Re-pulsarlo lo devuelve a
// `yes`, así que la sesión se puede dejar como se quiera sin salir de la TUI.
func TestDeleteToggleStaysForTheSession(t *testing.T) {
	f := newMergeFixture(t, mergeItems()...)

	m := press(t, f.m, "m")
	m = press(t, m, "tab")
	m = press(t, m, "s")
	// El resultado se aplica al modelo: sin eso la acción sigue marcada como en
	// curso y la segunda pulsación no sale.
	m = send(t, m, actionMsg{cycle: m.cycle, outcome: waitOutcome(t, m)})

	if got := f.adp.MergeDeleteCount(false); got != 1 {
		t.Fatalf("el primer merge: %d sin borrado, want 1", got)
	}

	// Segundo merge sin tocar el toggle: hereda el valor apagado.
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

// TestTabOutsideTheArmedMergeCyclesSections: `tab` con el merge armado es el
// toggle, y FUERA de ahí sigue siendo lo que siempre fue, la sección
// siguiente. Un estado que se quedara con la tecla entera rompería la
// navegación.
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

// TestDeleteOnlyTravelsWithAMerge: approve no lleva la housekeeping del merge. Es
// lo único que hace que el aviso de approve no hable de ramas.
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

// TestMergeNoticeNamesTheBranch: el resultado dice qué pasó con la rama, no solo
// que el merge salió. Un "merge ok" a secas deja en suspense lo que pasó con la
// rama que el usuario pidió borrar, que es justo lo que se pierde después
// porque en el siguiente refresco ya no está.
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

// TestMergeNoticeSaysWhenTheBranchSurvived: cuando el borrado falla tras un merge
// que sí salió, el aviso lo dice y NO dice "merge falló". Reportarlo como fallo
// haría que el usuario buscara un cambio de estado del forge que no ocurrió.
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
