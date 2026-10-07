package executor

import (
	"context"
	"fmt"

	"prdash/internal/cache"
	"prdash/internal/forge/model"
	"prdash/internal/herdr"
	"prdash/internal/review/plan"
	"prdash/internal/worktree"
)

type Resolver interface {
	ResolveLocal(ref model.RepoRef) (string, bool)
	HasBare(ref model.RepoRef) bool
	EnsureBare(ctx context.Context, ref model.RepoRef) (string, error)
	RemoveBare(ref model.RepoRef) error
	FetchReviewRef(ctx context.Context, repo string, it model.Item) (string, error)
	WorktreePath(ref model.RepoRef, number int) string
	Remember(ref model.RepoRef, path string)
	RecordReview(it model.Item, rec cache.ReviewRecord) error
	ActiveReview(id model.ID) (cache.ReviewRecord, bool)
	ForgetReview(it model.Item) error
}

type HerdrPort interface {
	Available() bool
	MountLayout(ctx context.Context, container herdr.Container, pl plan.Plan) ([]string, error)
	Notify(ctx context.Context, title string, opts herdr.NotifyOptions) error
}

type Executor struct {
	Resolver  Resolver
	Worktrees worktree.Provisioner
	Herdr     HerdrPort
	Tools     plan.Tools
	Env       plan.Env
	// plan.Build.
	Planner func(pr model.Item, wt plan.Worktree) plan.Plan
}

type Result struct {
	RepoPath string
	Branch   string
	Worktree worktree.Worktree
	Plan     plan.Plan
	Reused   bool
	Herdr    bool
	Warnings []string
}

func (e *Executor) Mount(ctx context.Context, it model.Item) (Result, error) {
	var res Result

	repoPath, createdBare, err := e.resolveRepo(ctx, it)
	if err != nil {
		return res, err
	}
	res.RepoPath = repoPath

	wtPath := e.worktreePath(it)
	branch, err := e.Resolver.FetchReviewRef(ctx, repoPath, it)
	if err != nil {
		e.cleanupBare(createdBare, it)
		return res, err
	}
	res.Branch = branch

	res.Reused = worktree.Exists(wtPath)
	wt, err := e.Worktrees.Create(ctx, worktree.Spec{
		Repo:   repoPath,
		Branch: branch,
		Path:   wtPath,
		Label:  Label(it.Number),
	})
	if err != nil {
		e.cleanupBare(createdBare, it)
		return res, err
	}
	res.Worktree = wt

	res.Plan = e.buildPlan(it, wt)
	res.Warnings = append(res.Warnings, res.Plan.Warnings...)
	res.Herdr = e.mountLayout(ctx, wt, res.Plan, &res)

	if err := e.Resolver.RecordReview(it, cache.ReviewRecord{
		Repo:     repoPath,
		Worktree: wt.Path,
		Branch:   branch,
		Label:    wt.Label,
	}); err != nil {
		res.Warnings = append(res.Warnings, "could not register the active review: "+err.Error())
	}
	return res, nil
}

func (e *Executor) ActiveReview(it model.Item) (worktree.Worktree, bool) {
	rec, ok := e.Resolver.ActiveReview(it.ID())
	if !ok || rec.Worktree == "" {
		return worktree.Worktree{}, false
	}
	return worktree.Worktree{
		ID:     rec.Worktree,
		Label:  rec.Label,
		Path:   rec.Worktree,
		Branch: rec.Branch,
		Repo:   rec.Repo,
	}, true
}

func (e *Executor) RemoveReview(ctx context.Context, it model.Item) (bool, string, error) {
	rec, ok := e.Resolver.ActiveReview(it.ID())
	if !ok || rec.Worktree == "" {
		return false, "", nil
	}
	removed, reason, err := e.Worktrees.RemoveIfClean(ctx, rec.Worktree)
	if err != nil {
		return false, reason, err
	}
	if !removed {
		return false, reason, nil
	}
	_ = e.Resolver.ForgetReview(it)
	return true, "", nil
}

func (e *Executor) resolveRepo(ctx context.Context, it model.Item) (path string, created bool, err error) {
	if p, ok := e.Resolver.ResolveLocal(it.Ref); ok {
		return p, false, nil
	}
	hadBare := e.Resolver.HasBare(it.Ref)
	p, err := e.Resolver.EnsureBare(ctx, it.Ref)
	if err != nil {
		return "", false, err
	}
	e.Resolver.Remember(it.Ref, p)
	return p, !hadBare, nil
}

func (e *Executor) worktreePath(it model.Item) string {
	if rec, ok := e.Resolver.ActiveReview(it.ID()); ok && rec.Worktree != "" {
		return rec.Worktree
	}
	return e.Resolver.WorktreePath(it.Ref, it.Number)
}

func (e *Executor) buildPlan(it model.Item, wt worktree.Worktree) plan.Plan {
	pw := plan.Worktree{Path: wt.Path, Branch: wt.Branch, Label: wt.Label}
	if e.Planner != nil {
		return e.Planner(it, pw)
	}
	return plan.Build(it, pw, e.Tools, e.Env)
}

func (e *Executor) mountLayout(ctx context.Context, wt worktree.Worktree, pl plan.Plan, res *Result) bool {
	if e.Herdr == nil || !e.Herdr.Available() {
		res.Warnings = append(res.Warnings, "the review layout requires Herdr; the worktree was mounted")
		return false
	}
	container := herdr.Container{WorkspaceID: wt.WorkspaceID, PaneID: wt.RootPaneID}
	warns, err := e.Herdr.MountLayout(ctx, container, pl)
	res.Warnings = append(res.Warnings, warns...)
	if err != nil {
		res.Warnings = append(res.Warnings, fmt.Sprintf("could not open the layout: %v", err))
		return false
	}
	_ = e.Herdr.Notify(ctx, "prdash: review ready", herdr.NotifyOptions{Sound: "done"})
	return true
}

func (e *Executor) cleanupBare(created bool, it model.Item) {
	if !created {
		return
	}
	_ = e.Resolver.RemoveBare(it.Ref)
}

func Label(number int) string { return fmt.Sprintf("prdash-pr-%d", number) }
