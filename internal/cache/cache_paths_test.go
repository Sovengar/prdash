package cache

import (
	"os"
	"path/filepath"
	"testing"
)

func TestTheCachePathsHangFromXDGAndNotFromTheProgram(t *testing.T) {
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

	for name, path := range map[string]string{"Path": cachePath, "MemoPath": memoPath} {
		wantDir := filepath.Join(dir, DirName)
		if filepath.Dir(path) != wantDir {
			t.Errorf("%s gave %q, which does not hang from %q", name, path, wantDir)
		}
		if !filepath.IsAbs(path) {
			t.Errorf("%s gave a relative path: %q", name, path)
		}
	}

	if cachePath == memoPath {
		t.Errorf("Path and MemoPath gave the same file: %q", cachePath)
	}
	if filepath.Base(cachePath) != FileName {
		t.Errorf("Path gave the file %q, want %q", filepath.Base(cachePath), FileName)
	}
	if filepath.Base(memoPath) != MemoFileName {
		t.Errorf("MemoPath gave the file %q, want %q", filepath.Base(memoPath), MemoFileName)
	}
	if FileName == MemoFileName {
		t.Error("FileName and MemoFileName are the same: the two files clobber each other")
	}

	again, err := Path()
	if err != nil {
		t.Fatal(err)
	}
	if again != cachePath {
		t.Errorf("Path gave %q the first time and %q the second", cachePath, again)
	}
}

func TestSavingAndReadingBackDoNotClobberEachOther(t *testing.T) {
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
		t.Fatal("the cache directory already existed before saving")
	}

	if err := Save(cachePath, File{}); err != nil {
		t.Fatalf("Save with Path's route: %v", err)
	}
	if err := SaveMemo(memoPath, emptyMemo()); err != nil {
		t.Fatalf("SaveMemo with MemoPath's route: %v", err)
	}
	if _, ok := Load(cachePath); !ok {
		t.Error("what was saved with Path's route does not read back")
	}
	if _, ok := LoadMemo(memoPath); !ok {
		t.Error("what was saved with MemoPath's route does not read back")
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
		t.Error("the two files have the same content: one clobbered the other")
	}
}

func TestWithoutEnvironmentVariablesThePathsDegrade(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", "")
	t.Setenv("HOME", "")

	// The platform may still find somewhere, so this only says "fallback used".
	cachePath, cacheErr := Path()
	memoPath, memoErr := MemoPath()

	if cacheErr == nil && cachePath == "" {
		t.Error("Path gave an empty path with no error")
	}
	if memoErr == nil && memoPath == "" {
		t.Error("MemoPath gave an empty path with no error")
	}
	if cacheErr != nil && memoErr == nil {
		t.Errorf("Path failed but MemoPath did not: %v", cacheErr)
	}
	if cacheErr == nil {
		if filepath.Base(filepath.Dir(cachePath)) != DirName {
			t.Errorf("the path %q does not carry the program's subdirectory", cachePath)
		}
	}
	if memoErr == nil && cacheErr == nil && cachePath == memoPath {
		t.Error("with no environment the two paths collapsed into the same file")
	}
}
