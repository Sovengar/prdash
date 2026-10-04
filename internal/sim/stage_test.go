package sim

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"prdash/internal/testutil"
)

func simRepoMonta(t *testing.T) (repo, tmp string) {
	t.Helper()
	repo = filepath.Join(t.TempDir(), "repo")
	testutil.InitRepo(t, repo)
	testutil.CommitFile(t, repo, "base.txt", "base", "base")
	testutil.SetRemote(t, repo, "origin", repo)
	testutil.CommitFile(t, repo, "feat.txt", "feat", "feat")
	if _, err := os.Stat(filepath.Join(repo, ".git")); err != nil {
		t.Fatal(err)
	}
	git(t, repo, "branch", "feat/x")
	git(t, repo, "branch", "main-origin", "main")
	tmp = t.TempDir()
	return repo, tmp
}

func git(t *testing.T, dir string, args ...string) string {
	t.Helper()
	return testutil.RunGit(t, dir, args...)
}

func TestStageDejaActivaLaBaseParaIntegrarYLaDelItemParaRebasar(t *testing.T) {
	ctx := context.Background()

	repo, tmp := simRepoMonta(t)
	s := New(locatorDeStage())
	path, spec, err := s.stage(ctx, Place{Repo: repo, Branch: "feat/x"}, KindMerge, "main", tmp)
	if err != nil {
		t.Fatalf("merge: %v", err)
	}
	if got := git(t, path, "rev-parse", "--abbrev-ref", "HEAD"); got != "main" {
		t.Errorf("merge dejó activa %q, want main: el grafo de un merge es la base con "+
			"la rama encima", got)
	}
	if spec.Ref != "feat/x" {
		t.Errorf("el ref dibujado es %q, want la rama del item", spec.Ref)
	}
	if spec.Kind != KindMerge {
		t.Errorf("el spec no lleva el modo: %+v", spec)
	}

	repo, tmp = simRepoMonta(t)
	path, spec, err = s.stage(ctx, Place{Repo: repo, Branch: "feat/x"}, KindRebase, "main", tmp)
	if err != nil {
		t.Fatalf("rebase: %v", err)
	}
	if got := git(t, path, "rev-parse", "--abbrev-ref", "HEAD"); got != "feat/x" {
		t.Errorf("rebase dejó activa %q, want la rama del item: un rebase se calcula "+
			"desde la rama hacia la base", got)
	}
	if spec.Ref != "main" {
		t.Errorf("el ref dibujado es %q, want la base", spec.Ref)
	}
}

// The check goes BEFORE the clone; the order is what is pinned.
func TestStageNoClonaSiNoSeSabeLaRamaDelItem(t *testing.T) {
	repo, tmp := simRepoMonta(t)
	tmpAntes := contarEntradas(t, tmp)

	_, _, err := New(locatorDeStage()).stage(context.Background(),
		Place{Repo: repo, Branch: ""}, KindMerge, "main", tmp)

	if err == nil {
		t.Fatal("sin rama del item dio nil")
	}
	if !strings.Contains(err.Error(), "branch") {
		t.Errorf("el error %q no dice que falta la rama", err)
	}
	if !strings.Contains(err.Error(), "review") {
		t.Errorf("el error %q no dice qué hacer: montar el review", err)
	}
	// And nothing was cloned, which is half the reason for the order.
	if despues := contarEntradas(t, tmp); despues != tmpAntes {
		t.Errorf("clonó pese a no saber la rama: %d entradas antes, %d después", tmpAntes, despues)
	}
}

// The NORMAL case, not the rare one: local before remote.
func TestMaterializeUsaLaRamaLocalSiYaExiste(t *testing.T) {
	repo, tmp := simRepoMonta(t)
	s := New(locatorDeStage())
	path := filepath.Join(tmp, "clone")
	git(t, tmp, "clone", "--quiet", "--shared", repo, path)

	testutil.CommitFile(t, path, "local.txt", "local", "solo local")
	antes := git(t, path, "rev-parse", "main")

	if err := s.materialize(context.Background(), path, "main"); err != nil {
		t.Fatalf("materialize de una rama local: %v", err)
	}

	despues := git(t, path, "rev-parse", "main")
	if antes != despues {
		t.Errorf("materialize movió la rama local de %s a %s: un fetch no se trae el "+
			"commit local y el grafo dejaría de ser el de la review", antes, despues)
	}
	if _, err := os.Stat(filepath.Join(path, "local.txt")); err != nil {
		t.Errorf("materialize tiró el commit local: %v", err)
	}
}

// The usual case for the item's branch.
func TestMaterializeCreaLaRamaDelItemDesdeElRemoto(t *testing.T) {
	repo, tmp := simRepoMonta(t)
	s := New(locatorDeStage())
	path := filepath.Join(tmp, "clone")
	git(t, tmp, "clone", "--quiet", "--shared", repo, path)

	if tiene := git(t, path, "branch", "--list", "feat/x"); strings.TrimSpace(tiene) != "" {
		t.Fatalf("feat/x ya estaba en local, y este test necesita que no: %q", tiene)
	}
	if err := s.materialize(context.Background(), path, "feat/x"); err != nil {
		t.Fatalf("materialize de una rama remota: %v", err)
	}
	local := git(t, path, "rev-parse", "feat/x")
	remoto := git(t, path, "rev-parse", "origin/feat/x")
	if local != remoto {
		t.Errorf("la rama creada apunta a %s y el remoto a %s", local, remoto)
	}

	err := s.materialize(context.Background(), path, "no-existe")
	if err == nil {
		t.Fatal("una rama inexistente dio nil")
	}
	if !strings.Contains(err.Error(), "no-existe") {
		t.Errorf("el error %q no nombra la rama", err)
	}
	if !strings.Contains(err.Error(), "does not exist") {
		t.Errorf("el error %q no dice que no existe", err)
	}
	// The message says where to look: the local clone, which is where it was searched.
	if !strings.Contains(err.Error(), "local clone") {
		t.Errorf("el error %q no dice dónde se buscó", err)
	}
}

// --shared is not only an optimisation.
func TestElClonDeSimulacionEsTemporalYShared(t *testing.T) {
	repo, tmp := simRepoMonta(t)
	s := New(locatorDeStage())

	path, _, err := s.stage(context.Background(),
		Place{Repo: repo, Branch: "feat/x"}, KindMerge, "main", tmp)
	if err != nil {
		t.Fatal(err)
	}

	if filepath.Dir(path) != tmp {
		t.Errorf("el clon quedó en %q, want dentro de %q: el clon de simulación no puede "+
			"vivir junto al repo del usuario", filepath.Dir(path), tmp)
	}
	// Y no dejó nada en el repo de origen: ni refs nuevas, ni ramas, ni worktrees.
	ramas := git(t, repo, "branch", "--list")
	if strings.Contains(ramas, "sim") || strings.Contains(ramas, "tmp") {
		t.Errorf("la simulación dejó ramas en el repo de origen: %q", ramas)
	}
	// The source repo still has ONE worktree, its own, because a simulation clone is not registered as
	//one.
	lineas := strings.Split(strings.TrimSpace(git(t, repo, "worktree", "list")), "\n")
	if len(lineas) != 1 {
		t.Errorf("la simulación dejó worktrees en el repo de origen: %d", len(lineas)-1)
	}
}

func locatorDeStage() Locator { return locatorFalso{} }

func contarEntradas(t *testing.T, dir string) int {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	return len(entries)
}
