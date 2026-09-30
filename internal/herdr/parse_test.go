package herdr

import (
	"errors"
	"os"
	"testing"
)

// Fixtures de la forma real de la CLI: todo va envuelto en {"id","result"}.

const fixtureWorktreeCreated = `{
  "id": "cli:worktree:create",
  "result": {
    "type": "worktree_created",
    "workspace": {"workspace_id": "w18", "label": "prdash-pr-7"},
    "tab": {"tab_id": "w18:t1"},
    "root_pane": {"pane_id": "w18:p1"},
    "worktree": {
      "path": "/home/u/.herdr/worktrees/prdash/feat-x",
      "branch": "prdash/pr-7",
      "label": "repo",
      "is_linked_worktree": true,
      "is_prunable": false,
      "open_workspace_id": "w18"
    }
  }
}`

const fixtureWorktreeList = `{
  "id": "cli:worktree:list",
  "result": {
    "type": "worktree_list",
    "source": {
      "repo_key": "/repo/.git",
      "repo_name": "repo",
      "repo_root": "/repo",
      "source_checkout_path": "/repo",
      "source_workspace_id": "w17"
    },
    "worktrees": [
      {"path": "/repo", "branch": "main", "label": "repo", "is_linked_worktree": false, "is_prunable": false, "open_workspace_id": "w17"},
      {"path": "/repo.wt/prdash-pr-7", "branch": "prdash/pr-7", "label": "prdash-pr-7", "is_linked_worktree": true, "is_prunable": false, "open_workspace_id": ""}
    ]
  }
}`

const fixtureWorkspaceCreated = `{
  "id": "cli:workspace:create",
  "result": {
    "type": "workspace_created",
    "workspace": {"workspace_id": "w20", "label": "review"},
    "tab": {"tab_id": "w20:t1"},
    "root_pane": {"pane_id": "w20:p1"}
  }
}`

const fixtureTabCreated = `{
  "id": "cli:tab:create",
  "result": {
    "type": "tab_created",
    "tab": {"tab_id": "w20:t2"},
    "root_pane": {"pane_id": "w20:p5"}
  }
}`

const fixturePaneSplit = `{
  "id": "cli:pane:split",
  "result": {
    "type": "pane_info",
    "pane": {"pane_id": "w18:p2", "workspace_id": "w18", "tab_id": "w18:t1", "cwd": "/repo.wt/prdash-pr-7", "label": ""}
  }
}`

const fixturePaneList = `{
  "id": "cli:pane:list",
  "result": {
    "type": "pane_list",
    "panes": [
      {"pane_id": "w18:p1", "workspace_id": "w18", "tab_id": "w18:t1", "cwd": "/repo.wt/prdash-pr-7", "label": "TUICR"},
      {"pane_id": "w18:p2", "workspace_id": "w18", "tab_id": "w18:t1", "cwd": "/repo.wt/prdash-pr-7", "label": "Hunk"}
    ]
  }
}`

const fixtureNotification = `{
  "id": "cli:notification:show",
  "result": {"type": "notification_show", "shown": true, "reason": ""}
}`

