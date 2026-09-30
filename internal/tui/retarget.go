// Cambio de la rama destino de un PR/MR: un popup que lista las ramas del
// repositorio, deja filtrarlas y pide confirmación antes de mover el ítem.
//
// Las ramas salen del forge y no de un campo de texto por una razón concreta:
// cambiar la base a una rama que se le parece pero no es (`main` por `main-2`, o
// `release/2.0` por `release/2.0-rc1`) lo acepta el forge sin quejarse y no se ve
// hasta que el PR apunta a la rama equivocada. Una errata que ni el compilador ni
// el forge señalan es justo la que un buscador hace imposible, y su precio —una
// llamada al abrir— se paga una vez por repositorio.
package tui

import (
	"context"
	"fmt"
	"slices"
	"sort"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"prdash/internal/forge"
	"prdash/internal/forge/model"
	"prdash/internal/worktree"
)

// Geometría del popup.
const (
	// retargetChooserWidth es el ancho del popup: la base de la que se sale, el
	// filtro, las ramas y la ayuda. Ancho de más solo añade aire.
	retargetChooserWidth = 64
	// retargetRows es cuántas ramas se ven a la vez. Es una ventana y no un tope:
	// un repositorio con doscientas ramas se recorre con el filtro, y con las
	// flechas lo que se desplaza es la ventana, no el filtro.
	retargetRows = 12
	// retargetMinRows es el suelo de la ventana, para que en una terminal diminuta
	// se vean un par de ramas y no una caja de tres líneas.
	retargetMinRows = 3
	// retargetChrome son las líneas del popup que no son ramas: borde de arriba, la
	// de la base de la que se sale, la del filtro, la de la ayuda y el borde de
	// abajo.
	//
	// El filtro y la ayuda van en líneas propias y no en una sola porque juntas no
	// caben en un terminal estrecho, y lo que no cabe se va por la cola: en uno de
	// 52 columnas la ayuda perdía el "esc close", que es la única forma de cancelar.
	// Separadas, cada una cabe y no hay nada que perder.
	retargetChrome = 5
	// retargetMargin son las filas que el popup deja libres arriba y abajo, para
	// que se vea que es un overlay y no otra vista.
	retargetMargin = 2
	// retargetCurrentSuffix es lo que se le pone a la fila de la base actual. Va
	// como constante porque el ancho de la rama se calcula restándolo, y si el
	// rótulo y ese cálculo se escribieran en dos sitios, un cambio de uno
	// descuadraría el otro sin que nada lo notara.
	retargetCurrentSuffix = "  · current"
)

// retargetTimeout acota el listado de ramas. Son una o más páginas de un
// repositorio, y si tarda más que esto el problema es el forge: el popup lo dice,
// el ítem se queda como estaba y no se ha tocado nada.
const retargetTimeout = 30 * time.Second

// branchCacheTTL es cuánto se conserva el listado de un repositorio.
//
// Existe porque el popup se abre, se mira y se cierra con `esc` con facilidad, y
// paginar un repositorio de doscientas ramas en cada apertura es la clase de coste
// que hace que una acción termine sin usarse. Cinco minutos es un compromiso: lo
// bastante corto para que una rama recién creada aparezca sin pedir un refresco, y
// lo bastante largo para que volver al mismo repositorio no cueste nada.
const branchCacheTTL = 5 * time.Minute

// retargetState es la fase del popup.
type retargetState int

const (
	retargetClosed retargetState = iota
	// retargetListing pide las ramas del repositorio.
	retargetListing
	// retargetChoosing muestra el buscador y espera una rama.
	retargetChoosing
	// retargetConfirm enseña de qué base a cuál se va y espera el enter.
	retargetConfirm
)

