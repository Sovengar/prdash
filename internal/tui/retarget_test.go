package tui

import (
	"strings"
	"testing"
	"time"

	"prdash/internal/forge"
	"prdash/internal/forge/model"
	"prdash/internal/testutil"
	"prdash/internal/worktree"
)

// retargetFixture monta un modelo con un ítem abierto y un adapter que devuelve
// las ramas del repo. El ítem se registra en el adapter para que ItemState lo
// devuelva, que es lo que necesita el guard de la acción para no cortarla.
//
// Devuelve el modelo ya con el popup abierto y las ramas cargadas, que es el punto
// de partida de casi todos los tests de aquí.
func retargetFixture(t *testing.T, branches ...string) (Model, *testutil.FakeAdapter) {
	t.Helper()
	m, a := openRetargetFixture(t, branches...)
	// El listado se espera del canal y no se inyecta a mano: lo que interesa es
	// que el goroutine llegue y que su mensaje se aplique, y leer el canal es la
	// única forma de comprobarlo sin dormir y sin dejar la carrera a un `time.Sleep`.
	// Se lee UNA vez: leerlo aquí y otra vez después se quedaría esperando un
	// mensaje que ya se consumió.
	return send(t, m, waitMount(t, m).(branchesMsg)), a
}

// openRetargetFixture deja el modelo con el popup abierto y el listado todavía sin
// llegar, que es lo que necesitan los tests que hablan de lo que pasa mientras se
// espera. Quien lo use y necesite el listado, lee un evento del canal.
func openRetargetFixture(t *testing.T, branches ...string) (Model, *testutil.FakeAdapter) {
	t.Helper()
	a := ghAdapter()
	a.BranchLists = map[string][]string{"acme/widget": branches}
	it := mkItem("github", "github.com", "acme/widget", "Add widget", 7, "REVIEW_REQUIRED")
	a.ItemStates = map[string]model.Item{stateKey(it): it}

	m := newTestModel(t, a)
	m = send(t, m, page(1, "github", "github.com", model.SectionReview, model.ReviewRequested,
		[]model.Item{it}, false))

	return press(t, m, "e"), a
}

// loadBranches entrega el listado como si lo devolviera el forge. Se hace a mano
// en vez de esperar al goroutine porque los tests de la TUI no deben depender de
// un temporizador: lo que se prueba aquí es la reacción al mensaje, no la
// carrera.
func loadBranches(t *testing.T, m Model, a *testutil.FakeAdapter, branches []string) Model {
	t.Helper()
	if branches == nil {
		branches = a.BranchLists["acme/widget"]
	}
	it, _ := m.selected()
	return send(t, m, branchesMsg{seq: m.branchSeq, key: keyOf(it), names: branches})
}

// pressFilter escribe un filtro letra a letra, como lo haría el terminal. Importa
// que pase por msg.Text: el popup no lee Key.String() para el texto que se
// escribe, y un test que mandara solo el Code probaría un camino que el decoder
// nunca produce.
func pressFilter(t *testing.T, m Model, word string) Model {
	t.Helper()
	for _, r := range word {
		m = press(t, m, string(r))
	}
	return m
}

// TestEAbreElBuscadorYTraeLasRamasDelForge: `e` abre el popup y lo que aparece son
// las ramas del repositorio, no las bases que ya salen en el inbox. Un subconjunto
// dejaría fuera el destino que se busca sin decir que falta.
func TestEAbreElBuscadorYTraeLasRamasDelForge(t *testing.T) {
	m, a := retargetFixture(t, "main", "develop", "release/2.0")

	if m.retarget.state != retargetChoosing {
		t.Fatalf("estado del popup = %v, want el buscador", m.retarget.state)
	}
	if a.BranchCallCount("acme/widget") != 1 {
		t.Errorf("Branches = %d llamadas, want 1", a.BranchCallCount("acme/widget"))
	}
	// La base de la que se sale va la primera, aunque el forge la devuelva donde
	// quiera: es la única fila que describe el punto de partida.
	if m.retarget.view[0] != "main" {
		t.Errorf("la primera fila es %q, want la base actual", m.retarget.view[0])
	}
	if got := strings.Join(m.retarget.view, ","); got != "main,develop,release/2.0" {
		t.Errorf("view = %q, want las tres ramas", got)
	}
}

