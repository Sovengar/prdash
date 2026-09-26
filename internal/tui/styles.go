// Estilos y anchos de columna del inbox.
package tui

import (
	"charm.land/lipgloss/v2"

	"prdash/internal/state"
)

// lipglossStyle es un alias corto para las firmas de las celdas.
type lipglossStyle = lipgloss.Style

// Anchos de columna: el ancho es el tamaño total de la ranura, hueco de
// separación incluido, así que el texto usable es un rune menos (textWidth).
// Los anchos fijos además superan al header, para que ninguna columna sea más
// estrecha que su propio título. ITEM no está aquí porque su ancho sale del
// contenido (ver newRefLayout): solo tiene límites.
const (
	colForge  = 14 // "GLab@umane"
	colTitle  = 40
	colRole   = 11 // "review req" (10) + hueco de separación
	colState  = 18 // "changes requested"
	colChecks = 8
	// colDiff Aloja el par compacto de líneas: "+1.2k -6.7k" son 11 runes, el
	// peor caso que produce la compactación.
	colDiff = 12
)

// Límites de la columna ITEM: se dimensiona al sufijo más largo de todo el
// inbox, acotado para que no se coma TITLE.
const (
	itemWidthMin = 6  // "ITEM" (4) + hueco + 1 rune de texto
	itemWidthCap = 34 // "subgrupo/proyecto#1234"
)

// defaultOuterWidth es el ancho de trabajo antes del primer WindowSizeMsg: las
// cajas se dibujan siempre a un ancho exacto, así que sin este valor el primer
// render saldría a 0 columnas.
const defaultOuterWidth = 124

var (
	styleCursor = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("205"))
	styleDim    = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	styleHeader = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("14"))
	styleCount  = lipgloss.NewStyle().Foreground(lipgloss.Color("244"))
	styleForge  = lipgloss.NewStyle().Foreground(lipgloss.Color("39"))
	styleRef    = lipgloss.NewStyle().Foreground(lipgloss.Color("252"))
	styleTitle  = lipgloss.NewStyle()
	styleRole   = lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	styleEmpty  = lipgloss.NewStyle().Foreground(lipgloss.Color("241")).Italic(true)
	styleWarn   = lipgloss.NewStyle().Foreground(lipgloss.Color("208"))
	styleHint   = lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
	styleInfo   = lipgloss.NewStyle().Foreground(lipgloss.Color("39"))
	styleOK     = lipgloss.NewStyle().Foreground(lipgloss.Color("46"))
	styleError  = lipgloss.NewStyle().Foreground(lipgloss.Color("196"))

	styleDetailKey   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("39"))
	styleDetailTitle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("205"))

	styleStateError    = lipgloss.NewStyle().Foreground(lipgloss.Color("196"))
	styleStateChanges  = lipgloss.NewStyle().Foreground(lipgloss.Color("214"))
	styleStateReview   = lipgloss.NewStyle().Foreground(lipgloss.Color("39"))
	styleStateApproved = lipgloss.NewStyle().Foreground(lipgloss.Color("46"))
	styleStateLow      = lipgloss.NewStyle().Foreground(lipgloss.Color("244"))

	styleChecksFailing = lipgloss.NewStyle().Foreground(lipgloss.Color("196"))
	styleChecksPending = lipgloss.NewStyle().Foreground(lipgloss.Color("214"))
	styleChecksPassing = lipgloss.NewStyle().Foreground(lipgloss.Color("46"))

	// La columna DIFF va en dos colores dentro de la misma celda: verde lo que
	// el cambio añade, rojo lo que quita. Es notación de diff, no un juicio —
	// borrar 900 líneas suele ser justo lo que se quería, así que el rojo no dice
	// que el cambio sea malo, solo que esas líneas disappear. El color se reserva
	// para el estado, que sí decide si la acción procede.
	styleDiffAdd = lipgloss.NewStyle().Foreground(lipgloss.Color("46"))
	styleDiffDel = lipgloss.NewStyle().Foreground(lipgloss.Color("196"))
	// styleDiffUnknown es el gris de un diffstat que el forge no reportó: no es
	// un cero, es una ausencia, y no se colorea para que no se confunda con una
	// cifra.
	styleDiffUnknown = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))

	// borderColor es el color del borde de las cajas: gris muy tenue, para que
	// la estructura se lea sin competir con el contenido. Lo usan todas, incluida
	// la de comentarios, que va anidada dentro del panel de detalle: esa se
	// distingue por el sangrado, no por un tono distinto. Un tono más claro del
	// mismo gris la separaba del panel, pero salía amarillento en las paletas
	// cálidas, y lo que hacía falta era que no compartiera columnas con el borde
	// de fuera, no que se viera distinto.
	borderColor = lipgloss.Color("238")

	// styleBorder es borderColor como estilo, para poder repintar un tramo suelto
	// de una línea de borde. Hace falta al escribir sobre ella: un estilo de
	// lipgloss se cierra con `\x1b[0m`, y ese reset no restaura lo que había
	// antes, así que lo que venga después queda con el color de primer plano del
	// terminal en vez de con el del borde.
	styleBorder = lipgloss.NewStyle().Foreground(borderColor)
)

// styleForState elige el estilo de la columna de estado.
func styleForState(s state.State) lipglossStyle {
	switch s {
	case state.StateError:
		return styleStateError
	case state.StateChangesRequested:
		return styleStateChanges
	case state.StateReviewRequired:
		return styleStateReview
	case state.StateApproved:
		return styleStateApproved
	default:
		return styleStateLow
	}
}
