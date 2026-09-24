// Package state deriva, a partir del estado crudo que devuelve cada forge, un
// estado único con precedencia explícita y un score de atención. Lo comparten
// la TUI y el modo de impresión para que el orden sea el mismo.
package state

import "prdash/internal/forge/model"

// State es el estado derivado de un ítem del inbox.
type State int

const (
	// StateDraft es un PR/MR en borrador.
	StateDraft State = iota
	// StatePending está abierto sin decisión de review.
	StatePending
	// StateApproved está aprobado.
	StateApproved
	// StateReviewRequired espera review.
	StateReviewRequired
	// StateChangesRequested tiene cambios pedidos.
	StateChangesRequested
	// StateMerged está mergeado.
	StateMerged
	// StateClosed está cerrado sin mergear.
	StateClosed
	// StateError tiene checks fallidos o un fallo del forge.
	StateError
)

// String devuelve la etiqueta del estado para la UI y el modo de impresión.
func (s State) String() string {
	switch s {
	case StateDraft:
		return "draft"
	case StatePending:
		return "pending"
	case StateApproved:
		return "approved"
	case StateReviewRequired:
		return "review required"
	case StateChangesRequested:
		return "changes requested"
	case StateMerged:
		return "merged"
	case StateClosed:
		return "closed"
	case StateError:
		return "error"
	default:
		return "unknown"
	}
}

// Score es la prioridad de atención para el orden del inbox:
// error > changes-requested > review-required > approved > pending >
// merged/closed > draft.
func (s State) Score() int {
	switch s {
	case StateError:
		return 8
	case StateChangesRequested:
		return 7
	case StateReviewRequired:
		return 6
	case StateApproved:
		return 5
	case StatePending:
		return 4
	case StateMerged, StateClosed:
		return 2
	case StateDraft:
		return 1
	default:
		return 0
	}
}

// Derive deduce el estado del ítem a partir de lo que reportó el forge.
// Precedencia: cerrado/mergeado > checks fallidos > cambios pedidos > review
// pedido > aprobado > borrador > abierto.
func Derive(it model.Item) State {
	switch normalize(it.State) {
	case "merged":
		return StateMerged
	case "closed":
		return StateClosed
	}

	if it.Checks.State == model.ChecksFailing {
		return StateError
	}

	switch normalize(it.ReviewDecision) {
	case "changes_requested":
		return StateChangesRequested
	case "review_required":
		return StateReviewRequired
	case "approved":
		return StateApproved
	}

	if it.State == "draft" {
		return StateDraft
	}
	return StatePending
}

// normalize compara estados sin depender de mayúsculas ni separadores.
func normalize(s string) string {
	out := make([]rune, 0, len(s))
	for _, r := range s {
		switch r {
		case '-', ' ', '\t':
			out = append(out, '_')
		default:
			if r >= 'A' && r <= 'Z' {
				r += 'a' - 'A'
			}
			out = append(out, r)
		}
	}
	return string(out)
}
