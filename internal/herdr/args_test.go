package herdr

import (
	"reflect"
	"testing"
	"time"
)

// A flag with an empty value is NOT sent: `--cwd ""` tells Herdr "use the empty directory", which is
// not the same as saying nothing, and that difference lands in the wrong repo.
func TestArgsWithOmitsEmptyValue(t *testing.T) {
	base := args{"command"}

	if got := base.with("--cwd", ""); !reflect.DeepEqual(got, args{"command"}) {
		t.Errorf("with the empty value it gave %q, want only the command: an empty flag is not sent", got)
	}
	// The input list is not modified: the methods reuse it, and mutating it would leave flags
	// behind.
	before := append(args{}, base...)
	base.with("--cwd", "/repo")
	if !reflect.DeepEqual(base, before) {
		t.Errorf("with mutated the input list: %q", base)
	}

	if got := base.with("--cwd", "/repo"); !reflect.DeepEqual(got, args{"command", "--cwd", "/repo"}) {
		t.Errorf("with value it gave %q, want [command --cwd /repo]", got)
	}
	if got := base.with("--a", "1").with("--b", "2").with("--c", ""); !reflect.DeepEqual(got,
		args{"command", "--a", "1", "--b", "2"}) {
		t.Errorf("chained gave %q, want [command --a 1 --b 2]: the empty one must not displace the others", got)
	}
}

// TestArgsWithIfAppendsWholeFlagOrNothing: a flag without a value either goes or it does not.
func TestArgsWithIfAppendsWholeFlagOrNothing(t *testing.T) {
	base := args{"command"}
	if got := base.withIf(false, "--force"); !reflect.DeepEqual(got, args{"command"}) {
		t.Errorf("with a false condition it gave %q, want only the command", got)
	}
	if got := base.withIf(true, "--force"); !reflect.DeepEqual(got, args{"command", "--force"}) {
		t.Errorf("with a true condition it gave %q, want [command --force]", got)
	}
	if got := base.with("--cwd", "/r").withIf(true, "--no-focus"); !reflect.DeepEqual(got,
		args{"command", "--cwd", "/r", "--no-focus"}) {
		t.Errorf("chained gave %q, want [command --cwd /r --no-focus]", got)
	}
}

