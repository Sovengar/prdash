package reporesolver

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"prdash/internal/forge/model"
	"prdash/internal/testutil"
)

// Only two things here, because the rest of the package is tested: RemoveBare, which was at 0%,
//and buildIndex's two prunings.

// The two halves matter for opposite reasons.
func TestQuitarElBareBorraLoQueHayYToleraLoQueNo(t *testing.T) {
	r := New(Options{MemoPath: filepath.Join(t.TempDir(), "memo.json")})
	ref := model.RepoRef{Forge: "github", Host: "github.com", Project: "o/r", Owner: "o", Name: "r"}

	bare := r.barePath(ref)
	if _, err := os.Stat(bare); err == nil {
		t.Fatal("el bare ya existía antes de probar")
	}
	if err := r.RemoveBare(ref); err != nil {
		t.Errorf("quitar un bare que no existe dio %v, want nil: la limpieza no puede "+
			"fallar y tapar el error del montaje", err)
	}
	// And the bare's path still does not exist. Checking the parent would prove nothing here.
	if _, err := os.Stat(bare); err == nil {
		t.Error("quitar un bare inexistente lo dejó creado")
	}

	// Existe: se borra entero.
	if err := os.MkdirAll(filepath.Join(bare, "objects", "pack"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(bare, "HEAD"), []byte("ref: refs/heads/main\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := r.RemoveBare(ref); err != nil {
		t.Fatalf("RemoveBare: %v", err)
	}
	if _, err := os.Stat(bare); err == nil {
		t.Error("el bare sigue en disco después de quitarlo")
	}
	if _, err := os.Stat(filepath.Dir(bare)); err != nil {
		t.Errorf("RemoveBare se llevó el directorio padre: %v", err)
	}

	if err := r.RemoveBare(ref); err != nil {
		t.Errorf("la segunda vez dio %v, want nil", err)
	}

	otro := model.RepoRef{Forge: "github", Host: "github.com", Project: "o/otro", Owner: "o", Name: "otro"}
	if err := os.MkdirAll(r.barePath(otro), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := r.RemoveBare(ref); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(r.barePath(otro)); err != nil {
		t.Errorf("quitar un repo se llevó el bare del otro: %v", err)
	}
}

// Pruning hidden directories is noise with consequences: a `.git` directory is hidden.
func TestElIndicePodaLoOcultoYNoIndexaLoQueNoEsRepo(t *testing.T) {
	base := t.TempDir()

	repo := filepath.Join(base, "proyecto")
	testutil.InitRepo(t, repo)
	testutil.CommitFile(t, repo, "base.txt", "base", "base")
	testutil.SetRemote(t, repo, "origin", "https://github.com/acme/proyecto.git")

	oculto := filepath.Join(base, ".cache")
	if err := os.MkdirAll(oculto, 0o755); err != nil {
		t.Fatal(err)
	}
	dentro := filepath.Join(oculto, "dentro")
	testutil.InitRepo(t, dentro)
	testutil.CommitFile(t, dentro, "x.txt", "x", "x")
	testutil.SetRemote(t, dentro, "origin", "https://github.com/otro/oculto.git")

	if err := os.MkdirAll(filepath.Join(base, "normal"), 0o755); err != nil {
		t.Fatal(err)
	}
	sinRemoto := filepath.Join(base, "sin-remoto")
	testutil.InitRepo(t, sinRemoto)
	testutil.CommitFile(t, sinRemoto, "y.txt", "y", "y")

	r := resolverConHosts(t, base)
	idx := r.buildIndex()

	claveRepo := repoKey(model.RepoRef{Forge: "github", Host: "github.com", Project: "acme/proyecto",
		Owner: "acme", Name: "proyecto"})
	claveOculto := repoKey(model.RepoRef{Forge: "github", Host: "github.com", Project: "otro/oculto",
		Owner: "otro", Name: "oculto"})

	if _, ok := idx[claveRepo]; !ok {
		t.Errorf("el repo normal no se indexo: %v", idx)
	}
	if _, ok := idx[claveOculto]; ok {
		t.Errorf("un repo dentro de un directorio oculto se indexo: %v", idx)
	}
	for clave, local := range idx {
		if strings.Contains(local, string(os.PathSeparator)+".cache") {
			t.Errorf("una entrada del indice apunta dentro de un oculto: %s -> %s", clave, local)
		}
		if !strings.HasPrefix(local, base) {
			t.Errorf("una entrada del indice apunta fuera del root: %s -> %s", clave, local)
		}
		// The key is the canonical one, not the raw URL: that is what lets ResolveLocal and the index
		// agree.
		if !strings.Contains(clave, "acme/proyecto") {
			t.Errorf("una clave del indice no parece canonica: %q", clave)
		}
	}
	if got := idx[claveRepo]; got != repo {
		t.Errorf("la entrada del indice apunta a %q, want %q", got, repo)
	}
}

// Not general robustness: a configured root that does not exist is normal (an unmounted volume).
func TestUnRootQueNoExisteNoSeLlevaPorDelanteElBueno(t *testing.T) {
	base := t.TempDir()
	repo := filepath.Join(base, "proyecto")
	testutil.InitRepo(t, repo)
	testutil.CommitFile(t, repo, "base.txt", "base", "base")
	testutil.SetRemote(t, repo, "origin", "https://github.com/acme/proyecto.git")

	bueno := repoKey(model.RepoRef{Forge: "github", Host: "github.com", Project: "acme/proyecto",
		Owner: "acme", Name: "proyecto"})
	inexistente := filepath.Join(base, "no-existe")
	conMalo := resolverConHosts(t, inexistente, base).buildIndex()
	soloBueno := resolverConHosts(t, base).buildIndex()

	if _, ok := conMalo[bueno]; !ok {
		t.Fatalf("un root inexistente se llevo por delante el bueno: %v", conMalo)
	}
	if len(conMalo) != len(soloBueno) {
		t.Errorf("un root inexistente cambio el indice: %d entradas con el malo, %d sin el",
			len(conMalo), len(soloBueno))
	}
	fichero := filepath.Join(base, "un-fichero")
	if err := os.WriteFile(fichero, []byte("no soy un directorio"), 0o644); err != nil {
		t.Fatal(err)
	}
	conFichero := resolverConHosts(t, fichero, base).buildIndex()
	if len(conFichero) != len(soloBueno) {
		t.Errorf("un root que es un fichero cambio el indice: %v", conFichero)
	}

	// And with no roots: an empty index, not an error.
	vacio := resolverConHosts(t).buildIndex()
	if len(vacio) != 0 {
		t.Errorf("sin roots dio %v", vacio)
	}
}

// Without the mapping, parseRemote does not know the host.
func resolverConHosts(t *testing.T, roots ...string) *Resolver {
	t.Helper()
	return New(Options{
		Roots:    roots,
		MemoPath: filepath.Join(t.TempDir(), "memo.json"),
		Hosts:    map[string]string{"github.com": "github"},
	})
}
