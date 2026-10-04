package tui

import (
	"strings"
	"testing"
	"time"

	"prdash/internal/forge/model"
	"prdash/internal/testutil"
)

func modelConNuevoCiclo(t *testing.T) Model {
	t.Helper()
	m := newTestModel(t, &testutil.FakeAdapter{ForgeName: "github", HostName: "github.com"})
	m.cycle = 1
	m.beginRefresh() // deja cycle en 2
	return m
}

// The "but re-arms" half is the one that matters.
func TestUnMensajeDeCicloObsoletoSeDescartaPeroRearmaLaBomba(t *testing.T) {
	casos := []struct {
		nombre string
		viejo  interface{}
	}{
		{"authMsg", authMsg{cycle: 0, forge: "github", auth: model.AuthState{OK: false}}},
		{"pageMsg", pageMsg{cycle: 0, key: streamKey{forge: "github"}, items: nil}},
		{"forgeDoneMsg", forgeDoneMsg{cycle: 0, forge: "github"}},
		{"refreshDoneMsg", refreshDoneMsg{cycle: 0}},
		{"actionMsg", actionMsg{cycle: 0}},
	}

	for _, c := range casos {
		m := modelConNuevoCiclo(t)
		m.loading = true
		antes := m.loading

		salida, cmd := m.Update(c.viejo)

		got := salida.(Model)
		if got.loading != antes {
			t.Errorf("%s: un mensaje obsoleto tocó `loading` (%v -> %v)", c.nombre, antes, got.loading)
		}
		// And the pump is re-armed: withPump always returns a non-nil Cmd, and it is the only thing
		// that does.
		if cmd == nil {
			t.Errorf("%s: un mensaje obsoleto devuelve nil y mata la bomba de eventos", c.nombre)
		}
	}
}

// The other side, so the table above is not worth "everything is re-armed".
func TestUnMensajeVigenteSiTocaLoQueToca(t *testing.T) {
	m := modelConNuevoCiclo(t)
	m.loading = true
	m.lastRefresh = time.Time{}

	salida, cmd := m.Update(refreshDoneMsg{cycle: m.cycle})
	got := salida.(Model)

	if got.loading {
		t.Error("refreshDoneMsg vigente dejó `loading` a true: el spinner no pararía")
	}
	if got.lastRefresh.IsZero() {
		t.Error("refreshDoneMsg vigente no anotó lastRefresh: el borde no diría cuándo fue")
	}
	if cmd == nil {
		t.Error("refreshDoneMsg vigente no rearma el tick: el refresco muere tras uno")
	}

	m = modelConNuevoCiclo(t)
	m.statuses = map[string]*forgeStatus{"github": {forge: "github", host: "github.com"}}
	autenticado := model.AuthState{Forge: "github", OK: true, Login: "yo"}
	salida, _ = m.Update(authMsg{cycle: m.cycle, forge: "github", auth: autenticado})
	if !salida.(Model).statuses["github"].auth.OK {
		t.Error("authMsg vigente no escribió el estado de autenticación: el forge seguiría " +
			"marcado como degradado")
	}

	m = modelConNuevoCiclo(t)
	m.statuses = map[string]*forgeStatus{}
	if _, cmd := m.Update(authMsg{cycle: m.cycle, forge: "deshabilitado",
		auth: model.AuthState{OK: true}}); cmd == nil {
		t.Error("un authMsg de un forge desconocido no rearma la bomba")
	}
}

// The asymmetry with the channel's messages.
func TestLosMensajesDelModeloNoRearmanLaBomba(t *testing.T) {
	m := modelConNuevoCiclo(t)
	salida, cmd := m.Update(notifyMsg{text: "algo pasó", level: levelWarn})
	if cmd != nil {
		t.Error("notifyMsg devuelve un Cmd: cada aviso dejaría una goroutine esperando")
	}
	got := salida.(Model)
	if strings.TrimSpace(lastToast(got)) == "" {
		t.Error("notifyMsg no enseñó el aviso")
	}

	m = modelConNuevoCiclo(t)
	if _, cmd := m.Update(reviewCleanupMsg{}); cmd != nil {
		t.Error("reviewCleanupMsg devuelve un Cmd: no consume un lector y no debe rearmar")
	}
}

func TestElTickRependeAlDejarDePausarYNoCuandoEstaPausado(t *testing.T) {
	// Pausado: rearma sin consultar.
	m := newTestModel(t, &testutil.FakeAdapter{ForgeName: "github", HostName: "github.com"})
	// paused() is `loading || actionBusy`; there is no flag of its own.
	m.loading = true
	m.tickPending = true

	salida, cmd := m.Update(tickMsg{})
	got := salida.(Model)

	// My first version asserted the opposite, having read the name instead of the code.
	if !got.tickPending {
		t.Error("un tick pausado no dejó ninguno pendiente: al reanudar no habría siguiente")
	}
	if cmd == nil {
		t.Error("el tick pausado no se rearma: al reanudar no habría siguiente tick")
	}
	if got.cycle != m.cycle {
		t.Errorf("un tick pausado arrancó un ciclo: cycle %d -> %d", m.cycle, got.cycle)
	}

	m2 := newTestModel(t, &testutil.FakeAdapter{ForgeName: "github", HostName: "github.com"})
	m2.tickPending = true
	antes := m2.cycle
	salida2, _ := m2.Update(tickMsg{})
	if salida2.(Model).cycle == antes {
		t.Error("un tick sin pausar no arrancó ciclo: el refresco nunca ocurriría")
	}
}
