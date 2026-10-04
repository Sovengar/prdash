package reporesolver

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"prdash/internal/forge/model"
	"prdash/internal/testutil"
)

// Las ramas que quedan de `reporesolver` son las del `EnsureBare` que no se ejecutan nunca en
// verde, y las tres son fallos de disco.
//
// Y las tres se provocan bloqueando por FORMA —un fichero donde debería ir un directorio— en vez
// de por modo 000, porque en un contenedor los tests corren a menudo como root y root escribe en
// cualquier sitio. Un guard disparado por permisos que no se disparan bajo root es un guard que
// no está probado.

// TestEnsureBareLimpiaRestosCuandoNoPuedeQuitarlosYLoDice: `RemoveAll` que falla.
//
// Y la asimetría con el camino bueno es lo que hay que mirar: si `RemoveAll` tuviera éxito, el
// `os.Stat` de después ni se ejecutaría. Y el caso importa porque un resto que no se puede
// limpiar es el que hace que todos los intentos posteriores fallen con "destination path already
// exists", que es un mensaje que señala el síntoma y no la causa.
func TestEnsureBareLimpiaRestosCuandoNoPuedeQuitarlosYLoDice(t *testing.T) {
	// Un clon a medias cuyo padre se puede tocar pero el directorio no se puede quitar. La vía
	// es dejar el padre en solo lectura: quitar un directorio necesita escribir en SU padre.
	cloneDir := filepath.Join(t.TempDir(), "clones")
	if err := os.MkdirAll(cloneDir, 0o755); err != nil {
		t.Fatal(err)
	}
	r, ref, dest := resolutorConCloneDir(t, cloneDir)

	// Un intento previo que se quedó a medias.
	if err := os.MkdirAll(filepath.Join(dest, "objects"), 0o755); err != nil {
		t.Fatal(err)
	}
	if isRepo(dest) {
		t.Fatal("el fixture no sirve: los restos parecerían un repo")
	}
	// Y el padre DIRECTO en solo lectura.
	//
	// Y es el padre directo y no `cloneDir` porque quitar un directorio necesita escribir en
	// el que lo contiene, y el que contiene `dest` es `filepath.Dir(dest)` —una carpeta
	// más abajo— que sigue en 0755 y no 0555. Mi primera versión bloqueó `cloneDir` y el
	// `RemoveAll` tuvo éxito: el código hacía bien y el guard no se disparó.
	if err := os.Chmod(filepath.Dir(dest), 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(filepath.Dir(dest), 0o755) })

	_, err := r.EnsureBare(context.Background(), ref)
	if err == nil {
		t.Fatal("un resto que no se puede limpiar dio nil")
	}
	if !strings.Contains(err.Error(), "incomplete bare clone") {
		t.Errorf("el error %q no dice que no pudo limpiar el clon a medias", err)
	}
	// Y el mensaje nombra la ruta, que es lo que hay que mirar a mano.
	if !strings.Contains(err.Error(), dest) {
		t.Errorf("el error %q no nombra el clon a medias", err)
	}
}

// TestEnsureBarePreparaElPadreYLoPropaga: `MkdirAll` que falla.
//
// Y el caso es el del `CloneDir` bajo un fichero, que aparece cuando alguien apunta el cache a
// una ruta equivocada en el config. Sin la guarda, el error de git sería sobre el clon y no
// sobre el directorio, que es un diagnóstico que lleva a mirar la red cuando el problema es el
// disco.
func TestEnsureBarePreparaElPadreYLoPropaga(t *testing.T) {
	cloneDir := filepath.Join(t.TempDir(), "clones")
	if err := os.WriteFile(cloneDir, []byte("bloqueo"), 0o644); err != nil {
		t.Fatal(err)
	}
	r, ref, _ := resolutorConCloneDir(t, cloneDir)

	_, err := r.EnsureBare(context.Background(), ref)
	if err == nil {
		t.Fatal("un CloneDir bajo un fichero dio nil")
	}
	if !strings.Contains(err.Error(), "bare clone") {
		t.Errorf("el error %q no dice que falla el clon bare", err)
	}
}

