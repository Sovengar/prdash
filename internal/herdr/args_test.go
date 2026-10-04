package herdr

import (
	"reflect"
	"testing"
	"time"
)

// A flag with an empty value is NOT sent: `--cwd ""` tells Herdr "use the empty directory", which is
// not the same as saying nothing, and that difference lands in the wrong repo.
func TestArgsWithNoMandaElVacioNiUnFlag(t *testing.T) {
	base := args{"comando"}

	if got := base.with("--cwd", ""); !reflect.DeepEqual(got, args{"comando"}) {
		t.Errorf("con el valor vacío salió %q, want solo el comando: un flag vacío no se manda", got)
	}
	// The input list is not modified: the methods reuse it, and mutating it would leave flags
	// behind.
	antes := append(args{}, base...)
	base.with("--cwd", "/repo")
	if !reflect.DeepEqual(base, antes) {
		t.Errorf("with mutó la lista de entrada: %q", base)
	}

	if got := base.with("--cwd", "/repo"); !reflect.DeepEqual(got, args{"comando", "--cwd", "/repo"}) {
		t.Errorf("con valor salió %q, want [comando --cwd /repo]", got)
	}
	if got := base.with("--a", "1").with("--b", "2").with("--c", ""); !reflect.DeepEqual(got,
		args{"comando", "--a", "1", "--b", "2"}) {
		t.Errorf("encadenados salieron %q, want [comando --a 1 --b 2]: el vacío no debe desplazar a los demás", got)
	}
}

// TestArgsWithIfApendaElFlagEnteroONada: un flag sin valor, o va o no va.
func TestArgsWithIfApendaElFlagEnteroONada(t *testing.T) {
	base := args{"comando"}
	if got := base.withIf(false, "--force"); !reflect.DeepEqual(got, args{"comando"}) {
		t.Errorf("con la condición falsa salió %q, want solo el comando", got)
	}
	if got := base.withIf(true, "--force"); !reflect.DeepEqual(got, args{"comando", "--force"}) {
		t.Errorf("con la condición cierta salió %q, want [comando --force]", got)
	}
	if got := base.with("--cwd", "/r").withIf(true, "--no-focus"); !reflect.DeepEqual(got,
		args{"comando", "--cwd", "/r", "--no-focus"}) {
		t.Errorf("encadenado salió %q, want [comando --cwd /r --no-focus]", got)
	}
}

