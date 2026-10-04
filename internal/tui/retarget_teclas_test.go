package tui

import (
	"context"
	"strings"
	"testing"
	"time"

	"prdash/internal/forge/model"
	"prdash/internal/testutil"
)

func modelEnRetarget(t *testing.T, fase retargetState) Model {
	t.Helper()
	m := newTestModel(t, &testutil.FakeAdapter{ForgeName: "github", HostName: "github.com"})
	// Both all and view are filled because view is DERIVED from all: anything that reapplies the
	//filter rebuilds it from all.
	ramas := []string{"main", "release/2.0", "feat/x"}
	m.retarget = retargetPanel{
		state:  fase,
		item:   mkItem("github", "github.com", "acme/widget", "algo", 7, ""),
		all:    ramas,
		view:   ramas,
		cursor: 0,
	}
	return m
}

func TestEscCierraElPopupSalvoEnLaConfirmacionDondeEsUnPasoAtras(t *testing.T) {
	for _, fase := range []retargetState{retargetListing, retargetChoosing} {
		m := modelEnRetarget(t, fase)
		antes := m.branchSeq
		salida, _ := pulsar(t, m, "esc")
		got := salida

		if got.retarget.state != retargetClosed {
			t.Errorf("esc en fase %d no cerró el popup (state=%d)", fase, got.retarget.state)
		}
		if got.branchSeq == antes {
			t.Errorf("esc en fase %d no invalidó el listado en vuelo", fase)
		}
	}

	m := modelEnRetarget(t, retargetConfirm)
	m.retarget.query = "lo que el usuario escribió"
	antes := m.branchSeq
	salida, _ := pulsar(t, m, "esc")
	got := salida

	if got.retarget.state != retargetChoosing {
		t.Errorf("esc en la confirmación dejó el popup en %d, want %d (volver a elegir)",
			got.retarget.state, retargetChoosing)
	}
	if got.branchSeq != antes {
		t.Error("esc en la confirmación invalidó el listado: volver a la lista no es empezar " +
			"de cero, y pedir las ramas otra vez sería esperar al forge sin motivo")
	}
	if got.retarget.query != "lo que el usuario escribió" {
		t.Errorf("el filtro quedó en %q: volver a la lista no lo debe tirar",
			got.retarget.query)
	}
}

func TestEnLaFaseDeListadoNingunaTeclaHaceNadaYElPopupSigueEsperando(t *testing.T) {
	for _, tecla := range []string{"x", "enter", "j", "k", "up", "down", "tab", "?"} {
		m := modelEnRetarget(t, retargetListing)
		salida, cmd := pulsar(t, m, tecla)
		got := salida

		if got.retarget.state != retargetListing {
			t.Errorf("%q en la fase de listado movió el popup a %d", tecla, got.retarget.state)
		}
		if cmd != nil {
			t.Errorf("%q en la fase de listado devolvió un comando: una tecla consumida no "+
				"puede lanzar nada", tecla)
		}
	}
}

func TestEnLaConfirmacionSoloEnterHaceAlgoYEnterCierraElPopupPorqueElCambioEstaEnVuelo(t *testing.T) {
	m := modelEnRetarget(t, retargetConfirm)
	m.retarget.chosen = "release/2.0"
	salida, cmd := pulsar(t, m, "enter")

	if cmd != nil {
		t.Error("enter en la confirmación devolvió un tea.Cmd: el panel pone la petición en " +
			"vuelo y reescribe por el canal de eventos")
	}
	got := salida
	if got.retarget.state != retargetClosed {
		t.Errorf("tras confirmar el popup quedó en %d, want cerrado: lo que se le enseña al "+
			"usuario es el aviso de progreso de la cabecera, no el popup", got.retarget.state)
	}
	if got.retarget.chosen != "" {
		t.Errorf("la rama elegida quedó en %q tras cerrar: un segundo enter podría reaplicar "+
			"el mismo cambio", got.retarget.chosen)
	}
	if len(got.toast.texts()) == 0 && !strings.Contains(m.retarget.item.Title, "retarget") {
		t.Error("no quedó ningún aviso de progreso tras confirmar")
	}

	for _, tecla := range []string{"x", "j", "k", "tab", " "} {
		m2 := modelEnRetarget(t, retargetConfirm)
		m2.retarget.chosen = "main"
		salida2, cmd2 := pulsar(t, m2, tecla)
		if salida2.retarget.state != retargetConfirm {
			t.Errorf("%q en la confirmación movió el popup a %d", tecla, salida2.retarget.state)
		}
		if cmd2 != nil {
			t.Errorf("%q en la confirmación devolvió un comando", tecla)
		}
	}
}

