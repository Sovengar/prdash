package github

import (
	"context"
	"os"
	"strings"
	"testing"

	"prdash/internal/forge"
)

// TestMergePinsTheHeadCommit blinda el arreglo: sin `--match-head-commit`, `gh
// pr merge` integra el HEAD del momento. Entre el refresco del inbox y la
// pulsación la rama puede haber avanzado, y entonces el merge se lleva commits
// que nadie revisó. El pin no es una mejora: es la diferencia entre integrar lo
// que se leyó y lo que hay.
func TestMergePinsTheHeadCommit(t *testing.T) {
	for _, mode := range []forge.MergeMode{forge.MergeCommit, forge.Rebase, forge.Squash} {
		dir := t.TempDir()
		bin, argsFile := recorder(t, dir, "gh")
		const sha = "abc1234"

		warns := New("github.com", bin).Merge(context.Background(), mergeRef, 7, mode, sha)
		if len(warns) != 0 {
			t.Fatalf("modo %q: Merge = %+v, want sin warnings", mode, warns)
		}
		joined := strings.Join(readArgs(t, argsFile), " ")
		if !strings.Contains(joined, "--match-head-commit "+sha) {
			t.Errorf("modo %q: argv = %q, want el pin a %s", mode, joined, sha)
		}
	}
}

// TestMergeRefusesToPinNothing: sin SHA no hay pin posible. Degradar a un merge
// sin pin sería volver al bug que el pin arregla, así que se niega y lo dice.
func TestMergeRefusesToPinNothing(t *testing.T) {
	for _, sha := range []string{"", "   "} {
		dir := t.TempDir()
		bin, argsFile := recorder(t, dir, "gh")

		warns := New("github.com", bin).Merge(context.Background(), mergeRef, 7, forge.Rebase, sha)
		if len(warns) == 0 {
			t.Fatalf("sha %q: un merge sin pin debería reportar warning", sha)
		}
		if warns[0].Kind != "unsupported" {
			t.Errorf("sha %q: Kind = %q, want unsupported", sha, warns[0].Kind)
		}
		if _, err := os.Stat(argsFile); err == nil {
			t.Errorf("sha %q: no debería haber lanzado la CLI sin poder pinear", sha)
		}
	}
}

// TestMergeRefusesAnUnknownModeBeforePinning: el modo se valida primero. Un modo
// corrupto con un SHA válido no debe colarse hasta el punto de construir el
// argv, porque `gh pr merge` sin flag de estrategia abre un prompt.
func TestMergeRefusesAnUnknownModeBeforePinning(t *testing.T) {
	dir := t.TempDir()
	bin, argsFile := recorder(t, dir, "gh")

	warns := New("github.com", bin).Merge(context.Background(), mergeRef, 7, forge.MergeMode("cherry-pick"), "abc1234")
	if len(warns) == 0 || warns[0].Kind != "unsupported" {
		t.Fatalf("warnings = %+v, want unsupported", warns)
	}
	if _, err := os.Stat(argsFile); err == nil {
		t.Error("no debería haber lanzado la CLI con un modo desconocido")
	}
}

// TestPRFieldsAskForThePinAndTheRules: los dos datos que hacen posibles el pin y
// el filtro de modos tienen que estar en la query, o el resto no sirve de nada.
// Sin headRefOid el pin es siempre imposible y sin los merge*Allowed el filtro
// se queda sin datos.
func TestPRFieldsAskForThePinAndTheRules(t *testing.T) {
	for _, field := range []string{
		"headRefOid",
		"mergeCommitAllowed",
		"rebaseMergeAllowed",
		"squashMergeAllowed",
	} {
		if !strings.Contains(ghPRFields, field) {
			t.Errorf("ghPRFields no pide %q", field)
		}
	}
}
