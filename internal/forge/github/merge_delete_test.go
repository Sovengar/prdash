package github

import (
	"context"
	"strings"
	"testing"

	"prdash/internal/forge"
)

// TestMergeAsksForTheBranchDeletion: el flag de borrado solo aparece cuando la
// Confirmación lo pidió. Mandarlo siempre sería peor que no mandarlo: en un repo
// con merge queue gh RECHAZA el comando entero si ve `-d`, así que un `-d`
// implícito rompería merges que hoy funcionan.
func TestMergeAsksForTheBranchDeletion(t *testing.T) {
	for _, tc := range []struct {
		name   string
		delete bool
		want   bool // ¿debe aparecer --delete-branch?
	}{
		{"pedido", true, true},
		{"apagado", false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			bin, argsFile := recorder(t, dir, "gh")

			warns := New("github.com", bin).Merge(context.Background(), mergeRef, 7,
				forge.MergeRequest{Mode: forge.Squash, HeadSHA: headSHA, DeleteBranch: tc.delete})
			if len(warns) != 0 {
				t.Fatalf("Merge = %+v, want sin warnings", warns)
			}
			joined := strings.Join(readArgs(t, argsFile), " ")
			if got := strings.Contains(joined, "--delete-branch"); got != tc.want {
				t.Errorf("argv = %q, want --delete-branch presente = %v", joined, tc.want)
			}
		})
	}
}

// TestMergeWithDeleteStillPins: el borrado es un efecto POSTERIOR al merge, y
// añadirlo no puede relajar el pin. Un `--delete-branch` que se llevara por
// delante el `--match-head-commit` devolvería el bug que el pin arregla.
func TestMergeWithDeleteStillPins(t *testing.T) {
	dir := t.TempDir()
	bin, argsFile := recorder(t, dir, "gh")

	New("github.com", bin).Merge(context.Background(), mergeRef, 7,
		forge.MergeRequest{Mode: forge.Rebase, HeadSHA: headSHA, DeleteBranch: true})

	joined := strings.Join(readArgs(t, argsFile), " ")
	if !strings.Contains(joined, "--match-head-commit "+headSHA) {
		t.Errorf("argv = %q, want el pin a %s", joined, headSHA)
	}
}
