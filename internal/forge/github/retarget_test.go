package github

import (
	"context"
	"os"
	"strings"
	"testing"

	"prdash/internal/forge/model"
)

// TestRetargetUsesTheAPINotPrEdit fija el argv del cambio de base, y sobre todo lo
// que NO es: `gh pr edit --base` es la vía que documenta gh y hoy falla con un
// error de GraphQL por la deprecación de Projects Classic antes de tocar nada. Un
// test que solo afirmara "cambia la base" pasaría con las dos vías, así que este
// comprueba el camino bueno Y la ausencia del roto.
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

// TestRetargetRefusesEmptyBranch: sin rama no hay nada que enviar, y el argv con
// el campo vacío no es una operación inofensiva sino la que deja el PR sin base.
// El corte se comprueba por ausencia de llamada, no por el warning: lo que importa
// es que no se haya gastado nada.
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

// TestBranchesPaginatesAndProjectsElNombre: el listado tiene que traer el
// repositorio entero y no una página, porque el buscador que lo consume ofrece
// destinos y un subconjunto dejaría fuera la rama que se buscaba sin avisar.
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

// TestBranchesAvisaSinProyecto: sin proyecto no hay a qué preguntar, y el motivo
// tiene que ser el del notfound para que la TUI lo trate como conflicto y no como
// un repositorio sin ramas.
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
