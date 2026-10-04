package worktree

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"prdash/internal/herdr"
	"prdash/internal/testutil"
)

func attachDe(t *testing.T, runner *fakeRunner, spec Spec) (Worktree, error) {
	t.Helper()
	wt := Worktree{ID: spec.Path, Path: spec.Path}
	return wt, NewHerdrNative(runner, t.TempDir()).attach(context.Background(), &wt, spec)
}

func TestAttachAdoptaElWorkspaceQueYaEstaAbierto(t *testing.T) {
	spec := Spec{Repo: "/repos/acme", Branch: "feat/x", Path: "/wt/x", Label: "prdash/x"}
	runner := &fakeRunner{
		available: true,
		listResult: []herdr.WorktreeInfo{
			{Path: "/wt/otro", OpenWorkspaceID: "ws-otro"}, // otro checkout: se ignora
			{Path: spec.Path, OpenWorkspaceID: "ws-7"},
		},
		panes: map[string][]herdr.PaneInfo{
			"ws-7": {{PaneID: "ws-7:p0"}, {PaneID: "ws-7:p1"}},
		},
	}

	wt, err := attachDe(t, runner, spec)
	if err != nil {
		t.Fatalf("adoptar dio error: %v", err)
	}
	if wt.WorkspaceID != "ws-7" {
		t.Errorf("workspace=%q, want ws-7", wt.WorkspaceID)
	}
	if wt.RootPaneID != "ws-7:p0" {
		t.Errorf("pane base=%q, want ws-7:p0: es el primero de la lista", wt.RootPaneID)
	}
	if len(runner.wsCalls) != 0 {
		t.Errorf("se abrieron %d workspaces nuevos (%v) cuando ya había uno abierto adopting: "+
			"el review aparecería como un workspace suelto y nuevo en cada montaje",
			len(runner.wsCalls), runner.wsCalls)
	}
}

// Checked by path.
func TestAttachNoSeAdoptaElWorkspaceDeOtroCheckout(t *testing.T) {
	spec := Spec{Repo: "/repos/acme", Branch: "feat/x", Path: "/wt/x", Label: "prdash/x"}
	runner := &fakeRunner{
		available: true,
		listResult: []herdr.WorktreeInfo{
			{Path: "/wt/otro", OpenWorkspaceID: "ws-otro"},
		},
		panes:     map[string][]herdr.PaneInfo{"ws-otro": {{PaneID: "ws-otro:p0"}}},
		workspace: herdr.WorkspaceInfo{WorkspaceID: "ws-nuevo", RootPaneID: "ws-nuevo:p0"},
	}

	wt, err := attachDe(t, runner, spec)
	if err != nil {
		t.Fatalf("dar error: %v", err)
	}
	if wt.WorkspaceID != "ws-nuevo" {
		t.Errorf("workspace=%q, want ws-nuevo: el de otro checkout no se adopta, o el "+
			"review del PR 7 se monta en el workspace del PR 6", wt.WorkspaceID)
	}
	if len(runner.paneListArgs) != 0 {
		t.Errorf("se preguntó por los panes de %v, que son de otro checkout", runner.paneListArgs)
	}
}

// The case that justifies the whole check: the open_workspace_id is there, looks fine, and points
// at nothing.
func TestAttachUnIdCerradoNoSeAdopta(t *testing.T) {
	for _, c := range []struct {
		nombre string
		panes  []herdr.PaneInfo
		err    error
	}{
		{"el workspace ya no existe", nil, errors.New("workspace_not_found")},
		{"el workspace existe pero no tiene panes", []herdr.PaneInfo{}, nil},
	} {
		t.Run(c.nombre, func(t *testing.T) {
			spec := Spec{Repo: "/repos/acme", Branch: "feat/x", Path: "/wt/x"}
			runner := &fakeRunner{
				available:  true,
				listResult: []herdr.WorktreeInfo{{Path: spec.Path, OpenWorkspaceID: "ws-7"}},
				panes:      map[string][]herdr.PaneInfo{"ws-7": c.panes},
				panesErr:   map[string]error{"ws-7": c.err},
				workspace:  herdr.WorkspaceInfo{WorkspaceID: "ws-nuevo", RootPaneID: "ws-nuevo:p0"},
			}

			wt, err := attachDe(t, runner, spec)
			if err != nil {
				t.Fatalf("dar error: %v", err)
			}
			if wt.WorkspaceID != "ws-nuevo" {
				t.Errorf("workspace=%q, want ws-nuevo: un id que no apunta a nada no se adopta",
					wt.WorkspaceID)
			}
			if wt.RootPaneID != "ws-nuevo:p0" {
				t.Errorf("pane=%q, want el del workspace nuevo", wt.RootPaneID)
			}
			if len(runner.wsCalls) != 1 {
				t.Errorf("se abrieron %d workspaces, want 1: sin pane base hay que abrir uno",
					len(runner.wsCalls))
			}
			if len(runner.wsCalls) == 1 {
				w := runner.wsCalls[0]
				if w.Cwd != spec.Path {
					t.Errorf("cwd=%q, want %q: el workspace tiene que quedar en el worktree", w.Cwd, spec.Path)
				}
				if !w.NoFocus {
					t.Error("NoFocus=false: montar un review no puede robar el foco de la TUI")
				}
				if w.Label != spec.Label {
					t.Errorf("label=%q, want %q", w.Label, spec.Label)
				}
			}
		})
	}
}

