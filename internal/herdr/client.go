package herdr

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"
)

// DefaultTimeout es el límite por invocación de la CLI de Herdr.
const DefaultTimeout = 30 * time.Second

// execFunc es la firma de ejecución de la CLI, inyectable en tests.
type execFunc func(ctx context.Context, args ...string) (stdout, stderr []byte, err error)

// Client es la implementación real del puerto Port sobre la CLI de Herdr.
type Client struct {
	// Bin es el binario; vacío usa HERDR_BIN_PATH o "herdr".
	Bin string
	// Timeout por invocación; <=0 usa DefaultTimeout.
	Timeout time.Duration

	execFn execFunc
	getenv func(string) string

	versionOnce  sync.Once
	version      Version
	versionFound bool
}

// New construye un Client con el binario canónico y el entorno real.
func New() *Client { return &Client{Bin: defaultBin(), getenv: os.Getenv} }

// Available informa si Herdr está disponible: hay que correr dentro de Herdr
// (HERDR_ENV=1) y la versión debe alcanzar el mínimo soportado. Si la versión
// no se puede determinar, no se bloquea por drift (se asume compatible).
func (c *Client) Available() bool {
	if c.env("HERDR_ENV") != "1" {
		return false
	}
	v, ok := c.Version()
	if !ok {
		return true
	}
	return v.AtLeast(MinVersion)
}

// Version consulta y cachea la versión del binario.
func (c *Client) Version() (Version, bool) {
	c.versionOnce.Do(func() {
		out, _, err := c.run(context.Background(), "--version")
		if err != nil {
			return
		}
		c.version, c.versionFound = parseVersion(out)
	})
	return c.version, c.versionFound
}

// env resuelve una variable de entorno.
func (c *Client) env(key string) string {
	if c.getenv != nil {
		return c.getenv(key)
	}
	return os.Getenv(key)
}

// guard veta toda operación mutante cuando Herdr no está disponible (fuera de
// Herdr o por debajo de la versión mínima): el puerto nunca toca la sesión si
// no puede hacerlo con garantías. Las lecturas (list/version) no pasan por aquí.
//
// Los args identifican el subcomando vetado para que el error sea legible
// (`herdr worktree create: …`, no un `herdr []` vacío).
func (c *Client) guard(args ...string) error {
	if !c.Available() {
		return &Error{Args: args, Msg: "herdr unavailable (requires HERDR_ENV=1 and version >= " + MinVersion.String() + ")"}
	}
	return nil
}

// run ejecuta la CLI con timeout y devuelve stdout/stderr crudos.
func (c *Client) run(ctx context.Context, args ...string) ([]byte, []byte, error) {
	if c.execFn != nil {
		return c.execFn(ctx, args...)
	}
	timeout := c.Timeout
	if timeout <= 0 {
		timeout = DefaultTimeout
	}
	cctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	bin := c.Bin
	if bin == "" {
		bin = defaultBin()
	}
	cmd := exec.CommandContext(cctx, bin, args...)
	cmd.Env = os.Environ()
	var out, errb bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errb
	err := cmd.Run()
	return out.Bytes(), errb.Bytes(), err
}

// result ejecuta la CLI y exige éxito, convirtiendo el fallo en *Error.
func (c *Client) result(ctx context.Context, args ...string) ([]byte, error) {
	out, errb, err := c.run(ctx, args...)
	if err != nil {
		return nil, newError(args, err, errb)
	}
	return out, nil
}

// newError tipa el fallo leyendo el JSON de stderr (si lo hay).
//
// El mensaje sale de UNA de dos fuentes, nunca de las dos mezcladas: el `message` del
// servidor si el stderr es una respuesta de Herdr, y la primera línea del stderr si no
// lo es. Antes se empezaba por la primera línea y se sobrescribía con el `message`
// solo si venía, así que un servidor que manda código SIN mensaje dejaba el JSON
// crudo pegado en el mensaje: el usuario veía `{"error":{"code":"E_SOLO"}}` como
// explicación de un fallo, con el código repetido dos veces y sin nada legible.
//
// La razón de que sea excluyente es que el JSON solo contiene el código, y el código
// ya va en su campo. Si no hay `message`, lo que queda del stderr no dice nada que el
// campo Code no diga ya, y el mensaje del error de exec ("exit status 3") al menos
// dice que el proceso se cayó.
func newError(args []string, err error, stderr []byte) *Error {
	e := &Error{Args: args, Err: err}
	var exit *exec.ExitError
	if errors.As(err, &exit) {
		e.Exit = exit.ExitCode()
	}
	if code, msg := parseServerError(stderr); code != "" || msg != "" {
		e.Code = code
		e.Msg = msg
	} else {
		e.Msg = firstLine(string(stderr))
	}
	// Y si no queda nada legible, el error de exec. Un error sin texto se pinta como
	// un fallo sin explicación, que es peor que no pintar el error.
	if e.Msg == "" {
		e.Msg = err.Error()
	}
	return e
}

// WorktreeCreate crea y abre un worktree nativo. La rama debe existir ya en
// local; prdash nunca delega el fetch ni la creación de la rama.
func (c *Client) WorktreeCreate(ctx context.Context, spec WorktreeSpec) (WorktreeInfo, error) {
	if err := c.guard("worktree", "create"); err != nil {
		return WorktreeInfo{}, err
	}
	out, err := c.result(ctx, worktreeCreateArgs(spec)...)
	if err != nil {
		return WorktreeInfo{}, err
	}
	return parseWorktreeCreated(out)
}

