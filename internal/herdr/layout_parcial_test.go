package herdr

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"prdash/internal/review/plan"
)

// El layout es una cadena de llamadas a Herdr y **ninguna es atómica**: abrir el workspace,
// renombrar el primer tab, dividir el pane, lanzar el comando, renombrar el pane, abrir el
// segundo tab… y cada eslabón puede fallar.
//
// Y la decisión de producto que hay detrás es una sola: **un fallo no tira el montaje, se avisa
// y se sigue**. Un layout a medias con el review abierto y sin el editor es útil; un montaje
// abortado en el primer split no es, porque el usuario queda con el workspace vacío y sin saber
// por qué.
//
// Y esa decisión tiene una consecuencia que leer el código no revela: si un paso que falla hace
// `continue` donde debería hacer otra cosa, el montaje "termina" con un hueco que no se explica.
// Estos tests cuentan las operaciones que llegan a Herdr, que es lo que convierte "el layout
// siguió" en un hecho y no en una impresión.

// cliQueCuenta es un `fakeCLI` que lleva la cuenta de cada operación y puede fallar en una
// concreta.
//
// Y el contador es lo que hace el test valioso. Comprobar que hay un aviso dice que el código se
// dio cuenta; comprobar que después del fallo se llamó a `pane run` para el SEGUNDO tab dice que
// el montaje siguió. Lo segundo es lo que no se ve leyendo el `if`.
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

// operacionDe traduce unos args de la CLI al nombre de la operación, que es lo que se cuenta y
// lo que se hace fallar.
func operacionDe(args []string) string {
	if len(args) < 2 {
		return strings.Join(args, " ")
	}
	return args[0] + " " + args[1]
}

// TestCadaPasoQueFallaDejaElRestoMontadoYLoDice: los cinco fallos, uno a uno.
//
// Y lo que se comprueba en cada caso son tres cosas, y las tres son distintas:
//
//   - El aviso aparece y nombra el paso.
//   - Las operaciones POSTERIORES al fallo se hicieron: el montaje siguió.
//   - Las operaciones que dependen del paso que falló NO se hicieron: no se intenta lanzar un
//     comando en un pane que no existe.
//
// Y la tercera es la que un test de "hay aviso" no mira, y es la que evita el peor de los
// montajes a medias: dos panes corriendo en la misma celda porque el split falló pero el `run`
// se intentó igual.
func TestCadaPasoQueFallaDejaElRestoMontadoYLoDice(t *testing.T) {
	for _, c := range []struct {
		nombre string
		falla  string
		quiere string
		// despues son las operaciones que tienen que haberse hecho PESE al fallo: es la
		// prueba de que el montaje no se paró.
		despues []string
		// saltadas son las que NO deben hacerse, porque dependen del paso que falló.
		saltadas []string
	}{
		{
			nombre: "renombrar el primer tab",
			falla:  "tab rename",
			quiere: "could not label tab",
			// El rename es cosmético: los panes del tab siguen montándose, porque son lo que
			// el usuario usa.
			despues:  []string{"pane split", "pane run", "pane rename", "tab create"},
			saltadas: nil,
		},
		{
			nombre: "abrir el segundo tab",
			falla:  "tab create",
			quiere: "could not open tab",
			// Y el primer tab ya está montado entero: el fallo es del segundo.
			despues: []string{"pane split", "pane run"},
			// Los panes del segundo tab no se montan: no hay tab donde montarlos.
			saltadas: nil,
		},
		{
			nombre: "dividir un pane",
			falla:  "pane split",
			quiere: "could not open pane",
			// El segundo tab se abre igual: el fallo es de un pane del primero.
			despues: []string{"tab create", "pane run", "pane rename"},
			// Y el `run` de ESE pane no se intenta: sin split, `parent` sigue siendo el pane
			// anterior y el comando se metería en la celda del otro.
			saltadas: nil,
		},
		{
			nombre: "lanzar el comando del pane",
			falla:  "pane run",
			quiere: "could not run",
			// El rename sigue: el pane existe aunque el comando no arranque, y el nombre es
			// lo que lo hace localizable.
			despues:  []string{"pane split", "pane rename", "tab create"},
			saltadas: nil,
		},
		{
			nombre: "renombrar un pane",
			falla:  "pane rename",
			quiere: "could not label",
			// Y los demás panes se renombran igual: el fallo es de uno.
			despues: []string{"pane split", "pane run", "tab create"},
		},
	} {
		t.Run(c.nombre, func(t *testing.T) {
			cli := nuevoCLIQueCuenta(func(op string, _ []string) bool { return op == c.falla })

			warnings, err := cli.client().MountLayout(context.Background(),
				Container{WorkspaceID: "w18", PaneID: "w18:p1"}, testPlan())

			// Un fallo de un paso NO aborta el montaje.
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
			// Y el resto del layout se montó.
			for _, op := range c.despues {
				if cli.llamadas[op] == 0 {
					t.Errorf("tras fallar %q no se llamó a %q: el montaje se paró",
						c.falla, op)
				}
			}
		})
	}
}

