package herdr

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"prdash/internal/review/plan"
)

// The layout is a chain of Herdr calls and NONE is atomic: open the workspace, rename the first
//tab, split the pane, run the command, rename the pane.

// The counter is what makes the test valuable.
type countingCLI struct {
	*fakeCLI
	calls map[string]int
	fails func(op string, args []string) bool
}

func newCountingCLI(fails func(op string, args []string) bool) *countingCLI {
	c := &countingCLI{
		fakeCLI: &fakeCLI{},
		calls:   map[string]int{},
		fails:   fails,
	}
	c.env = map[string]string{"HERDR_ENV": "1"}
	c.respond = func(args []string) ([]byte, []byte, error) {
		op := operationOf(args)
		c.calls[op]++
		if c.fails != nil && c.fails(op, args) {
			return nil, []byte("herdr: algo no salió"), fmt.Errorf("exit status 1")
		}
		switch {
		case args[0] == "--version":
			return []byte("herdr 0.9.1\n"), nil, nil
		case args[0] == "workspace":
			return []byte(fixtureWorkspaceCreated), nil, nil
		case args[0] == "tab" && args[1] == "create":
			return []byte(`{"id":"cli:tab:create","result":{"type":"tab_created",` +
				`"tab":{"tab_id":"w18:t1"},"root_pane":{"pane_id":"w18:p9"}}}`), nil, nil
		case args[0] == "pane" && args[1] == "split":
			return []byte(`{"id":"cli:pane:split","result":{"type":"pane_info",` +
				`"pane":{"pane_id":"w18:p5","workspace_id":"w18"}}}`), nil, nil
		case args[0] == "pane" && args[1] == "list":
			return []byte(fixturePaneList), nil, nil
		default:
			return []byte(`{"id":"cli:ok","result":{"type":"ok"}}`), nil, nil
		}
	}
	return c
}

func operationOf(args []string) string {
	if len(args) < 2 {
		return strings.Join(args, " ")
	}
	return args[0] + " " + args[1]
}

func TestEachFailingStepLeavesTheRestMountedAndSaysSo(t *testing.T) {
	for _, c := range []struct {
		name    string
		fails   string
		want    string
		after   []string
		skipped []string
	}{
		{
			name:    "renaming the first tab",
			fails:   "tab rename",
			want:    "could not label tab",
			after:   []string{"pane split", "pane run", "pane rename", "tab create"},
			skipped: nil,
		},
		{
			name:    "opening the second tab",
			fails:   "tab create",
			want:    "could not open tab",
			after:   []string{"pane split", "pane run"},
			skipped: nil,
		},
		{
			name:    "splitting a pane",
			fails:   "pane split",
			want:    "could not open pane",
			after:   []string{"tab create", "pane run", "pane rename"},
			skipped: nil,
		},
		{
			name:    "running the pane's command",
			fails:   "pane run",
			want:    "could not run",
			after:   []string{"pane split", "pane rename", "tab create"},
			skipped: nil,
		},
		{
			name:  "renaming a pane",
			fails: "pane rename",
			want:  "could not label",
			after: []string{"pane split", "pane run", "tab create"},
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			cli := newCountingCLI(func(op string, _ []string) bool { return op == c.fails })

			warnings, err := cli.client().MountLayout(context.Background(),
				Container{WorkspaceID: "w18", PaneID: "w18:p1"}, testPlan())

			if err != nil {
				t.Fatalf("a step failure aborted the whole mount: %v", err)
			}
			joined := strings.Join(warnings, "\n")
			if !strings.Contains(joined, c.want) {
				t.Errorf("there is no warning for %q; the warnings are:\n%s", c.want, joined)
			}
			if len(warnings) > 0 && strings.TrimSpace(warnings[0]) == "" {
				t.Error("an empty warning is indistinguishable from not having warned")
			}
			for _, op := range c.after {
				if cli.calls[op] == 0 {
					t.Errorf("after failing %q, %q was not called: the mount stopped",
						c.fails, op)
				}
			}
		})
	}
}

// The dependency, not the panic: this is the case that justifies the `continue` after a failed
// split.
func TestPaneThatCannotOpenGetsNeitherCommandNorName(t *testing.T) {
	cli := newCountingCLI(func(op string, _ []string) bool { return op == "pane split" })

	warnings, err := cli.client().MountLayout(context.Background(),
		Container{WorkspaceID: "w18", PaneID: "w18:p1"}, testPlan())
	if err != nil {
		t.Fatalf("a split failure aborted the mount: %v", err)
	}
	if !strings.Contains(strings.Join(warnings, "\n"), "could not open pane") {
		t.Errorf("there is no warning for the failed split:\n%v", warnings)
	}

	attempts := cli.calls["pane split"]
	if attempts == 0 {
		t.Fatal("no split was attempted: the fixture never reached the point")
	}

	// The `run` count drops versus the good path, which is what says the panes that could not be
	//opened did NOT get their command.
	healthy := newCountingCLI(nil)
	if _, err := healthy.client().MountLayout(context.Background(),
		Container{WorkspaceID: "w18", PaneID: "w18:p1"}, testPlan()); err != nil {
		t.Fatalf("the healthy path did not mount: %v", err)
	}
	fromHealthy := healthy.calls["pane run"]
	if fromHealthy <= cli.calls["pane run"] {
		t.Errorf("%d pane run with all the splits down and %d with them up: some pane "+
			"that did not open received its command and got it injected into the previous one",
			cli.calls["pane run"], fromHealthy)
	}
	if cli.calls["pane run"] != cli.calls["pane rename"] {
		t.Errorf("%d pane run and %d pane rename", cli.calls["pane run"], cli.calls["pane rename"])
	}
	if cli.calls["tab create"] == 0 {
		t.Error("after all the splits failed the second tab was not opened: one pane's " +
			"failure stopped the whole layout")
	}
}

