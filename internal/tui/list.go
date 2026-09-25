// Ventana vertical de la lista: qué líneas se pintan y cómo se desplaza para
// mantener visible el ítem seleccionado.
//
// La lista se compone ENTERA como líneas sueltas (headers de sección, avisos y
// filas) y luego se recorta a la ventana visible. Mover solo las filas dejaría
// los headers huérfanos de sus ítems; componiendo primero y recortando después,
// el desplazamiento mantiene la coherencia de la tabla a cualquier offset.
package tui

import (
	"fmt"
	"strings"
)

// listLine es una línea de la lista con la fila a la que pertenece, o -1 si es
// chrome (header de sección, aviso, línea en blanco). Guardar la fila permite
// localizar la línea del cursor sin volver a componer nada.
type listLine struct {
	text string
	row  int
}

// Reparto vertical de la pantalla. El detalle se queda con el 40% de la altura
// (detailShare/5) y la lista con el resto del hueco libre.
const (
	detailShare   = 2 // 2/5 = 40% de la altura
	minListRows   = 3
	minDetailRows = 6
)

// splitRows reparte la altura entre la ventana de la lista y el panel de
// detalle, descontando el chrome fijo (cabecera, separador y barra de hints).
// Sin altura conocida devuelve 0 y 0: entonces no se recorta nada, que es lo
// que evita el primer render antes del primer WindowSizeMsg.
func splitRows(height, chrome int) (list, detail int) {
	if height <= 0 {
		return 0, 0
	}
	detail = max(height*detailShare/5, minDetailRows)
	if list = height - chrome - detail; list >= minListRows {
		return list, detail
	}
	// En una pantalla muy baja la lista conserva su mínimo (sin ella no hay
	// selección que mostrar) y el detalle cede lo que falte.
	return minListRows, max(0, height-chrome-minListRows)
}

// chromeLines son las líneas que la lista no puede usar: cabecera, hueco,
// separador del panel y barra de hints. Los avisos ya no ocupan sitio: van
// superpuestos como toast, así que el alto de la lista es siempre el mismo.
func (m *Model) chromeLines() int {
	return 4
}

// listLines compone la lista completa como líneas sueltas.
func (m *Model) listLines(inner int) []listLine {
	lines := make([]listLine, 0, len(m.rows())+2*len(m.inbox.Sections)+4)
	// Un solo layout para cabecera y filas: si cada línea midiera ITEM por su
	// cuenta, la tabla bailaría al escribir encima.
	lay := newRefLayout(m.inbox.Sections)
	row := 0
	for _, sec := range m.inbox.Sections {
		problems := m.sectionProblems(sec.Kind)
		header := fmt.Sprintf("%s (%d)", sec.Kind.String(), len(sec.Items))
		if m.sectionLoadingMore(sec.Kind) {
			header += " · loading more…"
		}
		// El prefijo de ruta común va en la cabecera, no en cada fila: es lo que
		// deja hueco a la columna ITEM para el sufijo sin truncar.
		line := styleHeader.Render(header)
		if prefix := lay.prefixOf(sec.Kind); prefix != "" {
			line += styleDim.Render("  · " + prefix + "/")
		}
		lines = append(lines, listLine{text: line, row: -1})

		for _, p := range problems {
			lines = append(lines, listLine{text: "  " + styleWarn.Render("⚠ "+p), row: -1})
		}

		switch {
		case len(sec.Items) > 0:
			lines = append(lines, listLine{text: "  " + headerLine(lay, inner-2), row: -1})
			for _, it := range sec.Items {
				lines = append(lines, listLine{text: m.renderItem(it, sec.Kind, lay, row == m.cursor, inner-2), row: row})
				row++
			}
		case len(problems) == 0:
			lines = append(lines, listLine{text: "  " + styleEmpty.Render("(empty)"), row: -1})
		}
		lines = append(lines, listLine{text: "", row: -1})
	}
	return lines
}

// cursorLine devuelve el índice de línea de la fila del cursor, o -1 si el
// inbox no tiene esa fila (vacío, o la lista se recompuso con otro criterio).
func cursorLine(lines []listLine, cursor int) int {
	for i, l := range lines {
		if l.row == cursor {
			return i
		}
	}
	return -1
}

// scrollFor devuelve el desplazamiento que deja visible la línea `target` en una
// ventana de `view` líneas, acotado al contenido disponible. target < 0 (sin
// fila que seguir) deja el desplazamiento donde estaba, solo acotado.
func scrollFor(current, target, total, view int) int {
	if target >= 0 {
		switch {
		case target < current:
			current = target
		case target >= current+view:
			current = target - view + 1
		}
	}
	return min(max(current, 0), max(0, total-view))
}

// syncScroll es el auto-scroll: deja visible la fila del cursor tras moverlo o
// tras reconstruir el inbox. Va en Update (no en View) porque View tiene
// receptor por valor y sus cambios se perderían.
func (m *Model) syncScroll() {
	view, _ := splitRows(m.height, m.chromeLines())
	if view <= 0 {
		return
	}
	lines := m.listLines(m.contentWidth())
	m.scroll = scrollFor(m.scroll, cursorLine(lines, m.cursor), len(lines), view)
}

// visibleList acota el desplazamiento al contenido actual y devuelve las líneas
// de la ventana. View no puede mutar el modelo, así que el recorte se resuelve
// aquí sobre una copia: si el estado del scroll quedó desfasado por un refresco,
// la vista sigue siendo coherente (scrollFor ya lo mantiene al día).
func visibleList(lines []listLine, scroll, view int) []listLine {
	if view <= 0 || len(lines) == 0 {
		return nil
	}
	start := min(max(scroll, 0), max(0, len(lines)-view))
	return lines[start:min(start+view, len(lines))]
}

// separator es la línea que parte la lista del panel de detalle.
func (m *Model) separator() string {
	return styleDim.Render(strings.Repeat("─", m.contentWidth()))
}
