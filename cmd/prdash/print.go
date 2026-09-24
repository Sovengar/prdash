// Modo `--print`: consulta los forges una vez e imprime el inbox en texto
// plano, en el mismo orden que la TUI. Pensado para scripts y para comprobar
// el pipeline sin abrir la interfaz.
package main

import (
	"context"
	"fmt"
	"os"
	"sync"
	"text/tabwriter"
	"time"

	"prdash/internal/config"
	"prdash/internal/forge"
	"prdash/internal/inbox"
	"prdash/internal/state"
)

// printTimeout acota la consulta de cada forge.
const printTimeout = 60 * time.Second

func runPrint(cfg config.Config, adapters []forge.Adapter) {
	// Una goroutine por forge con su propio timeout: una forge lenta no
	// bloquea a las demás. El orden de impresión queda fijado por índice.
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

	w := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
	for _, sec := range box.Sections {
		fmt.Fprintf(w, "%s (%d)\n", sec.Kind.String(), len(sec.Items))
		for _, it := range sec.Items {
			fmt.Fprintf(w, "  %s@%s\t%s#%d\t%s\t%s\n",
				it.Forge, it.Host, it.Ref.Project, it.Number, state.Derive(it), it.Title)
		}
	}
	_ = w.Flush()

	for _, warning := range box.Warnings {
		fmt.Fprintf(os.Stderr, "prdash: %s: %s (%s)\n", warning.Forge, warning.Msg, warning.Kind)
	}
}
