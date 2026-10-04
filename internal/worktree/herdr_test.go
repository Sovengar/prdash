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

// TestHerdrNativeRemoveRefusesPathOutsideBase blinda la guarda de raíz gestionada
// en el camino nativo: antes delegaba en el cliente sin comprobarla, así que una
// ruta propia fuera de la raíz se podía borrar bajo Herdr.
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

// TestHerdrNativeRemoveIfCleanRefusesPathOutsideBase extiende la misma guarda al
// candado "solo si limpio": la ruta fuera de la raíz se rechaza sin consultar el
// estado ni llamar al cliente nativo.
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

// herdrCleanWorktree crea un worktree limpio bajo una raíz propia y devuelve la
// raíz y su ruta. Los tests del candado nativo lo ensucian o lo dejan limpio.
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

// TestHerdrNativeRemoveIfCleanDeletesViaNativeWorkspace cubre el caso feliz del
// candado nativo: un checkout limpio se borra por su workspace, no por git
// directo, para no dejar el workspace de Herdr huérfano.
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

// TestHerdrNativeRemoveIfCleanKeepsDirty: el candado se delega en shouldRemove,
// así que un checkout sucio se conserva SIN llamar al borrado nativo.
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

// TestHerdrNativeRemoveIfCleanPropagatesRemoveError: si el borrado nativo falla,
// el error se propaga y no se reporta como borrado.
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

