package github

import (
	"context"
	"os"
	"strings"
	"testing"

	"prdash/internal/forge/model"
)

// `gh pr edit --base` is the documented way and it fails today before touching anything, on a
// deprecated Projects (classic) query that only errors in some repos.
func TestRetargetUsesTheAPINotPrEdit(t *testing.T) {
	dir := t.TempDir()
	bin, argsFile := recorder(t, dir, "gh")

	warns := New("github.com", bin).Retarget(context.Background(), mergeRef, 7, "release/2.0")
	if len(warns) != 0 {
		t.Fatalf("Retarget = %+v, want sin warnings", warns)
	}

	args := readArgs(t, argsFile)
	got := strings.Join(args, " ")
	if want := "-X PATCH repos/acme/widget/pulls/7"; !strings.Contains(got, want) {
		t.Errorf("argv = %q, want %q", got, want)
	}
	if want := "-f base=release/2.0"; !strings.Contains(got, want) {
		t.Errorf("argv = %q, want el campo base con la rama", got)
	}
	if strings.Contains(got, "pr edit") {
		t.Errorf("argv = %q, no debe usar `gh pr edit`: hoy falla con Projects (classic) being deprecated", got)
	}
}

// An argv with an empty field is not a harmless no-op: it is what leaves the PR with no target.
func TestRetargetRefusesEmptyBranch(t *testing.T) {
	for _, branch := range []string{"", "   "} {
		dir := t.TempDir()
		bin, argsFile := recorder(t, dir, "gh")

		warns := New("github.com", bin).Retarget(context.Background(), mergeRef, 7, branch)
		if !hasKind(warns, "unsupported") || !strings.Contains(firstMsgOf(warns), "nothing to retarget to") {
			t.Errorf("Retarget(%q) = %+v, want un motivo explícito", branch, warns)
		}
		if _, err := os.Stat(argsFile); err == nil {
			t.Errorf("Retarget(%q) llamó a la CLI: %q", branch, readArgs(t, argsFile))
		}
	}
}

func TestBranchesPaginatesAndProjectsElNombre(t *testing.T) {
	dir := t.TempDir()
	argsFile := dir + "/gh.args"
	bin := writeScript(t, dir, "gh",
		"#!/bin/sh\nprintf '%s\\n' \"$@\" > "+argsFile+"\nprintf 'main\\nrelease/2.0\\n'\n")

	names, warns := New("github.com", bin).Branches(context.Background(), mergeRef)
	if len(warns) != 0 {
		t.Fatalf("Branches = %+v, want sin warnings", warns)
	}
	if strings.Join(names, ",") != "main,release/2.0" {
		t.Errorf("Branches = %v, want las dos ramas en orden", names)
	}

	got := strings.Join(readArgs(t, argsFile), " ")
	for _, want := range []string{
		"repos/acme/widget/branches",
		"per_page=100", // sin esto, la API pagina de 30 en 30
		"--paginate",   // y sin esto, solo sale la primera página
		"--jq",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("argv = %q, want %q", got, want)
		}
	}
}

func TestBranchesAvisaSinProyecto(t *testing.T) {
	dir := t.TempDir()
	bin, _ := recorder(t, dir, "gh")

	names, warns := New("github.com", bin).Branches(context.Background(), model.RepoRef{})
	if len(names) != 0 {
		t.Errorf("Branches = %v, want lista vacía", names)
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
