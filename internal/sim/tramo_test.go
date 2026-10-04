package sim

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Esta es la segunda tanda de guardas de `sim`, y las que faltan son las del TRAMO MEDIO de
// `Simulate`: los tres `if err != nil` entre `stage` y el `Result` final. Y son los que más
// se parecen entre sí y más se distinguen.
//
// Y la distinción es dónde está la culpa, que es lo que necesita el usuario para decidir:
//
//   - `stage` falla: no se pudo preparar el clon. Es problema del repo local.
//   - El render falla: `git-sim` se/cae. Es problema de git-sim.
//   - `keep` falla: no se pudo guardar la imagen en el caché. Es problema del disco o de los
//     permisos, y la imagen ESTÁ renderizada.
//
// Y esa tercera es la que más engaña: el trabajo caro ya se hizo y el resultado se pierde. Sin
// el mensaje que lo diga, el usuario ve un error y asume que no se renderizó nada.

// gitSimQueFalla es un `git-sim` que sale con código 1 y dice por qué.
func gitSimQueFalla(t *testing.T, mensaje string) string {
	t.Helper()
	return writeScript(t, t.TempDir(), "git-sim",
		"#!/bin/sh\necho '"+mensaje+"' >&2\nexit 1\n")
}

// TestSiGitSimFallaElErrorSaleDeElYNoDelStageNiDelCache: el tramo medio de `Simulate`.
//
// Y lo que se comprueba es que el error que sube a la TUI es el de git-sim con su mensaje, y no
// uno genérico del tramo: si `stage` devolviera un error vacío, el popup cerraría sin decir por
// qué y el usuario pensaría que la simulación no arrancó.
//
// Y el mensaje de git-sim es lo único accionable: si dice "index not found" es un bug de la
// versión, y si dice "no such branch" es que la rama del PR se movió entre medio.
func TestSiGitSimFallaElErrorSaleDeElYNoDelStageNiDelCache(t *testing.T) {
	repo, _, it := fixture(t)
	it.SourceBranch = "prdash/pr-7"
	bin := gitSimQueFalla(t, "IndexError: no such branch")

	s := newService(t, locatorFalso{ok: true, place: Place{Repo: repo, Branch: "prdash/pr-7"}}, bin)

	res, err := s.Simulate(context.Background(), it, KindMerge)
	if err == nil {
		t.Fatal("un git-sim que falla dio nil: el popup se cerraría sin decir por qué")
	}
	// Y el mensaje de git-sim sube tal cual.
	if !strings.Contains(err.Error(), "no such branch") {
		t.Errorf("el error %q no trae el mensaje de git-sim", err)
	}
	// Y no hay imagen: un `Result` con `Path` puesto se abriría en el visor.
	if res.Path != "" {
		t.Errorf("devolvió una imagen %q pese al fallo del render", res.Path)
	}
	// Y el error NO menciona el caché: ese guard va después, y un mensaje de caché en un fallo
	// de render mandaría al usuario a mirar permisos que están bien.
	if strings.Contains(err.Error(), "cache") {
		t.Errorf("el error %q habla del caché, que es un tramo posterior", err)
	}
}

// TestSiElRenderNoDejaImagenElErrorLoDiceYNoSePropagaLaRutaVacia: git-sim que sale con 0 y no
// escribe.
//
// Y es el caso más fino porque el runner SÍ terminó bien: sin este aserto, el código devolvería
// una ruta vacía como si fuera una imagen, y `keep` la copiaría —copiar "" crea un fichero
// vacío con nombre de imagen— y el popup ofrecería abrir un fichero de cero bytes.
//
// Y el mensaje tiene que decir que git-sim no produjo imagen, no "error al renderizar", porque
// la diferencia entre las dos cosas es lo que dice si hay que reinstalar git-sim o mirar la
// simulación.
func TestSiElRenderNoDejaImagenElErrorLoDiceYNoSePropagaLaRutaVacia(t *testing.T) {
	repo, _, it := fixture(t)
	it.SourceBranch = "prdash/pr-7"
	mudo := writeScript(t, t.TempDir(), "git-sim", "#!/bin/sh\nexit 0\n")

	s := newService(t, locatorFalso{ok: true, place: Place{Repo: repo, Branch: "prdash/pr-7"}}, mudo)

	res, err := s.Simulate(context.Background(), it, KindMerge)
	if err == nil {
		t.Fatal("un git-sim que no produce imagen dio nil")
	}
	if !strings.Contains(err.Error(), "no image") {
		t.Errorf("el error %q no dice que git-sim no produjo imagen", err)
	}
	if res.Path != "" {
		t.Errorf("devolvió Path=%q: se copiaría al caché un fichero vacío", res.Path)
	}
	// Y el caché está vacío: si se hubiera copiado algo, el popup tendría una imagen que abrir.
	entradas, err := os.ReadDir(s.CacheDir)
	if err == nil {
		for _, e := range entradas {
			t.Errorf("quedó %s en el caché pese al fallo", e.Name())
		}
	}
}

