package cache

import (
	"os"
	"path/filepath"
	"testing"
)

// Path and MemoPath were at 0%. They do nothing exotic: they join a cache subdirectory.

func TestLasRutasDeLaCacheCuelganDeXDGYNoDelPrograma(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", dir)

	cachePath, err := Path()
	if err != nil {
		t.Fatalf("Path: %v", err)
	}
	memoPath, err := MemoPath()
	if err != nil {
		t.Fatalf("MemoPath: %v", err)
	}

	for nombre, ruta := range map[string]string{"Path": cachePath, "MemoPath": memoPath} {
		wantDir := filepath.Join(dir, DirName)
		if filepath.Dir(ruta) != wantDir {
			t.Errorf("%s dio %q, que no cuelga de %q", nombre, ruta, wantDir)
		}
		if !filepath.IsAbs(ruta) {
			t.Errorf("%s dio una ruta relativa: %q", nombre, ruta)
		}
	}

	if cachePath == memoPath {
		t.Errorf("Path y MemoPath dieron el mismo fichero: %q", cachePath)
	}
	if filepath.Base(cachePath) != FileName {
		t.Errorf("Path dio el fichero %q, want %q", filepath.Base(cachePath), FileName)
	}
	if filepath.Base(memoPath) != MemoFileName {
		t.Errorf("MemoPath dio el fichero %q, want %q", filepath.Base(memoPath), MemoFileName)
	}
	if FileName == MemoFileName {
		t.Error("FileName y MemoFileName son iguales: los dos ficheros se pisan")
	}

	otra, err := Path()
	if err != nil {
		t.Fatal(err)
	}
	if otra != cachePath {
		t.Errorf("Path dio %q la primera vez y %q la segunda", cachePath, otra)
	}
}

// Path composes a path that does not exist yet, so this closes the round trip.
func TestGuardarYLeyersePisanElPropio(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", dir)

	cachePath, err := Path()
	if err != nil {
		t.Fatal(err)
	}
	memoPath, err := MemoPath()
	if err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(filepath.Dir(cachePath)); err == nil {
		t.Fatal("el directorio de la cache ya existía antes de guardar")
	}

	if err := Save(cachePath, File{}); err != nil {
		t.Fatalf("Save con la ruta de Path: %v", err)
	}
	if err := SaveMemo(memoPath, emptyMemo()); err != nil {
		t.Fatalf("SaveMemo con la ruta de MemoPath: %v", err)
	}
	if _, ok := Load(cachePath); !ok {
		t.Error("lo guardado con la ruta de Path no se lee")
	}
	if _, ok := LoadMemo(memoPath); !ok {
		t.Error("lo guardado con la ruta de MemoPath no se lee")
	}
	a, err := os.ReadFile(cachePath)
	if err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(memoPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(a) == string(b) {
		t.Error("los dos ficheros tienen el mismo contenido: uno se ha pisado al otro")
	}
}

// Without HOME or XDG there is no path, and it says so. That is a minimal container.
func TestSinVariablesDeEntornoLasRutasDegradan(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", "")
	t.Setenv("HOME", "")

	// The platform may still find somewhere, so this only says "fallback used".
	cachePath, cacheErr := Path()
	memoPath, memoErr := MemoPath()

	if cacheErr == nil && cachePath == "" {
		t.Error("Path dio ruta vacía sin error")
	}
	if memoErr == nil && memoPath == "" {
		t.Error("MemoPath dio ruta vacía sin error")
	}
	if cacheErr != nil && memoErr == nil {
		t.Errorf("Path falló pero MemoPath no: %v", cacheErr)
	}
	if cacheErr == nil {
		if filepath.Base(filepath.Dir(cachePath)) != DirName {
			t.Errorf("la ruta %q no lleva el subdirectorio del programa", cachePath)
		}
	}
	if memoErr == nil && cacheErr == nil && cachePath == memoPath {
		t.Error("sin entorno, las dos rutas colapsaron al mismo fichero")
	}
}
