package github

import (
	"context"
	"os"
	"strings"
	"testing"

	"prdash/internal/forge"
)

// Without `--match-head-commit`, gh merges whatever HEAD is at that moment, and between the inbox
// refresh and the keypress the branch can have advanced.
func TestMergePinsTheHeadCommit(t *testing.T) {
	for _, mode := range []forge.MergeMode{forge.MergeCommit, forge.Rebase, forge.Squash} {
		dir := t.TempDir()
		bin, argsFile := recorder(t, dir, "gh")
		const sha = "abc1234"

		warns := New("github.com", bin).Merge(context.Background(), mergeRef, 7, forge.MergeRequest{Mode: mode, HeadSHA: sha})
		if len(warns) != 0 {
			t.Fatalf("mode %q: Merge = %+v, want no warnings", mode, warns)
		}
		joined := strings.Join(readArgs(t, argsFile), " ")
		if !strings.Contains(joined, "--match-head-commit "+sha) {
			t.Errorf("mode %q: argv = %q, want the pin to %s", mode, joined, sha)
		}
	}
}

func TestMergeRefusesToPinNothing(t *testing.T) {
	for _, sha := range []string{"", "   "} {
		dir := t.TempDir()
		bin, argsFile := recorder(t, dir, "gh")

		warns := New("github.com", bin).Merge(context.Background(), mergeRef, 7, forge.MergeRequest{Mode: forge.Rebase, HeadSHA: sha})
		if len(warns) == 0 {
			t.Fatalf("sha %q: a merge without a pin should report a warning", sha)
		}
		if warns[0].Kind != "unsupported" {
			t.Errorf("sha %q: Kind = %q, want unsupported", sha, warns[0].Kind)
		}
		if _, err := os.Stat(argsFile); err == nil {
			t.Errorf("sha %q: it should not have launched the CLI without being able to pin", sha)
		}
	}
}

func TestMergeRefusesAnUnknownModeBeforePinning(t *testing.T) {
	dir := t.TempDir()
	bin, argsFile := recorder(t, dir, "gh")

	warns := New("github.com", bin).Merge(context.Background(), mergeRef, 7, forge.MergeRequest{Mode: forge.MergeMode("cherry-pick"), HeadSHA: "abc1234"})
	if len(warns) == 0 || warns[0].Kind != "unsupported" {
		t.Fatalf("warnings = %+v, want unsupported", warns)
	}
	if _, err := os.Stat(argsFile); err == nil {
		t.Error("it should not have launched the CLI with an unknown mode")
	}
}

// Both data have to be in the query or the rest is useless.
func TestPRFieldsAskForThePinAndTheRules(t *testing.T) {
	for _, field := range []string{
		"headRefOid",
		"mergeCommitAllowed",
		"rebaseMergeAllowed",
		"squashMergeAllowed",
	} {
		if !strings.Contains(ghPRFields, field) {
			t.Errorf("ghPRFields does not ask for %q", field)
		}
	}
}