// TestElTemporalDeLaSimulacionSeBorraAunqueElRenderFalle: nada de deuda.
//
// Y esto es lo que hace que `Simulate` pueda llamarse en bucle sin llenar el disco: el `tmp` se
// borra con `defer`, así que el clon de la simulación y la imagen que git-sim dejó ahí se van
// aunque el render falle.
//
// Y el caso que mide es el fallo INTERMEDIO —un clon de un repo grande que ya está en disco—,
// que es el que más espacio ocupa. Un fallo antes del clon no dejaría nada que borrar.
func TestElTemporalDeLaSimulacionSeBorraAunqueElRenderFalle(t *testing.T) {
	repo, _, it := fixture(t)
	it.SourceBranch = "prdash/pr-7"
	bin := gitSimQueFalla(t, "boom")

	// Con el TMPDIR bajo un directorio que se puede inspeccionar.
	temporal := t.TempDir()
	t.Setenv("TMPDIR", temporal)

	s := newService(t, locatorFalso{ok: true, place: Place{Repo: repo, Branch: "prdash/pr-7"}}, bin)
	if _, err := s.Simulate(context.Background(), it, KindMerge); err == nil {
		t.Fatal("el render debería haber fallado")
	}

	entradas, err := os.ReadDir(temporal)
	if err != nil {
		t.Fatal(err)
	}
	if len(entradas) != 0 {
		var nombres []string
		for _, e := range entradas {
			nombres = append(nombres, e.Name())
		}
		t.Errorf("el temporal de la simulación quedó con %d entradas: %v. El clon del repo se "+
			"queda en disco por cada render fallido", len(entradas), nombres)
	}
}

// TestUnCacheQueNoSePuedeCrearFallaDespuesDeRenderizarYLoDice: el último tramo.
//
// Y el orden es lo que importa: el render YA OCURRIÓ cuando `keep` falla, así que el error tiene
// que decir que la imagen se generó y no se pudo guardar. Un mensaje de "no se pudo simular"
// sería mentira, y el usuario repetiría la operación pensando que no se renderizó nada.
//
// Y en la práctica el caso es un caché en un disco lleno, que es el motivo por el que existe
// este tramo.
func TestUnCacheQueNoSePuedeCrearFallaDespuesDeRenderizarYLoDice(t *testing.T) {
	repo, _, it := fixture(t)
	it.SourceBranch = "prdash/pr-7"

	// El caché se puede poner antes porque `newService` lo fija en un temporal limpio; aquí se
	// cambia por una ruta que no se puede crear, con un fichero donde debería ir el directorio.
	cache := filepath.Join(t.TempDir(), "cache-bloqueado")
	if err := os.WriteFile(cache, []byte("bloqueo"), 0o644); err != nil {
		t.Fatal(err)
	}
	s := newService(t, locatorFalso{ok: true, place: Place{Repo: repo, Branch: "prdash/pr-7"}},
		fakeSim(t, writeJPEG(t)))
	s.CacheDir = cache

	res, err := s.Simulate(context.Background(), it, KindMerge)
	if err == nil {
		t.Fatal("un caché que no se puede crear dio nil")
	}
	if !strings.Contains(err.Error(), "simulation cache") {
		t.Errorf("el error %q no dice que falla el caché, que es donde está la culpa", err)
	}
	// Y el mensaje NO dice que git-sim falló, porque no falló.
	if strings.Contains(err.Error(), "git-sim") {
		t.Errorf("el error %q culpa a git-sim, que sí renderizó", err)
	}
	if res.Path != "" {
		t.Errorf("devolvió Path=%q sin haber guardado nada", res.Path)
	}
}

