package testutil

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// gitEnv es el entorno con el que corren TODOS los comandos git de la suite.
//
// Los tests no heredan la config de git de la máquina que los ejecuta. Esa config
// es global y mutable, y los repos que montan los tests son repos de verdad: en
// cuanto una parte de ella alcanza a un clon, el resultado depende de quién lance
// la suite. Concretamente:
//
//   - identidad: un clon NO hereda user.name/user.email del repo de origen
//     (viven en su config local), así que un `commit` dentro de un clon se queda
//     sin autor. Con config global el test pasa; en un runner sin ella falla con
//     "Author identity unknown". Es lo que tumbó el primer CI de prdash.
//   - commit.gpgsign, core.hooksPath, core.autocrlf, init.defaultBranch, ...
//     envenenados en la config global llegan al clon y rompen commits, instalan
//     hooks o cambian que se compara en un diff.
//
// La identidad se da por variables de entorno (GIT_AUTHOR_*/GIT_COMMITTER_*) en
// vez de `git config` en cada repo: así vale para cualquier repo que cree el
// test, incluidos los clones, y no obliga a acordarse en cada helper nuevo.
// GIT_CONFIG_GLOBAL/SYSTEM a /dev/null dejan a git sin config que leer fuera de
// la local de cada repo, que es la que pone InitRepo. Requiere git >= 2.32; en
// uno más viejo esas variables se ignoran y los tests siguen valiendo, porque la
// identidad ya va en el env.
// Las variables GIT_* de localización sobreescriben a cmd.Dir por completo: con
// GIT_DIR puesto, `git config` escribe en ese repo y da igual en qué directorio
// se ejecute el comando. Si quien lanza la suite está dentro de un hook, de un
// `git -C`, o de cualquier contexto que exporte GIT_DIR/GIT_WORK_TREE, los
// fixtures escriben la config del repo ajeno en vez de la de su TempDir. Se
// filtran aquí, que es el mismo filtro que hace gitcmd.Env() en producción.
//
// Un repo real no necesita GIT_DIR para funcionar: cmd.Dir basta, y es lo que
// estos fixtures usan.
func gitEnv() []string {
	env := os.Environ()
	out := env[:0]
	for _, kv := range env {
		switch {
		case strings.HasPrefix(kv, "GIT_DIR="),
			strings.HasPrefix(kv, "GIT_WORK_TREE="),
			strings.HasPrefix(kv, "GIT_INDEX_FILE="),
			strings.HasPrefix(kv, "GIT_COMMON_DIR="),
			strings.HasPrefix(kv, "GIT_OBJECT_DIRECTORY="),
			strings.HasPrefix(kv, "GIT_ALTERNATE_OBJECT_DIRECTORIES="),
			strings.HasPrefix(kv, "GIT_NAMESPACE="),
			strings.HasPrefix(kv, "GIT_CEILING_DIRECTORIES="),
			strings.HasPrefix(kv, "GIT_PREFIX="):
			continue
		}
		out = append(out, kv)
	}
	return append(out,
		"GIT_TERMINAL_PROMPT=0",
		"LC_ALL=C",
		"GIT_CONFIG_GLOBAL=/dev/null",
		"GIT_CONFIG_SYSTEM=/dev/null",
		"GIT_AUTHOR_NAME=prdash tests",
		"GIT_AUTHOR_EMAIL=test@prdash.local",
		"GIT_COMMITTER_NAME=prdash tests",
		"GIT_COMMITTER_EMAIL=test@prdash.local",
	)
}

// RunGit ejecuta git en dir y devuelve stdout recortado, fallando el test ante
// error. Es la base de los fixtures de repos reales (sin red).
//
// dir es obligatorio y tiene que existir. Un dir vacío haría que git corriera
// en el directorio de trabajo del proceso de test —el del paquete, que vive
// DENTRO del repo— y un `git config` o un `git remote` de los fixtures escribiría
// entonces en la config del repo real del que se está leyendo el código. Ya
// pasó: una suite Green'se dejó el repo en core.bare=true con un origin
// apuntando a un TempDir. El error se ve al instante, que es lo que se busca.
func RunGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	requireDir(t, dir)
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = gitEnv()
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v en %s: %v\n%s", args, dir, err, out)
	}
	return strings.TrimSpace(string(out))
}

// requireDir aborta el test si dir no sirve como directorio de trabajo de git.
func requireDir(t *testing.T, dir string) {
	t.Helper()
	if err := checkDir(dir); err != nil {
		t.Fatalf("testutil: %v", err)
	}
}

// checkDir explica por qué dir no sirve como directorio de trabajo de git. Vive
// separada de requireDir para que el test pueda afirmar el motivo sin tener que
// provocar un t.Fatal.
func checkDir(dir string) error {
	if dir == "" {
		return errors.New("dir vacío: git correría en el repo real y escribiría en su config")
	}
	fi, err := os.Stat(dir)
	if err != nil {
		return fmt.Errorf("%q no existe: %w", dir, err)
	}
	if !fi.IsDir() {
		return fmt.Errorf("%q no es un directorio", dir)
	}
	return nil
}

// InitRepo crea un repo git normal en dir con identidad local.
func InitRepo(t *testing.T, dir string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	RunGit(t, dir, "init", "-b", "main")
	RunGit(t, dir, "config", "user.email", "test@prdash.local")
	RunGit(t, dir, "config", "user.name", "prdash tests")
	RunGit(t, dir, "config", "commit.gpgsign", "false")
}

// CommitFile escribe name con content en dir y lo commitea.
func CommitFile(t *testing.T, dir, name, content, msg string) {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	RunGit(t, dir, "add", "-A")
	RunGit(t, dir, "commit", "-m", msg)
}

// InitBare crea un repo bare (hace de origin/remoto local).
func InitBare(t *testing.T, dir string) {
	t.Helper()
	if dir == "" {
		t.Fatal("testutil: dir vacío: `git init --bare` caería en el repo real y le pondría core.bare")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	RunGit(t, dir, "init", "--bare", "-b", "main")
}

// SetRemote añade (o reemplaza) un remote en dir.
func SetRemote(t *testing.T, dir, name, url string) {
	t.Helper()
	requireDir(t, dir)
	rm := exec.Command("git", "remote", "remove", name)
	rm.Dir = dir
	rm.Env = gitEnv()
	_ = rm.Run() // no existía: no es un fallo
	RunGit(t, dir, "remote", "add", name, url)
}

// Push ejecuta `git push` con los args dados en dir.
func Push(t *testing.T, dir string, args ...string) {
	t.Helper()
	all := append([]string{"push"}, args...)
	RunGit(t, dir, all...)
}

// RefExists informa si un ref existe en dir.
func RefExists(t *testing.T, dir, ref string) bool {
	t.Helper()
	requireDir(t, dir)
	cmd := exec.Command("git", "show-ref", "--verify", "--quiet", ref)
	cmd.Dir = dir
	cmd.Env = gitEnv()
	return cmd.Run() == nil
}