// TestElFiltroDejaLasRamasQueLoContienen: el filtro va sobre el nombre entero, no
// sobre el último segmento, que es como se piensa en nombres como
// `fix/hunk-pane-argv`.
func TestElFiltroDejaLasRamasQueLoContienen(t *testing.T) {
	m, _ := retargetFixture(t, "main", "fix/hunk-pane-argv", "release/2.0")

	m = pressFilter(t, m, "hunk")
	if got := strings.Join(m.retarget.view, ","); got != "fix/hunk-pane-argv" {
		t.Errorf("view = %q, want solo la rama que contiene el filtro", got)
	}

	// `backspace` quita un carácter del filtro y `ctrl+u` lo borra entero: son las
	// dos correcciones que se necesitan sin llegar a `esc`, que además cerraría
	// el popup.
	m = press(t, m, "backspace")
	if m.retarget.query != "hun" {
		t.Errorf("query = %q, want que backspace quite un carácter", m.retarget.query)
	}
	m = press(t, m, "ctrl+u")
	if m.retarget.query != "" {
		t.Errorf("query = %q, want que ctrl+u la borre entera", m.retarget.query)
	}

	// Y no distingue mayúsculas, porque es una ayuda para recordar y no una
	// escritura: el nombre lo pone el forge.
	m = pressFilter(t, m, "HUNK")
	if got := strings.Join(m.retarget.view, ","); got != "fix/hunk-pane-argv" {
		t.Errorf("view = %q, want el filtro sin distinguir mayúsculas", got)
	}
}

// TestJYKConFiltroEscribeYNavega: es la regla que hace que las dos mitades no se
// pisen. Con el filtro vacío `j`/`k` mueven; en cuanto hay texto son dos letras
// más, porque escribir un nombre de rama con `j` tiene que ser posible.
func TestJYKConFiltroEscribeYNavega(t *testing.T) {
	m, _ := retargetFixture(t, "main", "develop", "release/2.0", "fix/jj-one")

	m = press(t, m, "j")
	if m.retarget.cursor != 1 {
		t.Fatalf("con el filtro vacío, `j` no movió el cursor (cursor=%d)", m.retarget.cursor)
	}

	// En cuanto el filtro tiene algo, `j` escribe. Es lo que permite escribir
	// `fix/jj-one`: si `j` navegara, no habría forma de teclear una `j`.
	m = pressFilter(t, m, "re")
	m = press(t, m, "j")
	if m.retarget.query != "rej" {
		t.Errorf("query = %q, want que `j` escriba con el filtro activo", m.retarget.query)
	}
	if m.retarget.cursor != 0 {
		t.Errorf("cursor = %d, want vuelta arriba en cuanto cambia el filtro", m.retarget.cursor)
	}

	// Y con el filtro puesto se navega con las flechas, que nunca son texto.
	m = press(t, m, "ctrl+u")
	m = press(t, m, "down")
	if m.retarget.cursor != 1 {
		t.Errorf("cursor = %d, want que `down` mueva con el filtro vacío", m.retarget.cursor)
	}
}

// TestElegirLaBaseQueYaTieneNoGastaLlamada: es un no-op y pedirlo al forge sería
// gastar una llamada para que conteste "sin cambios". Se corta en la vista, que es
// donde se sabe qué base tenía el ítem.
func TestElegirLaBaseQueYaTieneNoGastaLlamada(t *testing.T) {
	m, a := retargetFixture(t, "main", "develop")

	m = press(t, m, "enter") // main es la base actual y sale la primera
	if m.retarget.state != retargetClosed {
		t.Errorf("estado = %v, want el popup cerrado", m.retarget.state)
	}
	if a.RetargetCount() != 0 {
		t.Errorf("llamó al forge (%v) para no cambiar nada", a.Retargets)
	}
	assertToast(t, m, "already main")
}

