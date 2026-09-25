package executor

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"prdash/internal/forge/model"
	"prdash/internal/herdr"
	"prdash/internal/reporesolver"
	"prdash/internal/review/plan"
	"prdash/internal/testutil"
	"prdash/internal/worktree"
)

func githubRef() model.RepoRef {
	return model.RepoRef{Forge: "github", Host: "github.com", Project: "acme/widget", Owner: "acme", Name: "widget"}
}

func item(ref model.RepoRef, number int) model.Item {
	it := model.NewItem(ref, number)
	it.URL = fmt.Sprintf("https://github.com/%s/pull/%d", ref.Project, number)
	return it
}

// baseFixture crea un bare origin y un clon local conectado.
func baseFixture(t *testing.T) (origin, repo string) {
	t.Helper()
	origin = filepath.Join(t.TempDir(), "origin.git")
	testutil.InitBare(t, origin)
	repo = filepath.Join(t.TempDir(), "repo")
	testutil.InitRepo(t, repo)
	testutil.CommitFile(t, repo, "base.txt", "base", "base")
	testutil.SetRemote(t, repo, "origin", origin)
	testutil.Push(t, repo, "-u", "origin", "main")
	return origin, repo
}

// pushPR publica un commit como ref de review `refs/pull/<n>/head` en el origin.
func pushPR(t *testing.T, origin string, number int, content string) {
	t.Helper()
	work := filepath.Join(t.TempDir(), "pr")
	testutil.InitRepo(t, work)
	testutil.CommitFile(t, work, fmt.Sprintf("pr-%d.txt", number), content, "pr")
	testutil.Push(t, work, origin, fmt.Sprintf("HEAD:refs/pull/%d/head", number))
}

// harness junta un executor real (resolutor + worktree git directo) sobre
// repos locales, sin red.
type harness struct {
	origin   string
	ref      model.RepoRef
	cloneDir string
	wtDir    string
	resolver *reporesolver.Resolver
	pr       *worktree.GitDirect
	ex       *Executor
}

type harnessOpts struct {
	origin   string
	roots    []string
	cloneURL func(model.RepoRef) string
	herdr    HerdrPort
}

func newHarness(t *testing.T, opts harnessOpts) *harness {
	t.Helper()
	ref := githubRef()
	cloneDir := filepath.Join(t.TempDir(), "repos")
	wtDir := filepath.Join(t.TempDir(), "worktrees")
	memoPath := filepath.Join(t.TempDir(), "memo.json")

	cloneURL := opts.cloneURL
	if cloneURL == nil {
		origin := opts.origin
		cloneURL = func(model.RepoRef) string { return origin }
	}
	resolver := reporesolver.New(reporesolver.Options{
		Roots:       opts.roots,
		CloneDir:    cloneDir,
		WorktreeDir: wtDir,
		MemoPath:    memoPath,
		CloneURL:    cloneURL,
		ParseRemote: func(raw string) (model.RepoRef, bool) {
			if strings.TrimSpace(raw) == opts.origin {
				return ref, true
			}
			return model.RepoRef{}, false
		},
	})
	pr := worktree.NewGitDirect(wtDir)
	ex := &Executor{
		Resolver:  resolver,
		Worktrees: pr,
		Herdr:     opts.herdr,
		Tools:     plan.Tools{Tuicr: []string{"tuicr"}, Hunk: []string{"hunk"}, Agent: []string{"opencode"}},
	}
	return &harness{origin: opts.origin, ref: ref, cloneDir: cloneDir, wtDir: wtDir, resolver: resolver, pr: pr, ex: ex}
}

