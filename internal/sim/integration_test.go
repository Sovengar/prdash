package sim

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"prdash/internal/forge/model"
	"prdash/internal/testutil"
)

func TestSimulateWithRealGitSimMerge(t *testing.T) {
	if !NewRunner().Available() {
		t.Skip("git-sim no está instalado")
	}

	repo, _, it := fixture(t)
	svc := newService(t, stubLocator{place: Place{Repo: repo, Branch: "prdash/pr-7"}, ok: true}, DefaultBin)

	res, err := svc.Simulate(context.Background(), it, KindMerge)
	if err != nil {
		t.Fatalf("Simulate(merge): %v", err)
	}
	img, err := Load(res.Path)
	if err != nil {
		t.Fatalf("la imagen no decodifica: %v", err)
	}
	if b := img.Bounds(); b.Dx() < 32 || b.Dy() < 32 {
		t.Errorf("imagen de %dx%d, demasiado pequeña para un grafo", b.Dx(), b.Dy())
	}
	if cells := Cells(img, 20, 10); len(cells) != 10 {
		t.Errorf("Cells devolvió %d líneas, want 10", len(cells))
	}
	t.Logf("merge -> %s", filepath.Base(res.Path))

	// The real render must leave no traces: no worktrees, no branches, no new refs.
	if n := strings.Count(testutil.RunGit(t, repo, "worktree", "list"), "\n"); n != 0 {
		t.Errorf("quedan %d worktrees en el repo", n)
	}
	if out := testutil.RunGit(t, repo, "branch", "--format=%(refname)"); strings.TrimSpace(out) != "refs/heads/main\nrefs/heads/prdash/pr-7" {
		t.Errorf("el render alteró las ramas del repo:\n%s", out)
	}
}

func TestRenderDoesNotHangWithoutDisplay(t *testing.T) {
	if !NewRunner().Available() {
		t.Skip("git-sim no está instalado")
	}
	repo, _, it := fixture(t)
	svc := newService(t, stubLocator{place: Place{Repo: repo, Branch: "prdash/pr-7"}, ok: true}, DefaultBin)

	ctx, cancel := context.WithTimeout(context.Background(), simIntegrationBudget)
	defer cancel()
	if _, err := svc.Simulate(ctx, it, KindMerge); err != nil {
		t.Fatalf("Simulate tardó o falló: %v", err)
	}
}

// A loose ceiling for the real render: exceeding it means the problem is the render.
const simIntegrationBudget = 30 * time.Second

func TestKeptImageIsUsable(t *testing.T) {
	path := writeJPEG(t)
	dir := t.TempDir()
	cache := filepath.Join(dir, "cache")

	svc := &Service{CacheDir: cache}
	it := model.NewItem(model.RepoRef{Forge: "github", Project: "grupo/sub/proyecto"}, 12)

	got, err := svc.keep(path, it, KindMerge)
	if err != nil {
		t.Fatalf("keep: %v", err)
	}
	if filepath.Dir(got) != cache {
		t.Errorf("la imagen se guardó en %q, fuera del caché %q", got, cache)
	}
	if strings.ContainsAny(filepath.Base(got), "/:") {
		t.Errorf("el nombre %q no es un nombre de fichero válido", filepath.Base(got))
	}
	img, err := Load(got)
	if err != nil {
		t.Fatalf("la imagen conservada no decodifica: %v", err)
	}
	if b := img.Bounds(); b.Dx() != 8 {
		t.Errorf("imagen de %d columnas, want 8: la copia no es la original", b.Dx())
	}
	if _, err := os.Stat(got); err != nil {
		t.Errorf("la imagen no existe tras copiarla: %v", err)
	}
}
