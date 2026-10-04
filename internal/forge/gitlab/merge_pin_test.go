package gitlab

import (
	"context"
	"os"
	"strings"
	"testing"

	"prdash/internal/forge"
)

func TestMergePinsTheHeadCommit(t *testing.T) {
	for _, mode := range []forge.MergeMode{forge.MergeCommit, forge.Rebase, forge.Squash} {
		dir := t.TempDir()
		bin, argsFile := recorder(t, dir, "glab")
		const sha = "abc1234"

		warns := New("gitlab.example.com", bin).Merge(context.Background(), mergeRef, 7, forge.MergeRequest{Mode: mode, HeadSHA: sha})
		if len(warns) != 0 {
			t.Fatalf("modo %q: Merge = %+v, want sin warnings", mode, warns)
		}
		joined := strings.Join(readArgs(t, argsFile), " ")
		if !strings.Contains(joined, "--sha "+sha) {
			t.Errorf("modo %q: argv = %q, want el pin a %s", mode, joined, sha)
		}
	}
}

func TestMergeRefusesToPinNothing(t *testing.T) {
	for _, sha := range []string{"", "  "} {
		dir := t.TempDir()
		bin, argsFile := recorder(t, dir, "glab")

		warns := New("gitlab.example.com", bin).Merge(context.Background(), mergeRef, 7, forge.MergeRequest{Mode: forge.Squash, HeadSHA: sha})
		if len(warns) == 0 {
			t.Fatalf("sha %q: un merge sin pin debería reportar warning", sha)
		}
		if _, err := os.Stat(argsFile); err == nil {
			t.Errorf("sha %q: no debería haber lanzado la CLI sin poder pinear", sha)
		}
	}
}

// diffHeadSha has to be in the query; Project.mergeMethod does not exist in the schema.
func TestMRFieldsAskForThePin(t *testing.T) {
	if !strings.Contains(mrFields, "diffHeadSha") {
		t.Error("mrFields no pide diffHeadSha, así que el pin nunca podría satisfacerse")
	}
	// The negative assertion documents a decision, not a wish: if the schema ever grows it, the test
	// says so.
	if strings.Contains(mrFields, "mergeMethod") {
		t.Log("mrFields pide mergeMethod: la instancia lo soporta, se puede filtrar por reglas")
	}
}
