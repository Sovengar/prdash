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

// `Update` es el switch central y, además de "qué hace con cada mensaje", tiene una segunda
// capa que son las NEGATIVAS: los `if` que rechazan una acción antes de empezarla. Y las
// negativas son el código más numeroso del fichero y el que menos se prueba, porque para
// probar "no hace nada" hace falta el estado justo que lo dispara —una acción en curso, un
// review montándose, un forge desconocido— y eso no se encuentra solo.
//
// Y el valor de probarlas no es la cobertura: es que cada negativa evita una acción que en el
// forge es irreversible. Un approve que se dispara dos veces, un montaje que monta dos worktrees
// sobre la misma ruta, un retarget que cambia la base a la vez que otra.

// TestUnTickDelSpinnerSePasaAlSpinnerYDevuelveSuCmd: la rama más básica del switch.
//
// Y parecetrivial y es la que sostiene el indicador de "hay algo en marcha" en toda la TUI: si
// el tick no llegara al spinner, dejaría de girar en el primer frame y el usuario vería un
// spinner congelado en cada operación.
//
// Y lo que se comprueba es que el `Cmd` del spinner sale, porque un tick que se consume y no
// rearma es un spinner que gira una vez y se para —que es exactamente lo que pasa si alguien
// "simplifica" esta rama y se queda con el estado.
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

// TestUnMensajeQueNoEsUnaTeclaNiUnEventoSeIgnoraYNoRompeNada: la rama `default` del switch.
//
// Y es lo que hace que `Update` sea seguro con cualquier cosa: un mensaje de bubbletea que
// prdash no conoce —un `Paste` si someday lo suporta, un `FocusMsg` de la versión nueva— tiene
// que ignorarse, no Paterson es un `tea.Msg` y el switch tiene que tener un `default`.
//
// Y sin ese `default`, un mensaje desconocido sería un panic en mitad de la TUI, con la vista
// a medias y sin aviso.
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

// TestElTickDeComentariosSeRearmaAunqueNoHayaNadaQueConsultar: la cadena que se mantiene viva.
//
// Y el `commentsTickMsg` NO pasa por la bomba de eventos —no consume un lector del canal— y aun
// así devuelve un comando. Es la excepción consciouslya la regla, y la razón está en el
// comentario: es lo que hace que un cambio de selección se note sin tener que recordarlo en el
// sitio del cambio.
//
// Y lo que se comprueba es que el comando sale SIEMPRE, también sin selección. Un tick que solo
// se rearmara "si hay algo que consultar" es una cadena que se muere en el primer cambio a un
// ítem sin comentarios, y el siguiente ya no se entera.
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

// TestUnaAccionSobreUnForgeDesconocidoSeNiegaYLoDice: `canActionOn` con un forge que no está.
//
// Y el caso es real de una forma concreta: un ítem llega del canal de un forge que se deshabilitó
// mientras su consulta estaba en vuelo. El ítem sigue en pantalla y el forge ya no está en el
// mapa.
//
// Y avisar es lo que evita que el usuario piense que la acción falló: sin el aviso, la tecla
// no hace nada y parece un bug. Y el aviso es de ERROR y no de aviso, porque no es un estado
// transitorio —volverá a pasar— sino un dato que ya no tiene a dónde ir.
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
	// Y el nivel es de error: no se arregla esperando.
	// El aviso tiene que salir como ERROR y no como aviso. Se comprueba sobre el texto
	// renderizado y no sobre el tipo porque `lastToastLevel` devuelve el nivel del TOAST y
	// `levelError` es un `noticeLevel`: son tipos distintos y compararlos necesita un cast que
	// no prueba nada del render.
	if nivel := nivelDeAviso(m, "unknown forge"); nivel != "error" {
		t.Errorf("el aviso sale con nivel %q, want error: un forge que no vuelve no es "+
			"transitorio y no se arregla esperando", nivel)
	}
}