// TestSobreUnDestinoQueNoEsWorktreeNoSeCreaEncimaNiSeAdopta: la decision de `Create`.
//
// Y es la decisión que elige entre `herdr worktree create` y `reuse`, y hay una franja
// estrecha de destinos para los que `Exists` da verdadero y el destino NO es un worktree
// utilizable: un `.git` que es fichero pero no declara un repo que exista.
//
// Y las dos salidas son "no tocarlo", y esa es la propiedad que importa:
//
//   - No se llama a `herdr worktree create`, porque esa orden hace internamente `git worktree
//     add` y se niega sobre un path existente —sería un error diferido, no una creación—.
//   - No se pisa el directorio del usuario: lo que hubiera dentro sigue.
//
// Y sobre la franja concreta: `inspect` devuelve `ok` siempre que `.git` sea un fichero,
// porque lee la rama con `git rev-parse` e IGNORA su fallo. Así que un `.git` con un gitdir que
// no existe entra como "worktree de rama vacía" y se rechaza por la RAMA, no por no ser un
// worktree. El `!ok` de `reuse` es defensivo y no se llega por `Create`; lo que se comprueba es
// que esa franja tampoco acaba en una creación.
func TestSobreUnDestinoQueNoEsWorktreeNoSeCreaEncimaNiSeAdopta(t *testing.T) {
	destino := filepath.Join(t.TempDir(), "prdash-pr-7")
	if err := os.MkdirAll(destino, 0o755); err != nil {
		t.Fatal(err)
	}
	// Un `.git` que es FICHERO —que es lo que hace `Exists` dar true y entrar en `reuse`— pero
	// que no declara un gitdir real: el huerfano por definicion.
	if err := os.WriteFile(filepath.Join(destino, ".git"),
		[]byte("gitdir: /no/existe\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Con algo dentro, que es lo que lo hace un directorio de trabajo y no un hueco vacio.
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
	// Y `herdr worktree create` NO se llamo: se niega sobre un path existente, asi que
	// "crear encima" no es una alternativa sino un error diferido.
	if len(f.createCalls) != 0 {
		t.Errorf("se llamo a `worktree create` %d veces sobre un destino que ya existe",
			len(f.createCalls))
	}
	// Y el contenido del usuario sigue ahi: ni se adopto ni se piso.
	if _, err := os.Stat(filepath.Join(destino, "trabajo.txt")); err != nil {
		t.Errorf("el directorio del usuario cambio: %v", err)
	}
}

// TestAdoptarUnWorktreeDeOtraRamaSeNiegaYLoDice: el reuse que encuentra pero no es el suyo.
//
// Y el motivo es el que evita un montaje FALSO: si se adoptaba un checkout de otra rama, el
// review se montaría sobre código que nadie eligió y el plan lo Presentaría como el del ítem,
// sin ningún aviso. El error tiene que命名 las dos ramas, porque con dos worktrees de prdash
// abiertos en la misma raíz —que es lo normal— la pregunta "¿cuál es el mío?" no tiene
// respuesta sin el nombre.
func TestAdoptarUnWorktreeDeOtraRamaSeNiegaYLoDice(t *testing.T) {
	repo := repoConRama(t)
	testutil.RunGit(t, repo, "branch", "otra")

	// Un worktree real de `otra` en el destino que se pide para `feat/x`.
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
	// Y el mensaje trae las dos: sin la que se encontró, el usuario no sabe qué hay abierto;
	// sin la que se pedía, no sabe qué quería montar.
	for _, quiere := range []string{"otra", "feat/x", destino} {
		if !strings.Contains(err.Error(), quiere) {
			t.Errorf("el error %q no dice %q", err, quiere)
		}
	}
}

// TestRemoveDeUnWorktreeSinWorkspaceAbiertoDelegaEnElEscaneoDeGit: el fallback de `Remove`.
//
// Y es la ruta que se recorre en la limpieza al cerrar la TUI, y por eso importa: si `Remove`
// solo supiera quitar worktrees con workspace abierto, los que no lo tienen —un montaje a medias,
// un worktree creado a mano con `prdash worktrees add`— se quedarían en disco sin que nadie lo
// supiera, y el `Audit` siguiente los listaría como si fueran normales.
//
// Y el caso que hay que provocar es el que NO tiene workspace: Herdr devuelve una lista vacía
// y `Remove` cae al escaneo de git, que sí sabe quitarlo.
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

	// Herdr disponible pero sin ningún worktree nativo registrado: la lista viene vacía.
	f := &fakeRunner{available: true, listResult: nil}
	h := NewHerdrNative(f, base)
	if err := h.Remove(context.Background(), destino); err != nil {
		t.Fatalf("Remove sin workspace abierto falló: %v", err)
	}
	if Exists(destino) {
		t.Error("el worktree sigue en disco: se quedaría ahí sin que nadie lo supiera")
	}
	// Y Herdr NO se usó para quitarlo: `WorktreeRemove` es el camino del workspace, y con la
	// lista vacía no hay a qué llamar.
	if len(f.removeCalls) != 0 {
		t.Errorf("se llamó a WorktreeRemove %d veces sin workspace abierto", len(f.removeCalls))
	}
}

// TestReuseConUnGitQueEsDirectorioPropagaElErrorDeInspect: la rama que `Create` no alcanza.
//
// Y aquí está lo que el test documenta: `reuse` tiene un caller único, y ese caller ya ha
// comprobado `Exists(spec.Path)`, que es exactamente "`.git` es un FICHERO". Así que las dos
// guardas que hay dentro de `reuse` —el error de `inspect` y su `!ok`— son inalcanzables desde
// `Create`: para llegar al error de `inspect` haría falta un `.git` que sea DIRECTORIO, y
// `Exists` ya lo ha excluido.
//
// Y no se borran, y ese es el punto: `reuse` es un método con su propio contrato, no un
// fragmento que solo existe dentro de `Create`. Quitar las guardas porque hoy no se ejecutan
// las convierte en una trampa para el próximo caller, y un `inspect` que empezara a fallar por
// un motivo nuevo devolvería un `Worktree` a cero —con la rama vacía— en vez de un error.
//
// Y probarlas directamente es lo que hace que sigan siendo certain: el test llama a `reuse`
// con el `.git` que `Create` nunca le pasa y comprueba que el error sale.
func TestReuseConUnGitQueEsDirectorioPropagaElErrorDeInspect(t *testing.T) {
	repo := repoConRama(t)

	// Un repo NORMAL: `.git` es un directorio. `Create` no entraría en `reuse` con esto.
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
	// Y el error es el de `inspect`, no uno inventado: `reuse` lo propaga tal cual y no lo
	// reescribe, que es lo que permite saber que el destino estaba ocupado por otra cosa.
	if !strings.Contains(err.Error(), "linked worktree") {
		t.Errorf("el error %q no es el de inspect: reuse lo reescribió y se pierde el motivo", err)
	}
	if !strings.Contains(err.Error(), destino) {
		t.Errorf("el error %q no nombra la ruta ocupada", err)
	}
}

// TestUnWorktreeBloqueadoSePodaYSeBorraElResiduoSinTocarElResto: la rama de `sourceReachable`.
//
// Y el caso es `git worktree lock`, que es la forma de que el registro de git quede corrupto sin
// que el repo se haya ido: alguien bloquea un worktree —porque tiene una sesión de Herdr
// abierta en él, o porque lo está usando a mano— y `git worktree remove` se niega a quitarlo.
//
// Y la decisión es podar y borrar el residuo, y tiene dos mitades que hay que mirar:
//
//   - Se poda. Sin `git worktree prune` la entrada queda en `.git/worktrees/` para siempre y cada
//     `worktree list` la enseña, aunque el directorio ya no exista.
//   - Y se borra el checkout. Es lo que hace que el directorio se vaya, y es lo que distingue
//     este camino del anterior —donde no hay repo detrás y solo queda el checkout—.
//
// Y lo que NO puede pasar es tocar nada fuera del ownership: la poda es de git y no borra
// ficheros, y el `RemoveAll` va sobre la ruta que el caller ya ha comprobado que es un worktree
// nuestro. Por eso el test pone un worktree AJENO al lado y comprueba que sobrevive.
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

	// El bloqueo: `remove` se niega con el repo vivo, que es lo que hace alcanzable la rama.
	testutil.RunGit(t, repo, "worktree", "lock", nuestro)

	// Y un worktree ajeno al lado, que no existe para `Owned` pero que sí está en el mismo
	// repo: la poda de git lo menciona y el `RemoveAll` no lo puede tocar.
	ajeno := filepath.Join(base, "mio")
	testutil.RunGit(t, repo, "worktree", "add", "--quiet", ajeno, "feat/y")

	if err := raiz.Remove(context.Background(), nuestro); err != nil {
		t.Fatalf("quitar un worktree bloqueado falló: %v", err)
	}

	// El nuestro se fue.
	if Exists(nuestro) {
		t.Errorf("%s sigue en disco tras un Remove que sí salió", nuestro)
	}
	// Y aquí está el hallazgo del test: la entrada del worktree BLOQUEADO sigue en el registro
	// de git, con la marca `locked`, aunque su directorio ya no existe.
	//
	// Y no es un fallo de la poda: `git worktree prune` no toca una entrada bloqueada, porque
	// `lock` significa "no la toques". Así que la secuencia de `Remove` deja el `.git/worktrees/`
	// con una entrada que apunta a un directorio que ya no está, y `git worktree list` la
	// enseña —sin la marca `prunable`, porque git la considera viva.
	//
	// El daño es de higiene y es acumulativo: cada bloqueo que se quite desde prdash deja una
	// entrada, y el usuario ve en `git worktree list` worktrees que no existen. Arreglarlo
	// sería `unlock` antes de `prune`, que es un cambio de comportamiento —volverían a ser
	// tuyos worktrees que alguien había marcado como ocupados— y por eso no se hace aquí.
	//
	// Y lo que sí se comprueba es lo que NO puede pasar: el directorio se borra y el worktree
	// ajeno no se toca.
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

	// Y el ajeno sigue intacto: la poda es de git y el RemoveAll va por ownership.
	if !Exists(ajeno) {
		t.Error("el Remove se llevó un worktree ajeno que no es de prdash")
	}
	if _, err := os.Stat(filepath.Join(ajeno, ".git")); err != nil {
		t.Errorf("el worktree ajeno perdió su .git: %v", err)
	}
}

