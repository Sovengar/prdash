package main

import (
	"context"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"text/tabwriter"
	"time"

	"prdash/internal/worktree"
)

const worktreeTimeout = time.Duration(30e9)

func runWorktrees(pr worktree.Provisioner, stdout, stderr io.Writer, args []string) int {
	sub := "list"
	if len(args) > 0 && args[0] != "" {
		sub = args[0]
	}
	switch sub {
	case "list":
		return listWorktrees(pr, stdout, stderr)
	case "remove":
		orphans, dryRun, paths, err := parseRemoveArgs(args[1:])
		if err != nil {
			_, _ = fmt.Fprintf(stderr, "prdash worktrees remove: %v\n", err)
			return 2
		}
		return removeWorktrees(pr, stdout, stderr, orphans, dryRun, paths)
	default:
		_, _ = fmt.Fprintf(stderr, "prdash worktrees: unknown subcommand %q (list|remove)\n", sub)
		return 2
	}
}

func parseRemoveArgs(args []string) (orphans, dryRun bool, paths []string, err error) {
	for _, arg := range args {
		if arg == "--orphans" {
			orphans = true
		} else if arg == "--dry-run" {
			dryRun = true
		} else if strings.HasPrefix(arg, "-") {
			return false, false, nil, fmt.Errorf("unknown flag %q", arg)
		} else {
			paths = append(paths, arg)
		}
	}
	if orphans && len(paths) > 0 {
		return false, false, nil, fmt.Errorf("the two modes cannot be mixed: pass paths or --orphans, not both")
	}
	if dryRun && !orphans {
		return false, false, nil, fmt.Errorf("--dry-run requires --orphans")
	}
	if !orphans && len(paths) == 0 {
		return false, false, nil, fmt.Errorf("missing at least one path to remove")
	}
	return orphans, dryRun, paths, nil
}

func listWorktrees(pr worktree.Provisioner, stdout, stderr io.Writer) int {
	ctx, cancel := context.WithTimeout(context.Background(), worktreeTimeout)
	defer cancel()

	entries := pr.Audit(ctx)
	if len(entries) == 0 {
		_, _ = fmt.Fprintln(stdout, "no prdash review worktrees")
		return 0
	}

	w := tabwriter.NewWriter(stdout, 0, 4, 2, ' ', 0)
	_, _ = fmt.Fprintln(w, "WORKTREE\tBRANCH\tSTATE\tPATH")
	for _, e := range entries {
		state := "ok"
		if e.Orphan {
			state = "orphaned"
		}
		_, _ = fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", e.Label, e.Branch, state, e.Path)
	}
	_ = w.Flush()

	for _, e := range entries {
		if e.Orphan {
			_, _ = fmt.Fprintf(stderr, "prdash: %s is orphaned: %s\n", e.Path, e.Reason)
		}
	}
	return 0
}

func removeWorktrees(pr worktree.Provisioner, stdout, stderr io.Writer,
	orphans, dryRun bool, paths []string) int {
	return removeWorktreesWithin(pr, stdout, stderr, orphans, dryRun, paths, worktreeTimeout)
}

func removeWorktreesWithin(pr worktree.Provisioner, stdout, stderr io.Writer,
	orphans, dryRun bool, paths []string, budget time.Duration) int {
	if orphans {
		return removeOrphans(pr, stdout, stderr, dryRun, budget)
	}

	code := 0
	for _, raw := range paths {
		path, err := filepath.Abs(raw)
		if err != nil || !worktree.Owned("", path) || !worktree.Exists(path) {
			_, _ = fmt.Fprintf(stderr, "prdash worktrees remove: %s is not a prdash worktree; leaving it alone\n", raw)
			code = 1
			continue
		}
		if err := removeOne(pr, path, budget); err != nil {
			_, _ = fmt.Fprintf(stderr, "prdash worktrees remove: %s: %v\n", path, err)
			code = 1
			continue
		}
		_, _ = fmt.Fprintf(stdout, "worktree removed: %s\n", path)
	}
	return code
}

func removeOrphans(pr worktree.Provisioner, stdout, stderr io.Writer, dryRun bool, budget time.Duration) int {
	ctx, cancel := context.WithTimeout(context.Background(), budget)
	entries := pr.Audit(ctx)
	cancel()

	var orphans []worktree.Entry
	for _, e := range entries {
		if e.Orphan {
			orphans = append(orphans, e)
		}
	}
	if len(orphans) == 0 {
		_, _ = fmt.Fprintln(stdout, "no orphaned prdash worktrees")
		return 0
	}

	code := 0
	for _, e := range orphans {
		if dryRun {
			_, _ = fmt.Fprintf(stdout, "would remove: %s\n", e.Path)
			continue
		}
		if err := removeOne(pr, e.Path, budget); err != nil {
			_, _ = fmt.Fprintf(stderr, "prdash worktrees remove: %s: %v\n", e.Path, err)
			code = 1
			continue
		}
		_, _ = fmt.Fprintf(stdout, "worktree removed: %s\n", e.Path)
	}
	return code
}

func removeOne(pr worktree.Provisioner, path string, budget time.Duration) error {
	ctx, cancel := context.WithTimeout(context.Background(), budget)
	defer cancel()
	return pr.Remove(ctx, path)
}