// Scenario: Worktree desde un repo ya local.
func TestMountFromLocalRepoRegistersActiveReview(t *testing.T) {
	origin, repo := baseFixture(t)
	pushPR(t, origin, 7, "uno")
	h := newHarness(t, harnessOpts{origin: origin, roots: []string{filepath.Dir(repo)}})
	it := item(h.ref, 7)

	res, err := h.ex.Mount(context.Background(), it)
	if err != nil {
		t.Fatalf("Mount: %v", err)
	}
	if res.RepoPath != repo {
		t.Fatalf("RepoPath = %q, quiero el clon local %q", res.RepoPath, repo)
	}
	if res.Reused {
		t.Fatal("el primer montaje no reutiliza")
	}
	if !worktree.Exists(res.Worktree.Path) {
		t.Fatalf("el worktree no existe en %q", res.Worktree.Path)
	}
	if res.Branch != "prdash/pr-7" {
		t.Fatalf("rama = %q", res.Branch)
	}
	if got := testutil.RunGit(t, res.Worktree.Path, "rev-parse", "--abbrev-ref", "HEAD"); got != "prdash/pr-7" {
		t.Fatalf("HEAD del worktree = %q", got)
	}
	if _, ok := h.ex.ActiveReview(it); !ok {
		t.Fatal("el review debería quedar registrado como activo")
	}
}

// Scenario: Repo no clonado se clona en bare y se saca el worktree del clon.
func TestMountClonesBareWhenRepoNotLocal(t *testing.T) {
	origin, _ := baseFixture(t)
	pushPR(t, origin, 8, "ocho")
	h := newHarness(t, harnessOpts{origin: origin}) // sin roots: no hay clon local
	it := item(h.ref, 8)

	res, err := h.ex.Mount(context.Background(), it)
	if err != nil {
		t.Fatalf("Mount: %v", err)
	}
	wantBare := filepath.Join(h.cloneDir, "github", "github.com", "acme", "widget")
	if res.RepoPath != wantBare {
		t.Fatalf("RepoPath = %q, quiero %q", res.RepoPath, wantBare)
	}
	if got := testutil.RunGit(t, wantBare, "rev-parse", "--is-bare-repository"); got != "true" {
		t.Fatalf("el clon no es bare: %q", got)
	}
	if !worktree.Exists(res.Worktree.Path) {
		t.Fatal("falta el worktree sacado del clon bare")
	}
	if !strings.HasPrefix(res.Worktree.Path, h.wtDir) {
		t.Fatalf("el destino del worktree debe ser configurable: %q", res.Worktree.Path)
	}
}

// Scenario: PR de fork cuya rama no existe en origin.
func TestMountForkFetchesReviewRef(t *testing.T) {
	origin, repo := baseFixture(t)
	pushPR(t, origin, 9, "del fork")
	if testutil.RefExists(t, origin, "refs/heads/feature") {
		t.Fatal("la rama del fork no debería existir como rama normal de origin")
	}
	h := newHarness(t, harnessOpts{origin: origin, roots: []string{filepath.Dir(repo)}})
	it := item(h.ref, 9)

	res, err := h.ex.Mount(context.Background(), it)
	if err != nil {
		t.Fatalf("Mount: %v", err)
	}
	if res.Branch != "prdash/pr-9" {
		t.Fatalf("rama local del fork = %q", res.Branch)
	}
	if _, err := os.Stat(filepath.Join(res.Worktree.Path, "pr-9.txt")); err != nil {
		t.Fatalf("el worktree no trae el contenido del ref de fork: %v", err)
	}
	// La rama de origen sigue sin existir en origin: se trabajó sobre el ref de review.
	if testutil.RefExists(t, origin, "refs/heads/feature") {
		t.Fatal("no debería haberse creado una rama de origin")
	}
}

// Scenario: Reusar el worktree existente de un ítem.
func TestMountReusesExistingWorktree(t *testing.T) {
	origin, repo := baseFixture(t)
	pushPR(t, origin, 7, "uno")
	h := newHarness(t, harnessOpts{origin: origin, roots: []string{filepath.Dir(repo)}})
	it := item(h.ref, 7)
	ctx := context.Background()

	first, err := h.ex.Mount(ctx, it)
	if err != nil {
		t.Fatalf("Mount 1: %v", err)
	}
	second, err := h.ex.Mount(ctx, it)
	if err != nil {
		t.Fatalf("Mount 2: %v", err)
	}
	if !second.Reused {
		t.Fatal("el segundo montaje debería reutilizar")
	}
	if first.Worktree.Path != second.Worktree.Path {
		t.Fatalf("rutas distintas: %q vs %q", first.Worktree.Path, second.Worktree.Path)
	}
	if list := h.pr.List(ctx); len(list) != 1 {
		t.Fatalf("no debería duplicar el worktree: %+v", list)
	}
}

