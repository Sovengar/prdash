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

// testReport es lo que un helper de este paquete necesita de un test: marcar que es un helper,
// avisar de un fallo que detiene el test, y avisar de un fallo que no lo detiene.
//
// Y es una interfaz y no `*testing.T` por una razón concreta: `*testing.T` NO SE PUEDE DOBLEAR.
// `Fatalf` acaba en `runtime.Goexit()` y `testing.common` tiene campos privados, así que no hay
// forma deConstruir un `*testing.T` falso, y un doble hecho con `&testing.T{}` en crudo
// revienta. Y sin doble no hay forma de probar en proceso que un helper ABORTA ante un fixture
// roto: la única forma es tener un test que falle, y un test que se ve rojo no demuestra nada,
// se lee como un fallo del helper.
//
// Y con la interfaz, el camino del aviso se ejecuta en proceso con un doble que registra la
// llamada y VUELVE. Que vuelva es lo que hace que el test siga después del aviso, y que el test
// siga es justo lo que permite comprobar que solo se avisó una vez y con el prefijo.
//
// Y la interfaz solo se aplica donde hace falta —`aborta` y `RunConformance`, los dos sitios que
// reportan un fallo que el helper no puede resolver por su cuenta—. El resto de los helpers
// siguen con `*testing.T`: no tienen un camino de fallo propio que probar, y ensancharlos todos
// sería cambiar media API del paquete para no ganar nada.
type testReport interface {
	Helper()
	Fatalf(format string, args ...any)
	Error(args ...any)
}

// aborta es el ÚNICO sitio donde un helper de este paquete llama a `t.Fatal`.
//
// Y hay uno solo a propósito. Con seis, la pregunta "¿qué hace un helper cuando su fixture no
// sirve?" tiene seis respuestas distintas en el fichero, y con el tiempo cada una se decora un
// poco distinto —con prefijo, sin él, con un mensaje propio— y un test que ve dos mensajes
// distintos para el mismo fallo no sabe si son dos fallos o uno.
//
// Y el motivo de que exista como función y no sea un `t.Fatal` en cada sitio es que `t.Fatal`
// mata la goroutine: no hay forma de comprobar que se llama con un `*testing.T`, porque es un
// tipo concreto con un método privado. Con el reporte centrado en una función que toma
// `testReport`, la llamada se puede comprobar en proceso con un doble que registra y vuelve.
//
// Y lo que el doble NO comprueba, y por eso los tests por subproceso de
// `ayudas_fallo_test.go` siguen existiendo: que un `*testing.T` de verdad deja el test en rojo.
// Un doble puede contar las llamadas, y solo un `testing.T` real puede decir si el proceso de
// test terminó con fallo. Las dos mitades hacen falta y miden cosas distintas.
//
// Y el prefijo va aquí y no en cada llamada por el mismo motivo: un prefijo puesto seis veces se
// olvida una, y un test de fixture que ve `mkdir: permission denied` en vez de `testutil: mkdir:
// permission denied` no sabe si el fallo es del helper o del sistema.
func aborta(t testReport, err error) {
	t.Helper()
	if err == nil {
		return
	}
	t.Fatalf("testutil: %v", err)
}

// abortaCon ejecuta `fn` y aborta el test si devuelve error. Es la forma de que un helper
// delegadamente reporte el fallo sin escribir el `if` a mano, y sin que cada uno decida cómo
// prefijarlo.
func abortaCon(t testReport, fn func() error) {
	t.Helper()
	aborta(t, fn())
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
	out, err := runGit(dir, args...)
	aborta(t, err)
	return out
}

// runGit es `RunGit` sin la parte de `t.Fatal`: devuelve el error y deja que lo formatee quien
// lo llama.
//
// Y la separación es el mismo motivo por el que `checkDir` vive aparte, aplicado al resto de
// los helpers: **una guarda que aborta el test no se puede comprobar sin lanzar un
// subproceso**, porque `t.Fatal` mata la goroutine y `testing.TB` no sirve de doble —tiene un
// método privado—. Con el núcleo que devuelve error, el test afirma el motivo en el proceso; el
// envoltorio se sigue comprobando aparte, con subproceso, en `ayudas_fallo_test.go`.
//
// Y el error lleva los `args` y la salida de git porque sin ellos no se puede depurar: un
// "exit status 128" a secas no dice qué comando falló ni qué respondió.
func runGit(dir string, args ...string) (string, error) {
	if err := checkDir(dir); err != nil {
		return "", fmt.Errorf("testutil: %w", err)
	}
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = gitEnv()
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("git %v en %s: %w\n%s", args, dir, err, out)
	}
	return strings.TrimSpace(string(out)), nil
}

// requireDir ya no existe: `runGit` hace la comprobación de `dir` por el camino del error, así
// que los helpers que solo necesitaban esa guarda antes de un `git` la heredan gratis y sin una
// envoltura de `t.Fatal` que no se puede comprobar sin lanzar un subproceso.

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
	abortaCon(t, func() error { return creaRepoDir(dir) })
	RunGit(t, dir, "init", "-b", "main")
	RunGit(t, dir, "config", "user.email", "test@prdash.local")
	RunGit(t, dir, "config", "user.name", "prdash tests")
	RunGit(t, dir, "config", "commit.gpgsign", "false")
	desactivaAutoGC(t, dir)
}

