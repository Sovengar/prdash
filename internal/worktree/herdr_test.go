package worktree

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"prdash/internal/herdr"
	"prdash/internal/testutil"
)

var errWorkspaceProbe = errors.New("socket closed")

type fakeRunner struct {
	available    bool
	createInfo   herdr.WorktreeInfo
	createErr    error
	createCalls  []herdr.WorktreeSpec
	listResult   []herdr.WorktreeInfo
	removeCalls  []string
	removeErr    error
	workspace    herdr.WorkspaceInfo
	workspaceErr error
	wsCalls      []herdr.WorkspaceSpec
	panes        map[string][]herdr.PaneInfo
	panesErr     map[string]error
	paneListArgs []string
}

func (f *fakeRunner) Available() bool { return f.available }

func (f *fakeRunner) WorktreeCreate(_ context.Context, spec herdr.WorktreeSpec) (herdr.WorktreeInfo, error) {
	f.createCalls = append(f.createCalls, spec)
	return f.createInfo, f.createErr
}

func (f *fakeRunner) WorktreeList(context.Context, string) ([]herdr.WorktreeInfo, error) {
	return f.listResult, nil
}

func (f *fakeRunner) WorktreeRemove(_ context.Context, workspaceID string, _ bool) error {
	f.removeCalls = append(f.removeCalls, workspaceID)
	return f.removeErr
}

func (f *fakeRunner) WorkspaceCreate(_ context.Context, spec herdr.WorkspaceSpec) (herdr.WorkspaceInfo, error) {
	f.wsCalls = append(f.wsCalls, spec)
	return f.workspace, f.workspaceErr
}

func (f *fakeRunner) PaneList(_ context.Context, workspaceID string) ([]herdr.PaneInfo, error) {
	f.paneListArgs = append(f.paneListArgs, workspaceID)
	if err, ok := f.panesErr[workspaceID]; ok {
		return nil, err
	}
	return f.panes[workspaceID], nil
}

func TestSelectPicksNativeInsideHerdr(t *testing.T) {
	if _, ok := Select(&fakeRunner{available: true}, t.TempDir()).(*HerdrNative); !ok {
		t.Fatal("dentro de Herdr debería elegirse la provisión nativa")
	}
	if _, ok := Select(&fakeRunner{available: false}, t.TempDir()).(*GitDirect); !ok {
		t.Fatal("fuera de Herdr debería elegirse git directo")
	}
	if _, ok := Select(nil, t.TempDir()).(*GitDirect); !ok {
		t.Fatal("sin cliente de Herdr debería elegirse git directo")
	}
}

