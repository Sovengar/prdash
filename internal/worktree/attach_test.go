package worktree

import (
	"context"
	"errors"
	"testing"

	"prdash/internal/herdr"
)

// Los tests de `attach` cubren la ADOPCIÓN: cuando el checkout ya tiene un workspace
// abierto en Herdr, se adopta en vez de abrir uno nuevo.
//
// Y la razón de que la adopción se compruebe en vez de confiar es lo que hay que
// tener presente al leer los tests: el `open_workspace_id` que devuelve
// `worktree list` NO es de fiar. Herdr lo guarda en su sesión persistida, así que un
// workspace cerrado deja el id apuntando a nada y `pane list` responde
// `workspace_not_found` (comprobado en 0.9.1). Confiar en él dejaba el worktree sin
// pane base, y el layout se fabricaba entonces un workspace propio desligado del
// worktree: el review aparecía como un workspace suelto y NUEVO en cada montaje.
//
// O sea que `pane list` no es una comprobación deademic: es la que devuelve el pane
// base, y de paso es la que demuestra que el id apunta a algo.

// attachDe es el atajo para montar un HerdrNative sobre un fake ya configurado y
// devolver el worktree resultante.
func attachDe(t *testing.T, runner *fakeRunner, spec Spec) (Worktree, error) {
	t.Helper()
	wt := Worktree{ID: spec.Path, Path: spec.Path}
	return wt, NewHerdrNative(runner, t.TempDir()).attach(context.Background(), &wt, spec)
}

// TestAttachAdoptaElWorkspaceQueYaEstaAbierto: si el checkout tiene un workspace
// abierto con panes, se adopta TAL CUAL y no se abre ninguno nuevo.
//
// Y lo que se afirma es que no se abrió otro, que es la mitad de lo que pasa. Abrir
// un workspace nuevo no falla: tener DOS workspaces para el mismo worktree es lo que
// sale, y el síntoma es un review nuevo en cada montaje, con un id distinto cada vez.
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
	// Y el pane base es el PRIMERO de la lista, no el último: `pane list` devuelve
	// el pane base el primero, y es al que se le pega la segunda tab. Si fuera otro,
	// el layout montaría la segunda tab colgando de un pane que no es la raíz.
	if wt.RootPaneID != "ws-7:p0" {
		t.Errorf("pane base=%q, want ws-7:p0: es el primero de la lista", wt.RootPaneID)
	}
	// Y no se abrió nada nuevo. Este es el assert que importa.
	if len(runner.wsCalls) != 0 {
		t.Errorf("se abrieron %d workspaces nuevos (%v) cuando ya había uno abierto adopting: "+
			"el review aparecería como un workspace suelto y nuevo en cada montaje",
			len(runner.wsCalls), runner.wsCalls)
	}
}

// TestAttachNoSeAdoptaElWorkspaceDeOtroCheckout: un `open_workspace_id` que es real
// pero pertenece a OTRO worktree no se adopta.
//
// La comprobación es por path, y tiene que serlo: `worktree list` devuelve todos los
// worktrees del repo, así que el primero con un id abierto puede ser el de otro PR. Sin
// el filtro por path, el review del PR 7 se montaría en el workspace del PR 6, y
// ambos abrirían el mismo repo en el mismo sitio.
//
// Y el caso de verdad es el inverso: que el del PR 6 esté en la lista ANTES, porque el
// orden de `worktree list` es el de Herdr y no se controla.
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
	// Y los panes del otro no se llegaron a preguntar: filtrar por path es lo que
	// evita esa llamada, y una llamada de menos es una prueba de que el filtro está
	// antes y no después.
	if len(runner.paneListArgs) != 0 {
		t.Errorf("se preguntó por los panes de %v, que son de otro checkout", runner.paneListArgs)
	}
}

// TestAttachUnIdCerradoNoSeAdopta: el caso que justifica toda la comprobación.
//
// El `open_workspace_id` está, parece bueno, y no apunta a nada: el workspace se
// cerró pero el id sigue en la sesión de Herdr. `pane list` responde
// `workspace_not_found`. Confiar en el id dejaba el worktree sin pane base.
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
			// Y el workspace nuevo se abre con el cwd del worktree y SIN FOCUS, que
			// es lo que evita que montar un review robe el foco de la TUI.
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

// TestAttachSinListaNoAdopta: si `worktree list` falla, se abre workspace y punto.
//
// El caso es degradación pura, que es la clase de cosa que no se mira: el runner
// devuelve error y lo que hay que comprobar es que no se intenta adoptar nada
// —no hay nada que adoptar— y que se abre el workspace con los datos del llamador.
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

// TestAttachConIdDeWorkspaceVacioNoAdopta: un worktree de la lista con
// `open_workspace_id` vacío es un worktree que no tiene workspace abierto, y se
// salta sin preguntar por sus panes.
//
// Preguntar por un id vacío sería una llamada que Herdr no puede responder, y además
// es la forma de distinguir "no tiene workspace" de "tiene uno cerrado": los dos
// terminan abriendo uno nuevo, pero por caminos distintos.
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

// TestAttachSoloAbreUnWorkspaceCuandoTodoFalla: el peor caso. La lista trae el
// checkout pero su id no vale, así que se abre uno nuevo. Y aunque haya VARIOS
// worktrees en la lista, se abre UNO solo.
//
// El número de `workspace create` es lo que hay que mirar, porque abrir dos es un
// fallo silencioso: no da error, da un workspace huérfano por montaje.
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
	// Y solo se preguntó por los panes de la entrada del checkout, una vez. El del
	// otro worktree no se mira porque su path no es el nuestro.
	if len(runner.paneListArgs) != 1 || runner.paneListArgs[0] != "ws-cerrado" {
		t.Errorf("preguntó por los panes de %v, want exactamente [ws-cerrado]", runner.paneListArgs)
	}
}

// TestAttachUnIdMaloCortaLaBusqueda: el `break`. En cuanto el checkout aparece con
// un id que no vale, se deja de mirar el resto de la lista.
//
// Puede pasar que el MISMO path salga dos veces: un worktree registrado en más de un
// repo —el bare y su clon— aparece en la lista de cualquiera de los dos, y cada
// entrada trae el id que le conoce ese repo. Con la primera muerta y la segunda
// viva, sin `break` se adoptaría la segunda, y el worktree quedaría ligado al
// contenedor que otro repo tiene abierto sobre él.
//
// Que no sea un caso raro no lo hace menos real: el fallo no da error. Da un review
// montado en un workspace ajeno, que es de las cosas más difíciles de ver.
func TestAttachUnIdMaloCortaLaBusqueda(t *testing.T) {
	spec := Spec{Repo: "/repos/acme", Branch: "feat/x", Path: "/wt/x"}
	runner := &fakeRunner{
		available: true,
		listResult: []herdr.WorktreeInfo{
			// El mismo path dos veces, con ids distintos: el primero cerrado y el
			// segundo vivo.
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
	// Y el id vivo del segundo nunca se llegó a preguntar, que es el "se deja de
	// mirar" hecho visible.
	if len(runner.paneListArgs) != 1 || runner.paneListArgs[0] != "ws-cerrado" {
		t.Errorf("preguntó por los panes de %v, want exactamente [ws-cerrado]", runner.paneListArgs)
	}
}