// TestVersionAtLeastComparaPorCascada: el versionado de Herdr decide si se puede
// usar una capacidad, así que un `>` mal puesto convierte un requisito en "no
// disponible" o en "creo que sí" sobre una versión que no lo soporta.
//
// Los casos que importan son los de la frontera: la versión EXACTA mínima tiene
// que validar (por eso el patch es `>=` y no `>`), y un minor superior vale con
// cualquier patch. Un major superior manda sobre todo lo demás.
func TestVersionAtLeastComparaPorCascada(t *testing.T) {
	min := Version{Major: 0, Minor: 9, Patch: 3}
	cases := []struct {
		name string
		v    Version
		want bool
	}{
		{"la exacta mínima", Version{0, 9, 3, ""}, true},
		{"patch por encima", Version{0, 9, 4, ""}, true},
		{"minor por encima, patch por debajo", Version{0, 10, 0, ""}, true},
		{"major por encima con todo por debajo", Version{1, 0, 0, ""}, true},
		{"patch por debajo", Version{0, 9, 2, ""}, false},
		{"minor por debajo", Version{0, 8, 9, ""}, false},
		{"major por debajo", Version{0, 0, 0, ""}, false},
		{"todo cero contra todo cero", Version{0, 0, 0, ""}, true},
	}
	for _, c := range cases[:len(cases)-1] {
		t.Run(c.name, func(t *testing.T) {
			if got := c.v.AtLeast(min); got != c.want {
				t.Errorf("Version(%s).AtLeast(%s) = %v, want %v", c.v, min, got, c.want)
			}
		})
	}
	// La versión mínima 0.0.0 no es un caso raro: es lo que se recibe cuando no
	// se puede leer la versión, y "cualquier cosa cumple" es la degradación
	// correcta para no bloquear la TUI por un dato que no se sabe.
	if !(Version{}).AtLeast(Version{}) {
		t.Error("0.0.0 debería cumplir el mínimo 0.0.0: es la degradación cuando no se lee la versión")
	}
	if (Version{}).AtLeast(min) {
		t.Error("0.0.0 no debería cumplir un mínimo 0.9.3")
	}
	// Y una versión enorme cumple: el 0.x de Herdr es el que hace inútil un
	// rango de versiones, no un techo.
	if !(Version{99, 0, 0, ""}).AtLeast(min) {
		t.Error("un major muy alto debería cumplir cualquier mínimo 0.x")
	}
}

// TestErrorComponeElMensajeConLoQueHay: el mensaje de Herdr es lo que la TUI
// enseña, y las tres partes que se añaden (código del servidor, código de salida,
// causa) son opcionales e independientes. Un "exit 0" colgado o un código de
// servidor inventado son ruido que manda a mirar donde no es.
func TestErrorComponeElMensajeConLoQueHay(t *testing.T) {
	cases := []struct {
		name string
		err  *Error
		want string
	}{
		{
			"solo el mensaje",
			&Error{Args: []string{"pane", "list"}, Msg: "pane not found"},
			"herdr [pane list]: pane not found",
		},
		{
			"con código de servidor",
			&Error{Args: []string{"pane", "list"}, Msg: "pane not found", Code: "E_NOENT"},
			"herdr [pane list]: pane not found [E_NOENT]",
		},
		{
			"con código de salida",
			&Error{Args: []string{"pane", "list"}, Msg: "pane not found", Exit: 2},
			"herdr [pane list]: pane not found (exit 2)",
		},
		{
			"con los dos",
			&Error{Args: []string{"pane", "list"}, Msg: "pane not found", Exit: 2, Code: "E_NOENT"},
			"herdr [pane list]: pane not found [E_NOENT] (exit 2)",
		},
		{
			// Un exit 0 con código de salida != 0 es un dato raro pero posible si
			// el proceso se corta por una señal; el código se enseña igual.
			"sin args",
			&Error{Msg: "boom"},
			"herdr []: boom",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := c.err.Error(); got != c.want {
				t.Errorf("Error() = %q, want %q", got, c.want)
			}
		})
	}
	// Y la causa: Unwrap la preserva, que es lo que permite clasificar sin
	// depender del texto.
	causa := os.ErrNotExist
	err := &Error{Msg: "x", Err: causa}
	if !errors.Is(err, causa) {
		t.Error("errors.Is debería encontrar la causa a través del *Error")
	}
	if (&Error{Msg: "x"}).Unwrap() != nil {
		t.Error("sin causa, Unwrap debería devolver nil")
	}
}

func TestParseWorktreeCreated(t *testing.T) {
	info, err := parseWorktreeCreated([]byte(fixtureWorktreeCreated))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if info.WorkspaceID != "w18" || info.TabID != "w18:t1" || info.RootPaneID != "w18:p1" {
		t.Fatalf("contenedor = %+v", info)
	}
	if info.Path != "/home/u/.herdr/worktrees/prdash/feat-x" || info.Branch != "prdash/pr-7" {
		t.Fatalf("worktree = %+v", info)
	}
	// La etiqueta de ownership viene del workspace (--label); worktree.label es
	// el nombre del repo en la salida real de Herdr 0.9.1.
	if info.WorkspaceLabel != "prdash-pr-7" || info.Label != "repo" {
		t.Fatalf("labels = %+v", info)
	}
	if !info.IsLinkedWorktree || info.OpenWorkspaceID != "w18" {
		t.Fatalf("flags = %+v", info)
	}
}

