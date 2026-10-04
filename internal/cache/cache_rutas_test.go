package cache

import (
	"os"
	"path/filepath"
	"testing"
)

// `Path` y `MemoPath` son las dos rutas de la cache, y las dos estaban al 0%. No es que
// hagan nada raro: componen un directorio de la cache del usuario con un subdirectorio del
// programa. Lo que se fija es DÓNDE, porque de eso dependen dos decisiones que no se ven:
//
//   - La de aislamiento. Si la ruta no cuelga de `$XDG_CACHE_HOME`, un test lee la cache
//     de otro y el fallo aparece en el test equivocado.
//   - La de no pisar la cache de otro programa. Un `prdash` en la raíz del directorio de
//     cache se mezcla con lo que otro haya puesto ahí, y un `git prune` ajeno se lleva los
//     snapshots por delante.
//
// Y el caso que más importa es el de las dos funciones a la vez: `cache.json` y el memo no
// pueden acabar en el mismo fichero, o el segundo guardado pisa el primero.

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

	// Las dos cuelgan de XDG_CACHE_HOME, con el subdirectorio del programa en medio.
	for nombre, ruta := range map[string]string{"Path": cachePath, "MemoPath": memoPath} {
		wantDir := filepath.Join(dir, DirName)
		if filepath.Dir(ruta) != wantDir {
			t.Errorf("%s dio %q, que no cuelga de %q", nombre, ruta, wantDir)
		}
		if !filepath.IsAbs(ruta) {
			t.Errorf("%s dio una ruta relativa: %q", nombre, ruta)
		}
	}

	// Y los dos ficheros son distintos, que es lo que impide que el memo pise el snapshot.
	if cachePath == memoPath {
		t.Errorf("Path y MemoPath dieron el mismo fichero: %q", cachePath)
	}
	// Y cada uno con su nombre, que es lo que los diferencia dentro del directorio.
	if filepath.Base(cachePath) != FileName {
		t.Errorf("Path dio el fichero %q, want %q", filepath.Base(cachePath), FileName)
	}
	if filepath.Base(memoPath) != MemoFileName {
		t.Errorf("MemoPath dio el fichero %q, want %q", filepath.Base(memoPath), MemoFileName)
	}
	if FileName == MemoFileName {
		t.Error("FileName y MemoFileName son iguales: los dos ficheros se pisan")
	}

	// Y la ruta es estable entre llamadas, para que un test que la guarda y la vuelve a
	// pedir llegue al mismo sitio.
	otra, err := Path()
	if err != nil {
		t.Fatal(err)
	}
	if otra != cachePath {
		t.Errorf("Path dio %q la primera vez y %q la segunda", cachePath, otra)
	}
}

// TestGuardarYLeyersePisanElPropio: la ruta que compone `Path` sirve para guardar.
//
// Y este es el cierre del recorrido: `Path` compone una ruta que no existe todavía —el
// subdirectorio `prdash` dentro del cache del usuario no está en una máquina nueva— y
// `Save` tiene que crearlo. Sin esto, un `Save` con la ruta de `Path` fallaría en el primer
// arranque, que es exactamente cuando más se necesita que funcione.
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

	// El directorio no existe todavía.
	if _, err := os.Stat(filepath.Dir(cachePath)); err == nil {
		t.Fatal("el directorio de la cache ya existía antes de guardar")
	}

	if err := Save(cachePath, File{}); err != nil {
		t.Fatalf("Save con la ruta de Path: %v", err)
	}
	if err := SaveMemo(memoPath, emptyMemo()); err != nil {
		t.Fatalf("SaveMemo con la ruta de MemoPath: %v", err)
	}
	// Y los dos se leen como buenos.
	if _, ok := Load(cachePath); !ok {
		t.Error("lo guardado con la ruta de Path no se lee")
	}
	if _, ok := LoadMemo(memoPath); !ok {
		t.Error("lo guardado con la ruta de MemoPath no se lee")
	}
	// Y no se han pisado: los dos ficheros existen con contenidos distintos.
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

// TestSinVariablesDeEntornoLasRutasDegradan: sin HOME ni XDG no hay ruta, y se dice.
//
// Y el caso importa porque es el de un contenedor mínimo. `os.UserCacheDir` necesita
// `XDG_CACHE_HOME` o `HOME`; sin ninguno de los dos falla, y lo que se comprueba es que
// `Path` lo propaga en vez de devolver una cadena vacía. Una ruta vacía sería peor que un
// error: `Save` la aceptaría y escribiría en el directorio de trabajo del proceso.
func TestSinVariablesDeEntornoLasRutasDegradan(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", "")
	t.Setenv("HOME", "")

	// Puede que la plataforma siga encontrando un sitio (en macOS lo busca en otros sitios),
	// así que lo que se comprueba es la coherencia: si hay ruta, es un sitio real con el
	// subdirectorio del programa; si no hay, hay error.
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
