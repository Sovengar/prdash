// Package worktree provisions review worktrees: the port the orchestrator uses plus a direct-git
// implementation. The native Herdr one fulfils the same contract, so the caller never knows which runs.
package worktree

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"prdash/internal/gitcmd"
)

type Spec struct {
	Repo   string
	Branch string
	Path   string
	Label  string
}

type Worktree struct {
	ID          string
	Label       string
	Path        string
	Branch      string
	Repo        string
	WorkspaceID string
	RootPaneID  string
}

// The reasons RemoveIfClean keeps a worktree: the answer to "I did not delete it, and this is why".
const (
	// archivos sin trackear).
	KeptUncommitted = "the worktree has uncommitted changes"
	KeptUnreadable  = "could not read the worktree status"
)

type Provisioner interface {
	Create(ctx context.Context, spec Spec) (Worktree, error)
	Remove(ctx context.Context, id string) error
	RemoveIfClean(ctx context.Context, id string) (removed bool, reason string, err error)
	List(ctx context.Context) []Worktree
	Audit(ctx context.Context) []Entry
}

type GitDirect struct {
	// borrados de limpieza.
	Base string
	git  *gitcmd.Runner
}

func NewGitDirect(base string) *GitDirect {
	return &GitDirect{Base: base, git: gitcmd.New()}
}

func (g *GitDirect) Create(ctx context.Context, spec Spec) (Worktree, error) {
	if spec.Repo == "" || spec.Branch == "" || spec.Path == "" {
		return Worktree{}, fmt.Errorf("worktree: incomplete spec (repo, branch and destination are required)")
	}

	if wt, ok, err := g.inspect(ctx, spec.Path); err != nil {
		return Worktree{}, err
	} else if ok {
		if wt.Branch != spec.Branch {
			return Worktree{}, fmt.Errorf("worktree: %s already holds branch %s, not %s", spec.Path, wt.Branch, spec.Branch)
		}
		return wt, nil
	}

	if err := os.MkdirAll(filepath.Dir(spec.Path), 0o755); err != nil {
		return Worktree{}, fmt.Errorf("prepare the worktree destination %s: %w", spec.Path, err)
	}
	if _, err := g.git.Run(ctx, spec.Repo, "worktree", "add", "--quiet", spec.Path, spec.Branch); err != nil {
		g.cleanPartial(spec.Path)
		return Worktree{}, fmt.Errorf("create the worktree at %s: %w", spec.Path, err)
	}

	label := spec.Label
	if label == "" {
		label = filepath.Base(spec.Path)
	}
	return Worktree{ID: spec.Path, Label: label, Path: spec.Path, Branch: spec.Branch, Repo: spec.Repo}, nil
}

// The main repo is resolved from the worktree's own .git file, so nothing depends on in-memory state.
func (g *GitDirect) Remove(ctx context.Context, id string) error {
	if err := g.removablePath(id); err != nil {
		return err
	}
	repo := mainRepoOf(id)
	if repo == "" {
		if !isLinkedWorktree(id) {
			return fmt.Errorf("worktree: could not locate the repo for %s", id)
		}
		if err := os.RemoveAll(id); err != nil {
			return fmt.Errorf("remove the worktree checkout %s: %w", id, err)
		}
		return nil
	}
	if _, err := g.git.Run(ctx, repo, "worktree", "remove", "--force", id); err == nil {
		return nil
	}
	if sourceReachable(id) {
		_, _ = g.git.Run(ctx, repo, "worktree", "prune")
	}
	if err := os.RemoveAll(id); err != nil {
		return fmt.Errorf("remove the worktree checkout %s: %w", id, err)
	}
	return nil
}

func (g *GitDirect) removablePath(id string) error {
	if !Owned(filepath.Base(id), id) {
		return fmt.Errorf("worktree: %s is not a prdash worktree; leaving it alone", id)
	}
	if g.Base != "" {
		rel, err := filepath.Rel(g.Base, id)
		if err != nil || rel == "." || strings.HasPrefix(rel, "..") {
			return fmt.Errorf("worktree: %s is outside the managed root", id)
		}
	}
	return nil
}

// Fail-safe: an unreadable state is also a reason to keep.
func (g *GitDirect) shouldRemove(ctx context.Context, id string) (ok bool, reason string, err error) {
	if err := g.removablePath(id); err != nil {
		return false, "", err
	}
	if !Exists(id) {
		return false, "", nil
	}
	dirty, err := g.dirty(ctx, id)
	if err != nil {
		return false, KeptUnreadable, nil
	}
	if dirty {
		return false, KeptUncommitted, nil
	}
	return true, "", nil
}

// The auto-delete path after a merge: unlike Remove it never destroys uncommitted work.
func (g *GitDirect) RemoveIfClean(ctx context.Context, id string) (bool, string, error) {
	ok, reason, err := g.shouldRemove(ctx, id)
	if err != nil || !ok {
		return false, reason, err
	}
	if err := g.Remove(ctx, id); err != nil {
		return false, "", err
	}
	return true, "", nil
}

// `status --porcelain`, not `diff --quiet`: a new untracked file is uncommitted work and `diff` ignores it.
func (g *GitDirect) dirty(ctx context.Context, id string) (bool, error) {
	out, err := g.git.Run(ctx, id, "status", "--porcelain")
	if err != nil {
		return false, err
	}
	return strings.TrimSpace(out) != "", nil
}

func (g *GitDirect) List(ctx context.Context) []Worktree {
	entries := g.Audit(ctx)
	out := make([]Worktree, 0, len(entries))
	for _, e := range entries {
		out = append(out, e.Worktree)
	}
	return out
}

func Exists(path string) bool { return isLinkedWorktree(path) }

func (g *GitDirect) inspect(ctx context.Context, path string) (Worktree, bool, error) {
	info, err := os.Lstat(filepath.Join(path, ".git"))
	if err != nil {
		return Worktree{}, false, nil
	}
	if info.IsDir() {
		return Worktree{}, false, fmt.Errorf("worktree: %s already exists and is not a linked worktree", path)
	}
	branch, _ := g.git.Run(ctx, path, "rev-parse", "--abbrev-ref", "HEAD")
	return Worktree{
		ID:     path,
		Label:  filepath.Base(path),
		Path:   path,
		Branch: strings.TrimSpace(branch),
		Repo:   mainRepoOf(path),
	}, true, nil
}

func (g *GitDirect) cleanPartial(path string) {
	if g.Base != "" {
		rel, err := filepath.Rel(g.Base, path)
		if err != nil || strings.HasPrefix(rel, "..") {
			return
		}
	}
	if isLinkedWorktree(path) {
		return
	}
	_ = os.RemoveAll(path)
}

func isLinkedWorktree(path string) bool {
	info, err := os.Lstat(filepath.Join(path, ".git"))
	return err == nil && !info.IsDir()
}

func mainRepoOf(path string) string {
	gitdir := linkedGitDir(path)
	if gitdir == "" {
		return ""
	}
	return filepath.Dir(filepath.Dir(gitdir))
}
