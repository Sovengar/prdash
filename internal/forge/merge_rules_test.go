package forge

import (
	"slices"
	"testing"

	"prdash/internal/forge/model"
)

// TestAllowedModesReadsTheRepository: un repositorio que desactiva squash no debe
// ver squash en el menú. Antes se ofrecían los tres siempre, así que la
// herramienta prometía una estrategia que el forge iba a rechazar.
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

// TestAllowedModesDoesNotRestrictWhatItDoesNotKnow: GitLab no publica las
// estrategias por GraphQL. Filtrar sin dato sería peor que no filtrar: dejaría
// fuera el único modo que el repositorio quizá sí admite, y el usuario se
// quedaría sin salida legítima. No saber no es lo mismo que no permitir.
func TestAllowedModesDoesNotRestrictWhatItDoesNotKnow(t *testing.T) {
	got := AllowedModes(model.MergeRules{})
	want := []MergeMode{Rebase, MergeCommit, Squash}
	if !slices.Equal(got, want) {
		t.Errorf("AllowedModes(sin conocer) = %v, want %v", got, want)
	}
}

// TestAllowedModesPutsRebaseFirst: el orden de la lista es el orden en que se le
// ofrecen al usuario, y rebase es la única estrategia que no reescribe la
// historia publicada. Que salga primero no lo hace el default —no hay default—,
// pero sí inclina el menú.
func TestAllowedModesPutsRebaseFirst(t *testing.T) {
	got := AllowedModes(model.MergeRulesAll())
	if len(got) == 0 || got[0] != Rebase {
		t.Errorf("AllowedModes = %v, want rebase el primero", got)
	}
}

// TestAllowsModeEsConsistentWithAllowedModes: la respuesta tiene que salir de la
// misma fuente que la lista, o el menú y el gate cuentan historias distintas.
func TestAllowsModeIsConsistentWithAllowedModes(t *testing.T) {
	rules := model.MergeRules{Known: true, Rebase: true, MergeCommit: true}
	if !AllowsMode(rules, Rebase) {
		t.Error("rebase está permitido por las reglas y AllowsMode lo niega")
	}
	if AllowsMode(rules, Squash) {
		t.Error("squash no está permitido por las reglas y AllowsMode lo admite")
	}
}
