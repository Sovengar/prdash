package worktree

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"prdash/internal/testutil"
)

func newRepo(t *testing.T) string {
	t.Helper()
	repo := filepath.Join(t.TempDir(), "repo")
	testutil.InitRepo(t, repo)
	testutil.CommitFile(t, repo, "base.txt", "base", "base")
	return repo
}

func TestCreateListRemove(t *testing.T) {
	repo := newRepo(t)
	testutil.RunGit(t, repo, "branch", "feature")

	base := filepath.Join(t.TempDir(), "worktrees")
	dest := filepath.Join(base, "github", "github.com", "acme", "widget", "prdash-pr-1")
	g := NewGitDirect(base)
	ctx := context.Background()

	wt, err := g.Create(ctx, Spec{Repo: repo, Branch: "feature", Path: dest, Label: "prdash-pr-1"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if wt.Path != dest || wt.Branch != "feature" || wt.Label != "prdash-pr-1" {
		t.Fatalf("worktree = %+v", wt)
	}
	if !Exists(dest) {
		t.Fatal("el worktree debería existir")
	}
	if _, err := os.Stat(filepath.Join(dest, "base.txt")); err != nil {
		t.Fatalf("el worktree no trae el contenido de la rama: %v", err)
	}

	list := g.List(ctx)
	if len(list) != 1 || list[0].Path != dest || list[0].Branch != "feature" {
		t.Fatalf("List = %+v", list)
	}

	if err := g.Remove(ctx, dest); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if _, err := os.Stat(dest); !os.IsNotExist(err) {
		t.Fatalf("el worktree debería haberse quitado: %v", err)
	}
	if got := g.List(ctx); len(got) != 0 {
		t.Fatalf("List tras Remove = %+v", got)
	}
}

func TestCreateReusesExistingWorktree(t *testing.T) {
	repo := newRepo(t)
	testutil.RunGit(t, repo, "branch", "feature")

	base := t.TempDir()
	dest := filepath.Join(base, "prdash-pr-1")
	g := NewGitDirect(base)
	ctx := context.Background()

	first, err := g.Create(ctx, Spec{Repo: repo, Branch: "feature", Path: dest, Label: "prdash-pr-1"})
	if err != nil {
		t.Fatalf("Create 1: %v", err)
	}
	second, err := g.Create(ctx, Spec{Repo: repo, Branch: "feature", Path: dest, Label: "prdash-pr-1"})
	if err != nil {
		t.Fatalf("Create 2: %v", err)
	}
	if first.Path != second.Path {
		t.Fatalf("rutas distintas: %q vs %q", first.Path, second.Path)
	}
	if got := g.List(ctx); len(got) != 1 {
		t.Fatalf("no debería duplicar el worktree: %+v", got)
	}
}

func TestCoexistingWorktreesSameRepo(t *testing.T) {
	repo := newRepo(t)
	testutil.RunGit(t, repo, "branch", "feature-1")
	testutil.RunGit(t, repo, "branch", "feature-2")

	base := t.TempDir()
	g := NewGitDirect(base)
	ctx := context.Background()

	w1, err := g.Create(ctx, Spec{Repo: repo, Branch: "feature-1", Path: filepath.Join(base, "prdash-pr-1"), Label: "prdash-pr-1"})
	if err != nil {
		t.Fatalf("Create 1: %v", err)
	}
	w2, err := g.Create(ctx, Spec{Repo: repo, Branch: "feature-2", Path: filepath.Join(base, "prdash-pr-2"), Label: "prdash-pr-2"})
	if err != nil {
		t.Fatalf("Create 2: %v", err)
	}
	if w1.Path == w2.Path {
		t.Fatal("dos ítems del mismo repo deben tener worktrees distintos")
	}
	if !Exists(w1.Path) || !Exists(w2.Path) {
		t.Fatal("ambos worktrees deben coexistir")
	}
	if got := g.List(ctx); len(got) != 2 {
		t.Fatalf("List = %+v, quiero 2", got)
	}
}

func TestCreateBranchMismatchErrors(t *testing.T) {
	repo := newRepo(t)
	testutil.RunGit(t, repo, "branch", "feature-1")
	testutil.RunGit(t, repo, "branch", "feature-2")

	base := t.TempDir()
	dest := filepath.Join(base, "prdash-pr-1")
	g := NewGitDirect(base)
	ctx := context.Background()

	if _, err := g.Create(ctx, Spec{Repo: repo, Branch: "feature-1", Path: dest, Label: "prdash-pr-1"}); err != nil {
		t.Fatalf("Create: %v", err)
	}
	_, err := g.Create(ctx, Spec{Repo: repo, Branch: "feature-2", Path: dest, Label: "prdash-pr-1"})
	if err == nil || !strings.Contains(err.Error(), "feature-1") {
		t.Fatalf("esperaba error por rama ya presente, got %v", err)
	}
}

func TestCreateIncompleteSpecErrors(t *testing.T) {
	g := NewGitDirect(t.TempDir())
	if _, err := g.Create(context.Background(), Spec{Repo: "x"}); err == nil {
		t.Fatal("spec incompleto debería fallar")
	}
}

func TestCreateFailureLeavesNoPartialDir(t *testing.T) {
	repo := newRepo(t)
	base := t.TempDir()
	dest := filepath.Join(base, "prdash-pr-1")
	g := NewGitDirect(base)

	// La rama no existe: `git worktree add` falla y no debe dejar restos.
	if _, err := g.Create(context.Background(), Spec{Repo: repo, Branch: "no-existe", Path: dest, Label: "prdash-pr-1"}); err == nil {
		t.Fatal("esperaba error al sacar una rama inexistente")
	}
	if _, err := os.Stat(dest); !os.IsNotExist(err) {
		t.Fatalf("no debería quedar un worktree a medias: %v", err)
	}
}

// newCleanWorktree crea un worktree limpio bajo una raíz propia y devuelve la
// raíz, la ruta del checkout y el repo de origen (para poder ensuciarlo).
func newCleanWorktree(t *testing.T) (base, dest, repo string) {
	t.Helper()
	repo = newRepo(t)
	testutil.RunGit(t, repo, "branch", "feature")
	base = t.TempDir()
	dest = filepath.Join(base, "prdash-pr-1")
	if _, err := NewGitDirect(base).Create(context.Background(), Spec{Repo: repo, Branch: "feature", Path: dest, Label: "prdash-pr-1"}); err != nil {
		t.Fatalf("preparar worktree limpio: %v", err)
	}
	return base, dest, repo
}

// TestRemoveIfCleanRemovesCleanWorktree cubre el caso feliz de B: un worktree sin
// cambios se borra.
func TestRemoveIfCleanRemovesCleanWorktree(t *testing.T) {
	base, dest, _ := newCleanWorktree(t)
	removed, reason, err := NewGitDirect(base).RemoveIfClean(context.Background(), dest)
	if err != nil {
		t.Fatalf("RemoveIfClean: %v", err)
	}
	if !removed || reason != "" {
		t.Fatalf("removed=%v reason=%q, quiero borrado y sin motivo", removed, reason)
	}
	if _, err := os.Stat(dest); !os.IsNotExist(err) {
		t.Fatalf("el worktree limpio debería haberse borrado: %v", err)
	}
}

// TestRemoveIfCleanKeepsDirtyWorktree cubre el candado "solo si limpio": con
// cambios sin commitear se conserva y se explica.
func TestRemoveIfCleanKeepsDirtyWorktree(t *testing.T) {
	base, dest, _ := newCleanWorktree(t)
	if err := os.WriteFile(filepath.Join(dest, "base.txt"), []byte("editado"), 0o644); err != nil {
		t.Fatal(err)
	}

	removed, reason, err := NewGitDirect(base).RemoveIfClean(context.Background(), dest)
	if err != nil {
		t.Fatalf("RemoveIfClean: %v", err)
	}
	if removed || reason != KeptUncommitted {
		t.Fatalf("removed=%v reason=%q, quiero conservado por sucio", removed, reason)
	}
	if _, err := os.Stat(dest); err != nil {
		t.Fatalf("el worktree sucio no debería tocarse: %v", err)
	}
}

// TestRemoveIfCleanKeepsUntrackedWorktree fija que un archivo nuevo sin trackear
// también cuenta como sucio: `git diff --quiet` lo ignora, `status --porcelain`
// no.
func TestRemoveIfCleanKeepsUntrackedWorktree(t *testing.T) {
	base, dest, _ := newCleanWorktree(t)
	if err := os.WriteFile(filepath.Join(dest, "nuevo.txt"), []byte("sin trackear"), 0o644); err != nil {
		t.Fatal(err)
	}

	removed, reason, err := NewGitDirect(base).RemoveIfClean(context.Background(), dest)
	if err != nil {
		t.Fatalf("RemoveIfClean: %v", err)
	}
	if removed || reason != KeptUncommitted {
		t.Fatalf("removed=%v reason=%q, quiero conservado por untracked", removed, reason)
	}
	if _, err := os.Stat(dest); err != nil {
		t.Fatalf("el worktree con untracked no debería tocarse: %v", err)
	}
}

// TestRemoveIfCleanKeepsOnUnreadableStatus cubre el fail-safe: si no se puede
// leer el estado de git, se conserva en vez de borrar.
func TestRemoveIfCleanKeepsOnUnreadableStatus(t *testing.T) {
	base, dest, _ := newCleanWorktree(t)
	// El enlace .git apunta a un gitdir inexistente: el fichero sigue siendo un
	// worktree enlazado, pero `git status` no puede leerlo.
	if err := os.WriteFile(filepath.Join(dest, ".git"), []byte("gitdir: /nonexistent/prdash-gitdir\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	removed, reason, err := NewGitDirect(base).RemoveIfClean(context.Background(), dest)
	if err != nil {
		t.Fatalf("RemoveIfClean: %v", err)
	}
	if removed || reason != KeptUnreadable {
		t.Fatalf("removed=%v reason=%q, quiero conservado por estado ilegible", removed, reason)
	}
	if _, err := os.Stat(dest); err != nil {
		t.Fatalf("el worktree con estado ilegible no debería tocarse: %v", err)
	}
}

// TestRemoveIfCleanAbsentPathIsNoop cubre "si el worktree ya no está en disco, es
// un no-op sin error": es lo que hace idempotente a B.
func TestRemoveIfCleanAbsentPathIsNoop(t *testing.T) {
	base := t.TempDir()
	dest := filepath.Join(base, "prdash-pr-1")
	removed, reason, err := NewGitDirect(base).RemoveIfClean(context.Background(), dest)
	if err != nil || removed || reason != "" {
		t.Fatalf("removed=%v reason=%q err=%v, quiero no-op sin error", removed, reason, err)
	}
}

// TestRemoveIfCleanRefusesForeign usa los mismos guardas que Remove: un worktree
// ajeno nunca se borra, ni siquiera si está limpio.
func TestRemoveIfCleanRefusesForeign(t *testing.T) {
	repo := newRepo(t)
	testutil.RunGit(t, repo, "branch", "feature")
	base := t.TempDir()
	foreign := filepath.Join(base, "otra-herramienta")
	testutil.RunGit(t, repo, "worktree", "add", "--quiet", foreign, "feature")

	removed, _, err := NewGitDirect(base).RemoveIfClean(context.Background(), foreign)
	if err == nil {
		t.Fatal("no debería aceptar borrar un worktree ajeno")
	}
	if removed {
		t.Fatal("removed debería ser false")
	}
	if _, err := os.Stat(foreign); err != nil {
		t.Fatalf("el worktree ajeno no debería tocarse: %v", err)
	}
}

// TestRemoveIfCleanRefusesPathOutsideBase comprueba que el guarda de raíz sigue
// vigente en el camino de B.
func TestRemoveIfCleanRefusesPathOutsideBase(t *testing.T) {
	repo := newRepo(t)
	testutil.RunGit(t, repo, "branch", "feature")
	outside := filepath.Join(t.TempDir(), "prdash-pr-1")
	testutil.RunGit(t, repo, "worktree", "add", "--quiet", outside, "feature")

	removed, _, err := NewGitDirect(t.TempDir()).RemoveIfClean(context.Background(), outside)
	if err == nil {
		t.Fatal("no debería aceptar borrar fuera de la raíz gestionada")
	}
	if removed {
		t.Fatal("removed debería ser false")
	}
	if _, err := os.Stat(outside); err != nil {
		t.Fatalf("el worktree fuera de la raíz no debería tocarse: %v", err)
	}
}
