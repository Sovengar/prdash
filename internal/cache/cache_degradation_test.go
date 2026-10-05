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
func TestWhatCannotBeReadIsNotACache(t *testing.T) {
	cases := []struct {
		name    string
		content string
	}{
		{"json that is not json", `this is not json`},
		{"valid json but something else", `{"other":"thing"}`},
		{"json array", `[1,2,3]`},
		{"wrong version", `{"version":9999,"streams":[]}`},
		{"no version", `{"streams":[]}`},
		{"version as text", `{"version":"1","streams":[]}`},
		{"empty file", ``},
	}
	for _, c := range cases {
		path := filepath.Join(t.TempDir(), "cache.json")
		if err := os.WriteFile(path, []byte(c.content), 0o644); err != nil {
			t.Fatal(err)
		}
		f, ok := Load(path)
		if ok {
			t.Errorf("%s: Load returned true with %q", c.name, c.content)
		}
		// And what comes back is really empty, not half: a File with Streamed set would not be empty.
		if len(f.Streams) != 0 {
			t.Errorf("%s: Load returned %d streams without saying it did not load", c.name, len(f.Streams))
		}
	}
}

// The commonest case of all: the first time prdash opens there is no file.
func TestAMissingCacheIsNotAnError(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "does-not-exist", "cache.json")

	f, ok := Load(path)
	if ok {
		t.Error("Load returned true for a file that does not exist")
	}
	if f.Version != 0 || len(f.Streams) != 0 {
		t.Errorf("Load returned %+v instead of an empty File", f)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Error("Load created the file that did not exist")
	}
}

// The two halves are the contract between writer and reader.
func TestSaveStampsTheVersionAndCreatesTheDirectory(t *testing.T) {
	path := filepath.Join(t.TempDir(), "a", "b", "c", "cache.json")

	f := File{SavedAt: time.Unix(0, 0), Streams: []Stream{{Forge: "github", Items: []model.Item{{Title: "a PR"}}}}}
	if err := Save(path, f); err != nil {
		t.Fatalf("Save: %v", err)
	}

	got, ok := Load(path)
	if !ok {
		t.Fatal("what Save wrote, Load does not read")
	}
	if got.Version != version {
		t.Errorf("the saved version is %d, want %d", got.Version, version)
	}
	if len(got.Streams) != 1 || got.Streams[0].Forge != "github" {
		t.Errorf("the saved stream was not recovered: %+v", got.Streams)
	}
}