// retargetPanel es el estado del popup. Guarda el ítem con el que se abrió y no lo
// vuelve a leer del cursor: entre la apertura y el enter un refresco puede haber
// movido la selección, y lo que hay que cambiar de base es lo que el usuario vio,
// no lo que ahora esté debajo del cursor.
type retargetPanel struct {
	state retargetState
	item  model.Item
	// all son las ramas del repositorio ya ordenadas; view son las que casan con
	// el filtro; win es la primera fila visible de la lista.
	all    []string
	view   []string
	win    int
	cursor int
	query  string
	// chosen es la rama señalada en la fase de confirmación.
	chosen string
	// errMsg es el motivo por el que no hay lista que enseñar. Vacío = cargando o
	// cargada.
	errMsg string
}

// repoKey identifica un repositorio para el caché de ramas. Forge y host entran
// porque el mismo owner/repo puede vivir en dos forges distintos.
type repoKey struct {
	forge   string
	host    string
	project string
}

// keyOf compone la clave de caché de un ítem.
func keyOf(it model.Item) repoKey {
	return repoKey{forge: it.Forge, host: it.Host, project: it.Ref.Project}
}

// branchCache es un listado de ramas con el momento en que se pidió.
type branchCache struct {
	names     []string
	fetchedAt time.Time
}

// ReviewLookup dice si un ítem tiene ya un review montado. Es un puerto opcional:
// sin él la TUI no puede avisar de que el worktree quedó con la base antigua, y
// lo único que se pierde es ese aviso.
type ReviewLookup interface {
	ActiveReview(it model.Item) (worktree.Worktree, bool)
}

// ReviewRemover borra el worktree del review activo de un ítem, solo si está
// limpio. Es un puerto opcional y separado de ReviewLookup: una capacidad que
// BORRA no puede heredar el contrato "solo lectura y degradable" de un lookup, y
// su ausencia debe ser explícita ("sin remover no hay auto-borrado, y el merge
// sigue igual"). reason explica por qué se conservó el worktree; err reserva el
// fallo de la operación.
type ReviewRemover interface {
	RemoveReview(ctx context.Context, it model.Item) (removed bool, reason string, err error)
}

// SetReviewRemover inyecta el removedor de reviews. nil lo deshabilita: el merge
// funciona igual y solo deja de auto-borrar el worktree.
func (m *Model) SetReviewRemover(r ReviewRemover) { m.reviewRemover = r }

// branchesMsg entrega el listado de ramas de un repositorio. errMsg lleva el
// motivo de un listado que no llegó; err queda para un fallo que no venga ya
// traducido en un warning del forge.
type branchesMsg struct {
	seq    int
	key    repoKey
	names  []string
	errMsg string
}

// SetReviewLookup inyecta el registro de reviews montados. nil lo deshabilita: el
// cambio de base sigue funcionando y solo deja de avisar del worktree desfasado.
func (m *Model) SetReviewLookup(l ReviewLookup) { m.reviewLookup = l }

// openRetarget abre el popup sobre el ítem seleccionado.
//
// Los guards son los de cualquier acción —ítem accionable, forge conocido y
// autenticado, sin otra acción en curso— y se comprueban antes de gastar el
// listado: abrir un popup que va a terminar en un aviso es peor que el aviso.
func (m Model) openRetarget() (tea.Model, tea.Cmd) {
	it, _, ok := m.canAction(forge.ActionRetarget)
	if !ok {
		return m, nil
	}

	m.disarmMerge()
	m.retarget = retargetPanel{state: retargetListing, item: it}

	// Con el listado en caché se abre al instante, sin esperar nada. Es el camino
	// habitual —corregir una base que se acaba de equivocar— y el que hace que
	// abrir y cerrar el popup no cueste.
	if names, ok := m.cachedBranches(keyOf(it)); ok {
		m.fillBranches(names)
		return m, nil
	}
	return m, m.fetchBranches(it)
}

