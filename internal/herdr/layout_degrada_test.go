package herdr

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"prdash/internal/review/plan"
)

// Las ramas que quedan en el layout y en el runner, y que comparten una propiedad: son fallos
// que el código DEGRADA en vez de propagar. Y degradar tiene un precio —un popup a medias, un
// tab sin nombre— que hay que decidir consciously, y eso es lo que se fija aquí.

// TestUnTabQueNoSeAbreNoTiraElRestoYLoDicePorTab: el `continue` del segundo tab.
//
// Y son dos salidas distintas del mismo `if err != nil`, y ambas son avisos y no abortes:
//
//   - Herdr falla al abrir el tab: el aviso trae el error.
//   - Herdr responde sin pane raíz: el aviso lo dice SIN error, porque no hay ninguno —Herdr
//     salió bien— y un aviso con un error vacío parece un bug.
//
// Y lo que se comprueba en las dos es que el resto del layout se montó. Con un solo tab en el
// plan no se puede distinguir "continuó" de "no había nada más", así que el plan tiene dos.
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
					// Sin `root_pane`, que es lo que devuelve una versión que cambió el campo o
					// un Herdr que aún no ha creado nada.
					return []byte(`{"id":"cli:tab:create","result":{"type":"tab_created",` +
						`"tab":{"tab_id":"w18:t1"}}}`), nil, nil
				}
				return respondeNormal(args)
			},
			quiere: "no root pane",
		},
	} {
		t.Run(c.nombre, func(t *testing.T) {
			// El `respond` por defecto cuenta las operaciones, y sustituirlo se lleva el
			// contador con él —mi primera versión lo hacía y la comprobación de "el primer tab
			// se montó" leía un mapa vacío—. Así que el override cuenta también.
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
			// Y el aviso NOMBRA el tab, que es lo que permite saber cuál de los dos falló.
			if !strings.Contains(juntos, "Edit") {
				t.Errorf("el aviso no nombra el tab que falló:\n%s", juntos)
			}
			// Y el primer tab se montó entero: es lo que distingue "siguió" de "no había nada".
			if cli.llamadas["pane split"] == 0 || cli.llamadas["pane run"] == 0 {
				t.Error("el primer tab no se montó: el fallo del segundo paró el layout entero")
			}
		})
	}
}

// respondeNormal es el `respond` que usa `nuevoCLIQueCuenta`, separado para poder reusarlo desde
// los casos que sustituyen una sola operación.
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

// TestTabOfDevuelveVacioSinPaneYNoTumbaElLayout: la degradación de `tabOf`.
//
// Y `tabOf` busca en qué tab está un pane porque Herdr no tiene "el tab de este pane", y por eso
// degrada a cadena vacía en tres casos. Y el tercero es el que importa: si la LISTA de panes
// falla, `tabOf` devuelve "" y el layout sigue, porque nombrar el tab es COSMÉTICO.
//
// Y degradar ahí es lo correcto: el tab queda sin su etiqueta y el popup sigue montándose. Lo
// contrario —abortar— dejaría al usuario con el workspace vacío por un nombre que no aparece.
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

// TestUnaVariableDeEntornoSinNombreNoSeMeteEnElComandoDelPane: el filtro de `Env`.
//
// Y es la última frontera con el shell que queda: la lista de variables del pane se pasa a un
// comando, y una entrada sin `=` —un argumento suelto, o un `PATH` vacío— se convertiría en
// texto que el shell lee.
//
// Y el filtro tiene dos mitades: sin `=` no es una variable, y con el NOMBRE vacío tampoco lo
// es aunque traiga `=`. Las dos son entradas que un config mal escrito puede producir, y las dos
// tienen que quedar fuera sin tocar el resto.
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
		// Y la que sí era válida entre las malas llega: el filtro quita las malas y no todas.
		if len(malas) > 2 && !strings.Contains(got, "BUENA=1") {
			t.Errorf("el filtro quitó también las buenas: %q", got)
		}
	}
}
