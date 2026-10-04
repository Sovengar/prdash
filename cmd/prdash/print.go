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

// printTimeout acota la consulta de cada forge.
const printTimeout = 60 * time.Second

// reviewLookup resuelve el worktree del review activo de un ítem. nil deja la
// salida solo con la información de F1.
type reviewLookup func(model.Item) (worktree.Worktree, bool)

// runPrintTo imprime el inbox en stdout.
//
// Y el writer llega por parámetro en vez de ser `os.Stdout` fijo. La razón es que `run` ya
// recibe los writers inyectados para poder probarse entero, y si esta función escribiera a
// `os.Stdout` la salida del modo texto se escaparía del test: se podría comprobar el código de
// salida y los avisos, pero no UNA SOLA LÍNEA de la tabla, que es justo lo que este modo
// promete.
//
// Y no tiene una-envoltura fija a `os.Stdout` porque no la necesita: `run` la llama
// directamente. Existía una, y sobró en cuanto los tests del modo texto dejaron de necesitar
// capturadores por fd —una envoltura sin un solo llamador es código que solo se ve en la
// cobertura.
func runPrintTo(stdout, stderr io.Writer, adapters []forge.Adapter, reviews reviewLookup) {
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

	w := tabwriter.NewWriter(stdout, 0, 4, 2, ' ', 0)
	for _, sec := range box.Sections {
		_, _ = fmt.Fprintf(w, "%s (%d)\n", sec.Kind.String(), len(sec.Items))
		for _, it := range sec.Items {
			// El diffstat va con los números sin compactar: aquí lo lee un
			// script, no un ojo, y abrevia un recuento solo estorbaría.
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

// printDiff es el diffstat en una celda, sin el recuento de ficheros: la línea
// ya lleva el estado, el título y, si está, la ruta del review, y el número de
// ficheros vive en el detalle.
func printDiff(d model.DiffStat) string {
	if !d.Known {
		return "-"
	}
	return fmt.Sprintf("+%d -%d", d.Additions, d.Deletions)
}
