package main

import (
	"path/filepath"
	"strings"
	"testing"

	"prdash/internal/testutil"
	"prdash/internal/worktree"
)

// worktreeFixture crea un repo real con dos worktrees bajo la misma raíz: uno
// propio de prdash y otro ajeno.
func worktreeFixture(t *testing.T) (base, owned, foreign string) {
	t.Helper()
	repo := filepath.Join(t.TempDir(), "repo")
	testutil.InitRepo(t, repo)
	testutil.CommitFile(t, repo, "base.txt", "base", "base")
	testutil.RunGit(t, repo, "branch", "propia")
	testutil.RunGit(t, repo, "branch", "ajena")

	base = t.TempDir()
	owned = filepath.Join(base, "prdash-pr-1")
	foreign = filepath.Join(base, "otra-herramienta")
	testutil.RunGit(t, repo, "worktree", "add", "--quiet", owned, "propia")
	testutil.RunGit(t, repo, "worktree", "add", "--quiet", foreign, "ajena")
	return base, owned, foreign
}

// TestRunWorktreesListsOnlyOwned comprueba que el listado muestra los worktrees
// de prdash y nunca los ajenos que conviven con ellos.
func TestRunWorktreesListsOnlyOwned(t *testing.T) {
	base, owned, foreign := worktreeFixture(t)
	pr := worktree.NewGitDirect(base)

	var code int
	out := captureStdout(t, func() { code = runWorktrees(pr, []string{"list"}) })
	if code != 0 {
		t.Fatalf("código de salida = %d", code)
	}
	for _, want := range []string{"prdash-pr-1", owned, "propia", "ok"} {
		if !strings.Contains(out, want) {
			t.Errorf("el listado no contiene %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, foreign) || strings.Contains(out, "otra-herramienta") {
		t.Errorf("el listado no debería incluir worktrees ajenos:\n%s", out)
	}
}

// TestRunWorktreesDefaultsToList comprueba que sin subcomando se lista.
func TestRunWorktreesDefaultsToList(t *testing.T) {
	base, _, _ := worktreeFixture(t)
	var code int
	out := captureStdout(t, func() { code = runWorktrees(worktree.NewGitDirect(base), nil) })
	if code != 0 || !strings.Contains(out, "prdash-pr-1") {
		t.Fatalf("code=%d out=%q", code, out)
	}
}

// TestRunWorktreesRemoveRefusesForeign comprueba que un borrado explícito sobre
// un worktree ajeno se rechaza sin tocarlo.
func TestRunWorktreesRemoveRefusesForeign(t *testing.T) {
	base, owned, foreign := worktreeFixture(t)
	pr := worktree.NewGitDirect(base)

	if code := runWorktrees(pr, []string{"remove", foreign}); code == 0 {
		t.Fatal("borrar un worktree ajeno debería fallar")
	}
	if !worktree.Exists(foreign) {
		t.Fatal("el worktree ajeno no debería haberse tocado")
	}
	if !worktree.Exists(owned) {
		t.Fatal("el worktree propio no debería tocarse al rechazar otro")
	}
}

// TestRunWorktreesRemoveOwned comprueba que un borrado explícito de un worktree
// propio sí lo quita, dejando intactos los ajenos.
func TestRunWorktreesRemoveOwned(t *testing.T) {
	base, owned, foreign := worktreeFixture(t)
	pr := worktree.NewGitDirect(base)

	var code int
	captureStdout(t, func() { code = runWorktrees(pr, []string{"remove", owned}) })
	if code != 0 {
		t.Fatalf("código de salida = %d", code)
	}
	if worktree.Exists(owned) {
		t.Fatal("el worktree propio debería haberse borrado")
	}
	if !worktree.Exists(foreign) {
		t.Fatal("el worktree ajeno no debería tocarse")
	}
}

// TestRunWorktreesUsageErrors cubre los usos inválidos del subcomando.
func TestRunWorktreesUsageErrors(t *testing.T) {
	base, _, _ := worktreeFixture(t)
	pr := worktree.NewGitDirect(base)

	if code := runWorktrees(pr, []string{"bogus"}); code != 2 {
		t.Fatalf("subcomando desconocido debería salir con 2, got %d", code)
	}
	if code := runWorktrees(pr, []string{"remove"}); code != 2 {
		t.Fatalf("remove sin rutas debería salir con 2, got %d", code)
	}
}
