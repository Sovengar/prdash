package herdr

import (
	"context"
	"fmt"
	"strings"

	"prdash/internal/review/plan"
)

// splitRatio es la fracción que se lleva el pane nuevo en cada división.
const splitRatio = 0.5

// MountLayout abre el plan de panes sobre el contenedor dado y devuelve los
// avisos no fatales. El primer pane reutiliza el pane base del contenedor (el
// root pane del worktree); los siguientes se abren con `pane split --no-focus`.
//
// Es tolerante: un pane que no se puede abrir o lanzar no tumba el resto, se
// reporta como aviso. Solo falla si no hay forma de obtener un pane base.
func (c *Client) MountLayout(ctx context.Context, container Container, pl plan.Plan) ([]string, error) {
	if len(pl.Panes) == 0 {
		return nil, nil
	}
	if err := c.guard("pane", "split"); err != nil {
		return nil, err
	}

	anchor := container.PaneID
	if anchor == "" && container.WorkspaceID != "" {
		if panes, err := c.PaneList(ctx, container.WorkspaceID); err == nil && len(panes) > 0 {
			anchor = panes[0].PaneID
		}
	}
	if anchor == "" {
		ws, err := c.WorkspaceCreate(ctx, WorkspaceSpec{
			Cwd:     pl.Panes[0].Cwd,
			Label:   pl.Panes[0].Label,
			NoFocus: true,
		})
		if err != nil {
			return nil, fmt.Errorf("crear el workspace del review: %w", err)
		}
		anchor = ws.RootPaneID
	}
	if anchor == "" {
		return nil, fmt.Errorf("herdr no devolvió un pane base para el layout")
	}

	var warnings []string
	// Primer pane: reutiliza el pane base del contenedor.
	first := pl.Panes[0]
	if err := c.PaneRun(ctx, anchor, []string{paneCommand(first)}); err != nil {
		warnings = append(warnings, "no se pudo lanzar "+first.Label+": "+err.Error())
	}
	if err := c.PaneRename(ctx, anchor, first.Label); err != nil {
		warnings = append(warnings, "no se pudo etiquetar "+first.Label+": "+err.Error())
	}

	// Panes siguientes: se dividen encadenados (primero a la derecha; el resto
	// hacia abajo sobre el último creado) para un layout de columnas legible.
	parent := anchor
	for i, p := range pl.Panes[1:] {
		direction := "down"
		if i == 0 {
			direction = "right"
		}
		info, err := c.PaneSplit(ctx, SplitSpec{
			PaneID:    parent,
			Direction: direction,
			Ratio:     splitRatio,
			Cwd:       p.Cwd,
			Env:       p.Env,
			NoFocus:   true,
		})
		if err != nil {
			warnings = append(warnings, "no se pudo abrir el pane "+p.Label+": "+err.Error())
			continue
		}
		if err := c.PaneRun(ctx, info.PaneID, []string{paneCommand(p)}); err != nil {
			warnings = append(warnings, "no se pudo lanzar "+p.Label+": "+err.Error())
		}
		if err := c.PaneRename(ctx, info.PaneID, p.Label); err != nil {
			warnings = append(warnings, "no se pudo etiquetar "+p.Label+": "+err.Error())
		}
		parent = info.PaneID
	}
	return warnings, nil
}

// paneCommand compone la línea de shell que se envía al pane: sitúa el cwd,
// inyecta el entorno del plan y ejecuta el argv con `exec`.
func paneCommand(p plan.Pane) string {
	var b strings.Builder
	if p.Cwd != "" {
		b.WriteString("cd ")
		b.WriteString(shellQuote(p.Cwd))
		b.WriteString(" && ")
	}
	for _, kv := range p.Env {
		k, v, ok := strings.Cut(kv, "=")
		if !ok || k == "" {
			continue
		}
		b.WriteString("export ")
		b.WriteString(k)
		b.WriteString("=")
		b.WriteString(shellQuote(v))
		b.WriteString(" && ")
	}
	for i, a := range p.Argv {
		if i > 0 {
			b.WriteByte(' ')
		}
		b.WriteString(shellQuote(a))
	}
	return b.String()
}

// shellQuote cita un argumento para que el shell del pane lo trate literal.
func shellQuote(s string) string {
	if s == "" {
		return "''"
	}
	if shellSafe(s) {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// shellSafe informa si un texto no necesita comillas.
func shellSafe(s string) bool {
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
		case r == '-', r == '_', r == '.', r == '/', r == '@', r == ':', r == '=', r == '+', r == ',', r == '%', r == '#':
		default:
			return false
		}
	}
	return true
}
