package cache

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"prdash/internal/forge/model"
)

// Este fichero cubre lo que los tests de cache no cubrían, y lo que cubre es una clase
// entera: los caminos de DEGRADACIÓN. `Load` y `LoadMemo` devuelven un segundo booleano
// justamente para eso —"no hay cache" y "hay cache pero es basura" son el mismo caso para
// el llamador—, y ese segundo valor es el que decide si el inbox arranca vacío o se niega
// a arrancar.
//
// Y de ahí sale el motivo de que estos tests importen: un `false` que se cuela como
// `true` no rompe nada visible —la app funciona— y sí destruye el trabajo del usuario, que
// es la sesión de review montada y las rutas ya resueltas.

// TestLoQueNoSeLeeNoEsCache: los tres motivos por los que una cache existente se
// descarta, y los tres tienen que acabar en `false`.
//
// Y no son la misma cosa, aunque el código los trate igual: ausente, corrupto y versión
// desconocida. La versión desconocida es el caso interesante —un prdash nuevo leyendo la
// cache de uno viejo con un formato distinto— y es el que un test que solo usa "fichero
// que no existe" no tocaría nunca.
func TestLoQueNoSeLeeNoEsCache(t *testing.T) {
	casos := []struct {
		nombre    string
		contenido string
	}{
		{"json que no es json", `esto no es json`},
		{"json valido pero otra cosa", `{"otra":"cosa"}`},
		{"array json", `[1,2,3]`},
		{"version equivocada", `{"version":9999,"streams":[]}`},
		{"sin version", `{"streams":[]}`},
		{"version como texto", `{"version":"1","streams":[]}`},
		{"fichero vacio", ``},
	}
	for _, c := range casos {
		path := filepath.Join(t.TempDir(), "cache.json")
		if err := os.WriteFile(path, []byte(c.contenido), 0o644); err != nil {
			t.Fatal(err)
		}
		f, ok := Load(path)
		if ok {
			t.Errorf("%s: Load dio true con %q", c.nombre, c.contenido)
		}
		// Y cuando no se carga, lo que se devuelve está vacío de verdad, no a medias.
		// Un File con Streams nil se parece a uno con streams, y la diferencia se ve
		// en el rango.
		if len(f.Streams) != 0 {
			t.Errorf("%s: Load devolvio %d streams sin decir que no se cargo", c.nombre, len(f.Streams))
		}
	}
}

// TestLaCacheAusenteNoEsUnError: la primera vez que se abre prdash no hay fichero, y eso
// no es un fallo que haya que contar ni avisar.
//
// Y es el caso más frecuente de todos, así que el test se fija en lo que NO debe pasar:
// ni error, ni panic, ni fichero creado. Esto último es el detalle que importa: `Load`
// no debe crear nada, porque crear el fichero de cache en el primer arranque convertiría
// un repo de development limpio en uno con un fichero sin versionar.
func TestLaCacheAusenteNoEsUnError(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "no-existe", "cache.json")

	f, ok := Load(path)
	if ok {
		t.Error("Load dio true para un fichero que no existe")
	}
	if f.Version != 0 || len(f.Streams) != 0 {
		t.Errorf("Load devolvio %+v en vez de un File vacio", f)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Error("Load creo el fichero que no existia")
	}
}

// TestSaveEstampaLaVersionYCreaElDirectorio: el snapshot que se guarda lleva la versión
// dentro, y `Save` crea el directorio.
//
// Y las dos mitades son el contrato entre quien escribe y quien lee. `Load` rechaza lo que
// no lleva la versión buena, así que `Save` tiene que estampar la suya aunque el `File`
// que le pasen la traiga a cero: si no, un `File` recién construido se guardaría como
// ilegible y se perdería en la siguiente lectura.
//
// Y el directorio, porque la cache vive bajo el home del usuario y ese padre puede no
// existir. Es un mkdir -p de una llamada.
func TestSaveEstampaLaVersionYCreaElDirectorio(t *testing.T) {
	path := filepath.Join(t.TempDir(), "a", "b", "c", "cache.json")

	f := File{SavedAt: time.Unix(0, 0), Streams: []Stream{{Forge: "github", Items: []model.Item{{Title: "un PR"}}}}}
	if err := Save(path, f); err != nil {
		t.Fatalf("Save: %v", err)
	}

	// Y lo guardado se vuelve a leer como bueno, que es el viaje completo.
	got, ok := Load(path)
	if !ok {
		t.Fatal("lo que Save escribio, Load no lo lee")
	}
	if got.Version != version {
		t.Errorf("la version guardada es %d, want %d", got.Version, version)
	}
	if len(got.Streams) != 1 || got.Streams[0].Forge != "github" {
		t.Errorf("el stream guardado no se recupero: %+v", got.Streams)
	}
}

