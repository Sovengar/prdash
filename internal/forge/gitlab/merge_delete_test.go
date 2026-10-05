package gitlab

import (
	"context"
	"strings"
	"testing"

	"prdash/internal/forge"
)

// The flag is `--remove-source-branch`, nothing like gh's `--delete-branch`.
func TestMergeAsksForTheBranchDeletion(t *testing.T) {
	for _, tc := range []struct {
		name   string
		delete bool
		want   bool
	}{
		{"requested", true, true},
		{"off", false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			bin, argsFile := recorder(t, dir, "glab")

			warns := New("gitlab.example.com", bin).Merge(context.Background(), mergeRef, 7,
				forge.MergeRequest{Mode: forge.Squash, HeadSHA: headSHA, DeleteBranch: tc.delete})
			if len(warns) != 0 {
				t.Fatalf("Merge = %+v, want no warnings", warns)
			}
			joined := strings.Join(readArgs(t, argsFile), " ")
			if got := strings.Contains(joined, "--remove-source-branch"); got != tc.want {
				t.Errorf("argv = %q, want --remove-source-branch present = %v", joined, tc.want)
			}
			if strings.Contains(joined, "--delete-branch") {
				t.Errorf("argv = %q: --delete-branch belongs to gh, glab does not understand it", joined)
			}
		})
	}
}

func TestMergeWithDeleteStillPins(t *testing.T) {
	dir := t.TempDir()
	bin, argsFile := recorder(t, dir, "glab")

	New("gitlab.example.com", bin).Merge(context.Background(), mergeRef, 7,
		forge.MergeRequest{Mode: forge.Rebase, HeadSHA: headSHA, DeleteBranch: true})

	joined := strings.Join(readArgs(t, argsFile), " ")
	if !strings.Contains(joined, "--sha "+headSHA) {
		t.Errorf("argv = %q, want the pin to %s", joined, headSHA)
	}
	if !strings.Contains(joined, "--yes") {
		t.Errorf("argv = %q, want --yes: without it glab opens a prompt", joined)
	}
}
