package worktree

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"prdash/internal/testutil"
)

func repoConRama(t *testing.T, extra ...string) string {
	t.Helper()
	repo := filepath.Join(t.TempDir(), "repo")
	testutil.InitRepo(t, repo)
	testutil.CommitFile(t, repo, "base.txt", "base", "base")
	for _, r := range extra {
		testutil.RunGit(t, repo, "branch", r)
	}
	return repo
}

// Create's first guard.
func TestUnSpecIncompletoNoSeProcesaNiSeTocaNada(t *testing.T) {
	repo := repoConRama(t)
	raiz := t.TempDir()
	g := NewGitDirect(raiz)

	for _, c := range []struct {
		nombre string
		spec   Spec
	}{
		{"sin repo", Spec{Branch: "main", Path: filepath.Join(raiz, "wt")}},
		{"sin rama", Spec{Repo: repo, Path: filepath.Join(raiz, "wt")}},
		{"sin destino", Spec{Repo: repo, Branch: "main"}},
		{"todo vacío", Spec{}},
	} {
		_, err := g.Create(context.Background(), c.spec)
		if err == nil {
			t.Errorf("%s: Create dio nil", c.nombre)
			continue
		}
		// The message says WHAT is missing, not "incomplete spec": the reader has to know whether
		// mounting another item's review fixes it.
		if !strings.Contains(err.Error(), "incomplete spec") {
			t.Errorf("%s: el error %q no dice que el spec está incompleto", c.nombre, err)
		}
	}

	entradas, err := os.ReadDir(raiz)
	if err != nil {
		t.Fatal(err)
	}
	if len(entradas) != 0 {
		t.Errorf("un spec incompleto dejó %d entradas en la raíz: %v", len(entradas), entradas)
	}
}

// cleanPartial's cleanup.
func TestUnaRamaQueNoExisteFallaYNoDejaElDirectorioResiduo(t *testing.T) {
	repo := repoConRama(t)
	raiz := t.TempDir()
	g := NewGitDirect(raiz)
	destino := filepath.Join(raiz, "prdash-pr-7")

	_, err := g.Create(context.Background(), Spec{
		Repo: repo, Branch: "feat/no-existe", Path: destino, Label: "prdash-pr-7",
	})
	if err == nil {
		t.Fatal("crear un worktree de una rama que no existe dio nil")
	}
	if !strings.Contains(err.Error(), "worktree") {
		t.Errorf("el error %q no dice que falla el worktree", err)
	}

	if _, err := os.Stat(destino); !os.IsNotExist(err) {
		t.Errorf("quedó el directorio %s tras un add fallido: el siguiente intento falla con "+
			"'already exists' y el mensaje no señala el primer fallo", destino)
	}
	lineas := strings.Split(strings.TrimSpace(
		testutil.RunGit(t, repo, "worktree", "list")), "\n")
	for _, l := range lineas {
		if strings.Contains(l, "prdash-pr-7") {
			t.Errorf("el worktree fallido quedó en el registro de git: %q", l)
		}
	}
	testutil.RunGit(t, repo, "branch", "otra")
	ok, err := g.Create(context.Background(), Spec{
		Repo: repo, Branch: "otra", Path: filepath.Join(raiz, "prdash-pr-8"),
		Label: "prdash-pr-8",
	})
	if err != nil {
		t.Fatalf("la raíz no sirve después de un fallo: %v", err)
	}
	if !Exists(ok.Path) {
		t.Error("el segundo worktree no existe después de un fallo del primero")
	}
}

func TestUnDestinoQueYaEsUnRepoNoSePisaNiSeLeeComoWorktree(t *testing.T) {
	raiz := t.TempDir()
	repo := filepath.Join(raiz, "prdash-pr-7")
	testutil.InitRepo(t, repo)
	testutil.CommitFile(t, repo, "trabajo.txt", "importante", "importante")

	g := NewGitDirect(raiz)
	ctx := context.Background()

	visto, ok, err := g.inspect(ctx, repo)
	if err == nil {
		t.Error("inspect de un repo normal dio nil: alguien podría tomarlo por un worktree")
	}
	if ok {
		t.Error("inspect dijo que un repo normal es un worktree enlazado")
	}
	if visto.Path != "" {
		t.Errorf("inspect devolvió un worktree: %+v", visto)
	}
	if !strings.Contains(err.Error(), "not a linked worktree") {
		t.Errorf("el error %q no dice que no es un worktree enlazado", err)
	}

	if _, err := g.Create(ctx, Spec{
		Repo: filepath.Join(t.TempDir(), "otro"), Branch: "main",
		Path: repo, Label: "prdash-pr-7",
	}); err == nil {
		t.Error("Create sobre un repo existente dio nil")
	}
	if _, err := os.Stat(filepath.Join(repo, "trabajo.txt")); err != nil {
		t.Errorf("el repo del usuario perdió su contenido: %v", err)
	}

	if err := g.Remove(ctx, repo); err == nil {
		t.Error("Remove de un repo normal dio nil")
	}
	if _, err := os.Stat(filepath.Join(repo, "trabajo.txt")); err != nil {
		t.Errorf("Remove se llevó el repo del usuario: %v", err)
	}
}

// Skipping .git directories is not an optimisation.
func TestAuditNoEntraEnLosDirectoriosGit(t *testing.T) {
	raiz := t.TempDir()
	repo := repoConRama(t, "feat/x")
	g := NewGitDirect(raiz)

	if _, err := g.Create(context.Background(), Spec{
		Repo: repo, Branch: "feat/x", Path: filepath.Join(raiz, "prdash-pr-7"),
		Label: "prdash-pr-7",
	}); err != nil {
		t.Fatal(err)
	}

	entradas := g.Audit(context.Background())
	if len(entradas) != 1 {
		t.Fatalf("Audit devolvió %d entradas, want 1: %+v", len(entradas), entradas)
	}
	if entradas[0].Path != filepath.Join(raiz, "prdash-pr-7") {
		t.Errorf("Audit devolvió %q", entradas[0].Path)
	}

	for _, e := range entradas {
		if strings.Contains(e.Path, string(filepath.Separator)+".git"+string(filepath.Separator)) {
			t.Errorf("Audit listó algo de dentro de un .git: %q", e.Path)
		}
		if !strings.HasPrefix(filepath.Base(e.Path), LabelPrefix) {
			t.Errorf("Audit listó %q, que no lleva el prefijo de ownership", e.Path)
		}
	}
}

// The lock's last turn.
func TestRemoveIfCleanPropagaElFalloDeQuitarlo(t *testing.T) {
	raiz := t.TempDir()
	repo := repoConRama(t, "feat/x")
	g := NewGitDirect(raiz)
	wt, err := g.Create(context.Background(), Spec{
		Repo: repo, Branch: "feat/x", Path: filepath.Join(raiz, "prdash-pr-7"),
		Label: "prdash-pr-7",
	})
	if err != nil {
		t.Fatal(err)
	}

	borrado, motivo, err := g.RemoveIfClean(context.Background(), wt.Path)
	if err != nil || !borrado || motivo != "" {
		t.Fatalf("RemoveIfClean de un worktree limpio: (%v, %q, %v)", borrado, motivo, err)
	}

	if _, _, err := g.RemoveIfClean(context.Background(), filepath.Join(t.TempDir(), "ajeno")); err == nil {
		t.Error("RemoveIfClean de una ruta ajena dio nil")
	}
}
