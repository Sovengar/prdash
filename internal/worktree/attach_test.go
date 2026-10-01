package worktree

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"prdash/internal/herdr"
	"prdash/internal/testutil"
)

// Los tests de `attach` cubren la ADOPCIÓN: cuando el checkout ya tiene un workspace
// abierto en Herdr, se adopta en vez de abrir uno nuevo. Y al final, los de `reuse`,
// que es la otra mitad: la etiqueta de un checkout que ya existe.
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

// TestReuseLaEtiquetaDelLlamadorPisaLaDelCheckout: al REUTILIZAR un checkout que ya
// existe, la etiqueta es la que pide el llamador, no la que tenia el checkout.
//
// Y el caso de verdad es el contrario del que parece: el checkout en disco puede tener
// una etiqueta de la sesión anterior, puesta por quien lo montara entonces. Si esa
// ganara, el worktree volveria con el nombre viejo y prdash dejaria de reconocerlo
// como el worktree que esta mirando — que es como un PR aparece dos veces en la
// lista, o como el suyo desaparece.
//
// O sea que el `if spec.Label != ""` no es una defensa contra un dato malo: es la
// regla de que la etiqueta la manda quien esta montando AHORA, no la que quedo escrita
// la vez anterior.
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

	// El llamador pide una etiqueta DISTINTA, que es lo unico que hace el test
	//ouro: si coincidieran, las dos reglas darian lo mismo y no se probaria nada.
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
	// Y que la etiqueta sea la del llamador es lo que va al workspace, no la del
	// repo: el `--label` del workspace es por donde se le reconoce.
	if len(runner.wsCalls) != 1 || runner.wsCalls[0].Label != "prdash/acme#12" {
		t.Errorf("el workspace se abrió con label %q, want la del llamador",
			labelDe(runner.wsCalls))
	}
}

// TestReuseSinEtiquetaDelLlamadorConservaLaDelCheckout: si el llamador NO dio
// etiqueta, se conserva la del checkout —que es el NOMBRE DEL DIRECTORIO, porque eso
// es lo que `inspect` deduce del disco— y no se cae a la del nativo.
//
// Y esto NO es lo mismo que en `Create`, y la diferencia es el motivo de que sean dos
// ramas y no una. Al reutilizar hay un checkout REAL en disco, y su nombre es un dato
// de verdad: es como se le llama a esa cosa en el sistema de archivos, y es lo único
// que lo distingue de los demás worktrees del mismo repo. En `Create` no hay checkout
// previo, así que ahí sí se recurre a los datos de Herdr.
//
// La tentación sería usar la del nativo, que es el nombre del repo. Sería peor: todos
// los worktrees del mismo repo se llamarían igual, y la etiqueta dejaría de distinguir
// el PR 7 del PR 6. Un nombre de repo no identifica un worktree.
//
// Un `if` de estos dos parece el mismo y no lo es, y por eso el comentario de la regla
// está en el sitio: para que se lea que la diferencia es DEL DATO QUE HAY, no una
// inconsistencia entre un sitio y otro.
func TestReuseSinEtiquetaDelLlamadorConservaLaDelCheckout(t *testing.T) {
	repo := newRepo(t)
	testutil.RunGit(t, repo, "branch", "feature")

	base := t.TempDir()
	// El nombre del directorio es lo que va a acabar siendo la etiqueta.
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

	// El llamador NO da etiqueta: Spec.Label va vacío.
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