// WorktreeList lista los worktrees del repo en cwd.
func (c *Client) WorktreeList(ctx context.Context, cwd string) ([]WorktreeInfo, error) {
	out, err := c.result(ctx, worktreeListArgs(cwd)...)
	if err != nil {
		return nil, err
	}
	list, err := parseWorktreeList(out)
	if err != nil {
		return nil, err
	}
	return list.Worktrees, nil
}

// WorktreeRemove quita el checkout de un worktree ligado a un workspace.
func (c *Client) WorktreeRemove(ctx context.Context, workspaceID string, force bool) error {
	if err := c.guard("worktree", "remove"); err != nil {
		return err
	}
	_, err := c.result(ctx, worktreeRemoveArgs(workspaceID, force)...)
	return err
}

// WorkspaceCreate crea un workspace.
func (c *Client) WorkspaceCreate(ctx context.Context, spec WorkspaceSpec) (WorkspaceInfo, error) {
	if err := c.guard("workspace", "create"); err != nil {
		return WorkspaceInfo{}, err
	}
	out, err := c.result(ctx, workspaceCreateArgs(spec)...)
	if err != nil {
		return WorkspaceInfo{}, err
	}
	return parseWorkspaceCreated(out)
}

// WorkspaceClose cierra un workspace. Con group intenta cerrar el grupo de
// worktrees vinculados (algunas versiones lo exigen).
func (c *Client) WorkspaceClose(ctx context.Context, workspaceID string, group bool) error {
	if err := c.guard("workspace", "close"); err != nil {
		return err
	}
	_, err := c.result(ctx, workspaceCloseArgs(workspaceID, group)...)
	return err
}

// TabCreate crea una pestaña dentro de un workspace.
func (c *Client) TabCreate(ctx context.Context, spec TabSpec) (TabInfo, error) {
	if err := c.guard("tab", "create"); err != nil {
		return TabInfo{}, err
	}
	out, err := c.result(ctx, tabCreateArgs(spec)...)
	if err != nil {
		return TabInfo{}, err
	}
	return parseTabCreated(out)
}

// TabRename etiqueta un tab. Es la única forma de nombrar el tab que ya trae el
// contenedor (el root pane del worktree): `tab create --label` solo aplica a los
// tabs que crea quien lo invoca.
func (c *Client) TabRename(ctx context.Context, tabID, label string) error {
	if err := c.guard("tab", "rename"); err != nil {
		return err
	}
	_, err := c.result(ctx, "tab", "rename", tabID, label)
	return err
}

// PaneSplit divide un pane y devuelve el pane nuevo.
func (c *Client) PaneSplit(ctx context.Context, spec SplitSpec) (PaneInfo, error) {
	if err := c.guard("pane", "split"); err != nil {
		return PaneInfo{}, err
	}
	out, err := c.result(ctx, splitArgs(spec)...)
	if err != nil {
		return PaneInfo{}, err
	}
	return parsePaneSplit(out)
}

// PaneRun lanza un comando en un pane. Es fire-and-forget: envía el comando más
// Enter al shell del pane y no espera a que termine. El argv se une en una sola
// línea de shell (con comillas) para que un argumento con espacios no se rompa.
func (c *Client) PaneRun(ctx context.Context, paneID string, argv []string) error {
	if len(argv) == 0 {
		return fmt.Errorf("pane run: empty argv")
	}
	if err := c.guard("pane", "run"); err != nil {
		return err
	}
	_, err := c.result(ctx, "pane", "run", paneID, strings.Join(argv, " "))
	return err
}

// PaneWaitOutput espera a que la salida de un pane contenga match.
func (c *Client) PaneWaitOutput(ctx context.Context, paneID, match string, timeout time.Duration) error {
	if err := c.guard("pane", "wait-output"); err != nil {
		return err
	}
	_, err := c.result(ctx, waitOutputArgs(paneID, match, timeout)...)
	return err
}

// PaneRename etiqueta un pane.
func (c *Client) PaneRename(ctx context.Context, paneID, label string) error {
	if err := c.guard("pane", "rename"); err != nil {
		return err
	}
	_, err := c.result(ctx, "pane", "rename", paneID, label)
	return err
}

// PaneFocus mueve el foco en una dirección. En Herdr 0.9.x el foco es
// direccional: no existe focus absoluto por id de pane.
func (c *Client) PaneFocus(ctx context.Context, direction string) error {
	if direction == "" {
		direction = "right"
	}
	if err := c.guard("pane", "focus"); err != nil {
		return err
	}
	_, err := c.result(ctx, "pane", "focus", "--direction", direction)
	return err
}

// PaneList lista los panes, opcionalmente acotado a un workspace.
func (c *Client) PaneList(ctx context.Context, workspaceID string) ([]PaneInfo, error) {
	out, err := c.result(ctx, paneListArgs(workspaceID)...)
	if err != nil {
		return nil, err
	}
	return parsePaneList(out)
}

// Notify muestra una notificación de Herdr.
func (c *Client) Notify(ctx context.Context, title string, opts NotifyOptions) error {
	if err := c.guard("notification", "show"); err != nil {
		return err
	}
	_, err := c.result(ctx, notifyArgs(title, opts)...)
	return err
}
