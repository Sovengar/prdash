// Ventana vertical de la lista: qué líneas se pintan y cómo se desplaza para
// mantener visible el ítem seleccionado.
//
// La lista de la sección activa se compone ENTERA como líneas sueltas (prefijo
// de ruta común, avisos, header de columnas y filas) y luego se recorta a la
// ventana visible. Mover solo las filas dejaría el header huérfano de sus ítems;
// componiendo primero y recortando después, el desplazamiento mantiene la
// coherencia de la tabla a cualquier offset.
package tui

import (
	"prdash/internal/inbox"
)

// listLine es una línea de la lista con la fila a la que pertenece, o -1 si es
// chrome (prefijo, aviso, header de columnas, línea en blanco). Guardar la fila
// permite localizar la línea del cursor sin volver a componer nada.
type listLine struct {
	text string
	row  int
}

// Reparto vertical de la pantalla. El detalle se queda con el 40% de la altura
// (detailShare/5) y la lista con el resto del hueco libre; el reparto vive en
// computeLayout (layout.go) porque las cajas ya se llevan 2 líneas de cromo
// cada una.

// listLines compone la lista de la sección activa como líneas sueltas: la línea
// del prefijo de ruta común si la hay, los avisos, el header de columnas y las
// filas —o el estado vacío— y el indicador de paginación. Sin cabecera interna
// de sección: el título y el conteo viven en la leyenda del borde.
func (m *Model) listLines(inner int) []listLine {
	items := m.rows()
	problems := m.sectionProblems(m.activeSection)
	lines := make([]listLine, 0, len(items)+len(problems)+4)

	// Un solo layout para la activa: si cada línea midiera ITEM por su cuenta,
	// la tabla bailaría al escribir encima. Solo se dimensiona la sección que se
	// pinta, así no se gasta ancho en sufijos de secciones que no se ven.
	lay := newRefLayout([]inbox.Section{{Kind: m.activeSection, Items: items}})

	// El prefijo de ruta común va en una línea fija al inicio del cuerpo, no en
	// cada fila: es lo que deja hueco a la columna ITEM para el sufijo sin
	// truncar. Sin prefijo común (un solo ítem, o sin nada en común) la línea no
	// se pinta y cada celda ITEM lleva la ruta completa recortada por la cola.
	if prefix := lay.prefixOf(m.activeSection); prefix != "" {
		lines = append(lines, listLine{text: "  " + styleDim.Render("· "+prefix+"/"), row: -1})
	}

	for _, p := range problems {
		lines = append(lines, listLine{text: "  " + styleWarn.Render("⚠ "+p), row: -1})
	}

	switch {
	case len(items) > 0:
		lines = append(lines, listLine{text: "  " + headerLine(lay, inner-2), row: -1})
		for i, it := range items {
			lines = append(lines, listLine{text: m.renderItem(it, m.activeSection, lay, i == m.cursor, inner-2), row: i})
		}
	case len(problems) == 0:
		lines = append(lines, listLine{text: "  " + styleEmpty.Render("(empty)"), row: -1})
	}

	// Los avisos son de la activa: si la sección que pagina no se ve, su
	// indicador tampoco. Al volver a ella reaparece.
	if m.sectionLoadingMore(m.activeSection) {
		lines = append(lines, listLine{text: "  " + styleDim.Render("loading more…"), row: -1})
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
	view := m.layout().bodyLines
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
