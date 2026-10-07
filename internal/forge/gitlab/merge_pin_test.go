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
			t.Fatalf("mode %q: Merge = %+v, want no warnings", mode, warns)
		}
		joined := strings.Join(readArgs(t, argsFile), " ")
		if !strings.Contains(joined, "--sha "+sha) {
			t.Errorf("mode %q: argv = %q, want the pin to %s", mode, joined, sha)
		}
	}
}

func TestMergeRefusesToPinNothing(t *testing.T) {
	for _, sha := range []string{"", "  "} {
		dir := t.TempDir()
		bin, argsFile := recorder(t, dir, "glab")

		warns := New("gitlab.example.com", bin).Merge(context.Background(), mergeRef, 7, forge.MergeRequest{Mode: forge.Squash, HeadSHA: sha})
		if len(warns) == 0 {
			t.Fatalf("sha %q: a merge without a pin should report a warning", sha)
		}
		if _, err := os.Stat(argsFile); err == nil {
			t.Errorf("sha %q: it should not have launched the CLI without being able to pin", sha)
		}
	}
}

// diffHeadSha has to be in the query; Project.mergeMethod does not exist in the schema.
func TestMRFieldsAskForThePin(t *testing.T) {
	if !strings.Contains(mrFields, "diffHeadSha") {
		t.Error("mrFields does not ask for diffHeadSha, so the pin could never be satisfied")
	}
	// The negative assertion documents a decision: if the schema ever grows mergeMethod, the test says so.
	if strings.Contains(mrFields, "mergeMethod") {
		t.Log("mrFields asks for mergeMethod: the instance supports it, rules filtering is possible")
	}
}
