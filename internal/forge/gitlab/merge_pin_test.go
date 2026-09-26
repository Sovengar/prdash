package gitlab

import (
	"context"
	"os"
	"strings"
	"testing"

	"prdash/internal/forge"
)

// TestMergePinsTheHeadCommit blinda el arreglo: `--sha` es lo que hace que `glab
// mr merge` rechace el merge si la rama se movió. Es el mismo género de trampa
// que `--auto-merge` pero al revés: sin el pin, GitLab integra el HEAD del
// momento, que puede no ser el que se leyó y revisó.
func TestMergePinsTheHeadCommit(t *testing.T) {
	for _, mode := range []forge.MergeMode{forge.MergeCommit, forge.Rebase, forge.Squash} {
		dir := t.TempDir()
		bin, argsFile := recorder(t, dir, "glab")
		const sha = "abc1234"

		warns := New("gitlab.example.com", bin).Merge(context.Background(), mergeRef, 7, mode, sha)
		if len(warns) != 0 {
			t.Fatalf("modo %q: Merge = %+v, want sin warnings", mode, warns)
		}
		joined := strings.Join(readArgs(t, argsFile), " ")
		if !strings.Contains(joined, "--sha "+sha) {
			t.Errorf("modo %q: argv = %q, want el pin a %s", mode, joined, sha)
		}
	}
}

// TestMergeRefusesToPinNothing: sin SHA no hay pin posible, y un merge sin pin es
// exactamente el fallo que este arreglo evita.
func TestMergeRefusesToPinNothing(t *testing.T) {
	for _, sha := range []string{"", "  "} {
		dir := t.TempDir()
		bin, argsFile := recorder(t, dir, "glab")

		warns := New("gitlab.example.com", bin).Merge(context.Background(), mergeRef, 7, forge.Squash, sha)
		if len(warns) == 0 {
			t.Fatalf("sha %q: un merge sin pin debería reportar warning", sha)
		}
		if _, err := os.Stat(argsFile); err == nil {
			t.Errorf("sha %q: no debería haber lanzado la CLI sin poder pinear", sha)
		}
	}
}

// TestMRFieldsAskForThePin: `diffHeadSha` tiene que estar en la query. Es
// comprobable contra el schema real de la instancia: `Project.mergeMethod`, que
// sería lo que traería las estrategias admitidas, NO existe ahí, y por eso las
// reglas de merge llegan sin conocer en GitLab.
func TestMRFieldsAskForThePin(t *testing.T) {
	if !strings.Contains(mrFields, "diffHeadSha") {
		t.Error("mrFields no pide diffHeadSha, así que el pin nunca podría satisfacerse")
	}
	// La aserción negativa documenta una decisión, no un deseo: si algún día el
	// schema de la instancia expone mergeMethod, hay que ir a por las reglas.
	if strings.Contains(mrFields, "mergeMethod") {
		t.Log("mrFields pide mergeMethod: la instancia lo soporta, se puede filtrar por reglas")
	}
}