// creaRepoDir prepara el directorio del repo. Vive aparte por el mismo motivo que
// `initBare`: el `t.Fatal` de un `MkdirAll` no se puede comprobar sin un subproceso.
func creaRepoDir(dir string) error {
	return os.MkdirAll(dir, 0o755)
}

// desactivaAutoGC quita el `git gc --auto` que git lanza en background tras un push o un
// fetch suficientes.
//
// Y el motivo es que ese proceso **sobrevive al comando que lo disparó**: sigue escribiendo
// en `.git/objects/pack` mientras el test ya ha terminado y `t.TempDir()` intenta borrar el
// árbol. El síntoma es un fallo de limpieza —
//
//	unlinkat /tmp/.../.git/objects/pack: directory not empty
//
// — que no dice nada del código que el test estaba probando y que solo aparece en la máquina
// que tendrá el repositorio lo bastante grande o el disco lo bastante lento. Medido: un test
// de `reporesolver` que hace dos push y dos fetch lo dispara una de cada varias.
//
// Y apagarlo no esconde nada: un test que depende del gc lo diría con un aserto sobre los
// packs, y no hay ninguno. Además, con gc apagado los repos de los tests no crean ficheros
// que el resto de la suite tiene que ignorar.
func desactivaAutoGC(t *testing.T, dir string) {
	t.Helper()
	RunGit(t, dir, "config", "gc.auto", "0")
}

// CommitFile escribe name con content en dir y lo commitea.
func CommitFile(t *testing.T, dir, name, content, msg string) {
	t.Helper()
	abortaCon(t, func() error { return commitFile(dir, name, content) })
	RunGit(t, dir, "add", "-A")
	RunGit(t, dir, "commit", "-m", msg)
}

// commitFile deja el fichero escrito y su directorio hecho, sin commitear.
//
// Y son dos guards y no uno porque fallan por motivos distintos: el padre puede no existir —
// un `name` con ruta— o puede haber algo donde debería ir un directorio —un fichero con el
// nombre del padre—. El segundo es el que se cuela en los fixtures, porque un
// `CommitFile(repo, "bloqueado/subido.txt", …)` con un `bloqueado` que es un fichero compila,
// se lee bien y solo falla al ejecutarse.
//
// Y vive aparte por el motivo de siempre: el `t.Fatal` no se puede comprobar sin subproceso.
func commitFile(dir, name, content string) error {
	path := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("preparar el directorio de %s: %w", name, err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return fmt.Errorf("escribir %s: %w", path, err)
	}
	return nil
}

// InitBare crea un repo bare (hace de origin/remoto local).
func InitBare(t *testing.T, dir string) {
	t.Helper()
	abortaCon(t, func() error { return initBare(dir) })
	RunGit(t, dir, "init", "--bare", "-b", "main")
	// El remoto es quien recibe los push y quien lanza el gc en background, así que también
	// necesita el auto-gc apagado. Ver `desactivaAutoGC`.
	desactivaAutoGC(t, dir)
}

// initBare comprueba y prepara el directorio de un repo bare.
//
// Y el `dir == ""` es la guarda que más ha costado: sin ella, `git init --bare` corre en el
// directorio del proceso de test —que vive DENTRO del repo— y le pone `core.bare=true` al repo
// real del que se está leyendo el código. Pasó de verdad. Vive aquí y no en el envoltorio
// para que el test pueda afirmar el motivo sin provocar un `t.Fatal`.
func initBare(dir string) error {
	if dir == "" {
		return errors.New("dir vacío: `git init --bare` caería en el repo real y le pondría core.bare")
	}
	return os.MkdirAll(dir, 0o755)
}

// SetRemote añade (o reemplaza) un remote en dir.
func SetRemote(t *testing.T, dir, name, url string) {
	t.Helper()
	quitaRemote(dir, name)
	_, err := runGit(dir, "remote", "add", name, url)
	aborta(t, err)
}

// quitaRemote quita un remote y IGNORA el error a propósito: un remote que no existía es el
// caso normal de la primera llamada, y abortar ahí obligaría a cada test a comprobar antes de
// poner. Vive aparte porque ignora su error por diseño y un `if err != nil { t.Fatal }` al
// revés de lo que hace el resto del fichero es peor que no tener función.
func quitaRemote(dir, name string) {
	rm := exec.Command("git", "remote", "remove", name)
	rm.Dir = dir
	rm.Env = gitEnv()
	_ = rm.Run() // no existía: no es un fallo
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
	aborta(t, checkDir(dir))
	cmd := exec.Command("git", "show-ref", "--verify", "--quiet", ref)
	cmd.Dir = dir
	cmd.Env = gitEnv()
	return cmd.Run() == nil
}