// The FULL argv of each command with the whole list asserted. Not a golden of a drawing: a list, and
// the list is the contract.
func TestArgvOfEachCommand(t *testing.T) {
	cases := []struct {
		name string
		got  []string
		want []string
	}{
		{
			"worktree create empty",
			worktreeCreateArgs(WorktreeSpec{}),
			[]string{"worktree", "create"},
		},
		{
			"worktree create with everything",
			worktreeCreateArgs(WorktreeSpec{
				Cwd: "/repo", Branch: "feat", Path: "/wt", Label: "label", NoFocus: true,
			}),
			[]string{"worktree", "create", "--cwd", "/repo", "--branch", "feat",
				"--path", "/wt", "--label", "label", "--no-focus"},
		},
		{
			"worktree create partially set",
			worktreeCreateArgs(WorktreeSpec{Cwd: "/repo", NoFocus: true}),
			[]string{"worktree", "create", "--cwd", "/repo", "--no-focus"},
		},

		{
			"worktree list without cwd",
			worktreeListArgs(""),
			[]string{"worktree", "list"},
		},
		{
			"worktree list with cwd",
			worktreeListArgs("/repo"),
			[]string{"worktree", "list", "--cwd", "/repo"},
		},

		// --force is opt-in: dropping a checkout with changes without asking is worse than not being
		// able to.
		{
			"worktree remove without force",
			worktreeRemoveArgs("ws1", false),
			[]string{"worktree", "remove", "--workspace", "ws1"},
		},
		{
			"worktree remove with force",
			worktreeRemoveArgs("ws1", true),
			[]string{"worktree", "remove", "--workspace", "ws1", "--force"},
		},

		{
			"workspace create empty",
			workspaceCreateArgs(WorkspaceSpec{}),
			[]string{"workspace", "create"},
		},
		{
			"workspace create with everything",
			workspaceCreateArgs(WorkspaceSpec{Cwd: "/repo", Label: "et", NoFocus: true}),
			[]string{"workspace", "create", "--cwd", "/repo", "--label", "et", "--no-focus"},
		},

		{
			"workspace close without group",
			workspaceCloseArgs("ws1", false),
			[]string{"workspace", "close", "ws1"},
		},
		{
			"workspace close with group",
			workspaceCloseArgs("ws1", true),
			[]string{"workspace", "close", "ws1", "--group"},
		},

		{
			"tab create empty",
			tabCreateArgs(TabSpec{}),
			[]string{"tab", "create"},
		},
		{
			"tab create with everything",
			tabCreateArgs(TabSpec{WorkspaceID: "ws1", Cwd: "/repo", Label: "et", NoFocus: true}),
			[]string{"tab", "create", "--workspace", "ws1", "--cwd", "/repo",
				"--label", "et", "--no-focus"},
		},

		// The ratio is omitted when zero, because zero is Herdr's default and sending it would overwrite
		// it.
		{
			"pane split without ratio",
			splitArgs(SplitSpec{PaneID: "p1", Direction: "right"}),
			[]string{"pane", "split", "--pane", "p1", "--direction", "right"},
		},
		{
			"pane split with ratio",
			splitArgs(SplitSpec{PaneID: "p1", Direction: "down", Ratio: 0.5}),
			[]string{"pane", "split", "--pane", "p1", "--direction", "down", "--ratio", "0.5"},
		},
		{
			"pane split with integer ratio",
			splitArgs(SplitSpec{PaneID: "p1", Direction: "right", Ratio: 1}),
			[]string{"pane", "split", "--pane", "p1", "--direction", "right", "--ratio", "1"},
		},
		// An empty env is NOT sent: `--env ""` tells Herdr to define an unnamed variable.
		{
			"pane split with empty env in the middle",
			splitArgs(SplitSpec{PaneID: "p1", Direction: "right", Env: []string{"A=1", "", "B=2"}}),
			[]string{"pane", "split", "--pane", "p1", "--direction", "right",
				"--env", "A=1", "--env", "B=2"},
		},
		{
			"pane split with everything",
			splitArgs(SplitSpec{PaneID: "p1", Direction: "down", Ratio: 0.25,
				Cwd: "/repo", Env: []string{"A=1", "B=2"}, NoFocus: true}),
			[]string{"pane", "split", "--pane", "p1", "--direction", "down",
				"--ratio", "0.25", "--cwd", "/repo",
				"--env", "A=1", "--env", "B=2", "--no-focus"},
		},
		{
			"pane split with env of a single empty",
			splitArgs(SplitSpec{PaneID: "p1", Direction: "right", Env: []string{""}}),
			[]string{"pane", "split", "--pane", "p1", "--direction", "right"},
		},

		// The timeout is omitted when there is no deadline: a timeout of 0 means "no limit", not "as soon
		// as possible".
		{
			"wait-output without timeout",
			waitOutputArgs("p1", "ready", 0),
			[]string{"pane", "wait-output", "--match", "ready", "p1"},
		},
		{
			"wait-output with timeout",
			waitOutputArgs("p1", "ready", 5*time.Second),
			[]string{"pane", "wait-output", "--match", "ready", "p1", "--timeout", "5000"},
		},
		{
			"wait-output with sub-second timeout",
			waitOutputArgs("p1", "x", 1500*time.Millisecond),
			[]string{"pane", "wait-output", "--match", "x", "p1", "--timeout", "1500"},
		},

		{
			"pane list without workspace",
			paneListArgs(""),
			[]string{"pane", "list"},
		},
		{
			"pane list with workspace",
			paneListArgs("ws1"),
			[]string{"pane", "list", "--workspace", "ws1"},
		},

		{
			"notify with nothing",
			notifyArgs("title", NotifyOptions{}),
			[]string{"notification", "show", "title"},
		},
		{
			"notify with everything",
			notifyArgs("title", NotifyOptions{Body: "body", Sound: "done"}),
			[]string{"notification", "show", "title", "--body", "body", "--sound", "done"},
		},
		{
			"notify with only body",
			notifyArgs("t", NotifyOptions{Body: "c"}),
			[]string{"notification", "show", "t", "--body", "c"},
		},
		{
			"notify with only sound",
			notifyArgs("t", NotifyOptions{Sound: "request"}),
			[]string{"notification", "show", "t", "--sound", "request"},
		},
	}

	for _, c := range cases {
		if !reflect.DeepEqual([]string(c.got), c.want) {
			t.Errorf("%s:\n  got  %q\n  want %q", c.name, c.got, c.want)
		}
	}
}

