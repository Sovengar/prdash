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
	"os"
	"path/filepath"
	"strings"
	"text/tabwriter"
	"time"

	"prdash/internal/worktree"
)

// worktreeTimeout acota el listado y cada borrado.
const worktreeTimeout = 30 * time.Second

// runWorktrees despacha `prdash worktrees [list|remove <ruta>…]`.
func runWorktrees(pr worktree.Provisioner, args []string) int {
	sub := "list"
	if len(args) > 0 && args[0] != "" {
		sub = args[0]
	}
	switch sub {
	case "list":
		return listWorktrees(pr)
	case "remove":
		orphans, dryRun, paths, err := parseRemoveArgs(args[1:])
		if err != nil {
			fmt.Fprintf(os.Stderr, "prdash worktrees remove: %v\n", err)
			return 2
		}
		return removeWorktrees(pr, orphans, dryRun, paths)
	default:
		fmt.Fprintf(os.Stderr, "prdash worktrees: unknown subcommand %q (list|remove)\n", sub)
		return 2
	}
}

// parseRemoveArgs interpreta los argumentos de `remove` sin tocar os.Args, de modo
// que sea testeable aislado. Los dos modos —rutas explícitas y `--orphans`— son
// excluyentes; `--dry-run` solo acompaña a `--orphans`. Cualquier uso inválido es
// un error de uso (el llamador lo traduce a exit 2).
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
func listWorktrees(pr worktree.Provisioner) int {
	ctx, cancel := context.WithTimeout(context.Background(), worktreeTimeout)
	defer cancel()

	entries := pr.Audit(ctx)
	if len(entries) == 0 {
		fmt.Println("no prdash review worktrees")
		return 0
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
	fmt.Fprintln(w, "WORKTREE\tRAMA\tESTADO\tRUTA")
	for _, e := range entries {
		state := "ok"
		if e.Orphan {
			state = "orphaned"
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", e.Label, e.Branch, state, e.Path)
	}
	_ = w.Flush()

	for _, e := range entries {
		if e.Orphan {
			fmt.Fprintf(os.Stderr, "prdash: %s is orphaned: %s\n", e.Path, e.Reason)
		}
	}
	return 0
}

// removeWorktrees borra las rutas pedidas explícitamente o, con `--orphans`, el
// lote que el propio Audit marca como huérfano. Cualquier ruta que no sea un
// worktree propio, o que no exista, se rechaza sin tocarla.
func removeWorktrees(pr worktree.Provisioner, orphans, dryRun bool, paths []string) int {
	ctx, cancel := context.WithTimeout(context.Background(), worktreeTimeout)
	defer cancel()

	if orphans {
		return removeOrphans(pr, ctx, dryRun)
	}

	code := 0
	for _, raw := range paths {
		path, err := filepath.Abs(raw)
		if err != nil || !worktree.Owned("", path) || !worktree.Exists(path) {
			fmt.Fprintf(os.Stderr, "prdash worktrees remove: %s is not a prdash worktree; leaving it alone\n", raw)
			code = 1
			continue
		}
		if err := pr.Remove(ctx, path); err != nil {
			fmt.Fprintf(os.Stderr, "prdash worktrees remove: %v\n", err)
			code = 1
			continue
		}
		fmt.Printf("worktree removed: %s\n", path)
	}
	return code
}

// removeOrphans borra en lote los worktrees que Audit marca como huérfanos, y
// solo esos. Cero huérfanos es el caso feliz (exit 0): informa y no toca nada.
// Con dryRun imprime el lote exacto por el mismo camino de código y no borra.
func removeOrphans(pr worktree.Provisioner, ctx context.Context, dryRun bool) int {
	var orphans []worktree.Entry
	for _, e := range pr.Audit(ctx) {
		if e.Orphan {
			orphans = append(orphans, e)
		}
	}
	if len(orphans) == 0 {
		fmt.Println("no orphaned prdash worktrees")
		return 0
	}

	code := 0
	for _, e := range orphans {
		if dryRun {
			fmt.Printf("would remove: %s\n", e.Path)
			continue
		}
		if err := pr.Remove(ctx, e.Path); err != nil {
			fmt.Fprintf(os.Stderr, "prdash worktrees remove: %v\n", err)
			code = 1
			continue
		}
		fmt.Printf("worktree removed: %s\n", e.Path)
	}
	return code
}