// TestElCambioSeConfirmaAntesDeSalir: la segunda pulsación es la confirmación y la
// línea del medio dice qué pasa a ser verdad. Un `enter` de más sin leer sería
// cambiar la base de un PR a ciegas.
func TestElCambioSeConfirmaAntesDeSalir(t *testing.T) {
	m, a := retargetFixture(t, "main", "release/2.0")

	m = press(t, m, "down")
	m = press(t, m, "enter")
	if m.retarget.state != retargetConfirm {
		t.Fatalf("estado = %v, want la confirmación", m.retarget.state)
	}
	if a.RetargetCount() != 0 {
		t.Fatalf("la confirmación ya llamó al forge (%v)", a.Retargets)
	}

	box := stripANSI(m.retargetOverlay2())
	for _, want := range []string{"main", "→", "release/2.0", "enter apply"} {
		if !strings.Contains(box, want) {
			t.Errorf("la confirmación no dice %q:\n%s", want, box)
		}
	}

	// Y el `enter` de la confirmación es lo que sale. El resultado llega por el
	// canal, así que se espera en vez de suponerlo.
	m = press(t, m, "enter")
	out := waitOutcome(t, m)
	if !out.OK {
		t.Errorf("la acción salió = %+v, want OK", out)
	}
	if out.Base != "release/2.0" {
		t.Errorf("Outcome.Base = %q, want la rama aplicada para el aviso", out.Base)
	}
	if a.RetargetCount() != 1 || a.Retargets[0] != "release/2.0" {
		t.Errorf("el adapter recibió %v, want release/2.0", a.Retargets)
	}
	if m.retarget.state != retargetClosed {
		t.Errorf("el popup sigue abierto tras aplicar")
	}
	if !m.actionBusy {
		t.Error("la acción no quedó marcada como en curso")
	}
}

// retargetOverlay2 es la caja del popup como texto, para poder buscar dentro.
func (m Model) retargetOverlay2() string {
	box, _ := m.retargetOverlay()
	return box
}

// TestEscEnLaConfirmacionVuelveALaLista: el error más probable al confirmar es
// señalar la fila equivocada, y volver a la lista lo deshace sin volver a pedir las
// ramas. Cerrar el popup sería perder las tres pulsaciones.
func TestEscEnLaConfirmacionVuelveALaLista(t *testing.T) {
	m, a := retargetFixture(t, "main", "release/2.0")

	m = press(t, m, "down")
	m = press(t, m, "enter")
	m = press(t, m, "esc")

	if m.retarget.state != retargetChoosing {
		t.Errorf("estado = %v, want volver al buscador", m.retarget.state)
	}
	if a.BranchCallCount("acme/widget") != 1 {
		t.Errorf("volvió a pedir las ramas: %d llamadas", a.BranchCallCount("acme/widget"))
	}
}

// TestElPopupNoDejaPasarLasTeclasALaVista: abierto, se lleva el teclado entero. Si
// una tecla se colara, `a` aprobaría el PR que el usuario ya no está mirando.
func TestElPopupNoDejaPasarLasTeclasALaVista(t *testing.T) {
	m, a := retargetFixture(t, "main", "release/2.0")

	m = press(t, m, "a")
	if a.ActionCallCount("approve", mkRef(), 7) != 0 {
		t.Error("una tecla del popup disparó un approve en la vista")
	}
	if m.retarget.state != retargetChoosing {
		t.Errorf("estado = %v, want el popup abierto", m.retarget.state)
	}
}

// TestElListadoSeCacheaPorRepositorio: abrir y cerrar el popup no puede costar una
// paginación de ramas cada vez, que es lo que hace que la acción termine sin
// usarse. Volver al mismo repo sin esperar el TTL no vuelve a preguntar.
func TestElListadoSeCacheaPorRepositorio(t *testing.T) {
	m, a := retargetFixture(t, "main", "release/2.0")
	if a.BranchCallCount("acme/widget") != 1 {
		t.Fatalf("primera apertura = %d llamadas, want 1", a.BranchCallCount("acme/widget"))
	}

	m = press(t, m, "esc")
	m = press(t, m, "e")
	if m.retarget.state != retargetChoosing {
		t.Fatalf("estado = %v, want el buscador abierto al instante desde el caché", m.retarget.state)
	}
	if a.BranchCallCount("acme/widget") != 1 {
		t.Errorf("la segunda apertura vuelva a pedir las ramas: %d llamadas", a.BranchCallCount("acme/widget"))
	}
}

// TestElCacheCaduca: una rama creada hace un minuto tiene que aparecer, o el
// buscador mentiría sobre lo que el repositorio tiene.
func TestElCacheCaduca(t *testing.T) {
	m, a := retargetFixture(t, "main")
	key := keyOf(m.retargetItemForTest())
	m.branchCache[key] = branchCache{names: []string{"main"}, fetchedAt: time.Now().Add(-branchCacheTTL - time.Second)}

	m = press(t, m, "esc")
	m = press(t, m, "e")
	if m.retarget.state != retargetListing {
		t.Errorf("estado = %v, want que vuelva a preguntar por un caché caducado", m.retarget.state)
	}
	waitMount(t, m) // el segundo pedido sale solo, como el primero
	if a.BranchCallCount("acme/widget") != 2 {
		t.Errorf("llamadas = %d, want 2: un caché caducado se vuelve a pedir", a.BranchCallCount("acme/widget"))
	}
}

