package herdr

import (
	"context"
	"strings"
	"testing"
	"time"
)

// Herdr's CLI operations split in two for a reason that is not aesthetic: the ones that MODIFY the
//session go through the guard.

// The control case is at the end.
func TestNingunaOperacionMutanteTocaLaSesionFueraDeHerdr(t *testing.T) {
	llamadas := 0
	vistos := [][]string{}
	fuera := &Client{
		Bin:    "no-debe-ejecutarse",
		getenv: func(string) string { return "" },
		execFn: func(_ context.Context, args ...string) ([]byte, []byte, error) {
			llamadas++
			vistos = append(vistos, args)
			return []byte(`{"id":"1","result":{"type":"ok"}}`), nil, nil
		},
	}
	ctx := context.Background()
	ref := Container{WorkspaceID: "w1", PaneID: "p1"}

	ops := []struct {
		nombre string
		llamar func() error
	}{
		{"WorktreeRemove", func() error { return fuera.WorktreeRemove(ctx, "w1", true) }},
		{"WorkspaceCreate", func() error { _, err := fuera.WorkspaceCreate(ctx, WorkspaceSpec{Label: "x"}); return err }},
		{"WorkspaceClose", func() error { return fuera.WorkspaceClose(ctx, "w1", true) }},
		{"TabCreate", func() error {
			_, err := fuera.TabCreate(ctx, TabSpec{WorkspaceID: "w1", Label: "t"})
			return err
		}},
		{"TabRename", func() error { return fuera.TabRename(ctx, "t1", "nuevo") }},
		{"PaneSplit", func() error {
			_, err := fuera.PaneSplit(ctx, SplitSpec{PaneID: ref.PaneID, Direction: "right"})
			return err
		}},
		{"PaneRun", func() error { return fuera.PaneRun(ctx, "p1", []string{"echo", "hola"}) }},
		{"PaneWaitOutput", func() error { return fuera.PaneWaitOutput(ctx, "p1", "hola", time.Second) }},
		{"PaneRename", func() error { return fuera.PaneRename(ctx, "p1", "titulo") }},
		{"PaneFocus", func() error { return fuera.PaneFocus(ctx, "right") }},
		{"MountLayout", func() error { _, err := fuera.MountLayout(ctx, ref, testPlan()); return err }},
	}
	// Notify does not go through the guard: it returns warnings, not an error.

	for _, op := range ops {
		if err := op.llamar(); err == nil {
			t.Errorf("%s: fuera de Herdr dio nil, y una operacion que modifica la sesion "+
				"tiene que negarse", op.nombre)
		}
	}
	// And none of them reached the binary. This is the assertion that matters: a veto honoured in the
	// message but not in the call is no veto.
	if len(vistos) != 0 {
		t.Errorf("se ejecutaron %d llamadas al binario estando fuera de Herdr: %v", len(vistos), vistos)
	}

	dentro := &Client{
		Bin:    "herdr",
		getenv: func(k string) string { return map[string]string{"HERDR_ENV": "1"}[k] },
		execFn: func(_ context.Context, args ...string) ([]byte, []byte, error) {
			vistos = append(vistos, args)
			if len(args) > 0 && args[0] == "pane" && len(args) > 1 && args[1] == "split" {
				return []byte(`{"id":"1","result":{"pane":{"pane_id":"p2"}}}`), nil, nil
			}
			return []byte(`{"id":"1","result":{"type":"ok"}}`), nil, nil
		},
	}
	refDentro := Container{WorkspaceID: "w1", PaneID: "p1"}
	ok := []struct {
		nombre string
		llamar func() error
	}{
		{"WorktreeRemove", func() error { return dentro.WorktreeRemove(ctx, "w1", true) }},
		{"WorkspaceClose", func() error { return dentro.WorkspaceClose(ctx, "w1", true) }},
		{"TabRename", func() error { return dentro.TabRename(ctx, "t1", "nuevo") }},
		{"PaneRun", func() error { return dentro.PaneRun(ctx, "p1", []string{"echo", "hola"}) }},
		{"PaneFocus", func() error { return dentro.PaneFocus(ctx, "right") }},
		{"PaneSplit", func() error {
			_, err := dentro.PaneSplit(ctx, SplitSpec{PaneID: refDentro.PaneID, Direction: "right"})
			return err
		}},
	}
	for _, op := range ok {
		if err := op.llamar(); err != nil {
			t.Errorf("%s: dentro de Herdr dio error: %v", op.nombre, err)
		}
	}
	if len(vistos) == 0 {
		t.Error("dentro de Herdr no se ejecutó ninguna llamada: el veto de arriba no lo " +
			"provoca el veto, lo provoca el no estar dentro")
	}
	// The warning names Herdr instead of surfacing an internal error.
	err := fuera.PaneRun(ctx, "p1", []string{"echo", "hola"})
	if err == nil {
		t.Error("el veto no dio error")
	} else {
		texto := err.Error()
		for _, quiere := range []string{"herdr unavailable", "HERDR_ENV", "0.9.0"} {
			if !strings.Contains(texto, quiere) {
				t.Errorf("el aviso del veto %q no menciona %q", texto, quiere)
			}
		}
	}
	// MountLayout outside Herdr fails with an ERROR, not only with warnings: there is no layout to
	// open.
	if _, err := fuera.MountLayout(ctx, ref, testPlan()); err == nil {
		t.Error("MountLayout fuera de Herdr dio nil")
	}
}

