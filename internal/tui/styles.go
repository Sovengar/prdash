package tui

import (
	"charm.land/lipgloss/v2"

	"prdash/internal/state"
)

type lipglossStyle = lipgloss.Style

const (
	colForge  = 14 // "GLab@umane"
	colTitle  = 40
	colRole   = 11 // "review req" (10) + hueco de separación
	colState  = 18 // "changes requested"
	colChecks = 8
	colDiff   = 12
)

const (
	itemWidthMin = 6  // "ITEM" (4) + hueco + 1 rune de texto
	itemWidthCap = 34 // "subgrupo/proyecto#1234"
)

// Boxes are always drawn at an exact width, so without this the first render would come out at 0
// columns.
const defaultOuterWidth = 124

var (
	styleCursor       = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("205"))
	styleDim          = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	styleLegendActive = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("205"))
	styleCount        = lipgloss.NewStyle().Foreground(lipgloss.Color("244"))
	styleForge        = lipgloss.NewStyle().Foreground(lipgloss.Color("39"))
	styleRef          = lipgloss.NewStyle().Foreground(lipgloss.Color("252"))
	styleTitle        = lipgloss.NewStyle()
	styleRole         = lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	styleEmpty        = lipgloss.NewStyle().Foreground(lipgloss.Color("241")).Italic(true)
	styleWarn         = lipgloss.NewStyle().Foreground(lipgloss.Color("208"))
	styleHint         = lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
	styleInfo         = lipgloss.NewStyle().Foreground(lipgloss.Color("39"))
	styleOK           = lipgloss.NewStyle().Foreground(lipgloss.Color("46"))
	styleError        = lipgloss.NewStyle().Foreground(lipgloss.Color("196"))

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

	styleDiffAdd     = lipgloss.NewStyle().Foreground(lipgloss.Color("46"))
	styleDiffDel     = lipgloss.NewStyle().Foreground(lipgloss.Color("196"))
	styleDiffUnknown = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))

	borderColor = lipgloss.Color("238")

	styleBorder = lipgloss.NewStyle().Foreground(borderColor)
)

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
