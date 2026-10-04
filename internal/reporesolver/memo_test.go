package reporesolver

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"prdash/internal/forge/model"
	"prdash/internal/testutil"
)

// Aquí solo hay dos cosas, porque las demás de este paquete ya tienen test: `RemoveBare`,
// que estaba al 0%, y las dos podas de `buildIndex`, que solo se ejercitaban con un repo
// limpio y por eso no llegaban a la rama de podar nada.
//
// Y las dos importan por lo queROMpen cuando no funcionan, no por lo que hacen cuando
// funcionan.

// TestQuitarElBareBorraLoQueHayYToleraLoQueNo: `RemoveBare`.
//
// Y las dos mitades importan por motivos opuestos, que es lo que hace que la función exista
// en vez de llamar a `os.RemoveAll` en el sitio del error.
//
// La primera: si el clon existe, se borra entero, incluido su contenido. Un bare a medias
// ocupa lo que ocupa un clon completo, y dejar uno por cada montaje fallido llena el disco
// sin que nada avise de que está pasando.
//
// La segunda: si NO existe, `RemoveBare` devuelve nil. Y no por cortesía: se llama en la
// limpieza de un error de montaje, y si la limpieza falla porque el clon no estaba, quien lee
// el error ve "no such file or directory" en vez de lo que pasó de verdad, que es el fallo
// del montaje. La limpieza no puede tapar el error que la motivó.
func TestQuitarElBareBorraLoQueHayYToleraLoQueNo(t *testing.T) {
	r := New(Options{MemoPath: filepath.Join(t.TempDir(), "memo.json")})
	ref := model.RepoRef{Forge: "github", Host: "github.com", Project: "o/r", Owner: "o", Name: "r"}

	// No existe: nil, y sin crear nada en el camino.
	bare := r.barePath(ref)
	if _, err := os.Stat(bare); err == nil {
		t.Fatal("el bare ya existía antes de probar")
	}
	if err := r.RemoveBare(ref); err != nil {
		t.Errorf("quitar un bare que no existe dio %v, want nil: la limpieza no puede "+
			"fallar y tapar el error del montaje", err)
	}
	// Y la ruta del bare sigue sin existir. Comprobar el padre no serviría de nada aquí:
	// lo crea el constructor del resolver, no `RemoveBare`, así que ese aserto no
	// distinguiría un `RemoveBare` que creara directorios de uno que no.
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
	// Y el padre sobrevive, que es lo que evita que dos montajes del mismo repo se pisen
	// el directorio uno a otro.
	if _, err := os.Stat(filepath.Dir(bare)); err != nil {
		t.Errorf("RemoveBare se llevó el directorio padre: %v", err)
	}

	// Y quitarlo dos veces no falla la segunda, que es lo que pasa si el código de limpieza
	// se llama en un reintento.
	if err := r.RemoveBare(ref); err != nil {
		t.Errorf("la segunda vez dio %v, want nil", err)
	}

	// Y dos repos distintos no se pisan: el bare de uno no es el del otro.
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

// TestElIndicePodaLoOcultoYNoIndexaLoQueNoEsRepo: las podas de `buildIndex`.
//
// Y la poda de los ocultos es de ruido con consecuencias: un `~/dev/.cache` o un
// `~/dev/.something` contienen directorios que no son repos, y un repo encontrado dentro de
// uno produce una entrada en el índice con un remoto que no es el del repo, que hace que un
// fetch vaya a un sitio equivocado. Y la poda es por NOMBRE de la entrada, no por la ruta
// completa, así que salta el subdirectorio entero en vez de entrar y comprobar cada hijo.
//
// Y el directorio sin remoto tiene que quedar fuera del índice sin romper el escaneo, que
// es el caso de cualquier carpeta que alguien tenga en `~/dev`.
func TestElIndicePodaLoOcultoYNoIndexaLoQueNoEsRepo(t *testing.T) {
	base := t.TempDir()

	// Un repo normal, que sí se indexa.
	repo := filepath.Join(base, "proyecto")
	testutil.InitRepo(t, repo)
	testutil.CommitFile(t, repo, "base.txt", "base", "base")
	testutil.SetRemote(t, repo, "origin", "https://github.com/acme/proyecto.git")

	// Un directorio oculto CON un repo dentro: no se indexa, y no se baja a mirarlo.
	oculto := filepath.Join(base, ".cache")
	if err := os.MkdirAll(oculto, 0o755); err != nil {
		t.Fatal(err)
	}
	dentro := filepath.Join(oculto, "dentro")
	testutil.InitRepo(t, dentro)
	testutil.CommitFile(t, dentro, "x.txt", "x", "x")
	testutil.SetRemote(t, dentro, "origin", "https://github.com/otro/oculto.git")

	// Un directorio normal sin remoto: no hay nada que indexar y no debe romper nada.
	if err := os.MkdirAll(filepath.Join(base, "normal"), 0o755); err != nil {
		t.Fatal(err)
	}
	// Y un repo sin ningún remoto, que es un clon recién hecho antes de apuntarlo.
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
		// Y la clave es la canónica, no la URL cruda: es lo que hace que `ResolveLocal`
		// y el memo hable el mismo idioma.
		if !strings.Contains(clave, "acme/proyecto") {
			t.Errorf("una clave del indice no parece canonica: %q", clave)
		}
	}
	// Y la entrada del repo bueno apunta a su ruta, que es lo que hace que
	// `ResolveLocal` acierte.
	if got := idx[claveRepo]; got != repo {
		t.Errorf("la entrada del indice apunta a %q, want %q", got, repo)
	}
}

// TestUnRootQueNoExisteNoSeLlevaPorDelanteElBueno: la parte que se traga el error.
//
// Y no es robustez en general: un root configurado que no existe es el caso de `~/dev` en
// una máquina donde todavía no se ha clonado nada, que es el primer arranque. Si eso
// hiciera fallar la resolución, la app no arrancaría en una máquina nueva.
//
// Y el escaneo no puede abortar a medio camino por el root malo y quedarse sin los demás: por
// eso la comparación es entre los dos índices y no contra un valor absoluto.
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
	// Y un root que es un FICHERO, que es más raro pero llega: un `Roots` mal escrito.
	fichero := filepath.Join(base, "un-fichero")
	if err := os.WriteFile(fichero, []byte("no soy un directorio"), 0o644); err != nil {
		t.Fatal(err)
	}
	conFichero := resolverConHosts(t, fichero, base).buildIndex()
	if len(conFichero) != len(soloBueno) {
		t.Errorf("un root que es un fichero cambio el indice: %v", conFichero)
	}

	// Y sin ningún root: índice vacío, no un error. Un resolver sin roots configurados es
	// el caso de `--print` con una config mínima.
	vacio := resolverConHosts(t).buildIndex()
	if len(vacio) != 0 {
		t.Errorf("sin roots dio %v", vacio)
	}
}

// resolverConHosts construye un resolver con los hosts de GitHub mapeados y la memoria en
// un temporal. Sin el mapeo, `parseRemote` no sabe que "github.com" es el forge github y el
// índice queda vacío —la primera versión de estos tests no lo mapeaba y.Index() salía
// vacío en los tres, sin que el fallo dijera nada del mapeo—.
func resolverConHosts(t *testing.T, roots ...string) *Resolver {
	t.Helper()
	return New(Options{
		Roots:    roots,
		MemoPath: filepath.Join(t.TempDir(), "memo.json"),
		Hosts:    map[string]string{"github.com": "github"},
	})
}
