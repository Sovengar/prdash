package cache

import (
	"os"
	"path/filepath"
	"testing"
)

func TestMemoRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "memo.json")
	s := OpenStore(path)
	s.SetRoute("github/github.com/acme/widget", "/home/u/dev/widget")
	s.SetReview("github/github.com/acme/widget#7", ReviewRecord{
		Repo:     "/home/u/dev/widget",
		Worktree: "/home/u/.local/share/prdash/worktrees/github/github.com/acme/widget/prdash-pr-7",
		Branch:   "prdash/pr-7",
		Label:    "prdash-pr-7",
	})

	reopened := OpenStore(path)
	if got, ok := reopened.Route("github/github.com/acme/widget"); !ok || got != "/home/u/dev/widget" {
		t.Fatalf("ruta recordada = %q, %v", got, ok)
	}
	rec, ok := reopened.Review("github/github.com/acme/widget#7")
	if !ok || rec.Branch != "prdash/pr-7" || rec.Worktree == "" {
		t.Fatalf("review recordado = %+v, %v", rec, ok)
	}
}

func TestMemoMissingIsSilent(t *testing.T) {
	s := OpenStore(filepath.Join(t.TempDir(), "nope.json"))
	if _, ok := s.Route("x"); ok {
		t.Fatal("memoria ausente debería ser silenciosa")
	}
	if _, ok := s.Review("x"); ok {
		t.Fatal("memoria ausente debería ser silenciosa")
	}
}

func TestMemoCorruptIsSilent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "memo.json")
	if err := os.WriteFile(path, []byte("{ roto"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, ok := LoadMemo(path); ok {
		t.Fatal("memoria corrupta debería ignorarse")
	}
}

func TestMemoWrongVersionIsSilent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "memo.json")
	if err := os.WriteFile(path, []byte(`{"version":999,"routes":{},"reviews":{}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, ok := LoadMemo(path); ok {
		t.Fatal("versión desconocida debería ignorarse")
	}
}
