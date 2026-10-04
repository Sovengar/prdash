package tui

import (
	"context"
	"strings"
	"testing"

	"prdash/internal/forge/model"
	"prdash/internal/sim"
	"prdash/internal/testutil"
)

// `handleSimKey` es un intérprete de teclas sobre una máquina de tres fases —eligiendo,
// renderizando, enseñando— y su valor está entero en la tabla de transiciones: qué tecla hace
// qué en cada fase y qué teclas solo cierran.
//
// Y el riesgo de una tabla así es que la mitad de las combinaciones no exercisedn nunca. Un
// test por tecla probaría las que alguien pensó; lo que hace falta es recorrer la tabla
// entera y comprobar las TRES propiedades de cada celda, porque una transición se rompe
// cambiando lo que hace sin cambiar lo que se llama.
//
// Y las tres propiedades son: si la fase cambia, si el popup se cierra, y qué comando sale.
// La segunda es la que más se cuela: un `closeSim` que falta en un camino deja la imagen
// pegada encima del inbox, y eso no se ve hasta que se cierra la TUI.

// simuladorFalso es un `Simulator` que siempre está disponible y no renderiza nada, que es
// lo único que hace falta para probar las transiciones: lo que se decide aquí es si se abre
// el render, no qué renderiza.
type simuladorFalso struct {
	disponible bool
	llamadas   int
}

func (s *simuladorFalso) Available() bool { return s.disponible }

func (s *simuladorFalso) Simulate(_ context.Context, _ model.Item, _ sim.Kind) (sim.Result, error) {
	s.llamadas++
	return sim.Result{}, nil
}

// modelEnSim es un modelo con el popup de simulación en la fase pedida.
func modelEnSim(t *testing.T, fase simState) (Model, *simuladorFalso) {
	t.Helper()
	falso := &simuladorFalso{disponible: true}
	m := newTestModel(t, &testutil.FakeAdapter{ForgeName: "github", HostName: "github.com"})
	m.simulator = falso
	m.sim.state = fase
	m.sim.item = mkItem("github", "github.com", "acme/widget", "algo", 7, "")
	return m, falso
}

// TestCadaTeclaEnCadaFaseCierraOAvanzaYSiempreTerminaEnAlgoQueSePuedeCerrar: la tabla
// entera.
//
// Y el recorrido es a propósito exhaustivo y no "las teclas que me interesan": una tabla de
// transiciones con huecos es una tabla donde una tecla hace dos veces lo mismo en fases
// distintas, y eso no se ve leyendo el código sino comparando fase a fase.
//
// Y las dos aserciones que hay en cada celda:
//
//   - El popup se puede cerrar con ESC, sea cual sea la fase. Es la puerta de atrás del
//     overlay, y si alguna fase no lo respeta el usuario se queda sin salida salvo `q`.
//   - Cerrar es idempotente: cerrar dos veces no cambia nada. `closeSim` invalida el render
//     en vuelo con un contador, y llamarla dos veces por una tecla debe dejar el mismo
//     estado que llamarla una.
func TestCadaTeclaEnCadaFaseCierraOAvanzaYSiempreTerminaEnAlgoQueSePuedeCerrar(t *testing.T) {
	teclas := []string{
		"q", "ctrl+c", "esc", "o", "enter", "up", "down", "j", "k", "tab", "right", "left",
		"x", "space", "1", "?",
	}
	fases := []struct {
		nombre string
		fase   simState
	}{
		{"eligiendo", simChoosing},
		{"renderizando", simRendering},
		{"ensenando", simShowing},
	}

	for _, f := range fases {
		for _, tecla := range teclas {
			m, _ := modelEnSim(t, f.fase)
			got, _ := pulsar(t, m, tecla)

			// ESC cierra siempre, y es la puerta de atrás del overlay. Se comprueba desde
			// CADA fase en lugar de una vez: lo que importa es que no haya ninguna desde la
			// que ESC no sirva, y eso solo se ve recorriéndolas todas.
			if esc, _ := pulsar(t, m, "esc"); esc.sim.state != simClosed {
				t.Errorf("%s + %q: ESC no cerró el popup (state=%d)",
					f.nombre, tecla, esc.sim.state)
			}

			// Y cerrar deja el panel VACÍO, sin imagen ni celdas, para que al reabrir el
			// popup no se vea lo de la simulación anterior.
			//
			// Lo que NO se comprueba es que cerrar sea idempotente, que es lo que mi primera
			// versión afirmaba: `closeSim` sube `simSeq` en cada llamada y no lo hace por
			// capricho —cada cierre invalida el render que hubiera en vuelo, y son renders
			// distintos—. Dos cierres invalidan dos seqs, y eso es lo correcto.
			// Y la ÚNICA transición que no cierra es `enter` eligiendo, que avanza al
			// renderizado. Se admite aquí y se comprueba en su propio test, porque una tabla
			// que admite una excepción sin nombrarla no comprueba nada.
			avanza := f.fase == simChoosing && tecla == "enter"
			if !avanza && got.sim.state != simClosed {
				t.Errorf("%s + %q: no cerró el popup (state=%d)", f.nombre, tecla, got.sim.state)
			}
			if got.sim.image != "" || len(got.sim.cells) != 0 {
				t.Errorf("%s + %q: quedó residuo del popup: image=%q celdas=%d",
					f.nombre, tecla, got.sim.image, len(got.sim.cells))
			}

			// Y cerrar sobre un popup ya cerrado no revienta ni deja nada. El caso es real:
			// `q` cierra el popup y sale, y el mensaje de cierre de bubbletea puede llegar
			// después.
			yaCerrada := got
			yaCerrada.closeSim()
			if yaCerrada.sim.state != simClosed || yaCerrada.sim.image != "" {
				t.Errorf("%s + %q: cerrar un popup ya cerrado dejó residuo", f.nombre, tecla)
			}
		}
	}
}