func TestParseWorktreeList(t *testing.T) {
	list, err := parseWorktreeList([]byte(fixtureWorktreeList))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if list.RepoRoot != "/repo" || len(list.Worktrees) != 2 {
		t.Fatalf("list = %+v", list)
	}
	if list.Worktrees[1].Path != "/repo.wt/prdash-pr-7" || !list.Worktrees[1].IsLinkedWorktree {
		t.Fatalf("worktree[1] = %+v", list.Worktrees[1])
	}
}

func TestParseWorkspaceAndTabAndPane(t *testing.T) {
	ws, err := parseWorkspaceCreated([]byte(fixtureWorkspaceCreated))
	if err != nil || ws.WorkspaceID != "w20" || ws.RootPaneID != "w20:p1" {
		t.Fatalf("workspace = %+v, err=%v", ws, err)
	}
	tab, err := parseTabCreated([]byte(fixtureTabCreated))
	if err != nil || tab.TabID != "w20:t2" || tab.RootPaneID != "w20:p5" {
		t.Fatalf("tab = %+v, err=%v", tab, err)
	}
	pane, err := parsePaneSplit([]byte(fixturePaneSplit))
	if err != nil || pane.PaneID != "w18:p2" || pane.Cwd != "/repo.wt/prdash-pr-7" {
		t.Fatalf("pane = %+v, err=%v", pane, err)
	}
	panes, err := parsePaneList([]byte(fixturePaneList))
	if err != nil || len(panes) != 2 || panes[1].Label != "Hunk" {
		t.Fatalf("panes = %+v, err=%v", panes, err)
	}
}

func TestParseNotification(t *testing.T) {
	shown, reason, err := parseNotification([]byte(fixtureNotification))
	if err != nil || !shown || reason != "" {
		t.Fatalf("notification shown=%v reason=%q err=%v", shown, reason, err)
	}
}

func TestParseVersion(t *testing.T) {
	v, ok := parseVersion([]byte("herdr 0.9.1-preview.2026-09-21-0ff0f27e2226\n"))
	if !ok || v.Major != 0 || v.Minor != 9 || v.Patch != 1 {
		t.Fatalf("version = %+v ok=%v", v, ok)
	}
	if !v.AtLeast(MinVersion) {
		t.Fatalf("%s debería ser >= %s", v, MinVersion)
	}
	if (Version{Major: 0, Minor: 8, Patch: 2}).AtLeast(MinVersion) {
		t.Fatal("0.8.2 no debería alcanzar el mínimo")
	}
	if _, ok := parseVersion([]byte("nope")); ok {
		t.Fatal("sin semver no debería haber versión")
	}
}

func TestParseMalformedResult(t *testing.T) {
	if _, err := parseWorktreeCreated([]byte("{")); err == nil {
		t.Fatal("una respuesta ilegible debería fallar, no entrar en pánico")
	}
	if _, err := parsePaneSplit([]byte(`{"id":"x"}`)); err == nil {
		t.Fatal("un sobre sin .result útil debería fallar")
	}
}

func TestParseServerErrorVariants(t *testing.T) {
	code, msg := parseServerError([]byte(`{"error":{"code":"worktree_create_failed","message":"no such branch"}}`))
	if code != "worktree_create_failed" || msg != "no such branch" {
		t.Fatalf("variant error = %q %q", code, msg)
	}
	code, msg = parseServerError([]byte(`{"code":"x","message":"y"}`))
	if code != "x" || msg != "y" {
		t.Fatalf("variant flat = %q %q", code, msg)
	}
	if code, msg := parseServerError([]byte("no json")); code != "" || msg != "" {
		t.Fatalf("texto sin json = %q %q", code, msg)
	}
}
