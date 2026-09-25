// Cableado del orquestador de review: junta el resolutor de repos, la provisión
// de worktree (nativa dentro de Herdr, git directo fuera) y el puerto de layout
// en un executor listo para la TUI o los subcomandos del plugin.
package main

import (
	"os"
	"os/exec"
	"strings"

	"prdash/internal/config"
	"prdash/internal/herdr"
	"prdash/internal/reporesolver"
	"prdash/internal/review/executor"
	"prdash/internal/review/plan"
	"prdash/internal/selection"
	"prdash/internal/tui"
	"prdash/internal/worktree"
)

// trackSelection hace que la TUI persista el ítem seleccionado para que la
// acción `prdash.mount-review` invocada sin URL pueda montarlo. Si la ruta de
// estado no se puede resolver, la TUI sigue funcionando sin persistencia.
func trackSelection(m *tui.Model) {
	if p, err := selection.Path(); err == nil {
		m.SetSelectionPath(p)
	}
}

// buildExecutor arma el orquestador de review sobre la config. La selección del
// provisioner la decide worktree.Select: el llamador no sabe cuál corre.
func buildExecutor(cfg config.Config) *executor.Executor {
	client := herdr.New()
	tools := plan.Tools{
		Tuicr: cfg.ToolArgs("tuicr"),
		Hunk:  cfg.ToolArgs("hunk"),
		Agent: cfg.ToolArgs("agent"),
	}
	return &executor.Executor{
		Resolver: reporesolver.New(reporesolver.Options{
			Roots:       cfg.Roots,
			CloneDir:    cfg.CloneDir,
			WorktreeDir: cfg.WorktreeDir,
			Hosts:       hostsOf(cfg),
			Prefixes:    clonePrefixesOf(cfg),
		}),
		Worktrees: worktree.Select(client, cfg.WorktreeDir),
		Herdr:     client,
		Tools:     tools,
		Env:       plan.Env{Available: toolAvailability(tools)},
	}
}

// hostsOf mapea host → nombre de forge para normalizar remotos y URLs.
func hostsOf(cfg config.Config) map[string]string {
	hosts := map[string]string{}
	if h := cfg.Forges.GitHub.Host; h != "" {
		hosts[h] = "github"
	}
	if h := cfg.Forges.GitLab.Host; h != "" {
		hosts[h] = "gitlab"
	}
	return hosts
}

// clonePrefixesOf mapea host → relative URL root de clonado/web, para que el
// resolutor construya y normalice URLs de instancias servidas en subcarpeta
// (p. ej. GitLab self-managed con api_base "/git/api/v4/" → "git").
func clonePrefixesOf(cfg config.Config) map[string]string {
	prefixes := map[string]string{}
	if h := cfg.Forges.GitHub.Host; h != "" {
		if p := cfg.Forges.GitHub.ClonePrefix(); p != "" {
			prefixes[h] = p
		}
	}
	if h := cfg.Forges.GitLab.Host; h != "" {
		if p := cfg.Forges.GitLab.ClonePrefix(); p != "" {
			prefixes[h] = p
		}
	}
	return prefixes
}

// toolAvailability comprueba qué binarios del plan están instalados, de modo
// que un pane ausente se omita con aviso en vez de tumbar el layout.
func toolAvailability(tools plan.Tools) map[string]bool {
	return map[string]bool{
		string(plan.KindTuicr): binaryAvailable(tools.Tuicr),
		string(plan.KindHunk):  binaryAvailable(tools.Hunk),
		string(plan.KindAgent): binaryAvailable(tools.Agent),
	}
}

// binaryAvailable informa si el argv de una herramienta se puede ejecutar. Un
// argv vacío se considera no disponible (pane omitido).
func binaryAvailable(argv []string) bool {
	if len(argv) == 0 || strings.TrimSpace(argv[0]) == "" {
		return false
	}
	if strings.ContainsRune(argv[0], os.PathSeparator) {
		info, err := os.Stat(argv[0])
		return err == nil && !info.IsDir()
	}
	_, err := exec.LookPath(argv[0])
	return err == nil
}