// Same asymmetry as the simulation, same reason: esc closes the popup and stays, q leaves.
func TestQYSalirCierranYAbandonanElPopup(t *testing.T) {
	for _, fase := range []retargetState{retargetListing, retargetChoosing, retargetConfirm} {
		for _, tecla := range []string{"q", "ctrl+c"} {
			m := modelEnRetarget(t, fase)
			salida, cmd := pulsar(t, m, tecla)
			got := salida

			if got.retarget.state != retargetClosed {
				t.Errorf("%q en fase %d no cerró el popup", tecla, fase)
			}
			if cmd == nil {
				t.Errorf("%q en fase %d no pidió salir de la TUI", tecla, fase)
			}
		}
	}
}

func TestConFiltroVacioJKNaveganYConFiltroPuestoSonLetrasDelFiltro(t *testing.T) {
	// `k` is tested with the cursor mid-list and not at zero, because clampRetargetCursor CLAMPS
	//instead of wrapping.
	m := modelEnRetarget(t, retargetChoosing)
	if antes := m.retarget.cursor; pulsarM(t, m, "j").retarget.cursor == antes {
		t.Error("con el filtro vacío, j no movió el cursor")
	}
	mArriba := modelEnRetarget(t, retargetChoosing)
	mArriba.retarget.cursor = 2
	if antes := mArriba.retarget.cursor; pulsarM(t, mArriba, "k").retarget.cursor == antes {
		t.Error("con el filtro vacío, k no movió el cursor")
	}
	mPrimero := modelEnRetarget(t, retargetChoosing)
	if got := pulsarM(t, mPrimero, "k").retarget.cursor; got != 0 {
		t.Errorf("k en el primero dejó el cursor en %d, want 0: el recorte no envuelve", got)
	}
	m2 := modelEnRetarget(t, retargetChoosing)
	if antes := m2.retarget.cursor; pulsarM(t, m2, "down").retarget.cursor == antes {
		t.Error("con el filtro vacío, down no movió el cursor")
	}

	m3 := modelEnRetarget(t, retargetChoosing)
	m3.retarget.query = "re"
	antes := m3.retarget.cursor
	salida := pulsarM(t, m3, "j")
	if salida.retarget.cursor != antes {
		t.Error("con filtro escrito, j movió el cursor: se escribiría y se navegaría a la vez")
	}
	if !strings.Contains(salida.retarget.query, "j") {
		t.Errorf("con filtro escrito, j no acabó en el filtro: quedó %q", salida.retarget.query)
	}

	m4 := modelEnRetarget(t, retargetChoosing)
	m4.retarget.query = "release"
	borrado := pulsarM(t, m4, "ctrl+u")
	if borrado.retarget.query != "" {
		t.Errorf("ctrl+u dejó el filtro en %q", borrado.retarget.query)
	}
	// The cursor is explicitly back at ZERO because applyQuery returns it to the top when it reapplies
	//the filter, and k/j clamp rather than wrap.
	paraNavegar := pulsarm(t, borrado, "j")
	paraNavegar.retarget.cursor = 0
	if tras := pulsarm(t, paraNavegar, "j"); tras.retarget.cursor != 1 {
		t.Errorf("tras ctrl+u, j no vuelve a navegar: el cursor quedó en %d, want 1. Se queda "+
			"como letra con el filtro vacío", tras.retarget.cursor)
	}
	if tras := pulsarm(t, paraNavegar, "j"); tras.retarget.query != "" {
		t.Errorf("j con el filtro vacío escribió %q", tras.retarget.query)
	}
	if len(borrado.retarget.view) != 3 {
		t.Errorf("tras ctrl+u quedan %d ramas visibles, want las 3: el filtro se quitó pero "+
			"la lista se comió", len(borrado.retarget.view))
	}
}

// This is the case a filtered-to-zero listing produces: the user types something that matches
// nothing and presses enter.
func TestEnterSinSeleccionNoHaceNada(t *testing.T) {
	m := modelEnRetarget(t, retargetChoosing)
	m.retarget.view = nil
	m.retarget.cursor = 0

	salida, cmd := pulsar(t, m, "enter")
	got := salida

	if got.retarget.state != retargetChoosing {
		t.Errorf("enter sin selección movió el popup a %d", got.retarget.state)
	}
	if got.retarget.chosen != "" {
		t.Errorf("enter sin selección eligió %q: se aplicaría un cambio de base que nadie "+
			"pidió", got.retarget.chosen)
	}
	if cmd != nil {
		t.Error("enter sin selección devolvió un comando: no hay nada que confirmar")
	}
}

