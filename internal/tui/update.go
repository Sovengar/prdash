// Update y View del inbox.
package tui

import (
	"context"
	"fmt"
	"strings"
	"time"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"

	"prdash/internal/forge"
	"prdash/internal/forge/model"
	"prdash/internal/review/executor"
	"prdash/internal/state"
)

// Update procesa mensajes: eventos de los forges, teclas, tick y resize.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		return m, nil

	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd

	case authMsg:
		if msg.cycle != m.cycle {
			// Ciclo obsoleto: se descarta su dato pero SIEMPRE se rearma la
			// bomba (si no, se pierden lectores del canal y el refresco muere).
			return m.withPump(nil)
		}
		if st := m.statuses[msg.forge]; st != nil {
			st.auth = msg.auth
		}
		return m.withPump(nil)

	case pageMsg:
		if msg.cycle != m.cycle {
			return m.withPump(nil)
		}
		m.applyPage(msg)
		return m.withPump(nil)

	case forgeDoneMsg:
		if msg.cycle != m.cycle {
			return m.withPump(nil)
		}
		if st := m.statuses[msg.forge]; st != nil {
			st.loading = false
		}
		return m.withPump(nil)

	case refreshDoneMsg:
		if msg.cycle != m.cycle {
			// Ciclo obsoleto (no debería ocurrir: no se solapan ciclos). No
			// toca datos, ni `loading`, ni la cadena de ticks; solo rearma la
			// bomba porque consumió un evento del canal.
			return m.withPump(nil)
		}
		m.loading = false
		m.lastRefresh = time.Now()
		m.recomputeBackoff()
		m.saveSnapshot()
		return m.withPump(m.armTick())

	case tickMsg:
		// El tick pendiente acaba de dispararse.
		m.tickPending = false
		if m.paused() {
			return m, m.armTick()
		}
		updated, cmd := m.beginRefresh()
		return updated, cmd

	case actionMsg:
		m.applyAction(msg.outcome, msg.cycle)
		return m.withPump(nil)

	case notifyMsg:
		m.setNotice(msg.text, msg.level)
		return m, nil

	case toastTickMsg:
		// Poda los avisos caducados y rearma el tick. No toca el canal de
		// eventos: es un reloj, no un lector.
		m.toast.update()
		return m, tickToast()

	case mountMsg:
		m.mountBusy = false
		m.applyMount(msg.result, msg.err)
		return m.withPump(nil)

	case tea.KeyPressMsg:
		return m.handleKey(msg)
	}
	return m, nil
}

// withPump rearma la bomba de eventos tras procesar un evento del canal: el
// mensaje consumió un lector y aquí se arma exactamente uno de nuevo.
func (m Model) withPump(cmd tea.Cmd) (tea.Model, tea.Cmd) {
	m.releaseReader()
	return m, tea.Batch(cmd, m.armReader())
}

// releaseReader marca que un lector del canal terminó (un evento entregado).
func (m *Model) releaseReader() {
	if m.readers > 0 {
		m.readers--
	}
}

// applyAction vuelca el resultado de una acción en el estado: refresca el ítem,
// registra la denegación por permisos o avisa del conflicto.
//
// Política de ciclo: el `Item` releído es el estado más reciente que existe del
// forge (se lee DESPUÉS de la acción), así que se aplica siempre, aunque el
// ciclo de refresco haya avanzado. Descartarlo revertiría el ítem a un estado
// anterior. El ciclo se registra por ítem para que una página capturada antes
// de la acción no lo pise (ver reconcileFirstPage).
func (m *Model) applyAction(out forge.Outcome, cycle int) {
	m.actionBusy = false
	if out.HasItem {
		m.actionCycle[out.Item.ID()] = cycle
		m.applyItemUpdate(out.Item)
	}
	switch {
	case out.Perm:
		m.denied[out.ID] = out.Msg
		m.setNotice(string(out.Kind)+" disabled: "+out.Msg, levelWarn)
	case out.Conflict:
		m.setNotice("forge conflict: "+out.Msg, levelError)
	case out.OK:
		m.setNotice(actionDoneNotice(out), levelOK)
	default:
		m.setNotice("error: "+out.Msg, levelError)
	}
}

