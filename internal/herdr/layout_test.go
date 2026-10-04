package herdr

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"prdash/internal/review/plan"
)

type fakeLayout struct {
	fakeCLI
	splits int
	tabs   int
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
		case args[0] == "tab" && args[1] == "create":
			f.tabs++
			return []byte(fmt.Sprintf(`{"id":"cli:tab:create","result":{"type":"tab_created","tab":{"tab_id":"w18:t%d"},"root_pane":{"pane_id":"w18:p%d"}}}`, f.tabs+1, f.splits+1)), nil, nil
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
	return plan.Plan{Tabs: []plan.Tab{
		{Label: plan.LabelReview, Panes: []plan.Pane{
			{Kind: plan.KindTuicr, Label: "TUICR", Cwd: "/wt/prdash-pr-7", Argv: []string{"tuicr", "pr", "https://github.com/o/r/pull/7"}, Env: []string{"PRDASH_NUMBER=7"}},
			{Kind: plan.KindEditor, Label: "Editor", Dir: plan.DirRight, Cwd: "/wt/prdash-pr-7", Argv: []string{"vi"}, Env: []string{"PRDASH_NUMBER=7"}},
		}},
		{Label: plan.LabelEdit, Panes: []plan.Pane{
			{Kind: plan.KindHunk, Label: "Hunk", Cwd: "/wt/prdash-pr-7", Argv: []string{"hunk", "diff", "main...HEAD"}, Env: []string{"PRDASH_NUMBER=7"}},
			{Kind: plan.KindAgent, Label: "Agente", Dir: plan.DirRight, Cwd: "/wt/prdash-pr-7", Argv: []string{"opencode"}, Env: []string{"PRDASH_NUMBER=7"}},
		}},
	}}
}

func TestMountLayoutRenamesRootTabAndCreatesTheRest(t *testing.T) {
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

	if !f.called("tab", "rename", "w18:t1", plan.LabelReview) {
		t.Fatalf("el tab raíz debería renombrarse a %q: %v", plan.LabelReview, f.calls)
	}
	if f.tabs != 1 {
		t.Fatalf("tabs creados = %d, quiero 1 (el primero reutiliza el del contenedor)", f.tabs)
	}
	if !hasCall(f.calls, "tab", "create", "--workspace", "w18", "--label", plan.LabelEdit, "--no-focus") {
		t.Fatalf("el segundo tab debería crearse en el workspace: %v", f.calls)
	}
}

func TestMountLayoutFillsEachTabIndependently(t *testing.T) {
	f := newFakeLayout()
	c := f.client()

	if _, err := c.MountLayout(context.Background(), Container{WorkspaceID: "w18", PaneID: "w18:p1"}, testPlan()); err != nil {
		t.Fatalf("MountLayout: %v", err)
	}

	if !f.called("pane", "run", "w18:p1") || !f.called("pane", "rename", "w18:p1", "TUICR") {
		t.Fatalf("el primer pane debería lanzarse sobre el root pane: %v", f.calls)
	}
	if !f.called("pane", "run", "w18:p2") || !f.called("pane", "rename", "w18:p2", "Hunk") {
		t.Fatalf("el segundo tab debería poblarse desde su propio root pane: %v", f.calls)
	}
	if !f.called("pane", "rename", "w18:p3", "Agente") {
		t.Fatalf("el agente debería ocupar el pane dividido: %v", f.calls)
	}
	if hasSplit(f.calls, "w18:p1", "right") && hasSplit(f.calls, "w18:p2", "right") {
		return
	}
	t.Fatalf("cada tab debería dividir a la derecha desde su base: %v", f.calls)
}

func TestMountLayoutSendsCwdAndEnvToEverySplit(t *testing.T) {
	f := newFakeLayout()
	c := f.client()

	if _, err := c.MountLayout(context.Background(), Container{WorkspaceID: "w18", PaneID: "w18:p1"}, testPlan()); err != nil {
		t.Fatalf("MountLayout: %v", err)
	}
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

// If the container points at a workspace Herdr no longer knows, the mount must fail naming it.
func TestMountLayoutFailsOnDeadWorkspace(t *testing.T) {
	f := newFakeLayout()
	base := f.respond
	f.respond = func(args []string) ([]byte, []byte, error) {
		if len(args) > 2 && args[0] == "pane" && args[1] == "list" {
			return nil, nil, fmt.Errorf("workspace w1B not found")
		}
		return base(args)
	}

	warns, err := f.client().MountLayout(context.Background(), Container{WorkspaceID: "w1B"}, testPlan())
	if err == nil {
		t.Fatalf("un workspace muerto no debería montar en silencio (warnings=%v)", warns)
	}
	if !strings.Contains(err.Error(), "w1B") {
		t.Fatalf("el error debería nombrar el workspace: %v", err)
	}
	if f.called("workspace", "create") {
		t.Fatal("no debería crear un workspace de repuesto")
	}
}

func TestMountLayoutCreatesWorkspaceWithoutContainer(t *testing.T) {
	f := newFakeLayout()
	c := f.client()
	pl := plan.Plan{Tabs: []plan.Tab{{Label: plan.LabelReview, Panes: []plan.Pane{{Kind: plan.KindTuicr, Label: "TUICR", Cwd: "/wt", Argv: []string{"tuicr"}}}}}}

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
		if call[0] == "pane" || call[0] == "tab" {
			t.Fatalf("no debería tocar panes ni tabs con plan vacío: %v", call)
		}
	}
}

func TestMountLayoutTabFailureIsWarning(t *testing.T) {
	f := newFakeLayout()
	base := f.respond
	f.respond = func(args []string) ([]byte, []byte, error) {
		if len(args) > 1 && args[0] == "tab" && args[1] == "create" {
			return nil, nil, fmt.Errorf("sin espacio")
		}
		return base(args)
	}
	warns, err := f.client().MountLayout(context.Background(), Container{WorkspaceID: "w18", PaneID: "w18:p1"}, testPlan())
	if err != nil {
		t.Fatalf("un tab fallido no debería abortar: %v", err)
	}
	if len(warns) == 0 || !strings.Contains(strings.Join(warns, " "), plan.LabelEdit) {
		t.Fatalf("warnings = %v", warns)
	}
	if !f.called("pane", "rename", "w18:p1", "TUICR") {
		t.Fatalf("el primer tab debería quedarse montado: %v", f.calls)
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
	warns, err := f.client().MountLayout(context.Background(), Container{WorkspaceID: "w18", PaneID: "w18:p1"}, pl)
	if err != nil {
		t.Fatalf("un split fallido no debería abortar: %v", err)
	}
	if len(warns) == 0 || !strings.Contains(strings.Join(warns, " "), "Editor") {
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

func hasCall(calls [][]string, sub string, flags ...string) bool {
	for _, c := range calls {
		if len(c) == 0 || c[0] != sub {
			continue
		}
		all := true
		for _, want := range flags {
			if !contains(c, want) {
				all = false
				break
			}
		}
		if all {
			return true
		}
	}
	return false
}
