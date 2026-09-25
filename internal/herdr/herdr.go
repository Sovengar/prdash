// Package herdr es el único punto de acoplamiento con Herdr: lee HERDR_ENV,
// invoca el binario (HERDR_BIN_PATH o el del PATH) y parsea su salida JSON. El
// resto del programa habla con el puerto Port y nunca toca Herdr directamente.
//
// Fuera de Herdr, o con una versión por debajo del mínimo soportado, el puerto
// se declara no disponible y ninguna operación mutante se ejecuta. Toda salida
// de Herdr se trata como datos: los errores de servidor llegan como JSON por
// stderr y se convierten en un Error tipado, nunca en un pánico.
package herdr

import (
	"context"
	"fmt"
	"os"
	"time"

	"prdash/internal/review/plan"
)

// MinVersion es la versión mínima de Herdr cuyas capacidades usa prdash: CLI de
// worktree (create/list/remove por socket), layout de panes con --no-focus,
// panes/acciones de plugin y link handlers. El detalle de capacidades y sus
// mínimos está en docs/research/herdr-0.9.1-contract.md.
var MinVersion = Version{Major: 0, Minor: 9, Patch: 0}

// Version es una versión semver simplificada del binario de Herdr.
type Version struct {
	Major int
	Minor int
	Patch int
	// Raw es el texto original del que se extrajo la versión.
	Raw string
}

// AtLeast informa si v es mayor o igual que min.
func (v Version) AtLeast(min Version) bool {
	switch {
	case v.Major != min.Major:
		return v.Major > min.Major
	case v.Minor != min.Minor:
		return v.Minor > min.Minor
	default:
		return v.Patch >= min.Patch
	}
}

// String devuelve "major.minor.patch".
func (v Version) String() string { return fmt.Sprintf("%d.%d.%d", v.Major, v.Minor, v.Patch) }

// InHerdr informa si el proceso corre dentro de un pane gestionado por Herdr.
// Es la única lectura de HERDR_ENV en todo el programa.
func InHerdr() bool { return os.Getenv("HERDR_ENV") == "1" }

// WorktreeSpec describe la creación de un worktree nativo de Herdr.
type WorktreeSpec struct {
	// Cwd es el repo (root) desde el que se crea; obligatorio en la práctica.
	Cwd string
	// Branch es una rama local ya existente que se checkoutea.
	Branch string
	// Path es el destino del worktree; vacío usa el default de Herdr.
	Path string
	// Label etiqueta el workspace creado.
	Label string
	// NoFocus evita robar el foco al crear (trabajo de fondo).
	NoFocus bool
}

// WorktreeInfo es el resultado de crear o listar un worktree nativo.
type WorktreeInfo struct {
	WorkspaceID      string
	TabID            string
	RootPaneID       string
	Path             string
	Branch           string
	Label            string
	OpenWorkspaceID  string
	IsLinkedWorktree bool
}

// WorkspaceSpec describe la creación de un workspace.
type WorkspaceSpec struct {
	Cwd     string
	Label   string
	NoFocus bool
}

// WorkspaceInfo es el resultado de crear un workspace.
type WorkspaceInfo struct {
	WorkspaceID string
	TabID       string
	RootPaneID  string
}

// TabSpec describe la creación de una pestaña.
type TabSpec struct {
	WorkspaceID string
	Cwd         string
	Label       string
	NoFocus     bool
}

// TabInfo es el resultado de crear una pestaña.
type TabInfo struct {
	TabID      string
	RootPaneID string
}

// SplitSpec describe la división de un pane.
type SplitSpec struct {
	// PaneID es el pane que se divide.
	PaneID string
	// Direction es "right" o "down".
	Direction string
	// Ratio es la fracción del pane nuevo (0-1); 0 usa el default de Herdr.
	Ratio float64
	// Cwd es el directorio de trabajo del pane nuevo.
	Cwd string
	// Env son variables KEY=VALUE para el proceso del pane nuevo.
	Env []string
	// NoFocus evita robar el foco.
	NoFocus bool
}

// PaneInfo identifica un pane.
type PaneInfo struct {
	PaneID      string
	WorkspaceID string
	TabID       string
	Cwd         string
	Label       string
}

// NotifyOptions ajusta una notificación.
type NotifyOptions struct {
	Body  string
	Sound string // none|done|request (vacío = default de Herdr)
}

// Container identifica dónde abrir un layout: el workspace y el pane base que
// se reutiliza como primer pane del plan. Un PaneID vacío hace que el layout
// cree su propio workspace.
type Container struct {
	WorkspaceID string
	PaneID      string
}

// Error es el fallo de una invocación de Herdr. Preserva el código de salida y
// el código de error del servidor (cuando la respuesta JSON por stderr se puede
// parsear) para poder clasificarlo sin depender del texto.
type Error struct {
	Args []string
	Exit int
	Code string
	Msg  string
	Err  error
}

// Error compone el mensaje incluyendo el código de salida.
func (e *Error) Error() string {
	base := fmt.Sprintf("herdr %v: %s", e.Args, e.Msg)
	if e.Code != "" {
		base = fmt.Sprintf("%s [%s]", base, e.Code)
	}
	if e.Exit != 0 {
		return fmt.Sprintf("%s (exit %d)", base, e.Exit)
	}
	return base
}

// Unwrap expone la causa subyacente.
func (e *Error) Unwrap() error { return e.Err }

// Port reúne las capacidades de Herdr que prdash consume. La implementación
// real es *Client; los tests usan dobles en memoria.
type Port interface {
	// Available informa si Herdr está presente, dentro de Herdr y con versión
	// suficiente. Ningún método mutante debe llamarse si es false.
	Available() bool
	// Version devuelve la versión detectada del binario.
	Version() (Version, bool)

	WorktreeCreate(ctx context.Context, spec WorktreeSpec) (WorktreeInfo, error)
	WorktreeList(ctx context.Context, cwd string) ([]WorktreeInfo, error)
	WorktreeRemove(ctx context.Context, workspaceID string, force bool) error

	WorkspaceCreate(ctx context.Context, spec WorkspaceSpec) (WorkspaceInfo, error)
	WorkspaceClose(ctx context.Context, workspaceID string, group bool) error
	TabCreate(ctx context.Context, spec TabSpec) (TabInfo, error)

	PaneSplit(ctx context.Context, spec SplitSpec) (PaneInfo, error)
	PaneRun(ctx context.Context, paneID string, argv []string) error
	PaneWaitOutput(ctx context.Context, paneID, match string, timeout time.Duration) error
	PaneRename(ctx context.Context, paneID, label string) error
	PaneFocus(ctx context.Context, direction string) error
	PaneList(ctx context.Context, workspaceID string) ([]PaneInfo, error)

	Notify(ctx context.Context, title string, opts NotifyOptions) error

	// MountLayout aplica un plan de panes sobre el contenedor y devuelve los
	// avisos no fatales (pane que no se pudo lanzar/etiquetar).
	MountLayout(ctx context.Context, container Container, pl plan.Plan) ([]string, error)
}

// defaultBin resuelve el binario de Herdr: la vía canónica es HERDR_BIN_PATH,
// que apunta al binario realmente en ejecución (socket/pipe correctos).
func defaultBin() string {
	if p := os.Getenv("HERDR_BIN_PATH"); p != "" {
		return p
	}
	return "herdr"
}
