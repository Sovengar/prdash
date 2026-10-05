package gitlab

import (
	"context"
	"os"
	"strings"
	"testing"

	"prdash/internal/forge/model"
)

// `glab mr update` is not broken, it is an EDIT command whose purpose is opening title and
// description in an editor.
func TestRetargetUsesTheAPINotMrUpdate(t *testing.T) {
	dir := t.TempDir()
	bin, argsFile := recorder(t, dir, "glab")

	warns := New("gitlab.example.com", bin).Retarget(context.Background(), mergeRef, 7, "release/2.0")
	if len(warns) != 0 {
		t.Fatalf("Retarget = %+v, want no warnings", warns)
	}

	got := strings.Join(readArgs(t, argsFile), " ")
	// A nested project needs the %2F: without it the path splits in two and the request goes to the
	// wrong place.
	if want := "-X PUT projects/grp%2Fproj/merge_requests/7"; !strings.Contains(got, want) {
		t.Errorf("argv = %q, want %q with the project urlencoded", got, want)
	}
	if want := "-f target_branch=release/2.0"; !strings.Contains(got, want) {
		t.Errorf("argv = %q, want the target_branch field with the branch", got)
	}
	if strings.Contains(got, "mr update") {
		t.Errorf("argv = %q, must not use `glab mr update`: it is an editing command", got)
	}
	// The method has to be explicit: with -f glab falls back to POST, and a POST on the MR update
	// route does not work.
	if !strings.Contains(got, "--hostname gitlab.example.com") {
		t.Errorf("argv = %q, want the host pinned", got)
	}
}

func TestRetargetRefusesEmptyBranch(t *testing.T) {
	dir := t.TempDir()
	bin, argsFile := recorder(t, dir, "glab")

	warns := New("gitlab.example.com", bin).Retarget(context.Background(), mergeRef, 7, "  ")
	if len(warns) == 0 || !strings.Contains(warns[0].Msg, "nothing to retarget to") {
		t.Errorf("Retarget = %+v, want an explicit reason", warns)
	}
	if _, err := os.Stat(argsFile); err == nil {
		t.Errorf("Retarget called the CLI: %q", readArgs(t, argsFile))
	}
}

// glab has no --jq, so NDJSON is the only way each page arrives on its own line.
func TestBranchesPaginatesAsNDJSON(t *testing.T) {
	dir := t.TempDir()
	argsFile := dir + "/glab.args"
	body := "#!/bin/sh\nprintf '%s\\n' \"$@\" > " + argsFile + "\n" +
		"printf '{\"name\":\"main\"}\\n{\"name\":\"release/2.0\"}\\n'\n"
	bin := writeScript(t, dir, "glab", body)

	names, warns := New("gitlab.example.com", bin).Branches(context.Background(), mergeRef)
	if len(warns) != 0 {
		t.Fatalf("Branches = %+v, want no warnings", warns)
	}
	if strings.Join(names, ",") != "main,release/2.0" {
		t.Errorf("Branches = %v, want the two branches in order", names)
	}

	got := strings.Join(readArgs(t, argsFile), " ")
	for _, want := range []string{
		"projects/grp%2Fproj/repository/branches",
		"per_page=100",
		"--paginate",
		"--output ndjson", // and not `--jq`, which glab does not have
	} {
		if !strings.Contains(got, want) {
			t.Errorf("argv = %q, want %q", got, want)
		}
	}
}

func TestBranchesWarnsWhenTheOutputIsUnreadable(t *testing.T) {
	dir := t.TempDir()
	bin := writeScript(t, dir, "glab", "#!/bin/sh\nprintf 'I am not json\\n'\n")

	names, warns := New("gitlab.example.com", bin).Branches(context.Background(), mergeRef)
	if len(names) != 0 {
		t.Errorf("Branches = %v, want an empty list", names)
	}
	if len(warns) == 0 || warns[0].Kind != "parse" {
		t.Errorf("Branches = %+v, want a parse warning", warns)
	}
}

func TestBranchesWarnsWithoutProject(t *testing.T) {
	dir := t.TempDir()
	bin, _ := recorder(t, dir, "glab")

	names, warns := New("gitlab.example.com", bin).Branches(context.Background(), model.RepoRef{})
	if len(names) != 0 {
		t.Errorf("Branches = %v, want an empty list", names)
	}
	if len(warns) == 0 || warns[0].Kind != "notfound" {
		t.Errorf("Branches = %+v, want notfound", warns)
	}
}