// The two exits of newWorkspace, which is what runs first.
func TestWithoutContainerTheLayoutOpensItsOwnWorkspaceAndFailsWithoutBasePane(t *testing.T) {
	cli := newCountingCLI(nil)
	warnings, err := cli.client().MountLayout(context.Background(), Container{}, testPlan())
	if err != nil {
		t.Fatalf("without a container it could not mount: %v", err)
	}
	if len(warnings) != 0 {
		t.Errorf("the mount without a container warned: %v", warnings)
	}
	if cli.calls["workspace create"] == 0 {
		t.Error("without a container no workspace was opened: that is the intended path")
	}

	withoutPane := newCountingCLI(func(op string, _ []string) bool { return false })
	withoutPane.respond = func(args []string) ([]byte, []byte, error) {
		if args[0] == "--version" {
			return []byte("herdr 0.9.1\n"), nil, nil
		}
		return []byte(`{"id":"cli:ws","result":{"type":"workspace_created",` +
			`"workspace":{"workspace_id":"w19"}}}`), nil, nil
	}
	if _, err := withoutPane.client().MountLayout(context.Background(), Container{}, testPlan()); err == nil {
		t.Error("a workspace with no base pane gave nil: the layout would open invisible " +
			"tabs and return 'mounted'")
	} else if !strings.Contains(err.Error(), "base pane") {
		t.Errorf("the error %q does not say the base pane is missing", err)
	}

	down := newCountingCLI(func(op string, _ []string) bool { return op == "workspace create" })
	if _, err := down.client().MountLayout(context.Background(), Container{}, testPlan()); err == nil {
		t.Error("a workspace that cannot be opened gave nil")
	} else if !strings.Contains(err.Error(), "review workspace") {
		t.Errorf("the error %q does not say the review workspace failed to open", err)
	}
}

func TestStaleWorkspaceFailsInsteadOfOpeningAnother(t *testing.T) {
	stale := newCountingCLI(func(op string, _ []string) bool { return op == "pane list" })
	stale.respond = func(args []string) ([]byte, []byte, error) {
		switch {
		case args[0] == "--version":
			return []byte("herdr 0.9.1\n"), nil, nil
		case args[0] == "pane" && args[1] == "list":
			return nil, []byte("no such workspace"), fmt.Errorf("exit status 1")
		default:
			return []byte(`{"id":"cli:ok","result":{"type":"ok"}}`), nil, nil
		}
	}
	_, err := stale.client().MountLayout(context.Background(),
		Container{WorkspaceID: "w99"}, testPlan())
	if err == nil {
		t.Fatal("a stale workspace gave nil: another one would be opened and the review left detached")
	}
	if !strings.Contains(err.Error(), "w99") {
		t.Errorf("the error %q does not name the stale workspace", err)
	}
	if !strings.Contains(err.Error(), "gone") {
		t.Errorf("the error %q does not say the workspace is gone", err)
	}

	// The neighbouring case, easiest to confuse with the previous one: the workspace EXISTS but has no
	//panes. That is not a Herdr error, it is a freshly created workspace.
	empty := newCountingCLI(nil)
	empty.respond = func(args []string) ([]byte, []byte, error) {
		switch {
		case args[0] == "--version":
			return []byte("herdr 0.9.1\n"), nil, nil
		case args[0] == "pane" && args[1] == "list":
			return []byte(`{"id":"cli:pl","result":{"type":"pane_list","panes":[]}}`), nil, nil
		default:
			return []byte(`{"id":"cli:ok","result":{"type":"ok"}}`), nil, nil
		}
	}
	_, err = empty.client().MountLayout(context.Background(),
		Container{WorkspaceID: "w99"}, testPlan())
	if err == nil {
		t.Fatal("a workspace with no panes gave nil")
	}
	if !strings.Contains(err.Error(), "no panes") {
		t.Errorf("the error %q does not tell 'there are no panes' from 'the workspace does not exist'", err)
	}
	if strings.Contains(err.Error(), "gone") {
		t.Errorf("the 'no panes' error says %q, which is the other case's message", err)
	}
}

func TestTabWithoutLabelIsNotRenamed(t *testing.T) {
	withoutLabel := plan.Plan{Tabs: []plan.Tab{
		{Label: "", Panes: []plan.Pane{
			{Kind: plan.KindTuicr, Label: "TUICR", Cwd: "/wt", Argv: []string{"tuicr"}},
		}},
		{Label: plan.LabelEdit, Panes: []plan.Pane{
			{Kind: plan.KindAgent, Label: "Agent", Cwd: "/wt", Argv: []string{"opencode"}},
		}},
	}}

	cli := newCountingCLI(func(op string, _ []string) bool { return op == "tab rename" })
	if _, err := cli.client().MountLayout(context.Background(),
		Container{WorkspaceID: "w18", PaneID: "w18:p1"}, withoutLabel); err != nil {
		t.Fatalf("a tab with no label aborted the mount: %v", err)
	}
	if cli.calls["tab rename"] != 0 {
		t.Errorf("tab rename was called %d times with the first tab having no label",
			cli.calls["tab rename"])
	}

	withLabel := newCountingCLI(nil)
	if _, err := withLabel.client().MountLayout(context.Background(),
		Container{WorkspaceID: "w18", PaneID: "w18:p1"}, testPlan()); err != nil {
		t.Fatalf("the healthy path failed: %v", err)
	}
	if withLabel.calls["tab rename"] == 0 {
		t.Error("with a label the tab was not renamed: the previous assertion would prove nothing")
	}
}
