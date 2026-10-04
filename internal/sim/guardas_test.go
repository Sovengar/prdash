package sim

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"prdash/internal/forge/model"
)

// Estos son los fallos de INFRAESTRUCTURA de una simulación: el sitio donde no hay disco, no
// hay HOME, el repo ya no está, la rama no existe en el clon. Ninguno se puede provocar con un
// doble de `os` o de `exec`, y los tres que más daño hacen son los que dejan al usuario
// mirando un popup que no avanza.
//
// Y el patrón de todos es el mismo: **una cadena de `if err != nil` que devuelve el error y
// nada más**. La tentación es tratarlos como formalidad —el error se vería igual— y el precio
// es que ninguno está probado hasta que pasa en producción, que es cuando el disco está lleno
// y el usuario necesita que el error diga qué hacer.
//
// Y el entorno se manipula de verdad: `TMPDIR` apuntado a un fichero, `XDG_CACHE_HOME` sin
// valor, un repo que se borra por debajo. Todo eso lo produce el SO.

// bloqueaConUnFichero deja un fichero donde se esperaba un directorio, que es la forma de
// negar una escritura que funciona como root y como usuario.
//
// Y el modo 000 no sirve: en un contenedor los tests corren a menudo como root, y root escribe
// en cualquier sitio, así que el guard no se dispararía y el test pasaría sin probar nada. La
// forma no depende del usuario.
func bloqueaConUnFichero(t *testing.T, ruta string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(ruta), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(ruta, []byte("bloqueo"), 0o644); err != nil {
		t.Fatal(err)
	}
}

// TestSinCacheDirNoSePuedeSimularYElErrorLoDice: la primera guarda, la del entorno.
//
// Y `os.UserCacheDir` falla cuando no hay ni `XDG_CACHE_HOME` ni `HOME`, que es lo que pasa en
// un contenedor sinVariables de entorno y en una sesión de servicio sin HOME.
//
// Y la cadena completa importa: `DefaultCacheDir` falla → `cacheDir` lo propaga → `keep` lo
// envuelve con "locate the simulation cache" → `Simulate` lo devuelve. Cada capa añade el suyo
// y el usuario lee una cadena que dice dónde se rompió, no solo que se rompió algo.
func TestSinCacheDirNoSePuedeSimularYElErrorLoDice(t *testing.T) {
	// Las dos vacías: es el caso real de un contenedor. Poner solo una no vale, porque
	// `os.UserCacheDir` mira la otra como reserva y el test pasaría sin disparar el guard.
	t.Setenv("XDG_CACHE_HOME", "")
	t.Setenv("HOME", "")

	if _, err := DefaultCacheDir(); err == nil {
		t.Fatal("DefaultCacheDir sin HOME ni XDG_CACHE_HOME dio nil")
	}

	// Y `cacheDir` sin `CacheDir` puesto propaga ese error en vez de inventarse una ruta.
	s := &Service{}
	if _, err := s.cacheDir(); err == nil {
		t.Error("cacheDir sin CacheDir ni entorno dio nil")
	}

	// Y el camino bueno: con `XDG_CACHE_HOME` puesto, sale la ruta de debajo. Esto es lo que
	// evita que "siempre falla" cuente como prueba de la guarda.
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	dir, err := DefaultCacheDir()
	if err != nil {
		t.Fatalf("DefaultCacheDir con entorno: %v", err)
	}
	if filepath.Base(dir) != "sim" {
		t.Errorf("DefaultCacheDir dio %q, want .../prdash/sim", dir)
	}
}

// TestSinTMPDIRNoSePreparaElDirectorioDeLaSimulacion: la segunda guarda.
//
// Y esta es la que se dispara en un sistema donde `/tmp` está lleno o montado en solo lectura
// —un contenedor con el tmpfs al límite es el caso típico—. Y el mensaje tiene que decir que
// es el DIRECTORIO DE TRABAJO, porque hay tres temporales en el camino —el de la simulación,
// el del clon y el de las imágenes— y saber cuál falló dice si esperar o si no hay nada que
// esperar.
func TestSinTMPDIRNoSePreparaElDirectorioDeLaSimulacion(t *testing.T) {
	bloqueaConUnFichero(t, filepath.Join(t.TempDir(), "tmp-bloqueado"))
	t.Setenv("TMPDIR", filepath.Join(t.TempDir(), "tmp-bloqueado"))

	repo, review, it := fixture(t)
	it.SourceBranch = "prdash/pr-7"
	s := newService(t, locatorFalso{ok: true, place: Place{Repo: repo, Branch: "prdash/pr-7"}},
		fakeSim(t, writeJPEG(t)))

	_, err := s.Simulate(context.Background(), it, KindMerge)
	if err == nil {
		t.Fatal("sin TMPDIR utilizable se simuló igualmente")
	}
	if !strings.Contains(err.Error(), "simulation directory") {
		t.Errorf("el error %q no dice que falla el directorio de la simulación: hay tres "+
			"temporales en el camino y saber cuál es dice si hay que esperar", err)
	}
	_ = review
}