// TestUnListadoIlegibleSeExplica: una respuesta que no se entendió no es un
// repositorio sin ramas. El popup lo dice, en vez de abrirse vacío y dejar que el
// usuario piense que no hay destino al que moverse.
func TestUnListadoIlegibleSeExplica(t *testing.T) {
	a := ghAdapter()
	it := mkItem("github", "github.com", "acme/widget", "Add widget", 7, "REVIEW_REQUIRED")
	m := newTestModel(t, a)
	m = send(t, m, page(1, "github", "github.com", model.SectionReview, model.ReviewRequested,
		[]model.Item{it}, false))
	m = press(t, m, "e")

	m = send(t, m, branchesMsg{seq: m.branchSeq, errMsg: "gh: Not Found (HTTP 404)"})

	box := stripANSI(m.retargetOverlay2())
	if !strings.Contains(box, "404") {
		t.Errorf("el popup no explica el fallo del forge:\n%s", box)
	}
	if m.retarget.state != retargetListing {
		t.Errorf("estado = %v, want quedarse esperando con el motivo a la vista", m.retarget.state)
	}
}

// TestElPopupAplicaSobreElItemQueSeConfirmo: un refresco puede mover el cursor
// mientras el popup está abierto. Lo que hay que cambiar de base es lo que el
// usuario vio, no lo que ahora esté debajo del cursor.
func TestElPopupAplicaSobreElItemQueSeConfirmo(t *testing.T) {
	m, a := retargetFixture(t, "main", "release/2.0")
	other := mkItem("github", "github.com", "acme/otro", "Otro PR", 9, "REVIEW_REQUIRED")
	m = send(t, m, page(1, "github", "github.com", model.SectionReview, model.ReviewRequested,
		[]model.Item{other}, false))
	if !m.selectedIs(other) {
		t.Fatalf("el cursor no está sobre el otro ítem: el test no probaría nada")
	}

	m = press(t, m, "down") // el popup sigue con su teclado
	m = press(t, m, "enter")
	m = press(t, m, "enter")
	waitOutcome(t, m)

	if a.RetargetCount() != 1 || a.Retargets[0] != "release/2.0" {
		t.Errorf("el adapter recibió %v, want el cambio del ítem confirmado", a.Retargets)
	}
	if n := a.ActionCallCount("retarget", mkRef(), 7); n != 1 {
		t.Errorf("retarget sobre acme/widget = %d, want 1 (el ítem del popup, no el del cursor)", n)
	}
}

// TestUnListadoTardioNoSePinta: si el popup se cerró mientras se pedían las ramas,
// el listado que llega después se descarta. Pintarlo saltaría la vista al sitio
// donde estaba, y si además fuera de otro repositorio, mostraría sus ramas.
func TestUnListadoTardioNoSePinta(t *testing.T) {
	m, _ := retargetFixture(t, "main", "release/2.0")
	m = press(t, m, "esc") // cerrado: el pedido en vuelo queda obsoleto

	// El mensaje llega con el seq que tenía antes del cierre.
	m = send(t, m, branchesMsg{seq: m.branchSeq, names: []string{"otra/cosa"}})

	if m.retarget.state != retargetClosed {
		t.Errorf("un listado tardío reabrió el popup: estado = %v", m.retarget.state)
	}
}

// TestElAvisoDiceDeQueBaseAQue: el resultado de un retarget sin las dos ramas no
// dice nada de lo que pasó con el PR, que es lo que el usuario acaba de hacer.
func TestElAvisoDiceDeQueBaseAQue(t *testing.T) {
	m, a := retargetFixture(t, "main", "release/2.0")
	m, out := applyBranchAndMove(t, m, a)
	// El releído trae ya la base nueva, que es lo que pinta la ficha.
	it, _ := m.selected()
	it.TargetBranch = out.Base

	m = applyActionResult(t, m, forge.Outcome{
		Kind: forge.ActionRetarget, OK: true, HasItem: true, Item: it,
		Base: out.Base, FromBase: "main",
	})
	if !strings.Contains(lastToast(m), "main") || !strings.Contains(lastToast(m), "release/2.0") {
		t.Errorf("el aviso = %q, want las dos ramas", lastToast(m))
	}
}

