package main

import (
	"os"
	"os/exec"
	"strings"

	"prdash/internal/config"
	"prdash/internal/forge/model"
	"prdash/internal/herdr"
	"prdash/internal/reporesolver"
	"prdash/internal/review/executor"
	"prdash/internal/review/plan"
	"prdash/internal/sim"
	"prdash/internal/worktree"
)

type simLocator struct{ ex *executor.Executor }

func (l simLocator) Locate(it model.Item) (sim.Place, bool) {
	wt, ok := l.ex.ActiveReview(it)
	if !ok || wt.Repo == "" {
		return sim.Place{}, false
	}
	return sim.Place{Repo: wt.Repo, Branch: wt.Branch}, true
}

func buildSimulator(cfg config.Config, ex *executor.Executor) *sim.Service {
	svc := sim.New(simLocator{ex: ex})
	if dir, err := sim.DefaultCacheDir(); err == nil {
		svc.CacheDir = dir
	}
	return svc
}

// Provisioner choice belongs to worktree.Select; the caller does not know which one runs.
func buildExecutor(cfg config.Config) *executor.Executor {
	client := herdr.New()
	tools := plan.Tools{
		Tuicr:  paneTool(cfg, "tuicr"),
		Hunk:   paneTool(cfg, "hunk"),
		Agent:  paneTool(cfg, "agent"),
		Editor: paneTool(cfg, "editor"),
	}
	return &executor.Executor{
		Resolver: reporesolver.New(reporesolver.Options{
			Roots:       cfg.Roots,
			CloneDir:    cfg.CloneDir,
			WorktreeDir: cfg.WorktreeDir,
			Hosts:       hostsOf(cfg),
			Prefixes:    clonePrefixesOf(cfg),
		}),
		Worktrees: worktree.Select(client, cfg.WorktreeDir),
		Herdr:     client,
		Tools:     tools,
		Env:       plan.Env{Available: toolAvailability(tools)},
	}
}

func hostsOf(cfg config.Config) map[string]string {
	hosts := map[string]string{}
	if h := cfg.Forges.GitHub.Host; h != "" {
		hosts[h] = "github"
	}
	if h := cfg.Forges.GitLab.Host; h != "" {
		hosts[h] = "gitlab"
	}
	return hosts
}

func clonePrefixesOf(cfg config.Config) map[string]string {
	prefixes := map[string]string{}
	if h := cfg.Forges.GitHub.Host; h != "" {
		if p := cfg.Forges.GitHub.ClonePrefix(); p != "" {
			prefixes[h] = p
		}
	}
	if h := cfg.Forges.GitLab.Host; h != "" {
		if p := cfg.Forges.GitLab.ClonePrefix(); p != "" {
			prefixes[h] = p
		}
	}
	return prefixes
}

func paneTool(cfg config.Config, name string) plan.Tool {
	if argv, ok := cfg.PaneOverride(name); ok {
		return plan.Tool{Argv: argv, Override: true}
	}
	return plan.Tool{Argv: cfg.ToolArgs(name)}
}

func toolAvailability(tools plan.Tools) map[string]bool {
	return map[string]bool{
		string(plan.KindTuicr): binaryAvailable(tools.Binary(plan.KindTuicr)),
		string(plan.KindHunk):  binaryAvailable(tools.Binary(plan.KindHunk)),
		string(plan.KindAgent): binaryAvailable(tools.Binary(plan.KindAgent)),
	}
}

func binaryAvailable(name string) bool {
	if strings.TrimSpace(name) == "" {
		return false
	}
	if strings.ContainsRune(name, os.PathSeparator) {
		info, err := os.Stat(name)
		return err == nil && !info.IsDir()
	}
	_, err := exec.LookPath(name)
	return err == nil
}
