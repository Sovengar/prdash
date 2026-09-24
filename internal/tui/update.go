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
		if msg.cycle == m.cycle {
			m.lastRefresh = time.Now()
			m.recomputeBackoff()
			m.saveSnapshot()
		}
		// Cualquier fin de ciclo baja la carga y reprograma el tick: un
		// refreshDoneMsg obsoleto no debe dejar `loading` atascado (pausaría el
		// auto-refresco para siempre).
		m.loading = false
		return m.withPump(m.tickCmd())

	case tickMsg:
		if m.paused() {
			return m, m.tickCmd() // reprograma sin refrescar
		}
		updated, cmd := m.beginRefresh()
		return updated, tea.Batch(cmd, waitForEvent(m.events))

	case actionMsg:
		m.applyAction(msg.outcome, msg.cycle)
		return m.withPump(nil)

	case notifyMsg:
		m.setNotice(msg.text, msg.level)
		return m, nil

	case tea.KeyPressMsg:
		return m.handleKey(msg)
	}
	return m, nil
}

// withPump rearma la bomba de eventos tras procesar un evento del canal.
func (m Model) withPump(cmd tea.Cmd) (tea.Model, tea.Cmd) {
	return m, tea.Batch(cmd, waitForEvent(m.events))
}

// applyAction vuelca el resultado de una acción en el estado: refresca el ítem,
// registra la denegación por permisos o avisa del conflicto.
//
// Política de ciclo: el `Item` releído es el estado más reciente que existe del
// forge (se lee DESPUÉS de la acción), así que se aplica siempre, aunque el
// ciclo de refresco haya avanzado. Descartarlo revertiría el ítem a un estado
// anterior; el ciclo solo se registra para diagnóstico.
func (m *Model) applyAction(out forge.Outcome, cycle int) {
	_ = cycle
	m.actionBusy = false
	if out.HasItem {
		m.applyItemUpdate(out.Item)
	}
	switch {
	case out.Perm:
		m.denied[out.ID] = out.Msg
		m.setNotice(string(out.Kind)+" deshabilitado: "+out.Msg, levelWarn)
	case out.Conflict:
		m.setNotice("conflicto en el forge: "+out.Msg, levelError)
	case out.OK:
		m.setNotice(string(out.Kind)+" ok", levelOK)
	default:
		m.setNotice("error: "+out.Msg, levelError)
	}
}

// handleKey enruta las teclas: detalle, navegación y acciones configurables.
func (m Model) handleKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	key := msg.String()

	if m.detailOpen {
		return m.handleDetailKey(key)
	}

	switch key {
	case "q", "ctrl+c":
		m.cancel()
		return m, tea.Quit
	case "up", "k":
		m.cursor = max(0, m.cursor-1)
		return m, nil
	case "down", "j":
		m.cursor = min(m.cursor+1, max(0, len(m.rows())-1))
		return m, nil
	case "home":
		m.cursor = 0
		return m, nil
	case "end":
		m.cursor = max(0, len(m.rows())-1)
		return m, nil
	case "tab":
		m.gotoNextSection()
		return m, nil
	}

	switch m.cfg.ActionForKey(key) {
	case "refresh":
		updated, cmd := m.beginRefresh()
		return updated, tea.Batch(cmd, waitForEvent(m.events))
	case "detail":
		m.openDetail()
		return m, nil
	case "approve":
		return m, m.startAction(forge.ActionApprove)
	case "merge":
		return m, m.startAction(forge.ActionMerge)
	case "mount-review":
		m.setNotice("montar review requiere Herdr (etapa posterior)", levelWarn)
		return m, nil
	case "open-browser":
		it, ok := m.selected()
		if !ok || it.URL == "" {
			m.setNotice("no hay URL que abrir", levelWarn)
			return m, nil
		}
		return m, m.openBrowserCmd(it.URL)
	case "quit":
		m.cancel()
		return m, tea.Quit
	}
	return m, nil
}

// handleDetailKey gestiona las teclas mientras el detalle está abierto. Volver
// no toca el cursor: la selección del inbox se conserva.
func (m Model) handleDetailKey(key string) (tea.Model, tea.Cmd) {
	switch key {
	case "esc", "q", "ctrl+c":
		m.detailOpen = false
		return m, nil
	}
	switch m.cfg.ActionForKey(key) {
	case "detail":
		m.detailOpen = false
		return m, nil
	case "quit":
		m.cancel()
		return m, tea.Quit
	case "refresh":
		updated, cmd := m.beginRefresh()
		return updated, tea.Batch(cmd, waitForEvent(m.events))
	case "approve":
		return m, m.startAction(forge.ActionApprove)
	case "merge":
		return m, m.startAction(forge.ActionMerge)
	}
	return m, nil
}

// openDetail abre el detalle del ítem seleccionado sin mover el cursor. Guarda
// solo su identidad: el contenido se deriva del estado vivo (liveDetail).
func (m *Model) openDetail() {
	it, ok := m.selected()
	if !ok {
		return
	}
	m.detailID = it.ID()
	m.detailItem = it
	m.detailOpen = true
}

