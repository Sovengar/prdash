// prdash — cross-forge inbox of PRs/MRs. Read-only: it queries GitHub through `gh` and a
// self-managed GitLab through `glab`, and shows created-by-me, review and mentions.
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

// The shell: pick the mode, warn about config, delegate. Everything below returns a value so it
// can be tested; what is left here are the three things that cannot be abstracted without inventing
// something worse: read os.Args, load config from disk, and exit the process.
func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

type mode int

const (
	modeTUI mode = iota
	modePrint
	modeWorktrees
)

// The mode is the ONLY way to say what to do. An earlier version also had a `print bool`, and having
// both is worse than having neither: `run` ended up reading both and any divergence surfaced as a
// mode that cannot happen.
type opts struct {
	mode mode
	sub  string
	args []string
}

// Own FlagSet rather than the global one, which `flag.Parse` mutates forever and test order does
// not guarantee. The subcommand is read BEFORE the flags: the paths `worktrees remove` manages are
// absolute, so they cannot start with `-`, and reading flags first would eat `--orphans` as a prdash flag.
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

	// Own FlagSet, not the global singleton: a test using the global would contaminate the rest of the
	// suite, and test order within a binary is not guaranteed.
	fs := flag.NewFlagSet("prdash", flag.ContinueOnError)
	fs.SetOutput(io.Discard) // los errores los formatea `run`, con el stderr del llamador
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

// Three injected inputs and a returned exit code instead of os.Exit, which is what makes the whole
// body testable.
func run(args []string, stdout, stderr io.Writer) int {
	o, err := parseOpts(args)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, "prdash:", err)
		return 2
	}

	cfg, warn := config.Load()
	if warn != "" {
		// Warn without aborting: bad config degrades to defaults and the UI must still come up.
		_, _ = fmt.Fprintln(stderr, "prdash:", warn)
	}

	if o.mode == modeWorktrees {
		return runWorktrees(worktree.Select(herdr.New(), cfg.WorktreeDir), stdout, stderr, o.args)
	}

	adapters := buildAdapters(cfg)
	if len(adapters) == 0 {
		// Not fatal: the inbox just comes up empty, which is what makes opening it useful to see WHY.
		_, _ = fmt.Fprintln(stderr, "prdash: no forges enabled in the config")
	}

	// One executor for all three modes, the review registry and the simulator.
	ex := buildExecutor(cfg)

	if o.mode == modePrint {
		runPrintTo(stdout, stderr, adapters, ex.ActiveReview)
		return 0
	}

	model := wire(cfg, adapters, ex)
	if err := arrancaTUI(model); err != nil {
		_, _ = fmt.Fprintln(stderr, "prdash:", err)
		return 1
	}
	return 0
}

// Own function so that `run` does not depend on a terminal: `tea.NewProgram(m).Run` needs a TTY, so
// without this the `return 0` path of a UI that DOES start was untestable while the failure path was.
var arrancaTUI = func(m tea.Model) error {
	_, err := tea.NewProgram(m).Run()
	return err
}

// Its own function because wiring is where things get forgotten: a missing SetX among the
// seven still compiles. The executor is a parameter so the four consumers are ONE instance.
func wire(cfg config.Config, adapters []forge.Adapter, ex *executor.Executor) tui.Model {
	model := tui.New(cfg, adapters)
	model.SetMounter(ex)
	model.SetSimulator(buildSimulator(cfg, ex))
	model.SetGraphics(herdr.NewGraphics())
	model.SetReviewLookup(ex)
	model.SetReviewRemover(ex)
	return model
}

// Bitbucket is registered even when not operational, so the inbox can report it.
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
