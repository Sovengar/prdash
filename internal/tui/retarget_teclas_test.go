package tui

import (
	"context"
	"strings"
	"testing"
	"time"

	"prdash/internal/forge/model"
	"prdash/internal/testutil"
)

// `handleRetargetKey` es un intérprete de teclas sobre una máquina de TRES fases visibles más
// una de espera, y su valor está en la tabla de transiciones: qué tecla hace qué en cada fase.
//
// Y hay una tecla que NO se comporta igual en todas, que es la que hace que este popup no se
// pueda leer de un vistazo: **`esc` es un paso atrás en la confirmación y un cierre en el resto**.
// El motivo está en el comentario del código y es el correcto: el error más probable al
// confirmar es haber señalado la fila equivocada, y volver a la lista lo deshace sin volver a
// pedir las ramas al forge.
//
// Y esa asimetría es exactamente la clase de cosa que un test de "las teclas cierran el popup"
// pasa por alto, porque `esc` sí cierra el popup —en cuatro de las cinco fases— y el test pasa.

// modelEnRetarget es un modelo con el popup de cambio de base en la fase pedida y un listado de
// ramas ya recibido.
func modelEnRetarget(t *testing.T, fase retargetState) Model {
	t.Helper()
	m := newTestModel(t, &testutil.FakeAdapter{ForgeName: "github", HostName: "github.com"})
	// `all` y `view` van los dos, porque `view` es DERIVADA de `all`: cualquier cosa que
	// reaplique el filtro —una letra, `ctrl+u`— la reconstruye desde `all`. Un fixture que
	// rellena solo `view` deja `all` vacío y el filtro se come la lista entera, que es lo que
	// me pasó: `ctrl+u` dejaba el popup sin ramas y parecía un bug del código.
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

// TestEscCierraElPopupSalvoEnLaConfirmacionDondeEsUnPasoAtras: la tecla que no es igual en
// todas las fases.
//
// Y las dos mitades importan por motivos distintos:
//
//   - En las otras fases `esc` CIERRA, y cierra de verdad: el popup desaparece y el invalidado
//     del listado en vuelo sube el contador. Un `esc` que no cerrara dejaría el popup encima del
//     inbox con una lista de ramas que ya no se va a usar.
//   - En la confirmación `esc` NO cierra y VUELVE a elegir, y `esc` en la lista sigue cerrando.
//     Eso es lo que hace recuperable el error de haber señalado la fila equivocada.
//
// Y la comprobación de que "vuelve" es que el estado es EXACTAMENTE `retargetChoosing`, no "no
// es confirm": volver a la lista con un filtro puesto sería perder lo que el usuario había
// escrito, y volver a pedir las ramas sería esperar al forge otra vez.
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

	// Y en la confirmación: un paso atrás, sin cerrar y sin invalidar.
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
	// Y el filtro se conserva: perderlo sería tirar lo que el usuario escribió.
	if got.retarget.query != "lo que el usuario escribió" {
		t.Errorf("el filtro quedó en %q: volver a la lista no lo debe tirar",
			got.retarget.query)
	}
}

// TestEnLaFaseDeListadoNingunaTeclaHaceNadaYElPopupSigueEsperando: la espera.
//
// Y la regla es "cualquier tecla se consume y no pasa nada", y es la correcta: mientras el
// listing está en vuelo, dejar pasar la tecla a la vista dispararía acciones sobre un ítem que
// el usuario ya no está mirando —el popup es el que tiene el teclado—.
//
// Y lo que se comprueba es la parte que no es visible: el spinner sigue y el popup sigue ABIERTO.
// Un `esc` mal puesto cerraría el popup y el usuario perdería el spinner sin saber si el forge
// _contestó o no.
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

// TestEnLaConfirmacionSoloEnterHaceAlgoYEnterCierraElPopupPorqueElCambioEstaEnVuelo: la fase
// de dos tiempos.
//
// Y `enter` es el único que actúa, porque la confirmación es una pregunta de sí o no y las demás
// teclas las trata el `default` como "nada".
//
// Y lo que hace `enter` NO es quedarse en el popup esperando: lo CIERRA, y mi primera versión
// afirmaba lo contrario. El motivo es el del comentario de `startRetarget`: la petición va en
// vuelo y lo que se le enseña al usuario es el aviso de progreso —"retarget (main → release/2.0)
// in progress"— en la cabecera, no un popup con un spinner. Dejarlo abierto daría dos señales a
// la vez y el popup sin hacer nada.
//
// Y por eso `chosen` se vacía: el popup se reinicia entero al cerrarse, que es lo que impide
// que un segundo `enter` reaplique el mismo cambio.
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
	// Y hay un aviso de progreso que nombra las DOS ramas. Es lo único que distingue un
	// retarget de otro en una lista de avisos iguales, así que sin él el usuario no sabría
	// a qué cambio se refiere.
	if len(got.toast.texts()) == 0 && !strings.Contains(m.retarget.item.Title, "retarget") {
		t.Error("no quedó ningún aviso de progreso tras confirmar")
	}

	// Y cualquier otra tecla no hace nada: ni cierra ni cambia nada.
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