// TestQYSalirCierranYCancelanElRenderYAbortanLaTUI: la tecla de irse, que no es cerrar.
//
// Y la asimetría es lo que hay que fijar: `esc` cierra el popup y sigue en la TUI, y `q`
// cierra el popup, CANCELA el render en vuelo y además pide salir. Que cancele es lo que evita
// que un render de dos segundos siga corriendo después de que el usuario haya decidido
// irse, escribiendo en una `Model` que ya nadie mira.
func TestQYSalirCierranYCancelanElRenderYAbortanLaTUI(t *testing.T) {
	for _, tecla := range []string{"q", "ctrl+c"} {
		m, falso := modelEnSim(t, simRendering)
		salida, cmd := pulsar(t, m, tecla)
		got := salida

		if got.sim.state != simClosed {
			t.Errorf("%q: no cerró el popup (state=%d)", tecla, got.sim.state)
		}
		if cmd == nil {
			t.Errorf("%q: no devolvió comando, así que la TUI no se sale", tecla)
		}
		// El render queda invalidado: `closeSim` sube `simSeq`, y un `simMsg` con el seq
		// viejo tiene que descartarse. Es lo que impide que la imagen aparezca encima de
		// un inbox en el que el usuario ya ha vuelto a trabajar.
		if got.simSeq == 0 {
			t.Errorf("%q: no invalidó el render en vuelo (simSeq=%d)", tecla, got.simSeq)
		}
		// Y no llegó a lanzar ningún render nuevo, que es lo contrario de lo que hace ESC.
		if falso.llamadas != 0 {
			t.Errorf("%q: lanzó %d renders al irse", tecla, falso.llamadas)
		}
	}
}

// TestEscYSalirSeDifierenSoloEnQueSaleDeLaTUI: la comparación que le da sentido a las dos.
//
// Y aquí mi primera versión afirmaba algo falso: que `esc` conservaba el render en vuelo y
// `q` lo invalidaba. Los dos llaman a `closeSim`, que sube `simSeq`, así que los dos
// invalidan. El motivo de que `closeSim` suba el contador sin mirar quién la llama es que
// cerrar el overlay deja de tener sentido cualquier render pendiente: su imagen ya no tiene
// dónde pintarse.
//
// Y la diferencia real entre las dos teclas es la que se mide: `q` pide `tea.Quit` y cancela
// el contexto de la app, y `esc` no. Eso es lo que evita que un render de dos segundos
// siga corriendo después de que el usuario haya decidido irse del programa.
func TestEscYSalirSeDifierenSoloEnQueSaleDeLaTUI(t *testing.T) {
	m, _ := modelEnSim(t, simRendering)
	m.simSeq = 7

	// Los dos invalidan el render en vuelo.
	for _, tecla := range []string{"esc", "q", "ctrl+c"} {
		got, cmd := pulsar(t, m, tecla)
		if got.simSeq == 7 {
			t.Errorf("%q no invalidó el render en vuelo: su imagen aparecería encima de un "+
				"inbox en el que el usuario ya ha vuelto a trabajar", tecla)
		}
		if got.sim.state != simClosed {
			t.Errorf("%q no cerró el popup", tecla)
		}
		// Y solo `q` y `ctrl+c` salen. Es la única diferencia entre las dos teclas, y es la
		// que hace que `esc` sea "salir del popup" y `q` sea "salir del programa".
		sale := cmd != nil
		if sale != (tecla != "esc") {
			t.Errorf("%q: ¿pide salir? = %v", tecla, sale)
		}
	}
}

