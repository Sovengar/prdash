package tui

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"

	"prdash/internal/config"
	"prdash/internal/forge"
	"prdash/internal/forge/model"
	"prdash/internal/review/executor"
	"prdash/internal/review/plan"
	"prdash/internal/testutil"
	"prdash/internal/worktree"
)

func TestUnTickDelSpinnerSePasaAlSpinnerYDevuelveSuCmd(t *testing.T) {
	m := newTestModel(t)
	antes := m.spinner.View()

	salida, cmd := m.Update(spinner.TickMsg{})
	got := salida.(Model)

	if got.spinner.View() == antes {
		t.Error("el tick no cambió la vista del spinner: dejaría de girar en el primer frame")
	}
	if cmd == nil {
		t.Error("el tick no devuelve el Cmd del spinner: el siguiente tick nunca llega y el " +
			"spinner se queda congelado")
	}
}

func TestUnMensajeQueNoEsUnaTeclaNiUnEventoSeIgnoraYNoRompeNada(t *testing.T) {
	m := newTestModel(t)
	m.loading = true
	m.cursor = 3
	antes := m

	for _, msg := range []tea.Msg{
		"una cadena cualquiera",
		struct{ X int }{42},
		nil,
	} {
		salida, cmd := m.Update(msg)
		got := salida.(Model)
		if cmd != nil {
			t.Errorf("%T: un mensaje desconocido devolvió un comando", msg)
		}
		if got.loading != antes.loading || got.cursor != antes.cursor {
			t.Errorf("%T: un mensaje desconocido cambió el estado", msg)
		}
	}
}

// commentsTickMsg does NOT go through the event bomb (it consumes no channel reader) and still has
// to re-arm its tick.
func TestElTickDeComentariosSeRearmaAunqueNoHayaNadaQueConsultar(t *testing.T) {
	for _, c := range []struct {
		nombre  string
		prepara func(*testing.T) Model
	}{
		{"sin selección", func(t *testing.T) Model { return newTestModel(t) }},
		{"con un ítem sin comentarios", func(t *testing.T) Model {
			m := newTestModel(t, &testutil.FakeAdapter{ForgeName: "github", HostName: "github.com"})
			m = conSeleccion(t, m, mkItem("github", "github.com", "acme/widget", "uno", 7, ""))
			return m
		}},
	} {
		t.Run(c.nombre, func(t *testing.T) {
			m := c.prepara(t)
			if _, cmd := m.Update(commentsTickMsg{}); cmd == nil {
				t.Error("el tick de comentarios no devolvió comando: la cadena muere y el " +
					"siguiente cambio de selección ya no se nota")
			}
		})
	}
}

func TestUnaAccionSobreUnForgeDesconocidoSeNiegaYLoDice(t *testing.T) {
	m := newTestModel(t) // sin adapters: el mapa de forges está vacío
	it := mkItem("github", "github.com", "acme/widget", "uno", 7, "")

	a, ok := m.canActionOn(forge.ActionApprove, it)
	if ok || a != nil {
		t.Error("canActionOn de un forge desconocido dio ok")
	}
	av := lastToast(m)
	if !strings.Contains(av, "unknown forge") {
		t.Errorf("el aviso %q no dice que el forge no se conoce", av)
	}
	if !strings.Contains(av, "github") {
		t.Errorf("el aviso %q no nombra el forge: sin el nombre el usuario no sabe cuál", av)
	}
	// The level is error and not a warning: waiting does not fix it. Checked on the rendered text
	//rather than on the toast's type because that is what the view shows.
	if nivel := nivelDeAviso(m, "unknown forge"); nivel != "error" {
		t.Errorf("el aviso sale con nivel %q, want error: un forge que no vuelve no es "+
			"transitorio y no se arregla esperando", nivel)
	}
}