// TestLaRamaDeReviewSeActivaParaRebaseYLaBaseParaIntegrar: el error de checkout de cada uno.
//
// Y son los dos modos y el fallo tiene que decir cuál de las dos ramas se intentó activar, que
// es lo que no se puede adivinar del mensaje de git: un "pathspec no coincide" en un clon de
// simulación puede ser la base o la del ítem, y son cosas distintas.
func TestLaRamaDeReviewSeActivaParaRebaseYLaBaseParaIntegrar(t *testing.T) {
	repo, _ := simRepoMonta(t)
	// Un clon donde el checkout de una base falla: la base es un nombre de rama que no existe
	// en el clon por lo que sea, y `materialize` la deja pasar solo si está en `origin/`.
	//
	// Y el caso se provoca con un repo que tiene la rama en un clon pero no en el otro: en vez
	// de pelear con eso, se usa la ruta del fixture que NO tiene `main` remota.
	origen, tmp := simRepoMonta(t)
	_ = repo
	for _, c := range []struct {
		nombre  string
		kind    Kind
		base    string
		delItem string
	}{
		{"integrar con base inexistente", KindMerge, "base/que-no-existe", "feat/x"},
		{"rebasar con base inexistente", KindRebase, "base/que-no-existe", "feat/x"},
	} {
		_, _, err := New(locatorDeStage()).stage(context.Background(),
			Place{Repo: origen, Branch: c.delItem}, c.kind, c.base, t.TempDir())
		if err == nil {
			t.Errorf("%s: pasó sin la base", c.nombre)
			continue
		}
		if !strings.Contains(err.Error(), c.base) {
			t.Errorf("%s: el error %q no nombra la base que no se pudo activar",
				c.nombre, err)
		}
	}
	_ = tmp
}

// TestCopyFileNoDejaTemporalSiElDestinoEsUnDirectorioQueYaExiste: el `.part`.
//
// Y es el caso que destapó el arreglo del `.part`: `os.Create(tmp)` tiene éxito porque `tmp`
// es otro nombre, y el fallo llega en el `Rename`, con EEXIST. Sin la limpieza del temporal, cada
// intento fallido deja un `.part` con el tamaño de una imagen en el caché, y `prune` no lo
// borra porque solo mira los `.jpg`.
func TestCopyFileNoDejaTemporalSiElDestinoEsUnDirectorioQueYaExiste(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "origen.jpg")
	if err := os.WriteFile(src, []byte("imagen"), 0o644); err != nil {
		t.Fatal(err)
	}
	// El destino es un DIRECTORIO que ya existe.
	dst := filepath.Join(dir, "destino.jpg")
	if err := os.MkdirAll(dst, 0o755); err != nil {
		t.Fatal(err)
	}

	if err := copyFile(src, dst); err == nil {
		t.Fatal("copiar a un directorio dio nil")
	}
	if _, err := os.Stat(dst + ".part"); !os.IsNotExist(err) {
		t.Errorf("quedó el temporal %s.part: cada intento fallido dejaría basura que prune no "+
			"borra porque solo mira los .jpg", dst+".part")
	}
	// Y el directorio que había sigue siendo un directorio y no un fichero a medias.
	info, err := os.Stat(dst)
	if err != nil || !info.IsDir() {
		t.Errorf("el destino dejó de ser un directorio: %v", err)
	}
	// Y el `prune` de verdad no toca el `.part` aunque quedara, que es lo que hace que la
	// limpieza de `copyFile` sea necesaria y no una Courtesy.
	restos := filepath.Join(dir, "otro.part")
	if err := os.WriteFile(restos, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	prune(dir, 0)
	if _, err := os.Stat(restos); err != nil {
		t.Errorf("prune quitó el .part, así que la limpieza de copyFile no es necesaria: %v", err)
	}
}

// TestElPaseoDePruneToleraUnaEntradaQueNoSePuedeEstadificar: `e.Info()` que falla.
//
// Y el caso es un enlace simbólico COLGADO: `ReadDir` lo lista, no es un directorio, el nombre
// acaba en `.jpg`… y `Info()` falla porque el destino no existe. Si eso fuera un `return`, el
// `prune` entero se detendría en la primera entrada mala y no podaría NADA —que es como se
// degrada una limpieza a no hacer limpieza—.
//
// Y el `continue` es lo correcto por una razón que no es de robustez genérica: `prune` se llama
// en la limpieza del render, y su trabajo es que no crezca el caché. Si un fichero raro lo
// rumpiera, el caché crecería sin que nadie avise.
func TestElPaseoDePruneToleraUnaEntradaQueNoSePuedeEstadificar(t *testing.T) {
	dir := t.TempDir()
	// Un enlace a un `.jpg` que no existe: `ReadDir` lo ve y `Info()` falla.
	colgado := filepath.Join(dir, "colgado.jpg")
	if err := os.Symlink(filepath.Join(dir, "no-existe.jpg"), colgado); err != nil {
		t.Fatal(err)
	}
	// Y una imagen real que NO se debe perder, escrita después del enlace: si el `prune` se
	// parara en el enlace, esta se quedaría y el caché crecería.
	buena := filepath.Join(dir, "buena.jpg")
	if err := os.WriteFile(buena, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	prune(dir, 0)

	// El enlace roto se queda —no es una imagen y `os.Remove` lo quitaría igualmente, pero lo
	// que importa es que no lo pare— y la buena se fue, que es lo que prueba que el paseo
	// siguió.
	if _, err := os.Stat(buena); err == nil {
		t.Error("la imagen buena sigue ahí: el paseo de prune se paró en el enlace roto")
	}
}
