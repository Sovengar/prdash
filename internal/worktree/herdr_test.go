package worktree

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"prdash/internal/herdr"
	"prdash/internal/testutil"
)

// errWorkspaceProbe simula que Herdr no puede abrir el workspace del checkout.
var errWorkspaceProbe = errors.New("socket closed")

// fakeRunner es un doble en memoria del subconjunto de Herdr que usa la
// provisión nativa. No toca ningún proceso.
type fakeRunner struct {
	available    bool
	createInfo   herdr.WorktreeInfo
	createErr    error
	createCalls  []herdr.WorktreeSpec
	listResult   []herdr.WorktreeInfo
	removeCalls  []string
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
	return nil
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

// TestHerdrNativeCreateKeepsOwnershipLabel cubre que `worktree.label` que
// reporta el nativo (el nombre del repo) no pise la etiqueta de ownership que
// prdash pidió con --label. Verificado contra Herdr 0.9.1 real.
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

// TestHerdrNativeReuseAdoptsCheckoutWithNoOpenWorkspace es el caso que fazia
// aparecer el review como un workspace suelto: el checkout ya está en disco de
// una sesión anterior pero su workspace de Herdr está cerrado, así que
// `worktree list` no devuelve `open_workspace_id`. Sin contenedor, el layout se
// fabricaba un workspace propio y el worktree quedaba desligado de Herdr. La
// vuelta es abrir un workspace **con cwd en el worktree**: Herdr lo registra
// como el workspace abierto de ese worktree y el sidebar lo muestra como tal.
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
	// El cwd es la pieza crítica: es lo que hace que Herdr lo ligue al worktree.
	if got := runner.wsCalls[0]; got.Cwd != dest || !got.NoFocus || got.Label != "prdash-pr-1" {
		t.Fatalf("spec del workspace = %+v", got)
	}
	if wt.WorkspaceID != "w1C" || wt.RootPaneID != "w1C:p1" {
		t.Fatalf("contenedor = %+v", wt)
	}
}

// Si ni siquiera el workspace se puede abrir, es mejor fallar con un motivo
// accionable que devolver un worktree sin contenedor: el layout semountaría en
// un workspace suelto sin avisar de nada.
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

// TestHerdrNativeReuseIgnoresStaleOpenWorkspaceId es el caso que hizo aparecer
// el review como un workspace suelto la segunda vez. `worktree list` puede
// devolver un `open_workspace_id` que ya no existe: Herdr lo guarda en su sesión
// persistida y un workspace cerrado deja el id apuntando a nada. Aceptarlo a
// ciegas deja el worktree sin pane base, y el layout se fabrica entonces un
// workspace propio desligado del worktree.
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

// Con un workspace vivo se reutiliza tal cual y se levanta su pane base, para que
// el layout no tenga que descubrirlo con un `pane list` propio.
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

// TestHerdrNativeReuseRejectsBranchMismatch comprueba que reutilizar un
// checkout con otra rama falla con un error claro, igual que git directo.
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
