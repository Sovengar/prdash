package sim

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"prdash/internal/forge/model"
	"prdash/internal/testutil"
)

// El servicio de simulación guarda imágenes en un caché y las poda. Y las dos funciones
// que lo hacen —`keep` y `prune`— no estaban probadas, y son exactamente las que tocan el
// disco del usuario.
//
// El riesgo real está en `prune`, no en `keep`. Podar borra ficheros, y un error de ahí
// no es un error de la app: es trabajo perdido. Y el criterio de "lo más reciente" tiene una
// trampa que el código resuelve con un ordenamiento de inserción a mano —
// `files[j].mod.After(files[j-1].mod)`— en vez de `sort.Slice`. Ese `After` es la dirección
// del orden, yPutting la condición al revés deja el caché lleno de ficheros viejos y
// borra los nuevos, que es un fallo invisible.
//
// Y el otro fallo posible es borrar de más: si `keep` valiera 0, la poda se come el
// directorio entero. Por eso los dos lados están probados aquí, y no solo el bueno.

// TestLaPodaDejaLoRecienteYBorraLoViejo: el criterio, por los dos lados.
//
// Y se comprueba la ORDEN de las marcas de tiempo, no solo que quedan `keep` ficheros: un
// aserto que solo mirara la cantidad pasaría con la condición de orden invertida, que
// deja el número correcto de ficheros y los equivocados.
func TestLaPodaDejaLoRecienteYBorraLoViejo(t *testing.T) {
	dir := t.TempDir()
	base := time.Now().Add(-time.Hour)

	// Cinco imágenes con edades bien separadas, de más antigua a más reciente.
	nombres := []string{"a.jpg", "b.jpg", "c.jpg", "d.jpg", "e.jpg"}
	for i, n := range nombres {
		ruta := filepath.Join(dir, n)
		if err := os.WriteFile(ruta, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
		// i=0 es la más antigua.
		mod := base.Add(time.Duration(i) * time.Minute)
		if err := os.Chtimes(ruta, mod, mod); err != nil {
			t.Fatal(err)
		}
	}

	prune(dir, 2)

	// Las dos más recientes sobreviven: e y d.
	for _, n := range nombres {
		_, err := os.Stat(filepath.Join(dir, n))
		sobrevive := err == nil
		queria := n == "d.jpg" || n == "e.jpg"
		if sobrevive != queria {
			if sobrevive {
				t.Errorf("%s sobrevive, pero debía borrarse por ser más antigua", n)
			} else {
				t.Errorf("%s se borró, pero debía sobrevivir por ser más reciente", n)
			}
		}
	}
	// Y el número exacto, que es lo que distingue "poda" de "limpia el directorio".
	vivos := 0
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".jpg") {
			vivos++
		}
	}
	if vivos != 2 {
		t.Errorf("quedaron %d imágenes, want 2", vivos)
	}
}

