package state

import (
	"strings"
	"testing"

	"prdash/internal/forge/model"
)

// base es un ítem abierto, sano y accionable: el punto de partida desde el que
// cada test ensucia un campo. Sin esto, cada caso repetiría el literal entero y
// un cambio en la forma del ítem tocaría veinte tests.
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

// TestMergeBlockRefusesWhatTheForgeRefuses: un bloqueo duro no lo levanta ninguna
// pulsación. Son las propiedades del forge, no políticas.
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

// TestMergeBlockSeesTheDraftUnderAnyReviewDecision: el caso que el gate no veía
// cuando leía el borrador en el estado crudo.
//
// base() viene aprobado, así que añadirle IsDraft deja un ítem que Derive
// clasifica como approved y cuyo borrador desaparece de la columna de estado.
// MergeBlock tiene que seguir frenando el merge, porque la pregunta que hace no
// es a quién mira el operador primero sino si el forge va a integrar esto, y
// GitHub rechaza el borrador antes de mirar la review que sea.
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

// TestMergeBlockWarnsAboutConflictingBranches: el conflicto de ramas se anuncia
// y no se veta.
//
// Es la asimetría del gate deliberada: GitHub no va a integrar el PR mientras las
// ramas se pisen, pero un rebase lo arregla en un comando y el gate no puede
// saber si el usuario lo ha hecho ya. Vetarlo dejaría al PR sin salida desde
// aquí; no decirlo gastaría una llamada entera en descubrirlo.
func TestMergeBlockWarnsAboutConflictingBranches(t *testing.T) {
	it := base()
	it.TargetBranch = "main"
	it.Mergeable = model.Mergeability{Known: true, Conflicted: true}

	block := MergeBlock(it)
	if !strings.Contains(block.Reason, "conflicts") {
		t.Fatalf("Reason = %q, want que mencione el conflicto", block.Reason)
	}
	// El nombre de la rama delante dice qué hay que rebasar, y sin vetar: un
	// rebase lo arregla, así que la segunda pulsación tiene que servir.
	if !strings.Contains(block.Reason, "main") {
		t.Errorf("Reason = %q, want que nombre la rama destino", block.Reason)
	}
	if block.Hard {
		t.Error("Hard = true, want false: un rebase lo arregla y el veto no tiene salida")
	}
}

// TestMergeBlockPrefersTheConflictOverTheCI: un PR que choca tampoco pasa el CI,
// y decir "CI is failing" manda al operador a mirar el sitio equivocado. El
// conflicto va primero porque es lo que hay que rehacer.
func TestMergeBlockPrefersTheConflictOverTheCI(t *testing.T) {
	it := base()
	it.TargetBranch = "main"
	it.Mergeable = model.Mergeability{Known: true, Conflicted: true}
	it.Checks = model.Checks{State: model.ChecksFailing, Total: 4, Failing: 2}

	if reason := MergeBlock(it).Reason; !strings.Contains(reason, "conflicts") {
		t.Errorf("Reason = %q, want el conflicto antes que el CI", reason)
	}
}

// TestMergeBlockStaysQuietWithoutTheData: lo que no se sabe no se anuncia. Un
// aviso de conflicto que sale sin datos es un aviso falso, y un aviso falso que
// se repite entrena a ignorar la caja.
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

// TestMergeBlockWarnsWithoutForbidding: el CI rojo, el CI corriendo y los cambios
// pedidos son política, no propiedad del forge. Bloquearlos del todo convertiría
// la herramienta en un muro —un check inestable dejaría el PR sin poder mergear
// nunca— y no bloquearlos los haría invisibles. Se anuncian y se piden dos
// veces.
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

// TestMergeBlockIsQuietOnAHealthyItem: el caso normal no puede producir ruido. Un
// gate que avisa siempre entrena al operador a ignorar el aviso, que es
// exactamente lo que un gate debe evitar.
func TestMergeBlockIsQuietOnAHealthyItem(t *testing.T) {
	if block := MergeBlock(base()); block.Reason != "" {
		t.Errorf("MergeBlock = %+v, want sin bloqueo ni aviso", block)
	}
}

// TestMergeBlockPrefersFailingCI: con el CI rojo y cambios pedidos a la vez, el
// motivo es el CI. Es el dato que el operador necesita primero, y la precedencia
// es la misma que ya usa Derive para ordenar el inbox.
func TestMergeBlockPrefersFailingCI(t *testing.T) {
	it := base()
	it.Checks = model.Checks{State: model.ChecksFailing, Total: 2, Failing: 1}
	it.ReviewDecision = "CHANGES_REQUESTED"

	block := MergeBlock(it)
	if !strings.Contains(block.Reason, "CI is failing") {
		t.Errorf("Reason = %q, want el motivo del CI", block.Reason)
	}
}
