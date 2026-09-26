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
		{"draft", func(it *model.Item) { it.State = "draft" }, "draft"},
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
