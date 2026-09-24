// Update y View del inbox.
package tui

import (
	"fmt"
	"strings"
	"time"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"

	"prdash/internal/forge/model"
	"prdash/internal/inbox"
)

// Update procesa mensajes: resultados de forges, teclas y resize.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		return m, nil

	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd

	case forgeResultMsg:
		m.applyResult(msg.result, time.Now())
		return m.withPump(nil)

	case refreshDoneMsg:
		m.loading = false
		m.lastRefresh = time.Now()
		return m.withPump(nil)

	case tea.KeyPressMsg:
		return m.handleKey(msg)
	}
	return m, nil
}

// withPump rearma la bomba de eventos tras procesar un evento del canal.
func (m Model) withPump(cmd tea.Cmd) (tea.Model, tea.Cmd) {
	return m, tea.Batch(cmd, waitForEvent(m.events))
}

// applyResult reemplaza el resultado de un forge y recompone el inbox.
func (m *Model) applyResult(res inbox.ForgeResult, now time.Time) {
	replaced := false
	for i := range m.results {
		if m.results[i].Forge == res.Forge && m.results[i].Host == res.Host {
			m.results[i] = res
			replaced = true
			break
		}
	}
	if !replaced {
		m.results = append(m.results, res)
	}
	for i := range m.statuses {
		if m.statuses[i].Forge == res.Forge && m.statuses[i].Host == res.Host {
			m.statuses[i].Warnings = res.Warnings
			m.statuses[i].UpdatedAt = now
			m.statuses[i].Auth = authFromWarnings(res)
		}
	}
	m.rebuild()
}

// authFromWarnings deduce el estado de autenticación de un resultado: un
// warning de tipo auth significa forge no operativo.
func authFromWarnings(res inbox.ForgeResult) model.AuthState {
	for _, w := range res.Warnings {
		if w.Kind == "auth" {
			return model.AuthState{Forge: res.Forge, OK: false, Reason: w.Msg}
		}
	}
	return model.AuthState{Forge: res.Forge, OK: true}
}

// handleKey enruta las teclas: navegación fija y acciones configurables.
func (m Model) handleKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	key := msg.String()

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

	switch m.cfg.KeyFor(key) {
	case "refresh":
		cmd := m.startRefreshCmd()
		return m, cmd
	case "quit":
		m.cancel()
		return m, tea.Quit
	}
	return m, nil
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

// View compone la pantalla del inbox.
func (m Model) View() tea.View {
	v := tea.NewView(m.render())
	v.AltScreen = true
	return v
}

// render pinta la cabecera, las tres secciones y los hints.
func (m *Model) render() string {
	var b strings.Builder
	inner := m.contentWidth()

	b.WriteString(styleHeader.Render("prdash"))
	if m.loading {
		b.WriteString(" " + m.spinner.View() + styleCount.Render(" refreshing…"))
	}
	b.WriteString("  " + styleCount.Render("updated "+m.lastRefreshLabel(time.Now())))
	b.WriteString("  " + forgesStatusLine(m.statuses))
	b.WriteString("\n\n")

	row := 0
	for _, sec := range m.inbox.Sections {
		problems := m.sectionProblems(sec.Kind)
		b.WriteString(styleHeader.Render(fmt.Sprintf("%s (%d)", sec.Kind.String(), len(sec.Items))))
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

// hintLine compone la barra de hints con las acciones disponibles.
func (m *Model) hintLine() string {
	parts := []string{"j/k move"}
	if k := m.cfg.KeyFor("section-next"); k != "" {
		parts = append(parts, k+" section")
	}
	if k := m.cfg.KeyFor("refresh"); k != "" {
		parts = append(parts, k+" refresh")
	}
	if k := m.cfg.KeyFor("quit"); k != "" {
		parts = append(parts, k+" quit")
	}
	return strings.Join(parts, " · ")
}

// forgesStatusLine resume si cada forge está operativo.
func forgesStatusLine(statuses []forgeStatus) string {
	parts := make([]string, 0, len(statuses))
	for _, s := range statuses {
		label := s.Forge
		if s.Auth.OK {
			label += " ✓"
		} else {
			label += " ✗"
		}
		parts = append(parts, label)
	}
	return strings.Join(parts, " · ")
}
