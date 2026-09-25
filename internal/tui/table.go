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

// Índices de columna. ITEM es la única de ancho variable: el resto son
// constantes y solo hay que nombrarlas una vez.
const (
	colForgeIdx = iota
	colRefIdx
	colTitleIdx
	colRoleIdx
	colStateIdx
	colChecksIdx
)

// tableColumns son las columnas en orden de prioridad: en anchos estrechos se
// omiten por la derecha para no truncar las columnas de estado. El ancho de ITEM
// es nominal: newRefLayout lo sustituye por el que pide el contenido.
var tableColumns = []tableColumn{
	{"FORGE", colForge},
	{"ITEM", itemWidthMin},
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
func fitColumns(l refLayout, innerWidth int) int {
	used := 0
	for i, c := range l.cols {
		if used+c.width > innerWidth {
			return max(1, i)
		}
		used += c.width
	}
	return len(l.cols)
}

// headerLine compone el header de columnas que cabe en el ancho dado.
func headerLine(l refLayout, innerWidth int) string {
	var b strings.Builder
	for _, c := range l.cols[:fitColumns(l, innerWidth)] {
		b.WriteString(pad(c.title, c.width))
	}
	return styleCount.Render(strings.TrimRight(b.String(), " "))
}

// itemCells compone las celdas de un ítem. `viewer` es el login del usuario en
// ese forge: lo necesita la columna ROLE para marcar los ítems propios. `sec` es
// la sección a la que pertenece: de ella sale el prefijo de ruta que la celda de
// ITEM no repite.
func itemCells(it model.Item, sec model.Section, viewer string, l refLayout) []cell {
	refW := l.cols[colRefIdx].width
	return []cell{
		{truncate(forgeBadge(it), l.cols[colForgeIdx].width), styleForge, l.cols[colForgeIdx].width},
		{truncateTail(refSuffix(it, l.prefixOf(sec)), refW), styleRef, refW},
		{truncate(it.Title, l.cols[colTitleIdx].width), styleTitle, l.cols[colTitleIdx].width},
		{roleText(it, viewer), styleRole, l.cols[colRoleIdx].width},
		{state.Derive(it).String(), styleForState(state.Derive(it)), l.cols[colStateIdx].width},
		{checksText(it.Checks), styleChecks(it.Checks), l.cols[colChecksIdx].width},
	}
}

// renderCells pinta una fila: pad() sobre el texto plano y luego el estilo.
func renderCells(cells []cell, l refLayout, innerWidth int) string {
	var b strings.Builder
	for _, c := range cells[:min(len(cells), fitColumns(l, innerWidth))] {
		b.WriteString(c.style.Render(pad(c.text, c.width)))
	}
	return b.String()
}

// forgeLabel indica forge y host del ítem. Es la forma larga: vive en el
// detalle, donde sí cabe la ruta completa.
func forgeLabel(it model.Item) string {
	if it.Host == "" {
		return it.Forge
	}
	return it.Forge + "@" + it.Host
}

// forgeShortNames mapea forge → etiqueta corta de la columna FORGE.
var forgeShortNames = map[string]string{
	"github":    "GH",
	"gitlab":    "GLab",
	"bitbucket": "BB",
}

// forgePublicHosts son los hosts públicos de cada forge: ahí la etiqueta corta
// ya es suficiente y no hace falta repetir el proveedor ni el host.
var forgePublicHosts = map[string]string{
	"github":    "github.com",
	"gitlab":    "gitlab.com",
	"bitbucket": "bitbucket.org",
}

// forgeBadge etiqueta la columna FORGE sin el proveedor ni el host entero: solo
// la abreviatura en el host estándar ("GH", "GLab") y abreviatura + primera
// etiqueta del host en uno self-hosted ("GLab@umane"). La ruta completa se
// reserva para el detalle, donde no estorba.
func forgeBadge(it model.Item) string {
	short := forgeShortNames[it.Forge]
	if short == "" {
		short = it.Forge // forge sin abreviatura conocida: se muestra tal cual
	}
	if short == "" || it.Host == "" {
		return short
	}
	if strings.EqualFold(it.Host, forgePublicHosts[it.Forge]) {
		return short
	}
	label, _, _ := strings.Cut(it.Host, ".")
	return short + "@" + label
}

// refLabel compone la referencia corta del ítem: "proyecto#número".
func refLabel(it model.Item) string {
	return it.Ref.Project + "#" + strconv.Itoa(it.Number)
}

// roleText indica el papel del usuario en el ítem: por qué lo tiene en el inbox
// y, cuando no hay review pendiente, si es el autor. "own" es la señal de que
// approve no va a funcionar, antes de pulsarlo.
func roleText(it model.Item, viewer string) string {
	switch it.ReviewKind {
	case model.ReviewRequested:
		return "review req"
	case model.ReviewAssigned:
		return "assigned"
	}
	if ok, _ := state.CanApprove(it, viewer); !ok {
		return "own"
	}
	return "-"
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
