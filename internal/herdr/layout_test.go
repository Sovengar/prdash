package herdr

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"prdash/internal/review/plan"
)

// fakeLayout responde a las operaciones de layout con ids incrementales.
type fakeLayout struct {
	fakeCLI
	splits int
}

func newFakeLayout() *fakeLayout {
	f := &fakeLayout{}
	f.env = map[string]string{"HERDR_ENV": "1"}
	f.respond = func(args []string) ([]byte, []byte, error) {
		switch {
		case args[0] == "--version":
			return []byte("herdr 0.9.1\n"), nil, nil
		case args[0] == "workspace":
			return []byte(fixtureWorkspaceCreated), nil, nil
		case args[0] == "pane" && args[1] == "split":
			f.splits++
			pane := fmt.Sprintf("w18:p%d", f.splits+1)
			return []byte(fmt.Sprintf(`{"id":"cli:pane:split","result":{"type":"pane_info","pane":{"pane_id":%q,"workspace_id":"w18"}}}`, pane)), nil, nil
		case args[0] == "pane" && args[1] == "list":
			return []byte(fixturePaneList), nil, nil
		default:
			return []byte(`{"id":"cli:ok","result":{"type":"ok"}}`), nil, nil
		}
	}
	return f
}

func testPlan() plan.Plan {
	return plan.Plan{Panes: []plan.Pane{
		{Kind: plan.KindTuicr, Label: "TUICR", Cwd: "/wt/prdash-pr-7", Argv: []string{"tuicr", "pr", "https://github.com/o/r/pull/7"}, Env: []string{"PRDASH_NUMBER=7"}},
		{Kind: plan.KindAgent, Label: "Agente", Cwd: "/wt/prdash-pr-7", Argv: []string{"opencode"}, Env: []string{"PRDASH_NUMBER=7"}},
		{Kind: plan.KindHunk, Label: "Hunk", Cwd: "/wt/prdash-pr-7", Argv: []string{"hunk", "session", "review"}, Env: []string{"PRDASH_NUMBER=7"}},
	}}
}

func TestMountLayoutUsesRootPaneAndSplits(t *testing.T) {
	f := newFakeLayout()
	c := f.client()

	warns, err := c.MountLayout(context.Background(), Container{WorkspaceID: "w18", PaneID: "w18:p1"}, testPlan())
	if err != nil {
		t.Fatalf("MountLayout: %v", err)
	}
	if len(warns) != 0 {
		t.Fatalf("warnings = %v", warns)
	}
	if f.called("workspace", "create") {
		t.Fatal("con root pane no debería crear otro workspace")
	}

	// Primer pane: run y rename sobre el root.
	if !f.called("pane", "run", "w18:p1") {
		t.Fatal("el primer pane debería lanzarse sobre el root pane")
	}
	if !f.called("pane", "rename", "w18:p1", "TUICR") {
		t.Fatal("el primer pane debería renombrarse")
	}
	// Divisiones: right sobre el root y down sobre el pane nuevo.
	if !hasSplit(f.calls, "w18:p1", "right") || !hasSplit(f.calls, "w18:p2", "down") {
		t.Fatalf("divisiones = %v", f.calls)
	}
	// Todos los splits sin foco y con cwd.
	for _, call := range f.calls {
		if len(call) >= 2 && call[0] == "pane" && call[1] == "split" {
			if !contains(call, "--no-focus") {
				t.Fatalf("split sin --no-focus: %v", call)
			}
			if !contains(call, "--cwd") || !contains(call, "/wt/prdash-pr-7") {
				t.Fatalf("split sin cwd del worktree: %v", call)
			}
			if !contains(call, "--env") || !contains(call, "PRDASH_NUMBER=7") {
				t.Fatalf("split sin env del plan: %v", call)
			}
		}
	}
}

func TestMountLayoutCreatesWorkspaceWithoutContainer(t *testing.T) {
	f := newFakeLayout()
	c := f.client()
	pl := plan.Plan{Panes: []plan.Pane{{Kind: plan.KindTuicr, Label: "TUICR", Cwd: "/wt", Argv: []string{"tuicr"}}}}

	if _, err := c.MountLayout(context.Background(), Container{}, pl); err != nil {
		t.Fatalf("MountLayout: %v", err)
	}
	if !f.called("workspace", "create") {
		t.Fatal("sin pane base debería crear un workspace")
	}
	if !f.called("pane", "run", "w20:p1") {
		t.Fatal("debería lanzar el pane en el root del workspace creado")
	}
}

func TestMountLayoutEmptyPlanIsNoop(t *testing.T) {
	f := newFakeLayout()
	if warns, err := f.client().MountLayout(context.Background(), Container{PaneID: "w18:p1"}, plan.Plan{}); err != nil || warns != nil {
		t.Fatalf("plan vacío: warns=%v err=%v", warns, err)
	}
	for _, call := range f.calls {
		if call[0] == "pane" {
			t.Fatalf("no debería tocar panes con plan vacío: %v", call)
		}
	}
}

func TestMountLayoutPaneFailureIsWarning(t *testing.T) {
	f := newFakeLayout()
	base := f.respond
	f.respond = func(args []string) ([]byte, []byte, error) {
		if len(args) > 1 && args[0] == "pane" && args[1] == "split" {
			return nil, nil, fmt.Errorf("sin espacio")
		}
		return base(args)
	}
	pl := testPlan()
	warns, err := f.client().MountLayout(context.Background(), Container{PaneID: "w18:p1"}, pl)
	if err != nil {
		t.Fatalf("un split fallido no debería abortar: %v", err)
	}
	if len(warns) == 0 || !strings.Contains(strings.Join(warns, " "), "Hunk") {
		t.Fatalf("warnings = %v", warns)
	}
}

func TestPaneCommandQuotesCwdAndEnv(t *testing.T) {
	p := plan.Pane{
		Label: "TUICR",
		Cwd:   "/tmp/my wt",
		Argv:  []string{"tuicr", "pr", "https://github.com/o/r/pull/7"},
		Env:   []string{"PRDASH_URL=https://x/y?z=1"},
	}
	got := paneCommand(p)
	want := "cd '/tmp/my wt' && export PRDASH_URL='https://x/y?z=1' && tuicr pr https://github.com/o/r/pull/7"
	if got != want {
		t.Fatalf("paneCommand =\n%q\nwant\n%q", got, want)
	}
}

func hasSplit(calls [][]string, parent, direction string) bool {
	for _, c := range calls {
		if len(c) >= 4 && c[0] == "pane" && c[1] == "split" && contains(c, parent) && contains(c, direction) {
			return true
		}
	}
	return false
}