// TestUnRepoQueYaNoEstaFallaAlClonarYNoAlCheckout: los fallos de git en el staging.
//
// Y el orden es lo que se fija: primero se clona, y un repo que ya no está falla ahí. Es el
// caso real de un clon local que otro proceso borró —la limpieza de `prdash worktrees remove`
// lo hace— entre que el popup se abre y que se pulsa enter.
//
// Y lo que se comprueba es que el error nombra el repo. "exit 128" sin más no dice si hay que
// re-montar el review o si el disco está lleno, y esas dos cosas llevan a acciones distintas.
func TestUnRepoQueYaNoEstaFallaAlClonarYNoAlCheckout(t *testing.T) {
	// El repo que `Locator` da no existe. Es más fuerte que borrarlo después: desde el
	// principio no hay nada que clonar.
	s := newService(t, locatorFalso{ok: true, place: Place{
		Repo: filepath.Join(t.TempDir(), "repo-que-no-existe"), Branch: "prdash/pr-7",
	}}, fakeSim(t, writeJPEG(t)))

	_, _, err := s.stage(context.Background(), s.locatorPlace(), KindMerge, "main", t.TempDir())
	if err == nil {
		t.Fatal("se clonó un repo que no existe")
	}
	if !strings.Contains(err.Error(), "clone") {
		t.Errorf("el error %q no dice que falla el clon", err)
	}
	if !strings.Contains(err.Error(), "repo-que-no-existe") {
		t.Errorf("el error %q no nombra el repo que no se pudo clonar", err)
	}
}

// TestUnaRamaQueNoEstaEnElClonFallaAlMaterializar: la guarda de `materialize`.
//
// Y la asimetría con el clon es lo que importa: el clon puede ser un `--shared` de un repo que
// sí tiene la rama pero cuyo HEAD no es la del ítem, así que `git clone` solo trae `main` en
// local. `materialize` la crea desde `origin/`, y si tampoco está ahí, es que la rama no existe
// de verdad.
//
// Y el error tiene que NOMBRAR la rama: sin ella, un popup que dice "the branch does not
// exist in the local clone" deja a quien lee sin saber si es la base o la del ítem, y son
// dos cosas que se arreglan distinto.
func TestUnaRamaQueNoEstaEnElClonFallaAlMaterializar(t *testing.T) {
	repo, _ := simRepoMonta(t)

	for _, c := range []struct {
		nombre  string
		base    string
		delItem string
		falta   string
	}{
		// Y en los dos casos falta UNA de las dos y la otra está bien, para que el error
		// tenga que ser el de la que falta y no el de cualquiera. La primera versión tenía
		// las columnas cruzadas y pedía que el error nombrara la rama del ítem cuando lo que
		// faltaba era la base: el código hacía bien y el test estaba mal.
		{"falta la rama del ítem", "main", "feat/inexistente", "feat/inexistente"},
		{"falta la base", "base/inexistente", "feat/x", "base/inexistente"},
	} {
		// Cada caso con su temporal: stage clona DENTRO de tmp, y el clon del primer caso
		// se queda ahí, así que el segundo falla con "destination path already exists" en
		// vez de con el motivo que se quiere medir.
		_, _, err := New(locatorDeStage()).stage(context.Background(),
			Place{Repo: repo, Branch: c.delItem}, KindMerge, c.base, t.TempDir())
		if err == nil {
			t.Errorf("%s: pasó sin la rama", c.nombre)
			continue
		}
		if !strings.Contains(err.Error(), c.falta) {
			t.Errorf("%s: el error %q no nombra la ref que falta (%q)",
				c.nombre, err, c.falta)
		}
	}

	// Y el caso bueno, porque si no "siempre falla" no prueba que la guarda sea la que
	// dispara: una rama que sí está no debe fallar.
	if _, _, err := New(locatorDeStage()).stage(context.Background(),
		Place{Repo: repo, Branch: "feat/x"}, KindMerge, "main", t.TempDir()); err != nil {
		t.Errorf("una rama que sí existe dio error: %v", err)
	}
}

