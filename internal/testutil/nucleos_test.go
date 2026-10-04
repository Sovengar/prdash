package testutil

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// Este fichero es la mitad en proceso de las guardas del paquete, y la otra mitad está en
// `ayudas_fallo_test.go` con subprocesos. Las dos existen por la misma razón y se necesitan
// las dos:
//
//   - Aquí se comprueba que cada núcleo devuelve el MOTIVO: qué error da y qué dice. Es lo que
//     permite entender un fallo cuando ocurre.
//   - Allí se comprueba que cada envoltorio ABORTA de verdad, que es la mitad que importa para
//     un test que usa el helper: un helper que devuelve en vez de abortar deja el test
//     siguiente asserts sobre un repo vacío.
//
// Y la razón por la que hacen falta dos es `t.Fatal`: mata la goroutine y `testing.TB` no
// sirve de doble porque tiene un método privado. Un núcleo que devuelve error sí se puede
// comprobar en el proceso, y por eso los helpers se dividieron en núcleo y envoltorio.

// TestLosNucleosDevuelvenElMotivo: las guardas, sin subproceso.
//
// Y cada caso comprueba algo distinto del mensaje, porque los motivos sirven para cosas
// distintas: el `dir` que no vale dice dónde se calleron (y con qué operación) para no perder
// tiempo; el fallo de git dice qué comando se rompió y qué respondió; `initBare` explica qué
// rompería en el repo real, porque ese es el daño.
func TestLosNucleosDevuelvenElMotivo(t *testing.T) {
	dir := t.TempDir()

	// `runGit` con un dir que no sirve. El motivo viene de `checkDir` y el prefijo del
	// paquete lo añade `runGit`: sin él, un error de git y un error de directorio sería
	// indistinguibles en un log de CI que solo tiene una línea.
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

	// Y `runGit` con un comando que falla. El error lleva los args Y la salida de git, que es
	// lo único que permite depurar sin reproducir a mano, y conserva el `*exec.ExitError`
	// con `%w` para que un test pueda clasificarlo sin parsear texto.
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

	// Y el camino bueno, porque un núcleo que devuelve error tiene siempre camino bueno, y
	// probarlo evita que "siempre falla" cuente como prueba de nada.
	out, err := runGit(repo, "status", "--porcelain")
	if err != nil {
		t.Fatalf("runGit del camino bueno: %v", err)
	}
	if out != "" {
		t.Errorf("un repo recién creado dio status %q, want vacío", out)
	}
}

// TestInitBareNiegaElDirectorioVacioConElMotivoDelDaño: la guarda que más ha costado.
//
// Y no se trata solo de que aborte, sino de que el mensaje diga QUÉ haría: `git init --bare`
// en el directorio del proceso de test, que vive DENTRO del repo del proyecto, le pondría
// `core.bare=true` al repo del que se está leyendo el código. Y no lo restauraría nadie.
//
// Y el mensaje lo dice en futuro del daño concreto —"le pondría core.bare"— y no en
// condicional genérico, porque el que lo lee está decidiendo si el fallo de su test es este o
// es otro.
func TestInitBareNiegaElDirectorioVacioConElMotivoDelDaño(t *testing.T) {
	err := initBare("")
	if err == nil {
		t.Fatal("initBare con dir vacío dio nil: `git init --bare` correría en el repo real")
	}
	if !strings.Contains(err.Error(), "core.bare") {
		t.Errorf("el error %q no dice qué rompería en el repo real", err)
	}

	// Y con un padre que es un fichero: se puede crear el directorio, no se puede el repo. El
	// modo 000 no valdría aquí porque en un contenedor los tests pueden correr como root y
	// root escribe en cualquier sitio; bloquear por FORMA funciona siempre y en cualquiera.
	bloqueo := filepath.Join(t.TempDir(), "bloqueo")
	if err := os.WriteFile(bloqueo, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := initBare(filepath.Join(bloqueo, "repo.git")); err == nil {
		t.Error("initBare con un padre que es fichero dio nil")
	}

	// Y el camino bueno, anidado incluido: `MkdirAll` tiene que crear toda la cadena.
	anidado := filepath.Join(t.TempDir(), "a", "b", "c", "repo.git")
	if err := initBare(anidado); err != nil {
		t.Errorf("initBare anidado: %v", err)
	}
	if _, err := os.Stat(anidado); err != nil {
		t.Errorf("initBare no creó el directorio: %v", err)
	}
}

// TestCommitFileDistingueLosDosFallosDeEscritura: los dos guards, que fallan por distinto motivo.
//
// Y la distinción importa al leer el fallo. Un `name` con ruta y un padre que es un fichero es
// un problema de PREPARACIÓN —el path está mal—; un destino que ya es un directorio es un
// problema de CONTENIDO —el nombre está mal—.
//
// Y los dos son fallos de FIXTURE, no de producción, así que el mensaje tiene que nombrar el
// fichero: un `permission denied` sin pathname en una suite con veinte fixtures que commitean
// es una búsqueda a ciegas.
func TestCommitFileDistingueLosDosFallosDeEscritura(t *testing.T) {
	dir := t.TempDir()
	repo := filepath.Join(dir, "repo")
	InitRepo(t, repo)

	// El padre del fichero existe como fichero. Y el repo tiene que existir de verdad: con
	// solo el directorio, `MkdirAll` del padre del "bloqueo" tenía éxito porque la cadena
	// `repo/bloqueado` no existía, y el guard que se quería probar —el padre que es un
	// fichero— no se daba. Los dos fallos venían del mismo fixture incompleto.
	bloqueo := filepath.Join(repo, "bloqueo")
	if err := os.WriteFile(bloqueo, []byte("soy un fichero"), 0o644); err != nil {
		t.Fatal(err)
	}
	err := commitFile(repo, "bloqueo/subido.txt", "x")
	if err == nil {
		t.Fatal("commitFile con un padre que es fichero dio nil")
	}
	// Y el mensaje NOMBRA el fichero que no se pudo escribir.
	if !strings.Contains(err.Error(), "bloqueo/subido.txt") {
		t.Errorf("el error %q no dice qué fichero no se pudo escribir", err)
	}

	// El destino existe como directorio.
	soloDir := filepath.Join(repo, "soy-un-dir")
	if err := os.MkdirAll(soloDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := commitFile(repo, "soy-un-dir", "x"); err == nil {
		t.Error("commitFile con un destino que es directorio dio nil")
	}

	// Y el camino bueno, con las dos cosas: crea el padre anidado y escribe el contenido
	// exacto. Un núcleo que devolviera nil sin escribir pasaría el test de arriba.
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