// fetchBranches pide el listado de ramas en segundo plano. No bloquea la TUI: es
// una o más peticiones al forge, y bloquear el update loop se nota como un
// congelón, que es justo lo que un overlay no puede hacer.
//
// El resultado va al canal de eventos como cualquier otro trabajo, para que su
// entrega no dependa de que el usuario siga tocando teclas.
func (m *Model) fetchBranches(it model.Item) tea.Cmd {
	m.branchSeq++
	seq, appCtx, events := m.branchSeq, m.ctx, m.events
	a := m.byForge[it.Forge]
	key := keyOf(it)
	go func() {
		ctx, cancel := context.WithTimeout(appCtx, retargetTimeout)
		defer cancel()
		names, warns := a.Branches(ctx, it.Ref)
		msg := branchesMsg{seq: seq, key: key, names: names}
		if len(warns) > 0 {
			msg.errMsg = warnMsg(warns)
		}
		sendEvent(appCtx, events, msg)
	}()
	return nil
}

// warnMsg compone el motivo que enseña el popup a partir de los warnings del
// forge. Se usa el texto de la CLI y no uno inventado porque es el único que dice
// qué pasó: un "no se pudieron leer las ramas" de invención taparía tanto un 404
// como un token caducado, que piden acciones opuestas.
func warnMsg(warns []model.Warning) string {
	for _, w := range warns {
		if strings.TrimSpace(w.Msg) != "" {
			return w.Msg
		}
	}
	return "the branches could not be read"
}

// applyBranches recoge el listado. Un pedido obsoleto se descarta sin tocar nada:
// el popup se cerró, o se reabrió para otro ítem, y pintar lo que llegó del pedido
// anterior sería mostrar las ramas de otro repositorio.
func (m *Model) applyBranches(msg branchesMsg) {
	if msg.seq != m.branchSeq || m.retarget.state == retargetClosed {
		return
	}
	if msg.errMsg != "" {
		m.retarget.errMsg = msg.errMsg
		return
	}
	if len(msg.names) == 0 {
		m.retarget.errMsg = "the repository has no branches to choose from"
		return
	}
	m.storeBranches(msg.key, msg.names)
	m.fillBranches(msg.names)
}

// fillBranches pasa del listado al buscador: ordena, deduplica y deja el cursor
// arriba con la base actual marcada.
func (m *Model) fillBranches(names []string) {
	m.retarget.all = orderBranches(names, m.retarget.item.TargetBranch)
	m.retarget.cursor, m.retarget.win, m.retarget.query = 0, 0, ""
	m.retarget.view = filterBranches(m.retarget.all, "")
	m.retarget.state = retargetChoosing
}

// cachedBranches devuelve el listado cacheado de un repositorio si sigue vigente.
func (m *Model) cachedBranches(key repoKey) ([]string, bool) {
	entry, ok := m.branchCache[key]
	if !ok || time.Since(entry.fetchedAt) > branchCacheTTL {
		return nil, false
	}
	return entry.names, true
}

// storeBranches guarda el listado de un repositorio.
func (m *Model) storeBranches(key repoKey, names []string) {
	if m.branchCache == nil {
		m.branchCache = map[repoKey]branchCache{}
	}
	m.branchCache[key] = branchCache{names: names, fetchedAt: time.Now()}
}

// orderBranches deja la base actual la primera y el resto alfabético.
//
// Que la base actual salga la primera no es un capricho de orden: es la única fila
// que describe el punto de partida, y en un repositorio largo es la primera que se
// va de la ventana. El resto va alfabético porque es el único orden que se puede
// anticipar sin recorrerlo entero.
//
// De paso deduplica: un nombre repetido produce dos filas idénticas que parecen dos
// destinos, y elegir una u otra no cambia nada.
func orderBranches(names []string, base string) []string {
	seen := make(map[string]bool, len(names))
	rest := make([]string, 0, len(names))
	head := ""
	for _, name := range names {
		name = strings.TrimSpace(name)
		if name == "" || seen[name] {
			continue
		}
		seen[name] = true
		if name == base && head == "" {
			head = name
			continue
		}
		rest = append(rest, name)
	}
	sort.Strings(rest)
	if head == "" {
		return rest
	}
	return append([]string{head}, rest...)
}

