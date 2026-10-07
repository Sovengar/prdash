package worktree

import (
	"context"
	"fmt"
	"path/filepath"

	"prdash/internal/herdr"
)

type HerdrRunner interface {
	Available() bool
	WorktreeCreate(ctx context.Context, spec herdr.WorktreeSpec) (herdr.WorktreeInfo, error)
	WorktreeList(ctx context.Context, cwd string) ([]herdr.WorktreeInfo, error)
	WorktreeRemove(ctx context.Context, workspaceID string, force bool) error
	WorkspaceCreate(ctx context.Context, spec herdr.WorkspaceSpec) (herdr.WorkspaceInfo, error)
	PaneList(ctx context.Context, workspaceID string) ([]herdr.PaneInfo, error)
}

// Tied to a Herdr workspace only inside Herdr; prdash still resolves the fetch and branch.
type HerdrNative struct {
	client HerdrRunner
	scan   *GitDirect
}

func NewHerdrNative(client HerdrRunner, base string) *HerdrNative {
	return &HerdrNative{client: client, scan: NewGitDirect(base)}
}

func Select(client HerdrRunner, base string) Provisioner {
	if client != nil && client.Available() {
		return NewHerdrNative(client, base)
	}
	return NewGitDirect(base)
}

func (h *HerdrNative) Create(ctx context.Context, spec Spec) (Worktree, error) {
	if spec.Repo == "" || spec.Branch == "" || spec.Path == "" {
		return Worktree{}, fmt.Errorf("worktree: incomplete spec (repo, branch and destination are required)")
	}
	if Exists(spec.Path) {
		return h.reuse(ctx, spec)
	}

	info, err := h.client.WorktreeCreate(ctx, herdr.WorktreeSpec{
		Cwd:     spec.Repo,
		Branch:  spec.Branch,
		Path:    spec.Path,
		Label:   spec.Label,
		NoFocus: true,
	})
	if err != nil {
		return Worktree{}, fmt.Errorf("create the native worktree at %s: %w", spec.Path, err)
	}

	return composed(spec, info), nil
}

// Herdr's field wins over the caller's — a wrong precedence has no visible error. The label excepts.
func composed(spec Spec, info herdr.WorktreeInfo) Worktree {
	wt := Worktree{
		ID:          spec.Path,
		Label:       spec.Label,
		Path:        spec.Path,
		Branch:      spec.Branch,
		Repo:        spec.Repo,
		WorkspaceID: info.WorkspaceID,
		RootPaneID:  info.RootPaneID,
	}
	if info.Path != "" {
		wt.Path = info.Path
		wt.ID = info.Path
	}
	if info.Branch != "" {
		wt.Branch = info.Branch
	}
	if wt.Label == "" {
		wt.Label = info.WorkspaceLabel
	}
	if wt.Label == "" {
		wt.Label = info.Label
	}
	if wt.Label == "" && wt.Path != "" {
		wt.Label = filepath.Base(wt.Path)
	}
	return wt
}

// Reusing another branch's checkout is a fake mount; a checkout with no open workspace gets one.
func (h *HerdrNative) reuse(ctx context.Context, spec Spec) (Worktree, error) {
	existing, ok, err := h.scan.inspect(ctx, spec.Path)
	if err != nil {
		return Worktree{}, err
	}
	if !ok {
		return Worktree{}, fmt.Errorf("worktree: %s does not hold a linked worktree", spec.Path)
	}
	if existing.Branch != spec.Branch {
		return Worktree{}, fmt.Errorf("worktree: %s already holds branch %s, not %s", spec.Path, existing.Branch, spec.Branch)
	}

	wt := existing
	wt.Repo = spec.Repo
	if spec.Label != "" {
		wt.Label = spec.Label
	}
	if err := h.attach(ctx, &wt, spec); err != nil {
		return Worktree{}, err
	}
	return wt, nil
}

func (h *HerdrNative) attach(ctx context.Context, wt *Worktree, spec Spec) error {
	if infos, err := h.client.WorktreeList(ctx, spec.Repo); err == nil {
		for _, info := range infos {
			if info.Path != spec.Path || info.OpenWorkspaceID == "" {
				continue
			}
			if panes, err := h.client.PaneList(ctx, info.OpenWorkspaceID); err == nil && len(panes) > 0 {
				wt.WorkspaceID = info.OpenWorkspaceID
				wt.RootPaneID = panes[0].PaneID
				return nil
			}
			break // the id is not reliable: fall back to adoption
		}
	}
	info, err := h.client.WorkspaceCreate(ctx, herdr.WorkspaceSpec{
		Cwd:     spec.Path,
		Label:   spec.Label,
		NoFocus: true,
	})
	if err != nil {
		return fmt.Errorf("open a Herdr workspace for the worktree at %s (remove it with `prdash worktrees remove %s` and mount again to recreate it): %w", spec.Path, spec.Path, err)
	}
	wt.WorkspaceID = info.WorkspaceID
	wt.RootPaneID = info.RootPaneID
	return nil
}

func (h *HerdrNative) Remove(ctx context.Context, id string) error {
	if err := h.scan.removablePath(id); err != nil {
		return err
	}
	if repo := mainRepoOf(id); repo != "" {
		if infos, err := h.client.WorktreeList(ctx, repo); err == nil {
			for _, info := range infos {
				if info.Path == id && info.OpenWorkspaceID != "" {
					if err := h.client.WorktreeRemove(ctx, info.OpenWorkspaceID, true); err != nil {
						return fmt.Errorf("remove the native worktree %s: %w", id, err)
					}
					return nil
				}
			}
		}
	}
	return h.scan.Remove(ctx, id)
}

func (h *HerdrNative) RemoveIfClean(ctx context.Context, id string) (bool, string, error) {
	ok, reason, err := h.scan.shouldRemove(ctx, id)
	if err != nil || !ok {
		return false, reason, err
	}
	if err := h.Remove(ctx, id); err != nil {
		return false, "", err
	}
	return true, "", nil
}

// Native worktrees are real git worktrees, so the listing is the same scan.
func (h *HerdrNative) List(ctx context.Context) []Worktree { return h.scan.List(ctx) }

func (h *HerdrNative) Audit(ctx context.Context) []Entry { return h.scan.Audit(ctx) }