// TestKeepPropagaElFalloDeCrearElCacheYNoDejaNada: la tercera guarda.
//
// Y el caso es un caché que no se puede crear porque su padre es un fichero. Y lo que se
// comprueba además es que NO se crea un directorio a medias, porque un caché a medias con
// imágenes dentro hace que el siguiente intento encuentre un directorio existente y_directories
// que no puede escribir, y el error del segundo intento no diría nada del primero.
func TestKeepPropagaElFalloDeCrearElCacheYNoDejaNada(t *testing.T) {
	cache := filepath.Join(t.TempDir(), "cache-bloqueado")
	bloqueaConUnFichero(t, cache)

	s := &Service{CacheDir: cache}
	_, err := s.keep(writeJPEG(t), itemConRama("prdash/pr-7"), KindMerge)
	if err == nil {
		t.Fatal("un caché que no se puede crear dio nil")
	}
	if !strings.Contains(err.Error(), "simulation cache") {
		t.Errorf("el error %q no dice que falla el caché", err)
	}
	info, statErr := os.Stat(cache)
	if statErr == nil && info.IsDir() {
		t.Error("se creó el caché a medias: el siguiente intento lo encontraría existente")
	}
}

// TestGitSimQueNoProduceImagenLoDice: el runner, y el final de la cadena.
//
// Y es un fallo que se cuela fácil porque git-sim puede salir con 0 y no haber escrito nada: un
// wrapper que solo aceptara la imagen si el runner terminó bien se tragaría el stdout vacío y
// devolvería una ruta vacía, que luego `keep` copiaría como si fuera un fichero.
//
// Y el mensaje tiene que decir que no produjo imagen y no ser un error vacío: "no hay nada que
// abrir" sin más no distingue de un bug.
func TestGitSimQueNoProduceImagenLoDice(t *testing.T) {
	// Un git-sim que sale con 0 y no imprime nada.
	mudo := writeScript(t, t.TempDir(), "git-sim", "#!/bin/sh\nexit 0\n")

	r := &Runner{Bin: mudo}
	_, err := r.Render(context.Background(), t.TempDir(), t.TempDir(), Spec{Kind: KindMerge, Ref: "main"})
	if err == nil {
		t.Fatal("un git-sim que no produce imagen dio nil")
	}
	if !strings.Contains(err.Error(), "no image") {
		t.Errorf("el error %q no dice que git-sim no produjo imagen", err)
	}
	// Y el nombre del binario está: un error sin el comando que lo causó obliga a ir a
	// buscar cuál de los dos es.
	if !strings.Contains(err.Error(), "git-sim") {
		t.Errorf("el error %q no nombra git-sim", err)
	}
}

// TestSimulatePropagaElFalloDelStageYDiceQueVinoDeAhí: la cadena entera, sin mocks.
//
// Y es el test que cierra el círculo: un clon que falla tiene que salir como error de
// `Simulate` con el nombre del repo, no como un error de render ni como un popup en blanco.
//
// Y el popup en blanco es el desenlace que importa, porque `Simulate` se llama desde una
// goroutine que publica el resultado por el canal de eventos: si devolviera un `Result` vacío
// sin error, el popup se abriría sobre una imagen que no existe.
func TestSimulatePropagaElFalloDelStageYDiceQueVinoDeAhí(t *testing.T) {
	noExiste := filepath.Join(t.TempDir(), "repo-que-no-existe")
	s := newService(t, locatorFalso{ok: true, place: Place{Repo: noExiste, Branch: "prdash/pr-7"}},
		fakeSim(t, writeJPEG(t)))

	res, err := s.Simulate(context.Background(), itemConRama("prdash/pr-7"), KindMerge)
	if err == nil {
		t.Fatal("Simulate con un repo inexistente dio nil")
	}
	// Y el resultado vacío: un `Result` con `Path` puesto y la imagen inexistente se abriría
	// en el visor como un fichero que no está.
	if res.Path != "" {
		t.Errorf("devolvió una imagen %q pese al fallo", res.Path)
	}
	if !strings.Contains(err.Error(), noExiste) {
		t.Errorf("el error %q no nombra el repo que falló", err)
	}
}

// locatorPlace devuelve el `Place` que el locator del servicio da, para llamar a `stage` con
// los mismos datos que usaría `Simulate`.
func (s *Service) locatorPlace() Place {
	if l, ok := s.Locator.(locatorFalso); ok {
		return l.place
	}
	return Place{}
}

// itemConRama es un ítem con la rama del review puesta, que es lo que `stage` necesita para no
// avisar de que falta montar el review.
func itemConRama(branch string) model.Item {
	it := model.NewItem(model.RepoRef{
		Forge: "github", Host: "github.com", Project: "acme/widget",
	}, 7)
	it.SourceBranch = branch
	it.TargetBranch = "main"
	return it
}