// TestQYSalirCierranYAbandonanElPopup: la tecla de irse, que también cancela.
//
// Y es la misma asimetría que en la simulación, con el mismo motivo: `esc` cierra el popup y
// sigue en la TUI, y `q` cierra el popup, cancela el contexto de la app y pide salir. Que
// cancele es lo que evita que un listado de ramas en vuelo siga escribiendo en un modelo que ya
// nadie mira.
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

// TestConFiltroVacioJKNaveganYConFiltroPuestoSonLetrasDelFiltro: el doble oficio de `j` y `k`.
//
// Y es la regla que hace que el buscador sea usable sin ratón, y tiene dos mitades que se
// pisan si se implementa mal:
//
//   - Con el filtro VACÍO, `j` y `k` mueven el cursor. Es lo que espera quien viene de una
//     lista.
//   - Con algo escrito, `j` y `k` son LETRAS del filtro, porque en cuanto se escribe algo la
//     navegación pasa a las flechas —que nunca son texto—.
//
// Y la regla de por qué no se navega con `j`/`k` escribiendo: si se moviera el cursor al teclear,
// cada letra cambiaría la selección y `enter` acabaría 망 El resultado de una búsqueda, que no es
// lo que el usuario quiere confirmar.
//
// Y el caso de `ctrl+u` es el que cierra el círculo: borra el filtro y devuelve `j`/`k` a su
// segundo oficio. Sin eso, vaciar el filtro a mano dejaría al usuario sin forma de navegar.
func TestConFiltroVacioJKNaveganYConFiltroPuestoSonLetrasDelFiltro(t *testing.T) {
	// Filtro vacío: `j` y `k` mueven el cursor.
	//
	// Y `k` se prueba con el cursor en medio de la lista y no en cero, porque
	// `clampRetargetCursor` RECORTA en vez de envolver: en el primero `k` se queda donde está
	// y mi aserto de "se movió" fallaba. Es un recorte a propósito —una lista de ramas en la
	// que `k` en el primero te teletransporta al final es desconcertante—, pero es
	// exactamente el tipo de aserto que mide el test equivocado si no se mira el recorte.
	m := modelEnRetarget(t, retargetChoosing)
	if antes := m.retarget.cursor; pulsarM(t, m, "j").retarget.cursor == antes {
		t.Error("con el filtro vacío, j no movió el cursor")
	}
	mArriba := modelEnRetarget(t, retargetChoosing)
	mArriba.retarget.cursor = 2
	if antes := mArriba.retarget.cursor; pulsarM(t, mArriba, "k").retarget.cursor == antes {
		t.Error("con el filtro vacío, k no movió el cursor")
	}
	// Y el recorte: en el primero, `k` no se mueve y no hay indice negativo.
	mPrimero := modelEnRetarget(t, retargetChoosing)
	if got := pulsarM(t, mPrimero, "k").retarget.cursor; got != 0 {
		t.Errorf("k en el primero dejó el cursor en %d, want 0: el recorte no envuelve", got)
	}
	// Y las flechas también, en los dos casos: con filtro vacío o no, las flechas navegan.
	m2 := modelEnRetarget(t, retargetChoosing)
	if antes := m2.retarget.cursor; pulsarM(t, m2, "down").retarget.cursor == antes {
		t.Error("con el filtro vacío, down no movió el cursor")
	}

	// Con filtro puesto: `j` y `k` son letras.
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

	// Y `ctrl+u` borra el filtro y devuelve `j`/`k` a su segundo oficio.
	m4 := modelEnRetarget(t, retargetChoosing)
	m4.retarget.query = "release"
	borrado := pulsarM(t, m4, "ctrl+u")
	if borrado.retarget.query != "" {
		t.Errorf("ctrl+u dejó el filtro en %q", borrado.retarget.query)
	}
	//
	// Y `j` vuelve a navegar. Con el cursor a CERO explícitamente porque `applyQuery` lo
	// devuelve al principio al reaplicar el filtro, y `k`/`j` recortan: un aserto medido desde
	// el final de la lista fallaría por el recorte y no por el segundo oficio de la tecla.
	paraNavegar := pulsarm(t, borrado, "j")
	paraNavegar.retarget.cursor = 0
	if tras := pulsarm(t, paraNavegar, "j"); tras.retarget.cursor != 1 {
		t.Errorf("tras ctrl+u, j no vuelve a navegar: el cursor quedó en %d, want 1. Se queda "+
			"como letra con el filtro vacío", tras.retarget.cursor)
	}
	// Y el filtro sigue vacío: `j` navegó, no escribió.
	if tras := pulsarm(t, paraNavegar, "j"); tras.retarget.query != "" {
		t.Errorf("j con el filtro vacío escribió %q", tras.retarget.query)
	}
	// Y la lista se recuperó entera: `ctrl+u` quita el filtro, no las ramas.
	if len(borrado.retarget.view) != 3 {
		t.Errorf("tras ctrl+u quedan %d ramas visibles, want las 3: el filtro se quitó pero "+
			"la lista se comió", len(borrado.retarget.view))
	}
}