// Scenario: Varios PRs del mismo repo no chocan.
func TestMountTwoPRsSameRepoCoexist(t *testing.T) {
	origin, repo := baseFixture(t)
	pushPR(t, origin, 1, "uno")
	pushPR(t, origin, 2, "dos")
	h := newHarness(t, harnessOpts{origin: origin, roots: []string{filepath.Dir(repo)}})
	ctx := context.Background()

	r1, err := h.ex.Mount(ctx, item(h.ref, 1))
	if err != nil {
		t.Fatalf("Mount 1: %v", err)
	}
	r2, err := h.ex.Mount(ctx, item(h.ref, 2))
	if err != nil {
		t.Fatalf("Mount 2: %v", err)
	}
	if r1.Worktree.Path == r2.Worktree.Path {
		t.Fatal("cada ítem debe tener su propio worktree en una ruta distinta")
	}
	if !worktree.Exists(r1.Worktree.Path) || !worktree.Exists(r2.Worktree.Path) {
		t.Fatal("ambos worktrees deben coexistir")
	}
	if list := h.pr.List(ctx); len(list) != 2 {
		t.Fatalf("List = %+v, quiero 2", list)
	}
}

// Scenario: Sin permisos de clon o fetch, el fallo es claro y no deja basura.
func TestMountCloneFailureLeavesNoGarbage(t *testing.T) {
	h := newHarness(t, harnessOpts{
		origin: filepath.Join(t.TempDir(), "privado.git"), // no existe / sin acceso
		cloneURL: func(model.RepoRef) string {
			return filepath.Join(t.TempDir(), "sin-permiso.git")
		},
	})
	it := item(h.ref, 5)

	_, err := h.ex.Mount(context.Background(), it)
	if err == nil {
		t.Fatal("esperaba un error claro de clonado")
	}
	bare := filepath.Join(h.cloneDir, "github", "github.com", "acme", "widget")
	if _, statErr := os.Stat(bare); !os.IsNotExist(statErr) {
		t.Fatalf("no debería quedar clon bare: %v", statErr)
	}
	if leftoverTemps(h.cloneDir) != 0 {
		t.Fatalf("quedaron temporales de clonado en %s", h.cloneDir)
	}
	if list := h.pr.List(context.Background()); len(list) != 0 {
		t.Fatalf("no debería quedar worktree: %+v", list)
	}
}

func TestMountFetchFailureCleansNewBare(t *testing.T) {
	origin, _ := baseFixture(t) // origin sin refs de review
	h := newHarness(t, harnessOpts{origin: origin})
	it := item(h.ref, 42)

	_, err := h.ex.Mount(context.Background(), it)
	if err == nil {
		t.Fatal("esperaba un error de fetch del ref de review")
	}
	bare := filepath.Join(h.cloneDir, "github", "github.com", "acme", "widget")
	if _, statErr := os.Stat(bare); !os.IsNotExist(statErr) {
		t.Fatalf("un clon bare recién creado debe limpiarse si el fetch falla: %v", statErr)
	}
	if list := h.pr.List(context.Background()); len(list) != 0 {
		t.Fatalf("no debería quedar worktree: %+v", list)
	}
}