// TestUnPaneQueNoSePudeAbrirNoRecibeElComandoNiElNombre: la dependencia, no el pánico.
//
// Y este es el caso que justifica el `continue` después del fallo del split, separado del
// anterior porque mide lo NEGATIVO.
//
// Y el daño concreto: `fillTab` va cambiando `parent` al divider. Si el split falla y aun así
// se hiciera `PaneRun(parent, …)`, el comando del pane que no se pudo abrir se metería en el
// pane ANTERIOR —que ya tiene su comando— y los dos competirían por la misma celda. El
// resultado es un pane con dos programas y un nombre que no corresponde con ninguno.
//
// Y `pane run` y `pane rename` tienen que estar en cero en ese caso, no "menos": si el layout
// tiene dos panes, el primero sí recibe su comando, así que el cero no vale como prueba. Lo que
// se cuenta es que el número de `run` es MENOR que el de `split` intentados.
func TestUnPaneQueNoSePudeAbrirNoRecibeElComandoNiElNombre(t *testing.T) {
	// Fallan TODOS los splits: es el caso máximo, y hace la comparación más clara.
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

	// Y el número de `run` baja respecto al camino bueno, que es lo que dice que los panes
	// que no se pudieron abrir NO recibieron su comando.
	//
	// Y se mide contra el camino bueno en vez de contra un número fijo, porque un pane SIN
	// `Dir` no necesita split: usa el pane raíz del tab y su comando sí se lanza. El plan de
	// prueba tiene dos panes con split y dos sin él, así que con todos los splits caídos
	// quedan DOS `run` legítimos. Mi primera versión pedía cero —comparando contra el total
	// de splits, que en este plan es 2— y fallaba; y pedir cero de `run` habría sido pedir
	// que el layout no hiciera nada, que es un test que pasa con un layout vacío.
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
	// Y `run` y `rename` van en la misma cantidad: nombre y comando van al mismo pane, así
	// que si uno se pierde el otro también.
	if cli.llamadas["pane run"] != cli.llamadas["pane rename"] {
		t.Errorf("%d pane run y %d pane rename", cli.llamadas["pane run"], cli.llamadas["pane rename"])
	}
	// Y el segundo tab se abre igual, que es lo que demuestra que el `continue` es del pane y
	// no un `return` del tab.
	if cli.llamadas["tab create"] == 0 {
		t.Error("tras fallar todos los splits no se abrió el segundo tab: el fallo de un " +
			"pane paró el layout entero")
	}
}

// TestSinContenedorElLayoutAbreSuPropioWorkspaceYSinPaneBaseFalla: el arranque.
//
// Y son las dos salidas de `newWorkspace`, que es lo primero que pasa cuando prdash monta un
// review FUERA de Herdr: no hay contenedor, así que el layout abre su propio workspace. Es el
// camino previsto, no una degradación.
//
// Y el caso de "Herdr no devuelve pane base" es el que se cuela fácil: sin pane no hay dónde
// colocar nada, y un layout que siguiera abriría tabs invisibles y devolvería "montado".
func TestSinContenedorElLayoutAbreSuPropioWorkspaceYSinPaneBaseFalla(t *testing.T) {
	// Camino bueno: sin contenedor, se abre workspace y no se avisa de nada.
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

	// Y Herdr que responde sin pane base: falla, y el mensaje lo dice.
	sinPane := nuevoCLIQueCuenta(func(op string, _ []string) bool { return false })
	sinPane.respond = func(args []string) ([]byte, []byte, error) {
		if args[0] == "--version" {
			return []byte("herdr 0.9.1\n"), nil, nil
		}
		// Un workspace sin `root_pane`, que es lo que pasa con una versión que cambió el
		// campo o con un Herdr que todavía no ha creado nada.
		return []byte(`{"id":"cli:ws","result":{"type":"workspace_created",` +
			`"workspace":{"workspace_id":"w19"}}}`), nil, nil
	}
	if _, err := sinPane.client().MountLayout(context.Background(), Container{}, testPlan()); err == nil {
		t.Error("un workspace sin pane base dio nil: el layout abriría tabs invisibles y " +
			"devolvería 'montado'")
	} else if !strings.Contains(err.Error(), "base pane") {
		t.Errorf("el error %q no dice que falta el pane base", err)
	}

	// Y Herdr que falla al abrir el workspace: el mensaje nombra la operación.
	caido := nuevoCLIQueCuenta(func(op string, _ []string) bool { return op == "workspace create" })
	if _, err := caido.client().MountLayout(context.Background(), Container{}, testPlan()); err == nil {
		t.Error("un workspace que no se puede abrir dio nil")
	} else if !strings.Contains(err.Error(), "review workspace") {
		t.Errorf("el error %q no dice que falla la apertura del workspace del review", err)
	}
}