// handleKey enruta las teclas: navegación y acciones configurables. No hay vista
// a pantalla completa que abrir ni cerrar: la ficha del ítem vive siempre en el
// panel inferior, así que todas las teclas sirven en todo momento.
func (m Model) handleKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	key := msg.String()

	// Merge es la única acción con dos tiempos. Mientras está armada solo
	// cuentan la tecla de modo, `esc` y salir; cualquier otra tecla desarma y se
	// comporta como si el merge no se hubiera pulsado, para que un `m` a
	// destiempo no deje la vista esperando una segunda pulsación.
	if m.mergeArmed {
		return m.handleMergeArmed(msg, key)
	}

	switch key {
	case "q", "ctrl+c":
		m.cancel()
		return m, tea.Quit
	case "up", "k":
		m.moveCursor(m.cursor - 1)
		return m, nil
	case "down", "j":
		m.moveCursor(m.cursor + 1)
		return m, nil
	case "home":
		m.goTop()
		return m, nil
	case "end":
		m.moveCursor(len(m.rows()))
		return m, nil
	case "pgup":
		m.pageBy(-m.pageRows())
		return m, nil
	case "pgdown":
		m.pageBy(m.pageRows())
		return m, nil
	}

	switch m.cfg.ActionForKey(key) {
	case "section-next":
		m.gotoNextSection()
		return m, nil
	case "refresh":
		return m.startRefresh()
	case "approve":
		return m, m.startAction(forge.ActionApprove, "")
	case "merge":
		return m.armMerge()
	case "mount-review":
		return m.startMount()
	case "open-browser":
		it, ok := m.selected()
		if !ok || it.URL == "" {
			m.setNotice("no URL to open", levelWarn)
			return m, nil
		}
		return m, m.openBrowserCmd(it.URL)
	case "quit":
		m.cancel()
		return m, tea.Quit
	}
	return m, nil
}

// armMerge es la primera pulsación de merge: no ejecuta nada, solo deja la vista
// pidiendo la segunda tecla. Los guards se comprueban aquí y no al confirmar
// para no armar un merge que ya se sabe inválido (ítem cerrado, forge sin
// autenticar, acción en curso).
func (m Model) armMerge() (tea.Model, tea.Cmd) {
	it, _, ok := m.canAction(forge.ActionMerge)
	if !ok {
		return m, nil
	}
	m.mergeArmed = true
	m.mergeArmedID = it.ID()
	return m, nil
}

// handleMergeArmed atiende la segunda pulsación: la tecla ES el modo. No hay modo
// por defecto, así que la Confirmación y la elección son el mismo gesto y no
// existe un camino que mergee con una estrategia que el usuario no ha nombrado.
//
// `esc` cancela. `q` y `ctrl+c` salen, como en el resto de la vista: cancelar
// con `esc` y salir con `q` son dos intenciones distintas y colapsarlas en una
// haría que `q` dejara de cerrar la TUI.
func (m Model) handleMergeArmed(msg tea.KeyPressMsg, key string) (tea.Model, tea.Cmd) {
	var mode forge.MergeMode
	switch key {
	case "m":
		mode = forge.MergeCommit
	case "r":
		mode = forge.Rebase
	case "s":
		mode = forge.Squash
	case "esc":
		m.disarmMerge()
		m.setNotice("merge cancelled", levelInfo)
		return m, nil
	case "q", "ctrl+c":
		m.disarmMerge()
		m.cancel()
		return m, tea.Quit
	default:
		m.disarmMerge()
		return m.handleKey(msg)
	}

	// El cursor puede haberse movido por un refresco entre el armado y la
	// confirmación. Si ya no está el mismo ítem, el merge sale sobre lo que el
	// usuario confirmó o no sale.
	it, ok := m.selected()
	if !ok || it.ID() != m.mergeArmedID {
		m.disarmMerge()
		m.setNotice("the selected item changed: press merge again", levelWarn)
		return m, nil
	}
	m.disarmMerge()
	return m, m.startAction(forge.ActionMerge, mode)
}

// disarmMerge limpia el estado de armado.
func (m *Model) disarmMerge() {
	m.mergeArmed = false
	m.mergeArmedID = model.ID{}
}

// startRefresh arranca un ciclo de refresco si no hay uno en vuelo (no se
// solapan ciclos). No arma un lector del canal: el refresco local no consume
// eventos del canal, así que la bomba sigue con su único lector.
func (m Model) startRefresh() (tea.Model, tea.Cmd) {
	if m.loading {
		m.setNotice("refresh in progress", levelInfo)
		return m, nil
	}
	updated, cmd := m.beginRefresh()
	return updated, cmd
}

