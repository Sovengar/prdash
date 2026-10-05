package reporesolver

import (
	"os"
	"path/filepath"
	"testing"

	"prdash/internal/cache"
)

// The most obvious thing in the world and exactly what nobody tests.
func TestTheExplicitMemoPathWins(t *testing.T) {
	xdg := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", xdg)
	// HOME too, so reading the cache directory does not depend on which one the test sets.
	t.Setenv("HOME", t.TempDir())

	explicit := filepath.Join(t.TempDir(), "memo.json")
	defaultPath := filepath.Join(xdg, cache.DirName, cache.MemoFileName)

	r := New(Options{MemoPath: explicit})
	r.store.SetRoute("clave", "/ruta/explicita")

	if _, err := os.Stat(explicit); err != nil {
		t.Fatalf("with an explicit MemoPath it did not write to the given path: %v", err)
	}
	// And it was NOT written to the default path.
	if _, err := os.Stat(defaultPath); err == nil {
		t.Fatalf("with an explicit MemoPath it ALSO wrote to the default path (%s): "+
			"the given path is being ignored", defaultPath)
	}
}

func TestWithoutMemoPathTheDefaultIsUsed(t *testing.T) {
	xdg := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", xdg)
	t.Setenv("HOME", t.TempDir())

	defaultPath := filepath.Join(xdg, cache.DirName, cache.MemoFileName)

	// Written with one resolver and read with another.
	r := New(Options{})
	r.store.SetRoute("clave", "/ruta/recordada")

	if _, err := os.Stat(defaultPath); err != nil {
		t.Fatalf("without MemoPath it did not write to the default path (%s): %v", defaultPath, err)
	}

	other := New(Options{})
	if route, ok := other.store.Route("clave"); !ok || route != "/ruta/recordada" {
		t.Errorf("a new resolver did not find the remembered route (ok=%v, route=%q): "+
			"the memo is not at the default path", ok, route)
	}
}

// The default path carries the product's subdirectory.
func TestTheDefaultPathIsTheCacheOneAndNoOther(t *testing.T) {
	xdg := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", xdg)
	t.Setenv("HOME", t.TempDir())

	r := New(Options{})
	r.store.SetRoute("k", "/r")

	want := filepath.Join(xdg, cache.DirName, cache.MemoFileName)
	if _, err := os.Stat(want); err != nil {
		t.Errorf("the memo is not at %s: %v", want, err)
	}
	inHome := filepath.Join(os.Getenv("HOME"), cache.MemoFileName)
	if _, err := os.Stat(inHome); err == nil {
		t.Errorf("the memo was also written to HOME (%s): two programs step on each other", inHome)
	}
}

// If the cache directory cannot be determined, New does not fail: the whole repo is built without
// memo persistence.
func TestAnUnreadableDefaultPathDoesNotBreakNew(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", "")
	t.Setenv("HOME", "")

	r := New(Options{})
	if r == nil || r.store == nil {
		t.Fatal("New returned nil with an unreadable cache: config that does not degrade to default")
	}
	r.store.SetRoute("k", "/r")
	if route, ok := r.store.Route("k"); !ok || route != "/r" {
		t.Errorf("the in-memory store is no good: ok=%v route=%q", ok, route)
	}
}
