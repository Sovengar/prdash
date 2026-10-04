package testutil

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

const casoDeFalloEnv = "PRDASH_TESTUTIL_CASO_DE_FALLO"

// The seven t.Fatal paths.
func TestLosHelpersAbortanEnVozAltaEnVezDeDevolverCosas(t *testing.T) {
	if caso := os.Getenv(casoDeFalloEnv); caso != "" {
		ejecutaCasoDeFallo(t, caso)
		// If the helper did NOT abort we get here, and `t.Failed()` is what has to be checked rather
		//than taking the case as good.
		if !t.Failed() {
			t.Fatalf("el caso %q NO abortó: el helper devolvió en vez de fallar", caso)
		}
		return
	}

	for _, c := range casosDeFallo() {
		t.Run(c.nombre, func(t *testing.T) {
			salida, code := correCasoEnSubproceso(t, c.nombre)

			if code == 0 {
				t.Fatalf("el helper %s NO abortó. Un fixture roto que no aborta hace que "+
					"los tests siguientes comprueben un repo vacío —o peor, pasen por "+
					"casualidad probando nada.\n%s", c.nombre, salida)
			}
			// And it says WHY: an abort with no reason is a "test failed" with no clue.
			if !strings.Contains(salida, c.quiere) {
				t.Errorf("abortó pero sin decir %q:\n%s", c.quiere, salida)
			}
		})
	}
}

type casoDeFallo struct {
	nombre string
	quiere string
}

func casosDeFallo() []casoDeFallo {
	return []casoDeFallo{
		{"git-falla", "exit status"},
		{"dir-inexistente", "no existe"},
		{"dir-vacio", "dir vacío"},
		{"dir-no-directorio", "no es un directorio"},
		// ENOTDIR and not EACCES: the obstruction is a FILE where the helper wants a directory.
		{"commit-padre-bloqueado", "not a directory"},
		{"commit-destino-directorio", "is a directory"},
		{"bare-vacio", "repo real"},
		{"bare-padre-bloqueado", "not a directory"},
		{"conformance-roto", "debería reportar unsupported"},
	}
}

func ejecutaCasoDeFallo(t *testing.T, caso string) {
	t.Helper()
	switch caso {
	case "git-falla":
		repo := repoDePrueba(t)
		RunGit(t, repo, "checkout", "no-existe-esta-rama")

	case "dir-inexistente":
		// The guard that has already passed for real: without it git would run in the process's own
		// directory.
		RunGit(t, filepath.Join(t.TempDir(), "no-existe"), "status")

	case "dir-vacio":
		RunGit(t, "", "status")

	case "dir-no-directorio":
		fichero := filepath.Join(t.TempDir(), "soy-un-fichero")
		if err := os.WriteFile(fichero, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
		RunGit(t, fichero, "status")

	case "commit-padre-bloqueado":
		repo := repoDePrueba(t)
		bloqueo := filepath.Join(repo, "bloqueo")
		if err := os.WriteFile(bloqueo, []byte("soy un fichero"), 0o644); err != nil {
			t.Fatal(err)
		}
		CommitFile(t, repo, "bloqueo/subido.txt", "x", "no se puede")

	case "commit-destino-directorio":
		repo := repoDePrueba(t)
		if err := os.MkdirAll(filepath.Join(repo, "soy-un-dir"), 0o755); err != nil {
			t.Fatal(err)
		}
		CommitFile(t, repo, "soy-un-dir", "x", "destino que es un directorio")

	case "bare-vacio":
		InitBare(t, "")

	case "bare-padre-bloqueado":
		bloqueo := filepath.Join(t.TempDir(), "bloqueo")
		if err := os.WriteFile(bloqueo, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
		InitBare(t, filepath.Join(bloqueo, "repo.git"))

	case "conformance-roto":
		// An adapter that does not support an operation has to SAY SO, not return an empty list as if
		//nothing were there.
		RunConformance(t, &adapterQueFalla{roto: "no-avisar"},
			ConformanceOptions{Unsupported: true})
	}
}

func repoDePrueba(t *testing.T) string {
	t.Helper()
	repo := filepath.Join(t.TempDir(), "repo")
	InitRepo(t, repo)
	CommitFile(t, repo, "a.txt", "a", "a")
	return repo
}

func correCasoEnSubproceso(t *testing.T, caso string) (string, int) {
	t.Helper()
	cmd := exec.Command(os.Args[0],
		"-test.run=^TestLosHelpersAbortanEnVozAltaEnVezDeDevolverCosas$",
		"-test.v")
	cmd.Env = append(os.Environ(), casoDeFalloEnv+"="+caso)
	out, err := cmd.CombinedOutput()

	codigo := 0
	var salidaErr *exec.ExitError
	if err != nil {
		if e, ok := err.(*exec.ExitError); ok {
			salidaErr = e
			codigo = e.ExitCode()
		} else {
			t.Fatalf("no se pudo ejecutar el subproceso: %v", err)
		}
	}
	_ = salidaErr
	// The failure has to be the abort's, not a panic from the child; a panic hangs the test.
	if strings.Contains(string(out), "panic:") {
		t.Errorf("el subproceso del caso %s entró en panic en vez de abortar el test:\n%s",
			caso, out)
	}
	return string(out), codigo
}
