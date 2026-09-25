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