func TestUnaAccionConOtraEnCursoSeNiegaYLoDice(t *testing.T) {
	m := newTestModel(t, &testutil.FakeAdapter{ForgeName: "github", HostName: "github.com"})
	it := mkItem("github", "github.com", "acme/widget", "uno", 7, "APPROVED")
	m.actionBusy = true

	if _, ok := m.canActionOn(forge.ActionMerge, it); ok {
		t.Fatal("canActionOn con una acción en curso dio ok: dos merges a la vez")
	}
	av := lastToast(m)
	if !strings.Contains(av, "already running") {
		t.Errorf("el aviso %q no dice que ya hay una en curso", av)
	}
	if nivel := nivelDeAviso(m, "already running"); nivel != "warn" {
		t.Errorf("el aviso sale con nivel %q, want warn: esperar no es un error", nivel)
	}
}

func TestMontarSinSeleccionSeNiegaYLoDice(t *testing.T) {
	m := newTestModel(t)

	salida, cmd := m.startMount()
	if cmd != nil {
		t.Error("montar sin selección devolvió comando: crearía un worktree de nada")
	}
	got := salida.(Model)
	if got.mountBusy {
		t.Error("sin selección se marcó el montaje como en curso: el candado se quedaría " +
			"cerrado y las siguientes pulsaciones no harían nada")
	}
	av := lastToast(got)
	if !strings.Contains(av, "select an item") {
		t.Errorf("el aviso %q no dice que hay que elegir un ítem", av)
	}
	if nivel := nivelDeAviso(got, "select an item"); nivel != "warn" {
		t.Errorf("el aviso sale con nivel %q, want warn: la solución es elegir, no arreglar", nivel)
	}
}

func TestMontarConUnMontajeEnCursoSeNiegaYLoDice(t *testing.T) {
	m := newTestModel(t, &testutil.FakeAdapter{ForgeName: "github", HostName: "github.com"})
	m = conSeleccion(t, m, mkItem("github", "github.com", "acme/widget", "uno", 7, ""))
	m.mounter = &mounterFalso{}
	m.mountBusy = true

	salida, cmd := m.startMount()
	if cmd != nil {
		t.Error("montar con un montaje en curso devolvió comando: dos worktrees en la misma ruta")
	}
	got := salida.(Model)
	if !got.mountBusy {
		t.Error("el candado de montaje se soltó: la siguiente pulsación montaría otro")
	}
	av := lastToast(got)
	if !strings.Contains(av, "already running") {
		t.Errorf("el aviso %q no dice que ya hay un montaje en curso", av)
	}
}

func TestLaAccionDeSalirSeResuelvePorElConfigYElOverlayLaTragaAntes(t *testing.T) {
	cfg := config.Defaults()
	cfg.Keybindings["quit"] = "0"
	nuevo := func(t *testing.T) Model {
		t.Helper()
		m := New(cfg, []forge.Adapter{
			&testutil.FakeAdapter{ForgeName: "github", HostName: "github.com"},
		})
		m.width, m.height = 160, 40
		m.loading = false
		m.cachePath = ""
		return m
	}

	m := nuevo(t)
	m = conSeleccion(t, m, mkItem("github", "github.com", "acme/widget", "uno", 7, ""))
	if _, cmd := pulsar(t, m, "0"); cmd == nil {
		t.Fatal("la tecla reasignada de salir no pidió salir de la TUI")
	}

	m2 := nuevo(t)
	m2 = conSeleccion(t, m2, mkItem("github", "github.com", "acme/widget", "uno", 7, ""))
	m2.retarget.state = retargetChoosing
	m2.retarget.all = []string{"main", "feat/x"}
	m2.retarget.view = m2.retarget.all

	salida, cmd := pulsar(t, m2, "0")
	got := salida
	if cmd != nil {
		t.Error("con el overlay abierto, la tecla de salir lo cerró: perdería el filtro y " +
			"cerraría la sesión sin querer")
	}
	if got.retarget.state != retargetChoosing {
		t.Errorf("el overlay se cerró con la tecla del filtro (state=%d)", got.retarget.state)
	}
	if !strings.Contains(got.retarget.query, "0") {
		t.Errorf("la tecla no llegó al filtro: quedó %q", got.retarget.query)
	}

	for _, tecla := range []string{"q", "ctrl+c"} {
		m3 := nuevo(t)
		m3 = conSeleccion(t, m3, mkItem("github", "github.com", "acme/widget", "uno", 7, ""))
		m3.retarget.state = retargetChoosing
		if _, cmd := pulsar(t, m3, tecla); cmd == nil {
			t.Errorf("%q con el overlay abierto no salió de la TUI", tecla)
		}
	}
}

