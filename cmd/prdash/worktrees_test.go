package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"bytes"
	"prdash/internal/testutil"
	"prdash/internal/worktree"
)

func worktreeRepoFixture(t *testing.T) (repo, base, owned, foreign string) {
	t.Helper()
	repo = filepath.Join(t.TempDir(), "repo")
	testutil.InitRepo(t, repo)
	testutil.CommitFile(t, repo, "base.txt", "base", "base")
	testutil.RunGit(t, repo, "branch", "propia")
	testutil.RunGit(t, repo, "branch", "ajena")

	base = t.TempDir()
	owned = filepath.Join(base, "prdash-pr-1")
	foreign = filepath.Join(base, "otra-herramienta")
	testutil.RunGit(t, repo, "worktree", "add", "--quiet", owned, "propia")
	testutil.RunGit(t, repo, "worktree", "add", "--quiet", foreign, "ajena")
	return repo, base, owned, foreign
}

func worktreeFixture(t *testing.T) (base, owned, foreign string) {
	t.Helper()
	_, base, owned, foreign = worktreeRepoFixture(t)
	return base, owned, foreign
}

func worktreeOrphanFixture(t *testing.T) (base string, orphans []string, healthy, foreign string) {
	t.Helper()
	base = t.TempDir()

	repoOrphans := filepath.Join(t.TempDir(), "repo-huérfanos")
	testutil.InitRepo(t, repoOrphans)
	testutil.CommitFile(t, repoOrphans, "base.txt", "base", "base")
	testutil.RunGit(t, repoOrphans, "branch", "uno")
	testutil.RunGit(t, repoOrphans, "branch", "dos")
	o1 := filepath.Join(base, "prdash-pr-1")
	o2 := filepath.Join(base, "prdash-pr-2")
	testutil.RunGit(t, repoOrphans, "worktree", "add", "--quiet", o1, "uno")
	testutil.RunGit(t, repoOrphans, "worktree", "add", "--quiet", o2, "dos")

	repoHealthy := filepath.Join(t.TempDir(), "repo-sano")
	testutil.InitRepo(t, repoHealthy)
	testutil.CommitFile(t, repoHealthy, "base.txt", "base", "base")
	testutil.RunGit(t, repoHealthy, "branch", "sana")
	testutil.RunGit(t, repoHealthy, "branch", "ajena")
	healthy = filepath.Join(base, "prdash-pr-3")
	foreign = filepath.Join(base, "otra-herramienta")
	testutil.RunGit(t, repoHealthy, "worktree", "add", "--quiet", healthy, "sana")
	testutil.RunGit(t, repoHealthy, "worktree", "add", "--quiet", foreign, "ajena")

	if err := os.RemoveAll(repoOrphans); err != nil {
		t.Fatal(err)
	}
	return base, []string{o1, o2}, healthy, foreign
}

type fakeProvisioner struct {
	entries []worktree.Entry
	delays  map[string]time.Duration
	removed []string
}

func (f *fakeProvisioner) Create(context.Context, worktree.Spec) (worktree.Worktree, error) {
	return worktree.Worktree{}, nil
}
func (f *fakeProvisioner) RemoveIfClean(context.Context, string) (bool, string, error) {
	return false, "", nil
}
func (f *fakeProvisioner) List(context.Context) []worktree.Worktree { return nil }
func (f *fakeProvisioner) Audit(context.Context) []worktree.Entry   { return f.entries }
func (f *fakeProvisioner) Remove(ctx context.Context, id string) error {
	if d := f.delays[id]; d > 0 {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(d):
		}
	}
	f.removed = append(f.removed, id)
	return nil
}

// Each deletion in the batch has its own budget.
func TestRunWorktreesRemoveOrphansPerItemBudget(t *testing.T) {
	const (
		slow = "/base/prdash-pr-1"
		fast = "/base/prdash-pr-2"
	)
	pr := &fakeProvisioner{
		entries: []worktree.Entry{
			{Worktree: worktree.Worktree{Path: slow}, Orphan: true},
			{Worktree: worktree.Worktree{Path: fast}, Orphan: true},
		},
		delays: map[string]time.Duration{slow: 200 * time.Millisecond},
	}

	var stdout, stderr bytes.Buffer
	code := removeWorktreesWithin(pr, &stdout, &stderr, true, false, nil, 40*time.Millisecond)

	if code != 1 {
		t.Fatalf("un ítem que agota su presupuesto debería marcar fallo, code=%d", code)
	}
	if errOut := stderr.String(); !strings.Contains(errOut, slow) {
		t.Errorf("stderr = %q, quiero que nombre el ítem que agotó su presupuesto", errOut)
	}
	if len(pr.removed) != 1 || pr.removed[0] != fast {
		t.Fatalf("removed = %v, quiero que el segundo ítem se borre pese al primero", pr.removed)
	}
}

