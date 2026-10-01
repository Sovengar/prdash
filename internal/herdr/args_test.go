package herdr

import (
	"reflect"
	"testing"
	"time"
)

// TestArgsWithNoMandaElVacioNiUnFlag: la regla única de los flags opcionales.
//
// Un flag con valor vacío NO se manda, y no por descuido: `--cwd ""` le dice a Herdr
// "usa el directorio vacío", que no es lo mismo que no decir nada. La diferencia
// sale en el sitio más malo, que es abrir el repo equivocado.
//
// Y con el valor puesto, el flag va con su valor detrás y en ese orden. El orden
// importa porque Herdr distingue `tab create --label` de `tab rename`, y un par
// flag/valor desordenado no da error: da el valor al flag anterior.
func TestArgsWithNoMandaElVacioNiUnFlag(t *testing.T) {
	base := args{"comando"}

	// Vacío: no se añade nada, y la lista no se toca.
	if got := base.with("--cwd", ""); !reflect.DeepEqual(got, args{"comando"}) {
		t.Errorf("con el valor vacío salió %q, want solo el comando: un flag vacío no se manda", got)
	}
	// Y la lista de entrada no se modifica: los métodos la reutilizan, y mutarla
	// dejaría flags de la llamada anterior en la siguiente.
	antes := append(args{}, base...)
	base.with("--cwd", "/repo")
	if !reflect.DeepEqual(base, antes) {
		t.Errorf("with mutó la lista de entrada: %q", base)
	}

	// Con valor: el flag y el valor, y en ese orden.
	if got := base.with("--cwd", "/repo"); !reflect.DeepEqual(got, args{"comando", "--cwd", "/repo"}) {
		t.Errorf("con valor salió %q, want [comando --cwd /repo]", got)
	}
	// Y encadenados, en el orden en que se encadenan.
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
	// Y encadenado con with, que es como se usa de verdad.
	if got := base.with("--cwd", "/r").withIf(true, "--no-focus"); !reflect.DeepEqual(got,
		args{"comando", "--cwd", "/r", "--no-focus"}) {
		t.Errorf("encadenado salió %q, want [comando --cwd /r --no-focus]", got)
	}
}

// TestArgvDeCadaComando: el argv COMPLETO de cada comando, con la lista entera
// afirmada.
//
// Esto no es un golden de un dibujo: es una lista, y una lista se compara entera.
// Lo que se afirma es el contrato con la CLI, y el contrato tiene tres partes que
// aquí se ven las tres:
//
//   - qué flags van, que es la pregunta que respondía el guard en línea;
//   - en qué orden, porque Herdr distingue `tab create --label` de `tab rename` y un
//     par desordenado no da error, da el valor al flag anterior;
//   - y qué NO va, que es la mitad del trabajo: un flag opcional vacío se omite, y
//     un flag que se manda de más es un error de la CLI.
func TestArgvDeCadaComando(t *testing.T) {
	casos := []struct {
		nombre string
		got    []string
		want   []string
	}{
		// worktree create: todos los flags opcionales, en orden.
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

		// --force es opt-in: quitar un checkout con cambios sin preguntar es peor
		// que no poder hacerlo.
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

		// El ratio se omite si es cero, porque cero es el default de Herdr y
		// mandarlo sobrescribe ese default. Y un ratio de 0 además es imposible
		// (es una fracción), así que mandarlo no da error: da un pane de tamaño 0.
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
		// El env vacío NO se manda: `--env ""` le dice a Herdr que defina una
		// variable sin nombre.
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

		// El timeout se omite si no hay plazo: un timeout de 0 quiere decir "sin
		// límite", no "cero segundos", y mandarlo haría fallar la espera al
		// instante.
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

		// El sonido vacío es el default de Herdr, y mandarlo fijaría uno.
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

// TestArgvNuncaLevaUnValorVacio: por encima de cada comando, una regla.
//
// Ningún flag con valor puede llevar una cadena vacía detrás, y ningún flag sin
// valor puede faltar si su condición se cumple. Es la versión agregada de lo que
// se afirma caso por caso, y sirve para los casos que nadie escribió: un comando
// nuevo, un flag nuevo.
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

	// Los flags que llevan valor detrás, para saber dónde no puede haber un vacío.
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
			// Un flag con valor, seguido de nada, es un argv que la CLI va a
			// rechazar. Y un valor que empieza por -- también, porque se lee como
			// otro flag: por eso el cwd vacío es peor que no mandarlo.
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
