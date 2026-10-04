package tui

import (
	"context"
	"strings"
	"testing"

	"prdash/internal/forge/model"
	"prdash/internal/sim"
	"prdash/internal/testutil"
)

type simuladorFalso struct {
	disponible bool
	llamadas   int
}

func (s *simuladorFalso) Available() bool { return s.disponible }

func (s *simuladorFalso) Simulate(_ context.Context, _ model.Item, _ sim.Kind) (sim.Result, error) {
	s.llamadas++
	return sim.Result{}, nil
}

func modelEnSim(t *testing.T, fase simState) (Model, *simuladorFalso) {
	t.Helper()
	falso := &simuladorFalso{disponible: true}
	m := newTestModel(t, &testutil.FakeAdapter{ForgeName: "github", HostName: "github.com"})
	m.simulator = falso
	m.sim.state = fase
	m.sim.item = mkItem("github", "github.com", "acme/widget", "algo", 7, "")
	return m, falso
}

// The sweep is deliberately exhaustive and not "the keys I care about": a transition table with a
// hole in it is a keypress that does nothing, with no error.
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

			if esc, _ := pulsar(t, m, "esc"); esc.sim.state != simClosed {
				t.Errorf("%s + %q: ESC no cerró el popup (state=%d)",
					f.nombre, tecla, esc.sim.state)
			}

			// Closing leaves the panel EMPTY, with no image and no cells, so reopening does not show the previous
			//simulation.
			avanza := f.fase == simChoosing && tecla == "enter"
			if !avanza && got.sim.state != simClosed {
				t.Errorf("%s + %q: no cerró el popup (state=%d)", f.nombre, tecla, got.sim.state)
			}
			if got.sim.image != "" || len(got.sim.cells) != 0 {
				t.Errorf("%s + %q: quedó residuo del popup: image=%q celdas=%d",
					f.nombre, tecla, got.sim.image, len(got.sim.cells))
			}

			yaCerrada := got
			yaCerrada.closeSim()
			if yaCerrada.sim.state != simClosed || yaCerrada.sim.image != "" {
				t.Errorf("%s + %q: cerrar un popup ya cerrado dejó residuo", f.nombre, tecla)
			}
		}
	}
}

// The asymmetry is the point: esc closes the popup and stays in the TUI, q closes it and leaves.
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
		if got.simSeq == 0 {
			t.Errorf("%q: no invalidó el render en vuelo (simSeq=%d)", tecla, got.simSeq)
		}
		if falso.llamadas != 0 {
			t.Errorf("%q: lanzó %d renders al irse", tecla, falso.llamadas)
		}
	}
}

// My first version asserted something false: that esc keeps the render in flight and q invalidates it.
// Both invalidate it.
func TestEscYSalirSeDifierenSoloEnQueSaleDeLaTUI(t *testing.T) {
	m, _ := modelEnSim(t, simRendering)
	m.simSeq = 7

	for _, tecla := range []string{"esc", "q", "ctrl+c"} {
		got, cmd := pulsar(t, m, tecla)
		if got.simSeq == 7 {
			t.Errorf("%q no invalidó el render en vuelo: su imagen aparecería encima de un "+
				"inbox en el que el usuario ya ha vuelto a trabajar", tecla)
		}
		if got.sim.state != simClosed {
			t.Errorf("%q no cerró el popup", tecla)
		}
		sale := cmd != nil
		if sale != (tecla != "esc") {
			t.Errorf("%q: ¿pide salir? = %v", tecla, sale)
		}
	}
}

