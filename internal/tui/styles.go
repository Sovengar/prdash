// Estilos y anchos de columna del inbox.
package tui

import (
	"charm.land/lipgloss/v2"

	"prdash/internal/state"
)

// lipglossStyle es un alias corto para las firmas de las celdas.
type lipglossStyle = lipgloss.Style

// Anchos de columna: siempre mayores que su header para que pad() garantice
// separación entre columnas.
const (
	colForge  = 26 // "gitlab@gitlab.example.com"
	colRef    = 30 // "grupo/sub/proyecto#1234"
	colTitle  = 40
	colRole   = 10 // "requested" / "assigned"
	colState  = 18
	colChecks = 8
)

var (
	styleCursor  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("205"))
	styleDim     = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	styleHeader  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("14"))
	styleCount   = lipgloss.NewStyle().Foreground(lipgloss.Color("244"))
	styleForge   = lipgloss.NewStyle().Foreground(lipgloss.Color("39"))
	styleRef     = lipgloss.NewStyle().Foreground(lipgloss.Color("252"))
	styleTitle   = lipgloss.NewStyle()
	styleRole    = lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	styleEmpty   = lipgloss.NewStyle().Foreground(lipgloss.Color("241")).Italic(true)
	styleWarn    = lipgloss.NewStyle().Foreground(lipgloss.Color("208"))
	styleHint    = lipgloss.NewStyle().Foreground(lipgloss.Color("241"))

	styleStateError    = lipgloss.NewStyle().Foreground(lipgloss.Color("196"))
	styleStateChanges  = lipgloss.NewStyle().Foreground(lipgloss.Color("214"))
	styleStateReview   = lipgloss.NewStyle().Foreground(lipgloss.Color("39"))
	styleStateApproved = lipgloss.NewStyle().Foreground(lipgloss.Color("46"))
	styleStateLow      = lipgloss.NewStyle().Foreground(lipgloss.Color("244"))

	styleChecksFailing = lipgloss.NewStyle().Foreground(lipgloss.Color("196"))
	styleChecksPending = lipgloss.NewStyle().Foreground(lipgloss.Color("214"))
	styleChecksPassing = lipgloss.NewStyle().Foreground(lipgloss.Color("46"))
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
