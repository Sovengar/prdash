package reporesolver

import (
	"os"
	"path/filepath"
	"testing"

	"prdash/internal/cache"
)

// TestElMemoPathExplicitoManda: si se dice dónde está la memoria, ahí está.
//
// Es lo más tonto del mundo y es exactamente lo que no se probaba: todos los tests
// de este paquete pasaban un MemoPath explícito, así que la mitad de la función New
// —la que decide la ruta por defecto— no se ejecutaba nunca. Y la ruta por defecto no
// es decorativa: es la que decide si la memoria de rutas y de reviews sobrevive entre
// ejecuciones.
//
// Se comprueba por el fichero, no por el store: el store no expone su ruta, pero
// escribe al guardar, y dónde aparece el fichero SÍ es observable desde fuera.
func TestElMemoPathExplicitoManda(t *testing.T) {
	xdg := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", xdg)
	// HOME también, para que la lectura del directorio de cache no dependa de cuál
	// de las dos variables cogiera en esta máquina.
	t.Setenv("HOME", t.TempDir())

	explicito := filepath.Join(t.TempDir(), "memo.json")
	porDefecto := filepath.Join(xdg, cache.DirName, cache.MemoFileName)

	r := New(Options{MemoPath: explicito})
	r.store.SetRoute("clave", "/ruta/explicita")

	if _, err := os.Stat(explicito); err != nil {
		t.Fatalf("con MemoPath explícito no se escribió en la ruta indicada: %v", err)
	}
	// Y NO se escribió en la de por defecto. Un "!= \"\"" en la condición de la ruta
	// vacía haría justo esto: cargaría la de por defecto y perdería la que le dijeron.
	if _, err := os.Stat(porDefecto); err == nil {
		t.Fatalf("con MemoPath explícito se escribió TAMBIÉN en la ruta por defecto (%s): "+
			"la ruta indicada se está ignorando", porDefecto)
	}
}

// TestSinMemoPathSeUsaElPorDefecto: sin ruta indicada, la memoria vive en el
// directorio de cache del usuario, y SOBREVIVE a que se abra otro resolver.
//
// Esto es lo que hace que prdash recuerde dónde está el repo de un ítem entre
// ejecuciones, así que no es un detalle de configuración.
func TestSinMemoPathSeUsaElPorDefecto(t *testing.T) {
	xdg := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", xdg)
	t.Setenv("HOME", t.TempDir())

	porDefecto := filepath.Join(xdg, cache.DirName, cache.MemoFileName)

	// Se escribe una ruta con un resolver, y se lee con otro. Si la ruta por defecto
	// no se usara, el segundo resolver no sabría nada.
	r := New(Options{})
	r.store.SetRoute("clave", "/ruta/recordada")

	if _, err := os.Stat(porDefecto); err != nil {
		t.Fatalf("sin MemoPath no se escribió en la ruta por defecto (%s): %v", porDefecto, err)
	}

	otro := New(Options{})
	if ruta, ok := otro.store.Route("clave"); !ok || ruta != "/ruta/recordada" {
		t.Errorf("un resolver nuevo no encontró la ruta recordada (ok=%v, ruta=%q): "+
			"la memoria no está en la ruta por defecto", ok, ruta)
	}
}

// TestLaRutaPorDefectoEsLaDelCacheYNoOtra: la ruta por defecto es la del directorio de
// cache del usuario, con el subdirectorio del producto dentro.
//
// No es un capricho: si se guardara en el HOME a secas, dos programas que guarden
// un fichero con el mismo nombre en el HOME se pisan. Y si se guardara junto al
// ejecutable, sería de solo lectura en la mayoría de las instalaciones.
func TestLaRutaPorDefectoEsLaDelCacheYNoOtra(t *testing.T) {
	xdg := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", xdg)
	t.Setenv("HOME", t.TempDir())

	r := New(Options{})
	r.store.SetRoute("k", "/r")

	quiere := filepath.Join(xdg, cache.DirName, cache.MemoFileName)
	if _, err := os.Stat(quiere); err != nil {
		t.Errorf("la memoria no está en %s: %v", quiere, err)
	}
	// Y el subdirectorio es el del producto, no el HOME a secas.
	enHome := filepath.Join(os.Getenv("HOME"), cache.MemoFileName)
	if _, err := os.Stat(enHome); err == nil {
		t.Errorf("la memoria se escribió también en el HOME (%s): dos programas se pisan", enHome)
	}
}

// TestUnaRutaPorDefectoIlegibleNoRompeNew: si el directorio de cache no se puede
// averiguar, New no falla.
//
// El repo entero se construye sobre la regla de que config y entorno degradan a
// defaults con un aviso, y no abortan. Una ruta de memoria ilegible es memoria
// vacía: se pierde lo que se hubiera recordado, y el resolver sigue funcionando. La
// alternativa —un error que se propaga— dejaría a prdash sin arrancar en una máquina
// donde no se puede escribir en el cache, que es un sitio perfectamente normal.
func TestUnaRutaPorDefectoIlegibleNoRompeNew(t *testing.T) {
	// Sin XDG_CACHE_HOME ni HOME, el directorio de cache no se puede averiguar.
	t.Setenv("XDG_CACHE_HOME", "")
	t.Setenv("HOME", "")

	r := New(Options{})
	if r == nil || r.store == nil {
		t.Fatal("New devolvió nil con el cache ilegible: config que no degrada a default")
	}
	// Y el store sirve, en memoria: se puede escribir y leer, aunque no se guarde.
	r.store.SetRoute("k", "/r")
	if ruta, ok := r.store.Route("k"); !ok || ruta != "/r" {
		t.Errorf("el store en memoria no sirve: ok=%v ruta=%q", ok, ruta)
	}
}