// canAction comprueba los guards de disponibilidad de una acción sobre el ítem
// seleccionado y, si alguno falla, deja el aviso puesto. Lo comparten el armado
// de merge y la ejecución: armar una acción que después no se puede ejecutar
// dejaría al usuario con una confirmación que solo puede terminar en decepción.
func (m *Model) canAction(kind forge.ActionKind) (model.Item, forge.Adapter, bool) {
	it, ok := m.selected()
	if !ok {
		m.setNotice("select an item first", levelWarn)
		return model.Item{}, nil, false
	}
	a := m.byForge[it.Forge]
	if a == nil {
		m.setNotice("unknown forge: "+it.Forge, levelError)
		return model.Item{}, nil, false
	}
	if st := m.statuses[it.Forge]; st != nil && !st.auth.OK {
		m.setNotice("action disabled: "+it.Forge+" is not authenticated", levelWarn)
		return model.Item{}, nil, false
	}
	if reason := m.denied[it.ID()]; reason != "" {
		m.setNotice(string(kind)+" disabled: "+reason, levelWarn)
		return model.Item{}, nil, false
	}
	// Aprobar lo propio no lo admite ningún forge: se corta aquí, antes de
	// gastar la llamada a la CLI y su relectura, y no espera al rechazo. El
	// motivo ya dice qué ha pasado, así que no lleva prefijo.
	if reason := m.selfDenied[it.ID()]; reason != "" && kind == forge.ActionApprove {
		m.setNotice(reason, levelWarn)
		return model.Item{}, nil, false
	}
	if m.actionBusy {
		m.setNotice("an action is already running", levelWarn)
		return model.Item{}, nil, false
	}
	if ok, reason := state.Actionable(it); !ok {
		m.setNotice(reason, levelWarn)
		return model.Item{}, nil, false
	}
	return it, a, true
}

// actionDoneNotice confirma la acción. Merge dice con qué estrategia se
// integró: es el dato que decide si el resultado es el que el usuario quería, y
// sin él un "merge ok" no dice nada de qué se hizo.
func actionDoneNotice(out forge.Outcome) string {
	if out.Kind == forge.ActionMerge {
		return string(out.Kind) + " (" + out.Mode.Label() + ") ok"
	}
	return string(out.Kind) + " ok"
}

// startAction lanza una acción rápida sobre el ítem seleccionado tras los
// guards de disponibilidad. mode solo viaja con ActionMerge: approve lo ignora,
// y el aviso lo dice para que el resultado diga con qué estrategia se integró.
func (m *Model) startAction(kind forge.ActionKind, mode forge.MergeMode) tea.Cmd {
	it, a, ok := m.canAction(kind)
	if !ok {
		return nil
	}

	m.actionBusy = true
	m.setNotice(actionProgressNotice(kind, mode), levelInfo)

	appCtx := m.ctx
	events := m.events
	ref, number := it.Ref, it.Number
	cycle := m.cycle
	go func() {
		ctx, cancel := context.WithTimeout(appCtx, actionTimeout)
		defer cancel()
		sendEvent(appCtx, events, actionMsg{cycle: cycle, outcome: forge.RunAction(ctx, a, kind, ref, number, mode)})
	}()
	return nil
}

// actionProgressNotice describe la acción en curso. El modo entra solo en merge:
// es lo que el usuario tiene que poder leer mientras espera, porque decide si
// el resultado le va a gustar.
func actionProgressNotice(kind forge.ActionKind, mode forge.MergeMode) string {
	if kind == forge.ActionMerge {
		return string(kind) + " (" + mode.Label() + ") en curso…"
	}
	return string(kind) + " en curso…"
}

// startMount lanza el montaje del review del ítem seleccionado en segundo
// plano. Sin montador inyectado informa que la acción requiere Herdr: es la
// degradación fuera de Herdr, que no cuelga la TUI ni lanza procesos.
func (m Model) startMount() (tea.Model, tea.Cmd) {
	it, ok := m.selected()
	if !ok {
		m.setNotice("select an item first", levelWarn)
		return m, nil
	}
	if m.mounter == nil {
		m.setNotice("mounting a review requires Herdr", levelWarn)
		return m, nil
	}
	if m.mountBusy {
		m.setNotice("a review mount is already running", levelWarn)
		return m, nil
	}
	m.mountBusy = true
	m.setNotice("mounting review…", levelInfo)

	appCtx := m.ctx
	events := m.events
	mounter := m.mounter
	go func() {
		ctx, cancel := context.WithTimeout(appCtx, mountTimeout)
		defer cancel()
		res, err := mounter.Mount(ctx, it)
		sendEvent(appCtx, events, mountMsg{result: res, err: err})
	}()
	return m, nil
}

// applyMount vuelca el resultado del montaje en el aviso de la cabecera.
func (m *Model) applyMount(res executor.Result, err error) {
	text, level := mountNotice(res, err)
	m.setNotice(text, level)
}

// mountNotice compone el aviso del montaje: error, layout montado o worktree
// montado con el layout pendiente de Herdr.
func mountNotice(res executor.Result, err error) (string, noticeLevel) {
	if err != nil {
		return "could not mount review: " + err.Error(), levelError
	}
	if res.Herdr {
		return fmt.Sprintf("review mounted: %d panes in %d tabs, %s", res.Plan.PaneCount(), len(res.Plan.Tabs), res.Worktree.Path), levelOK
	}
	return "the review layout requires Herdr; the worktree was mounted at " + res.Worktree.Path, levelWarn
}

