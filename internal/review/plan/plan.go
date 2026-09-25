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
	"strings"

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

// Binarios por defecto de cada pane, usados cuando la herramienta no fija un
// argv propio ni un override de `[commands]`.
const (
	defaultTuicrBin = "tuicr"
	defaultHunkBin  = "hunk"
	defaultAgentBin = "opencode"
)

// Tool es la configuración de argv de un pane. Con Override, Argv es el argv
// completo fijado en `[commands]` y Build lo usa verbatim, sin añadir los
// argumentos por defecto del pane. Sin Override, Argv es el binario/base de
// `[tools]` y Build le añade esos argumentos (URL del ítem, target del diff).
type Tool struct {
	Argv     []string
	Override bool
}

// Tools agrupa la configuración de argv de cada pane.
type Tools struct {
	Tuicr Tool
	Hunk  Tool
	Agent Tool
}

// Binary devuelve el ejecutable efectivo de un pane (override, base de `[tools]`
// o binario por defecto), para comprobar su disponibilidad. Vacío = sin comando.
func (t Tools) Binary(kind Kind) string {
	switch kind {
	case KindTuicr:
		return t.Tuicr.binary(defaultTuicrBin)
	case KindHunk:
		return t.Hunk.binary(defaultHunkBin)
	case KindAgent:
		return t.Agent.binary(defaultAgentBin)
	}
	return ""
}

// effective devuelve el argv con el que lanzar el pane: el override verbatim si
// lo hay; si no, el base configurado (o el binario por defecto) más los
// argumentos propios del pane.
func (t Tool) effective(defaultBin string, extra ...string) []string {
	if t.Override {
		return append([]string(nil), t.Argv...)
	}
	base := t.Argv
	if len(base) == 0 {
		base = []string{defaultBin}
	}
	argv := make([]string, 0, len(base)+len(extra))
	argv = append(argv, base...)
	argv = append(argv, extra...)
	return argv
}

// binary devuelve el primer argv efectivo del pane. Un override vacío no tiene
// ejecutable (pane omitido); un base vacío sin override cae al default.
func (t Tool) binary(defaultBin string) string {
	if len(t.Argv) > 0 {
		return t.Argv[0]
	}
	if t.Override {
		return ""
	}
	return defaultBin
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

	add := func(kind Kind, label string, tool Tool, defaultBin string, extra ...string) {
		if !env.available(string(kind)) {
			p.Warnings = append(p.Warnings, label+" is not installed: pane omitted")
			return
		}
		argv := tool.effective(defaultBin, extra...)
		if len(argv) == 0 || strings.TrimSpace(argv[0]) == "" {
			p.Warnings = append(p.Warnings, label+" has no configured command: pane omitted")
			return
		}
		p.Panes = append(p.Panes, Pane{
			Kind:  kind,
			Label: label,
			Cwd:   wt.Path,
			Argv:  argv,
			Env:   paneEnv(pr, wt),
		})
	}

	add(KindTuicr, "TUICR", tools.Tuicr, defaultTuicrBin, "pr", ReviewTarget(pr))
	add(KindHunk, "Hunk", tools.Hunk, defaultHunkBin, hunkArgs(pr)...)
	add(KindAgent, "Agente", tools.Agent, defaultAgentBin)
	return p
}

// hunkArgs son los argumentos del diff del PR/MR para el pane de Hunk: compara
// contra la rama destino en su merge-base (`<base>...HEAD`), la misma semántica
// que GitHub/GitLab muestran para un PR/MR. Sin rama destino cae a `hunk diff`
// (cambios del worktree). Hunk 0.16.0 acepta el revspec de tres puntos como
// target único de `hunk diff`; no se usa `hunk session review`, que exporta una
// sesión viva y exige `<session-id>`/`--repo` en vez de abrir una review.
func hunkArgs(pr model.Item) []string {
	if pr.TargetBranch == "" {
		return []string{"diff"}
	}
	return []string{"diff", pr.TargetBranch + "...HEAD"}
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
	if pr.TargetBranch != "" {
		env = append(env, "PRDASH_BASE="+pr.TargetBranch)
	}
	if pr.URL != "" {
		env = append(env, "PRDASH_URL="+pr.URL)
	}
	return env
}
