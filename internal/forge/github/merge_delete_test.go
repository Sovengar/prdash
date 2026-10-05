package github

import (
	"context"
	"strings"
	"testing"

	"prdash/internal/forge"
)

// Sending it always would be worse than not sending it: in a repo with a merge queue gh rejects
// the whole command.
func TestMergeAsksForTheBranchDeletion(t *testing.T) {
	for _, tc := range []struct {
		name   string
		delete bool
		want   bool // should --delete-branch appear?
	}{
		{"requested", true, true},
		{"off", false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			bin, argsFile := recorder(t, dir, "gh")

			warns := New("github.com", bin).Merge(context.Background(), mergeRef, 7,
				forge.MergeRequest{Mode: forge.Squash, HeadSHA: headSHA, DeleteBranch: tc.delete})
			if len(warns) != 0 {
				t.Fatalf("Merge = %+v, want no warnings", warns)
			}
			joined := strings.Join(readArgs(t, argsFile), " ")
			if got := strings.Contains(joined, "--delete-branch"); got != tc.want {
				t.Errorf("argv = %q, want --delete-branch present = %v", joined, tc.want)
			}
		})
	}
}

func TestMergeWithDeleteStillPins(t *testing.T) {
	dir := t.TempDir()
	bin, argsFile := recorder(t, dir, "gh")

	New("github.com", bin).Merge(context.Background(), mergeRef, 7,
		forge.MergeRequest{Mode: forge.Rebase, HeadSHA: headSHA, DeleteBranch: true})

	joined := strings.Join(readArgs(t, argsFile), " ")
	if !strings.Contains(joined, "--match-head-commit "+headSHA) {
		t.Errorf("argv = %q, want the pin to %s", joined, headSHA)
	}
}