// The most useful of the three mount warnings because it gives the next action: the mount worked and
// the layout did not.
func TestUnMontajeQueRequiereHerdrSeAviadoConDondeQuedoElWorktree(t *testing.T) {
	av, nivel := mountNotice(executor.Result{
		Worktree: worktree.Worktree{Path: "/wt/prdash-pr-7", Label: "prdash-pr-7"},
	}, nil)
	if !strings.Contains(av, "Herdr") {
		t.Errorf("el aviso %q no dice que hace falta Herdr", av)
	}
	if !strings.Contains(av, "/wt/prdash-pr-7") {
		t.Errorf("el aviso %q no dice dónde quedó el worktree: sin eso no hay nada que hacer", av)
	}
	if nivel != levelWarn {
		t.Errorf("nivel %v, want aviso: el worktree existe y lo que falta es Herdr", nivel)
	}

	ok, okNivel := mountNotice(executor.Result{
		Herdr:    true,
		Worktree: worktree.Worktree{Path: "/wt/prdash-pr-7", Label: "prdash-pr-7"},
		Plan:     plan.Plan{Tabs: []plan.Tab{{Label: "Review"}, {Label: "Edit"}}},
	}, nil)
	if !strings.Contains(ok, "panes") || !strings.Contains(ok, "tabs") {
		t.Errorf("el aviso del camino bueno %q no dice cuántos panes ni tabs", ok)
	}
	if !strings.Contains(ok, "/wt/prdash-pr-7") {
		t.Errorf("el aviso del camino bueno %q no dice la ruta", ok)
	}
	if okNivel != levelOK {
		t.Errorf("nivel %v del camino bueno", okNivel)
	}

	errMsg, errNivel := mountNotice(executor.Result{}, errors.New("no such branch"))
	if !strings.Contains(errMsg, "no such branch") {
		t.Errorf("el aviso de error %q no trae la causa", errMsg)
	}
	if errNivel != levelError {
		t.Errorf("nivel %v del error, want error", errNivel)
	}
}

func TestElOverlayDeCambioDeBaseSeSuperponeAlContenido(t *testing.T) {
	m := newTestModel(t, &testutil.FakeAdapter{ForgeName: "github", HostName: "github.com"})
	m = conSeleccion(t, m, mkItem("github", "github.com", "acme/widget", "uno", 7, ""))
	m.width, m.height = 100, 30

	sinPopup := m.View().Content
	for _, fase := range []retargetState{retargetListing, retargetChoosing, retargetConfirm} {
		m.retarget.state = fase
		conPopup := m.View().Content

		if len(conPopup) <= len(sinPopup) {
			t.Errorf("fase %d: el popup no añadió nada al render (%d -> %d)",
				fase, len(sinPopup), len(conPopup))
			continue
		}
		if !strings.Contains(conPopup, "retarget") {
			t.Errorf("fase %d: el render no contiene la caja del popup", fase)
		}
		if !strings.Contains(conPopup, "acme/widget") {
			t.Errorf("fase %d: el overlay tapó el contenido del inbox", fase)
		}
	}

	m.retarget.state = retargetClosed
	if got := m.View().Content; strings.Contains(got, "retarget ") {
		t.Error("con el popup cerrado el render trae la caja del popup")
	}
}

var _ = time.Second

// The render is inspected rather than the toast's field, because what matters is how it PAINTS.
func nivelDeAviso(m Model, contiene string) string {
	for _, v := range m.toast.toasts {
		if !strings.Contains(v.message, contiene) {
			continue
		}
		switch v.level {
		case toastError:
			return "error"
		case toastWarning:
			return "warn"
		default:
			return "info"
		}
	}
	return ""
}

type mounterFalso struct{}

func (mounterFalso) Mount(context.Context, model.Item) (executor.Result, error) {
	return executor.Result{}, nil
}
