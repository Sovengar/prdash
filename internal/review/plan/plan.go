// Package plan construye el plan de panes del review a partir de un ítem, su
// worktree y el entorno (herramientas configuradas y binarios disponibles).
//
// Es puro: no toca red, subproceso, disco ni Herdr. La aplicación del plan vive
// en el executor; este paquete solo decide qué panes habría que abrir, con qué
// cwd, argv y entorno. Un binario ausente omite su pane con un aviso en vez de
// tumbar el layout.
package plan

import (
	"strconv"

	"prdash/internal/forge/model"
)

// Kind identifica el tipo de pane del review.
type Kind string

const (
	// KindTuicr es el pane de review de TUICR.
	KindTuicr Kind = "tuicr"
	// KindHunk es el pane del diff con Hunk.
	KindHunk Kind = "hunk"
	// KindAgent es el pane del agente.
	KindAgent Kind = "agent"
)

// Pane es un pane planificado, aún sin abrir.
type Pane struct {
	Kind  Kind
	Label string
	Cwd   string
	Argv  []string
	Env   []string
}

// Plan es el conjunto de panes más los avisos de lo que se omitió.
type Plan struct {
	Panes    []Pane
	Warnings []string
}

// Worktree es el worktree sobre el que trabajan todos los panes.
type Worktree struct {
	Path   string
	Branch string
	Label  string
}

// Tools es el argv base configurable de cada herramienta. Un argv vacío deja el
// pane sin comando y se omite con aviso.
type Tools struct {
	Tuicr []string
	Hunk  []string
	Agent []string
}

// Env describe el entorno del montaje. Available nil asume que todas las
// herramientas están presentes; si trae claves, solo esas se consideran
// instaladas.
type Env struct {
	Available map[string]bool
}

// Build construye el plan de panes del review. No falla: lo que no se puede
// montar se reporta en Warnings.
func Build(pr model.Item, wt Worktree, tools Tools, env Env) Plan {
	var p Plan

	add := func(kind Kind, label string, base []string, extra ...string) {
		if !env.available(string(kind)) {
			p.Warnings = append(p.Warnings, label+" is not installed: pane omitted")
			return
		}
		if len(base) == 0 {
			p.Warnings = append(p.Warnings, label+" has no configured command: pane omitted")
			return
		}
		argv := make([]string, 0, len(base)+len(extra))
		argv = append(argv, base...)
		argv = append(argv, extra...)
		p.Panes = append(p.Panes, Pane{
			Kind:  kind,
			Label: label,
			Cwd:   wt.Path,
			Argv:  argv,
			Env:   paneEnv(pr, wt),
		})
	}

	add(KindTuicr, "TUICR", tools.Tuicr, "pr", ReviewTarget(pr))
	add(KindHunk, "Hunk", tools.Hunk, "session", "review")
	add(KindAgent, "Agente", tools.Agent)
	return p
}

// ReviewTarget es el argumento con el que TUICR identifica el ítem: su URL si
// la hay, o "proyecto#número".
func ReviewTarget(pr model.Item) string {
	if pr.URL != "" {
		return pr.URL
	}
	return pr.Ref.Project + "#" + strconv.Itoa(pr.Number)
}

// available decide si una herramienta está presente; un Env sin mapa asume que
// todas lo están.
func (e Env) available(kind string) bool {
	if e.Available == nil {
		return true
	}
	return e.Available[kind]
}

// paneEnv es el entorno inyectado a cada pane para que las herramientas sepan
// sobre qué ítem y worktree trabajan.
func paneEnv(pr model.Item, wt Worktree) []string {
	env := []string{
		"PRDASH_REPO=" + pr.Ref.Project,
		"PRDASH_NUMBER=" + strconv.Itoa(pr.Number),
		"PRDASH_WORKTREE=" + wt.Path,
	}
	if wt.Branch != "" {
		env = append(env, "PRDASH_BRANCH="+wt.Branch)
	}
	if pr.URL != "" {
		env = append(env, "PRDASH_URL="+pr.URL)
	}
	return env
}