// No value flag may carry an empty string, and no valueless flag may carry one.
func TestArgvNeverCarriesAnEmptyValue(t *testing.T) {
	argv := map[string][]string{
		"worktree create":       []string(worktreeCreateArgs(WorktreeSpec{Cwd: "/a", Branch: "b", Path: "/c", Label: "d", NoFocus: true})),
		"worktree create empty": []string(worktreeCreateArgs(WorktreeSpec{})),
		"worktree list":         []string(worktreeListArgs("")),
		"worktree remove":       []string(worktreeRemoveArgs("ws", true)),
		"workspace create":      []string(workspaceCreateArgs(WorkspaceSpec{Cwd: "/a", Label: "b", NoFocus: true})),
		"workspace close":       []string(workspaceCloseArgs("ws", true)),
		"tab create":            []string(tabCreateArgs(TabSpec{WorkspaceID: "w", Cwd: "/a", Label: "b", NoFocus: true})),
		"tab create empty":      []string(tabCreateArgs(TabSpec{})),
		"pane split":            []string(splitArgs(SplitSpec{PaneID: "p", Direction: "right", Ratio: 0.5, Cwd: "/a", Env: []string{"A=1"}, NoFocus: true})),
		"pane split empty":      []string(splitArgs(SplitSpec{PaneID: "p", Direction: "right"})),
		"wait-output":           []string(waitOutputArgs("p", "m", time.Second)),
		"wait-output empty":     []string(waitOutputArgs("p", "m", 0)),
		"pane list":             []string(paneListArgs("ws")),
		"pane list empty":       []string(paneListArgs("")),
		"notify":                []string(notifyArgs("t", NotifyOptions{Body: "b", Sound: "done"})),
		"notify empty":          []string(notifyArgs("t", NotifyOptions{})),
	}

	withValue := map[string]bool{
		"--cwd": true, "--branch": true, "--path": true, "--label": true,
		"--workspace": true, "--body": true, "--sound": true,
		"--ratio": true, "--timeout": true, "--env": true, "--pane": true, "--match": true,
	}

	for name, a := range argv {
		if len(a) == 0 {
			t.Errorf("%s: empty argv", name)
		}
		for i, s := range a {
			if s == "" {
				t.Errorf("%s: element %d is an empty string: %q", name, i, a)
			}
			// A value flag followed by nothing is an argv the CLI will reject.
			if withValue[s] {
				if i+1 >= len(a) {
					t.Errorf("%s: flag %q is last and carries no value: %q", name, s, a)
					continue
				}
				if next := a[i+1]; next == "" {
					t.Errorf("%s: flag %q carries an empty value: %q", name, s, a)
				} else if len(next) > 1 && next[0] == '-' && next[1] == '-' {
					t.Errorf("%s: flag %q carries the value %q, which the CLI would read as another flag: %q",
						name, s, next, a)
				}
			}
		}
	}
}
