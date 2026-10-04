// Subcomando `prdash worktrees`: gestiona los worktrees de review que
// pertenecen a prdash. El ownership vive en el nombre/label (`prdash-…`): nunca
// lista ni borra worktrees ajenos, y solo borra cuando se le pide de forma
// explícita. Al cerrar la app los worktrees se conservan: no hay borrado
// implícito.
//
// La provisión la decide el llamador (nativo de Herdr dentro de Herdr, git
// directo fuera), de modo que el comando funciona en ambos entornos.
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

// worktreeTimeout acota el listado y cada borrado.
const worktreeTimeout = 30 * time.Second

// runWorktrees despacha `prdash worktrees [list|remove <ruta>…]`.
//
// Y los dos `io.Writer` no son un detalle de estilo: son lo que hace que este comando sea
// testeable. `listWorktrees` imprimía a `os.Stdout` y `os.Stderr` fijos, así que la única
// forma de comprobar qué imprime era replacing los ficheros del proceso entero, y eso obliga
// a un test por proceso —o a no probar la salida— justo en el comando que BORRA ficheros
// del usuario, donde la salida es la prueba de que borró lo que dijo y nada más. Es el mismo
// seam que `run` y `runPrintTo` llevan ya, extendido a este subcomando.
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

// parseRemoveArgs interpreta los argumentos de `remove` sin tocar os.Args, de modo
// que sea testeable aislado. Los dos modos —rutas explícitas y `--orphans`— son
// excluyentes; `--dry-run` solo acompaña a `--orphans`. Cualquier uso inválido es
// un error de uso (el llamador lo traduce a exit 2).
//
// Todo token que empieza por `-` se trata como flag: las rutas que gestiona
// prdash son absolutas, así que un guion inicial nunca es una ruta legítima y
// rechazarlo evita que una errata de `--orphans`/`--dry-run` (p. ej. `--orphan`)
// se cuele como si fuera un path. Es una decisión, no un olvido: no hay forma de
// borrar una ruta que empiece por `-`.
func parseRemoveArgs(args []string) (orphans, dryRun bool, paths []string, err error) {
	for _, arg := range args {
		switch {
		case arg == "--orphans":
			orphans = true
		case arg == "--dry-run":
			dryRun = true
		case strings.HasPrefix(arg, "-"):
			return false, false, nil, fmt.Errorf("unknown flag %q", arg)
		default:
			paths = append(paths, arg)
		}
	}
	switch {
	case orphans && len(paths) > 0:
		return false, false, nil, fmt.Errorf("the two modes cannot be mixed: pass paths or --orphans, not both")
	case dryRun && !orphans:
		return false, false, nil, fmt.Errorf("--dry-run requires --orphans")
	case !orphans && len(paths) == 0:
		return false, false, nil, fmt.Errorf("missing at least one path to remove")
	}
	return orphans, dryRun, paths, nil
}

// listWorktrees imprime los worktrees propios, marcando los huérfanos.
//
// Y la tabla va a stdout y el aviso de huérfano a stderr a propósito, porque son cosas
// distintas: la tabla es el resultado que un script lee, y el aviso es la explicación de por
// qué una fila está marcada. Un `2>/dev/null` sobre el listado se lleva la explicación y deja
// la tabla, que es lo que se quiere; al revés se pierde el resultado y se queda el ruido.
func listWorktrees(pr worktree.Provisioner, stdout, stderr io.Writer) int {
	ctx, cancel := context.WithTimeout(context.Background(), worktreeTimeout)
	defer cancel()

	entries := pr.Audit(ctx)
	if len(entries) == 0 {
		_, _ = fmt.Fprintln(stdout, "no prdash review worktrees")
		return 0
	}

	w := tabwriter.NewWriter(stdout, 0, 4, 2, ' ', 0)
	_, _ = fmt.Fprintln(w, "WORKTREE\tRAMA\tESTADO\tRUTA")
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

// removeWorktrees borra las rutas pedidas explícitamente o, con `--orphans`, el
// lote que el propio Audit marca como huérfano. Cualquier ruta que no sea un
// worktree propio, o que no exista, se rechaza sin tocarla.
func removeWorktrees(pr worktree.Provisioner, stdout, stderr io.Writer,
	orphans, dryRun bool, paths []string) int {
	return removeWorktreesWithin(pr, stdout, stderr, orphans, dryRun, paths, worktreeTimeout)
}

// removeWorktreesWithin es removeWorktrees con un presupuesto por ítem inyectable:
// es el seam que deja a los tests acotar el plazo sin esperar el de producción.
//
// Cada borrado —y el Audit del lote— recibe su propio presupuesto en vez de
// compartir uno global: un ítem lento que agota el suyo no puede consumir el
// tiempo de los siguientes ni dejar un borrado parcial por timeout.
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

// removeOrphans borra en lote los worktrees que Audit marca como huérfanos, y
// solo esos. Cero huérfanos es el caso feliz (exit 0): informa y no toca nada.
// Con dryRun imprime el lote exacto por el mismo camino de código y no borra.
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

// removeOne borra un solo worktree con su propio presupuesto, para que el tiempo
// de un ítem del lote no se descuente del de los demás.
func removeOne(pr worktree.Provisioner, path string, budget time.Duration) error {
	ctx, cancel := context.WithTimeout(context.Background(), budget)
	defer cancel()
	return pr.Remove(ctx, path)
}