// filterBranches deja las ramas que contienen el filtro, sin distinguir
// mayúsculas.
//
// El filtro va sobre el nombre entero y no sobre el último segmento: `fix` tiene
// que encontrar `fix/hunk-pane-argv`. Que no distinga mayúsculas es porque es una
// ayuda para recordar y no una escritura: el nombre que sale de aquí lo pone el
// forge, no el teclado, así que no depende de la búsqueda acertar las mayúsculas.
func filterBranches(all []string, query string) []string {
	q := strings.ToLower(strings.TrimSpace(query))
	if q == "" {
		return slices.Clone(all)
	}
	out := make([]string, 0, len(all))
	for _, name := range all {
		if strings.Contains(strings.ToLower(name), q) {
			out = append(out, name)
		}
	}
	return out
}

// applyQuery recalcula la vista tras cambiar el filtro y devuelve el cursor al
// principio, con la ventana en cero.
//
// Vuelve al principio siempre, y no solo cuando la lista cambia: escribir un
// carácter más con el cursor abajo dejaría seleccionada una fila que el filtro
// acaba de mover, y el `enter` de después aplicaría una base que el usuario ya no
// está viendo. Empezar de arriba es lo único que no sorprende, porque nada se
// aplica sin haberse leído, y cuesta una flecha.
func (m *Model) applyQuery() {
	m.retarget.view = filterBranches(m.retarget.all, m.retarget.query)
	m.retarget.cursor, m.retarget.win = 0, 0
}

// clampRetargetCursor mantiene el cursor dentro de la vista, que se encogió con
// el último filtro escrito.
func (m *Model) clampRetargetCursor() {
	if len(m.retarget.view) == 0 {
		m.retarget.cursor = 0
		return
	}
	m.retarget.cursor = min(max(0, m.retarget.cursor), len(m.retarget.view)-1)
}

// selectedBranch es la rama bajo el cursor, si la hay.
func (m Model) selectedBranch() (string, bool) {
	if m.retarget.cursor < 0 || m.retarget.cursor >= len(m.retarget.view) {
		return "", false
	}
	return m.retarget.view[m.retarget.cursor], true
}

// closeRetarget cierra el popup e invalida el listado en vuelo. No borra el caché:
// el listado de un repositorio no caduca por cerrar el popup.
func (m *Model) closeRetarget() {
	m.branchSeq++
	m.retarget = retargetPanel{}
}

// handleRetargetKey atiende el popup mientras está abierto. `q` y `ctrl+c` salen,
// como en el resto de la vista: cerrar con `esc` y salir con `q` son dos intenciones
// distintas.
//
// En la confirmación, en cambio, una tecla que no sea una elección no cierra el
// popup: se la come. El merge armado y el popup de simulación sí cancelan ante
// cualquier tecla, y allí tiene sentido porque están esperando un disparo. Aquí lo
// que hay es una línea que el usuario ya ha leído, y descartar la selección por una
// pulsación al vuelo perdería un trabajo de tres pulsaciones sin avisar. Cancelar es
// `esc`, que siempre está.
func (m Model) handleRetargetKey(msg tea.KeyPressMsg, key string) (tea.Model, tea.Cmd) {
	switch key {
	case "q", "ctrl+c":
		m.closeRetarget()
		m.cancel()
		return m, tea.Quit
	case "esc":
		// En la confirmación `esc` es un paso atrás y no un cierre: el error más
		// probable al confirmar es señalar la fila equivocada, y volver a la lista
		// lo deshace sin volver a pedir las ramas.
		if m.retarget.state == retargetConfirm {
			m.retarget.state = retargetChoosing
			return m, nil
		}
		m.closeRetarget()
		return m, nil
	}

	switch m.retarget.state {
	case retargetListing:
		// Mientras llegan las ramas no hay nada que elegir. `esc` ya salió por
		// arriba, así que cualquier otra tecla se consume y el popup sigue
		// esperando, con su spinner, a que el forge conteste.
		return m, nil

	case retargetConfirm:
		if key == "enter" {
			return m, m.startRetarget(m.retarget.chosen)
		}
		return m, nil

	default:
		return m.handleRetargetSearchKey(msg, key)
	}
}