// Pure degradation, the class of thing nobody looks at.
func TestAttachSinListaNoAdopta(t *testing.T) {
	spec := Spec{Repo: "/repos/acme", Branch: "feat/x", Path: "/wt/x", Label: "prdash/x"}
	runner := &fakeRunner{
		available:  true,
		listResult: nil,
		workspace:  herdr.WorkspaceInfo{WorkspaceID: "ws-nuevo", RootPaneID: "ws-nuevo:p0"},
	}

	wt, err := attachDe(t, runner, spec)
	if err != nil {
		t.Fatalf("sin lista debería abrir workspace y seguir: %v", err)
	}
	if wt.WorkspaceID != "ws-nuevo" {
		t.Errorf("workspace=%q, want ws-nuevo", wt.WorkspaceID)
	}
	if len(runner.paneListArgs) != 0 {
		t.Errorf("se preguntó por los panes (%v) sin lista que lo diga", runner.paneListArgs)
	}
}

func TestAttachConIdDeWorkspaceVacioNoAdopta(t *testing.T) {
	spec := Spec{Repo: "/repos/acme", Branch: "feat/x", Path: "/wt/x"}
	runner := &fakeRunner{
		available: true,
		listResult: []herdr.WorktreeInfo{
			{Path: spec.Path, OpenWorkspaceID: ""}, // sin workspace abierto
		},
		workspace: herdr.WorkspaceInfo{WorkspaceID: "ws-nuevo", RootPaneID: "ws-nuevo:p0"},
	}

	wt, err := attachDe(t, runner, spec)
	if err != nil {
		t.Fatalf("dar error: %v", err)
	}
	if wt.WorkspaceID != "ws-nuevo" {
		t.Errorf("workspace=%q, want ws-nuevo", wt.WorkspaceID)
	}
	if len(runner.paneListArgs) != 0 {
		t.Errorf("se preguntó por los panes de un id VACIO (%v): Herdr no puede "+
			"responder de un workspace que no existe", runner.paneListArgs)
	}
}

func TestAttachSoloAbreUnWorkspaceCuandoTodoFalla(t *testing.T) {
	spec := Spec{Repo: "/repos/acme", Branch: "feat/x", Path: "/wt/x"}
	runner := &fakeRunner{
		available: true,
		listResult: []herdr.WorktreeInfo{
			{Path: spec.Path, OpenWorkspaceID: "ws-cerrado"},
			{Path: "/wt/otro", OpenWorkspaceID: "ws-valido"},
		},
		panes: map[string][]herdr.PaneInfo{
			"ws-cerrado": nil,
			"ws-valido":  {{PaneID: "ws-valido:p0"}},
		},
		panesErr:  map[string]error{"ws-cerrado": errors.New("workspace_not_found")},
		workspace: herdr.WorkspaceInfo{WorkspaceID: "ws-nuevo", RootPaneID: "ws-nuevo:p0"},
	}

	wt, err := attachDe(t, runner, spec)
	if err != nil {
		t.Fatalf("dar error: %v", err)
	}
	if wt.WorkspaceID != "ws-nuevo" {
		t.Errorf("workspace=%q, want ws-nuevo", wt.WorkspaceID)
	}
	if len(runner.wsCalls) != 1 {
		t.Errorf("se abrieron %d workspaces, want 1: abrir dos deja un workspace huérfano "+
			"por montaje, y eso no da error, se ve como un review duplicado", len(runner.wsCalls))
	}
	if len(runner.paneListArgs) != 1 || runner.paneListArgs[0] != "ws-cerrado" {
		t.Errorf("preguntó por los panes de %v, want exactamente [ws-cerrado]", runner.paneListArgs)
	}
}

