package herdr

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

type envelope[T any] struct {
	Result T `json:"result"`
}

func decodeResult[T any](raw []byte) (T, error) {
	var e envelope[T]
	if err := json.Unmarshal(raw, &e); err != nil {
		return e.Result, fmt.Errorf("herdr: respuesta ilegible: %w", err)
	}
	return e.Result, nil
}

type worktreeCreatedResult struct {
	Workspace struct {
		WorkspaceID string `json:"workspace_id"`
		Label       string `json:"label"`
	} `json:"workspace"`
	Tab struct {
		TabID string `json:"tab_id"`
	} `json:"tab"`
	RootPane struct {
		PaneID string `json:"pane_id"`
	} `json:"root_pane"`
	Worktree struct {
		Path             string `json:"path"`
		Branch           string `json:"branch"`
		Label            string `json:"label"`
		IsLinkedWorktree bool   `json:"is_linked_worktree"`
		OpenWorkspaceID  string `json:"open_workspace_id"`
	} `json:"worktree"`
}

func parseWorktreeCreated(raw []byte) (WorktreeInfo, error) {
	r, err := decodeResult[worktreeCreatedResult](raw)
	if err != nil {
		return WorktreeInfo{}, err
	}
	if r.Worktree.Path == "" {
		return WorktreeInfo{}, fmt.Errorf("herdr: worktree create without .result.worktree.path")
	}
	return WorktreeInfo{
		WorkspaceID:      r.Workspace.WorkspaceID,
		WorkspaceLabel:   r.Workspace.Label,
		TabID:            r.Tab.TabID,
		RootPaneID:       r.RootPane.PaneID,
		Path:             r.Worktree.Path,
		Branch:           r.Worktree.Branch,
		Label:            r.Worktree.Label,
		OpenWorkspaceID:  r.Worktree.OpenWorkspaceID,
		IsLinkedWorktree: r.Worktree.IsLinkedWorktree,
	}, nil
}

type worktreeListResult struct {
	Source struct {
		RepoKey           string `json:"repo_key"`
		RepoName          string `json:"repo_name"`
		RepoRoot          string `json:"repo_root"`
		SourceWorkspaceID string `json:"source_workspace_id"`
	} `json:"source"`
	Worktrees []struct {
		Path             string `json:"path"`
		Branch           string `json:"branch"`
		Label            string `json:"label"`
		OpenWorkspaceID  string `json:"open_workspace_id"`
		IsLinkedWorktree bool   `json:"is_linked_worktree"`
		IsPrunable       bool   `json:"is_prunable"`
	} `json:"worktrees"`
}

type worktreeList struct {
	RepoRoot          string
	SourceWorkspaceID string
	Worktrees         []WorktreeInfo
}

func parseWorktreeList(raw []byte) (worktreeList, error) {
	r, err := decodeResult[worktreeListResult](raw)
	if err != nil {
		return worktreeList{}, err
	}
	out := worktreeList{RepoRoot: r.Source.RepoRoot, SourceWorkspaceID: r.Source.SourceWorkspaceID}
	for _, w := range r.Worktrees {
		out.Worktrees = append(out.Worktrees, WorktreeInfo{
			Path:             w.Path,
			Branch:           w.Branch,
			Label:            w.Label,
			OpenWorkspaceID:  w.OpenWorkspaceID,
			IsLinkedWorktree: w.IsLinkedWorktree,
		})
	}
	return out, nil
}

type workspaceCreatedResult struct {
	Workspace struct {
		WorkspaceID string `json:"workspace_id"`
	} `json:"workspace"`
	Tab struct {
		TabID string `json:"tab_id"`
	} `json:"tab"`
	RootPane struct {
		PaneID string `json:"pane_id"`
	} `json:"root_pane"`
}

func parseWorkspaceCreated(raw []byte) (WorkspaceInfo, error) {
	r, err := decodeResult[workspaceCreatedResult](raw)
	if err != nil {
		return WorkspaceInfo{}, err
	}
	if r.Workspace.WorkspaceID == "" {
		return WorkspaceInfo{}, fmt.Errorf("herdr: workspace create without .result.workspace.workspace_id")
	}
	return WorkspaceInfo{WorkspaceID: r.Workspace.WorkspaceID, TabID: r.Tab.TabID, RootPaneID: r.RootPane.PaneID}, nil
}

