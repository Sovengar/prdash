// prdash — inbox cross-forge de PRs/MRs en TUI.
//
// Consulta GitHub (vía `gh`) y un GitLab self-managed (vía `glab`) y muestra
// las tres secciones que le importan al usuario: creados por mí, review
// pedido/asignados y menciones. Es de solo lectura.
package main

import (
	"flag"
	"fmt"
	"os"

	tea "charm.land/bubbletea/v2"

	"prdash/internal/config"
	"prdash/internal/forge"
	"prdash/internal/forge/bitbucket"
	"prdash/internal/forge/github"
	"prdash/internal/forge/gitlab"
	"prdash/internal/herdr"
	"prdash/internal/tui"
	"prdash/internal/worktree"
)

func main() {
	// Los subcomandos del plugin de Herdr no pasan por el flag parser.
	if len(os.Args) > 1 && os.Args[1] == "herdr" {
		cfg, warn := config.Load()
		if warn != "" {
			fmt.Fprintln(os.Stderr, "prdash:", warn)
		}
		os.Exit(runHerdr(cfg, buildAdapters(cfg), os.Args[2:]))
	}

	// La limpieza de worktrees tampoco: es un subcomando explícito y no
	// interactivo.
	if len(os.Args) > 1 && os.Args[1] == "worktrees" {
		cfg, warn := config.Load()
		if warn != "" {
			fmt.Fprintln(os.Stderr, "prdash:", warn)
		}
		os.Exit(runWorktrees(worktree.Select(herdr.New(), cfg.WorktreeDir), os.Args[2:]))
	}

	printMode := flag.Bool("print", false, "imprime el inbox y sale")
	flag.Parse()

	cfg, warn := config.Load()
	if warn != "" {
		fmt.Fprintln(os.Stderr, "prdash:", warn) // notificar sin abortar
	}

	adapters := buildAdapters(cfg)
	if len(adapters) == 0 {
		fmt.Fprintln(os.Stderr, "prdash: no hay forges habilitados en la config")
	}

	if *printMode {
		// El executor resuelve el review activo de cada ítem desde la memoria
		// de rutas; no toca Herdr ni la red.
		runPrint(adapters, buildExecutor(cfg).ActiveReview)
		return
	}

	model := tui.New(cfg, adapters)
	model.SetMounter(buildExecutor(cfg))
	trackSelection(&model)
	if _, err := tea.NewProgram(model).Run(); err != nil {
		fmt.Fprintln(os.Stderr, "prdash:", err)
		os.Exit(1)
	}
}

// buildAdapters construye los adapters de los forges habilitados. Bitbucket se
// registra aunque no esté operativo, para que el inbox lo reporte.
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