// o is the only key that does NOT close. While choosing or rendering, o closes; with an image it
// opens it.
func TestOAbreLaImagenSoloCuandoHayImagenYCuandoLaHayCierraSiNo(t *testing.T) {
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

	m2, _ := modelEnSim(t, simShowing)
	m2.sim.image = ""
	salida2, cmd2 := pulsar(t, m2, "o")
	if salida2.sim.state != simClosed {
		t.Error("o sin imagen no cerró el popup")
	}
	if cmd2 != nil {
		t.Error("o sin imagen devolvió comando: ejecutaría un visor con una ruta vacía")
	}

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

func TestEnterEnLaFaseDeEleccionLanzaElRenderYEnLasOtrasCierra(t *testing.T) {
	m, _ := modelEnSim(t, simChoosing)
	m.sim.cursor = 0

	salida, cmd := pulsar(t, m, "enter")
	got := salida

	if got.sim.state != simRendering {
		t.Fatalf("enter no pasó a la fase de renderizado (state=%d)", got.sim.state)
	}
	// The command is nil on purpose: the render is a goroutine publishing on the events channel, not a
	//tea.Cmd, because it takes seconds.
	if cmd != nil {
		t.Error("enter devolvió un tea.Cmd: el render iría en el update loop y congelaría " +
			"el teclado con el popup puesto")
	}
	if got.sim.kind != simKinds[0] {
		t.Errorf("enter renderizó %q, y el cursor estaba en 0 (%q)", got.sim.kind, simKinds[0])
	}
	if got.sim.image != "" {
		t.Error("el panel conservaba la imagen anterior al empezar un render nuevo")
	}

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

// The `default` exists because a key arrives that was not foreseen, and a keypress that does
// nothing leaves the popup stuck.
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

// This is a consequence of rebase being excluded: with a single strategy there is no menu, so the
// arrows move nothing and enter must not swallow the choice.
func TestConUnaSolaEstrategiaLasFlechasNoHacenNadaYEnterNoSeComeLaEleccion(t *testing.T) {
	if len(simKinds) != 1 {
		t.Skipf("ahora hay %d estrategias y este test es del caso de una sola", len(simKinds))
	}

	m, _ := modelEnSim(t, simChoosing)
	for _, tecla := range []string{"up", "down", "left", "right", "j", "k", "tab"} {
		m2 := m
		if movió := m2.moveSimCursor(tecla); movió {
			t.Errorf("%q: moveSimCursor dijo que movió el cursor con una sola estrategia", tecla)
		}
	}
	if m.sim.cursor != 0 {
		t.Errorf("el cursor quedó en %d con una sola estrategia", m.sim.cursor)
	}
	if pulsarM(t, m, "enter").sim.state != simRendering {
		t.Error("enter no lanza el render con una sola estrategia")
	}
}

func TestAbrirElSimuladorNegaLasTresCosasYLasDice(t *testing.T) {
	m, _ := modelEnSim(t, simClosed)
	m.simulator = &simuladorFalso{disponible: false}
	m = conSeleccion(t, m, mkItem("github", "github.com", "acme/widget", "algo", 7, ""))
	if _, cmd := m.openSimulator(); cmd != nil {
		t.Error("sin git-sim devolvió comando")
	}
	if av := lastToast(m); !strings.Contains(av, "git-sim") {
		t.Errorf("sin git-sim no avisa de git-sim: %q", av)
	}

	m2, _ := modelEnSim(t, simClosed)
	if _, cmd := m2.openSimulator(); cmd != nil {
		t.Error("sin selección devolvió comando")
	}
	if av := lastToast(m2); !strings.Contains(av, "select an item") {
		t.Errorf("sin selección no avisa: %q", av)
	}

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

// It enters through a pageMsg rather than conItems+rebuild, because selected() reads m.rows(), the
// rows of the VISIBLE inbox section.
func conSeleccion(t *testing.T, m Model, it model.Item) Model {
	t.Helper()
	return send(t, m,
		page(m.cycle, it.Forge, it.Ref.Host, model.SectionReview, model.ReviewRequested,
			[]model.Item{it}, false))
}

func pulsarM(t *testing.T, m Model, key string) Model {
	t.Helper()
	got, _ := pulsar(t, m, key)
	return got
}
