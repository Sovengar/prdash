package reporesolver

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"prdash/internal/forge/model"
)

// `EnsureBare` clona el repo a un temporal y lo renombra a su sitio, y esa estrategia de
// "renombrar en vez de clonar directamente" es lo que hay que fijar: es lo que garantiza que
// nunca haya un clon a medio hacer en la ruta final.
//
// Y el motivo de que importe no es teórico. Un `git clone` que se corta —el disco lleno, la
// red se cae, Ctrl-C— deja un directorio con la mitad de los objetos. Si ese directorio
// estuviera en la ruta final, el siguiente intento lo encontraría ahí, `isRepo` lo daría por
// bueno, y se clonaría desde un clon roto: un fetch posterior daría "not a git repository"
// sin explicación.

// bareDePrueba deja un resolutor cuyo CloneDir está vacío y devuelve la ruta que le
// corresponde al bare de `ref`.
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
	// La ruta la compone `barePath`, que incluye forge y host. La primera versión de
	// este fixture la componía a mano como `acme/proy.git`, que no es la ruta de nadie, y
	// los restos se creaban en un sitio donde EnsureBare no miraba.
	dest = r.barePath(ref)
	return r, ref, dest, origin
}

// TestEnsureBareReutilizaElQueYaHayYNoVuelveAClonar: el camino de reutilización.
//
// Y lo que se comprueba con más cuidado es que la ruta canónica no se toca: si `EnsureBare`
// re-clonara encima de un bare sano, perdería el mirror local y un `fetch` posterior tardaría
// lo que tardara la red. Es lo que hace que `isRepo` vaya antes que cualquier escritura.
func TestEnsureBareReutilizaElQueYaHayYNoVuelveAClonar(t *testing.T) {
	r, ref, dest, _ := bareDePrueba(t)
	ctx := context.Background()

	if _, err := r.EnsureBare(ctx, ref); err != nil {
		t.Fatalf("el primer EnsureBare: %v", err)
	}
	// Un marcador dentro del bare, que un clon nuevo no tendría.
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
	// Y el bare sigue siendo un repo de verdad, no un directorio con objetos sueltos.
	if !isRepo(dest) {
		t.Error("tras el segundo EnsureBare la ruta no es un repo")
	}
}

// TestEnsureBareLimpiaLosRestosDeUnIntentoFallidoAntesDeReintentar: la limpieza.
//
// Y el caso es el que hace que la limpieza exista: un directorio en la ruta canónica que NO
// es un repo —porque un clon anterior se cortó a mitad— se tiene que quitar antes de
// reintentar. Sin la limpieza, el `git clone` fallaría con "destination path already exists",
// y el montaje de ese review se quedaría atascado para siempre sin importar cuántas veces se
// reintentara.
//
// Y el motivo de que el orden sea "repo sano primero, existencia después" es que un bare
// SANO también existe, y ese no se toca. Al revés, `EnsureBare` borraría el clon que acaba
// de hacer y volvería a empezar siempre.
func TestEnsureBareLimpiaLosRestosDeUnIntentoFallidoAntesDeReintentar(t *testing.T) {
	r, ref, dest, _ := bareDePrueba(t)
	ctx := context.Background()

	// Un intento previo que se quedó a medias: directorio con contenido pero sin repo.
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

// TestEnsureBarePropagaElFalloDeClonarYNoDejaBasura: el camino de error.
//
// Y lo que se comprueba no es solo el error: es que NO queda un temporal en el árbol de
// ficheros. El temporal se limpia en el camino de fallo, y sin esa limpieza cada intento
// fallido dejaría un directorio —del tamaño de un clon— que se acumularía sin que nada los
// borrara.
//
// Y el mensaje tiene que nombrar la URL que se intentó clonar: "clonar" a secas deja a quien
// depura sin saber si el problema es la URL, la red o las credenciales.
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
	// Y no quedó ni el temporal ni un repo a medio hacer en la ruta canónica.
	if _, err := os.Stat(dest); err == nil {
		t.Error("EnsureBare dejó algo en la ruta canónica tras fallar")
	}
	temporales := glob(t, filepath.Dir(dest), "*.tmp-*")
	if len(temporales) != 0 {
		t.Errorf("EnsureBare dejó %d temporales: %v", len(temporales), temporales)
	}

	// Y el fallo es REPETIBLE: un clon que falla una vez no deja el árbol en un estado que
	// hace que el siguiente intento falle por otro motivo —"destination already exists" en
	// vez de "no such repository"—, que es la clase de fallo que hace que un usuario piense
	// que su repo está mal cuando lo que está mal es el estado del caché.
	r.cloneURL = func(model.RepoRef) string { return "file:///no-existe/prueba.git" }
	_, err2 := r.EnsureBare(context.Background(), ref)
	if err2 == nil {
		t.Fatal("el segundo intento dio nil")
	}
	// El mensaje NO es identico —lleva el nombre del temporal, que cambia— y la primera
	// version de este aserto comparaba los strings enteros. Lo que tiene que repetirse es la
	// CAUSA, y sobre todo que no sea "destination path already exists": ese sería el
	// síntoma de que el primer intento dejó basura, que es justo lo que se comprueba.
	if !strings.Contains(err2.Error(), "no-existe/prueba.git") {
		t.Errorf("el segundo intento dio %v: vuelve a fallar por otra razón", err2)
	}
	if strings.Contains(err2.Error(), "already exists") {
		t.Errorf("el segundo intento falló por un clon previo que quedó: %v", err2)
	}
}

// TestRemoveBareYBorraLoQueHayYToleraLoQueNo: la limpieza del camino de error del montaje.
//
// Y `Mount` la llama cuando el montaje falla DESPUÉS de haber clonado. Y las dos mitades
// importan por motivos opuestos: si existe lo borra —un bare por cada montaje fallido llena
// el disco sin que nada avise—, y si no existe devuelve nil, porque se llama en la limpieza
// de un error y si la limpieza fallara por "no such file or directory", quien lee el error
// vería eso en vez del fallo del montaje.
func TestRemoveBareYBorraLoQueHayYToleraLoQueNo(t *testing.T) {
	r, ref, dest, _ := bareDePrueba(t)
	ctx := context.Background()

	// No existe: nil, y sin crear nada en el camino. El padre puede existir —lo crea el
	// constructor— así que lo que se comprueba es que el bare sigue sin aparecer.
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
	// Y el padre sobrevive, que es lo que evita que dos montajes del mismo repo se pisen
	// el directorio uno a otro.
	if _, err := os.Stat(filepath.Dir(dest)); err != nil {
		t.Errorf("RemoveBare se llevó el directorio padre: %v", err)
	}

	// Y quitarlo dos veces no falla la segunda, que es lo que pasa si el código de limpieza
	// se llama en un reintento.
	if err := r.RemoveBare(ref); err != nil {
		t.Errorf("la segunda vez dio %v, want nil", err)
	}
}
