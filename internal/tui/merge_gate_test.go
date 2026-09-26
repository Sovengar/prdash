// Tests del gate de merge en la TUI: qué frena, qué solo avisa, y qué pasa con
// una estrategia que el repositorio no admite.
//
// El gate no es una decisión de la TUI sino de state.MergeBlock, que ya tiene sus
// tests. Lo que se comprueba aquí es que la TUI lo consume sin diluirlo: que un
// bloqueo duro no arma, que uno blando arma pero se enseña, y que la lista de
// modos sale de las reglas del repositorio.
package tui

import (
	"strings"
	"testing"

	"prdash/internal/forge"
	"prdash/internal/forge/model"
)

// gatedItems devuelve un ítem sano con las reglas de merge del repositorio dadas.
func gatedItems(rules model.MergeRules) []model.Item {
	it := mkItem("github", "github.com", "acme/widget", "Add widget", 1, "")
	it.State = "open"
	it.Merge = rules
	return []model.Item{it}
}

// TestMergeBlockedOnADraftNoArms: un PR en borrador no se puede mergear en GitHub.
// Armar el merge solo para que la segunda tecla produzca un rechazo del forge
// gasta una llamada y teaches al operador que la confirmación no significa nada.
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

// TestMergeBlockedOnAMergedItemNoArms: un ítem ya mergeado está en el mismo
// grupo que el borrador por la misma razón —el forge lo rechaza—, pero el aviso
// tiene que decir cuál de los dos es.
func TestMergeBlockedOnAMergedItemNoArms(t *testing.T) {
	items := gatedItems(model.MergeRulesAll())
	items[0].State = "merged"
	f := newMergeFixture(t, items...)

	m := press(t, f.m, "m")
	if m.mergeArmed {
		t.Error("un ítem ya mergeado no debería armar el merge")
	}
}

// TestMergeOnFailingCIStillArmsButSays: el CI rojo no se puede prohibir —un check
// inestable dejaría el PR sin salida—, pero tampoco puede pasar inadvertido. El
// ítem arma y la confirmación lo dice, que es lo que convierte la segunda
// pulsación en una decisión y no en un reflejo.
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

// TestMergeConfirmationOnlyOffersAllowedModes: un repositorio con squash
// desactivado no debe ver `s` en la confirmación. La alternativa —ofrecerlo y
// dejar que el forge lo rechace— es la que producía el error sin contexto.
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

// TestMergeRefusesADisallowedMode: aunque la tecla llegue, el modo se revalida
// contra las reglas. Sin esto, el filtro del menú sería solo ASNUARIO y un modo
// no permitido se colaría por otra vía.
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

// TestMergeOnUnknownRulesOffersEveryMode: GitLab no publica las estrategias, así
// que llegan sin conocer. Filtrar sin dato dejaría al usuario sin salida
// legítima, que es peor que ofrecer una de más.
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

// TestMergePinsToTheHeadCommitItSaw: el argv real que sale del camino completo
// (armar, elegir modo, relectura, merge) lleva el pin. Si el pin viviera solo en
// el adapter y la TUI no lo supplyera, esto fallaría.
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

// TestMergeWithoutAHeadSHARefuses: sin SHA no hay pin y sin pin es el bug. La
// negación tiene que venir del adapter (que es quien sabe el flag de cada CLI),
// y el resultado no es un merge "de todas formas".
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
