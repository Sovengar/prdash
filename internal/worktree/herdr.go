package worktree

import (
	"context"
	"fmt"
	"path/filepath"

	"prdash/internal/herdr"
)

// nativa. *herdr.Client lo implementa.
type HerdrRunner interface {
	Available() bool
	WorktreeCreate(ctx context.Context, spec herdr.WorktreeSpec) (herdr.WorktreeInfo, error)
	WorktreeList(ctx context.Context, cwd string) ([]herdr.WorktreeInfo, error)
	WorktreeRemove(ctx context.Context, workspaceID string, force bool) error
	WorkspaceCreate(ctx context.Context, spec herdr.WorkspaceSpec) (herdr.WorkspaceInfo, error)
	PaneList(ctx context.Context, workspaceID string) ([]herdr.PaneInfo, error)
}

// The checkout is tied to a Herdr workspace, the container the layout uses. Only inside Herdr;
// prdash still resolves the fetch and the local branch.
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

// Pure on purpose: precedence rules between two sources are exactly what cannot be read by eye in a
// provisioning body.
//
// The rule is one: for EACH FIELD, what Herdr said wins if it said anything, and what the caller asked for
// otherwise. Herdr wins because it is the one that knows: if it moved the checkout, or renamed the branch,
// the Worktree has to reflect where it really is. Lying here has no visible error, it yields a worktree
// pointing at nothing with the review mounted on empty space.
//
// The exception is the label. spec.Label is the OWNERSHIP label prdash recognises the worktree by, while
// info.Label is the repo name the native reports and info.WorkspaceLabel the --label the workspace was
// opened with: neither is what was asked. So the label is not taken from Herdr when the caller gave one,
// and the others are only a fallback in that order. Inverted, the worktree would rename itself to the
// repo name and prdash would stop recognising it.
//
// The path name is the last resort because it is the only one always there.
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

// The checkout must match the branch asked for, like the direct-git provisioning: reusing another
// branch's checkout would be a fake mount.
//
// A checkout with no open workspace gets one opened with cwd inside the worktree instead of being
// returned containerless. Otherwise the layout opens its own workspace and the review shows up as a
// loose workspace detached from the worktree containing it. `herdr worktree create` cannot cover this:
// it does `git worktree add` internally and refuses an existing path, but `workspace create` with that
// cwd does get registered as the worktree's open workspace (verified on Herdr 0.9.1).
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
			break // el id no es fiable: se cae a la adopción
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
