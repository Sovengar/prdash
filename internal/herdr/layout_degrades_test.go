package herdr

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"prdash/internal/review/plan"
)

// Two exits from the same `if err != nil` and both are warnings, not errors.
func TestTabThatDoesNotOpenDoesNotDragTheRestAndNamesTheTab(t *testing.T) {
	for _, c := range []struct {
		name    string
		respond func(args []string) ([]byte, []byte, error)
		want    string
	}{
		{
			name: "Herdr fails to open the tab",
			respond: func(args []string) ([]byte, []byte, error) {
				if args[0] == "tab" && args[1] == "create" {
					return nil, []byte("tab limit reached"), fmt.Errorf("exit status 1")
				}
				return respondNormally(args)
			},
			want: "tab limit reached",
		},
		{
			name: "Herdr opens the tab but with no root pane",
			respond: func(args []string) ([]byte, []byte, error) {
				if args[0] == "tab" && args[1] == "create" {
					return []byte(`{"id":"cli:tab:create","result":{"type":"tab_created",` +
						`"tab":{"tab_id":"w18:t1"}}}`), nil, nil
				}
				return respondNormally(args)
			},
			want: "no root pane",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			cli := newCountingCLI(nil)
			cli.respond = func(args []string) ([]byte, []byte, error) {
				cli.calls[operationOf(args)]++
				return c.respond(args)
			}

			warnings, err := cli.client().MountLayout(context.Background(),
				Container{WorkspaceID: "w18", PaneID: "w18:p1"}, testPlan())
			if err != nil {
				t.Fatalf("a tab failure aborted the layout: %v", err)
			}
			joined := strings.Join(warnings, "\n")
			if !strings.Contains(joined, c.want) {
				t.Errorf("the warning %q does not say %q", joined, c.want)
			}
			// The warning NAMES the tab, which is what tells which of the two failed.
			if !strings.Contains(joined, "Edit") {
				t.Errorf("the warning does not name the tab that failed:\n%s", joined)
			}
			// The first tab was built whole, which is what tells "it continued" from "there was nothing".
			if cli.calls["pane split"] == 0 || cli.calls["pane run"] == 0 {
				t.Error("the first tab was not mounted: the second one's failure stopped the whole layout")
			}
		})
	}
}

func respondNormally(args []string) ([]byte, []byte, error) {
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

// tabOf exists because Herdr has no "the tab of this pane".
func TestTabOfReturnsEmptyWithoutPaneAndDoesNotBreakTheLayout(t *testing.T) {
	ctx := context.Background()

	empty := newCountingCLI(nil)
	if got := empty.client().tabOf(ctx, "w18", ""); got != "" {
		t.Errorf("tabOf without a pane gave %q, want empty", got)
	}
	if empty.calls["pane list"] != 0 {
		t.Error("tabOf without a pane asked Herdr: it had nothing to ask with")
	}

	failing := newCountingCLI(func(op string, _ []string) bool { return op == "pane list" })
	failing.respond = func(args []string) ([]byte, []byte, error) {
		if args[0] == "pane" && args[1] == "list" {
			return nil, []byte("no such workspace"), fmt.Errorf("exit status 1")
		}
		return respondNormally(args)
	}
	if got := failing.client().tabOf(ctx, "w18", "w18:p1"); got != "" {
		t.Errorf("tabOf with the list failing gave %q, want empty", got)
	}
	if _, err := failing.client().MountLayout(ctx,
		Container{WorkspaceID: "w18", PaneID: "w18:p1"}, testPlan()); err != nil {
		t.Errorf("a tab with no name broke the layout: %v. Naming the tab is cosmetic", err)
	}

	ok := newCountingCLI(nil)
	if got := ok.client().tabOf(ctx, "w18", "w18:p1"); got == "" {
		t.Error("tabOf did not find a pane that IS in the list")
	}
}

// The last boundary with the shell that is left: the variable list.
func TestUnnamedEnvironmentVariableDoesNotEnterThePaneCommand(t *testing.T) {
	good := plan.Pane{
		Kind: plan.KindTuicr, Label: "TUICR", Cwd: "/wt",
		Argv: []string{"tuicr", "pr", "https://github.com/o/r/pull/7"},
		Env:  []string{"PRDASH_NUMBER=7", "TERM=xterm-256color"},
	}
	if got := paneCommand(good); !strings.Contains(got, "PRDASH_NUMBER=7") ||
		!strings.Contains(got, "TERM=xterm-256color") {
		t.Errorf("the good variables did not reach the command: %q", got)
	}

	for _, bad := range [][]string{
		{"SIN_IGUAL"},
		{"=sin-nombre"},
		{"BUENA=1", "SIN_IGUAL", "=tampoco", "OTRA=2"},
	} {
		p := good
		p.Env = bad
		got := paneCommand(p)
		for _, entry := range bad {
			if entry == "BUENA=1" || entry == "OTRA=2" {
				if !strings.Contains(got, entry) {
					t.Errorf("with %v the good variable %q was lost", bad, entry)
				}
				continue
			}
			if strings.Contains(got, entry) {
				t.Errorf("with %v, the invalid entry %q reached the command: %q", bad, entry, got)
			}
		}
		if len(bad) > 2 && !strings.Contains(got, "BUENA=1") {
			t.Errorf("the filter removed the good ones too: %q", got)
		}
	}
}