// Scenario: Una herramienta ausente no tumba el layout (seam del puerto Herdr).
func TestMountWithoutHerdrStillProvisionsWorktree(t *testing.T) {
	origin, repo := baseFixture(t)
	pushPR(t, origin, 3, "tres")
	h := newHarness(t, harnessOpts{origin: origin, roots: []string{filepath.Dir(repo)}})
	h.ex.Env = plan.Env{Available: map[string]bool{"tuicr": true, "agent": true}}

	res, err := h.ex.Mount(context.Background(), item(h.ref, 3))
	if err != nil {
		t.Fatalf("Mount: %v", err)
	}
	if res.Herdr {
		t.Fatal("sin Herdr no debería reportarse layout montado")
	}
	if !worktree.Exists(res.Worktree.Path) {
		t.Fatal("el worktree debe montarse igualmente sin Herdr")
	}
	if len(res.Plan.Panes) != 2 {
		t.Fatalf("el pane de Hunk ausente debería omitirse: %+v", res.Plan.Panes)
	}
}

func TestMountWithHerdrAppliesPlan(t *testing.T) {
	origin, repo := baseFixture(t)
	pushPR(t, origin, 4, "cuatro")
	herdr := &fakeHerdr{available: true}
	h := newHarness(t, harnessOpts{origin: origin, roots: []string{filepath.Dir(repo)}, herdr: herdr})

	res, err := h.ex.Mount(context.Background(), item(h.ref, 4))
	if err != nil {
		t.Fatalf("Mount: %v", err)
	}
	if !res.Herdr || !herdr.mounted {
		t.Fatalf("Herdr debería haber montado el plan: %+v", herdr)
	}
	if len(herdr.plan.Panes) != 3 {
		t.Fatalf("el plan montado = %+v", herdr.plan)
	}
	if len(herdr.notified) == 0 {
		t.Fatal("un layout montado debería notificar")
	}
}

// TestMountPassesNativeContainerToLayout comprueba que el contenedor que
// devuelve la provisión nativa (workspace + root pane) llega al puerto Herdr.
func TestMountPassesNativeContainerToLayout(t *testing.T) {
	origin, repo := baseFixture(t)
	pushPR(t, origin, 6, "seis")
	herdr := &fakeHerdr{available: true}
	h := newHarness(t, harnessOpts{origin: origin, roots: []string{filepath.Dir(repo)}, herdr: herdr})
	h.ex.Worktrees = &fakeProvisioner{wt: worktree.Worktree{
		ID: "wt", Label: "prdash-pr-6", Path: "/tmp/wt-prdash-pr-6",
		Branch: "prdash/pr-6", Repo: repo, WorkspaceID: "w30", RootPaneID: "w30:p1",
	}}

	if _, err := h.ex.Mount(context.Background(), item(h.ref, 6)); err != nil {
		t.Fatalf("Mount: %v", err)
	}
	if herdr.container.WorkspaceID != "w30" || herdr.container.PaneID != "w30:p1" {
		t.Fatalf("contenedor = %+v", herdr.container)
	}
}

// fakeProvisioner devuelve un worktree fijo (contenedor nativo simulado).
type fakeProvisioner struct{ wt worktree.Worktree }

func (f *fakeProvisioner) Create(context.Context, worktree.Spec) (worktree.Worktree, error) {
	return f.wt, nil
}
func (f *fakeProvisioner) Remove(context.Context, string) error     { return nil }
func (f *fakeProvisioner) List(context.Context) []worktree.Worktree { return nil }
func (f *fakeProvisioner) Audit(context.Context) []worktree.Entry   { return nil }

// fakeHerdr es un doble en memoria del puerto Herdr.
type fakeHerdr struct {
	available bool
	mounted   bool
	container herdr.Container
	plan      plan.Plan
	notified  []string
}

func (f *fakeHerdr) Available() bool { return f.available }

func (f *fakeHerdr) MountLayout(_ context.Context, c herdr.Container, pl plan.Plan) ([]string, error) {
	f.mounted = true
	f.container = c
	f.plan = pl
	return nil, nil
}

func (f *fakeHerdr) Notify(_ context.Context, title string, _ herdr.NotifyOptions) error {
	f.notified = append(f.notified, title)
	return nil
}

func leftoverTemps(dir string) int {
	n := 0
	_ = filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		if strings.Contains(d.Name(), ".tmp-") {
			n++
		}
		return nil
	})
	return n
}
