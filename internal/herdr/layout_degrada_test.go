package herdr

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"prdash/internal/review/plan"
)

// Two exits from the same `if err != nil` and both are warnings, not errors.
func TestUnTabQueNoSeAbreNoTiraElRestoYLoDicePorTab(t *testing.T) {
	for _, c := range []struct {
		nombre   string
		responde func(args []string) ([]byte, []byte, error)
		quiere   string
	}{
		{
			nombre: "Herdr falla al abrir el tab",
			responde: func(args []string) ([]byte, []byte, error) {
				if args[0] == "tab" && args[1] == "create" {
					return nil, []byte("tab limit reached"), fmt.Errorf("exit status 1")
				}
				return respondeNormal(args)
			},
			quiere: "tab limit reached",
		},
		{
			nombre: "Herdr abre el tab pero sin pane raíz",
			responde: func(args []string) ([]byte, []byte, error) {
				if args[0] == "tab" && args[1] == "create" {
					return []byte(`{"id":"cli:tab:create","result":{"type":"tab_created",` +
						`"tab":{"tab_id":"w18:t1"}}}`), nil, nil
				}
				return respondeNormal(args)
			},
			quiere: "no root pane",
		},
	} {
		t.Run(c.nombre, func(t *testing.T) {
			cli := nuevoCLIQueCuenta(nil)
			cli.respond = func(args []string) ([]byte, []byte, error) {
				cli.llamadas[operacionDe(args)]++
				return c.responde(args)
			}

			warnings, err := cli.client().MountLayout(context.Background(),
				Container{WorkspaceID: "w18", PaneID: "w18:p1"}, testPlan())
			if err != nil {
				t.Fatalf("el fallo de un tab abortó el layout: %v", err)
			}
			juntos := strings.Join(warnings, "\n")
			if !strings.Contains(juntos, c.quiere) {
				t.Errorf("el aviso %q no dice %q", juntos, c.quiere)
			}
			// The warning NAMES the tab, which is what tells which of the two failed.
			if !strings.Contains(juntos, "Edit") {
				t.Errorf("el aviso no nombra el tab que falló:\n%s", juntos)
			}
			// The first tab was built whole, which is what tells "it continued" from "there was nothing".
			if cli.llamadas["pane split"] == 0 || cli.llamadas["pane run"] == 0 {
				t.Error("el primer tab no se montó: el fallo del segundo paró el layout entero")
			}
		})
	}
}

func respondeNormal(args []string) ([]byte, []byte, error) {
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
func TestTabOfDevuelveVacioSinPaneYNoTumbaElLayout(t *testing.T) {
	ctx := context.Background()

	// Sin pane conocido: vacío sin preguntar a Herdr.
	vacia := nuevoCLIQueCuenta(nil)
	if got := vacia.client().tabOf(ctx, "w18", ""); got != "" {
		t.Errorf("tabOf sin pane dio %q, want vacío", got)
	}
	if vacia.llamadas["pane list"] != 0 {
		t.Error("tabOf sin pane preguntó a Herdr: no tenía con qué")
	}

	// Y si la lista falla, degrada a vacío SIN tumbar el layout.
	fallo := nuevoCLIQueCuenta(func(op string, _ []string) bool { return op == "pane list" })
	fallo.respond = func(args []string) ([]byte, []byte, error) {
		if args[0] == "pane" && args[1] == "list" {
			return nil, []byte("no such workspace"), fmt.Errorf("exit status 1")
		}
		return respondeNormal(args)
	}
	if got := fallo.client().tabOf(ctx, "w18", "w18:p1"); got != "" {
		t.Errorf("tabOf con la lista fallando dio %q, want vacío", got)
	}
	if _, err := fallo.client().MountLayout(ctx,
		Container{WorkspaceID: "w18", PaneID: "w18:p1"}, testPlan()); err != nil {
		t.Errorf("un tab sin nombre tumbó el layout: %v. Nombrar el tab es cosmético", err)
	}

	// Y el camino bueno: encuentra el pane y devuelve su tab.
	ok := nuevoCLIQueCuenta(nil)
	if got := ok.client().tabOf(ctx, "w18", "w18:p1"); got == "" {
		t.Error("tabOf no encontró un pane que sí está en la lista")
	}
}

// The last boundary with the shell that is left: the variable list.
func TestUnaVariableDeEntornoSinNombreNoSeMeteEnElComandoDelPane(t *testing.T) {
	buena := plan.Pane{
		Kind: plan.KindTuicr, Label: "TUICR", Cwd: "/wt",
		Argv: []string{"tuicr", "pr", "https://github.com/o/r/pull/7"},
		Env:  []string{"PRDASH_NUMBER=7", "TERM=xterm-256color"},
	}
	if got := paneCommand(buena); !strings.Contains(got, "PRDASH_NUMBER=7") ||
		!strings.Contains(got, "TERM=xterm-256color") {
		t.Errorf("las variables buenas no llegaron al comando: %q", got)
	}

	for _, malas := range [][]string{
		{"SIN_IGUAL"},
		{"=sin-nombre"},
		{"BUENA=1", "SIN_IGUAL", "=tampoco", "OTRA=2"},
	} {
		p := buena
		p.Env = malas
		got := paneCommand(p)
		for _, mala := range malas {
			if mala == "BUENA=1" || mala == "OTRA=2" {
				if !strings.Contains(got, mala) {
					t.Errorf("con %v se perdió la variable buena %q", malas, mala)
				}
				continue
			}
			if strings.Contains(got, mala) {
				t.Errorf("con %v, la entrada inválida %q llegó al comando: %q", malas, mala, got)
			}
		}
		if len(malas) > 2 && !strings.Contains(got, "BUENA=1") {
			t.Errorf("el filtro quitó también las buenas: %q", got)
		}
	}
}
