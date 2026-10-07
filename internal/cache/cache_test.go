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
		t.Fatal("Load should find the snapshot")
	}
	if len(f.Streams) != 1 || f.Streams[0].Items[0].Title != "Add widget" || f.Streams[0].Cursor != "c1" {
		t.Fatalf("snapshot = %+v", f)
	}
}

func TestLoadMissingIsSilent(t *testing.T) {
	if _, ok := Load(filepath.Join(t.TempDir(), "nope.json")); ok {
		t.Fatal("a missing cache should be silent")
	}
}

func TestLoadCorruptIsSilent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "inbox.json")
	if err := os.WriteFile(path, []byte("{ not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, ok := Load(path); ok {
		t.Fatal("a corrupt cache should be ignored")
	}
}

func TestLoadWrongVersionIsSilent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "inbox.json")
	if err := os.WriteFile(path, []byte(`{"version":999,"streams":[]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, ok := Load(path); ok {
		t.Fatal("an unknown version should be ignored")
	}
}

func TestSavingJSONFailsWithAValueThatCannotBeSerializedAndLeavesNoFile(t *testing.T) {
	target := filepath.Join(t.TempDir(), "sub", "cache.json")

	err := saveJSON(target, map[string]any{
		"channel": make(chan int),
	})

	if err == nil {
		t.Fatal("a value that cannot be serialised gave nil: the file would be written with " +
			"the field empty and the reader would not know anything was lost")
	}
	if !strings.Contains(err.Error(), "chan") && !strings.Contains(err.Error(), "json") &&
		!strings.Contains(err.Error(), "unsupported") {
		t.Errorf("the error %q does not say the value cannot be serialised", err)
	}
	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Errorf("%s was left behind after a serialisation failure: the reader would treat it as "+
			"an empty cache instead of an error", target)
	}
	// The parent directory IS created: MkdirAll runs before the marshalling.
	if _, err := os.Stat(filepath.Dir(target)); err != nil {
		t.Errorf("the parent directory was not created: %v. MkdirAll runs before the marshalling "+
			"and is not undone", err)
	}

	snapshot := File{
		SavedAt: time.Unix(1700000000, 0).UTC(),
		Streams: []Stream{{Forge: "github", Host: "github.com", Cursor: "CUR2"}},
	}
	if err := Save(target, snapshot); err != nil {
		t.Fatalf("Save of a valid value: %v", err)
	}
	readBack, ok := Load(target)
	if !ok {
		t.Fatal("the saved file could not be read back")
	}
	if len(readBack.Streams) != 1 || readBack.Streams[0].Cursor != "CUR2" {
		t.Errorf("what was read back is not what was saved: %+v", readBack)
	}
	if readBack.Version != version {
		t.Errorf("Version = %d after saving, want %d: Save itself sets it", readBack.Version, version)
	}

	memo := filepath.Join(t.TempDir(), "memo.json")
	if err := SaveMemo(memo, Memo{
		Reviews: map[string]ReviewRecord{
			"github/github.com/acme/widget#7": {Repo: "/c", Worktree: "/w", Branch: "b"},
		},
	}); err != nil {
		t.Fatalf("SaveMemo of a valid value: %v", err)
	}
	memoBack, ok := LoadMemo(memo)
	if !ok || len(memoBack.Reviews) != 1 {
		t.Errorf("the memo was not read back: %+v ok=%v", memoBack, ok)
	}
	if memoBack.Version != memoVersion {
		t.Errorf("Version = %d after saving the memo, want %d", memoBack.Version, memoVersion)
	}
}