// handleRetargetSearchKey atiende el buscador.
//
// `j` y `k` navegan solo con el filtro vacío; en cuanto hay algo escrito son dos
// letras más del filtro. Es la misma regla que aplica el filtro de una lista, y es
// lo que evita que las dos mitades se pisen: con el filtro vacío se navega con las
// teclas de siempre, y en cuanto se escribe se navega con las flechas, que nunca
// son texto. `ctrl+u` borra el filtro y devuelve las letras a su segundo oficio.
func (m Model) handleRetargetSearchKey(msg tea.KeyPressMsg, key string) (tea.Model, tea.Cmd) {
	switch key {
	case "enter":
		branch, ok := m.selectedBranch()
		if !ok {
			return m, nil
		}
		// Señalar la base que ya tiene es un no-op, y gastarlo en una llamada al
		// forge para que conteste "sin cambios" es la clase de coste que hace que
		// una acción parezca rota. Se corta aquí y no en el adapter porque es una
		// decisión de la vista, no del forge.
		if branch == m.retarget.item.TargetBranch {
			m.closeRetarget()
			m.setNotice("the target branch is already "+branch, levelInfo)
			return m, nil
		}
		m.retarget.chosen = branch
		m.retarget.state = retargetConfirm
		return m, nil

	case "up":
		m.moveRetargetCursor(-1)
		return m, nil
	case "down":
		m.moveRetargetCursor(1)
		return m, nil

	case "ctrl+u":
		if m.retarget.query != "" {
			m.retarget.query = ""
			m.applyQuery()
		}
		return m, nil

	case "backspace":
		// Se borra rune a rune y no por graphemes: se están borrando teclas, y una
		// clave de composición deshecha a medias es peor que un carácter de más que
		// el siguiente `backspace` quita.
		if r := []rune(m.retarget.query); len(r) > 0 {
			m.retarget.query = string(r[:len(r)-1])
			m.applyQuery()
		}
		return m, nil
	}

	if m.retarget.query == "" {
		switch key {
		case "k":
			m.moveRetargetCursor(-1)
			return m, nil
		case "j":
			m.moveRetargetCursor(1)
			return m, nil
		}
	}

	// `Text` viene vacío en las teclas especiales y en las combinaciones con
	// modificador, así que este es el filtro de lo que es de verdad una tecla de
	// escribir.
	if msg.Text != "" && !msg.Mod.Contains(tea.ModCtrl) {
		m.retarget.query += msg.Text
		m.applyQuery()
	}
	return m, nil
}

// moveRetargetCursor mueve el cursor una fila y desplaza la ventana para que la
// fila siga visible. Es el mismo contrato que el de la lista principal: mover sin
// resincronizar deja la selección fuera de la caja.
func (m *Model) moveRetargetCursor(delta int) {
	m.retarget.cursor += delta
	m.clampRetargetCursor()
	m.retarget.win = m.retargetWindow()
}

// retargetWindow es la primera fila visible de la lista de ramas. La calcula solo
// el movimiento, y no el render: filtro y flechas comparten así una sola cuenta en
// lugar de tener cada una la suya.
func (m *Model) retargetWindow() int {
	rows := m.retargetRows()
	win := m.retarget.win
	switch {
	case m.retarget.cursor < win:
		win = m.retarget.cursor
	case m.retarget.cursor >= win+rows:
		win = m.retarget.cursor - rows + 1
	}
	return max(0, min(win, max(0, len(m.retarget.view)-rows)))
}

// retargetVisible son las ramas que se pintan y el índice de la primera.
func (m Model) retargetVisible() ([]string, int) {
	rows := m.retargetRows()
	start := min(m.retarget.win, len(m.retarget.view))
	return m.retarget.view[start:][:min(rows, len(m.retarget.view)-start)], start
}

// retargetRows son las filas de rama que caben en el popup, acotadas por la altura
// de la terminal. Sin el recorte, un popup más alto que la pantalla se dibuja a
// medias y la fila elegida puede quedar en la parte que no se ve.
func (m Model) retargetRows() int {
	free := m.height - 2*retargetMargin - retargetChrome
	return min(retargetRows, max(retargetMinRows, free))
}

