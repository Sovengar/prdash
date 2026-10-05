package worktree

import (
	"cmp"
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// A worktree with any other name is never listed and never deleted.
const LabelPrefix = "prdash-"

// The only gate into cleanup: whatever is not ours is not touched.
func Owned(label, path string) bool {
	return strings.HasPrefix(label, LabelPrefix) || strings.HasPrefix(filepath.Base(path), LabelPrefix)
}

type Entry struct {
	Worktree
	// Lets cleanup report it before deleting, so an unreachable source is visible.
	Orphan bool
	Reason string
}

func (g *GitDirect) Audit(ctx context.Context) []Entry {
	if g.Base == "" {
		return nil
	}
	var out []Entry
	_ = filepath.WalkDir(g.Base, func(path string, d fs.DirEntry, err error) error {
		if err != nil || !d.IsDir() {
			return nil
		}
		if d.Name() == ".git" {
			return fs.SkipDir
		}
		if !isLinkedWorktree(path) {
			return nil
		}
		if !Owned(filepath.Base(path), path) {
			return fs.SkipDir
		}
		branch, _ := g.git.Run(ctx, path, "rev-parse", "--abbrev-ref", "HEAD")
		e := Entry{Worktree: Worktree{
			ID:     path,
			Label:  filepath.Base(path),
			Path:   path,
			Branch: strings.TrimSpace(branch),
			Repo:   mainRepoOf(path),
		}}
		if !sourceReachable(path) {
			e.Orphan = true
			e.Reason = "the worktree source repo is no longer reachable"
		}
		out = append(out, e)
		return fs.SkipDir
	})
	slices.SortFunc(out, func(a, b Entry) int { return cmp.Compare(a.Path, b.Path) })
	return out
}

func linkedGitDir(path string) string {
	raw, err := os.ReadFile(filepath.Join(path, ".git"))
	if err != nil {
		return ""
	}
	rest, ok := strings.CutPrefix(strings.TrimSpace(string(raw)), "gitdir:")
	if !ok {
		return ""
	}
	gitdir := strings.TrimSpace(rest)
	if gitdir == "" {
		return ""
	}
	if !filepath.IsAbs(gitdir) {
		gitdir = filepath.Join(path, gitdir)
	}
	return gitdir
}

func sourceReachable(path string) bool {
	gitdir := linkedGitDir(path)
	if gitdir == "" {
		return false
	}
	_, err := os.Stat(gitdir)
	return err == nil
}
