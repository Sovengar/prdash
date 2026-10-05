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

const headSHA = "9f1c0de"

func TestMergePassesTheStrategyFlag(t *testing.T) {
	for _, tc := range []struct {
		mode forge.MergeMode
		want string // "" = no strategy flag must appear
	}{
		{forge.MergeCommit, ""},
		{forge.Rebase, "--rebase"},
		{forge.Squash, "--squash"},
	} {
		t.Run(string(tc.mode), func(t *testing.T) {
			dir := t.TempDir()
			bin, argsFile := recorder(t, dir, "glab")

			warns := New("gitlab.example.com", bin).Merge(context.Background(), mergeRef, 7, forge.MergeRequest{Mode: tc.mode, HeadSHA: headSHA})
			if len(warns) != 0 {
				t.Fatalf("Merge = %+v, want no warnings", warns)
			}
			args := readArgs(t, argsFile)
			joined := strings.Join(args, " ")
			if !strings.Contains(joined, "mr merge 7") {
				t.Errorf("argv = %q, want the mr merge shape", joined)
			}
			if tc.want == "" {
				for _, forbidden := range []string{"--squash", "--rebase"} {
					if strings.Contains(joined, forbidden) {
						t.Errorf("argv = %q; merge commit must not carry %q", joined, forbidden)
					}
				}
			} else if !strings.Contains(joined, tc.want) {
				t.Errorf("argv = %q, want the flag %q", joined, tc.want)
			}
		})
	}
}

// A silent bug: glab defaults --auto-merge to true, so with a pipeline running the command did
// not merge, it queued the MR and exited 0.
func TestMergeDisablesAutoMerge(t *testing.T) {
	for _, mode := range []forge.MergeMode{forge.MergeCommit, forge.Rebase, forge.Squash} {
		dir := t.TempDir()
		bin, argsFile := recorder(t, dir, "glab")

		New("gitlab.example.com", bin).Merge(context.Background(), mergeRef, 7, forge.MergeRequest{Mode: mode, HeadSHA: headSHA})

		joined := strings.Join(readArgs(t, argsFile), " ")
		if !strings.Contains(joined, "--auto-merge=false") {
			t.Errorf("mode %q: argv = %q, want --auto-merge=false", mode, joined)
		}
		if !strings.Contains(joined, "--yes") {
			t.Errorf("mode %q: argv = %q, want --yes", mode, joined)
		}
	}
}

func TestMergeRefusesUnknownMode(t *testing.T) {
	dir := t.TempDir()
	bin, argsFile := recorder(t, dir, "glab")

	warns := New("gitlab.example.com", bin).Merge(context.Background(), mergeRef, 7, forge.MergeRequest{Mode: forge.MergeMode("cherry-pick"), HeadSHA: headSHA})
	if len(warns) == 0 {
		t.Fatal("an unknown mode should report a warning")
	}
	if warns[0].Kind != "unsupported" {
		t.Errorf("Kind = %q, want unsupported", warns[0].Kind)
	}
	if _, err := os.Stat(argsFile); err == nil {
		t.Error("it should not have launched the CLI with an unknown mode")
	}
}