func TestSoloLecturaNoSeVeta(t *testing.T) {
	llamadas := 0
	vistos := [][]string{}
	a := &Client{
		Bin:    "herdr",
		getenv: func(string) string { return "" }, // fuera de Herdr a proposito
		execFn: func(_ context.Context, args ...string) ([]byte, []byte, error) {
			llamadas++
			vistos = append(vistos, args)
			return []byte(`{"result":{"source":{"repo_root":"/r"},"worktrees":[
				{"path":"/r","branch":"main","label":"et","open_workspace_id":"w1",
				 "is_linked_worktree":false},
				{"path":"/r/.worktrees/wt-1","branch":"feat/x","label":"prdash-pr-1",
				 "open_workspace_id":"w2","is_linked_worktree":true}
			]}}`), nil, nil
		},
	}

	list, err := a.WorktreeList(context.Background(), "/r")
	if err != nil {
		t.Fatalf("una lectura vetada dio error: %v", err)
	}
	if llamadas == 0 {
		t.Error("la lectura no llegó al binario: el veto se comió también las lecturas")
	}
	if len(list) != 2 {
		t.Fatalf("la lista dio %d entradas: %+v", len(list), list)
	}
	if list[0].Path != "/r" || list[0].Branch != "main" {
		t.Errorf("la primera entrada se leyo mal: %+v", list[0])
	}
	if !list[1].IsLinkedWorktree || list[1].OpenWorkspaceID != "w2" {
		t.Errorf("la segunda entrada perdio los flags: %+v", list[1])
	}
	if list[0].IsLinkedWorktree {
		t.Error("el repo principal aparecio como worktree enlazado")
	}

	roto := &Client{
		Bin:    "herdr",
		getenv: func(string) string { return "" },
		execFn: func(context.Context, ...string) ([]byte, []byte, error) {
			return nil, []byte("fatal: no"), &Error{Args: []string{"x"}, Msg: "no", Exit: 1}
		},
	}
	if _, err := roto.WorktreeList(context.Background(), "/r"); err == nil {
		t.Error("una lectura que falla dio nil")
	}
	vacio := &Client{
		Bin: "herdr", getenv: func(string) string { return "" },
		execFn: func(context.Context, ...string) ([]byte, []byte, error) {
			return []byte(`{"result":{"source":{},"worktrees":[]}}`), nil, nil
		},
	}
	lista, err := vacio.WorktreeList(context.Background(), "/r")
	if err != nil {
		t.Errorf("una lista vacia dio error: %v", err)
	}
	if len(lista) != 0 {
		t.Errorf("una lista vacia dio %d entradas", len(lista))
	}
	_ = vistos
}

