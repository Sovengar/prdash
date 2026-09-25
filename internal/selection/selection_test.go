package selection

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"prdash/internal/forge/model"
)

func sampleItem() model.Item {
	it := model.NewItem(model.RepoRef{
		Forge: "github", Host: "github.com", Project: "acme/widget", Owner: "acme", Name: "widget",
	}, 7)
	it.ReviewKind = model.ReviewRequested
	it.Section = model.SectionReview
	it.URL = "https://github.com/acme/widget/pull/7"
	return it
}

// TestRoundTripKeepsIdentity comprueba que la identidad completa sobrevive al
// guardado y la reconstrucción.
func TestRoundTripKeepsIdentity(t *testing.T) {
	path := filepath.Join(t.TempDir(), "selection.json")
	it := sampleItem()

	if err := Save(path, FromItem(it, time.Now())); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, ok := Load(path)
	if !ok {
		t.Fatal("la selección debería cargarse")
	}
	rebuilt, ok := got.Item()
	if !ok {
		t.Fatal("la selección debería reconstruir el ítem")
	}
	if rebuilt.ID() != it.ID() {
		t.Fatalf("ID = %+v, want %+v", rebuilt.ID(), it.ID())
	}
	if rebuilt.ReviewKind != it.ReviewKind || rebuilt.Section != it.Section || rebuilt.URL != it.URL {
		t.Fatalf("metadatos = %+v", rebuilt)
	}
	if rebuilt.Ref.Owner != "acme" || rebuilt.Ref.Name != "widget" {
		t.Fatalf("owner/name = %q/%q", rebuilt.Ref.Owner, rebuilt.Ref.Name)
	}
}

// TestLoadMissingOrCorruptIsFalse: ausente o corrupto no es un error, es "no
// hay selección".
func TestLoadMissingOrCorruptIsFalse(t *testing.T) {
	dir := t.TempDir()
	if _, ok := Load(filepath.Join(dir, "no-existe.json")); ok {
		t.Fatal("un fichero ausente no debería dar selección")
	}
	bad := filepath.Join(dir, "bad.json")
	if err := os.WriteFile(bad, []byte("{no json"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, ok := Load(bad); ok {
		t.Fatal("un fichero corrupto no debería dar selección")
	}
}

// TestItemRejectsIncompleteIdentity valida que una identidad incompleta no se
// pueda reconstruir.
func TestItemRejectsIncompleteIdentity(t *testing.T) {
	cases := []Selected{
		{},
		{Forge: "github", Host: "github.com", Project: "acme/widget"},
		{Forge: "github", Host: "", Project: "acme/widget", Number: 1},
		{Forge: "github", Host: "github.com", Project: "", Number: 1},
	}
	for _, c := range cases {
		if _, ok := c.Item(); ok {
			t.Fatalf("identidad incompleta no debería reconstruirse: %+v", c)
		}
	}
}

// TestFreshRejectsOldAndZero comprueba la vigencia de la selección.
func TestFreshRejectsOldAndZero(t *testing.T) {
	now := time.Now()
	if !(Selected{SavedAt: now}).Fresh(now, MaxAge) {
		t.Fatal("una selección reciente debería estar vigente")
	}
	if (Selected{SavedAt: now.Add(-2 * MaxAge)}).Fresh(now, MaxAge) {
		t.Fatal("una selección antigua debería estar obsoleta")
	}
	if (Selected{}).Fresh(now, MaxAge) {
		t.Fatal("una selección sin fecha debería estar obsoleta")
	}
}

// TestSaveIsAtomicNoTempLeftover comprueba que no queda el temporal del rename.
func TestSaveIsAtomicNoTempLeftover(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "selection.json")
	if err := Save(path, FromItem(sampleItem(), time.Now())); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if _, err := os.Stat(path + ".tmp"); !os.IsNotExist(err) {
		t.Fatalf("no debería quedar temporal: %v", err)
	}
}

// TestPathHonorsXDGStateHome comprueba que la ruta respeta XDG_STATE_HOME.
func TestPathHonorsXDGStateHome(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_STATE_HOME", dir)
	path, err := Path()
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(dir, DirName, FileName)
	if path != want {
		t.Fatalf("Path = %q, want %q", path, want)
	}
}
