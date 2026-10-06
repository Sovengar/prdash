// `--print` mode: query the forges once and print the inbox as plain text, in the same order the
// TUI paints it. For scripts and for checking the pipeline without opening the UI.
package main

import (
	"context"
	"fmt"
	"io"
	"sync"
	"text/tabwriter"
	"time"

	"prdash/internal/forge"
	"prdash/internal/forge/model"
	"prdash/internal/inbox"
	"prdash/internal/state"
	"prdash/internal/worktree"
)

// 60s; a literal because a const decl carries no coverage, so `*` here would be a mutant no test can reach (ADR 0011).
const printTimeout = time.Duration(60e9)

// A nil lookup leaves the output with just the F1 information.
type reviewLookup func(model.Item) (worktree.Worktree, bool)

// The writer is a parameter, not a fixed os.Stdout: a hardcoded one would let every line of the
// table escape the test while the exit code and the warnings were still checked.
func runPrintTo(stdout, stderr io.Writer, adapters []forge.Adapter, reviews reviewLookup) {
	results := make([]inbox.ForgeResult, len(adapters))
	var wg sync.WaitGroup
	for i, a := range adapters {
		wg.Add(1)
		go func(i int, a forge.Adapter) {
			defer wg.Done()
			ctx, cancel := context.WithTimeout(context.Background(), printTimeout)
			defer cancel()
			results[i] = forge.Collect(ctx, a)
		}(i, a)
	}
	wg.Wait()

	box := inbox.Build(results)

	w := tabwriter.NewWriter(stdout, 0, 4, 2, ' ', 0)
	for _, sec := range box.Sections {
		_, _ = fmt.Fprintf(w, "%s (%d)\n", sec.Kind.String(), len(sec.Items))
		for _, it := range sec.Items {
			line := fmt.Sprintf("  %s@%s\t%s#%d\t%s\t%s\t%s",
				it.Forge, it.Host, it.Ref.Project, it.Number, state.Derive(it), printDiff(it.Diff), it.Title)
			if reviews != nil {
				if wt, ok := reviews(it); ok && wt.Path != "" {
					line += "\treview:" + wt.Path
				}
			}
			_, _ = fmt.Fprintln(w, line)
		}
	}
	_ = w.Flush()

	for _, warning := range box.Warnings {
		_, _ = fmt.Fprintf(stderr, "prdash: %s: %s (%s)\n", warning.Forge, warning.Msg, warning.Kind)
	}
}

func printDiff(d model.DiffStat) string {
	if !d.Known {
		return "-"
	}
	return fmt.Sprintf("+%d -%d", d.Additions, d.Deletions)
}