func TestHerdrNativeCreateMapsContainer(t *testing.T) {
	base := filepath.Join(t.TempDir(), "worktrees")
	dest := filepath.Join(base, "github", "github.com", "acme", "widget", "prdash-pr-7")
	runner := &fakeRunner{
		available: true,
		createInfo: herdr.WorktreeInfo{
			WorkspaceID: "w18", TabID: "w18:t1", RootPaneID: "w18:p1",
			Path: dest, Branch: "prdash/pr-7", Label: "prdash-pr-7",
		},
	}
	h := NewHerdrNative(runner, base)

	wt, err := h.Create(context.Background(), Spec{Repo: "/repo", Branch: "prdash/pr-7", Path: dest, Label: "prdash-pr-7"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if wt.WorkspaceID != "w18" || wt.RootPaneID != "w18:p1" {
		t.Fatalf("contenedor = %+v", wt)
	}
	if wt.Path != dest || wt.Branch != "prdash/pr-7" || wt.Label != "prdash-pr-7" {
		t.Fatalf("worktree = %+v", wt)
	}
	if len(runner.createCalls) != 1 {
		t.Fatalf("createCalls = %+v", runner.createCalls)
	}
	if got := runner.createCalls[0]; !got.NoFocus || got.Cwd != "/repo" || got.Branch != "prdash/pr-7" || got.Path != dest {
		t.Fatalf("spec nativo = %+v", got)
	}
}

func TestHerdrNativeCreateKeepsOwnershipLabel(t *testing.T) {
	base := t.TempDir()
	dest := filepath.Join(base, "prdash-pr-7")
	runner := &fakeRunner{
		available: true,
		createInfo: herdr.WorktreeInfo{
			WorkspaceID: "w18", RootPaneID: "w18:p1",
			Path: dest, Branch: "prdash/pr-7",
			WorkspaceLabel: "prdash-pr-7", Label: "origin.git",
		},
	}
	wt, err := NewHerdrNative(runner, base).Create(context.Background(), Spec{
		Repo: "/repo", Branch: "prdash/pr-7", Path: dest, Label: "prdash-pr-7",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if wt.Label != "prdash-pr-7" {
		t.Fatalf("Label = %q, quiero la etiqueta de ownership", wt.Label)
	}
}

func TestHerdrNativeCreateReusesExistingWorktree(t *testing.T) {
	repo := newRepo(t)
	testutil.RunGit(t, repo, "branch", "feature")

	base := t.TempDir()
	dest := filepath.Join(base, "prdash-pr-1")
	if _, err := NewGitDirect(base).Create(context.Background(), Spec{Repo: repo, Branch: "feature", Path: dest, Label: "prdash-pr-1"}); err != nil {
		t.Fatalf("preparar worktree: %v", err)
	}

	runner := &fakeRunner{
		available:  true,
		listResult: []herdr.WorktreeInfo{{Path: dest, Branch: "feature", Label: "origin.git", OpenWorkspaceID: "w19"}},
		panes:      map[string][]herdr.PaneInfo{"w19": {{PaneID: "w19:p1", WorkspaceID: "w19", TabID: "w19:t1"}}},
	}
	h := NewHerdrNative(runner, base)

	wt, err := h.Create(context.Background(), Spec{Repo: repo, Branch: "feature", Path: dest, Label: "prdash-pr-1"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if len(runner.createCalls) != 0 {
		t.Fatal("un worktree existente no debería volver a crearse")
	}
	if wt.WorkspaceID != "w19" || wt.RootPaneID != "w19:p1" {
		t.Fatalf("debería resolver el workspace abierto: %+v", wt)
	}
	if wt.Label != "prdash-pr-1" {
		t.Fatalf("la etiqueta de ownership no debería pisarse con el nombre del repo: %q", wt.Label)
	}
	if list := h.List(context.Background()); len(list) != 1 || list[0].Path != dest {
		t.Fatalf("List = %+v", list)
	}
}

// The checkout is on disk from an earlier session but its Herdr workspace is closed, so
// `worktree list` returns no open_workspace_id: this is what made the review show up as a loose
// workspace.
func TestHerdrNativeReuseAdoptsCheckoutWithNoOpenWorkspace(t *testing.T) {
	repo := newRepo(t)
	testutil.RunGit(t, repo, "branch", "feature")

	base := t.TempDir()
	dest := filepath.Join(base, "prdash-pr-1")
	if _, err := NewGitDirect(base).Create(context.Background(), Spec{Repo: repo, Branch: "feature", Path: dest, Label: "prdash-pr-1"}); err != nil {
		t.Fatalf("preparar worktree: %v", err)
	}

	runner := &fakeRunner{
		available:  true,
		listResult: []herdr.WorktreeInfo{{Path: dest, Branch: "feature", Label: "origin.git"}}, // sin open_workspace_id
		workspace:  herdr.WorkspaceInfo{WorkspaceID: "w1C", TabID: "w1C:t1", RootPaneID: "w1C:p1"},
	}

	wt, err := NewHerdrNative(runner, base).Create(context.Background(), Spec{Repo: repo, Branch: "feature", Path: dest, Label: "prdash-pr-1"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if len(runner.createCalls) != 0 {
		t.Fatal("un worktree existente no debería volver a crearse")
	}
	if len(runner.wsCalls) != 1 {
		t.Fatalf("debería abrir un workspace para el checkout: %+v", runner.wsCalls)
	}
	if got := runner.wsCalls[0]; got.Cwd != dest || !got.NoFocus || got.Label != "prdash-pr-1" {
		t.Fatalf("spec del workspace = %+v", got)
	}
	if wt.WorkspaceID != "w1C" || wt.RootPaneID != "w1C:p1" {
		t.Fatalf("contenedor = %+v", wt)
	}
}

func TestHerdrNativeReuseFailsWhenWorkspaceCannotBeOpened(t *testing.T) {
	repo := newRepo(t)
	testutil.RunGit(t, repo, "branch", "feature")

	base := t.TempDir()
	dest := filepath.Join(base, "prdash-pr-1")
	if _, err := NewGitDirect(base).Create(context.Background(), Spec{Repo: repo, Branch: "feature", Path: dest, Label: "prdash-pr-1"}); err != nil {
		t.Fatalf("preparar worktree: %v", err)
	}

	runner := &fakeRunner{
		available:    true,
		listResult:   []herdr.WorktreeInfo{{Path: dest, Branch: "feature"}},
		workspaceErr: errWorkspaceProbe,
	}
	_, err := NewHerdrNative(runner, base).Create(context.Background(), Spec{Repo: repo, Branch: "feature", Path: dest, Label: "prdash-pr-1"})
	if err == nil {
		t.Fatal("sin workspace no debería devolverse un worktree sin contenedor")
	}
	if !strings.Contains(err.Error(), dest) {
		t.Fatalf("el error debería nombrar la ruta a limpiar: %v", err)
	}
}

// Herdr can return an open_workspace_id that no longer exists: it keeps it in its persisted session
// and a closed workspace leaves it pointing at nothing.
func TestHerdrNativeReuseIgnoresStaleOpenWorkspaceId(t *testing.T) {
	repo := newRepo(t)
	testutil.RunGit(t, repo, "branch", "feature")

	base := t.TempDir()
	dest := filepath.Join(base, "prdash-pr-13")
	if _, err := NewGitDirect(base).Create(context.Background(), Spec{Repo: repo, Branch: "feature", Path: dest, Label: "prdash-pr-13"}); err != nil {
		t.Fatalf("preparar worktree: %v", err)
	}

	runner := &fakeRunner{
		available:  true,
		listResult: []herdr.WorktreeInfo{{Path: dest, Branch: "feature", OpenWorkspaceID: "w1B"}},
		panesErr:   map[string]error{"w1B": errWorkspaceProbe}, // Herdr: workspace_not_found
		workspace:  herdr.WorkspaceInfo{WorkspaceID: "w1D", TabID: "w1D:t1", RootPaneID: "w1D:p1"},
	}

	wt, err := NewHerdrNative(runner, base).Create(context.Background(), Spec{Repo: repo, Branch: "feature", Path: dest, Label: "prdash-pr-13"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if len(runner.paneListArgs) == 0 {
		t.Fatal("un open_workspace_id debería comprobarse antes de confiar en él")
	}
	if len(runner.wsCalls) != 1 || runner.wsCalls[0].Cwd != dest {
		t.Fatalf("un workspace obsoleto debería cair en la adopción: %+v", runner.wsCalls)
	}
	if wt.WorkspaceID != "w1D" || wt.RootPaneID != "w1D:p1" {
		t.Fatalf("contenedor = %+v", wt)
	}
}

func TestHerdrNativeReuseUsesRootPaneOfOpenWorkspace(t *testing.T) {
	repo := newRepo(t)
	testutil.RunGit(t, repo, "branch", "feature")

	base := t.TempDir()
	dest := filepath.Join(base, "prdash-pr-1")
	if _, err := NewGitDirect(base).Create(context.Background(), Spec{Repo: repo, Branch: "feature", Path: dest, Label: "prdash-pr-1"}); err != nil {
		t.Fatalf("preparar worktree: %v", err)
	}

	runner := &fakeRunner{
		available:  true,
		listResult: []herdr.WorktreeInfo{{Path: dest, Branch: "feature", OpenWorkspaceID: "w19"}},
		panes:      map[string][]herdr.PaneInfo{"w19": {{PaneID: "w19:p1", WorkspaceID: "w19", TabID: "w19:t1"}}},
	}

	wt, err := NewHerdrNative(runner, base).Create(context.Background(), Spec{Repo: repo, Branch: "feature", Path: dest, Label: "prdash-pr-1"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if len(runner.wsCalls) != 0 {
		t.Fatalf("un workspace vivo no debería abrir otro: %+v", runner.wsCalls)
	}
	if wt.WorkspaceID != "w19" || wt.RootPaneID != "w19:p1" {
		t.Fatalf("contenedor = %+v", wt)
	}
}

func TestHerdrNativeRemoveUsesWorkspace(t *testing.T) {
	repo := newRepo(t)
	testutil.RunGit(t, repo, "branch", "feature")
	base := t.TempDir()
	dest := filepath.Join(base, "prdash-pr-1")
	if _, err := NewGitDirect(base).Create(context.Background(), Spec{Repo: repo, Branch: "feature", Path: dest, Label: "prdash-pr-1"}); err != nil {
		t.Fatalf("preparar worktree: %v", err)
	}

	runner := &fakeRunner{
		available:  true,
		listResult: []herdr.WorktreeInfo{{Path: dest, OpenWorkspaceID: "w21"}},
	}
	h := NewHerdrNative(runner, base)
	if err := h.Remove(context.Background(), dest); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if len(runner.removeCalls) != 1 || runner.removeCalls[0] != "w21" {
		t.Fatalf("removeCalls = %v", runner.removeCalls)
	}
}

func TestHerdrNativeRemoveRefusesPathOutsideBase(t *testing.T) {
	repo := newRepo(t)
	testutil.RunGit(t, repo, "branch", "feature")
	outside := filepath.Join(t.TempDir(), "prdash-pr-1")
	testutil.RunGit(t, repo, "worktree", "add", "--quiet", outside, "feature")

	runner := &fakeRunner{
		available:  true,
		listResult: []herdr.WorktreeInfo{{Path: outside, OpenWorkspaceID: "w99"}},
	}
	h := NewHerdrNative(runner, t.TempDir()) // raíz gestionada distinta
	if err := h.Remove(context.Background(), outside); err == nil {
		t.Fatal("no debería borrar fuera de la raíz gestionada")
	}
	if len(runner.removeCalls) != 0 {
		t.Fatalf("no debería llamar al cliente nativo: %v", runner.removeCalls)
	}
	if _, err := os.Stat(outside); err != nil {
		t.Fatalf("el worktree fuera de la raíz no debería tocarse: %v", err)
	}
}

func TestHerdrNativeRemoveIfCleanRefusesPathOutsideBase(t *testing.T) {
	repo := newRepo(t)
	testutil.RunGit(t, repo, "branch", "feature")
	outside := filepath.Join(t.TempDir(), "prdash-pr-1")
	testutil.RunGit(t, repo, "worktree", "add", "--quiet", outside, "feature")

	runner := &fakeRunner{
		available:  true,
		listResult: []herdr.WorktreeInfo{{Path: outside, OpenWorkspaceID: "w99"}},
	}
	h := NewHerdrNative(runner, t.TempDir())
	removed, _, err := h.RemoveIfClean(context.Background(), outside)
	if err == nil || removed {
		t.Fatalf("RemoveIfClean = (%v, _, %v), quiero rechazo", removed, err)
	}
	if len(runner.removeCalls) != 0 {
		t.Fatalf("no debería llamar al cliente nativo: %v", runner.removeCalls)
	}
	if _, err := os.Stat(outside); err != nil {
		t.Fatalf("el worktree fuera de la raíz no debería tocarse: %v", err)
	}
}

func TestHerdrNativeReuseRejectsBranchMismatch(t *testing.T) {
	repo := newRepo(t)
	testutil.RunGit(t, repo, "branch", "feature-1")
	testutil.RunGit(t, repo, "branch", "feature-2")

	base := t.TempDir()
	dest := filepath.Join(base, "prdash-pr-1")
	if _, err := NewGitDirect(base).Create(context.Background(), Spec{Repo: repo, Branch: "feature-1", Path: dest, Label: "prdash-pr-1"}); err != nil {
		t.Fatalf("preparar worktree: %v", err)
	}

	h := NewHerdrNative(&fakeRunner{available: true}, base)
	_, err := h.Create(context.Background(), Spec{Repo: repo, Branch: "feature-2", Path: dest, Label: "prdash-pr-1"})
	if err == nil || !strings.Contains(err.Error(), "feature-1") {
		t.Fatalf("esperaba error por rama ya presente, got %v", err)
	}
}

func TestHerdrNativeCreateIncompleteSpecErrors(t *testing.T) {
	h := NewHerdrNative(&fakeRunner{available: true}, t.TempDir())
	if _, err := h.Create(context.Background(), Spec{Repo: "x"}); err == nil {
		t.Fatal("spec incompleto debería fallar")
	}
}

func herdrCleanWorktree(t *testing.T) (base, dest string) {
	t.Helper()
	repo := newRepo(t)
	testutil.RunGit(t, repo, "branch", "feature")
	base = t.TempDir()
	dest = filepath.Join(base, "prdash-pr-1")
	if _, err := NewGitDirect(base).Create(context.Background(), Spec{Repo: repo, Branch: "feature", Path: dest, Label: "prdash-pr-1"}); err != nil {
		t.Fatalf("preparar worktree: %v", err)
	}
	return base, dest
}

func TestHerdrNativeRemoveIfCleanDeletesViaNativeWorkspace(t *testing.T) {
	base, dest := herdrCleanWorktree(t)
	runner := &fakeRunner{
		available:  true,
		listResult: []herdr.WorktreeInfo{{Path: dest, OpenWorkspaceID: "w21"}},
	}

	removed, reason, err := NewHerdrNative(runner, base).RemoveIfClean(context.Background(), dest)
	if err != nil || !removed || reason != "" {
		t.Fatalf("RemoveIfClean = (%v, %q, %v), quiero borrado limpio", removed, reason, err)
	}
	if len(runner.removeCalls) != 1 || runner.removeCalls[0] != "w21" {
		t.Fatalf("removeCalls = %v, quiero el borrado nativo del workspace", runner.removeCalls)
	}
}

func TestHerdrNativeRemoveIfCleanKeepsDirty(t *testing.T) {
	base, dest := herdrCleanWorktree(t)
	if err := os.WriteFile(filepath.Join(dest, "dirty.txt"), []byte("sin commitear"), 0o644); err != nil {
		t.Fatal(err)
	}
	runner := &fakeRunner{
		available:  true,
		listResult: []herdr.WorktreeInfo{{Path: dest, OpenWorkspaceID: "w21"}},
	}

	removed, reason, err := NewHerdrNative(runner, base).RemoveIfClean(context.Background(), dest)
	if err != nil || removed || reason != KeptUncommitted {
		t.Fatalf("RemoveIfClean = (%v, %q, %v), quiero conservado por sucio", removed, reason, err)
	}
	if len(runner.removeCalls) != 0 {
		t.Fatalf("un checkout sucio no debería llamar al borrado nativo: %v", runner.removeCalls)
	}
	if _, err := os.Stat(dest); err != nil {
		t.Fatalf("el worktree sucio no debería tocarse: %v", err)
	}
}

func TestHerdrNativeRemoveIfCleanPropagatesRemoveError(t *testing.T) {
	base, dest := herdrCleanWorktree(t)
	runner := &fakeRunner{
		available:  true,
		listResult: []herdr.WorktreeInfo{{Path: dest, OpenWorkspaceID: "w21"}},
		removeErr:  errors.New("workspace busy"),
	}

	removed, _, err := NewHerdrNative(runner, base).RemoveIfClean(context.Background(), dest)
	if err == nil || removed {
		t.Fatalf("RemoveIfClean = (%v, _, %v), quiero error propagado", removed, err)
	}
	if _, err := os.Stat(dest); err != nil {
		t.Fatalf("un fallo de borrado no debería tocar el checkout: %v", err)
	}
}

// The narrow band of destinations where Exists is true but the destination is not a worktree is
// what decides between `herdr worktree create` and `reuse`.
func TestSobreUnDestinoQueNoEsWorktreeNoSeCreaEncimaNiSeAdopta(t *testing.T) {
	destino := filepath.Join(t.TempDir(), "prdash-pr-7")
	if err := os.MkdirAll(destino, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(destino, ".git"),
		[]byte("gitdir: /no/existe\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(destino, "trabajo.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	f := &fakeRunner{available: true}
	h := NewHerdrNative(f, t.TempDir())

	wt, err := h.Create(context.Background(), Spec{
		Repo:   filepath.Join(t.TempDir(), "repo"),
		Branch: "feat/x",
		Path:   destino,
		Label:  "prdash-pr-7",
	})
	if err == nil {
		t.Fatalf("un destino que no es worktree dio nil y un worktree: %+v", wt)
	}
	if len(f.createCalls) != 0 {
		t.Errorf("se llamo a `worktree create` %d veces sobre un destino que ya existe",
			len(f.createCalls))
	}
	if _, err := os.Stat(filepath.Join(destino, "trabajo.txt")); err != nil {
		t.Errorf("el directorio del usuario cambio: %v", err)
	}
}

// Adopting another branch's checkout would mount the review on code nobody chose and present it as
// if it were the item's.
func TestAdoptarUnWorktreeDeOtraRamaSeNiegaYLoDice(t *testing.T) {
	repo := repoConRama(t)
	testutil.RunGit(t, repo, "branch", "otra")

	destino := filepath.Join(t.TempDir(), "prdash-pr-7")
	testutil.RunGit(t, repo, "worktree", "add", "--quiet", destino, "otra")

	h := NewHerdrNative(&fakeRunner{available: true}, t.TempDir())
	_, err := h.Create(context.Background(), Spec{
		Repo:   repo,
		Branch: "feat/x",
		Path:   destino,
		Label:  "prdash-pr-7",
	})
	if err == nil {
		t.Fatal("adoptó un worktree de otra rama")
	}
	for _, quiere := range []string{"otra", "feat/x", destino} {
		if !strings.Contains(err.Error(), quiere) {
			t.Errorf("el error %q no dice %q", err, quiere)
		}
	}
}

func TestRemoveDeUnWorktreeSinWorkspaceAbiertoDelegaEnElEscaneoDeGit(t *testing.T) {
	repo := repoConRama(t)
	testutil.RunGit(t, repo, "branch", "feat/x")

	base := t.TempDir()
	raiz := NewGitDirect(base)
	destino := filepath.Join(base, "prdash-pr-7")
	if _, err := raiz.Create(context.Background(), Spec{
		Repo: repo, Branch: "feat/x", Path: destino, Label: "prdash-pr-7",
	}); err != nil {
		t.Fatal(err)
	}

	f := &fakeRunner{available: true, listResult: nil}
	h := NewHerdrNative(f, base)
	if err := h.Remove(context.Background(), destino); err != nil {
		t.Fatalf("Remove sin workspace abierto falló: %v", err)
	}
	if Exists(destino) {
		t.Error("el worktree sigue en disco: se quedaría ahí sin que nadie lo supiera")
	}
	if len(f.removeCalls) != 0 {
		t.Errorf("se llamó a WorktreeRemove %d veces sin workspace abierto", len(f.removeCalls))
	}
}

// reuse has a single caller and that caller already checked Exists(spec.Path), which is exactly
// ".git is a FILE"; so this branch is unreachable from Create and has to be called directly.
func TestReuseConUnGitQueEsDirectorioPropagaElErrorDeInspect(t *testing.T) {
	repo := repoConRama(t)

	destino := filepath.Join(t.TempDir(), "prdash-pr-7")
	testutil.InitRepo(t, destino)
	testutil.CommitFile(t, destino, "a.txt", "a", "a")

	h := NewHerdrNative(&fakeRunner{available: true}, t.TempDir())
	_, err := h.reuse(context.Background(), Spec{
		Repo: repo, Branch: "feat/x", Path: destino, Label: "prdash-pr-7",
	})
	if err == nil {
		t.Fatal("adoptar un repo normal dio nil: el caller recibiría un Worktree a cero")
	}
	if !strings.Contains(err.Error(), "linked worktree") {
		t.Errorf("el error %q no es el de inspect: reuse lo reescribió y se pierde el motivo", err)
	}
	if !strings.Contains(err.Error(), destino) {
		t.Errorf("el error %q no nombra la ruta ocupada", err)
	}
}

// `git worktree lock` is how git's record goes corrupt without the repo being gone: someone locks a
// worktree and the directory disappears.
func TestUnWorktreeBloqueadoSePodaYSeBorraElResiduoSinTocarElResto(t *testing.T) {
	repo := repoConRama(t, "feat/x", "feat/y")

	base := t.TempDir()
	raiz := NewGitDirect(base)
	nuestro := filepath.Join(base, "prdash-pr-7")
	if _, err := raiz.Create(context.Background(), Spec{
		Repo: repo, Branch: "feat/x", Path: nuestro, Label: "prdash-pr-7",
	}); err != nil {
		t.Fatal(err)
	}

	testutil.RunGit(t, repo, "worktree", "lock", nuestro)

	ajeno := filepath.Join(base, "mio")
	testutil.RunGit(t, repo, "worktree", "add", "--quiet", ajeno, "feat/y")

	if err := raiz.Remove(context.Background(), nuestro); err != nil {
		t.Fatalf("quitar un worktree bloqueado falló: %v", err)
	}

	if Exists(nuestro) {
		t.Errorf("%s sigue en disco tras un Remove que sí salió", nuestro)
	}
	// A locked entry stays in git's record with the `locked` mark even when its directory is gone, and
	//`git worktree prune` does not touch a locked entry.
	var entradaViva string
	for _, linea := range strings.Split(
		testutil.RunGit(t, repo, "worktree", "list"), "\n") {
		if strings.Contains(linea, "prdash-pr-7") {
			entradaViva = linea
		}
	}
	if entradaViva == "" {
		t.Error("la entrada del worktree bloqueado desaparecio del registro: el bloqueo no " +
			"impide la poda, asi que algo mas la esta quitando")
	}
	if !strings.Contains(entradaViva, "locked") {
		t.Errorf("la entrada que queda no esta marcada como bloqueada: %q. Sin esa marca "+
			"la poda la habria quitado y el hallazgo seria otro", entradaViva)
	}
	if _, err := os.Stat(nuestro); !os.IsNotExist(err) {
		t.Errorf("el directorio %s volvio a aparecer: la entrada que queda dice que esta", nuestro)
	}

	if !Exists(ajeno) {
		t.Error("el Remove se llevó un worktree ajeno que no es de prdash")
	}
	if _, err := os.Stat(filepath.Join(ajeno, ".git")); err != nil {
		t.Errorf("el worktree ajeno perdió su .git: %v", err)
	}
}

// The sibling of the previous one: `.git` is a file (Exists is true) but git cannot read the branch
// from it.
func TestReuseSobreUnDestinoQueYaNoTieneGitSeNiegaYNoInventaUnWorktree(t *testing.T) {
	repo := repoConRama(t)
	destino := filepath.Join(t.TempDir(), "prdash-pr-7")
	if err := os.MkdirAll(destino, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(destino, ".git"),
		[]byte("gitdir: /no/existe\\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if !Exists(destino) {
		t.Fatal("el fixture no sirve: Exists tiene que dar true para entrar en reuse")
	}

	h := NewHerdrNative(&fakeRunner{available: true}, t.TempDir())
	wt, err := h.reuse(context.Background(), Spec{
		Repo: repo, Branch: "feat/x", Path: destino, Label: "prdash-pr-7",
	})
	if err == nil {
		t.Fatalf("adoptar un huérfano dio nil y un worktree: %+v. Con la rama vacía se "+
			"montaría el review del ítem sobre un checkout que no es suyo", wt)
	}
	if !strings.Contains(err.Error(), destino) {
		t.Errorf("el error %q no nombra el destino que no es un worktree", err)
	}
}

// Both of reuse's guards are literally the negation of what Create checks before calling it, so the
// test has to jump over Create to reach them.
func TestReuseSobreUnDestinoSinGitSeNiega(t *testing.T) {
	repo := repoConRama(t)
	destino := filepath.Join(t.TempDir(), "prdash-pr-7")
	if err := os.MkdirAll(destino, 0o755); err != nil {
		t.Fatal(err)
	}

	h := NewHerdrNative(&fakeRunner{available: true}, t.TempDir())
	wt, err := h.reuse(context.Background(), Spec{
		Repo: repo, Branch: "feat/x", Path: destino, Label: "prdash-pr-7",
	})
	if err == nil {
		t.Fatalf("adoptar un directorio sin .git dio nil y un worktree: %+v", wt)
	}
	if !strings.Contains(err.Error(), "linked worktree") {
		t.Errorf("el error %q no dice que el destino no es un worktree enlazado", err)
	}
	if _, err := os.Stat(destino); err != nil {
		t.Errorf("reuse quitó el directorio: %v. La guarda rechaza, no limpia", err)
	}
}
