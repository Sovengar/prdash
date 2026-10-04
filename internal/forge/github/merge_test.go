package github

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"prdash/internal/forge"
	"prdash/internal/forge/model"
)

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

var mergeRef = model.RepoRef{Forge: "github", Host: "github.com", Project: "acme/widget"}

const headSHA = "9f1c0de"

// The mode is not optional.
func TestMergePassesTheStrategyFlag(t *testing.T) {
	for _, tc := range []struct {
		mode forge.MergeMode
		want string
	}{
		{forge.MergeCommit, "--merge"},
		{forge.Rebase, "--rebase"},
		{forge.Squash, "--squash"},
	} {
		t.Run(string(tc.mode), func(t *testing.T) {
			dir := t.TempDir()
			bin, argsFile := recorder(t, dir, "gh")

			warns := New("github.com", bin).Merge(context.Background(), mergeRef, 7, forge.MergeRequest{Mode: tc.mode, HeadSHA: headSHA})
			if len(warns) != 0 {
				t.Fatalf("Merge = %+v, want sin warnings", warns)
			}
			args := strings.Join(readArgs(t, argsFile), " ")
			if !strings.Contains(args, "pr merge 7") {
				t.Errorf("argv = %q, want la forma de pr merge", args)
			}
			if !strings.Contains(args, tc.want) {
				t.Errorf("argv = %q, want el flag %q", args, tc.want)
			}
		})
	}
}

// An unknown mode is a warning, not an argv without the flag.
func TestMergeRefusesUnknownMode(t *testing.T) {
	dir := t.TempDir()
	bin, argsFile := recorder(t, dir, "gh")

	warns := New("github.com", bin).Merge(context.Background(), mergeRef, 7, forge.MergeRequest{Mode: forge.MergeMode("cherry-pick"), HeadSHA: headSHA})
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