// TestSaveFallaSiElDirectorioNoEsUnDirectorio: hay un camino donde `Save` sí falla, y es
// cuando el padre existe como fichero.
//
// Y no es un caso inventado para cubrir la rama: el `MkdirAll` devuelve un `ENOTDIR` y,
// como `Save` es best-effort y su error se ignora en el llamador, la consecuencia sería un
// fichero que nunca se guarda y ningún aviso. Al menos el error debe ser un error de
// verdad para que quien lo reciba pueda avisar.
func TestSaveFallaSiElDirectorioNoEsUnDirectorio(t *testing.T) {
	dir := t.TempDir()
	bloque := filepath.Join(dir, "bloque")
	if err := os.WriteFile(bloque, []byte("soy un fichero"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := Save(filepath.Join(bloque, "cache.json"), File{}); err == nil {
		t.Error("Save dio nil cuando el padre era un fichero")
	}
}

// TestLoMismoConLaMemoriaDeRutas: los tres motivos de descarte, en la memoria.
//
// Y aquí el coste de un `false` equivocado es otro: la memoria guarda las rutas de clones
// ya resueltas. Perderla no rompe nada de inmediato, pero obliga a volver a resolver cada
// ítem contra la forge, y en un inbox de veinte PRs son veinte llamadas de red que se
// habrían evitado. Por eso la memoria normaliza siempre sus mapas antes de devolverlos.
func TestLoMismoConLaMemoriaDeRutas(t *testing.T) {
	casos := []struct {
		nombre    string
		contenido string
	}{
		{"json que no es json", `nada`},
		{"version equivocada", `{"version":9999,"routes":{}}`},
		{"array json", `[]`},
	}
	for _, c := range casos {
		path := filepath.Join(t.TempDir(), "memo.json")
		if err := os.WriteFile(path, []byte(c.contenido), 0o644); err != nil {
			t.Fatal(err)
		}
		m, ok := LoadMemo(path)
		if ok {
			t.Errorf("%s: LoadMemo dio true con %q", c.nombre, c.contenido)
		}
		// Y los mapas vienen VACÍOS, no nil. La diferencia importa porque quien escribe
		// en un mapa nil hace panic; quien escribe en un mapa vacío no. Una memoria
		// descartada que sale con `Routes == nil` deja al llamador con una elección
		// entre dos fallos distintos, y el más probable es el panic.
		if m.Routes == nil {
			t.Errorf("%s: LoadMemo devolvio Routes nil", c.nombre)
		}
		if m.Reviews == nil {
			t.Errorf("%s: LoadMemo devolvio Reviews nil", c.nombre)
		}
		if len(m.Routes) != 0 || len(m.Reviews) != 0 {
			t.Errorf("%s: LoadMemo devolvio memoria con contenido: %+v", c.nombre, m)
		}
	}
}

// TestLoMismoConLaMemoriaAusente: sin fichero, memo vacía pero usable.
func TestLoMismoConLaMemoriaAusente(t *testing.T) {
	path := filepath.Join(t.TempDir(), "no-existe.json")

	m, ok := LoadMemo(path)
	if ok {
		t.Error("LoadMemo dio true para un fichero que no existe")
	}
	if m.Routes == nil || m.Reviews == nil {
		t.Errorf("LoadMemo devolvio mapas nil sin fichero: %+v", m)
	}
	// Y se puede escribir en ella sin panic, que es el uso real de este caso.
	m.Routes["github/h/proy"] = "/tmp/clones/proy"
	m.Reviews["github/h/proy#1"] = ReviewRecord{Repo: "proy", Worktree: "/tmp/wt", Branch: "b"}
}

// TestNormalizeArreglaLosMapasNIL: la normalización, por las dos ramas.
//
// Y el `ok` de LoadMemo no dice nada del contenido: un `{"version":1}` —memoria de una
// versión antigua que aún no tenía mapas— pasa la comprobación de versión y sale con los
// mapas a nil. Sin `normalize` eso llega al `Store`, que escribe en el mapa y hace panic.
func TestNormalizeArreglaLosMapasNIL(t *testing.T) {
	// Los dos nil.
	m := normalize(Memo{})
	if m.Routes == nil || m.Reviews == nil {
		t.Fatalf("normalize dejo un mapa a nil: %+v", m)
	}
	// Solo uno nil: el otro NO se toca. Es lo que distingue esto de inicializarlo todo a
	// cero, y perder contenido ya escrito sería un fallo peor que el que arregla.
	m = Memo{Routes: map[string]string{"a": "/b"}}
	m = normalize(m)
	if m.Routes["a"] != "/b" {
		t.Errorf("normalize perdio la ruta que ya estaba: %+v", m.Routes)
	}
	if m.Reviews == nil {
		t.Error("normalize no creo Reviews")
	}
}

// TestSaveMemoEstampaLaVersionYNormaliza: guardar es escribir un fichero que se pueda
// volver a leer, y eso son las dos cosas a la vez.
func TestSaveMemoEstampaLaVersionYNormaliza(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sub", "memo.json")

	// Un Memo con versión CERO y mapas nil: lo que da un Store recién creado.
	if err := SaveMemo(path, Memo{Routes: map[string]string{"k": "/v"}}); err != nil {
		t.Fatalf("SaveMemo: %v", err)
	}

	m, ok := LoadMemo(path)
	if !ok {
		t.Fatal("lo que SaveMemo escribio, LoadMemo no lo lee")
	}
	if m.Version != memoVersion {
		t.Errorf("la version guardada es %d, want %d", m.Version, memoVersion)
	}
	if m.Routes["k"] != "/v" {
		t.Errorf("la ruta guardada no se recupero: %+v", m.Routes)
	}
	// Y el mapa que no se usó salió vacío y no nil, para que el siguiente guardado
	// escriba en un sitio real.
	if m.Reviews == nil || len(m.Reviews) != 0 {
		t.Errorf("Reviews quedo %v", m.Reviews)
	}
}

// TestSaveMemoFallaConElPadreQueEsUnFichero: la otra mitad del error de `Save`.
func TestSaveMemoFallaConElPadreQueEsUnFichero(t *testing.T) {
	dir := t.TempDir()
	bloque := filepath.Join(dir, "bloque")
	if err := os.WriteFile(bloque, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := SaveMemo(filepath.Join(bloque, "memo.json"), emptyMemo()); err == nil {
		t.Error("SaveMemo dio nil cuando el padre era un fichero")
	}
}

// TestStoreSinRutaNoEscribeYNoRevienta: un `Store` con la ruta vacía es el caso
// "memoria solo en memoria", que existe para poder probar y para el modo sin disco.
//
// Y la diferencia con un `Store` roto es que aquí el fallo es silencioso a propósito: no
// hay a dónde escribir, no hay nada que avisar, y abortar el guardado sería peor que
// perderlo. LaRAMificación está justo en `saveLocked`, y sin un test que la recorra el
// camino se ejecuta pero nadie sabe que funciona.
func TestStoreSinRutaNoEscribeYNoRevienta(t *testing.T) {
	s := OpenStore("")
	s.SetRoute("github/h/proy", "/tmp/clones/proy")
	s.SetReview("github/h/proy#1", ReviewRecord{Repo: "proy", Worktree: "/tmp/wt", Branch: "b"})
	s.DeleteReview("github/h/proy#1")

	// Y lo que sí se puede hacer es lo que no toca disco: preguntar.
	if _, ok := s.Review("github/h/proy#1"); ok {
		t.Error("Review devolvio un registro que se borro")
	}
	if got, ok := s.Route("github/h/proy"); !ok || got != "/tmp/clones/proy" {
		t.Errorf("Route devolvio (%q, %v) tras guardarla", got, ok)
	}
}

// TestBorrarUnReviewQueNoEstaNoRompe: `delete` sobre una clave ausente es un no-op, y
// ese es el caso normal: se llama al cerrar un review que puede que ya se haya cerrado
// desde otra pestaña.
func TestBorrarUnReviewQueNoEstaNoRompe(t *testing.T) {
	s := OpenStore(filepath.Join(t.TempDir(), "memo.json"))
	s.SetReview("a#1", ReviewRecord{Repo: "proy"})
	s.DeleteReview("a#2") // nunca existió
	s.DeleteReview("a#2") // ni la segunda vez
	if _, ok := s.Review("a#1"); !ok {
		t.Error("borrar una clave ausente se llevó por delante la que sí estaba")
	}
	s.DeleteReview("a#1")
	if _, ok := s.Review("a#1"); ok {
		t.Error("Review devolvio un registro que se borro")
	}
}