func TestRunWorktreesListsOnlyOwned(t *testing.T) {
	base, owned, foreign := worktreeFixture(t)
	pr := worktree.NewGitDirect(base)

	var stdout, stderr bytes.Buffer
	code := runWorktrees(pr, &stdout, &stderr, []string{"list"})
	if code != 0 {
		t.Fatalf("código de salida = %d (stderr: %s)", code, stderr.String())
	}
	out := stdout.String()
	for _, want := range []string{"prdash-pr-1", owned, "propia", "ok"} {
		if !strings.Contains(out, want) {
			t.Errorf("el listado no contiene %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, foreign) || strings.Contains(out, "otra-herramienta") {
		t.Errorf("el listado no debería incluir worktrees ajenos:\n%s", out)
	}
}

func TestRunWorktreesDefaultsToList(t *testing.T) {
	base, _, _ := worktreeFixture(t)
	var stdout, stderr bytes.Buffer
	code := runWorktrees(worktree.NewGitDirect(base), &stdout, &stderr, nil)
	out := stdout.String()
	if code != 0 || !strings.Contains(out, "prdash-pr-1") {
		t.Fatalf("code=%d out=%q stderr=%q", code, out, stderr.String())
	}
}

func TestRunWorktreesRemoveRefusesForeign(t *testing.T) {
	base, owned, foreign := worktreeFixture(t)
	pr := worktree.NewGitDirect(base)

	var stdout, stderr bytes.Buffer
	if code := runWorktrees(pr, &stdout, &stderr, []string{"remove", foreign}); code == 0 {
		t.Fatal("borrar un worktree ajeno debería fallar")
	}
	if !worktree.Exists(foreign) {
		t.Fatal("el worktree ajeno no debería haberse tocado")
	}
	if !worktree.Exists(owned) {
		t.Fatal("el worktree propio no debería tocarse al rechazar otro")
	}
}

func TestRunWorktreesRemoveOwned(t *testing.T) {
	base, owned, foreign := worktreeFixture(t)
	pr := worktree.NewGitDirect(base)

	var stdout, stderr bytes.Buffer
	if code := runWorktrees(pr, &stdout, &stderr, []string{"remove", owned}); code != 0 {
		t.Fatalf("código de salida = %d", code)
	}
	if worktree.Exists(owned) {
		t.Fatal("el worktree propio debería haberse borrado")
	}
	if !worktree.Exists(foreign) {
		t.Fatal("el worktree ajeno no debería tocarse")
	}
}

func TestRunWorktreesRemoveOrphan(t *testing.T) {
	repo, base, owned, _ := worktreeRepoFixture(t)
	if err := os.RemoveAll(repo); err != nil {
		t.Fatal(err)
	}
	pr := worktree.NewGitDirect(base)

	var stdout, stderr bytes.Buffer
	if code := runWorktrees(pr, &stdout, &stderr, []string{"remove", owned}); code != 0 {
		t.Fatalf("borrar un huérfano propio debería funcionar, code=%d", code)
	}
	if worktree.Exists(owned) {
		t.Fatal("el checkout huérfano debería haberse borrado")
	}
}

func TestRunWorktreesUsageErrors(t *testing.T) {
	base, _, _ := worktreeFixture(t)
	pr := worktree.NewGitDirect(base)

	var stdout, stderr bytes.Buffer
	if code := runWorktrees(pr, &stdout, &stderr, []string{"bogus"}); code != 2 {
		t.Fatalf("subcomando desconocido debería salir con 2, got %d", code)
	}
	if code := runWorktrees(pr, &stdout, &stderr, []string{"remove"}); code != 2 {
		t.Fatalf("remove sin rutas debería salir con 2, got %d", code)
	}
}

func TestRunWorktreesRemoveNonexistentRefused(t *testing.T) {
	base, owned, foreign := worktreeFixture(t)
	pr := worktree.NewGitDirect(base)
	missing := filepath.Join(base, "prdash-pr-404")

	var stdout, stderr bytes.Buffer
	code := runWorktrees(pr, &stdout, &stderr, []string{"remove", missing})
	if code != 1 {
		t.Fatalf("código de salida = %d, want 1", code)
	}
	if errOut := stderr.String(); !strings.Contains(errOut, "not a prdash worktree") {
		t.Errorf("stderr = %q, want el rechazo de la ruta", errOut)
	}
	if _, err := os.Stat(missing); !os.IsNotExist(err) {
		t.Errorf("no debería crearse la ruta inexistente: %v", err)
	}
	if !worktree.Exists(owned) || !worktree.Exists(foreign) {
		t.Error("un rechazo no debería tocar worktrees existentes")
	}
}

func TestRunWorktreesRemoveOrphansMalformedGitDoesNotBreakBatch(t *testing.T) {
	base, orphans, healthy, _ := worktreeOrphanFixture(t)
	malformed := orphans[0]
	if err := os.WriteFile(filepath.Join(malformed, ".git"), []byte("garbage\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	pr := worktree.NewGitDirect(base)

	var stdout, stderr bytes.Buffer
	code := runWorktrees(pr, &stdout, &stderr, []string{"remove", "--orphans"})
	out := stdout.String()
	if code != 0 {
		t.Fatalf("un huérfano con .git irresoluble no debería hacer fallar el lote: code=%d\n%s", code, out)
	}
	for _, o := range orphans {
		if worktree.Exists(o) {
			t.Errorf("%s debería haberse borrado", o)
		}
		if !strings.Contains(out, o) {
			t.Errorf("la salida no nombra %s:\n%s", o, out)
		}
	}
	if !worktree.Exists(healthy) {
		t.Error("el worktree sano no debería tocarse")
	}
}

func TestRunWorktreesRemoveTwoExplicitPaths(t *testing.T) {
	repo := filepath.Join(t.TempDir(), "repo")
	testutil.InitRepo(t, repo)
	testutil.CommitFile(t, repo, "base.txt", "base", "base")
	testutil.RunGit(t, repo, "branch", "uno")
	testutil.RunGit(t, repo, "branch", "dos")
	testutil.RunGit(t, repo, "branch", "ajena")

	base := t.TempDir()
	owned1 := filepath.Join(base, "prdash-pr-1")
	owned2 := filepath.Join(base, "prdash-pr-2")
	foreign := filepath.Join(base, "otra-herramienta")
	testutil.RunGit(t, repo, "worktree", "add", "--quiet", owned1, "uno")
	testutil.RunGit(t, repo, "worktree", "add", "--quiet", owned2, "dos")
	testutil.RunGit(t, repo, "worktree", "add", "--quiet", foreign, "ajena")

	pr := worktree.NewGitDirect(base)
	var stdout, stderr bytes.Buffer
	code := runWorktrees(pr, &stdout, &stderr, []string{"remove", owned1, owned2})
	out := stdout.String()
	if code != 0 {
		t.Fatalf("código de salida = %d, out=%q", code, out)
	}
	if worktree.Exists(owned1) || worktree.Exists(owned2) {
		t.Error("las dos rutas propias deberían haberse borrado")
	}
	if !worktree.Exists(foreign) {
		t.Error("el worktree ajeno no debería tocarse")
	}
}

func TestRunWorktreesRemoveOrphansDryRunExcludesOthers(t *testing.T) {
	base, orphans, healthy, foreign := worktreeOrphanFixture(t)
	pr := worktree.NewGitDirect(base)

	var stdout, stderr bytes.Buffer
	code := runWorktrees(pr, &stdout, &stderr, []string{"remove", "--orphans", "--dry-run"})
	out := stdout.String()
	if code != 0 {
		t.Fatalf("código de salida = %d, out=%q", code, out)
	}
	for _, o := range orphans {
		if !strings.Contains(out, o) {
			t.Errorf("el lote debería incluir %s:\n%s", o, out)
		}
	}
	if strings.Contains(out, healthy) {
		t.Errorf("el lote no debería incluir el worktree sano %s:\n%s", healthy, out)
	}
	if strings.Contains(out, foreign) {
		t.Errorf("el lote no debería incluir el worktree ajeno %s:\n%s", foreign, out)
	}
}

func TestRunWorktreesRemoveMalformedGitOrphanByPath(t *testing.T) {
	base, orphans, healthy, _ := worktreeOrphanFixture(t)
	malformed := orphans[0]
	if err := os.WriteFile(filepath.Join(malformed, ".git"), []byte("garbage\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	pr := worktree.NewGitDirect(base)

	var code int
	var stdout, stderr bytes.Buffer
	code = runWorktrees(pr, &stdout, &stderr, []string{"remove", malformed})
	out := stdout.String()
	if code != 0 {
		t.Fatalf("borrar por ruta un huérfano con .git irresoluble debería funcionar: code=%d\n%s", code, out)
	}
	if worktree.Exists(malformed) {
		t.Error("el huérfano nombrado debería haberse borrado")
	}
	if !worktree.Exists(orphans[1]) {
		t.Error("el otro huérfano no debería tocarse al borrar una sola ruta")
	}
	if !worktree.Exists(healthy) {
		t.Error("el worktree sano no debería tocarse")
	}
}

func TestRunWorktreesRemoveOrphans(t *testing.T) {
	base, orphans, healthy, foreign := worktreeOrphanFixture(t)
	pr := worktree.NewGitDirect(base)

	var stdout, stderr bytes.Buffer
	code := runWorktrees(pr, &stdout, &stderr, []string{"remove", "--orphans"})
	out := stdout.String()
	if code != 0 {
		t.Fatalf("código de salida = %d, out=%q", code, out)
	}
	for _, o := range orphans {
		if worktree.Exists(o) {
			t.Errorf("el huérfano %s debería haberse borrado", o)
		}
		if !strings.Contains(out, o) {
			t.Errorf("la salida no nombra el huérfano %s:\n%s", o, out)
		}
	}
	if !worktree.Exists(healthy) {
		t.Error("el worktree propio sano no debería borrarse")
	}
	if !worktree.Exists(foreign) {
		t.Error("el worktree ajeno no debería tocarse")
	}
}

func TestRunWorktreesRemoveOrphansDryRun(t *testing.T) {
	base, orphans, healthy, foreign := worktreeOrphanFixture(t)
	pr := worktree.NewGitDirect(base)

	var stdout, stderr bytes.Buffer
	code := runWorktrees(pr, &stdout, &stderr, []string{"remove", "--orphans", "--dry-run"})
	out := stdout.String()
	if code != 0 {
		t.Fatalf("código de salida = %d, out=%q", code, out)
	}
	for _, o := range orphans {
		if !strings.Contains(out, o) {
			t.Errorf("--dry-run debería imprimir %s:\n%s", o, out)
		}
		if !worktree.Exists(o) {
			t.Errorf("--dry-run no debería borrar %s", o)
		}
	}
	if !worktree.Exists(healthy) || !worktree.Exists(foreign) {
		t.Error("--dry-run no debería tocar ni el sano ni el ajeno")
	}
}

func TestRunWorktreesRemoveOrphansNoneIsSuccess(t *testing.T) {
	base, _, _ := worktreeFixture(t) // sano + ajeno, sin huérfanos
	pr := worktree.NewGitDirect(base)

	var stdout, stderr bytes.Buffer
	code := runWorktrees(pr, &stdout, &stderr, []string{"remove", "--orphans"})
	out, errOut := stdout.String(), stderr.String()
	if code != 0 {
		t.Fatalf("cero huérfanos debería salir con 0, got %d", code)
	}
	if !strings.Contains(out, "no orphan") {
		t.Errorf("debería informar de que no hay huérfanos:\n%s", out)
	}
	if errOut != "" {
		t.Errorf("stderr = %q, want vacío", errOut)
	}
}

func TestRunWorktreesRemoveOrphansDryRunNoneIsSuccess(t *testing.T) {
	base, _, _ := worktreeFixture(t)
	pr := worktree.NewGitDirect(base)

	var stdout, stderr bytes.Buffer
	code := runWorktrees(pr, &stdout, &stderr, []string{"remove", "--orphans", "--dry-run"})
	out, errOut := stdout.String(), stderr.String()
	if code != 0 {
		t.Fatalf("cero huérfanos con --dry-run debería salir con 0, got %d", code)
	}
	if strings.Contains(out, base) {
		t.Errorf("no debería imprimir ninguna ruta de huérfano:\n%s", out)
	}
	if errOut != "" {
		t.Errorf("stderr = %q, want vacío", errOut)
	}
}

func TestRunWorktreesRemoveOrphansUsageErrors(t *testing.T) {
	base, orphans, healthy, _ := worktreeOrphanFixture(t)
	pr := worktree.NewGitDirect(base)

	cases := []struct {
		name string
		args []string
		want string
	}{
		{"orphans y rutas", []string{"remove", "--orphans", healthy}, "cannot be mixed"},
		{"dry-run sin orphans", []string{"remove", "--dry-run"}, "--orphans"},
		{"flag desconocido", []string{"remove", "--bogus"}, "unknown flag"},
		{"typo de orphans", []string{"remove", "--orphan"}, "unknown flag"},
		{"ruta con guion inicial", []string{"remove", "-prdash-pr-1"}, "unknown flag"},
		{"sin rutas ni orphans", []string{"remove"}, "path"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var out, errOut bytes.Buffer
			code := runWorktrees(pr, &out, &errOut, tc.args)
			stderr := errOut.String()
			if code != 2 {
				t.Fatalf("código de salida = %d, want 2", code)
			}
			if !strings.Contains(stderr, tc.want) {
				t.Errorf("stderr = %q, want que mencione %q", stderr, tc.want)
			}
			for _, o := range orphans {
				if !worktree.Exists(o) {
					t.Errorf("un uso inválido no debería borrar %s", o)
				}
			}
			if !worktree.Exists(healthy) {
				t.Error("un uso inválido no debería borrar el worktree sano")
			}
		})
	}
}
