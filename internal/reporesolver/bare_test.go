package reporesolver

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"prdash/internal/forge/model"
)

// Clone-then-rename instead of cloning in place, and that is what makes a failure leave nothing.

func bareDePrueba(t *testing.T) (r *Resolver, ref model.RepoRef, dest, origin string) {
	t.Helper()
	origin, _ = fixture(t)
	ref = ghRef()
	cloneDir := filepath.Join(t.TempDir(), "repos")
	r = New(Options{
		Roots:    []string{filepath.Dir(origin)},
		CloneDir: cloneDir,
		MemoPath: filepath.Join(t.TempDir(), "memo.json"),
	})
	r.cloneURL = func(model.RepoRef) string { return origin }
	// The path comes from barePath, which includes forge and host.
	dest = r.barePath(ref)
	return r, ref, dest, origin
}

func TestEnsureBareReutilizaElQueYaHayYNoVuelveAClonar(t *testing.T) {
	r, ref, dest, _ := bareDePrueba(t)
	ctx := context.Background()

	if _, err := r.EnsureBare(ctx, ref); err != nil {
		t.Fatalf("el primer EnsureBare: %v", err)
	}
	marca := filepath.Join(dest, "marcador")
	if err := os.WriteFile(marca, []byte("no me borres"), 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := r.EnsureBare(ctx, ref)
	if err != nil {
		t.Fatalf("el segundo EnsureBare: %v", err)
	}
	if got != dest {
		t.Errorf("EnsureBare dio %q, want la ruta canónica %q", got, dest)
	}
	if _, err := os.Stat(marca); err != nil {
		t.Errorf("EnsureBare re-clonó encima de un bare sano: un clon nuevo no traería el "+
			"marcador (%v)", err)
	}
	if !isRepo(dest) {
		t.Error("tras el segundo EnsureBare la ruta no es un repo")
	}
}

// The case that makes the cleanup exist.
func TestEnsureBareLimpiaLosRestosDeUnIntentoFallidoAntesDeReintentar(t *testing.T) {
	r, ref, dest, _ := bareDePrueba(t)
	ctx := context.Background()

	if err := os.MkdirAll(filepath.Join(dest, "objects"), 0o755); err != nil {
		t.Fatal(err)
	}
	resto := filepath.Join(dest, "objects", "incompleto")
	if err := os.WriteFile(resto, []byte("basura"), 0o644); err != nil {
		t.Fatal(err)
	}
	if isRepo(dest) {
		t.Fatal("el fixture no es lo que dice: los restos parecerían un repo")
	}

	got, err := r.EnsureBare(ctx, ref)
	if err != nil {
		t.Fatalf("EnsureBare con restos: %v", err)
	}
	if got != dest {
		t.Errorf("EnsureBare dio %q, want %q", got, dest)
	}
	if !isRepo(dest) {
		t.Error("tras EnsureBare la ruta canónica no es un repo")
	}
	if _, err := os.Stat(resto); err == nil {
		t.Error("los restos del intento fallido siguen ahí")
	}
}

// Not only the error: no temporary left behind.
func TestEnsureBarePropagaElFalloDeClonarYNoDejaBasura(t *testing.T) {
	r, ref, dest, _ := bareDePrueba(t)
	r.cloneURL = func(model.RepoRef) string { return "file:///no-existe/prueba.git" }

	_, err := r.EnsureBare(context.Background(), ref)
	if err == nil {
		t.Fatal("clonar una URL inexistente dio nil")
	}
	if !strings.Contains(err.Error(), "no-existe/prueba.git") {
		t.Errorf("el error %q no nombra la URL que se intentó clonar", err)
	}
	if _, err := os.Stat(dest); err == nil {
		t.Error("EnsureBare dejó algo en la ruta canónica tras fallar")
	}
	temporales := glob(t, filepath.Dir(dest), "*.tmp-*")
	if len(temporales) != 0 {
		t.Errorf("EnsureBare dejó %d temporales: %v", len(temporales), temporales)
	}

	// The failure is REPEATABLE: a clone that fails once does not leave the tree in a state that makes
	//the next attempt fail for a different reason.
	r.cloneURL = func(model.RepoRef) string { return "file:///no-existe/prueba.git" }
	_, err2 := r.EnsureBare(context.Background(), ref)
	if err2 == nil {
		t.Fatal("el segundo intento dio nil")
	}
	// The messages are not identical (they carry the temporary's name, which changes) and the first
	//version compared the whole strings.
	if !strings.Contains(err2.Error(), "no-existe/prueba.git") {
		t.Errorf("el segundo intento dio %v: vuelve a fallar por otra razón", err2)
	}
	if strings.Contains(err2.Error(), "already exists") {
		t.Errorf("el segundo intento falló por un clon previo que quedó: %v", err2)
	}
}

// Mount calls it when the mount fails AFTERWARD.
func TestRemoveBareYBorraLoQueHayYToleraLoQueNo(t *testing.T) {
	r, ref, dest, _ := bareDePrueba(t)
	ctx := context.Background()

	// It does not exist: nil, and nothing created on the way.
	if err := r.RemoveBare(ref); err != nil {
		t.Errorf("quitar un bare que no existe dio %v, want nil", err)
	}
	if _, err := os.Stat(dest); err == nil {
		t.Error("RemoveBare creó el bare")
	}

	// Existe: se borra.
	if _, err := r.EnsureBare(ctx, ref); err != nil {
		t.Fatal(err)
	}
	if err := r.RemoveBare(ref); err != nil {
		t.Fatalf("RemoveBare: %v", err)
	}
	if _, err := os.Stat(dest); err == nil {
		t.Error("el bare sigue en disco después de quitarlo")
	}
	// And the parent survives, which is what stops two mounts of the same repo from stepping on each
	// other.
	if _, err := os.Stat(filepath.Dir(dest)); err != nil {
		t.Errorf("RemoveBare se llevó el directorio padre: %v", err)
	}

	// Removing it twice does not fail the second time, which is what happens if the cleanup code is
	// not idempotent.
	if err := r.RemoveBare(ref); err != nil {
		t.Errorf("la segunda vez dio %v, want nil", err)
	}
}