// The FULL argv of each command with the whole list asserted. Not a golden of a drawing: a list, and
// the list is the contract.
func TestArgvDeCadaComando(t *testing.T) {
	casos := []struct {
		nombre string
		got    []string
		want   []string
	}{
		{
			"worktree create vacío",
			worktreeCreateArgs(WorktreeSpec{}),
			[]string{"worktree", "create"},
		},
		{
			"worktree create con todo",
			worktreeCreateArgs(WorktreeSpec{
				Cwd: "/repo", Branch: "feat", Path: "/wt", Label: "etiqueta", NoFocus: true,
			}),
			[]string{"worktree", "create", "--cwd", "/repo", "--branch", "feat",
				"--path", "/wt", "--label", "etiqueta", "--no-focus"},
		},
		{
			"worktree create a medias",
			worktreeCreateArgs(WorktreeSpec{Cwd: "/repo", NoFocus: true}),
			[]string{"worktree", "create", "--cwd", "/repo", "--no-focus"},
		},

		{
			"worktree list sin cwd",
			worktreeListArgs(""),
			[]string{"worktree", "list"},
		},
		{
			"worktree list con cwd",
			worktreeListArgs("/repo"),
			[]string{"worktree", "list", "--cwd", "/repo"},
		},

		// --force is opt-in: dropping a checkout with changes without asking is worse than not being
		// able to.
		{
			"worktree remove sin force",
			worktreeRemoveArgs("ws1", false),
			[]string{"worktree", "remove", "--workspace", "ws1"},
		},
		{
			"worktree remove con force",
			worktreeRemoveArgs("ws1", true),
			[]string{"worktree", "remove", "--workspace", "ws1", "--force"},
		},

		{
			"workspace create vacío",
			workspaceCreateArgs(WorkspaceSpec{}),
			[]string{"workspace", "create"},
		},
		{
			"workspace create con todo",
			workspaceCreateArgs(WorkspaceSpec{Cwd: "/repo", Label: "et", NoFocus: true}),
			[]string{"workspace", "create", "--cwd", "/repo", "--label", "et", "--no-focus"},
		},

		{
			"workspace close sin group",
			workspaceCloseArgs("ws1", false),
			[]string{"workspace", "close", "ws1"},
		},
		{
			"workspace close con group",
			workspaceCloseArgs("ws1", true),
			[]string{"workspace", "close", "ws1", "--group"},
		},

		{
			"tab create vacío",
			tabCreateArgs(TabSpec{}),
			[]string{"tab", "create"},
		},
		{
			"tab create con todo",
			tabCreateArgs(TabSpec{WorkspaceID: "ws1", Cwd: "/repo", Label: "et", NoFocus: true}),
			[]string{"tab", "create", "--workspace", "ws1", "--cwd", "/repo",
				"--label", "et", "--no-focus"},
		},

		// The ratio is omitted when zero, because zero is Herdr's default and sending it would overwrite
		// it.
		{
			"pane split sin ratio",
			splitArgs(SplitSpec{PaneID: "p1", Direction: "right"}),
			[]string{"pane", "split", "--pane", "p1", "--direction", "right"},
		},
		{
			"pane split con ratio",
			splitArgs(SplitSpec{PaneID: "p1", Direction: "down", Ratio: 0.5}),
			[]string{"pane", "split", "--pane", "p1", "--direction", "down", "--ratio", "0.5"},
		},
		{
			"pane split con ratio entero",
			splitArgs(SplitSpec{PaneID: "p1", Direction: "right", Ratio: 1}),
			[]string{"pane", "split", "--pane", "p1", "--direction", "right", "--ratio", "1"},
		},
		// An empty env is NOT sent: `--env ""` tells Herdr to define an unnamed variable.
		{
			"pane split con env vacío en medio",
			splitArgs(SplitSpec{PaneID: "p1", Direction: "right", Env: []string{"A=1", "", "B=2"}}),
			[]string{"pane", "split", "--pane", "p1", "--direction", "right",
				"--env", "A=1", "--env", "B=2"},
		},
		{
			"pane split con todo",
			splitArgs(SplitSpec{PaneID: "p1", Direction: "down", Ratio: 0.25,
				Cwd: "/repo", Env: []string{"A=1", "B=2"}, NoFocus: true}),
			[]string{"pane", "split", "--pane", "p1", "--direction", "down",
				"--ratio", "0.25", "--cwd", "/repo",
				"--env", "A=1", "--env", "B=2", "--no-focus"},
		},
		{
			"pane split con env de un solo vacío",
			splitArgs(SplitSpec{PaneID: "p1", Direction: "right", Env: []string{""}}),
			[]string{"pane", "split", "--pane", "p1", "--direction", "right"},
		},

		// The timeout is omitted when there is no deadline: a timeout of 0 means "no limit", not "as soon
		// as possible".
		{
			"wait-output sin timeout",
			waitOutputArgs("p1", "listo", 0),
			[]string{"pane", "wait-output", "--match", "listo", "p1"},
		},
		{
			"wait-output con timeout",
			waitOutputArgs("p1", "listo", 5*time.Second),
			[]string{"pane", "wait-output", "--match", "listo", "p1", "--timeout", "5000"},
		},
		{
			"wait-output con timeout sub-segundo",
			waitOutputArgs("p1", "x", 1500*time.Millisecond),
			[]string{"pane", "wait-output", "--match", "x", "p1", "--timeout", "1500"},
		},

		{
			"pane list sin workspace",
			paneListArgs(""),
			[]string{"pane", "list"},
		},
		{
			"pane list con workspace",
			paneListArgs("ws1"),
			[]string{"pane", "list", "--workspace", "ws1"},
		},

		{
			"notify sin nada",
			notifyArgs("titulo", NotifyOptions{}),
			[]string{"notification", "show", "titulo"},
		},
		{
			"notify con todo",
			notifyArgs("titulo", NotifyOptions{Body: "cuerpo", Sound: "done"}),
			[]string{"notification", "show", "titulo", "--body", "cuerpo", "--sound", "done"},
		},
		{
			"notify con solo body",
			notifyArgs("t", NotifyOptions{Body: "c"}),
			[]string{"notification", "show", "t", "--body", "c"},
		},
		{
			"notify con solo sound",
			notifyArgs("t", NotifyOptions{Sound: "request"}),
			[]string{"notification", "show", "t", "--sound", "request"},
		},
	}

	for _, c := range casos {
		if !reflect.DeepEqual([]string(c.got), c.want) {
			t.Errorf("%s:\n  dio  %q\n  want %q", c.nombre, c.got, c.want)
		}
	}
}

