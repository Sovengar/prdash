package herdr

import (
	"context"
	"fmt"
	"strings"

	"prdash/internal/review/plan"
)

// splitRatio es la fracción que se lleva el pane nuevo en cada división.
const splitRatio = 0.5

// MountLayout abre el plan sobre el workspace del contenedor y devuelve los
// avisos no fatales.
//
// El primer tab reutiliza el tab que ya trae el contenedor (el del root pane del
// worktree) y lo renombra a su etiqueta, para no dejar una pestaña huérfana que
// el usuario tendría que cerrar a mano; los siguientes se crean con
// `tab create --no-focus`. Dentro de cada tab, el primer pane reutiliza el pane
// base y los demás se abren con `pane split` en la dirección que fija el plan.
//
// Es tolerante: un tab o un pane que no se puede abrir o lanzar no tumba el
// resto, se reporta como aviso. Solo falla si no hay forma de obtener un pane
// base.
func (c *Client) MountLayout(ctx context.Context, container Container, pl plan.Plan) ([]string, error) {
	if len(pl.Tabs) == 0 {
		return nil, nil
	}
	if err := c.guard("pane", "split"); err != nil {
		return nil, err
	}

	workspaceID, anchor, err := c.basePane(ctx, container, pl.Tabs[0])
	if err != nil {
		return nil, err
	}

	var warnings []string
	// Primer tab: el que ya viene con el contenedor, renombrado a su etiqueta.
	first := pl.Tabs[0]
	if id := c.tabOf(ctx, workspaceID, anchor); id != "" && first.Label != "" {
		if err := c.TabRename(ctx, id, first.Label); err != nil {
			warnings = append(warnings, "could not label tab "+first.Label+": "+err.Error())
		}
	}
	c.fillTab(ctx, first, anchor, &warnings)

	// Tabs siguientes: los crea Herdr, siempre sin foco para que el montaje no
	// robe la vista a medio hacer.
	for _, tab := range pl.Tabs[1:] {
		info, err := c.TabCreate(ctx, TabSpec{
			WorkspaceID: workspaceID,
			Cwd:         tab.Cwd(),
			Label:       tab.Label,
			NoFocus:     true,
		})
		if err != nil {
			warnings = append(warnings, "could not open tab "+tab.Label+": "+err.Error())
			continue
		}
		if info.RootPaneID == "" {
			warnings = append(warnings, "could not open tab "+tab.Label+": herdr returned no root pane")
			continue
		}
		c.fillTab(ctx, tab, info.RootPaneID, &warnings)
	}
	return warnings, nil
}

// basePane resuelve el workspace y el pane base del montaje. Devolver el workspace
// junto al pane es lo que permite abrir los tabs siguientes en él.
func (c *Client) basePane(ctx context.Context, container Container, tab plan.Tab) (workspaceID, anchor string, err error) {
	if container.PaneID != "" {
		return container.WorkspaceID, container.PaneID, nil
	}
	if container.WorkspaceID == "" {
		// Sin contenedor (provisión con git directo, o worktree ya cerrado): el
		// layout abre su propio workspace. Es el camino previsto.
		return c.newWorkspace(ctx, tab)
	}
	// El llamador cree que este worktree vive en un workspace de Herdr. Si no sale
	// un pane de él, el id está obsoleto —Herdr lo guarda en su sesión persistida
	// y un workspace cerrado lo deja apuntando a nada— y abrir otro workspace
	// produciría un review desligado del worktree que lo contiene, sin avisar de
	// nada. Mejor fallar con el id en el mensaje que fingir un montaje correcto.
	panes, listErr := c.PaneList(ctx, container.WorkspaceID)
	if listErr != nil {
		return "", "", fmt.Errorf("the workspace %s of this worktree is gone: %w", container.WorkspaceID, listErr)
	}
	if len(panes) == 0 {
		return "", "", fmt.Errorf("the workspace %s of this worktree has no panes to mount on", container.WorkspaceID)
	}
	return container.WorkspaceID, panes[0].PaneID, nil
}

// newWorkspace abre el workspace del layout cuando no hay ninguno que reutilizar.
func (c *Client) newWorkspace(ctx context.Context, tab plan.Tab) (string, string, error) {
	ws, err := c.WorkspaceCreate(ctx, WorkspaceSpec{
		Cwd:     tab.Cwd(),
		Label:   tab.Label,
		NoFocus: true,
	})
	if err != nil {
		return "", "", fmt.Errorf("create the review workspace: %w", err)
	}
	if ws.RootPaneID == "" {
		return "", "", fmt.Errorf("herdr did not return a base pane for the layout")
	}
	return ws.WorkspaceID, ws.RootPaneID, nil
}

// fillTab puebla un tab ya abierto: el primer pane reutiliza el pane base y los
// siguientes se dividen encadenados (cada uno sobre el último creado) en la
// dirección que fija el plan.
func (c *Client) fillTab(ctx context.Context, tab plan.Tab, rootPaneID string, warnings *[]string) {
	parent := rootPaneID
	for i, p := range tab.Panes {
		if i > 0 {
			info, err := c.PaneSplit(ctx, SplitSpec{
				PaneID:    parent,
				Direction: direction(p),
				Ratio:     splitRatio,
				Cwd:       p.Cwd,
				Env:       p.Env,
				NoFocus:   true,
			})
			if err != nil {
				*warnings = append(*warnings, "could not open pane "+p.Label+": "+err.Error())
				continue
			}
			parent = info.PaneID
		}
		if err := c.PaneRun(ctx, parent, []string{paneCommand(p)}); err != nil {
			*warnings = append(*warnings, "could not run "+p.Label+": "+err.Error())
		}
		if err := c.PaneRename(ctx, parent, p.Label); err != nil {
			*warnings = append(*warnings, "could not label "+p.Label+": "+err.Error())
		}
	}
}

// tabOf devuelve el id del tab que contiene un pane. Herdr no tiene "el tab de
// este pane", así que se lista el workspace y se busca. Vacío si no se puede
// resolver: nombrar el tab es cosmético y no debe tumbar un layout ya montado.
func (c *Client) tabOf(ctx context.Context, workspaceID, paneID string) string {
	if paneID == "" {
		return ""
	}
	panes, err := c.PaneList(ctx, workspaceID)
	if err != nil {
		return ""
	}
	for _, p := range panes {
		if p.PaneID == paneID {
			return p.TabID
		}
	}
	return ""
}

// direction es la dirección de división del pane. Un plan que no la fija se
// abre a la derecha, que es la lectura de un diff junto a lo que lo comenta.
func direction(p plan.Pane) string {
	if p.Dir == "" {
		return plan.DirRight
	}
	return p.Dir
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
