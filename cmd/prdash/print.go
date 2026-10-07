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

const printTimeout = time.Duration(60e9)

type reviewLookup func(model.Item) (worktree.Worktree, bool)

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