// TestElAvisoAvisaDelWorktreeDesfasado: cambiar la base no toca el worktree, así
// que sigue con la que tenía el ítem. Se avisa y no se arregla porque el worktree
// es del usuario y puede tener cambios sin commitear.
func TestElAvisoAvisaDelWorktreeDesfasado(t *testing.T) {
	m, a := retargetFixture(t, "main", "release/2.0")
	m.SetReviewLookup(fakeLookup{path: "/tmp/wt/prdash-pr-7"})

	m, out := applyBranchAndMove(t, m, a)
	it, _ := m.selected()
	m = applyActionResult(t, m, forge.Outcome{
		Kind: forge.ActionRetarget, OK: true, HasItem: true, Item: it,
		Base: out.Base, FromBase: "main",
	})

	if !strings.Contains(lastToast(m), "mounted review") {
		t.Errorf("el aviso = %q, want que nombre el review desfasado", lastToast(m))
	}
}

// TestSinReviewMontadoNoHayQueAvisar: el aviso es por el caso real, no un adorno
// en todos los retarget.
func TestSinReviewMontadoNoHayQueAvisar(t *testing.T) {
	m, a := retargetFixture(t, "main", "release/2.0")
	m.SetReviewLookup(fakeLookup{}) // hay registro, pero no review montado

	m, out := applyBranchAndMove(t, m, a)
	it, _ := m.selected()
	m = applyActionResult(t, m, forge.Outcome{
		Kind: forge.ActionRetarget, OK: true, HasItem: true, Item: it,
		Base: out.Base, FromBase: "main",
	})

	if strings.Contains(lastToast(m), "mounted review") {
		t.Errorf("el aviso = %q, want que no hable de un review que no existe", lastToast(m))
	}
}

// TestSinRegistroDeReviewsNoHayQueAvisar: sin el puerto inyectado la acción
// funciona igual y solo se pierde el aviso. Es la degradación fuera de Herdr, que
// en el resto de la TUI tampoco cuelga nada.
func TestSinRegistroDeReviewsNoHayQueAvisar(t *testing.T) {
	m, a := retargetFixture(t, "main", "release/2.0")
	if m.reviewLookup != nil {
		t.Fatal("el test necesita el modelo sin registro: New no lo inyecta")
	}
	m, out := applyBranchAndMove(t, m, a)
	it, _ := m.selected()
	m = applyActionResult(t, m, forge.Outcome{
		Kind: forge.ActionRetarget, OK: true, HasItem: true, Item: it,
		Base: out.Base, FromBase: "main",
	})

	if !strings.Contains(lastToast(m), "release/2.0") {
		t.Errorf("el aviso = %q, want el resultado del cambio", lastToast(m))
	}
}

// TestUnItemNoAccionableNoAbreElPopup: la base solo se cambia en un ítem que
// todavía es algo que integrar. Abrir el popup y que falle al confirmar sería
// gastar la llamada de las ramas para nada.
func TestUnItemNoAccionableNoAbreElPopup(t *testing.T) {
	a := ghAdapter()
	it := mkItem("github", "github.com", "acme/widget", "Add widget", 7, "APPROVED")
	it.State = "MERGED"
	m := newTestModel(t, a)
	m = send(t, m, page(1, "github", "github.com", model.SectionReview, model.ReviewRequested,
		[]model.Item{it}, false))

	m = press(t, m, "e")

	if m.retarget.state != retargetClosed {
		t.Errorf("estado = %v, want que no abra sobre un ítem mergeado", m.retarget.state)
	}
	if a.BranchCallCount("acme/widget") != 0 {
		t.Error("pidió las ramas de un ítem que no es accionable")
	}
	assertToast(t, m, "already merged")
}

// TestElPopupNoSeAbreConElMergeArmado: son dos confirmaciones distintas y
// coexistiendo nadie sabe a cuál responde la próxima tecla. Con el merge armado la
// `e` no abre nada: la consume el merge, que desarma y se comporta como si no se
// hubiera pulsado. Es el comportamiento de siempre del merge armado, y lo que se
// comprueba aquí es que el popup no se cuela por el hueco.
func TestElPopupNoSeAbreConElMergeArmado(t *testing.T) {
	m, a := retargetFixture(t, "main", "release/2.0")
	m = press(t, m, "esc")

	m = press(t, m, "m") // arma el merge
	if !m.mergeArmed {
		t.Fatal("el merge no se armó: el test no probaría nada")
	}
	m = press(t, m, "e")

	if m.retarget.state != retargetClosed {
		t.Errorf("estado = %v, want que el popup no se abra con el merge armado", m.retarget.state)
	}
	if a.BranchCallCount("acme/widget") != 1 {
		t.Errorf("la `e` llegó a pedir ramas: %d llamadas", a.BranchCallCount("acme/widget"))
	}
}