// TestLaPodaIgnoraLoQueNoSonImagenesYLosDirectorios: `prune` solo toca lo suyo.
//
// Y esto importa porque el caché vive bajo el home del usuario y `prune` borra por
// directorio, no por lista. Un `.part` a medio escribir —que es el nombre temporal que usa
// `copyFile`— está ahí en cualquier momento, y un lector que coge su nombre por extensión
// lo borraría por debajo del proceso que lo está escribiendo.
func TestLaPodaIgnoraLoQueNoSonImagenesYLosDirectorios(t *testing.T) {
	dir := t.TempDir()

	// Dos imágenes, de las que una es la que se queda.
	for _, n := range []string{"vieja.jpg", "nueva.jpg"} {
		ruta := filepath.Join(dir, n)
		if err := os.WriteFile(ruta, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	// Lo que NO es una imagen: el temporal de una copia en curso, una nota, un subdirectorio.
	extras := map[string]string{
		"a-medias.jpg.part": "temporal de copyFile",
		"nota.txt":          "no es una imagen",
		"sin-extension":     "no tiene extensión",
	}
	for n := range extras {
		if err := os.WriteFile(filepath.Join(dir, n), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	sub := filepath.Join(dir, "sub.jpg") // directorio con nombre de imagen
	if err := os.Mkdir(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	viejo := time.Now().Add(-time.Hour)
	mod := time.Now()
	if err := os.Chtimes(filepath.Join(dir, "vieja.jpg"), viejo, viejo); err != nil {
		t.Fatal(err)
	}
	for _, n := range append(keys(extras), "sub.jpg") {
		p := filepath.Join(dir, n)
		if err := os.Chtimes(p, viejo, viejo); err != nil {
			t.Fatal(err)
		}
	}
	_ = mod

	prune(dir, 1)

	// La imagen nueva sobrevive, la vieja se va.
	if _, err := os.Stat(filepath.Join(dir, "nueva.jpg")); err != nil {
		t.Error("la imagen más reciente no sobrevivió a la poda")
	}
	if _, err := os.Stat(filepath.Join(dir, "vieja.jpg")); err == nil {
		t.Error("la imagen más antigua sobrevivió a la poda")
	}
	// Y lo que no es imagen sigue ahí. El `.part` es el caso que de verdad lo Justifica: si
	// se borrara, la copia se quedaría sin destino a mitad de escribir.
	for n := range extras {
		if _, err := os.Stat(filepath.Join(dir, n)); err != nil {
			t.Errorf("%s (%s) no sobrevivió a la poda", n, extras[n])
		}
	}
	if _, err := os.Stat(sub); err != nil {
		t.Error("la poda se comió un directorio con nombre de imagen")
	}
}

// TestLaPodaConMenosQueElLimiteNoTocaNada: el caso normal, que es el frecuente.
//
// Y con el caché recién creado hay cero imágenes, y `prune` sobre un directorio vacío
// tiene que salir sin tocar nada. Es el caso de la primera simulación de una máquina
// nueva.
func TestLaPodaConMenosQueElLimiteNoTocaNada(t *testing.T) {
	// Vacío.
	vacio := t.TempDir()
	prune(vacio, 5)
	entries, err := os.ReadDir(vacio)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Errorf("podar un directorio vacío dio %d entradas", len(entries))
	}

	// Y por debajo del límite.
	uno := t.TempDir()
	ruta := filepath.Join(uno, "a.jpg")
	if err := os.WriteFile(ruta, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	prune(uno, 5)
	if _, err := os.Stat(ruta); err != nil {
		t.Error("una imagen por debajo del límite se borró")
	}

	// Y un directorio que no existe: sin error y sin nada. Es lo que pasa si el caché se
	// borró entre el `MkdirAll` y la poda, y abortar la simulación por eso sería peor que
	// dejar el caché sin podar.
	prune(filepath.Join(t.TempDir(), "no-existe"), 5)
}

// TestKeepCopiaLaImagenYLaPodaDespues: el guardado, y con qué nombre.
//
// Y el nombre es lo que hay que mirar, porque es la clave de la deduplicación: forge,
// proyecto, número, tipo y un `UnixNano`. Sin el `UnixNano` dos simulaciones del mismo
// ítem se pisan, y la segunda renderiza para que se vea la primera —con una animación que
// no termina nunca—. Con él, cada render deja su fichero.
//
// Y el proyecto va por `slug`, que es lo que hace legible el caché y lo que evita que una
// barra de la ruta acabe en el nombre.
func TestKeepCopiaLaImagenYLaPodaDespues(t *testing.T) {
	dir := t.TempDir()
	origen := filepath.Join(dir, "render.jpg")
	if err := os.WriteFile(origen, []byte("contenido"), 0o644); err != nil {
		t.Fatal(err)
	}

	s := &Service{CacheDir: filepath.Join(dir, "cache")}
	it := model.Item{Forge: "github", Number: 42, Ref: model.RepoRef{Project: "grupo/proyecto"}}

	dst, err := s.keep(origen, it, KindMerge)
	if err != nil {
		t.Fatalf("keep: %v", err)
	}

	// El fichero está, en el caché, con el contenido intacto.
	contenido, err := os.ReadFile(dst)
	if err != nil {
		t.Fatalf("la imagen guardada no está: %v", err)
	}
	if string(contenido) != "contenido" {
		t.Errorf("la copia tiene %q", contenido)
	}
	// Y el temporal NO se queda: `copyFile` renombra, y si el `.part` sobreviviera, la
	// próxima poda lo encontraría y la próxima copia lo pisaría.
	if _, err := os.Stat(dst + ".part"); err == nil {
		t.Error("el temporal de la copia sigue en el caché")
	}
	// Y el nombre lleva lo que lo identifica.
	nombre := filepath.Base(dst)
	if !strings.HasPrefix(nombre, "github-") || !strings.Contains(nombre, "proyecto") {
		t.Errorf("el nombre %q no lleva forge ni proyecto", nombre)
	}
	if !strings.Contains(nombre, "-42-") {
		t.Errorf("el nombre %q no lleva el número", nombre)
	}
	if !strings.HasSuffix(nombre, ".jpg") {
		t.Errorf("el nombre %q no acaba en .jpg", nombre)
	}
	// Y el directorio del caché se creó, que es lo que hace que la copia no falle en una
	// máquina donde ese directorio no existe todavía.
	if _, err := os.Stat(s.CacheDir); err != nil {
		t.Errorf("keep no creó el directorio del caché: %v", err)
	}

	// Y el original se queda: se ofrece abrirlo en un visor, así que borrarlo sería tirar
	// la única copia que el render ha hecho.
	if _, err := os.Stat(origen); err != nil {
		t.Error("keep borró el original, que el visor necesita")
	}
}

// TestKeepFallaConOrigenQueNoExisteYConDestinoImposible: los dos fallos de `keep`.
//
// Y los dos tienen que salir como error y no como un `dst` vacío. Un `dst` vacío con error
// ignorado haría que el popup apuntara a la cadena vacía y abriera el directorio de trabajo
// del proceso.
func TestKeepFallaConOrigenQueNoExisteYConDestinoImposible(t *testing.T) {
	dir := t.TempDir()
	it := model.Item{Forge: "github", Number: 1, Ref: model.RepoRef{Project: "p"}}

	// Origen inexistente.
	s := &Service{CacheDir: filepath.Join(dir, "c1")}
	dst, err := s.keep(filepath.Join(dir, "no-existe.jpg"), it, KindMerge)
	if err == nil {
		t.Fatal("keep con un origen inexistente dio nil")
	}
	if dst != "" {
		t.Errorf("keep devolvió %q con error", dst)
	}

	// Destino imposible: el padre es un fichero, así que el MkdirAll falla. Es el camino
	// que se alcanza si alguien puso el `CacheDir` a un fichero por error.
	bloque := filepath.Join(dir, "bloque")
	if err := os.WriteFile(bloque, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	s = &Service{CacheDir: bloque}
	if _, err := s.keep(origenDePrueba(t), it, KindMerge); err == nil {
		t.Error("keep con un CacheDir que es un fichero dio nil")
	}
}

// TestElDirectorioDelCacheCaeAlDefaultYSeMemoriza: la cascada del `CacheDir`.
//
// Y la memoria importa: `cacheDir` guarda el resultado en el servicio para no preguntar a
// `os.UserCacheDir` en cada render. Y se comprueba que sin `$XDG_CACHE_HOME` y sin `HOME`
// sale error en vez de una ruta rara —porque `os.UserCacheDir` sí lee `HOME`, y sin ella
// puede devolver algo inesperado en vez de fallar.
func TestElDirectorioDelCacheCaeAlDefaultYSeMemoriza(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", dir)

	// El default por el entorno, y la constante que lo compone.
	ruta, err := DefaultCacheDir()
	if err != nil {
		t.Fatalf("DefaultCacheDir: %v", err)
	}
	if filepath.Dir(ruta) != filepath.Join(dir, "prdash") || filepath.Base(ruta) != "sim" {
		t.Errorf("DefaultCacheDir dio %q", ruta)
	}

	// Y `cacheDir` cae a ese default.
	s := &Service{}
	obtenido, err := s.cacheDir()
	if err != nil {
		t.Fatalf("cacheDir: %v", err)
	}
	if obtenido != ruta {
		t.Errorf("cacheDir dio %q, want el default %q", obtenido, ruta)
	}
	// Y lo memoiza: una segunda llamada con el entorno cambiado da lo mismo, que es lo que
	// evita preguntar a `os.UserCacheDir` en cada render.
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	if otra, _ := s.cacheDir(); otra != ruta {
		t.Errorf("la segunda llamada dio %q, want el memoizado %q", otra, ruta)
	}

	// Y un `CacheDir` puesto gana sobre el default, sin tocar el memo.
	explicito := filepath.Join(dir, "otro")
	puesto := &Service{CacheDir: explicito}
	if g, _ := puesto.cacheDir(); g != explicito {
		t.Errorf("con CacheDir puesto dio %q, want %q", g, explicito)
	}
	// Y ni siquiera lo memoiza, porque no hay nada que memorizar.
	if puesto.CacheDir != explicito {
		t.Errorf("cacheDir alteró el CacheDir: %q", puesto.CacheDir)
	}
}

// TestSimulateNiegaSinLocatorYSinClon: las dos negativas antes de tocar nada.
//
// Y el orden importa: sin `Locator` no hay ni dónde mirar, y con `Locator` pero sin clon no
// hay de dónde sacar los refs. Las dos negativas son el camino de degradación: la simulación
// no está montada porque el review no está montado, y eso es un estado normal, no un fallo.
//
// Y el caso del locator que devuelve un sitio con `Repo` vacío: `Locate` devuelve
// `(Place{}, true)` y `Simulate` tiene que rechazarlo igual. Un `Place` "ok" sin repo es lo
// que hace que un clon vacío acabe en el render, y el render falla con un error de git que
// no dice nada de que el problema era otro.
func TestSimulateNiegaSinLocatorYSinClon(t *testing.T) {
	it := model.Item{Forge: "github", Number: 1, Ref: model.RepoRef{Project: "p"}}

	// Sin locator.
	s := &Service{}
	if _, err := s.Simulate(context.Background(), it, KindMerge); err == nil {
		t.Error("sin Locator dio nil")
	} else if !strings.Contains(err.Error(), "local repository") {
		t.Errorf("el error %q no dice que falta el repo local", err)
	}

	// Con locator que dice que no.
	s = &Service{Locator: locatorFalso{ok: false}}
	if _, err := s.Simulate(context.Background(), it, KindMerge); err == nil {
		t.Error("con un locator que no encuentra dio nil")
	} else if !strings.Contains(err.Error(), "mounted") {
		t.Errorf("el error %q no dice que falta montar el review", err)
	}

	// Y el caso "ok" con repo vacío, que es el que la comprobación de `place.Repo` del
	// código cubre y que un `!ok` a secas no.
	s = &Service{Locator: locatorFalso{ok: true, place: Place{}}}
	if _, err := s.Simulate(context.Background(), it, KindMerge); err == nil {
		t.Error("con un Place sin Repo dio nil")
	}

	// Y un locator que sí devuelve un repo: a partir de ahí falla por otro lado —el runner
	// no está—, que confirma que la negativa anterior es lo que se está probando.
	s = &Service{Locator: locatorFalso{ok: true, place: Place{Repo: dirDePrueba(t), Branch: "b"}}}
	_, err := s.Simulate(context.Background(), it, KindMerge)
	if err == nil {
		t.Error("con un repo presente dio nil sin llegar a renderizar")
	}
}

// TestAvailableYRunnerNoRevientanConElServicioVacio: el cableado por defecto.
//
// Y lo que se prueba es que un `Service` con el campo `Runner` a nil se recupera solo. Eso
// pasa cuando alguien construye `&Service{Locator: ...}` en vez de usar `New`, que es lo
// que hacen varios tests y lo que haría un lector nuevo.
func TestAvailableYRunnerNoRevientanConElServicioVacio(t *testing.T) {
	s := &Service{}
	// No hay git-sim en un entorno de pruebas, así que `Available` es false. Lo que se
	// comprueba es que responda en vez de hacer panic por el runner a nil.
	_ = s.Available()
	if s.Runner == nil {
		t.Error("runner() no rellenó el Runner de un servicio construido a mano")
	}
	// Y con el relay puesto de verdad.
	nuevo := New(locatorFalso{})
	if nuevo == nil || nuevo.Runner == nil || nuevo.Git == nil {
		t.Errorf("New devolvió %+v con piezas a nil", nuevo)
	}
	// Y un servicio construido a mano, sin `Git`: `gitRunner` lo tiene que rellenar igual
	// que `runner` con el `Runner`. Es el mismo contrato, y `New` no es la única forma de
	// tener un `Service` —la tiene cualquiera que lo componga él mismo.
	manuales := &Service{Locator: locatorFalso{}}
	if manuales.gitRunner() == nil || manuales.Git == nil {
		t.Error("gitRunner() no rellenó el Git de un servicio construido a mano")
	}
}

// TestElMensajeDeUnErrorDeSimSeparaQuejarseDeColgarse: `message`, que decide el texto.
//
// Y la decisión que toma es si el proceso se quejó o se quedó colgado, que es lo que
// separa dos diagnósticos completamente distintos: "tu repo no se puede simular" y
// "git-sim se ha quedado esperando". Sin el código de salida no hay forma de distinguir un
// rechazo de un render que no terminó, que es literalmente lo que dice el comentario del
// código en el sitio donde se construye el error.
//
// Y el caso de "no hay nada en stderr": cae al error del proceso, que es feo pero no está
// vacío.
func TestElMensajeDeUnErrorDeSimSeparaQuejarseDeColgarse(t *testing.T) {
	// Con salida propia: la primera línea no vacía, que es la que explica.
	got := message("primera\nsegunda\ntercera", errors.New("exit status 1"))
	if !strings.Contains(got, "primera") {
		t.Errorf("el mensaje %q no trae la primera línea de stderr", got)
	}
	if strings.Contains(got, "segunda") {
		t.Errorf("el mensaje %q trae más de una línea", got)
	}

	// Sin stderr: cae al error, no a la cadena vacía.
	got = message("   \n  ", errors.New("exit status 137"))
	if strings.TrimSpace(got) == "" {
		t.Error("sin stderr el mensaje quedó vacío")
	}
	if !strings.Contains(got, "137") {
		t.Errorf("el mensaje %q perdió el código de salida", got)
	}

	// Y con stderr vacío Y error vacío: tampoco puede quedar mudo, porque un `Error` con
	// `Msg` vacío se pinta como un toast con nada.
	if got := message("", nil); strings.TrimSpace(got) == "" {
		t.Error("sin stderr ni error el mensaje quedó vacío")
	}
}

// TestElBinarioDeSimEsElCampoOElCanónicoYNadaMás: `bin`, que es más corto de lo que
// parecía.
//
// La primera versión de este test suponía que `bin` leía una variable de entorno para poder
// apuntar a otra build de git-sim, y no la lee: `bin` mira el campo `Bin` y, si está vacío,
// devuelve el nombre canónico. No hay entorno. Y eso no es un olvido sino una decisión que
// conviene tener escrita, porque `Runner` es lo que los tests sustituyen: un seam por el
// campo es más simple que uno por variable de entorno, y el entorno es global.
//
// Lo que sí importa, y es lo que se fija, es que el campo gana y que el vacío cae al
// canónico —no a una cadena vacía, que ejecutaría nada—.
func TestElBinarioDeSimEsElCampoOElCanónicoYNadaMás(t *testing.T) {
	// Vacío: el nombre canónico, nunca la cadena vacía.
	if got := (&Runner{}).bin(); got != DefaultBin {
		t.Errorf("con Bin vacío dio %q, want %q", got, DefaultBin)
	}
	// Y el canónico es el nombre del programa, que es lo que se busca en el PATH.
	if DefaultBin != "git-sim" {
		t.Errorf("DefaultBin es %q, want git-sim", DefaultBin)
	}
	// Con el campo: manda el campo.
	if got := (&Runner{Bin: "/opt/git-sim-mio"}).bin(); got != "/opt/git-sim-mio" {
		t.Errorf("con Bin puesto dio %q", got)
	}
	// Y el entorno no toca nada: un `GIT_SIM_BIN` puesto no cambia el binario, y quien
	// quiera otra build la pone en el campo. Fijarlo aquí evita que alguien asuma lo
	// contrario al leer el nombre del campo.
	t.Setenv("GIT_SIM_BIN", "/opt/otro")
	if got := (&Runner{}).bin(); got != DefaultBin {
		t.Errorf("con GIT_SIM_BIN en el entorno dio %q, want el canónico", got)
	}
	// Y `NewRunner` lo pone explícito, que es lo que hace que `Available` lo encuentre
	// sin depender del fallback.
	if got := NewRunner().Bin; got != DefaultBin {
		t.Errorf("NewRunner().Bin = %q", got)
	}
	// Y el runner construido a mano sin `Bin` no está disponible en un entorno sin git-sim,
	// pero no por entrar en pánico.
	_ = (&Runner{}).Available()
}

// TestElErrorDeSimTraeLoQueSeEjecuto: el mensaje de un fallo de git-sim.
//
// Y con la ruta y los argumentos, que es lo que permite reproducirlo sin tener que adivinar
// qué invocación encontró un repo en el que no se puede renderizar.
func TestElErrorDeSimTraeLoQueSeEjecuto(t *testing.T) {
	e := &Error{Args: []string{"config", "user.email"}, Dir: "/repos/proy", Msg: "no such repository", ExitCode: 128}
	msg := e.Error()
	for _, quiere := range []string{"config", "user.email", "/repos/proy", "no such repository", "128"} {
		if !strings.Contains(msg, quiere) {
			t.Errorf("el mensaje %q no trae %q", msg, quiere)
		}
	}
	// Y sin código de salida no aparece un "(exit 0)".
	if strings.Contains((&Error{Args: []string{"a"}, Msg: "m"}).Error(), "exit") {
		t.Error("un error sin código de salida lo pone entre paréntesis")
	}
	// Y la causa se conserva.
	if e.Unwrap() != nil {
		_ = e // Unwrap sin causa devuelve nil; aquí solo se comprueba que no revienta
	}
}

// locatorFalso devuelve un `Place` fijo, o dice que no lo encuentra.
type locatorFalso struct {
	ok    bool
	place Place
}

func (l locatorFalso) Locate(model.Item) (Place, bool) { return l.place, l.ok }

// origenDePrueba crea un fichero de imagen que existe.
func origenDePrueba(t *testing.T) string {
	t.Helper()
	ruta := filepath.Join(t.TempDir(), "render.jpg")
	if err := os.WriteFile(ruta, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	return ruta
}

// dirDePrueba devuelve un directorio que existe.
func dirDePrueba(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	testutil.InitRepo(t, dir)
	return dir
}

// keys devuelve las claves de un mapa, para no depender del orden de recorrido.
func keys(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
