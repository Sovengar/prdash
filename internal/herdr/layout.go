package herdr

import (
	"context"
	"fmt"
	"strings"

	"prdash/internal/review/plan"
)

const splitRatio = 0.5

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
	first := pl.Tabs[0]
	if id := c.tabOf(ctx, workspaceID, anchor); id != "" && first.Label != "" {
		if err := c.TabRename(ctx, id, first.Label); err != nil {
			warnings = append(warnings, "could not label tab "+first.Label+": "+err.Error())
		}
	}
	c.fillTab(ctx, first, anchor, &warnings)

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

func (c *Client) basePane(ctx context.Context, container Container, tab plan.Tab) (workspaceID, anchor string, err error) {
	if container.PaneID != "" {
		return container.WorkspaceID, container.PaneID, nil
	}
	if container.WorkspaceID == "" {
		return c.newWorkspace(ctx, tab)
	}
	panes, listErr := c.PaneList(ctx, container.WorkspaceID)
	if listErr != nil {
		return "", "", fmt.Errorf("the workspace %s of this worktree is gone: %w", container.WorkspaceID, listErr)
	}
	if len(panes) == 0 {
		return "", "", fmt.Errorf("the workspace %s of this worktree has no panes to mount on", container.WorkspaceID)
	}
	return container.WorkspaceID, panes[0].PaneID, nil
}

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

func direction(p plan.Pane) string {
	if p.Dir == "" {
		return plan.DirRight
	}
	return p.Dir
}

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

func shellQuote(s string) string {
	if s == "" {
		return "''"
	}
	if shellSafe(s) {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

func shellSafe(s string) bool {
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			continue
		}
		if r == '-' || r == '_' || r == '.' || r == '/' || r == '@' || r == ':' || r == '=' || r == '+' || r == ',' || r == '%' {
			continue
		}
		return false
	}
	return true
}
