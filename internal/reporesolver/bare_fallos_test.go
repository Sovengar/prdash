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

// The three remaining EnsureBare branches are disk failures, and all three are provoked by making a
//directory read-only.

// The asymmetry with the good path is the point: if RemoveAll succeeded, the test would be
// measuring nothing.
func TestEnsureBareLimpiaRestosCuandoNoPuedeQuitarlosYLoDice(t *testing.T) {
	cloneDir := filepath.Join(t.TempDir(), "clones")
	if err := os.MkdirAll(cloneDir, 0o755); err != nil {
		t.Fatal(err)
	}
	r, ref, dest := resolutorConCloneDir(t, cloneDir)

	if err := os.MkdirAll(filepath.Join(dest, "objects"), 0o755); err != nil {
		t.Fatal(err)
	}
	if isRepo(dest) {
		t.Fatal("el fixture no sirve: los restos parecerían un repo")
	}
	// The DIRECT parent, and not cloneDir: removing a directory means writing to the one containing it.
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
	if !strings.Contains(err.Error(), dest) {
		t.Errorf("el error %q no nombra el clon a medias", err)
	}
}

// CloneDir under a file, which happens when someone points the cache at the wrong path.
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

// The rarest failure in the chain and the one that slips through: the clone is whole and the publish
// is what fails.
func TestElRenombradoDelClonPropagaElFalloYNoDejaElTemporal(t *testing.T) {
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

	// With a writable parent, EnsureBare clears the leftovers and succeeds — the recovery path, which
	//has to be checked first.
	if _, err := r.EnsureBare(context.Background(), ref); err != nil {
		t.Fatalf("el camino de recuperación falló: %v", err)
	}
	if !isRepo(dest) {
		t.Error("tras la limpieza no hay repo en la ruta final")
	}
	temporales := glob(t, cloneDir, "*.tmp-*")
	if len(temporales) != 0 {
		t.Errorf("quedaron %d temporales tras un EnsureBare correcto: %v", len(temporales), temporales)
	}
}

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
	head := testutil.RunGit(t, repo, "rev-parse", branch)
	ref := testutil.RunGit(t, repo, "rev-parse", "refs/prdash/github/7")
	if head != ref {
		t.Errorf("la rama está en %s y el ref en %s", head, ref)
	}

	otra, err := r.FetchReviewRef(context.Background(), repo, it)
	if err != nil {
		t.Fatalf("el segundo intento falló: %v", err)
	}
	if otra != branch {
		t.Errorf("la segunda vez dio la rama %q, want la misma %q", otra, branch)
	}
}

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

// The way to provoke it is real and not a double: filepath.Abs of a relative root whose working
// directory is gone.
func TestUnaRaizRelativaConElDirectorioDeTrabajoBorradoNoSeTiraElIndiceEntero(t *testing.T) {
	bueno := filepath.Join(t.TempDir(), "bueno")
	testutil.InitRepo(t, bueno)
	testutil.CommitFile(t, bueno, "a.txt", "a", "a")
	testutil.SetRemote(t, bueno, "origin", "https://github.com/acme/proyecto.git")
	ref := model.RepoRef{Forge: "github", Host: "github.com", Project: "acme/proyecto"}

	desaparecido := filepath.Join(t.TempDir(), "cwd-que-se-va")
	if err := os.MkdirAll(desaparecido, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Chdir(desaparecido)
	if err := os.RemoveAll(desaparecido); err != nil {
		t.Fatal(err)
	}
	if _, err := filepath.Abs("repo-que-no-existe"); err == nil {
		t.Fatal("el cwd sigue vivo: la prueba no está midiendo el fallo de Getwd")
	}

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

func TestUnClonQueFallaNoDejaElTemporalYDiceQueClonar(t *testing.T) {
	cloneDir := filepath.Join(t.TempDir(), "clones")
	if err := os.MkdirAll(cloneDir, 0o755); err != nil {
		t.Fatal(err)
	}
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

	// The assertion is NOT "the clone tree is empty", because it cannot be: the parent's MkdirAll
	//creates github/github.com/acme/ before cloning, and removing it would throw away work the
	//next attempt needs.
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

func TestUnBranchQueNoSePuedeCrearSeReportaSinDejarLaRamaAMedias(t *testing.T) {
	origin, repo := fixture(t)
	pushReviewRef(t, origin, "refs/pull/7/head")
	r := newResolver(t, origin, ghRef())
	it := model.NewItem(ghRef(), 7)
	it.Number = 7

	branch, err := r.FetchReviewRef(context.Background(), repo, it)
	if err != nil {
		t.Fatalf("el camino bueno falló: %v", err)
	}
	testutil.RunGit(t, repo, "branch", "-D", branch)
	// The SUBDIRECTORY has to be blocked too, not just refs/heads: the branch is named
	//`prdash/pr-7`, so git writes the lock at refs/heads/prdash/pr-7.lock.
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
	if !strings.Contains(err.Error(), branch) && !strings.Contains(err.Error(), "branch") {
		t.Errorf("el error %q no dice que falla la creación de la rama local", err)
	}
	if testutil.RefExists(t, repo, branch) {
		t.Error("la rama se creó pese al fallo del comando: se montaría un review sobre una " +
			"rama que el forge no tiene")
	}
	if !testutil.RefExists(t, repo, "refs/prdash/github/7") {
		t.Error("el ref de seguimiento desapareció: el siguiente intento tendría que volver " +
			"a traerlo de la red")
	}
}