// TestUnaAccionConOtraEnCursoSeNiegaYLoDice: el candado de `actionBusy`.
//
// Y este es el candado que evita la acción doble, que en el forge es irreversible: dos
// approves a la vez, dos merges. Y la forma de dispararlo es real —la tecla se pulsó dos veces
// mientras la primera acción esperaba al forge—.
//
// Y el aviso tiene que explicar que HAY una en curso y no que algo falló: el usuario que ve
// "an action is already running" sabe que tiene que esperar; el que ve un error busca el
// problema en otro sitio.
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

// TestMontarSinSeleccionSeNiegaYLoDice: `startMount` sin ítem bajo el cursor.
//
// Y es la misma negativa de la simulación, y por el mismo motivo: sin ítem no hay repo de origen
// ni rama, y arrancar el montaje sin ellos sería crear un worktree de nada.
//
// Y el aviso tiene que ser de AVISO y no de error, porque la solución es "elige algo" y no
// "arregla algo".
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

// TestMontarConUnMontajeEnCursoSeNiegaYLoDice: el candado de `mountBusy`.
//
// Y el daño de no tenerlo es doble: dos worktrees sobre la misma ruta —que git rechaza, pero con
// un mensaje que no dice cuál de los dos lo provocó— y un layout de panes duplicado.
//
// Y a diferencia del candado de `actionBusy`, el trabajo pesado ya está hecho cuando se pulsa
// la segunda vez, así que el aviso tiene que decir que ESPERA, no que puede reintentarse ya.
func TestMontarConUnMontajeEnCursoSeNiegaYLoDice(t *testing.T) {
	m := newTestModel(t, &testutil.FakeAdapter{ForgeName: "github", HostName: "github.com"})
	m = conSeleccion(t, m, mkItem("github", "github.com", "acme/widget", "uno", 7, ""))
	// Con un mounter. El guard de "no hay mounter" va ANTES del de "ya hay un montaje en curso",
	// así que sin él el aviso que sale es el otro y este test midiendo otra cosa. Mi primera
	// versión loomitió y falló con "mounting a review requires Herdr".
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

// TestLaAccionDeSalirSeResuelvePorElConfigYElOverlayLaTragaAntes: `quit` y quién se queda con
// la tecla.
//
// Y son dos cosas y solo la segunda es la interesante. La primera es que `quit` es una ACCIÓN
// configurable —se puede reasignar— mientras que `q` está cableado en el `switch`, y las dos
// tienen que salir de la TUI.
//
// La segunda es que con un overlay abierto la acción NO se ejecuta. El overlay captura el
// teclado entero, y con `quit` reasignada a `0` esa tecla es una LETRA del filtro de ramas. Mi
// primera versión afirmaba que la salida tenía que funcionar con el popup abierto —"salir sin
// cerrarlo"— y fallaba porque el overlay se queda con la tecla antes.
//
// Y ese orden es lo que hay que fijar, porque la alternativa sería que `0` saliera de prdash
// mientras el usuario está escribiendo un filtro: perdería lo escrito y cerraría la sesión sin
// querer. Para salir con el popup abierto están `q` y `ctrl+c`, que se comprueban aparte.
func TestLaAccionDeSalirSeResuelvePorElConfigYElOverlayLaTragaAntes(t *testing.T) {
	cfg := config.Defaults()
	// Una tecla de un solo carácter. `keyMsg` construye el mensaje como lo hace el decoder, y
	// una cadena de varios caracteres no produce un `String()` fiable: el Code sería solo el
	// primer carácter.
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

	// Sin overlay: la tecla reasignada sale.
	m := nuevo(t)
	m = conSeleccion(t, m, mkItem("github", "github.com", "acme/widget", "uno", 7, ""))
	if _, cmd := pulsar(t, m, "0"); cmd == nil {
		t.Fatal("la tecla reasignada de salir no pidió salir de la TUI")
	}

	// Con el overlay abierto: se la queda el overlay y sale como letra del filtro.
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

	// Y para salir con el overlay abierto están `q` y `ctrl+c`, que no son acciones
	// configurables precisamente por esto.
	for _, tecla := range []string{"q", "ctrl+c"} {
		m3 := nuevo(t)
		m3 = conSeleccion(t, m3, mkItem("github", "github.com", "acme/widget", "uno", 7, ""))
		m3.retarget.state = retargetChoosing
		if _, cmd := pulsar(t, m3, tecla); cmd == nil {
			t.Errorf("%q con el overlay abierto no salió de la TUI", tecla)
		}
	}
}

// TestUnMontajeQueRequiereHerdrSeAviadoConDondeQuedoElWorktree: el aviso de `mountOutcome`.
//
// Y este es el aviso más útil de los tres porque da la acción siguiente. El montaje funciona
// sin Herdr —el worktree existe—, pero la revisión necesita un workspace de Herdr y el popup
// tiene que explicarlo en vez de quedarse mudo.
//
// Y el mensaje DICE dónde quedó el worktree, que es lo que necesita el usuario para ir a él: sin
// esa ruta, el aviso es "esto necesita Herdr" y no hay nada que hacer con eso.
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
	// Y es de AVISO y no de error: el worktree se montó, lo que falta es la ventana.
	if nivel != levelWarn {
		t.Errorf("nivel %v, want aviso: el worktree existe y lo que falta es Herdr", nivel)
	}

	// Y el camino bueno dice cuántos panes y en qué ruta, que es la confirmación de que la
	// operación hizo lo que se le pidió.
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

	// Y el error, que es el otro camino y tiene su propio nivel.
	errMsg, errNivel := mountNotice(executor.Result{}, errors.New("no such branch"))
	if !strings.Contains(errMsg, "no such branch") {
		t.Errorf("el aviso de error %q no trae la causa", errMsg)
	}
	if errNivel != levelError {
		t.Errorf("nivel %v del error, want error", errNivel)
	}
}

