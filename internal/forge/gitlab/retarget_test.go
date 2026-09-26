package gitlab

import (
	"context"
	"os"
	"strings"
	"testing"

	"prdash/internal/forge/model"
)

// TestRetargetUsesTheAPINotMrUpdate fija el argv del cambio de base y sobre todo
// que no sea `glab mr update --target-branch`: ese comando es de edición y su
// razón de ser es abrir título y descripción en el editor, así que con un flag de
// campo abierto esa puerta se entreabre. El PUT manda el campo y nada más.
func TestRetargetUsesTheAPINotMrUpdate(t *testing.T) {
	dir := t.TempDir()
	bin, argsFile := recorder(t, dir, "glab")

	warns := New("gitlab.example.com", bin).Retarget(context.Background(), mergeRef, 7, "release/2.0")
	if len(warns) != 0 {
		t.Fatalf("Retarget = %+v, want sin warnings", warns)
	}

	got := strings.Join(readArgs(t, argsFile), " ")
	// El proyecto anidado necesita el %2F: sin él la ruta se parte en dos y la
	// petición va a un sitio que no existe.
	if want := "-X PUT projects/grp%2Fproj/merge_requests/7"; !strings.Contains(got, want) {
		t.Errorf("argv = %q, want %q con el proyecto urlencoded", got, want)
	}
	if want := "-f target_branch=release/2.0"; !strings.Contains(got, want) {
		t.Errorf("argv = %q, want el campo target_branch con la rama", got)
	}
	if strings.Contains(got, "mr update") {
		t.Errorf("argv = %q, no debe usar `glab mr update`: es un comando de edición", got)
	}
	// El método tiene que ser explícito: con `-f`, glab cae a POST, y un POST
	// sobre la ruta de un MR no actualiza nada y contesta 200 como si lo hubiera
	// hecho.
	if !strings.Contains(got, "--hostname gitlab.example.com") {
		t.Errorf("argv = %q, want el host fijado", got)
	}
}

// TestRetargetRefusesEmptyBranch: sin rama no hay nada que enviar, y el corte se
// comprueba por ausencia de llamada, que es lo que de verdad importa.
func TestRetargetRefusesEmptyBranch(t *testing.T) {
	dir := t.TempDir()
	bin, argsFile := recorder(t, dir, "glab")

	warns := New("gitlab.example.com", bin).Retarget(context.Background(), mergeRef, 7, "  ")
	if len(warns) == 0 || !strings.Contains(warns[0].Msg, "nothing to retarget to") {
		t.Errorf("Retarget = %+v, want un motivo explícito", warns)
	}
	if _, err := os.Stat(argsFile); err == nil {
		t.Errorf("Retarget llamó a la CLI: %q", readArgs(t, argsFile))
	}
}

// TestBranchesPaginatesAsNDJSON: glab no tiene `--jq`, así que el NDJSON es la
// única forma de que cada página venga en una línea. Y tiene que traer el
// repositorio entero, porque el buscador ofrece destinos y un subconjunto
// dejaría fuera la rama que se buscaba sin avisar.
func TestBranchesPaginatesAsNDJSON(t *testing.T) {
	dir := t.TempDir()
	argsFile := dir + "/glab.args"
	body := "#!/bin/sh\nprintf '%s\\n' \"$@\" > " + argsFile + "\n" +
		"printf '{\"name\":\"main\"}\\n{\"name\":\"release/2.0\"}\\n'\n"
	bin := writeScript(t, dir, "glab", body)

	names, warns := New("gitlab.example.com", bin).Branches(context.Background(), mergeRef)
	if len(warns) != 0 {
		t.Fatalf("Branches = %+v, want sin warnings", warns)
	}
	if strings.Join(names, ",") != "main,release/2.0" {
		t.Errorf("Branches = %v, want las dos ramas en orden", names)
	}

	got := strings.Join(readArgs(t, argsFile), " ")
	for _, want := range []string{
		"projects/grp%2Fproj/repository/branches",
		"per_page=100",
		"--paginate",
		"--output ndjson", // y no `--jq`, que glab no tiene
	} {
		if !strings.Contains(got, want) {
			t.Errorf("argv = %q, want %q", got, want)
		}
	}
}

// TestBranchesAvisaSiLaSalidaNoSeEntiende: una respuesta ilegible no es un
// repositorio sin ramas. Devolver la lista vacía sin motivo haría que el buscador
// pareciera un repo vacío, que es un diagnóstico falso.
func TestBranchesAvisaSiLaSalidaNoSeEntiende(t *testing.T) {
	dir := t.TempDir()
	bin := writeScript(t, dir, "glab", "#!/bin/sh\nprintf 'no soy json\\n'\n")

	names, warns := New("gitlab.example.com", bin).Branches(context.Background(), mergeRef)
	if len(names) != 0 {
		t.Errorf("Branches = %v, want lista vacía", names)
	}
	if len(warns) == 0 || warns[0].Kind != "parse" {
		t.Errorf("Branches = %+v, want un warning de parseo", warns)
	}
}

func TestBranchesAvisaSinProyecto(t *testing.T) {
	dir := t.TempDir()
	bin, _ := recorder(t, dir, "glab")

	names, warns := New("gitlab.example.com", bin).Branches(context.Background(), model.RepoRef{})
	if len(names) != 0 {
		t.Errorf("Branches = %v, want lista vacía", names)
	}
	if len(warns) == 0 || warns[0].Kind != "notfound" {
		t.Errorf("Branches = %+v, want notfound", warns)
	}
}
