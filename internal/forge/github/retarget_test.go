package github

import (
	"context"
	"os"
	"strings"
	"testing"

	"prdash/internal/forge/model"
)

// `gh pr edit --base` fails today on a deprecated Projects (classic) query that only errors in some repos, so the API is used.
func TestRetargetUsesTheAPINotPrEdit(t *testing.T) {
	dir := t.TempDir()
	bin, argsFile := recorder(t, dir, "gh")

	warns := New("github.com", bin).Retarget(context.Background(), mergeRef, 7, "release/2.0")
	if len(warns) != 0 {
		t.Fatalf("Retarget = %+v, want no warnings", warns)
	}

	args := readArgs(t, argsFile)
	got := strings.Join(args, " ")
	if want := "-X PATCH repos/acme/widget/pulls/7"; !strings.Contains(got, want) {
		t.Errorf("argv = %q, want %q", got, want)
	}
	if want := "-f base=release/2.0"; !strings.Contains(got, want) {
		t.Errorf("argv = %q, want the base field with the branch", got)
	}
	if strings.Contains(got, "pr edit") {
		t.Errorf("argv = %q, must not use `gh pr edit`: today it fails with Projects (classic) being deprecated", got)
	}
}

// An argv with an empty field is not a harmless no-op: it is what leaves the PR with no target.
func TestRetargetRefusesEmptyBranch(t *testing.T) {
	for _, branch := range []string{"", "   "} {
		dir := t.TempDir()
		bin, argsFile := recorder(t, dir, "gh")

		warns := New("github.com", bin).Retarget(context.Background(), mergeRef, 7, branch)
		if !hasKind(warns, "unsupported") || !strings.Contains(firstMsgOf(warns), "nothing to retarget to") {
			t.Errorf("Retarget(%q) = %+v, want an explicit reason", branch, warns)
		}
		if _, err := os.Stat(argsFile); err == nil {
			t.Errorf("Retarget(%q) called the CLI: %q", branch, readArgs(t, argsFile))
		}
	}
}

func TestBranchesPaginatesAndProjectsTheName(t *testing.T) {
	dir := t.TempDir()
	argsFile := dir + "/gh.args"
	bin := writeScript(t, dir, "gh",
		"#!/bin/sh\nprintf '%s\\n' \"$@\" > "+argsFile+"\nprintf 'main\\nrelease/2.0\\n'\n")

	names, warns := New("github.com", bin).Branches(context.Background(), mergeRef)
	if len(warns) != 0 {
		t.Fatalf("Branches = %+v, want no warnings", warns)
	}
	if strings.Join(names, ",") != "main,release/2.0" {
		t.Errorf("Branches = %v, want the two branches in order", names)
	}

	got := strings.Join(readArgs(t, argsFile), " ")
	for _, want := range []string{
		"repos/acme/widget/branches",
		"per_page=100", // without this the API pages 30 at a time
		"--paginate",   // and without this only the first page comes out
		"--jq",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("argv = %q, want %q", got, want)
		}
	}
}

func TestBranchesWarnsWithoutProject(t *testing.T) {
	dir := t.TempDir()
	bin, _ := recorder(t, dir, "gh")

	names, warns := New("github.com", bin).Branches(context.Background(), model.RepoRef{})
	if len(names) != 0 {
		t.Errorf("Branches = %v, want an empty list", names)
	}
	if !hasKind(warns, "notfound") {
		t.Errorf("Branches = %+v, want notfound", warns)
	}
}

func hasKind(warns []model.Warning, kind string) bool {
	for _, w := range warns {
		if w.Kind == kind {
			return true
		}
	}
	return false
}

func firstMsgOf(warns []model.Warning) string {
	if len(warns) == 0 {
		return ""
	}
	return warns[0].Msg
}