// TestLaTeclaSePuedeRebind: la acción sale de [keybindings] como las demás.
func TestLaTeclaSePuedeRebind(t *testing.T) {
	a := ghAdapter()
	a.BranchLists = map[string][]string{"acme/widget": {"main", "develop"}}
	m := newTestModel(t, a)
	m.cfg.Keybindings["retarget"] = "T"
	m = send(t, m, page(1, "github", "github.com", model.SectionReview, model.ReviewRequested,
		[]model.Item{mkItem("github", "github.com", "acme/widget", "Add widget", 7, "REVIEW_REQUIRED")}, false))

	m = press(t, m, "T")
	m = send(t, m, waitMount(t, m).(branchesMsg))

	if m.retarget.state != retargetChoosing {
		t.Errorf("estado = %v, want que el rebind abra el buscador", m.retarget.state)
	}
}

// TestUnRechazoDelForgeSeEnsenaSinElArgv: el motivo tiene que ser el del forge, no
// la línea de stderr con el comando entero. Un rechazo por una rama que no existe
// ("Proposed base branch 'x' was not found") es accionable; `gh api -X PATCH …`
// no lo es, y con el argv delante ni se lee.
//
// Y no puede salir como conflicto: un refresco no arregla un nombre de rama malo,
// así que prometer un refresco sería mandar al usuario a mirar algo que no ha
// cambiado.
func TestUnRechazoDelForgeSeEnsenaSinElArgv(t *testing.T) {
	m, a := retargetFixture(t, "main", "release/2.0")
	a.ActionWarnings = map[string][]model.Warning{
		"retarget:" + stateKey(m.retarget.item): {{
			Forge: "github", Kind: "validation",
			Msg: "Proposed base branch 'release/2.0' was not found",
		}},
	}

	m, out := applyBranchAndMove(t, m, a)
	m = applyActionResult(t, m, out)

	if out.OK {
		t.Error("un rechazo del forge no puede salir como OK")
	}
	if out.Conflict {
		t.Errorf("salió como conflicto: un refresco no arregla el nombre de la rama")
	}
	if strings.Contains(lastToast(m), "-X PATCH") {
		t.Errorf("el aviso = %q, want el motivo del forge y no el argv", lastToast(m))
	}
	if !strings.Contains(lastToast(m), "was not found") {
		t.Errorf("el aviso = %q, want el motivo del forge", lastToast(m))
	}
}

// fakeLookup es el registro de reviews montados: dice que hay uno con worktree si
// se le pasa una ruta, y que no hay ninguno si se deja vacía.
type fakeLookup struct{ path string }

func (f fakeLookup) ActiveReview(model.Item) (worktree.Worktree, bool) {
	if f.path == "" {
		return worktree.Worktree{}, false
	}
	return worktree.Worktree{Path: f.path}, true
}

// mkRef es la referencia del ítem del fixture, la que se usa como clave en el
// adapter.
func mkRef() model.RepoRef {
	return model.RepoRef{Forge: "github", Host: "github.com", Project: "acme/widget", Owner: "acme", Name: "widget"}
}

// applyBranchAndMove recorre el popup entero hasta aplicar el cambio y devuelve el
// resultado, que llega por el canal como cualquier otra acción. Se espera antes de
// afirmar: el adapter lo llama el goroutine, y comprobarlo antes sería mirar una
// carrera.
func applyBranchAndMove(t *testing.T, m Model, a *testutil.FakeAdapter) (Model, forge.Outcome) {
	t.Helper()
	m = press(t, m, "down")
	m = press(t, m, "enter")
	m = press(t, m, "enter")
	out := waitOutcome(t, m)
	if a.RetargetCount() != 1 {
		t.Fatalf("el popup no llegó a aplicar: %v", a.Retargets)
	}
	return m, out
}

// applyActionResult inyecta el resultado de la acción como si volviera del canal.
func applyActionResult(t *testing.T, m Model, out forge.Outcome) Model {
	t.Helper()
	return send(t, m, actionMsg{cycle: m.cycle, outcome: out})
}

// selectedIs dice si el ítem bajo el cursor es el dado.
func (m Model) selectedIs(it model.Item) bool {
	got, ok := m.selected()
	return ok && got.ID() == it.ID()
}

// retargetItemForTest es el ítem del fixture, para llegar a su clave de caché sin
// tener el popup abierto.
func (m Model) retargetItemForTest() model.Item {
	it, _ := m.selected()
	return it
}
