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
	// KindEditor es el pane del editor. No lleva herramientas de review: solo la
	// orden con la que el usuario edita el worktree.
	KindEditor Kind = "editor"
)

// Dirección con la que un pane se divide respecto al pane anterior de su tab.
const (
	// DirReuse marca el pane que reutiliza el pane base de su tab, sin dividir.
	DirReuse = ""
	// DirRight divide a la derecha del pane anterior.
	DirRight = "right"
	// DirDown divide hacia abajo del pane anterior.
	DirDown = "down"
)

// Etiquetas de los tabs del review. Son la nomenclatura que ve el usuario en la
// barra de pestañas del workspace, así que viven aquí y no en el puerto que las
// aplica.
const (
	// LabelReview es el tab de lectura: la review y el editor.
	LabelReview = "Review"
	// LabelEdit es el tab de trabajo: el diff y el agente.
	LabelEdit = "Edit"
)

// Pane es un pane planificado, aún sin abrir.
type Pane struct {
	Kind  Kind
	Label string
	// Dir es la dirección de la división que crea el pane respecto al anterior
	// de su tab. DirReuse (vacío) marca el pane que reutiliza el pane base del
	// tab, que es el primero.
	Dir  string
	Cwd  string
	Argv []string
	Env  []string
}

// Tab agrupa los panes que comparten una pestaña del workspace de review.
type Tab struct {
	Label string
	Panes []Pane
}

// Cwd es el directorio de trabajo del tab: el de su primer pane, que es el del
// worktree. Vacío si el tab no tiene panes.
func (t Tab) Cwd() string {
	if len(t.Panes) == 0 {
		return ""
	}
	return t.Panes[0].Cwd
}

// Plan es el conjunto de tabs más los avisos de lo que se omitió. Los tabs que
// se quedan sin panes no se incluyen: una pestaña en blanco es ruido que el
// usuario tendría que cerrar a mano.
type Plan struct {
	Tabs     []Tab
	Warnings []string
}

// PaneCount es el número de panes del plan, sumados todos los tabs. Es lo que
// informa el montaje al usuario.
func (p Plan) PaneCount() int {
	n := 0
	for _, t := range p.Tabs {
		n += len(t.Panes)
	}
	return n
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
	defaultEditBin  = "vi"
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
	Tuicr  Tool
	Hunk   Tool
	Agent  Tool
	Editor Tool
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
	case KindEditor:
		return t.Editor.binary(defaultEditBin)
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

// Build construye el plan de panes del review: un tab de lectura (la review y el
// editor) y otro de trabajo (el diff y el agente). No falla: lo que no se puede
// montar se reporta en Warnings.
func Build(pr model.Item, wt Worktree, tools Tools, env Env) Plan {
	var p Plan

	// compose compone un pane del plan. Un argv vacío no es una herramienta
	// ausente sino configuración inválida, y se avisa con el mismo criterio:
	// montar un shell en un pane que debería tener la review no es un layout.
	compose := func(kind Kind, label, dir string, tool Tool, defaultBin string, extra ...string) (Pane, bool) {
		argv := tool.effective(defaultBin, extra...)
		if len(argv) == 0 || strings.TrimSpace(argv[0]) == "" {
			p.Warnings = append(p.Warnings, label+" has no configured command: pane omitted")
			return Pane{}, false
		}
		return Pane{
			Kind:  kind,
			Label: label,
			Dir:   dir,
			Cwd:   wt.Path,
			Argv:  argv,
			Env:   paneEnv(pr, wt),
		}, true
	}

	// add es el pane de una herramienta opcional: sin binario no se monta, pero
	// se avisa para que el hueco se explique en vez de desaparecer en silencio.
	add := func(tab *Tab, kind Kind, label, dir string, tool Tool, defaultBin string, extra ...string) {
		if !env.available(string(kind)) {
			p.Warnings = append(p.Warnings, label+" is not installed: pane omitted")
			return
		}
		if pane, ok := compose(kind, label, dir, tool, defaultBin, extra...); ok {
			tab.Panes = append(tab.Panes, pane)
		}
	}

	review := Tab{Label: LabelReview}
	add(&review, KindTuicr, "TUICR", DirReuse, tools.Tuicr, defaultTuicrBin, "pr", ReviewTarget(pr))
	// El editor se compone sin comprobar disponibilidad: a diferencia de
	// tuicr/hunk/agente, su orden puede ser una función o un alias del shell (el
	// clásico `vi` que expande a `nvim .`) y no existir como binario en el PATH,
	// así que un chequeo lo borraría siempre y dejaría el tab de review con un
	// solo pane sin explicación. Y una orden mal escrita se ve en el propio
	// pane, que es justo cuando el usuario lo está mirando.
	if editor, ok := compose(KindEditor, "Editor", DirRight, tools.Editor, defaultEditBin); ok {
		review.Panes = append(review.Panes, editor)
	}
	edit := Tab{Label: LabelEdit}
	// Hunk sin revspec: `hunk diff` a secas revisa el WORKING TREE. El pane
	// comparte tab con el editor y el agente, así que lo que se quiere ver es lo
	// que se está tocando; el diff del PR/MR contra la rama destino es un
	// objetivo fijo que no se mueve mientras editas y esconde los cambios en
	// curso. Sigue disponible por `[commands].hunk` (p. ej. `hunk diff
	// main...HEAD`), y no se usa `hunk session review`, que exporta una sesión
	// viva y exige `<session-id>`/`--repo` en vez de abrir una review.
	add(&edit, KindHunk, "Hunk", DirReuse, tools.Hunk, defaultHunkBin, "diff")
	add(&edit, KindAgent, "Agente", DirRight, tools.Agent, defaultAgentBin)
	p.Tabs = nonEmpty(review, edit)
	return p
}

// nonEmpty descarta los tabs que se quedaron sin panes.
func nonEmpty(tabs ...Tab) []Tab {
	out := make([]Tab, 0, len(tabs))
	for _, t := range tabs {
		if len(t.Panes) > 0 {
			out = append(out, t)
		}
	}
	return out
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