// The `break`: as soon as the checkout appears with an id that does not hold, the rest of the
// list is not read.
func TestAttachUnIdMaloCortaLaBusqueda(t *testing.T) {
	spec := Spec{Repo: "/repos/acme", Branch: "feat/x", Path: "/wt/x"}
	runner := &fakeRunner{
		available: true,
		listResult: []herdr.WorktreeInfo{
			{Path: spec.Path, OpenWorkspaceID: "ws-cerrado"},
			{Path: spec.Path, OpenWorkspaceID: "ws-vivo"},
		},
		panes: map[string][]herdr.PaneInfo{
			"ws-cerrado": nil,
			"ws-vivo":    {{PaneID: "ws-vivo:p0"}},
		},
		panesErr:  map[string]error{"ws-cerrado": errors.New("workspace_not_found")},
		workspace: herdr.WorkspaceInfo{WorkspaceID: "ws-nuevo", RootPaneID: "ws-nuevo:p0"},
	}

	wt, err := attachDe(t, runner, spec)
	if err != nil {
		t.Fatalf("dar error: %v", err)
	}
	if wt.WorkspaceID != "ws-nuevo" {
		t.Errorf("workspace=%q, want ws-nuevo: en cuanto el checkout aparece con un id que "+
			"no vale, se deja de mirar el resto. Sin eso se adoptaría el id de la segunda "+
			"entrada, que es de otro repo y abriría el review en un workspace ajeno",
			wt.WorkspaceID)
	}
	if len(runner.paneListArgs) != 1 || runner.paneListArgs[0] != "ws-cerrado" {
		t.Errorf("preguntó por los panes de %v, want exactamente [ws-cerrado]", runner.paneListArgs)
	}
}

// On REUSING an existing checkout the label is the caller's, not the one the checkout had.
func TestReuseLaEtiquetaDelLlamadorPisaLaDelCheckout(t *testing.T) {
	repo := newRepo(t)
	testutil.RunGit(t, repo, "branch", "feature")

	base := t.TempDir()
	dest := filepath.Join(base, "prdash-pr-1")
	if _, err := NewGitDirect(base).Create(context.Background(),
		Spec{Repo: repo, Branch: "feature", Path: dest, Label: "etiqueta-VIEJA"}); err != nil {
		t.Fatalf("preparar worktree: %v", err)
	}

	runner := &fakeRunner{
		available: true,
		listResult: []herdr.WorktreeInfo{
			{Path: dest, Branch: "feature", Label: "la-del-repo-del-nativo"},
		},
		workspace: herdr.WorkspaceInfo{WorkspaceID: "w1", RootPaneID: "w1:p1"},
	}

	wt, err := NewHerdrNative(runner, base).Create(context.Background(),
		Spec{Repo: repo, Branch: "feature", Path: dest, Label: "prdash/acme#12"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if wt.Label != "prdash/acme#12" {
		t.Errorf("label=%q, want la del llamador prdash/acme#12: si gana la del checkout "+
			"o la del nativo, el worktree vuelve con el nombre de la sesión anterior y "+
			"prdash deja de reconocerlo", wt.Label)
	}
	if len(runner.wsCalls) != 1 || runner.wsCalls[0].Label != "prdash/acme#12" {
		t.Errorf("el workspace se abrió con label %q, want la del llamador",
			labelDe(runner.wsCalls))
	}
}

// Without a caller label the checkout's is kept, which is the DIRECTORY NAME, because that is what
// Herdr reports.
func TestReuseSinEtiquetaDelLlamadorConservaLaDelCheckout(t *testing.T) {
	repo := newRepo(t)
	testutil.RunGit(t, repo, "branch", "feature")

	base := t.TempDir()
	dest := filepath.Join(base, "el-pr-7-del-repo-acme")
	if _, err := NewGitDirect(base).Create(context.Background(),
		Spec{Repo: repo, Branch: "feature", Path: dest, Label: "lo-que-pase"}); err != nil {
		t.Fatalf("preparar worktree: %v", err)
	}

	runner := &fakeRunner{
		available: true,
		listResult: []herdr.WorktreeInfo{
			{Path: dest, Branch: "feature", Label: "acme-widget"},
		},
		workspace: herdr.WorkspaceInfo{WorkspaceID: "w1", RootPaneID: "w1:p1"},
	}

	wt, err := NewHerdrNative(runner, base).Create(context.Background(),
		Spec{Repo: repo, Branch: "feature", Path: dest})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if wt.Label != "el-pr-7-del-repo-acme" {
		t.Errorf("label=%q, want el nombre del checkout el-pr-7-del-repo-acme", wt.Label)
	}
	if wt.Label == "acme-widget" {
		t.Error("la etiqueta es el nombre del repo que reporta el nativo: todos los " +
			"worktrees de ese repo se llamarían igual y dejarían de distinguirse")
	}
}

func labelDe(calls []herdr.WorkspaceSpec) string {
	if len(calls) == 0 {
		return "<no se abrió ningún workspace>"
	}
	return calls[0].Label
}
