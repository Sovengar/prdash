// Detalle de un ítem del inbox.
package tui

import (
	"fmt"
	"strings"
	"time"

	"prdash/internal/forge/model"
	"prdash/internal/state"
)

// renderDetail pinta el detalle del ítem: identidad, ramas, estado de review y
// de checks, sin salir de la TUI.
func (m *Model) renderDetail() string {
	it := m.detailItem

	var b strings.Builder
	b.WriteString(styleDetailTitle.Render(it.Title))
	b.WriteString("\n\n")

	b.WriteString(label("Item", refLabel(it)))
	b.WriteString(label("Forge", forgeLabel(it)))
	b.WriteString(label("Autor", orDash(it.Author)))
	b.WriteString(label("Origen", orDash(it.SourceBranch)))
	b.WriteString(label("Destino", orDash(it.TargetBranch)))
	b.WriteString(label("Número", fmt.Sprintf("#%d", it.Number)))
	b.WriteString(label("URL", orDash(it.URL)))
	b.WriteString("\n")

	b.WriteString(label("Estado", state.Derive(it).String()))
	b.WriteString(label("Review", orDash(reviewLabel(it))))
	b.WriteString(label("Checks", checksDetail(it.Checks)))
	b.WriteString(label("Actualizado", relativeTime(it.UpdatedAt)))
	b.WriteString(label("Rol", roleText(it)))
	b.WriteString("\n")

	if reason := m.denied[it.ID()]; reason != "" {
		b.WriteString(styleWarn.Render("  acción deshabilitada: "+reason) + "\n\n")
	}

	b.WriteString(styleHint.Render("esc/" + m.cfg.KeyFor("detail") + " volver al inbox · el cursor no se pierde"))
	return b.String()
}

// label compone una línea "etiqueta: valor" del detalle.
func label(key, value string) string {
	return styleDetailKey.Render(pad(key+":", 14)) + value + "\n"
}

// reviewLabel traduce la decisión de review y el tipo de review a texto.
func reviewLabel(it model.Item) string {
	decision := it.ReviewDecision
	switch decision {
	case "APPROVED":
		decision = "aprobado"
	case "CHANGES_REQUESTED":
		decision = "cambios pedidos"
	case "REVIEW_REQUIRED":
		decision = "review pendiente"
	}
	if decision == "" {
		decision = it.State
	}
	switch it.ReviewKind {
	case model.ReviewRequested:
		return decision + " · review pedido"
	case model.ReviewAssigned:
		return decision + " · asignado"
	default:
		return decision
	}
}

// checksDetail describe el estado de los checks para el detalle.
func checksDetail(c model.Checks) string {
	if c.Total == 0 && c.State == model.ChecksUnknown {
		return "sin checks"
	}
	base := string(c.State)
	if c.Failing > 0 || c.Pending > 0 {
		base += fmt.Sprintf(" (%d/%d fallan, %d pendientes)", c.Failing, c.Total, c.Pending)
	} else {
		base += fmt.Sprintf(" (%d)", c.Total)
	}
	return base
}

// relativeTime formatea una marca temporal como "3h", "2d".
func relativeTime(t time.Time) string {
	if t.IsZero() {
		return "-"
	}
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "ahora"
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd", int(d.Hours()/24))
	}
}

func orDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}