// startAction lanza una acción rápida sobre el ítem seleccionado tras los
// guards de disponibilidad.
func (m *Model) startAction(kind forge.ActionKind) tea.Cmd {
	it, ok := m.selected()
	if !ok {
		m.setNotice("selecciona un ítem", levelWarn)
		return nil
	}
	a := m.byForge[it.Forge]
	if a == nil {
		m.setNotice("forge desconocido: "+it.Forge, levelError)
		return nil
	}
	if st := m.statuses[it.Forge]; st != nil && !st.auth.OK {
		m.setNotice("acción deshabilitada: "+it.Forge+" sin autenticar", levelWarn)
		return nil
	}
	if reason := m.denied[it.ID()]; reason != "" {
		m.setNotice(string(kind)+" deshabilitado: "+reason, levelWarn)
		return nil
	}
	if m.actionBusy {
		m.setNotice("ya hay una acción en curso", levelWarn)
		return nil
	}
	if ok, reason := state.Actionable(it); !ok {
		m.setNotice(reason, levelWarn)
		return nil
	}

	m.actionBusy = true
	m.setNotice(string(kind)+" en curso…", levelInfo)

	appCtx := m.ctx
	events := m.events
	ref, number := it.Ref, it.Number
	cycle := m.cycle
	go func() {
		ctx, cancel := context.WithTimeout(appCtx, actionTimeout)
		defer cancel()
		sendEvent(appCtx, events, actionMsg{cycle: cycle, outcome: forge.RunAction(ctx, a, kind, ref, number)})
	}()
	return nil
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
			m.cursor = m.sectionOffsets()[idx]
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

// View compone la pantalla: detalle o inbox.
func (m Model) View() tea.View {
	content := m.renderInbox()
	if m.detailOpen {
		content = m.renderDetail()
	}
	v := tea.NewView(content)
	v.AltScreen = true
	return v
}

// renderInbox pinta la cabecera, las tres secciones y los hints.
func (m *Model) renderInbox() string {
	var b strings.Builder
	inner := m.contentWidth()

	b.WriteString(styleHeader.Render("prdash"))
	if m.loading {
		b.WriteString(" " + m.spinner.View() + styleCount.Render(" refreshing…"))
	}
	b.WriteString("  " + m.forgesStatusLine(time.Now()))
	b.WriteString("\n")
	if m.notice != "" {
		b.WriteString(styleNotice(m.level).Render("  "+m.notice) + "\n")
	}
	b.WriteString("\n")

	row := 0
	for _, sec := range m.inbox.Sections {
		problems := m.sectionProblems(sec.Kind)
		header := fmt.Sprintf("%s (%d)", sec.Kind.String(), len(sec.Items))
		if m.sectionLoadingMore(sec.Kind) {
			header += " · cargando más…"
		}
		b.WriteString(styleHeader.Render(header))
		b.WriteString("\n")

		for _, p := range problems {
			b.WriteString("  " + styleWarn.Render("⚠ "+p) + "\n")
		}

		switch {
		case len(sec.Items) > 0:
			b.WriteString("  " + headerLine(inner-2) + "\n")
			for _, it := range sec.Items {
				b.WriteString(m.renderItem(it, row == m.cursor, inner-2))
				b.WriteString("\n")
				row++
			}
		case len(problems) == 0:
			b.WriteString("  " + styleEmpty.Render("(vacío)") + "\n")
		}
		b.WriteString("\n")
	}

	b.WriteString(styleHint.Render(m.hintLine()))
	return b.String()
}

// renderItem pinta una fila de ítem con el cursor delante si está seleccionada.
func (m *Model) renderItem(it model.Item, selected bool, inner int) string {
	prefix := "  "
	if selected {
		prefix = styleCursor.Render("▸ ")
	}
	return prefix + renderCells(itemCells(it), inner)
}

// contentWidth es el ancho útil para las tablas.
func (m *Model) contentWidth() int {
	if m.width <= 0 {
		return 120
	}
	return max(40, m.width-4)
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

// hintLine compone la barra de hints con las acciones disponibles.
func (m *Model) hintLine() string {
	parts := []string{"j/k move"}
	if k := m.cfg.KeyFor("section-next"); k != "" {
		parts = append(parts, k+" section")
	}
	if k := m.cfg.KeyFor("detail"); k != "" {
		parts = append(parts, k+" detail")
	}
	if k := m.cfg.KeyFor("approve"); k != "" {
		parts = append(parts, k+" approve")
	}
	if k := m.cfg.KeyFor("merge"); k != "" {
		parts = append(parts, k+" merge")
	}
	if k := m.cfg.KeyFor("refresh"); k != "" {
		parts = append(parts, k+" refresh")
	}
	if k := m.cfg.KeyFor("quit"); k != "" {
		parts = append(parts, k+" quit")
	}
	return strings.Join(parts, " · ")
}

// styleNotice elige el estilo del aviso de cabecera.
func styleNotice(level noticeLevel) lipglossStyle {
	switch level {
	case levelOK:
		return styleOK
	case levelWarn:
		return styleWarn
	case levelError:
		return styleError
	default:
		return styleInfo
	}
}