// moveCursor mueve el cursor a una fila (fuera de rango se acota) y desplaza la
// ventana para que la fila siga visible. Todo el movimiento pasa por aquí: si el
// cursor se moviera sin syncScroll, la fila podría quedarse fuera de la ventana
// con la selección en otra parte de la pantalla.
func (m *Model) moveCursor(row int) {
	m.cursor = row
	m.clampCursor()
	m.syncScroll()
}

// goTop lleva el cursor a la primera fila y la ventana al principio de la
// lista. No basta con mover el cursor: el auto-scroll pondría la fila bajo el
// borde superior, dejando el título de la sección y el header de columnas fuera
// de la pantalla.
func (m *Model) goTop() {
	m.cursor = 0
	m.scroll = 0
}

// pageBy mueve cursor y ventana a la vez, una ventana cada uno, para que pgup y
// pgdown lean como un salto de página: el ítem seleccionado conserva su
// posición en la pantalla y la lista se desplaza entera. Mover solo el cursor
// dejaría la lista casi quieta y el panel de detalle saltando de ítem en ítem.
func (m *Model) pageBy(delta int) {
	m.cursor += delta
	m.scroll += delta
	m.clampCursor()
	m.syncScroll()
}

// pageRows es el salto de pgup/pgdn: una ventana de lista, para que la tecla
// avance justo lo que se ve. Sin altura conocida, una media docena de filas.
func (m *Model) pageRows() int {
	view := m.layout().bodyLines
	if view <= 0 {
		return 6
	}
	return view
}

// gotoNextSection mueve el cursor al primer ítem de la siguiente sección con
// contenido.
func (m *Model) gotoNextSection() {
	sections := m.inbox.Sections
	if len(sections) == 0 {
		return
	}
	cur := m.sectionIndexAtCursor()
	for step := 1; step <= len(sections); step++ {
		idx := (cur + step) % len(sections)
		if len(sections[idx].Items) > 0 {
			m.moveCursor(m.sectionOffsets()[idx])
			return
		}
	}
}

// sectionOffsets devuelve el índice de la primera fila de cada sección.
func (m *Model) sectionOffsets() []int {
	offsets := make([]int, len(m.inbox.Sections))
	acc := 0
	for i, s := range m.inbox.Sections {
		offsets[i] = acc
		acc += len(s.Items)
	}
	return offsets
}

// sectionIndexAtCursor devuelve la sección en la que está el cursor.
func (m *Model) sectionIndexAtCursor() int {
	idx := 0
	for i, off := range m.sectionOffsets() {
		if m.cursor >= off {
			idx = i
		} else {
			break
		}
	}
	return idx
}

// View compose la pantalla: el inbox partido en lista y panel de detalle. El
// panel se queda con el 40% inferior, así que no hay una segunda vista a la que
// saltar para leer más.
func (m Model) View() tea.View {
	var v view
	lay := m.layout()
	it, ok := m.selected()
	v = m.compose(lay, m.listSection(lay), m.detailSection(it, ok, lay.detailLines))
	// Los avisos van superpuestos abajo a la derecha: la vista de fondo no se
	// vuelve a componer, solo se recorta por donde hace falta. Solo aterrizan en
	// el interior de las cajas, así que no pisan ningún borde.
	if toasts := m.toast.blocks(m.contentWidth()); len(toasts) > 0 {
		v.text = overlayToasts(v.text, toasts, m.contentWidth(), v.rows)
	}
	out := tea.NewView(v.text)
	out.AltScreen = true
	return out
}

// renderItem pinta una fila de ítem con el cursor delante si está seleccionada.
func (m *Model) renderItem(it model.Item, sec model.Section, lay refLayout, selected bool, inner int) string {
	prefix := "  "
	if selected {
		prefix = styleCursor.Render("▸ ")
	}
	return prefix + renderCells(itemCells(it, sec, m.viewerLogin(it.Forge), lay), lay, inner)
}

// contentWidth es el ancho interior de las cajas: el de la terminal menos los dos
// bordes. Es el ancho con el que se maquetan la lista, el detalle y los atajos.
func (m *Model) contentWidth() int {
	return max(38, m.outerWidth()-2)
}

// forgesStatusLine muestra la última actualización y el estado de cada forge.
func (m *Model) forgesStatusLine(now time.Time) string {
	names := m.sortedForgeNames()

	parts := make([]string, 0, len(names))
	for _, name := range names {
		st := m.statuses[name]
		mark := "✓"
		if !st.auth.OK {
			mark = "✗"
		}
		label := name + " " + mark + " " + lastRefreshLabel(st.updatedAt, now)
		if st.loading {
			label += "…"
		}
		parts = append(parts, label)
	}
	return styleCount.Render(strings.Join(parts, " · "))
}