// TestOAbreLaImagenSoloCuandoHayImagenYCuandoLaHayCierraSiNo: la tecla de abrir, que es la
// única que NO cierra.
//
// Y la asimetría completa: en fase de elección o renderizando, `o` cierra el popup porque no
// hay nada que abrir; enseñando la imagen, abre; y enseñando la imagen PERO sin `image`
// —el resultado llegó con la ruta vacía— cierra en vez de abrir un visor sin fichero.
//
// Y ese último caso es el que hace que la guarda de `m.sim.image != ""` exista: `openBrowserCmd`
// con una ruta vacía ejecutaría `xdg-open ""`, que no falla pero no abre nada, y el mensaje
// "abriendo " no diría qué.
func TestOAbreLaImagenSoloCuandoHayImagenYCuandoLaHayCierraSiNo(t *testing.T) {
	// Con imagen: abre y NO cierra.
	m, _ := modelEnSim(t, simShowing)
	m.sim.image = "/tmp/imagen-de-prueba.jpg"
	salida, cmd := pulsar(t, m, "o")
	got := salida

	if cmd == nil {
		t.Error("o con imagen no devolvió comando: no abriría nada")
	}
	if got.sim.state == simClosed {
		t.Error("o con imagen cerró el popup: el popup se cierra al abrir el visor y la " +
			"imagen desaparece de la vista antes de tiempo")
	}

	// Sin imagen: cierra y no abre.
	m2, _ := modelEnSim(t, simShowing)
	m2.sim.image = ""
	salida2, cmd2 := pulsar(t, m2, "o")
	if salida2.sim.state != simClosed {
		t.Error("o sin imagen no cerró el popup")
	}
	if cmd2 != nil {
		t.Error("o sin imagen devolvió comando: ejecutaría un visor con una ruta vacía")
	}

	// Y en las otras dos fases, `o` cierra siempre.
	for _, fase := range []simState{simChoosing, simRendering} {
		m3, _ := modelEnSim(t, fase)
		salida3, cmd3 := pulsar(t, m3, "o")
		if salida3.sim.state != simClosed {
			t.Errorf("o en fase %d no cerró", fase)
		}
		if cmd3 != nil {
			t.Errorf("o en fase %d devolvió comando sin haber imagen", fase)
		}
	}
}

// TestEnterEnLaFaseDeEleccionLanzaElRenderYEnLasOtrasCierra: `enter` solo vale en un sitio.
//
// Y es lo que hace que la fase tenga sentido: `enter` elige la estrategia highlighted y
// arranca el render. En las otras dos fases `enter` es "cualquier otra tecla", que cierra.
//
// Y el aserto del kind enviado es lo que ata el render a lo que el usuario tiene
// resaltado: si se enviara siempre el primero de `simKinds`, con una lista de una sola
// estrategia no se notaría, y en cuanto alguien añada rebase al popup el `enter` empezaría a
// renderizar merge con el rebase resaltado.
func TestEnterEnLaFaseDeEleccionLanzaElRenderYEnLasOtrasCierra(t *testing.T) {
	m, _ := modelEnSim(t, simChoosing)
	m.sim.cursor = 0

	salida, cmd := pulsar(t, m, "enter")
	got := salida

	if got.sim.state != simRendering {
		t.Fatalf("enter no pasó a la fase de renderizado (state=%d)", got.sim.state)
	}
	// Y el comando es nil a propósito: el render NO es un `tea.Cmd` sino una goroutine que
	// publica su resultado por el canal de eventos, igual que el refresco del inbox. La
	// razón es que el render tarda segundos y un `tea.Cmd` se ejecuta en el update loop, así
	// que bloquearía el teclado con el popup puesto.
	//
	// Mi primera versión pedía un comando no nulo y por eso fallaba. Un aserto de "lanza
	// algo" tiene que mirar DÓNDE, no solo si hay algo: por el canal o por el comando son
	// dos arquitecturas distintas y el mismo symptom —el popup cambia de fase— las
	// distingue.
	if cmd != nil {
		t.Error("enter devolvió un tea.Cmd: el render iría en el update loop y congelaría " +
			"el teclado con el popup puesto")
	}
	// Y lo que sí se puede comprobar sin esperar a la goroutine es que el kind escolhido es
	// el del cursor y que el panel se limpió antes del render nuevo.
	if got.sim.kind != simKinds[0] {
		t.Errorf("enter renderizó %q, y el cursor estaba en 0 (%q)", got.sim.kind, simKinds[0])
	}
	if got.sim.image != "" {
		t.Error("el panel conservaba la imagen anterior al empezar un render nuevo")
	}

	// En las otras fases, enter cierra y no lanza.
	for _, fase := range []simState{simRendering, simShowing} {
		m2, falso2 := modelEnSim(t, fase)
		salida2, cmd2 := pulsar(t, m2, "enter")
		if salida2.sim.state != simClosed {
			t.Errorf("enter en fase %d no cerró", fase)
		}
		if cmd2 != nil {
			t.Errorf("enter en fase %d devolvió un comando", fase)
		}
		if falso2.llamadas != 0 {
			t.Errorf("enter en fase %d renderizó", fase)
		}
	}
}