// TestReuseSobreUnDestinoQueYaNoTieneGitSeNiegaYNoInventaUnWorktree: el `!ok` de `reuse`.
//
// Y es el hermano del anterior y cubre la otra guarda inalcanzable desde `Create`: el `.git` es
// un fichero —`Exists` da verdadero— pero git no puede leer la rama desde él.
//
// Y pasa de verdad: `git worktree add` deja el directorio y el fichero `.git` a medias si se
// interrumpe, y un `.git` copiado a mano con un `gitdir:` que no corresponde deja el mismo
// rastro. El destino existe y parece un worktree, y no lo es.
//
// Y la respuesta tiene que ser un error, no un `Worktree` a cero con la rama vacía: eso último
// se montaría como si fuera el review del ítem, con el plan puesto y el trabajo sin el código que
// el usuario eligió.
func TestReuseSobreUnDestinoQueYaNoTieneGitSeNiegaYNoInventaUnWorktree(t *testing.T) {
	repo := repoConRama(t)
	destino := filepath.Join(t.TempDir(), "prdash-pr-7")
	if err := os.MkdirAll(destino, 0o755); err != nil {
		t.Fatal(err)
	}
	// Un `.git` de fichero que no declara un gitdir real: el huérfano por definición.
	if err := os.WriteFile(filepath.Join(destino, ".git"),
		[]byte("gitdir: /no/existe\\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Y `Exists` da verdadero, que es lo que hace entrar en `reuse`.
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
	// Y el motivo nombra el destino, que es lo que el usuario necesita para ir a mirarlo.
	if !strings.Contains(err.Error(), destino) {
		t.Errorf("el error %q no nombra el destino que no es un worktree", err)
	}
}

// TestReuseSobreUnDestinoSinGitSeNiega: la guarda `!ok`, y por qué necesita llamarse directo.
//
// Y aquí la aritmética es la que obliga al test a saltarse a `Create`. Las dos guardas de
// `reuse` son, literalmente, la negación de lo que `Create` comprueba antes de llamar:
//
//   - `Create` entra en `reuse` solo si `Exists(spec.Path)`, que es `Lstat(.git)` sin error Y
//     `.git` no es un directorio.
//   - `inspect` da error cuando `.git` ES un directorio, y `ok = false` cuando el `Lstat` FALLA.
//
// O sea: las dos ramas de `reuse` describen estados que su único caller ya ha excluido. Ni
// siquiera hay una ruta por la que se crucen: son la negación una de la otra.
//
// Y no se borran, y el motivo es el del test de al lado: `reuse` es un método con su propio
// contrato y no un fragmento que solo existe dentro de `Create`. Quitar las guardas porque hoy
// son inalcanzables las convierte en una trampa para el próximo caller —y el próximo caller
// es exactamente el caso en el que importarían, porque sería el que añadiese el destino sin
// comprobar `Exists`—.
//
// Y el precio de mantenerlas es que solo se pueden probar llamando al método directamente, que
// es lo que hace este test. No es un rodeo: es la manera de comprobar el contrato de `reuse`
// como función, que es lo que es.
func TestReuseSobreUnDestinoSinGitSeNiega(t *testing.T) {
	repo := repoConRama(t)
	// Un directorio que existe y NO tiene `.git`. `inspect` da `ok = false` y sin error, que
	// es el único caso en el que se puede llegar a esa guarda.
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
	// Y el trabajo del usuario sigue ahí: la guarda no limpia, solo rechaza.
	if _, err := os.Stat(destino); err != nil {
		t.Errorf("reuse quitó el directorio: %v. La guarda rechaza, no limpia", err)
	}
}
