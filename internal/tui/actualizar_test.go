package tui

import (
	"strings"
	"testing"
	"time"

	"prdash/internal/forge/model"
	"prdash/internal/testutil"
)

// `Update` es el switch central, y su regla no es "qué hace con cada mensaje" sino "qué
// rearma y qué no". Y esa regla es lo que sostiene la bomba de eventos: un mensaje que
// consume un lector del canal tiene que devolver SIEMPRE un `Cmd` que arme otro, porque si
// no, el día que ese mensaje llegue el refresco muere en silencio.
//
// Y el caso que de verdad importa es el de un mensaje de CICLO OBSOLETO, que es donde la
// regla se pone a prueba: el mensaje ya se consumió del canal, y descartarlo por ser viejo
// no lo devuelve. Hay cinco mensajes con ciclo —`authMsg`, `pageMsg`, `forgeDoneMsg`,
// `refreshDoneMsg` y `actionMsg`— y los cinco tienen la misma obligation.
//
// Y el otro extremo es igual de importante: `notifyMsg` y `reviewCleanupMsg` NO rearmean
// bomba, y no es un olvido. Vienen del propio modelo, no del canal, así que no consumieron
// un lector; rearmar ahí filtraría una goroutine por cada aviso.

// modelConNuevoCiclo devuelve un modelo con el contador de ciclo en uno, para tener siempre
// un ciclo vigente con el que comparar un mensaje obsoleto.
func modelConNuevoCiclo(t *testing.T) Model {
	t.Helper()
	m := newTestModel(t, &testutil.FakeAdapter{ForgeName: "github", HostName: "github.com"})
	m.cycle = 1
	m.beginRefresh() // deja cycle en 2
	return m
}

// TestUnMensajeDeCicloObsoletoSeDescartaPeroRearmaLaBomba: la regla, en los cinco.
//
// Y el "pero rearma" es la mitad que importa. Un mensaje viejo se descarta SIN tocar datos
// —si no, un refresco que tarda diez segundos podría pisar el resultado de uno que ya
// terminó— pero el lector que consumió ya está gastado. Devolver `nil` ahí no deja el inbox
// mal: lo deja congelado, y la siguiente tecla de refresh no responde.
func TestUnMensajeDeCicloObsoletoSeDescartaPeroRearmaLaBomba(t *testing.T) {
	// Los cinco mensajes que llevan ciclo, cada uno con el suyo.
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

		// Y el mensaje no tocó datos. El `loading` es el testigo: un `refreshDoneMsg`
		// vigente lo apagaría.
		got := salida.(Model)
		if got.loading != antes {
			t.Errorf("%s: un mensaje obsoleto tocó `loading` (%v -> %v)", c.nombre, antes, got.loading)
		}
		// Y la bomba se rearma. `withPump` devuelve un Cmd no nulo siempre, y es lo
		// único que evita que el refresco muera.
		if cmd == nil {
			t.Errorf("%s: un mensaje obsoleto devuelve nil y mata la bomba de eventos", c.nombre)
		}
	}
}

// TestUnMensajeVigenteSiTocaLoQueToca: el otro lado, para que la tabla de arriba no valiera
// por ser "todo se descarta".
//
// Y cada uno tiene su efecto propio, y son distintos entre sí: por eso la regla no puede
// ser "descartar los viejos y ya". El `loading` que apaga `refreshDoneMsg` es el que hace
// desaparecer el spinner; el `auth` que escribe `authMsg` es lo que deja de marcar el forge
// como degradado; y los `lastRefresh`/`backoff` los escribe `refreshDoneMsg` para que el
// próximo tick sepa cuándo fue el último.
func TestUnMensajeVigenteSiTocaLoQueToca(t *testing.T) {
	// refreshDoneMsg: apaga el loading y anota cuándo fue.
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

	// authMsg: escribe el estado del forge y rearma.
	m = modelConNuevoCiclo(t)
	m.statuses = map[string]*forgeStatus{"github": {forge: "github", host: "github.com"}}
	autenticado := model.AuthState{Forge: "github", OK: true, Login: "yo"}
	salida, _ = m.Update(authMsg{cycle: m.cycle, forge: "github", auth: autenticado})
	if !salida.(Model).statuses["github"].auth.OK {
		t.Error("authMsg vigente no escribió el estado de autenticación: el forge seguiría " +
			"marcado como degradado")
	}

	// Y uno para un forge que no está en el mapa: no entra en pánico. Es el caso de un
	// forge que se deshabilitó mientras su consulta estaba en vuelo.
	m = modelConNuevoCiclo(t)
	m.statuses = map[string]*forgeStatus{}
	if _, cmd := m.Update(authMsg{cycle: m.cycle, forge: "deshabilitado",
		auth: model.AuthState{OK: true}}); cmd == nil {
		t.Error("un authMsg de un forge desconocido no rearma la bomba")
	}
}

