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
type cliQueCuenta struct {
	*fakeCLI
	llamadas map[string]int
	falla    func(op string, args []string) bool
}

func nuevoCLIQueCuenta(falla func(op string, args []string) bool) *cliQueCuenta {
	c := &cliQueCuenta{
		fakeCLI:  &fakeCLI{},
		llamadas: map[string]int{},
		falla:    falla,
	}
	c.env = map[string]string{"HERDR_ENV": "1"}
	c.respond = func(args []string) ([]byte, []byte, error) {
		op := operacionDe(args)
		c.llamadas[op]++
		if c.falla != nil && c.falla(op, args) {
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

func operacionDe(args []string) string {
	if len(args) < 2 {
		return strings.Join(args, " ")
	}
	return args[0] + " " + args[1]
}

func TestCadaPasoQueFallaDejaElRestoMontadoYLoDice(t *testing.T) {
	for _, c := range []struct {
		nombre   string
		falla    string
		quiere   string
		despues  []string
		saltadas []string
	}{
		{
			nombre:   "renombrar el primer tab",
			falla:    "tab rename",
			quiere:   "could not label tab",
			despues:  []string{"pane split", "pane run", "pane rename", "tab create"},
			saltadas: nil,
		},
		{
			nombre:   "abrir el segundo tab",
			falla:    "tab create",
			quiere:   "could not open tab",
			despues:  []string{"pane split", "pane run"},
			saltadas: nil,
		},
		{
			nombre:   "dividir un pane",
			falla:    "pane split",
			quiere:   "could not open pane",
			despues:  []string{"tab create", "pane run", "pane rename"},
			saltadas: nil,
		},
		{
			nombre:   "lanzar el comando del pane",
			falla:    "pane run",
			quiere:   "could not run",
			despues:  []string{"pane split", "pane rename", "tab create"},
			saltadas: nil,
		},
		{
			nombre:  "renombrar un pane",
			falla:   "pane rename",
			quiere:  "could not label",
			despues: []string{"pane split", "pane run", "tab create"},
		},
	} {
		t.Run(c.nombre, func(t *testing.T) {
			cli := nuevoCLIQueCuenta(func(op string, _ []string) bool { return op == c.falla })

			warnings, err := cli.client().MountLayout(context.Background(),
				Container{WorkspaceID: "w18", PaneID: "w18:p1"}, testPlan())

			if err != nil {
				t.Fatalf("un fallo de un paso abortó el montaje entero: %v", err)
			}
			juntos := strings.Join(warnings, "\n")
			if !strings.Contains(juntos, c.quiere) {
				t.Errorf("no hay aviso de %q; los avisos son:\n%s", c.quiere, juntos)
			}
			if len(warnings) > 0 && strings.TrimSpace(warnings[0]) == "" {
				t.Error("un aviso vacío es indistinguible de no haber avisado")
			}
			for _, op := range c.despues {
				if cli.llamadas[op] == 0 {
					t.Errorf("tras fallar %q no se llamó a %q: el montaje se paró",
						c.falla, op)
				}
			}
		})
	}
}

// The dependency, not the panic: this is the case that justifies the `continue` after a failed
// split.
func TestUnPaneQueNoSePudeAbrirNoRecibeElComandoNiElNombre(t *testing.T) {
	cli := nuevoCLIQueCuenta(func(op string, _ []string) bool { return op == "pane split" })

	warnings, err := cli.client().MountLayout(context.Background(),
		Container{WorkspaceID: "w18", PaneID: "w18:p1"}, testPlan())
	if err != nil {
		t.Fatalf("un fallo de split abortó el montaje: %v", err)
	}
	if !strings.Contains(strings.Join(warnings, "\n"), "could not open pane") {
		t.Errorf("no hay aviso del split fallido:\n%v", warnings)
	}

	intentos := cli.llamadas["pane split"]
	if intentos == 0 {
		t.Fatal("no se intentó ningún split: el fixture no llegó al punto")
	}

	// The `run` count drops versus the good path, which is what says the panes that could not be
	//opened did NOT get their command.
	sano := nuevoCLIQueCuenta(nil)
	if _, err := sano.client().MountLayout(context.Background(),
		Container{WorkspaceID: "w18", PaneID: "w18:p1"}, testPlan()); err != nil {
		t.Fatalf("el camino bueno no montó: %v", err)
	}
	delBueno := sano.llamadas["pane run"]
	if delBueno <= cli.llamadas["pane run"] {
		t.Errorf("%d pane run con todos los splits caídos y %d con ellos bien: algún pane "+
			"que no se abrió recibió su comando y se lo metió al anterior",
			cli.llamadas["pane run"], delBueno)
	}
	if cli.llamadas["pane run"] != cli.llamadas["pane rename"] {
		t.Errorf("%d pane run y %d pane rename", cli.llamadas["pane run"], cli.llamadas["pane rename"])
	}
	if cli.llamadas["tab create"] == 0 {
		t.Error("tras fallar todos los splits no se abrió el segundo tab: el fallo de un " +
			"pane paró el layout entero")
	}
}

// The two exits of newWorkspace, which is what runs first.
func TestSinContenedorElLayoutAbreSuPropioWorkspaceYSinPaneBaseFalla(t *testing.T) {
	cli := nuevoCLIQueCuenta(nil)
	warnings, err := cli.client().MountLayout(context.Background(), Container{}, testPlan())
	if err != nil {
		t.Fatalf("sin contenedor no se pudo montar: %v", err)
	}
	if len(warnings) != 0 {
		t.Errorf("el montaje sin contenedor avisó: %v", warnings)
	}
	if cli.llamadas["workspace create"] == 0 {
		t.Error("sin contenedor no se abrió ningún workspace: es el camino previsto")
	}

	sinPane := nuevoCLIQueCuenta(func(op string, _ []string) bool { return false })
	sinPane.respond = func(args []string) ([]byte, []byte, error) {
		if args[0] == "--version" {
			return []byte("herdr 0.9.1\n"), nil, nil
		}
		return []byte(`{"id":"cli:ws","result":{"type":"workspace_created",` +
			`"workspace":{"workspace_id":"w19"}}}`), nil, nil
	}
	if _, err := sinPane.client().MountLayout(context.Background(), Container{}, testPlan()); err == nil {
		t.Error("un workspace sin pane base dio nil: el layout abriría tabs invisibles y " +
			"devolvería 'montado'")
	} else if !strings.Contains(err.Error(), "base pane") {
		t.Errorf("el error %q no dice que falta el pane base", err)
	}

	caido := nuevoCLIQueCuenta(func(op string, _ []string) bool { return op == "workspace create" })
	if _, err := caido.client().MountLayout(context.Background(), Container{}, testPlan()); err == nil {
		t.Error("un workspace que no se puede abrir dio nil")
	} else if !strings.Contains(err.Error(), "review workspace") {
		t.Errorf("el error %q no dice que falla la apertura del workspace del review", err)
	}
}

func TestUnWorkspaceObsoletoFallaEnVezDeAbrirOtro(t *testing.T) {
	obsoleto := nuevoCLIQueCuenta(func(op string, _ []string) bool { return op == "pane list" })
	obsoleto.respond = func(args []string) ([]byte, []byte, error) {
		switch {
		case args[0] == "--version":
			return []byte("herdr 0.9.1\n"), nil, nil
		case args[0] == "pane" && args[1] == "list":
			return nil, []byte("no such workspace"), fmt.Errorf("exit status 1")
		default:
			return []byte(`{"id":"cli:ok","result":{"type":"ok"}}`), nil, nil
		}
	}
	_, err := obsoleto.client().MountLayout(context.Background(),
		Container{WorkspaceID: "w99"}, testPlan())
	if err == nil {
		t.Fatal("un workspace obsoleto dio nil: se abriría otro y el review quedaría desligado")
	}
	if !strings.Contains(err.Error(), "w99") {
		t.Errorf("el error %q no nombra el workspace obsoleto", err)
	}
	if !strings.Contains(err.Error(), "gone") {
		t.Errorf("el error %q no dice que el workspace se fue", err)
	}

	// The neighbouring case, easiest to confuse with the previous one: the workspace EXISTS but has no
	//panes. That is not a Herdr error, it is a freshly created workspace.
	vacio := nuevoCLIQueCuenta(nil)
	vacio.respond = func(args []string) ([]byte, []byte, error) {
		switch {
		case args[0] == "--version":
			return []byte("herdr 0.9.1\n"), nil, nil
		case args[0] == "pane" && args[1] == "list":
			return []byte(`{"id":"cli:pl","result":{"type":"pane_list","panes":[]}}`), nil, nil
		default:
			return []byte(`{"id":"cli:ok","result":{"type":"ok"}}`), nil, nil
		}
	}
	_, err = vacio.client().MountLayout(context.Background(),
		Container{WorkspaceID: "w99"}, testPlan())
	if err == nil {
		t.Fatal("un workspace sin panes dio nil")
	}
	if !strings.Contains(err.Error(), "no panes") {
		t.Errorf("el error %q no distingue 'no hay panes' de 'no existe el workspace'", err)
	}
	if strings.Contains(err.Error(), "gone") {
		t.Errorf("el error de 'no hay panes' dice %q, que es el mensaje del otro caso", err)
	}
}

func TestUnTabSinEtiquetaNoSeIntentaRenombrar(t *testing.T) {
	sinEtiqueta := plan.Plan{Tabs: []plan.Tab{
		{Label: "", Panes: []plan.Pane{
			{Kind: plan.KindTuicr, Label: "TUICR", Cwd: "/wt", Argv: []string{"tuicr"}},
		}},
		{Label: plan.LabelEdit, Panes: []plan.Pane{
			{Kind: plan.KindAgent, Label: "Agente", Cwd: "/wt", Argv: []string{"opencode"}},
		}},
	}}

	cli := nuevoCLIQueCuenta(func(op string, _ []string) bool { return op == "tab rename" })
	if _, err := cli.client().MountLayout(context.Background(),
		Container{WorkspaceID: "w18", PaneID: "w18:p1"}, sinEtiqueta); err != nil {
		t.Fatalf("un tab sin etiqueta abortó el montaje: %v", err)
	}
	if cli.llamadas["tab rename"] != 0 {
		t.Errorf("se llamó a tab rename %d veces con el primer tab sin etiqueta",
			cli.llamadas["tab rename"])
	}

	conEtiqueta := nuevoCLIQueCuenta(nil)
	if _, err := conEtiqueta.client().MountLayout(context.Background(),
		Container{WorkspaceID: "w18", PaneID: "w18:p1"}, testPlan()); err != nil {
		t.Fatalf("el camino bueno falló: %v", err)
	}
	if conEtiqueta.llamadas["tab rename"] == 0 {
		t.Error("con etiqueta no se renombró el tab: el aserto anterior no probaría nada")
	}
}