// Save does fail in one place: the parent exists as a file.
func TestSaveFailsWhenTheDirectoryIsNotADirectory(t *testing.T) {
	dir := t.TempDir()
	blocker := filepath.Join(dir, "blocker")
	if err := os.WriteFile(blocker, []byte("I am a file"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := Save(filepath.Join(blocker, "cache.json"), File{}); err == nil {
		t.Error("Save returned nil when the parent was a file")
	}
}

// The cost of a wrong false is different here: the memo holds the resolved clone paths.
func TestTheSameForTheRouteMemo(t *testing.T) {
	cases := []struct {
		name    string
		content string
	}{
		{"json that is not json", `nothing`},
		{"wrong version", `{"version":9999,"routes":{}}`},
		{"json array", `[]`},
	}
	for _, c := range cases {
		path := filepath.Join(t.TempDir(), "memo.json")
		if err := os.WriteFile(path, []byte(c.content), 0o644); err != nil {
			t.Fatal(err)
		}
		m, ok := LoadMemo(path)
		if ok {
			t.Errorf("%s: LoadMemo returned true with %q", c.name, c.content)
		}
		// The maps come back EMPTY, not nil: writing to a nil map panics and writing to an empty
		//one does not.
		if m.Routes == nil {
			t.Errorf("%s: LoadMemo returned Routes nil", c.name)
		}
		if m.Reviews == nil {
			t.Errorf("%s: LoadMemo returned Reviews nil", c.name)
		}
		if len(m.Routes) != 0 || len(m.Reviews) != 0 {
			t.Errorf("%s: LoadMemo returned a memo with content: %+v", c.name, m)
		}
	}
}

func TestTheSameForAMissingMemo(t *testing.T) {
	path := filepath.Join(t.TempDir(), "does-not-exist.json")

	m, ok := LoadMemo(path)
	if ok {
		t.Error("LoadMemo returned true for a file that does not exist")
	}
	if m.Routes == nil || m.Reviews == nil {
		t.Errorf("LoadMemo returned nil maps with no file: %+v", m)
	}
	m.Routes["github/h/proy"] = "/tmp/clones/proy"
	m.Reviews["github/h/proy#1"] = ReviewRecord{Repo: "proy", Worktree: "/tmp/wt", Branch: "b"}
}

// LoadMemo's ok says nothing about the CONTENT: a {"version":1} is a valid empty memo.
func TestNormalizeFixesNilMaps(t *testing.T) {
	m := normalize(Memo{})
	if m.Routes == nil || m.Reviews == nil {
		t.Fatalf("normalize left a nil map: %+v", m)
	}
	// Only one nil: the other is NOT touched, which is what distinguishes this from zeroing everything.
	m = Memo{Routes: map[string]string{"a": "/b"}}
	m = normalize(m)
	if m.Routes["a"] != "/b" {
		t.Errorf("normalize lost the route that was already there: %+v", m.Routes)
	}
	if m.Reviews == nil {
		t.Error("normalize did not create Reviews")
	}
}

func TestSaveMemoStampsTheVersionAndNormalizes(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sub", "memo.json")

	if err := SaveMemo(path, Memo{Routes: map[string]string{"k": "/v"}}); err != nil {
		t.Fatalf("SaveMemo: %v", err)
	}

	m, ok := LoadMemo(path)
	if !ok {
		t.Fatal("what SaveMemo wrote, LoadMemo does not read")
	}
	if m.Version != memoVersion {
		t.Errorf("the saved version is %d, want %d", m.Version, memoVersion)
	}
	if m.Routes["k"] != "/v" {
		t.Errorf("the saved route was not recovered: %+v", m.Routes)
	}
	if m.Reviews == nil || len(m.Reviews) != 0 {
		t.Errorf("Reviews ended up %v", m.Reviews)
	}
}

func TestSaveMemoFailsWhenTheParentIsAFile(t *testing.T) {
	dir := t.TempDir()
	blocker := filepath.Join(dir, "blocker")
	if err := os.WriteFile(blocker, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := SaveMemo(filepath.Join(blocker, "memo.json"), emptyMemo()); err == nil {
		t.Error("SaveMemo returned nil when the parent was a file")
	}
}

func TestStoreWithoutAPathWritesNothingAndDoesNotCrash(t *testing.T) {
	s := OpenStore("")
	s.SetRoute("github/h/proy", "/tmp/clones/proy")
	s.SetReview("github/h/proy#1", ReviewRecord{Repo: "proy", Worktree: "/tmp/wt", Branch: "b"})
	s.DeleteReview("github/h/proy#1")

	if _, ok := s.Review("github/h/proy#1"); ok {
		t.Error("Review returned a record that was deleted")
	}
	if got, ok := s.Route("github/h/proy"); !ok || got != "/tmp/clones/proy" {
		t.Errorf("Route returned (%q, %v) after saving it", got, ok)
	}
}

func TestDeletingAReviewThatIsNotThereDoesNotBreak(t *testing.T) {
	s := OpenStore(filepath.Join(t.TempDir(), "memo.json"))
	s.SetReview("a#1", ReviewRecord{Repo: "proy"})
	s.DeleteReview("a#2") // never existed
	s.DeleteReview("a#2") // nor the second time
	if _, ok := s.Review("a#1"); !ok {
		t.Error("deleting an absent key took down the one that was there")
	}
	s.DeleteReview("a#1")
	if _, ok := s.Review("a#1"); ok {
		t.Error("Review returned a record that was deleted")
	}
}