// No value flag may carry an empty string, and no valueless flag may carry one.
func TestArgvNuncaLevaUnValorVacio(t *testing.T) {
	argv := map[string][]string{
		"worktree create":       []string(worktreeCreateArgs(WorktreeSpec{Cwd: "/a", Branch: "b", Path: "/c", Label: "d", NoFocus: true})),
		"worktree create vacío": []string(worktreeCreateArgs(WorktreeSpec{})),
		"worktree list":         []string(worktreeListArgs("")),
		"worktree remove":       []string(worktreeRemoveArgs("ws", true)),
		"workspace create":      []string(workspaceCreateArgs(WorkspaceSpec{Cwd: "/a", Label: "b", NoFocus: true})),
		"workspace close":       []string(workspaceCloseArgs("ws", true)),
		"tab create":            []string(tabCreateArgs(TabSpec{WorkspaceID: "w", Cwd: "/a", Label: "b", NoFocus: true})),
		"tab create vacío":      []string(tabCreateArgs(TabSpec{})),
		"pane split":            []string(splitArgs(SplitSpec{PaneID: "p", Direction: "right", Ratio: 0.5, Cwd: "/a", Env: []string{"A=1"}, NoFocus: true})),
		"pane split vacío":      []string(splitArgs(SplitSpec{PaneID: "p", Direction: "right"})),
		"wait-output":           []string(waitOutputArgs("p", "m", time.Second)),
		"wait-output vacío":     []string(waitOutputArgs("p", "m", 0)),
		"pane list":             []string(paneListArgs("ws")),
		"pane list vacío":       []string(paneListArgs("")),
		"notify":                []string(notifyArgs("t", NotifyOptions{Body: "b", Sound: "done"})),
		"notify vacío":          []string(notifyArgs("t", NotifyOptions{})),
	}

	conValor := map[string]bool{
		"--cwd": true, "--branch": true, "--path": true, "--label": true,
		"--workspace": true, "--body": true, "--sound": true,
		"--ratio": true, "--timeout": true, "--env": true, "--pane": true, "--match": true,
	}

	for nombre, a := range argv {
		if len(a) == 0 {
			t.Errorf("%s: argv vacío", nombre)
		}
		for i, s := range a {
			if s == "" {
				t.Errorf("%s: el elemento %d es una cadena vacía: %q", nombre, i, a)
			}
			// A value flag followed by nothing is an argv the CLI will reject.
			if conValor[s] {
				if i+1 >= len(a) {
					t.Errorf("%s: el flag %q es el último y no lleva valor: %q", nombre, s, a)
					continue
				}
				if siguiente := a[i+1]; siguiente == "" {
					t.Errorf("%s: el flag %q lleva un valor vacío: %q", nombre, s, a)
				} else if len(siguiente) > 1 && siguiente[0] == '-' && siguiente[1] == '-' {
					t.Errorf("%s: el flag %q lleva el valor %q, que la CLI leería como otro flag: %q",
						nombre, s, siguiente, a)
				}
			}
		}
	}
}