// NewGraphics was at 0% because every test constructed it differently.
func TestElClienteDeGraficosSeConstruyeYLeeElEntorno(t *testing.T) {
	// The socket variable is HERDR_SOCKET_PATH, not HERDR_SOCK.
	t.Setenv("HERDR_PANE_ID", "p9")
	t.Setenv("HERDR_SOCKET_PATH", "/tmp/herdr.sock")
	g := NewGraphics()
	if g.getenv == nil {
		t.Fatal("NewGraphics dejó getenv a nil")
	}
	if got := g.env("HERDR_PANE_ID"); got != "p9" {
		t.Errorf("env dio %q, want p9", got)
	}
	if got := g.socket(); got != "/tmp/herdr.sock" {
		t.Errorf("socket dio %q, want el de HERDR_SOCKET_PATH", got)
	}
	if got := g.pane(); got != "p9" {
		t.Errorf("pane dio %q, want p9", got)
	}
	directo := &Graphics{Socket: "/tmp/otro.sock", PaneID: "p0"}
	if got := directo.socket(); got != "/tmp/otro.sock" {
		t.Errorf("con Socket puesto dio %q", got)
	}
	if got := directo.pane(); got != "p0" {
		t.Errorf("con PaneID puesto dio %q", got)
	}

	// Sin el entorno: vacíos, no "undefined" ni el nombre de la variable.
	t.Setenv("HERDR_PANE_ID", "")
	t.Setenv("HERDR_SOCKET_PATH", "")
	g = NewGraphics()
	if got := g.env("HERDR_PANE_ID"); got != "" {
		t.Errorf("sin variable dio %q, want vacío", got)
	}
	if haveGraphicsTarget(g.socket(), g.pane()) {
		t.Error("sin socket ni pane dio un destino valido")
	}

	// A Graphics with a nil getenv falls back to os.Getenv instead of panicking.
	gA := &Graphics{}
	t.Setenv("HERDR_PANE_ID", "p-con-nil")
	if got := gA.env("HERDR_PANE_ID"); got != "p-con-nil" {
		t.Errorf("con getenv a nil dio %q, want el valor real del entorno", got)
	}
}

// Same case in the normal client.
func TestElClienteTambienCaeAOsGetenvSinGetenv(t *testing.T) {
	cA := &Client{Bin: "herdr"}
	t.Setenv("HERDR_ENV", "1")
	if got := cA.env("HERDR_ENV"); got != "1" {
		t.Errorf("con getenv a nil dio %q, want 1", got)
	}
	// Y sin la variable.
	t.Setenv("HERDR_ENV", "")
	if got := cA.env("HERDR_ENV"); got != "" {
		t.Errorf("sin variable dio %q", got)
	}
	propio := &Client{getenv: func(k string) string {
		if k == "HERDR_ENV" {
			return "1"
		}
		return ""
	}}
	if got := propio.env("HERDR_ENV"); got != "1" {
		t.Errorf("con getenv propio dio %q", got)
	}
	t.Setenv("HERDR_ENV", "")
	if got := propio.env("HERDR_ENV"); got != "1" {
		t.Errorf("tras vaciar la variable dio %q: manda el getenv propio", got)
	}
}

// The caching matters, not the version: Available queries on every call.
func TestLaVersionSeConsultaUnaSolaYSeCachea(t *testing.T) {
	llamadas := 0
	a := &Client{
		Bin:    "herdr",
		getenv: func(k string) string { return map[string]string{"HERDR_ENV": "1"}[k] },
		execFn: func(_ context.Context, args ...string) ([]byte, []byte, error) {
			llamadas++
			if len(args) == 1 && args[0] == "--version" {
				return []byte("herdr 0.9.1-preview.7\n"), nil, nil
			}
			return []byte(`{"id":"1","result":{"type":"ok"}}`), nil, nil
		},
	}

	v, ok := a.Version()
	if !ok {
		t.Fatal("Version dio false con una salida válida")
	}
	if v.Major != 0 || v.Minor != 9 || v.Patch != 1 {
		t.Errorf("Version dio %d.%d.%d, want 0.9.1", v.Major, v.Minor, v.Patch)
	}
	// Y cinco llamadas más no vuelven a preguntar.
	for i := 0; i < 5; i++ {
		a.Version()
		a.Available()
	}
	if llamadas != 1 {
		t.Errorf("se consultó la versión %d veces, want 1: Available la llama en cada "+
			"operación y sin cacheo cada montaje sería varias llamadas a un subproceso",
			llamadas)
	}

	// With --version failing it is not blocked: that is the development build, where every operation
	// would fail.
	roto := &Client{
		Bin:    "herdr",
		getenv: func(k string) string { return map[string]string{"HERDR_ENV": "1"}[k] },
		execFn: func(context.Context, ...string) ([]byte, []byte, error) {
			return nil, nil, &Error{Args: []string{"--version"}, Msg: "no such flag", Exit: 1}
		},
	}
	if _, ok := roto.Version(); ok {
		t.Error("Version dio true con un --version que falla")
	}
	if !roto.Available() {
		t.Error("Available veto por drift cuando la versión no se puede determinar: " +
			"eso deja prdash sin Herdr en una build de desarrollo")
	}
}
