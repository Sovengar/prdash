package cache

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"prdash/internal/forge/model"
)

// This file covers the DEGRADATION paths: Load and LoadMemo return a second boolean.

// The three reasons an existing cache is discarded are not the same thing, although all three
// end in false.
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
		// And what comes back is really empty, not half: a File with Streamed set would not be empty.
		if len(f.Streams) != 0 {
			t.Errorf("%s: Load devolvio %d streams sin decir que no se cargo", c.nombre, len(f.Streams))
		}
	}
}

// The commonest case of all: the first time prdash opens there is no file.
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

// The two halves are the contract between writer and reader.
func TestSaveEstampaLaVersionYCreaElDirectorio(t *testing.T) {
	path := filepath.Join(t.TempDir(), "a", "b", "c", "cache.json")

	f := File{SavedAt: time.Unix(0, 0), Streams: []Stream{{Forge: "github", Items: []model.Item{{Title: "un PR"}}}}}
	if err := Save(path, f); err != nil {
		t.Fatalf("Save: %v", err)
	}

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

// Save does fail in one place: the parent exists as a file.
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

// The cost of a wrong false is different here: the memo holds the resolved clone paths.
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
		// The maps come back EMPTY, not nil: writing to a nil map panics and writing to an empty
		//one does not.
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

func TestLoMismoConLaMemoriaAusente(t *testing.T) {
	path := filepath.Join(t.TempDir(), "no-existe.json")

	m, ok := LoadMemo(path)
	if ok {
		t.Error("LoadMemo dio true para un fichero que no existe")
	}
	if m.Routes == nil || m.Reviews == nil {
		t.Errorf("LoadMemo devolvio mapas nil sin fichero: %+v", m)
	}
	m.Routes["github/h/proy"] = "/tmp/clones/proy"
	m.Reviews["github/h/proy#1"] = ReviewRecord{Repo: "proy", Worktree: "/tmp/wt", Branch: "b"}
}

// LoadMemo's ok says nothing about the CONTENT: a {"version":1} is a valid empty memo.
func TestNormalizeArreglaLosMapasNIL(t *testing.T) {
	m := normalize(Memo{})
	if m.Routes == nil || m.Reviews == nil {
		t.Fatalf("normalize dejo un mapa a nil: %+v", m)
	}
	// Only one nil: the other is NOT touched, which is what distinguishes this from zeroing everything.
	m = Memo{Routes: map[string]string{"a": "/b"}}
	m = normalize(m)
	if m.Routes["a"] != "/b" {
		t.Errorf("normalize perdio la ruta que ya estaba: %+v", m.Routes)
	}
	if m.Reviews == nil {
		t.Error("normalize no creo Reviews")
	}
}

func TestSaveMemoEstampaLaVersionYNormaliza(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sub", "memo.json")

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
	if m.Reviews == nil || len(m.Reviews) != 0 {
		t.Errorf("Reviews quedo %v", m.Reviews)
	}
}

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

func TestStoreSinRutaNoEscribeYNoRevienta(t *testing.T) {
	s := OpenStore("")
	s.SetRoute("github/h/proy", "/tmp/clones/proy")
	s.SetReview("github/h/proy#1", ReviewRecord{Repo: "proy", Worktree: "/tmp/wt", Branch: "b"})
	s.DeleteReview("github/h/proy#1")

	if _, ok := s.Review("github/h/proy#1"); ok {
		t.Error("Review devolvio un registro que se borro")
	}
	if got, ok := s.Route("github/h/proy"); !ok || got != "/tmp/clones/proy" {
		t.Errorf("Route devolvio (%q, %v) tras guardarla", got, ok)
	}
}

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