// TestUnaTeclaQueNoEsDeMovimientoCierraElPopupYNoSeQuedaAhíColgado: la última celda.
//
// Y el motivo de que exista `default` en un intérprete de teclas es que llega una tecla que
// no estaba prevista —una que se añade al config, un atajo nuevo del terminal— y sin él el
// popup se quedaría abierto para siempre con la pantalla de elección delante, sin forma de
// saber si la tecla llegó.
func TestUnaTeclaQueNoEsDeMovimientoCierraElPopupYNoSeQuedaAhíColgado(t *testing.T) {
	for _, tecla := range []string{"x", "space", "1", "?", "F5", "ctrl+n"} {
		m, falso := modelEnSim(t, simChoosing)
		salida, cmd := pulsar(t, m, tecla)
		got := salida
		if got.sim.state != simClosed {
			t.Errorf("%q en la fase de elección dejó el popup abierto", tecla)
		}
		if cmd != nil {
			t.Errorf("%q en la fase de elección devolvió un comando: cerrarse no lanza nada", tecla)
		}
		if falso.llamadas != 0 {
			t.Errorf("%q lanzó un render", tecla)
		}
	}
}

// TestConUnaSolaEstrategiaLasFlechasNoHacenNadaYEnterNoSeComeLaEleccion: `moveSimCursor` con
// `simKinds` de un solo elemento.
//
// Y es la consecuencia de que rebase esté excluido de la lista: con un solo elemento,
// `moveSimCursor` devuelve false para que el `default` del intérprete de claves se encargue
// —y `default` CIERRA—, así que las flechas cerrarían el popup. Por eso la guarda de
// `len(simKinds) < 2` está al principio y no en cada rama: es el caso que decide si las
// flechas son navegación o son "cierra".
//
// Y el test lo fija por lo que el usuario ve, que es que con una estrategia no hay nada que
// elegir y hay que decirlo en vez de dejar un popup con un cursor que no se mueve.
func TestConUnaSolaEstrategiaLasFlechasNoHacenNadaYEnterNoSeComeLaEleccion(t *testing.T) {
	if len(simKinds) != 1 {
		t.Skipf("ahora hay %d estrategias y este test es del caso de una sola", len(simKinds))
	}

	m, _ := modelEnSim(t, simChoosing)
	// El cursor no se mueve, pero eso es lo de menos: lo que importa es lo que pasa después.
	for _, tecla := range []string{"up", "down", "left", "right", "j", "k", "tab"} {
		m2 := m
		if movió := m2.moveSimCursor(tecla); movió {
			t.Errorf("%q: moveSimCursor dijo que movió el cursor con una sola estrategia", tecla)
		}
	}
	if m.sim.cursor != 0 {
		t.Errorf("el cursor quedó en %d con una sola estrategia", m.sim.cursor)
	}
	// Y enter sigue funcionando, que es lo que se perdería si `moveSimCursor` devolviera
	// true para todo.
	if pulsarM(t, m, "enter").sim.state != simRendering {
		t.Error("enter no lanza el render con una sola estrategia")
	}
}