type tabCreatedResult struct {
	Tab struct {
		TabID string `json:"tab_id"`
	} `json:"tab"`
	RootPane struct {
		PaneID string `json:"pane_id"`
	} `json:"root_pane"`
}

func parseTabCreated(raw []byte) (TabInfo, error) {
	r, err := decodeResult[tabCreatedResult](raw)
	if err != nil {
		return TabInfo{}, err
	}
	return TabInfo{TabID: r.Tab.TabID, RootPaneID: r.RootPane.PaneID}, nil
}

type paneSplitResult struct {
	Pane struct {
		PaneID      string `json:"pane_id"`
		WorkspaceID string `json:"workspace_id"`
		TabID       string `json:"tab_id"`
		Cwd         string `json:"cwd"`
		Label       string `json:"label"`
	} `json:"pane"`
}

func parsePaneSplit(raw []byte) (PaneInfo, error) {
	r, err := decodeResult[paneSplitResult](raw)
	if err != nil {
		return PaneInfo{}, err
	}
	if r.Pane.PaneID == "" {
		return PaneInfo{}, fmt.Errorf("herdr: pane split without .result.pane.pane_id")
	}
	return PaneInfo{PaneID: r.Pane.PaneID, WorkspaceID: r.Pane.WorkspaceID, TabID: r.Pane.TabID, Cwd: r.Pane.Cwd, Label: r.Pane.Label}, nil
}

type paneListResult struct {
	Panes []struct {
		PaneID      string `json:"pane_id"`
		WorkspaceID string `json:"workspace_id"`
		TabID       string `json:"tab_id"`
		Cwd         string `json:"cwd"`
		Label       string `json:"label"`
	} `json:"panes"`
}

func parsePaneList(raw []byte) ([]PaneInfo, error) {
	r, err := decodeResult[paneListResult](raw)
	if err != nil {
		return nil, err
	}
	out := make([]PaneInfo, 0, len(r.Panes))
	for _, p := range r.Panes {
		out = append(out, PaneInfo{PaneID: p.PaneID, WorkspaceID: p.WorkspaceID, TabID: p.TabID, Cwd: p.Cwd, Label: p.Label})
	}
	return out, nil
}

type notificationResult struct {
	Shown  bool   `json:"shown"`
	Reason string `json:"reason"`
}

func parseNotification(raw []byte) (bool, string, error) {
	r, err := decodeResult[notificationResult](raw)
	if err != nil {
		return false, "", err
	}
	return r.Shown, r.Reason, nil
}

var versionRe = regexp.MustCompile(`(\d+)\.(\d+)\.(\d+)`)

// ("herdr 0.9.1-preview.…").
func parseVersion(raw []byte) (Version, bool) {
	m := versionRe.FindSubmatch(raw)
	if m == nil {
		return Version{}, false
	}
	major, _ := strconv.Atoi(string(m[1]))
	minor, _ := strconv.Atoi(string(m[2]))
	patch, _ := strconv.Atoi(string(m[3]))
	return Version{Major: major, Minor: minor, Patch: patch, Raw: strings.TrimSpace(string(raw))}, true
}

// Best-effort: the exact server shape is unverified, so both variants are accepted.
func parseServerError(stderr []byte) (code, msg string) {
	text := strings.TrimSpace(string(stderr))
	if text == "" {
		return "", ""
	}
	var generic map[string]json.RawMessage
	if err := json.Unmarshal([]byte(text), &generic); err != nil {
		return "", ""
	}
	if raw, ok := generic["error"]; ok {
		var inner struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		}
		if json.Unmarshal(raw, &inner) == nil {
			return inner.Code, inner.Message
		}
	}
	var flat struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	}
	if json.Unmarshal([]byte(text), &flat) == nil {
		return flat.Code, flat.Message
	}
	return "", ""
}

func firstLine(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return strings.TrimSpace(s[:i])
	}
	return s
}