func TestUnListadoQueLlegaVacioDiceQueNoHayRamasYNoAbreElSelector(t *testing.T) {
	m := modelEnRetarget(t, retargetListing)
	m.branchSeq = 5

	salida := send(t, m, branchesMsg{seq: 5, key: repoKeyOf(m), names: nil})
	got := salida

	if strings.TrimSpace(got.retarget.errMsg) == "" {
		t.Error("un listado vacío no dejó mensaje: el popup parecería esperando al forge")
	}
	if !strings.Contains(got.retarget.errMsg, "no branches") {
		t.Errorf("el mensaje %q no dice que no hay ramas", got.retarget.errMsg)
	}
	if got.retarget.state == retargetChoosing {
		t.Error("se abrió el selector sin ramas: se mostraría un buscador vacío")
	}
}

func TestUnListadoObsoletoSeDescartaYNoSeGuarda(t *testing.T) {
	m := modelEnRetarget(t, retargetListing)
	m.branchSeq = 7

	salida := send(t, m, branchesMsg{seq: 3, key: repoKeyOf(m), names: []string{"main", "vieja"}})
	got := salida

	if _, hay := got.branchCache[repoKeyOf(got)]; hay {
		t.Error("un listado obsoleto se guardó en el caché: el popup se abriría con ramas " +
			"de hace diez minutos")
	}
	if strings.Contains(got.retarget.errMsg, "no branches") {
		t.Error("un listado obsoleto puso un mensaje de error: no es un fallo, es un resultado " +
			"que ya no le pertenece a nadie")
	}
}

func TestUnListadoObsoletoConErrorTampocoSeGuardaNiSeMuestra(t *testing.T) {
	m := modelEnRetarget(t, retargetChoosing)
	m.branchSeq = 7
	m.retarget.errMsg = ""

	salida := send(t, m, branchesMsg{
		seq: 3, key: repoKeyOf(m), errMsg: "el forge no contesta",
	})
	got := salida

	if got.retarget.errMsg != "" {
		t.Errorf("un error de una petición obsoleta se mostró: %q", got.retarget.errMsg)
	}
	if _, hay := got.branchCache[repoKeyOf(got)]; hay {
		t.Error("un listado obsoleto con error se guardó en el caché")
	}
}

// storeBranches has a LAZY map initialisation that a Model from New does not trigger.
func TestUnListadoSeGuardaEnElCacheYLaSegundaPeticionNoLoVuelveAPedir(t *testing.T) {
	m := modelEnRetarget(t, retargetListing)
	m.branchSeq = 5
	key := repoKeyOf(m)

	salida := send(t, m, branchesMsg{seq: 5, key: key, names: []string{"main", "feat/x"}})
	got := salida

	guardado, hay := got.branchCache[key]
	if !hay {
		t.Fatalf("el listado no se guardó en el caché; el mapa es %v", got.branchCache)
	}
	if len(guardado.names) != 2 || guardado.names[0] != "main" {
		t.Errorf("el caché guarda %v", guardado.names)
	}
	if guardado.fetchedAt.IsZero() {
		t.Error("el caché no anota cuándo se pidió: no hay forma de saber si caduca")
	}
	if !guardado.fetchedAt.After(time.Time{}) {
		t.Error("la fecha del caché es anterior a la nada")
	}
	if got.retarget.state != retargetChoosing {
		t.Errorf("tras el listado el popup quedó en %d, want %d", got.retarget.state, retargetChoosing)
	}
}

func repoKeyOf(m Model) repoKey {
	return keyOf(m.retarget.item)
}

var _ = context.Background
var _ = model.RepoRef{}

func pulsarm(t *testing.T, m Model, key string) Model {
	t.Helper()
	return pulsarM(t, m, key)
}

// Reachable because Model is built zero-valued in one place.
func TestStoreBranchesSobreUnModeloDeValorCeroNoPeta(t *testing.T) {
	var m Model
	m.storeBranches(repoKey{forge: "github", host: "github.com", project: "o/r"},
		[]string{"main"})

	if len(m.branchCache) != 1 {
		t.Errorf("el mapa quedó con %d entradas, want 1: storeBranches no lo inicializó",
			len(m.branchCache))
	}
}