// retargetBoxWidth es el ancho exterior del popup. Solo el ancho: la altura la
// calcula quien compone la caja, con las líneas que de verdad tiene, y devolverla
// aquí era un número que nadie leía (las tres cajas del popup la descartaban con
// `_`), que es la clase de valor que luego se permite mutar sin que nada se entere.
func (m Model) retargetBoxWidth() int {
	return min(m.contentWidth(), retargetChooserWidth)
}

// retargetOverlay compone la caja del popup, o dice que no hay nada que pintar.
func (m Model) retargetOverlay() (string, bool) {
	switch m.retarget.state {
	case retargetListing:
		return m.retargetBusyBox(), true
	case retargetChoosing:
		return m.retargetSearchBox(), true
	case retargetConfirm:
		return m.retargetConfirmBox(), true
	default:
		return "", false
	}
}

// retargetBusyBox es la fase de carga. No dice un porcentaje porque no hay nada
// que medir: la petición va al forge y no se sabe cuánto queda. untruecer un
// progreso inventado es peor que no medir.
//
// Si el listado vino con un motivo, lo que se enseña es el motivo y el spinner
// desaparece. Se queda en la misma fase a propósito —no hay lista que elegir— y
// por eso la única salida es `esc`: sin ella, un fallo del forge dejaría el popup
// esperando algo que no va a llegar.
func (m Model) retargetBusyBox() string {
	width := m.retargetBoxWidth()
	if m.retarget.errMsg != "" {
		body := styleError.Render(m.retarget.errMsg) + "\n" + styleHint.Render("esc close")
		return borderedBox(" retarget "+refLabel(m.retarget.item), body, width)
	}
	body := m.spinner.View() + styleInfo.Render(" reading the branches…")
	body += "\n" + styleHint.Render("esc close")
	return borderedBox(" retarget "+refLabel(m.retarget.item), body, width)
}

// retargetSearchBox es el buscador: de dónde se sale, qué se ha escrito y las
// ramas que quedan.
//
// La base actual se marca con un punto y no con el cursor porque son dos señales
// distintas: el cursor es la fila que `enter` elige, y la fila elegida nunca es la
// de partida —esa se corta antes de llegar aquí—, así que si compartieran símbolo
// el popup no diría nunca qué es lo que está elegido.
func (m Model) retargetSearchBox() string {
	width := m.retargetBoxWidth()
	it := m.retarget.item
	inner := max(8, width-2)

	from := it.TargetBranch
	if from == "" {
		from = "unknown"
	}
	header := styleRef.Render("from ") + styleCount.Render(from) + styleDim.Render("  ·  ") +
		styleDim.Render(branchCountLabel(m))

	body := make([]string, 0, m.retargetRows()+2)
	switch {
	case m.retarget.errMsg != "":
		body = append(body, styleError.Render(m.retarget.errMsg))
	case len(m.retarget.view) == 0:
		body = append(body, styleEmpty.Render("no branch matches the filter"))
	default:
		names, start := m.retargetVisible()
		for i, name := range names {
			marker := "  "
			if start+i == m.retarget.cursor {
				marker = styleCursor.Render("▸ ")
			}
			suffix := ""
			room := inner - ansi.StringWidth(marker)
			if name == it.TargetBranch {
				suffix = styleDim.Render(retargetCurrentSuffix)
				room -= ansi.StringWidth(retargetCurrentSuffix)
			}
			body = append(body, marker+styleRef.Render(padRight(ansi.Truncate(name, room, ""), room))+suffix)
		}
	}
	body = append(body, "")

	filter := styleDim.Render("filter ")
	if m.retarget.query == "" {
		filter += styleEmpty.Render("(type to search)")
	} else {
		filter += styleOK.Render(m.retarget.query)
	}
	help := styleHint.Render("↑↓ move · enter choose · esc close")

	return borderedBox(" retarget "+refLabel(it),
		strings.Join(append([]string{header}, append(body, filter, help)...), "\n"), width)
}

