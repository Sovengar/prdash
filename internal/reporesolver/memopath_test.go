package reporesolver

import (
	"os"
	"path/filepath"
	"testing"

	"prdash/internal/cache"
)

// The most obvious thing in the world and exactly what nobody tests.
func TestElMemoPathExplicitoManda(t *testing.T) {
	xdg := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", xdg)
	// HOME too, so reading the cache directory does not depend on which one the test sets.
	t.Setenv("HOME", t.TempDir())

	explicito := filepath.Join(t.TempDir(), "memo.json")
	porDefecto := filepath.Join(xdg, cache.DirName, cache.MemoFileName)

	r := New(Options{MemoPath: explicito})
	r.store.SetRoute("clave", "/ruta/explicita")

	if _, err := os.Stat(explicito); err != nil {
		t.Fatalf("con MemoPath explícito no se escribió en la ruta indicada: %v", err)
	}
	// And it was NOT written to the default path.
	if _, err := os.Stat(porDefecto); err == nil {
		t.Fatalf("con MemoPath explícito se escribió TAMBIÉN en la ruta por defecto (%s): "+
			"la ruta indicada se está ignorando", porDefecto)
	}
}

func TestSinMemoPathSeUsaElPorDefecto(t *testing.T) {
	xdg := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", xdg)
	t.Setenv("HOME", t.TempDir())

	porDefecto := filepath.Join(xdg, cache.DirName, cache.MemoFileName)

	// Written with one resolver and read with another.
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

// The default path carries the product's subdirectory.
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
	enHome := filepath.Join(os.Getenv("HOME"), cache.MemoFileName)
	if _, err := os.Stat(enHome); err == nil {
		t.Errorf("la memoria se escribió también en el HOME (%s): dos programas se pisan", enHome)
	}
}

// If the cache directory cannot be determined, New does not fail: the whole repo is built without
// memo persistence.
func TestUnaRutaPorDefectoIlegibleNoRompeNew(t *testing.T) {
	// Sin XDG_CACHE_HOME ni HOME, el directorio de cache no se puede averiguar.
	t.Setenv("XDG_CACHE_HOME", "")
	t.Setenv("HOME", "")

	r := New(Options{})
	if r == nil || r.store == nil {
		t.Fatal("New devolvió nil con el cache ilegible: config que no degrada a default")
	}
	r.store.SetRoute("k", "/r")
	if ruta, ok := r.store.Route("k"); !ok || ruta != "/r" {
		t.Errorf("el store en memoria no sirve: ok=%v ruta=%q", ok, ruta)
	}
}
