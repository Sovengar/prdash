package main

import (
	"flag"
	"fmt"
	"io"
	"os"

	tea "charm.land/bubbletea/v2"

	"prdash/internal/config"
	"prdash/internal/forge"
	"prdash/internal/forge/bitbucket"
	"prdash/internal/forge/github"
	"prdash/internal/forge/gitlab"
	"prdash/internal/herdr"
	"prdash/internal/review/executor"
	"prdash/internal/tui"
	"prdash/internal/worktree"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

type mode int

const (
	modeTUI mode = iota
	modePrint
	modeWorktrees
)

type opts struct {
	mode mode
	sub  string
	args []string
}

func parseOpts(args []string) (opts, error) {
	o := opts{mode: modeTUI, sub: "list"}

	if len(args) > 0 && args[0] == "worktrees" {
		o.mode = modeWorktrees
		o.args = args[1:]
		if len(o.args) > 0 && o.args[0] != "" {
			o.sub = o.args[0]
		}
		return o, nil
	}

	fs := flag.NewFlagSet("prdash", flag.ContinueOnError)
	fs.SetOutput(io.Discard) // `run` formats the errors, on the caller's stderr
	print := fs.Bool("print", false, "print the inbox and exit")
	if err := fs.Parse(args); err != nil {
		return opts{}, err
	}
	if *print {
		o.mode = modePrint
	}
	o.args = fs.Args()
	return o, nil
}

func run(args []string, stdout, stderr io.Writer) int {
	o, err := parseOpts(args)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "prdash:", err)
		return 2
	}

	cfg, warn := config.Load()
	if warn != "" {
		_, _ = fmt.Fprintln(stderr, "prdash:", warn)
	}

	if o.mode == modeWorktrees {
		return runWorktrees(worktree.Select(herdr.New(), cfg.WorktreeDir), stdout, stderr, o.args)
	}

	adapters := buildAdapters(cfg)
	if len(adapters) == 0 {
		_, _ = fmt.Fprintln(stderr, "prdash: no forges enabled in the config")
	}

	ex := buildExecutor(cfg)

	if o.mode == modePrint {
		runPrintTo(stdout, stderr, adapters, ex.ActiveReview)
		return 0
	}

	model := wire(cfg, adapters, ex)
	if err := startTUI(model); err != nil {
		_, _ = fmt.Fprintln(stderr, "prdash:", err)
		return 1
	}
	return 0
}

var startTUI = func(m tea.Model) error {
	_, err := tea.NewProgram(m).Run()
	return err
}

func wire(cfg config.Config, adapters []forge.Adapter, ex *executor.Executor) tui.Model {
	model := tui.New(cfg, adapters)
	model.SetMounter(ex)
	model.SetSimulator(buildSimulator(cfg, ex))
	model.SetGraphics(herdr.NewGraphics())
	model.SetReviewLookup(ex)
	model.SetReviewRemover(ex)
	return model
}

func buildAdapters(cfg config.Config) []forge.Adapter {
	var adapters []forge.Adapter
	if cfg.Forges.GitHub.Enabled {
		adapters = append(adapters, github.New(cfg.Forges.GitHub.Host, cfg.Tools.GH))
	}
	if cfg.Forges.GitLab.Enabled {
		adapters = append(adapters, gitlab.New(cfg.Forges.GitLab.Host, cfg.Tools.Glab))
	}
	if cfg.Forges.Bitbucket.Enabled {
		adapters = append(adapters, bitbucket.New("bitbucket.org"))
	}
	return adapters
}
