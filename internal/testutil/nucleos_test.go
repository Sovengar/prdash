package testutil

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// The in-process half of the package's guards; the other half is the subprocess tests.

// Each case checks something different.
func TestLosNucleosDevuelvenElMotivo(t *testing.T) {
	dir := t.TempDir()

	// The reason comes from checkDir and the path prefix.
	fichero := filepath.Join(dir, "fichero")
	if err := os.WriteFile(fichero, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		nombre string
		dir    string
		quiere string
	}{
		{"dir inexistente", filepath.Join(dir, "no-existe"), "no existe"},
		{"dir vacío", "", "dir vacío"},
		{"dir que no es directorio", fichero, "no es un directorio"},
	} {
		_, err := runGit(c.dir, "status")
		if err == nil {
			t.Errorf("%s: runGit dio nil", c.nombre)
			continue
		}
		if !strings.Contains(err.Error(), c.quiere) {
			t.Errorf("%s: el error %q no dice %q", c.nombre, err, c.quiere)
		}
		if !strings.Contains(err.Error(), "testutil:") {
			t.Errorf("%s: el error no lleva el prefijo del paquete: %q", c.nombre, err)
		}
	}

	repo := filepath.Join(dir, "repo")
	InitRepo(t, repo)
	_, err := runGit(repo, "checkout", "rama-que-no-existe")
	if err == nil {
		t.Fatal("un checkout a una rama inexistente dio nil")
	}
	for _, quiere := range []string{"checkout", "rama-que-no-existe", repo} {
		if !strings.Contains(err.Error(), quiere) {
			t.Errorf("el error %q no dice %q", err, quiere)
		}
	}
	var salida *exec.ExitError
	if !errors.As(err, &salida) {
		t.Errorf("el error no envuelve el *exec.ExitError, así que no se puede clasificar "+
			"sin parsear texto: %v", err)
	}

	out, err := runGit(repo, "status", "--porcelain")
	if err != nil {
		t.Fatalf("runGit del camino bueno: %v", err)
	}
	if out != "" {
		t.Errorf("un repo recién creado dio status %q, want vacío", out)
	}
}

// The guard that cost the most.
func TestInitBareNiegaElDirectorioVacioConElMotivoDelDaño(t *testing.T) {
	err := initBare("")
	if err == nil {
		t.Fatal("initBare con dir vacío dio nil: `git init --bare` correría en el repo real")
	}
	if !strings.Contains(err.Error(), "core.bare") {
		t.Errorf("el error %q no dice qué rompería en el repo real", err)
	}

	bloqueo := filepath.Join(t.TempDir(), "bloqueo")
	if err := os.WriteFile(bloqueo, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := initBare(filepath.Join(bloqueo, "repo.git")); err == nil {
		t.Error("initBare con un padre que es fichero dio nil")
	}

	anidado := filepath.Join(t.TempDir(), "a", "b", "c", "repo.git")
	if err := initBare(anidado); err != nil {
		t.Errorf("initBare anidado: %v", err)
	}
	if _, err := os.Stat(anidado); err != nil {
		t.Errorf("initBare no creó el directorio: %v", err)
	}
}

// Two guards that fail for different reasons.
func TestCommitFileDistingueLosDosFallosDeEscritura(t *testing.T) {
	dir := t.TempDir()
	repo := filepath.Join(dir, "repo")
	InitRepo(t, repo)

	// The file's parent exists as a file, and the repo has to be real: with only the directory,
	//git refuses.
	bloqueo := filepath.Join(repo, "bloqueo")
	if err := os.WriteFile(bloqueo, []byte("soy un fichero"), 0o644); err != nil {
		t.Fatal(err)
	}
	err := commitFile(repo, "bloqueo/subido.txt", "x")
	if err == nil {
		t.Fatal("commitFile con un padre que es fichero dio nil")
	}
	// The message NAMES the file that could not be written.
	if !strings.Contains(err.Error(), "bloqueo/subido.txt") {
		t.Errorf("el error %q no dice qué fichero no se pudo escribir", err)
	}

	soloDir := filepath.Join(repo, "soy-un-dir")
	if err := os.MkdirAll(soloDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := commitFile(repo, "soy-un-dir", "x"); err == nil {
		t.Error("commitFile con un destino que es directorio dio nil")
	}

	if err := commitFile(repo, "anidado/subido.txt", "contenido"); err != nil {
		t.Fatalf("commitFile del camino bueno: %v", err)
	}
	raw, err := os.ReadFile(filepath.Join(repo, "anidado", "subido.txt"))
	if err != nil {
		t.Fatalf("leer lo escrito: %v", err)
	}
	if string(raw) != "contenido" {
		t.Errorf("el fichero tiene %q, want contenido", raw)
	}
}