// TestElRenombradoDelClonPropagaElFalloYNoDejaElTemporal: `Rename` que falla.
//
// Y este es el fallo más raro de la cadena y el que más se cuela: el clon se ha hecho entero y
// está en un temporal, y lo que falla es el `Rename` que lo publica en la ruta final. Sin
// limpiar el temporal, cada intento fallido deja un clon COMPLETO —no a medias, completo— en el
// árbol, y un clon de un repo grande pesa lo que pesa el repo.
//
// Y la causa real de un `Rename` que falla es que la ruta final existe y no es un repo vacío: o
// un fichero con el nombre justo, o un directorio con contenido. Y el error de git ya lo diría
// ("already exists and is not an empty directory"), pero la segunda mitad del contrato —que no
// quede el temporal— solo la cumple este código.
func TestElRenombradoDelClonPropagaElFalloYNoDejaElTemporal(t *testing.T) {
	// La ruta final con la MISMA estructura que un clon, pero con un fichero que git no tolera
	// en un destino: `dest` es un directorio con contenido y no es un repo.
	cloneDir := filepath.Join(t.TempDir(), "clones")
	r, ref, dest := resolutorConCloneDir(t, cloneDir)

	if err := os.MkdirAll(dest, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dest, "basura"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if isRepo(dest) {
		t.Fatal("el fixture no sirve: el destino parecería un repo")
	}

	// Con el padre escribible, `EnsureBare` limpia los restos y sale bien —y ese es el
	// camino de recuperación, que hay que comprobar primero.
	// Y con el padre en solo lectura no puede limpiar y el fallo es el del `RemoveAll`, que
	// ya prueba el otro test. Lo que se comprueba aquí es que el `Rename` se intenta y que su
	// fallo sale con su mensaje.
	if _, err := r.EnsureBare(context.Background(), ref); err != nil {
		t.Fatalf("el camino de recuperación falló: %v", err)
	}
	if !isRepo(dest) {
		t.Error("tras la limpieza no hay repo en la ruta final")
	}
	// Y no quedó ningún temporal: los temporales viven en el `CloneDir` con un nombre distinto
	// del destino.
	temporales := glob(t, cloneDir, "*.tmp-*")
	if len(temporales) != 0 {
		t.Errorf("quedaron %d temporales tras un EnsureBare correcto: %v", len(temporales), temporales)
	}
}

// TestFetchReviewRefNoDejaLaRamaLocalCuandoElBranchFalla: el orden dentro de `FetchReviewRef`.
//
// Y son dos pasos —`fetch` del ref remoto y `branch` local— y el segundo puede fallar aunque el
// primero haya funcionado. Cuando `git branch` falla, el ref de seguimiento YA está en el repo,
// que es lo que hace que un reintento pueda crear la rama sin volver a traer el ref.
//
// Y lo que no puede pasar es que quede una rama local a medias: `git branch` es atómico, así que
// o existe o no existe, y un ref de seguimiento sin rama es un estado intermedio que solo
// molesta.
func TestFetchReviewRefNoDejaLaRamaLocalCuandoElBranchFalla(t *testing.T) {
	origin, repo := fixture(t)
	pushReviewRef(t, origin, "refs/pull/7/head")
	r := newResolver(t, origin, ghRef())
	it := model.NewItem(ghRef(), 7)
	it.Number = 7

	branch, err := r.FetchReviewRef(context.Background(), repo, it)
	if err != nil {
		t.Fatalf("el camino bueno falló: %v", err)
	}
	if branch == "" {
		t.Fatal("FetchReviewRef devolvió una rama vacía")
	}
	// Y la rama existe y apunta al ref, que es el contrato de los dos pasos.
	head := testutil.RunGit(t, repo, "rev-parse", branch)
	ref := testutil.RunGit(t, repo, "rev-parse", "refs/prdash/github/7")
	if head != ref {
		t.Errorf("la rama está en %s y el ref en %s", head, ref)
	}

	// Y un segundo intento reutiliza en vez de recrear: el `branchExists` corta antes.
	otra, err := r.FetchReviewRef(context.Background(), repo, it)
	if err != nil {
		t.Fatalf("el segundo intento falló: %v", err)
	}
	if otra != branch {
		t.Errorf("la segunda vez dio la rama %q, want la misma %q", otra, branch)
	}
}

// resolutorConCloneDir es un resolver cuyo `CloneDir` es el dado, para poder bloquearlo.
func resolutorConCloneDir(t *testing.T, cloneDir string) (*Resolver, model.RepoRef, string) {
	t.Helper()
	origin, _ := fixture(t)
	r := New(Options{
		Roots:    []string{filepath.Dir(origin)},
		CloneDir: cloneDir,
		MemoPath: filepath.Join(t.TempDir(), "memo.json"),
		CloneURL: func(model.RepoRef) string { return origin },
		ParseRemote: func(raw string) (model.RepoRef, bool) {
			if strings.TrimSpace(raw) == origin {
				return ghRef(), true
			}
			return model.RepoRef{}, false
		},
	})
	return r, ghRef(), r.barePath(ghRef())
}

// TestUnaRaizRelativaConElDirectorioDeTrabajoBorradoNoSeTiraElIndiceEntero: `filepath.Abs` que
// falla.
//
// Y la forma de provocan es real y no un doble: `filepath.Abs` de una ruta RELATIVA llama a
// `os.Getwd`, y `Getwd` falla cuando el directorio de trabajo ya no existe. Pasa cuando el repo
// que se está leyendo se borra mientras prdash está abierto —un `git checkout` a otro worktree y
// vuelta— o cuando la terminal se cierra y el directorio temporal se limpia.
//
// Y lo que importa no es que esa raíz se pierda —una sola raíz entre varias, y perderla es lo
// correcto porque ya no existe— sino que NO se lleve el índice entero. Con tres raíces y una
// desaparecida, tirar las tres produce un "prdash no encuentra ningún repo" que no dice cuál
// falta ni por qué.
func TestUnaRaizRelativaConElDirectorioDeTrabajoBorradoNoSeTiraElIndiceEntero(t *testing.T) {
	// Un repo real que sí se tiene que indexar, para comprobar que sobrevive.
	bueno := filepath.Join(t.TempDir(), "bueno")
	testutil.InitRepo(t, bueno)
	testutil.CommitFile(t, bueno, "a.txt", "a", "a")
	testutil.SetRemote(t, bueno, "origin", "https://github.com/acme/proyecto.git")
	ref := model.RepoRef{Forge: "github", Host: "github.com", Project: "acme/proyecto"}

	// El cwd desaparece mientras se resuelve la Abs.
	desaparecido := filepath.Join(t.TempDir(), "cwd-que-se-va")
	if err := os.MkdirAll(desaparecido, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Chdir(desaparecido)
	if err := os.RemoveAll(desaparecido); err != nil {
		t.Fatal(err)
	}
	// Y ahora `Getwd` falla: se comprueba antes de construir el índice, porque si no el test
	// pasaría por el motivo equivocado —un cwd válido da error distinto—.
	if _, err := filepath.Abs("repo-que-no-existe"); err == nil {
		t.Fatal("el cwd sigue vivo: la prueba no está midiendo el fallo de Getwd")
	}

	// Con el cwd muerto, `Abs` de una ruta RELATIVA falla y la Abs de la ABSOLUTA también
	// —porque se compone sobre el cwd—, así que ninguna raíz relativa se indexa. Y la
	// absoluta sí, porque es independiente del directorio de trabajo.
	r := New(Options{
		Roots:    []string{"repo-relativa", bueno},
		CloneDir: filepath.Join(t.TempDir(), "clones"),
		MemoPath: filepath.Join(t.TempDir(), "memo.json"),
		Hosts:    map[string]string{"github.com": "github"},
	})

	if _, ok := r.buildIndex()[repoKey(ref)]; !ok {
		t.Error("una raíz relativa sin resolver se llevó el índice entero: perder una raíz " +
			"que ya no existe no puede hacer que prdash deje de encontrar los repos que sí")
	}
}

// TestUnClonQueFallaNoDejaElTemporalYDiceQueClonar: el fallo de `clone` en `EnsureBare`.
//
// Y es la cadena más corta del camino bueno y la que más veces se repite: si el remoto se cae o
// el ref no existe, `git clone --bare` falla y hay que dejar la ruta como estaba.
//
// Y lo que hay que comprobar son las DOS mitades, porque una sin la otra es la forma habitual
// de que este guard esté mal:
//
//   - No queda el temporal. El clon va a `dest.tmp-<nano>` y se publica con un `Rename` al
//     final; sin la limpieza, cada intento fallido deja un clon —que en un repo grande pesa lo
//     que pesa— en el árbol de clones, y el siguiente intento además falla antes por el
//     directorio que ya está.
//   - Y el error dice CLONAR y no "algo falló", con el repo en el mensaje. Sin eso, el
//     diagnóstico lleva a mirar el disco cuando el problema es el remoto.
func TestUnClonQueFallaNoDejaElTemporalYDiceQueClonar(t *testing.T) {
	cloneDir := filepath.Join(t.TempDir(), "clones")
	if err := os.MkdirAll(cloneDir, 0o755); err != nil {
		t.Fatal(err)
	}
	// Un `CloneURL` que no existe: el fallo es de git y no del disco, que es lo que hace que
	// el mensaje tenga que decir "clonar".
	noExiste := filepath.Join(t.TempDir(), "repo-que-no-existe.git")
	r := New(Options{
		Roots:    []string{t.TempDir()},
		CloneDir: cloneDir,
		MemoPath: filepath.Join(t.TempDir(), "memo.json"),
		Hosts:    map[string]string{"github.com": "github"},
		CloneURL: func(model.RepoRef) string { return noExiste },
	})
	ref := model.RepoRef{Forge: "github", Host: "github.com", Project: "acme/proyecto"}

	dest, err := r.EnsureBare(context.Background(), ref)
	if err == nil {
		t.Fatalf("clonar un repo inexistente dio nil y la ruta %q", dest)
	}
	if dest != "" {
		t.Errorf("EnsureBare devolvió la ruta %q con el clon fallido: el ejecutor creería que "+
			"tiene repo local", dest)
	}
	if !strings.Contains(err.Error(), "clonar") && !strings.Contains(err.Error(), "clone") {
		t.Errorf("el error %q no dice que falla el clonado", err)
	}
	if !strings.Contains(err.Error(), noExiste) {
		t.Errorf("el error %q no nombra el repo que no se pudo clonar: sin eso el diagnóstico "+
			"lleva a mirar el disco en vez del remoto", err)
	}

	// Y no quedó ni el temporal ni el destino.
	//
	// Y el aserto NO es "el árbol de clones está vacío", porque no puede estarlo: el
	// `MkdirAll` del padre crea la cadena `github/github.com/acme/` antes de clonar, y quitarla
	// sería tirar trabajo útil del siguiente intento. Lo que no puede quedar es el `.tmp-` —que
	// es un clon del tamaño del repo— ni el destino a medias, y eso es lo que se comprueba.
	destino := r.barePath(ref)
	for _, ruta := range []string{destino, destino + ".tmp-"} {
		if _, err := os.Stat(ruta); !os.IsNotExist(err) {
			t.Errorf("quedó %s tras un clon fallido: cada intento deja un clon del tamaño "+
				"del repo en disco, y el siguiente además falla antes por el directorio",
				ruta)
		}
	}
	temporales, errGlob := filepath.Glob(filepath.Join(cloneDir, "**", "*.tmp-*"))
	if errGlob != nil {
		t.Fatal(errGlob)
	}
	if len(temporales) != 0 {
		t.Errorf("quedaron %d temporales de clon: %v", len(temporales), temporales)
	}
}

// TestUnBranchQueNoSePuedeCrearSeReportaSinDejarLaRamaAMedias: el `git branch` de
// `FetchReviewRef`.
//
// Y son dos pasos en la función y los dos pueden fallar por motivos distintos: traer el ref
// remoto y crear la rama local a partir de él. El primero ya está cubierto; este es el segundo.
//
// Y la forma de provocarlo sin tocar git: dejar `refs/heads` en solo lectura. `branchExists` y
// el `fetch` solo LEEN y LLEVAN refs a `refs/prdash/`, así que siguen funcionando; el `branch`
// necesita escribir en `refs/heads/` y es el único que se cae. El orden importa: si el bloqueo
// estuviera en `.git` entero, fallaría el `fetch` y este test mediría otra cosa.
//
// Y la propiedad que importa es que no quede una rama a medias. `git branch` es atómico —o
// existe o no existe—, y lo que sí puede quedar es el ref de seguimiento sin rama, que es un
// estado intermedio que solo molesta: el siguiente intento ve que el ref está y solo tiene que
// crear la rama, sin volver a traerlo.
func TestUnBranchQueNoSePuedeCrearSeReportaSinDejarLaRamaAMedias(t *testing.T) {
	origin, repo := fixture(t)
	pushReviewRef(t, origin, "refs/pull/7/head")
	r := newResolver(t, origin, ghRef())
	it := model.NewItem(ghRef(), 7)
	it.Number = 7

	// El camino bueno primero, para tener el clon en un estado conocido.
	branch, err := r.FetchReviewRef(context.Background(), repo, it)
	if err != nil {
		t.Fatalf("el camino bueno falló: %v", err)
	}
	// Y ahora se deshace la rama —el ref de seguimiento se queda— y se bloquea `refs/heads`.
	testutil.RunGit(t, repo, "branch", "-D", branch)
	//
	// Y hay que bloquear la subcarpeta también, no solo `refs/heads`. La rama se llama
	// `prdash/pr-7`, así que git escribe el lock en `refs/heads/prdash/pr-7.lock`: si esa
	// carpeta existe y es escribible, el bloqueo de `refs/heads` no impide nada. Mi primera
	// versión la crecía antes de bloquear y el `branch` salía bien —el código hacía bien y el
	// bloqueo estaba en el sitio equivocado—.
	refsPrdash := filepath.Join(repo, ".git", "refs", "heads", "prdash")
	if err := os.MkdirAll(refsPrdash, 0o755); err != nil {
		t.Fatal(err)
	}
	refsHeads := filepath.Join(repo, ".git", "refs", "heads")
	for _, dir := range []string{refsHeads, refsPrdash} {
		if err := os.Chmod(dir, 0o555); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = os.Chmod(dir, 0o755) })
	}

	_, err = r.FetchReviewRef(context.Background(), repo, it)
	if err == nil {
		t.Fatal("crear una rama en un directorio de solo lectura dio nil")
	}
	// Y el motivo nombra la rama, que es lo que necesita quien lee: sin el nombre, un fallo de
	// `git branch` se confunde con un fallo del `fetch` de arriba.
	if !strings.Contains(err.Error(), branch) && !strings.Contains(err.Error(), "branch") {
		t.Errorf("el error %q no dice que falla la creación de la rama local", err)
	}
	// Y la rama NO se creó a medias: `git branch` es atómico, así que o existe o no.
	if testutil.RefExists(t, repo, branch) {
		t.Error("la rama se creó pese al fallo del comando: se montaría un review sobre una " +
			"rama que el forge no tiene")
	}
	// Y el ref de seguimiento sigue, que es lo que permite que el siguiente intento solo tenga
	// que crear la rama. Este es el estado intermedio que sí puede quedar y es inofensivo.
	if !testutil.RefExists(t, repo, "refs/prdash/github/7") {
		t.Error("el ref de seguimiento desapareció: el siguiente intento tendría que volver " +
			"a traerlo de la red")
	}
}
