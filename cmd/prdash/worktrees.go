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
		return removeWorktrees(pr, args[1:])
	default:
		fmt.Fprintf(os.Stderr, "prdash worktrees: unknown subcommand %q (list|remove)\n", sub)
		return 2
	}
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

// removeWorktrees borra las rutas pedidas explícitamente. Cualquier ruta que no
// sea un worktree propio, o que no exista, se rechaza sin tocarla.
func removeWorktrees(pr worktree.Provisioner, paths []string) int {
	if len(paths) == 0 {
		fmt.Fprintln(os.Stderr, "prdash worktrees remove: missing at least one path to remove")
		return 2
	}

	ctx, cancel := context.WithTimeout(context.Background(), worktreeTimeout)
	defer cancel()

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
