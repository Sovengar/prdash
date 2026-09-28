package gitcmd

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestEnvQuitaLasGitDeLocalizacion fija lo que hace el filtro del entorno.
//
// Las variables GIT_* de localización le ganan a cmd.Dir: con GIT_DIR puesto,
// git opera en ese repo ignorando el directorio de trabajo. prdash elige el repo
// de cada ítem explícitamente, así que si una de ellas llega al subproceso una
// operación cae en un repo que no es el del ítem — y la que más duele es borrar
// la rama de otro proyecto. Aquí no se puede comprobar el efecto con git (eso
// necesita un repo de verdad, y el filtro es lo que lo impide), así que se
// comprueba la entrada del entorno, que es lo que el filtro controla.
func TestEnvQuitaLasGitDeLocalizacion(t *testing.T) {
	// Se“Weaponizan" las variables que tienen que desaparecer: las que fijan el
	// repo (GIT_DIR, GIT_WORK_TREE, GIT_INDEX_FILE, GIT_COMMON_DIR) y las que
	// reubican objetos o refs.
	for _, kv := range []string{
		"GIT_DIR=/tmp/otro/.git",
		"GIT_WORK_TREE=/tmp/otro",
		"GIT_INDEX_FILE=/tmp/otro/.git/index",
		"GIT_COMMON_DIR=/tmp/otro/.git",
		"GIT_OBJECT_DIRECTORY=/tmp/otro/.git/objects",
		"GIT_ALTERNATE_OBJECT_DIRECTORIES=/tmp/otro/.git/objects",
		"GIT_NAMESPACE=otro",
		"GIT_CEILING_DIRECTORIES=/tmp",
		"GIT_PREFIX=repo",
	} {
		t.Setenv(strings.SplitN(kv, "=", 2)[0], strings.SplitN(kv, "=", 2)[1])
	}

	env := Env()
	for _, kv := range env {
		for _, key := range []string{
			"GIT_DIR", "GIT_WORK_TREE", "GIT_INDEX_FILE", "GIT_COMMON_DIR",
			"GIT_OBJECT_DIRECTORY", "GIT_ALTERNATE_OBJECT_DIRECTORIES",
			"GIT_NAMESPACE", "GIT_CEILING_DIRECTORIES", "GIT_PREFIX",
		} {
			if strings.HasPrefix(kv, key+"=") {
				t.Errorf("Env() = %q, want sin %s: fija el repo y le gana a cmd.Dir", kv, key)
			}
		}
	}

	// Y lo que sí tiene que estar: el modo no interactivo y el locale inglés.
	for _, want := range []string{"GIT_TERMINAL_PROMPT=0", "GIT_PAGER=cat", "NO_COLOR=1", "LC_ALL=C"} {
		if !contains(env, want) {
			t.Errorf("Env() no trae %q", want)
		}
	}
}

// TestEnvQuitaElLocale: el idioma de los mensajes de git es parte del contrato,
// porque el clasificador de errores de los adapters busca texto en inglés.
func TestEnvQuitaElLocale(t *testing.T) {
	t.Setenv("LANG", "es_ES.UTF-8")
	t.Setenv("LC_ALL", "es_ES.UTF-8")
	t.Setenv("LANGUAGE", "es_ES")
	t.Setenv("LC_MESSAGES", "es_ES")

	env := Env()
	for _, kv := range env {
		if strings.HasPrefix(kv, "LANG=") || strings.HasPrefix(kv, "LANGUAGE=") || strings.HasPrefix(kv, "LC_MESSAGES=") {
			t.Errorf("Env() = %q, want el locale del usuario fuera", kv)
		}
	}
	if !contains(env, "LC_ALL=C") {
		t.Error("Env() debería fijar LC_ALL=C")
	}
}

// TestEnvConservaLoQueNoToca: un filtro que se pasa de celoso rompería el acceso
// a credenciales y a la configuración del usuario, que es justo lo que la TUI
// necesita para hablar con el forge. Solo se quitan lo que estorba.
func TestEnvConservaLoQueNoToca(t *testing.T) {
	t.Setenv("PRTEST_MARKER", "se-que-debe-conservar")
	if !contains(Env(), "PRTEST_MARKER=se-que-debe-conservar") {
		t.Error("Env() tiró una variable que no debía tocar")
	}
}

// TestGitEnRepoRealIgnoraElGITDirHeredado es la prueba de efecto, con git de
// verdad. Se monta un repo "víctima" fuera del fixture y se exporta su GIT_DIR
// como si quien lanzara la suite viniera de dentro de él. Run tiene que operar
// en el repo del fixture, no en la víctima: si el filtro de Env() no estuviera,
// `git config` de abajo escribiría en la config de la víctima.
func TestGitEnRepoRealIgnoraElGITDirHeredado(t *testing.T) {
	victima := filepath.Join(t.TempDir(), "victima.git")
	if err := os.MkdirAll(victima, 0o755); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command("git", "init", "--bare", "-b", "main", victima).CombinedOutput(); err != nil {
		t.Fatalf("init --bare: %v\n%s", err, out)
	}
	marker := filepath.Join(victima, "config")
	if err := os.WriteFile(marker, nil, 0o644); err != nil {
		t.Fatal(err)
	}

	// El repo del fixture, en otro árbol y sin relación con la víctima.
	repo := filepath.Join(t.TempDir(), "repo")
	if err := os.MkdirAll(repo, 0o755); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command("git", "init", "-b", "main", repo).CombinedOutput(); err != nil {
		t.Fatalf("init: %v\n%s", err, out)
	}

	t.Setenv("GIT_DIR", filepath.Join(victima, ".git"))
	r := New()
	if _, err := r.Run(t.Context(), repo, "config", "prdash.test", "escrito-en-el-fixture"); err != nil {
		t.Fatalf("Run: %v", err)
	}

	// Lo escrito tiene que estar en el repo del fixture... Las comprobaciones
	// van con un entorno limpio a propósito: si heredaran el GIT_DIR de este
	// test, serían ellas las que miraran en la víctima y pasarían por buena
	// la configuración que hay que comprobar.
	if out, err := gitLimpio(t, "-C", repo, "config", "--get", "prdash.test").CombinedOutput(); err != nil {
		t.Errorf("la config no quedó en el repo del fixture: %v\n%s", err, out)
	} else if got := strings.TrimSpace(string(out)); got != "escrito-en-el-fixture" {
		t.Errorf("valor = %q, want escrito-en-el-fixture", got)
	}

	// ...y NO en la víctima, que es de donde no debía salir nada.
	if out, _ := gitLimpio(t, "--git-dir="+victima, "config", "--get", "prdash.test").CombinedOutput(); strings.TrimSpace(string(out)) != "" {
		t.Errorf("la config de la víctima se modificó: %q — GIT_DIR le ganó a cmd.Dir", out)
	}
}

// gitLimpio ejecuta git sin las GIT_* de localización del entorno del test, que
// es justo lo que la prueba está comprobando que Run() ya no propaga.
func gitLimpio(t *testing.T, args ...string) *exec.Cmd {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Env = Env()
	return cmd
}

func contains(env []string, want string) bool {
	for _, kv := range env {
		if kv == want {
			return true
		}
	}
	return false
}