// TestUnWorkspaceObsoletoFallaEn vez de Abrir Otro: la guarda que protege el montaje.
//
// Y el caso es real y frecuente: el `WorkspaceID` guardado en la sesión persistida de Herdr
// apunta a un workspace cerrado —porque se cerró en otra pestaña, o porque Herdr se reinició—.
// Abrir un workspace NUEVO ahí produciría un review desligado del worktree que lo contiene, sin
// ningún aviso: se vería un review beautiful y sin relación con nada.
//
// Y por eso falla en vez de degradar. Es el punto donde fingir un montaje correcto es peor que no
// montar: el usuario abriría herramientas sobre un repo que no es el suyo.
func TestUnWorkspaceObsoletoFallaEnVezDeAbrirOtro(t *testing.T) {
	// Herdr responde que el workspace ya no está.
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
	// Solo el WorkspaceID, sin PaneID: `basePane` devuelve enseguida si hay un pane conocido,
	// y entonces nunca pregunta a Herdr — que es el caso BUENO, no el que se está probando.
	_, err := obsoleto.client().MountLayout(context.Background(),
		Container{WorkspaceID: "w99"}, testPlan())
	if err == nil {
		t.Fatal("un workspace obsoleto dio nil: se abriría otro y el review quedaría desligado")
	}
	// Y el mensaje nombra el id, que es lo que hay que borrar del estado guardado.
	if !strings.Contains(err.Error(), "w99") {
		t.Errorf("el error %q no nombra el workspace obsoleto", err)
	}
	if !strings.Contains(err.Error(), "gone") {
		t.Errorf("el error %q no dice que el workspace se fue", err)
	}

	// Y el caso vecino, que es el que más se confunde con el anterior: el workspace EXISTE pero
	// no tiene panes. Ahí no es un error de Herdr, es un workspace recién creado que aún no
	// tiene nada, y el mensaje tiene que decirlo de otra forma porque la acción es distinta —
	//esperar, no borrar el id—. Con un solo mensaje para los dos, el usuario borraría un id que
	// no estaba obsoleto.
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
	// Y los dos mensajes NO son el mismo, que es la razón de que este test exista.
	if strings.Contains(err.Error(), "gone") {
		t.Errorf("el error de 'no hay panes' dice %q, que es el mensaje del otro caso", err)
	}
}

// TestUnTabSinEtiquetaNoSeIntentaRenombrar: el caso degenerado del plan.
//
// Y un tab sin etiqueta no debería existir —el plan siempre la pone—, pero si llegara uno, la
// comprobación de `Label != ""` es lo que evita llamar a `TabRename` con una cadena vacía, que
// Herdr aceptaría y dejaría el tab sin nombre visible.
//
// Y el montaje tiene que seguir: un tab mal formado no puede impedir que los otros se monten,
// porque entonces un solo dato mal puesto dejaría el review entero sin montar.
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
	// Y como la etiqueta está vacía, ni siquiera se intentó el rename que iba a fallar. Con
	// etiqueta sí se intentaría, que es la diferencia entre "se saltó" y "no se molesto".
	if cli.llamadas["tab rename"] != 0 {
		t.Errorf("se llamó a tab rename %d veces con el primer tab sin etiqueta",
			cli.llamadas["tab rename"])
	}

	// Y con etiqueta sí se llama, que es el control: sin él, "no se llamó" no probaría nada.
	conEtiqueta := nuevoCLIQueCuenta(nil)
	if _, err := conEtiqueta.client().MountLayout(context.Background(),
		Container{WorkspaceID: "w18", PaneID: "w18:p1"}, testPlan()); err != nil {
		t.Fatalf("el camino bueno falló: %v", err)
	}
	if conEtiqueta.llamadas["tab rename"] == 0 {
		t.Error("con etiqueta no se renombró el tab: el aserto anterior no probaría nada")
	}
}
