package gitlab

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"prdash/internal/forge"
	"prdash/internal/forge/model"
)

// recorder devuelve una CLI falsa que vuelca sus argumentos en un fichero.
func recorder(t *testing.T, dir, name string) (bin, argsFile string) {
	t.Helper()
	argsFile = filepath.Join(dir, name+".args")
	bin = writeScript(t, dir, name, "#!/bin/sh\nprintf '%s\\n' \"$@\" > "+argsFile+"\n")
	return bin, argsFile
}

func readArgs(t *testing.T, path string) []string {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return strings.Fields(string(raw))
}

var mergeRef = model.RepoRef{Forge: "gitlab", Host: "gitlab.example.com", Project: "grp/proj"}

// TestMergePassesTheStrategyFlag: rebase y squash llevan su flag, y merge commit
// no lleva ninguno porque en glab es la ausencia de estrategia.
func TestMergePassesTheStrategyFlag(t *testing.T) {
	for _, tc := range []struct {
		mode forge.MergeMode
		want string // "" = no debe aparecer flag de estrategia
	}{
		{forge.MergeCommit, ""},
		{forge.Rebase, "--rebase"},
		{forge.Squash, "--squash"},
	} {
		t.Run(string(tc.mode), func(t *testing.T) {
			dir := t.TempDir()
			bin, argsFile := recorder(t, dir, "glab")

			warns := New("gitlab.example.com", bin).Merge(context.Background(), mergeRef, 7, tc.mode)
			if len(warns) != 0 {
				t.Fatalf("Merge = %+v, want sin warnings", warns)
			}
			args := readArgs(t, argsFile)
			joined := strings.Join(args, " ")
			if !strings.Contains(joined, "mr merge 7") {
				t.Errorf("argv = %q, want la forma de mr merge", joined)
			}
			if tc.want == "" {
				for _, forbidden := range []string{"--squash", "--rebase"} {
					if strings.Contains(joined, forbidden) {
						t.Errorf("argv = %q; merge commit no debe llevar %q", joined, forbidden)
					}
				}
			} else if !strings.Contains(joined, tc.want) {
				t.Errorf("argv = %q, want el flag %q", joined, tc.want)
			}
		})
	}
}

// TestMergeDisablesAutoMerge cubre un bug silencioso: glab tiene `--auto-merge`
// en true por defecto, así que con un pipeline en marcha `glab mr merge` no
// mergeaba, solo dejaba el MR en cola de auto-merge y salía con exit 0. El TUI
// informaba "merge ok" de un MR que seguía abierto.
func TestMergeDisablesAutoMerge(t *testing.T) {
	for _, mode := range []forge.MergeMode{forge.MergeCommit, forge.Rebase, forge.Squash} {
		dir := t.TempDir()
		bin, argsFile := recorder(t, dir, "glab")

		New("gitlab.example.com", bin).Merge(context.Background(), mergeRef, 7, mode)

		joined := strings.Join(readArgs(t, argsFile), " ")
		if !strings.Contains(joined, "--auto-merge=false") {
			t.Errorf("modo %q: argv = %q, want --auto-merge=false", mode, joined)
		}
		if !strings.Contains(joined, "--yes") {
			t.Errorf("modo %q: argv = %q, want --yes", mode, joined)
		}
	}
}

func TestMergeRefusesUnknownMode(t *testing.T) {
	dir := t.TempDir()
	bin, argsFile := recorder(t, dir, "glab")

	warns := New("gitlab.example.com", bin).Merge(context.Background(), mergeRef, 7, forge.MergeMode("cherry-pick"))
	if len(warns) == 0 {
		t.Fatal("un modo desconocido debería reportar warning")
	}
	if warns[0].Kind != "unsupported" {
		t.Errorf("Kind = %q, want unsupported", warns[0].Kind)
	}
	if _, err := os.Stat(argsFile); err == nil {
		t.Error("no debería haber lanzado la CLI con un modo desconocido")
	}
}