// TestEnterSinSeleccionNoHaceNada: el borde del `selectedBranch`.
//
// Y es el caso que aparece con un listado filtrado a cero: el usuario escribe algo que no
// matchea nada y pulsa enter. `selectedBranch` devuelve false porque el cursor no está en rango, y
// `enter` no debe hacer nada.
//
// Y no debe hacer nada en particular: NO debe elegir la rama vacía ni la primera del listado sin
// filtrar. Elegir la primera sería aplicar un cambio de base que el usuario no pidió, y es el
// peor de los desenlaces porque se aplica en el forge.
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

// TestUnListadoQueLlegaVacioDiceQueNoHayRamasYNoAbreElSelector: `applyBranches`.
//
// Y es el fallo que se ve cuando el forge contesta sin ramas, que pasa de verdad: un repositorio
// recién creado con la rama por defecto borrada, o un MR de un repo espejo.
//
// Y el mensaje importa: sin él, el popup se abriría con un buscador vacío y el usuario creería
// que el forge va lento. Y `errMsg` es lo que el popup pinta como error, no como hint.
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

// TestUnListadoObsoletoSeDescartaYNoSeGuarda: el `seq` del listado.
//
// Y es la versión del popup de cambio de base de la regla de `Update`, y el caso es el mismo:
// el popup se cerró y se reabrió mientras las ramas estaban en vuelo, y el resultado viejo
// pertenece a una petición que ya no existe.
//
// Y lo que se comprueba es que NO se guarda en el caché. Un listado viejo en el caché es peor
// que no tener: el popup se abriría con las ramas de hace diez minutos y el cambio de base aplicaría contra una rama que ya no existe.
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

// TestUnListadoObsoletoConErrorTampocoSeGuardaNiSeMuestra: el mismo caso por la otra vía.
//
// Y el error de un listado obsoleto es más delicado que el éxito: un mensaje de error de una
// petición vieja se vería en un popup que ya está eligiendo otra cosa, y el usuario cerraría un
// popup que funciona.
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

// TestUnListadoSeGuardaEnElCacheConSuFechaYElPopupAbreElSelector: `storeBranches`.
//
// Y `storeBranches` tiene una inicialización LAZADA del mapa que en un modelo de `New` no se
// dispara nunca, porque `New` ya lo crea. Mi primera versión afirmaba que el modelo arrancaba con
// el mapa a nil y falló: la afirmación era sobre el constructor equivocado.
//
// Y aun así el `if` no sobra: escribir en un mapa nil no es un error visible sino un PANIC, y el
// panic sale en el sitio más tonto posible —al abrir el popup— sin nada que lo relacione con la
// escritura del listado. Se deja comprobar aparte, con un modelo de valor cero.
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
	// Y con la fecha, que es lo que permite saber si hay que volver a pedirlo.
	if guardado.fetchedAt.IsZero() {
		t.Error("el caché no anota cuándo se pidió: no hay forma de saber si caduca")
	}
	if !guardado.fetchedAt.After(time.Time{}) {
		t.Error("la fecha del caché es anterior a la nada")
	}
	// Y el popup pasa a elegir.
	if got.retarget.state != retargetChoosing {
		t.Errorf("tras el listado el popup quedó en %d, want %d", got.retarget.state, retargetChoosing)
	}
}

// repoKeyOf es la clave de caché del repo del ítem que lleva el popup, que es con la que se
// indexan los listados.
func repoKeyOf(m Model) repoKey {
	return keyOf(m.retarget.item)
}

// branchesMsgDe deja constancia de que el fichero construye mensajes del canal de eventos.
var _ = context.Background
var _ = model.RepoRef{}

// pulsarm aplica una tecla y devuelve solo el modelo, para las comprobaciones encadenadas.
func pulsarm(t *testing.T, m Model, key string) Model {
	t.Helper()
	return pulsarM(t, m, key)
}

// TestStoreBranchesSobreUnModeloDeValorCeroNoPeta: el mapa nil, que sí es alcanzable.
//
// Y es alcanzable porque `Model` se construye a valor cero en un sitio: cuando un test —o un
// futuro camino de código— arma un `Model{}` sin pasar por `New`. Es un test de una línea, y
// existe porque el fallo que evita es un panic en vez de un error.
func TestStoreBranchesSobreUnModeloDeValorCeroNoPeta(t *testing.T) {
	var m Model
	m.storeBranches(repoKey{forge: "github", host: "github.com", project: "o/r"},
		[]string{"main"})

	if len(m.branchCache) != 1 {
		t.Errorf("el mapa quedó con %d entradas, want 1: storeBranches no lo inicializó",
			len(m.branchCache))
	}
}