// TestLosMensajesDelModeloNoRearmanLaBomba: los dos que NO deben.
//
// Y la asimetría con los del canal es lo que hay que fijar, porque es la clase de fuga que
// no se ve: cada aviso que rearma una bomba deja una goroutine esperando un mensaje que no
// va a llegar. Con veinte avisos, veinte goroutines colgadas, y el contador de lectores
// subiendo sin que nadie lea.
//
// Y `notifyMsg` es el caso fácil de confundir porque parece un mensaje más.
func TestLosMensajesDelModeloNoRearmanLaBomba(t *testing.T) {
	// notifyMsg: avisa, y nada más.
	m := modelConNuevoCiclo(t)
	salida, cmd := m.Update(notifyMsg{text: "algo pasó", level: levelWarn})
	if cmd != nil {
		t.Error("notifyMsg devuelve un Cmd: cada aviso dejaría una goroutine esperando")
	}
	got := salida.(Model)
	if strings.TrimSpace(lastToast(got)) == "" {
		t.Error("notifyMsg no enseñó el aviso")
	}

	// Y reviewCleanupMsg, que viene del propio modelo por la misma razón.
	m = modelConNuevoCiclo(t)
	if _, cmd := m.Update(reviewCleanupMsg{}); cmd != nil {
		t.Error("reviewCleanupMsg devuelve un Cmd: no consume un lector y no debe rearmar")
	}
}

// TestElTickRepondeAlDejarDePausarYNoCuandoEstaPausado: la puerta del tick.
//
// Y la asimetría es lo que importa: pausado, el tick se rearma SIN consultar —consultar con
// el refresco pausado es lo que gasta la cuota de la API—, y sin pausar, el tick arranca un
// ciclo. Un tick que hiciera las dos cosas gastaría peticiones con el refresco pausado, que
// es justo cuando el usuario ha dicho que no.
func TestElTickRependeAlDejarDePausarYNoCuandoEstaPausado(t *testing.T) {
	// Pausado: rearma sin consultar.
	m := newTestModel(t, &testutil.FakeAdapter{ForgeName: "github", HostName: "github.com"})
	// `paused()` es `loading || actionBusy`; no hay un flag propio. Con `loading` a true
	// basta, y además es el estado real: un refresco en vuelo pausa el tick siguiente.
	m.loading = true
	m.tickPending = true

	salida, cmd := m.Update(tickMsg{})
	got := salida.(Model)

	// Y `tickPending` vuelve a quedar a true. Y mi primera versión afirmaba lo
	// contrario, que era leer el nombre del campo al revés: el tick que acaba de
	// dispararse se consume y EN EL MISMO paso se rearma uno nuevo, así que a la
	// salida hay un tick pendiente de verdad. Lo que tiene que estar apagado es el
	// primero, y eso se ve en que `armTick` se llamó —que es lo que comprueba el ciclo,
	// más abajo—.
	if !got.tickPending {
		t.Error("un tick pausado no dejó ninguno pendiente: al reanudar no habría siguiente")
	}
	if cmd == nil {
		t.Error("el tick pausado no se rearma: al reanudar no habría siguiente tick")
	}
	// Y no hay ciclo: el contador de ciclos no se movió, que es lo que significa "no consulté".
	if got.cycle != m.cycle {
		t.Errorf("un tick pausado arrancó un ciclo: cycle %d -> %d", m.cycle, got.cycle)
	}

	// Y sin pausar: arranca un ciclo, y por tanto cambia el contador.
	m2 := newTestModel(t, &testutil.FakeAdapter{ForgeName: "github", HostName: "github.com"})
	m2.tickPending = true
	antes := m2.cycle
	salida2, _ := m2.Update(tickMsg{})
	if salida2.(Model).cycle == antes {
		t.Error("un tick sin pausar no arrancó ciclo: el refresco nunca ocurriría")
	}
}
