package cache

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"prdash/internal/forge/model"
)

func sample() File {
	it := model.NewItem(model.RepoRef{Forge: "github", Host: "github.com", Project: "acme/widget"}, 1)
	it.Title = "Add widget"
	return File{
		SavedAt: time.Now(),
		Streams: []Stream{{
			Forge:   "github",
			Host:    "github.com",
			Section: model.SectionAuthored,
			Cursor:  "c1",
			Items:   []model.Item{it},
		}},
	}
}

func TestSaveLoadRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "inbox.json")
	if err := Save(path, sample()); err != nil {
		t.Fatalf("Save: %v", err)
	}
	f, ok := Load(path)
	if !ok {
		t.Fatal("Load debería encontrar el snapshot")
	}
	if len(f.Streams) != 1 || f.Streams[0].Items[0].Title != "Add widget" || f.Streams[0].Cursor != "c1" {
		t.Fatalf("snapshot = %+v", f)
	}
}

func TestLoadMissingIsSilent(t *testing.T) {
	if _, ok := Load(filepath.Join(t.TempDir(), "nope.json")); ok {
		t.Fatal("cache ausente debería ser silencioso")
	}
}

func TestLoadCorruptIsSilent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "inbox.json")
	if err := os.WriteFile(path, []byte("{ not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, ok := Load(path); ok {
		t.Fatal("cache corrupto debería ignorarse")
	}
}

func TestLoadWrongVersionIsSilent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "inbox.json")
	if err := os.WriteFile(path, []byte(`{"version":999,"streams":[]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, ok := Load(path); ok {
		t.Fatal("versión desconocida debería ignorarse")
	}
}

// json.Marshal's error is the only non-disk one in the sequence, and it is what makes the helper
// take `any`. A truncated leftover would read as "no cache" and cost a full refetch.
func TestGuardarJSONFallaConUnValorQueNoSePuedeSerializarYNoDejaFichero(t *testing.T) {
	destino := filepath.Join(t.TempDir(), "sub", "cache.json")

	err := guardaJSON(destino, map[string]any{
		// A channel cannot be serialised, and no File or Memo contains one.
		"canal": make(chan int),
	})

	if err == nil {
		t.Fatal("un valor que no se puede serializar dio nil: el fichero se escribiría con " +
			"el campo vacío y el lector no sabría que se perdió")
	}
	if !strings.Contains(err.Error(), "chan") && !strings.Contains(err.Error(), "json") &&
		!strings.Contains(err.Error(), "unsupported") {
		t.Errorf("el error %q no dice que el valor no se puede serializar", err)
	}
	// No half-written file: Load treats unreadable JSON as "no cache", so a leftover would cost a
	// full refetch.
	if _, err := os.Stat(destino); !os.IsNotExist(err) {
		t.Errorf("quedó %s tras un fallo de serialización: el lector lo trataría como caché "+
			"vacía en vez de como un error", destino)
	}
	// The parent directory IS created, because MkdirAll goes before the marshalling: the order is
	// "prepare first".
	if _, err := os.Stat(filepath.Dir(destino)); err != nil {
		t.Errorf("el directorio padre no se creó: %v. El MkdirAll va antes del marshalling y "+
			"no se deshace", err)
	}

	snapshot := File{
		SavedAt: time.Unix(1700000000, 0).UTC(),
		Streams: []Stream{{Forge: "github", Host: "github.com", Cursor: "CUR2"}},
	}
	if err := Save(destino, snapshot); err != nil {
		t.Fatalf("Save de un valor válido: %v", err)
	}
	leido, ok := Load(destino)
	if !ok {
		t.Fatal("el fichero guardado no se pudo releer")
	}
	if len(leido.Streams) != 1 || leido.Streams[0].Cursor != "CUR2" {
		t.Errorf("lo releído no es lo guardado: %+v", leido)
	}
	if leido.Version != version {
		t.Errorf("Version = %d tras guardar, want %d: el propio Save la pone", leido.Version, version)
	}

	memo := filepath.Join(t.TempDir(), "memo.json")
	if err := SaveMemo(memo, Memo{
		Reviews: map[string]ReviewRecord{
			"github/github.com/acme/widget#7": {Repo: "/c", Worktree: "/w", Branch: "b"},
		},
	}); err != nil {
		t.Fatalf("SaveMemo de un valor válido: %v", err)
	}
	leida, ok := LoadMemo(memo)
	if !ok || len(leida.Reviews) != 1 {
		t.Errorf("la memoria no se relejo: %+v ok=%v", leida, ok)
	}
	if leida.Version != memoVersion {
		t.Errorf("Version = %d tras guardar la memoria, want %d", leida.Version, memoVersion)
	}
}
