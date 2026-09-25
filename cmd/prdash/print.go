// Modo `--print`: consulta los forges una vez e imprime el inbox en texto
// plano, en el mismo orden que la TUI. Pensado para scripts y para comprobar
// el pipeline sin abrir la interfaz.
//
// Además de la información de F1, si se le pasa un resolvedor de reviews activos
// añade la ruta del worktree de cada ítem ya montado, de forma determinista.
package main

import (
	"context"
	"fmt"
	"os"
	"sync"
	"text/tabwriter"
	"time"

	"prdash/internal/forge"
	"prdash/internal/forge/model"
	"prdash/internal/inbox"
	"prdash/internal/state"
	"prdash/internal/worktree"
)

// printTimeout acota la consulta de cada forge.
const printTimeout = 60 * time.Second

// reviewLookup resuelve el worktree del review activo de un ítem. nil deja la
// salida solo con la información de F1.
type reviewLookup func(model.Item) (worktree.Worktree, bool)

func runPrint(adapters []forge.Adapter, reviews reviewLookup) {
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
			line := fmt.Sprintf("  %s@%s\t%s#%d\t%s\t%s",
				it.Forge, it.Host, it.Ref.Project, it.Number, state.Derive(it), it.Title)
			if reviews != nil {
				if wt, ok := reviews(it); ok && wt.Path != "" {
					line += "\treview:" + wt.Path
				}
			}
			fmt.Fprintln(w, line)
		}
	}
	_ = w.Flush()

	for _, warning := range box.Warnings {
		fmt.Fprintf(os.Stderr, "prdash: %s: %s (%s)\n", warning.Forge, warning.Msg, warning.Kind)
	}
}
