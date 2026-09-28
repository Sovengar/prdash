package testutil

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestGitHelpersNoTocanElRepoReal es la red de seguridad de la suite entera.
//
// Los fixtures de git corren git de verdad. Si uno de ellos acaba con el
// directorio equivocado, el comando se ejecuta en el cwd del proceso de test —el
// del paquete, que está DENTRO del repo— y un `git config` o un `git remote`
// escribe en la config del repo real. No es un fallo de un test: es el repo de
// quien está leyendo el código el que sale con core.bare=true y un origin
// apuntando a un TempDir, y eso no aparece en ningún `go test` en rojo.
//
// Los guards de dir vacío y de directorio existente cortan esa vía. Este test
// comprueba que existen y que se disparan, y de paso que un repo de verdad se
// monta bien en un temp: si el guard se endureciera tanto que rompiera el
// fixture, la suite entera se caería aquí antes que en un test de negocio.
func TestGitHelpersNoTocanElRepoReal(t *testing.T) {
	// Un repo de verdad se monta y se consulta en su directorio.
	repo := filepath.Join(t.TempDir(), "repo")
	InitRepo(t, repo)
	CommitFile(t, repo, "base.txt", "base", "base")
	if got := RunGit(t, repo, "rev-parse", "--abbrev-ref", "HEAD"); got != "main" {
		t.Errorf("HEAD = %q, want main", got)
	}
	if !RefExists(t, repo, "refs/heads/main") {
		t.Error("el fixture debería dejar main en refs/heads/main")
	}

	// Y el remoto se puede poner sin tocar nada de fuera.
	origin := filepath.Join(t.TempDir(), "origin.git")
	InitBare(t, origin)
	SetRemote(t, repo, "origin", origin)
	if got := RunGit(t, repo, "remote", "get-url", "origin"); got != origin {
		t.Errorf("origin = %q, want %q", got, origin)
	}
}

// TestGitEnvNoDejaQueElGITDirHeredadoRedirijaLasEscrituras es la prueba de
// efecto del filtro, con git de verdad.
//
// Se monta una "víctima" fuera del fixture y se exporta su GIT_DIR como si
// quien lanzara la suite viniera de dentro de ella (un hook, un `git -C`, un
// `git commit` de otro repo). Las GIT_* de localización le ganan a cmd.Dir, así
// que sin el filtro un `git config` de InitRepo escribiría en la config de la
// víctima: el repo real de quien está leyendo el código, saliéndose con
// core.bare=true y un origin a un TempDir. Eso es exactamente lo que pasó.
//
// La comprobación va con git en un entorno limpio (cleanGitEnv) a propósito: si
// las aserciones heredaran el GIT_DIR de este test, serían ellas las que miraran
// en la víctima y pasarían por buena la configuración que hay que mirar.
func TestGitEnvNoDejaQueElGITDirHeredadoRedirijaLasEscrituras(t *testing.T) {
	// La víctima es un repo de TRABAJO, no uno bare: es el caso que de verdad
	// rompe. `git config` y `git remote` escriben en el GIT_DIR que hereden
	// apuntando al del cwd, y un repo bare ya inicializado se traga un `git init`
	// re-inicializándose en silencio, así que una víctima bare dejaría pasar al
	// bug. Con un repo de trabajo cada escritura se ve en la config.
	victima := filepath.Join(t.TempDir(), "victima")
	if out, err := exec.Command("git", "init", "-b", "main", victima).CombinedOutput(); err != nil {
		t.Fatalf("init: %v\n%s", err, out)
	}
	cfg := filepath.Join(victima, ".git", "config")
	antes, err := os.ReadFile(cfg)
	if err != nil {
		t.Fatal(err)
	}

	t.Setenv("GIT_DIR", filepath.Join(victima, ".git"))
	t.Setenv("GIT_WORK_TREE", victima)

	repo := filepath.Join(t.TempDir(), "repo")
	InitRepo(t, repo)
	CommitFile(t, repo, "base.txt", "base", "base")

	// Lo escrito tiene que estar en el repo del fixture.
	if out, err := cleanGitEnv(t, "-C", repo, "config", "--get", "user.email").CombinedOutput(); err != nil {
		t.Errorf("user.email no quedó en el repo del fixture: %v\n%s", err, out)
	} else if got := strings.TrimSpace(string(out)); got != "test@prdash.local" {
		t.Errorf("user.email = %q, want test@prdash.local", got)
	}
	if out, _ := cleanGitEnv(t, "-C", repo, "rev-parse", "--is-bare-repository").CombinedOutput(); strings.TrimSpace(string(out)) != "false" {
		t.Errorf("el fixture no debería ser bare: %q", out)
	}

	// Y la config de la víctima tiene que seguir byte a byte como estaba. Se
	// compara el fichero entero en vez de preguntar por claves sueltas: la
	// víctima es un repo bare creado aquí, así que ya tiene core.bare=true y
	// preguntar por esa clave daría un falso positivo. Lo que importa no es qué
	// claves existen, es que ninguna escritura del fixture haya llegado.
	despues, err := os.ReadFile(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if string(antes) != string(despues) {
		t.Errorf("la config de la víctima cambió:\n--- antes ---\n%s\n--- después ---\n%s\nGIT_DIR le ganó a cmd.Dir", antes, despues)
	}
}

// cleanGitEnv ejecuta git sin las GIT_* de localización del entorno del test,
// que es justo lo que la prueba comprueba que gitEnv() ya no propaga. Sin esto,
// las aserciones mirarían en la víctima por el mismo bug que se estáProbando.
func cleanGitEnv(t *testing.T, args ...string) *exec.Cmd {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Env = gitEnv()
	return cmd
}

// TestRequireDirRechazaLoQueNoEsUnDirectorio: la guarda tiene que cortar tanto el
// dir vacío (que haría que git corriera en el repo real) como una ruta que no
// existe (que dejaría que git cayera al cwd igual). Sin esto, un typo en un
// nombre de fixture pasa inadvertido y el git subsiguiente muta el repo real en
// lugar de abortar el test.
func TestRequireDirRechazaLoQueNoEsUnDirectorio(t *testing.T) {
	if err := checkDir(""); err == nil {
		t.Error("un dir vacío debería rechazarse: git correría en el repo real")
	}
	if err := checkDir(filepath.Join(t.TempDir(), "no-existe")); err == nil {
		t.Error("una ruta inexistente debería rechazarse")
	}

	// Un fichero no es un directorio, aunque exista.
	f := filepath.Join(t.TempDir(), "fichero.txt")
	if err := os.WriteFile(f, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := checkDir(f); err == nil {
		t.Error("un fichero plano no debería valer como directorio de git")
	}

	// Y un directorio de verdad pasa.
	if err := checkDir(t.TempDir()); err != nil {
		t.Errorf("un directorio real debería valer: %v", err)
	}
}