// branchCountLabel cuenta las ramas que se ven y las que hay. Con el filtro
// apagado dice solo cuántas hay; con el filtro puesto dice cuántas casan, que es
// la diferencia entre "el repositorio tiene dos" y "de doscientas, estas dos".
func branchCountLabel(m Model) string {
	total := len(m.retarget.all)
	if m.retarget.query == "" {
		return pluralBranches(total)
	}
	return fmt.Sprintf("%d of %s match", len(m.retarget.view), pluralBranches(total))
}

// pluralBranches dice "3 branches" o "1 branch", porque un popup que dice "1
// branches" hace dudar de la cuenta.
func pluralBranches(n int) string {
	if n == 1 {
		return "1 branch"
	}
	return fmt.Sprintf("%d branches", n)
}

// retargetConfirmBox es la confirmación: de qué base a cuál, y qué va a cambiar.
//
// No es un adorno. Mover la base rehace el diff, la mergeabilidad y el CI del
// ítem, y lo que se hubiera aprobado antes pasa a compararse contra otra cosa. Por
// eso la línea del medio dice qué deja de ser verdad, y no solo qué teclas hay.
func (m Model) retargetConfirmBox() string {
	width := m.retargetBoxWidth()
	from := m.retarget.item.TargetBranch
	if from == "" {
		from = "unknown"
	}
	flow := styleCount.Render(padRight(from, max(1, (width-6)/2))) +
		styleDim.Render(" → ") + styleOK.Render(m.retarget.chosen)

	// El texto va corto a propósito. Una frase que nombra la rama y además explica
	// lo que pasa se sale de la caja en un terminal normal, y una línea cortada a
	// media palabra se lee como un fallo de la caja y no como un mensaje.
	body := flow + "\n\n" +
		styleDim.Render("the diff, the checks and the mergeability are recomputed") + "\n" +
		styleHint.Render("enter apply · esc back")
	return borderedBox(" retarget "+refLabel(m.retarget.item), body, width)
}

// startRetarget lanza el cambio de base sobre el ítem que el popup confirmó.
//
// El guard corre sobre el ítem del panel y no sobre lo que hay bajo el cursor: si
// un refresco lo movió mientras el popup estaba abierto, el popup sigue siendo la
// pregunta que se estaba contestando.
func (m *Model) startRetarget(branch string) tea.Cmd {
	it := m.retarget.item
	a, ok := m.canActionOn(forge.ActionRetarget, it)
	if !ok {
		m.closeRetarget()
		return nil
	}
	from := it.TargetBranch
	m.closeRetarget()
	return m.launchAction(forge.ActionRetarget, it, a,
		retargetProgressNotice(from, branch),
		func(ctx context.Context) forge.Outcome {
			out := forge.RunRetarget(ctx, a, it.Ref, it.Number, branch)
			out.FromBase = from
			return out
		})
}

// retargetProgressNotice describe el cambio en curso nombrando las dos ramas: es lo
// único que distingue un retarget de otro entre una lista de avisos iguales, y es
// lo que el usuario tiene que poder leer mientras espera.
func retargetProgressNotice(from, to string) string {
	if from == "" {
		return string(forge.ActionRetarget) + " (→ " + to + ") en curso…"
	}
	return string(forge.ActionRetarget) + " (" + from + " → " + to + ") en curso…"
}

// staleReviewNotice avisa de que el worktree del review quedó con la base que
// tenía el ítem antes del cambio.
//
// Solo avisa: el worktree es del usuario y puede tener cambios sin commitear, así
// que rebasarlo o rehacerlo desde aquí escribiría donde no se ha pedido. Lo que sí
// se puede es decir la verdad —ese review se montó contra otra base— y que la
// decisión de remontarlo sea del operador.
func (m Model) staleReviewNotice(it model.Item) string {
	if m.reviewLookup == nil || it.Number == 0 {
		return ""
	}
	wt, ok := m.reviewLookup.ActiveReview(it)
	if !ok || wt.Path == "" {
		return ""
	}
	return "the mounted review still has the old base: mount the review again to pick it up"
}
