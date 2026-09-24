// Filas, columnas y celdas del inbox.
//
// Cada celda devuelve su texto plano y su estilo por separado; el render hace
// pad(texto) ANTES de aplicar el estilo, así los códigos ANSI nunca rompen el
// ancho de la tabla.
package tui

import (
	"strconv"
	"strings"
	"unicode/utf8"

	"prdash/internal/forge/model"
	"prdash/internal/state"
)

// tableColumn describe una columna (título y ancho).
type tableColumn struct {
	title string
	width int
}

// tableColumns son las columnas en orden de prioridad: en anchos estrechos se
// omiten por la derecha para no truncar las columnas de estado.
var tableColumns = []tableColumn{
	{"FORGE", colForge},
	{"ITEM", colRef},
	{"TITLE", colTitle},
	{"ROLE", colRole},
	{"STATE", colState},
	{"CHECKS", colChecks},
}

// cell es una celda con su texto plano, su estilo y su ancho.
type cell struct {
	text  string
	style lipglossStyle
	width int
}

// fitColumns devuelve cuántas columnas caben en el ancho disponible, dejando
// siempre al menos la de FORGE.
func fitColumns(innerWidth int) int {
	used := 0
	for i, c := range tableColumns {
		if used+c.width > innerWidth {
			return max(1, i)
		}
		used += c.width
	}
	return len(tableColumns)
}

// headerLine compone el header de columnas que cabe en el ancho dado.
func headerLine(innerWidth int) string {
	var b strings.Builder
	for _, c := range tableColumns[:fitColumns(innerWidth)] {
		b.WriteString(pad(c.title, c.width))
	}
	return styleCount.Render(strings.TrimRight(b.String(), " "))
}

// itemCells compone las celdas de un ítem.
func itemCells(it model.Item) []cell {
	return []cell{
		{forgeLabel(it), styleForge, colForge},
		{truncate(refLabel(it), colRef), styleRef, colRef},
		{truncate(it.Title, colTitle), styleTitle, colTitle},
		{roleText(it), styleRole, colRole},
		{state.Derive(it).String(), styleForState(state.Derive(it)), colState},
		{checksText(it.Checks), styleChecks(it.Checks), colChecks},
	}
}

// renderCells pinta una fila: pad() sobre el texto plano y luego el estilo.
func renderCells(cells []cell, innerWidth int) string {
	var b strings.Builder
	for _, c := range cells[:min(len(cells), fitColumns(innerWidth))] {
		b.WriteString(c.style.Render(pad(c.text, c.width)))
	}
	return b.String()
}

// forgeLabel indica forge y host del ítem.
func forgeLabel(it model.Item) string {
	if it.Host == "" {
		return it.Forge
	}
	return it.Forge + "@" + it.Host
}

// refLabel compone la referencia corta del ítem: "proyecto#número".
func refLabel(it model.Item) string {
	return it.Ref.Project + "#" + strconv.Itoa(it.Number)
}

// roleText indica si un ítem de la sección de review llegó por petición de
// review o por asignación.
func roleText(it model.Item) string {
	switch it.ReviewKind {
	case model.ReviewRequested:
		return "review req"
	case model.ReviewAssigned:
		return "assigned"
	default:
		return "-"
	}
}

// checksText resume el estado de los checks.
func checksText(c model.Checks) string {
	switch c.State {
	case model.ChecksFailing:
		return "✗" + strconv.Itoa(c.Failing)
	case model.ChecksPending:
		return "…" + strconv.Itoa(c.Pending)
	case model.ChecksPassing:
		return "✓"
	default:
		return "-"
	}
}

// styleChecks elige el color de la columna de checks.
func styleChecks(c model.Checks) lipglossStyle {
	switch c.State {
	case model.ChecksFailing:
		return styleChecksFailing
	case model.ChecksPending:
		return styleChecksPending
	case model.ChecksPassing:
		return styleChecksPassing
	default:
		return styleDim
	}
}

// pad rellena a la derecha midiendo runes (solo texto plano, sin ANSI).
func pad(s string, w int) string {
	n := utf8.RuneCountInString(s)
	if n >= w {
		return s
	}
	return s + strings.Repeat(" ", w-n)
}

// truncate recorta a w runes añadiendo "…" si hacía falta.
func truncate(s string, w int) string {
	if w <= 0 {
		return ""
	}
	if utf8.RuneCountInString(s) <= w {
		return s
	}
	runes := []rune(s)
	if w == 1 {
		return "…"
	}
	return string(runes[:w-1]) + "…"
}

// stripANSI quita secuencias ANSI para comparar contenido en tests.
func stripANSI(s string) string {
	var out strings.Builder
	inSeq := false
	for _, r := range s {
		switch {
		case r == '\x1b':
			inSeq = true
		case inSeq && (r == 'm' || r == 'K'):
			inSeq = false
		case !inSeq:
			out.WriteRune(r)
		}
	}
	return out.String()
}