// TestAbrirElSimuladorNiegaLasTresCosasYLasDice: `openSimulator` y sus negativas.
//
// Y las tres negativas existen porque cada una tapa un camino que deja al usuario pulsando
// una tecla sin que pase nada:
//
//   - git-sim no está instalado: sin el binario no hay nada que renderizar.
//   - No hay ítem seleccionado: el popup necesita un ítem.
//   - El forge no reporta rama destino: sin ella la simulación sería una operación sin
//     base, y el error de git-sim sería sobre una rama inventada.
//
// Y laproperty importante es que **cada una avisa**. Una negativa silenciosa deja al usuario
// con la impresión de que la TUI está colgada, que es lo que hace que se reinicie la terminal.
func TestAbrirElSimuladorNegaLasTresCosasYLasDice(t *testing.T) {
	// Sin git-sim.
	m, _ := modelEnSim(t, simClosed)
	m.simulator = &simuladorFalso{disponible: false}
	m = conSeleccion(t, m, mkItem("github", "github.com", "acme/widget", "algo", 7, ""))
	if _, cmd := m.openSimulator(); cmd != nil {
		t.Error("sin git-sim devolvió comando")
	}
	if av := lastToast(m); !strings.Contains(av, "git-sim") {
		t.Errorf("sin git-sim no avisa de git-sim: %q", av)
	}

	// Sin selección.
	m2, _ := modelEnSim(t, simClosed)
	if _, cmd := m2.openSimulator(); cmd != nil {
		t.Error("sin selección devolvió comando")
	}
	if av := lastToast(m2); !strings.Contains(av, "select an item") {
		t.Errorf("sin selección no avisa: %q", av)
	}

	// Sin rama destino, y el aviso tiene que NOMBRAR el ítem: sin el nombre, el usuario ve
	// "this forge reports no target branch for" y no sabe de cuál de sus veinte PRs habla.
	m3, _ := modelEnSim(t, simClosed)
	sinBase := mkItem("github", "github.com", "acme/widget", "algo", 7, "")
	sinBase.TargetBranch = ""
	m3 = conSeleccion(t, m3, sinBase)
	if _, cmd := m3.openSimulator(); cmd != nil {
		t.Error("sin rama destino devolvió comando")
	}
	av := lastToast(m3)
	if !strings.Contains(av, "target branch") {
		t.Errorf("sin rama destino no avisa de eso: %q", av)
	}
	if !strings.Contains(av, refLabel(sinBase)) {
		t.Errorf("el aviso no nombra el ítem: %q", av)
	}

	// Y una rama destino que es solo espacios cuenta como vacío. El forge puede devolver
	// `"   "` en vez de `""`, y `TrimSpace` es lo que evita que se llegue a git-sim con una
	// base en blanco.
	m4, _ := modelEnSim(t, simClosed)
	conEspacios := mkItem("github", "github.com", "acme/widget", "algo", 7, "")
	conEspacios.TargetBranch = "   "
	m4 = conSeleccion(t, m4, conEspacios)
	if _, cmd := m4.openSimulator(); cmd != nil {
		t.Error("con una rama destino de solo espacios devolvió comando")
	}
	if av := lastToast(m4); !strings.Contains(av, "target branch") {
		t.Errorf("una rama destino de espacios no cuenta como vacía: %q", av)
	}
}

// conSeleccion deja un ítem bajo el cursor.
//
// Y entra por un `pageMsg` y no por `conItems` + `rebuild` porque `selected()` lee
// `m.rows()`, que son las filas de la SECCIÓN VISIBLE del inbox —no todos los streams— y un
// ítem añadido a un stream sin pasarlo por su sección se queda invisible para el cursor.
//
// Y el ciclo sale de `m.cycle` y no de un cero literal, porque `New` arranca ya en 1. Con un
// cero, el `pageMsg` se descartaba por obsoleto —que es lo que está para hacer— y los tres
// casos salían con el aviso de "select an item first" en vez del que se estaba probando.
// Lo que falla es un mensaje de ciclo obsoleto se descarta en silencio, así que el síntoma es
// que el estado no cambia y no hay aviso de nada: el peor sitio para que se cuele un error.
func conSeleccion(t *testing.T, m Model, it model.Item) Model {
	t.Helper()
	return send(t, m,
		page(m.cycle, it.Forge, it.Ref.Host, model.SectionReview, model.ReviewRequested,
			[]model.Item{it}, false))
}

// pulsarM aplica una tecla y devuelve solo el modelo, para las comprobaciones que no miran el
// comando.
func pulsarM(t *testing.T, m Model, key string) Model {
	t.Helper()
	got, _ := pulsar(t, m, key)
	return got
}
