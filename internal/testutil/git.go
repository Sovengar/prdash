package testutil

import (
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
func gitEnv() []string {
	return append(os.Environ(),
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
func RunGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	if dir != "" {
		cmd.Dir = dir
	}
	cmd.Env = gitEnv()
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v en %s: %v\n%s", args, dir, err, out)
	}
	return strings.TrimSpace(string(out))
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
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	RunGit(t, dir, "init", "--bare", "-b", "main")
}

// SetRemote añade (o reemplaza) un remote en dir.
func SetRemote(t *testing.T, dir, name, url string) {
	t.Helper()
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
	cmd := exec.Command("git", "show-ref", "--verify", "--quiet", ref)
	cmd.Dir = dir
	cmd.Env = gitEnv()
	return cmd.Run() == nil
}
