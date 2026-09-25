package worktree

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"prdash/internal/testutil"
)

// TestOwnedReconocesoloLaMarcaPrdash comprueba que el ownership se resuelve por
// el prefijo `prdash-` en la etiqueta o en el nombre de la ruta.
func TestOwnedReconocesoloLaMarcaPrdash(t *testing.T) {
	cases := []struct {
		label string
		path  string
		want  bool
	}{
		{"prdash-pr-3", "/x/y/prdash-pr-3", true},
		{"", "/x/y/prdash-pr-3", true},
		{"prdash-pr-3", "/x/y/otra-cosa", true},
		{"otra-herramienta", "/x/y/otra-herramienta", false},
		{"", "/x/y/otra-herramienta", false},
		{"repodash", "/x/y/repodash", false},
	}
	for _, c := range cases {
		if got := Owned(c.label, c.path); got != c.want {
			t.Errorf("Owned(%q, %q) = %v, quiero %v", c.label, c.path, got, c.want)
		}
	}
}

// TestAuditListsOnlyOwnedWorktrees comprueba que el listado ignora los
// worktrees ajenos que convivan bajo la misma raíz.
func TestAuditListsOnlyOwnedWorktrees(t *testing.T) {
	repo := newRepo(t)
	testutil.RunGit(t, repo, "branch", "propia")
	testutil.RunGit(t, repo, "branch", "ajena")

	base := t.TempDir()
	owned := filepath.Join(base, "prdash-pr-1")
	foreign := filepath.Join(base, "otra-herramienta")
	if _, err := NewGitDirect(base).Create(context.Background(), Spec{Repo: repo, Branch: "propia", Path: owned, Label: "prdash-pr-1"}); err != nil {
		t.Fatalf("preparar worktree propio: %v", err)
	}
	testutil.RunGit(t, repo, "worktree", "add", "--quiet", foreign, "ajena")

	entries := NewGitDirect(base).Audit(context.Background())
	if len(entries) != 1 {
		t.Fatalf("Audit = %+v, quiero solo el worktree propio", entries)
	}
	if entries[0].Path != owned || entries[0].Orphan {
		t.Fatalf("entrada = %+v", entries[0])
	}
	if entries[0].Branch != "propia" {
		t.Fatalf("rama = %q", entries[0].Branch)
	}
}

// TestAuditFlagsOrphanWhenSourceGone marca huérfano un worktree cuyo repo de
// origen desapareció: la limpieza debe poder reportarlo.
func TestAuditFlagsOrphanWhenSourceGone(t *testing.T) {
	repo := newRepo(t)
	testutil.RunGit(t, repo, "branch", "feature")

	base := t.TempDir()
	dest := filepath.Join(base, "prdash-pr-1")
	if _, err := NewGitDirect(base).Create(context.Background(), Spec{Repo: repo, Branch: "feature", Path: dest, Label: "prdash-pr-1"}); err != nil {
		t.Fatalf("preparar worktree: %v", err)
	}
	if err := os.RemoveAll(repo); err != nil {
		t.Fatal(err)
	}

	entries := NewGitDirect(base).Audit(context.Background())
	if len(entries) != 1 || !entries[0].Orphan || entries[0].Reason == "" {
		t.Fatalf("esperaba un huérfano con motivo, got %+v", entries)
	}
}

// TestListExcludesForeignWorktrees fija que el listado del puerto respeta el
// ownership: los worktrees ajenos no se exponen.
func TestListExcludesForeignWorktrees(t *testing.T) {
	repo := newRepo(t)
	testutil.RunGit(t, repo, "branch", "propia")
	testutil.RunGit(t, repo, "branch", "ajena")

	base := t.TempDir()
	owned := filepath.Join(base, "prdash-pr-1")
	foreign := filepath.Join(base, "otra-herramienta")
	if _, err := NewGitDirect(base).Create(context.Background(), Spec{Repo: repo, Branch: "propia", Path: owned, Label: "prdash-pr-1"}); err != nil {
		t.Fatalf("preparar worktree propio: %v", err)
	}
	testutil.RunGit(t, repo, "worktree", "add", "--quiet", foreign, "ajena")

	list := NewGitDirect(base).List(context.Background())
	if len(list) != 1 || list[0].Path != owned {
		t.Fatalf("List = %+v, quiero solo el worktree propio", list)
	}
}

// TestAuditSkipsNonWorktreeDirs ignora directorios normales bajo la raíz.
func TestAuditSkipsNonWorktreeDirs(t *testing.T) {
	base := t.TempDir()
	if err := os.MkdirAll(filepath.Join(base, "prdash-pr-9"), 0o755); err != nil {
		t.Fatal(err)
	}
	if got := NewGitDirect(base).Audit(context.Background()); len(got) != 0 {
		t.Fatalf("un directorio sin worktree no debería listarse: %+v", got)
	}
}