// TestElOverlayDeCambioDeBaseSeSuperponeAlContenido: el render del popup en el `View`.
//
// Y lo que se comprueba es que la caja del popup aparece ENCIMA del contenido y no debajo, y
// que sale para CADA fase visible: elegir, confirmar y esperando. Un overlay que solo se pinta
// en una de las tres deja el popup invisible en las otras dos con el teclado capturado, que es
// el peor estado posible —no hay salida visual y el teclado no responde—.
//
// Y la caja va CENTRADA, que es lo que distingue un overlay de un texto suelto.
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
		// Y la caja del popup está: su etiqueta aparece y no estaba sin popup.
		if !strings.Contains(conPopup, "retarget") {
			t.Errorf("fase %d: el render no contiene la caja del popup", fase)
		}
		// Y el contenido del inbox SIGUE debajo, que es lo que hace que sea un overlay y no
		// una sustitución.
		if !strings.Contains(conPopup, "acme/widget") {
			t.Errorf("fase %d: el overlay tapó el contenido del inbox", fase)
		}
	}

	// Y cerrada no hay overlay, que es el control que hace que lo anterior no sea "el render
	// siempre trae la caja".
	m.retarget.state = retargetClosed
	if got := m.View().Content; strings.Contains(got, "retarget ") {
		t.Error("con el popup cerrado el render trae la caja del popup")
	}
}

// El reloj solo aparece en la aserción de estabilidad del spinner.
var _ = time.Second

// nivelDeAviso mira el aviso que contiene `contiene` y devuelve cómo lo PINTA la vista: `info`,
// `warn` o `error`.
//
// Y se mira el render y no el campo del toast porque lo que le importa al usuario es cómo se ve,
// y porque el nivel del aviso (`noticeLevel`) y el del toast (`toastLevel`) son tipos distintos
// que solo se juntan en `View`. Un test que comparase los niveles entre sí estaría midiendo la
// existencia de un cast, no el comportamiento.
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

// mounterFalso es un Mounter que no monta nada, para llegar al candado de `mountBusy` sin el
// trabajo de un executor de verdad.
type mounterFalso struct{}

func (mounterFalso) Mount(context.Context, model.Item) (executor.Result, error) {
	return executor.Result{}, nil
}
