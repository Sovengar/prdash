package cache

import (
	"os"
	"path/filepath"
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
