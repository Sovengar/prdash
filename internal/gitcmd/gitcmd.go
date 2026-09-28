// Package gitcmd ejecuta git por subproceso con un entorno no interactivo y
// locale inglés, de modo que ningún comando abra editor, pager o pida
// credenciales, y los mensajes de error sean parseables. Es el único adaptador
// de subproceso de git: lo comparten el resolutor de repos y la provisión de
// worktrees.
package gitcmd

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

// DefaultTimeout es el límite por invocación de git (fetch/clone pueden tardar).
const DefaultTimeout = 60 * time.Second

// Runner ejecuta git.
type Runner struct {
	// Bin es el binario de git; vacío usa "git".
	Bin string
	// Timeout por invocación; <=0 usa DefaultTimeout.
	Timeout time.Duration
}

// New construye un Runner con el binario y timeout por defecto.
func New() *Runner { return &Runner{Bin: "git", Timeout: DefaultTimeout} }

// Error es el fallo de una invocación de git, con el código de salida y la
// causa preservada (Unwrap) para poder clasificarlo sin depender del texto.
type Error struct {
	Args     []string
	Dir      string
	ExitCode int
	Msg      string
	Err      error
}

// Error compone el mensaje incluyendo el directorio y el código de salida.
func (e *Error) Error() string {
	base := fmt.Sprintf("git %s: %s", strings.Join(e.Args, " "), e.Msg)
	if e.Dir != "" {
		base = fmt.Sprintf("git -C %s %s: %s", e.Dir, strings.Join(e.Args, " "), e.Msg)
	}
	if e.ExitCode != 0 {
		return fmt.Sprintf("%s (exit %d)", base, e.ExitCode)
	}
	return base
}

// Unwrap expone la causa subyacente (p. ej. *exec.ExitError).
func (e *Error) Unwrap() error { return e.Err }

// Run ejecuta git en dir (vacío = directorio actual) y devuelve stdout
// recortado. Ante un fallo devuelve la salida parcial más el error.
func (r *Runner) Run(ctx context.Context, dir string, args ...string) (string, error) {
	bin := r.Bin
	if bin == "" {
		bin = "git"
	}
	timeout := r.Timeout
	if timeout <= 0 {
		timeout = DefaultTimeout
	}
	cctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(cctx, bin, args...)
	if dir != "" {
		cmd.Dir = dir
	}
	cmd.Env = Env()
	var out, errb bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errb
	if err := cmd.Run(); err != nil {
		msg := firstLine(strings.TrimSpace(errb.String()))
		if msg == "" {
			msg = err.Error()
		}
		gerr := &Error{Args: args, Dir: dir, Msg: msg, Err: err}
		var exit *exec.ExitError
		if errors.As(err, &exit) {
			gerr.ExitCode = exit.ExitCode()
		}
		return out.String(), gerr
	}
	return out.String(), nil
}

// Env compone el entorno del subproceso: descarta el locale del usuario para
// forzar mensajes en inglés, quita las variables GIT_* de localización y añade
// modo no interactivo (sin prompt de credenciales, sin pager, sin color).
//
// Las GIT_* de localización se quitan porque le ganan a cmd.Dir: con GIT_DIR (o
// GIT_WORK_TREE, GIT_INDEX_FILE…) en el entorno, git opera en ESE repo y da
// igual el directorio en el que se le ejecute. prdash elige el repo de cada ítem
// por su cuenta, así que heredar el contexto de git de quien lo lanzó haría que
// una operación de la TUI cayera en un repo que no es el del ítem — que es
// justo el fallo que borra la rama equivocada. prdash se llama siempre con el
// repo explícito en el argumento, no con el contexto del shell.
func Env() []string {
	env := os.Environ()
	out := env[:0]
	for _, kv := range env {
		switch {
		case strings.HasPrefix(kv, "LC_ALL="),
			strings.HasPrefix(kv, "LANG="),
			strings.HasPrefix(kv, "LANGUAGE="),
			strings.HasPrefix(kv, "LC_MESSAGES="),
			strings.HasPrefix(kv, "GIT_DIR="),
			strings.HasPrefix(kv, "GIT_WORK_TREE="),
			strings.HasPrefix(kv, "GIT_INDEX_FILE="),
			strings.HasPrefix(kv, "GIT_COMMON_DIR="),
			strings.HasPrefix(kv, "GIT_OBJECT_DIRECTORY="),
			strings.HasPrefix(kv, "GIT_ALTERNATE_OBJECT_DIRECTORIES="),
			strings.HasPrefix(kv, "GIT_NAMESPACE="),
			strings.HasPrefix(kv, "GIT_CEILING_DIRECTORIES="),
			strings.HasPrefix(kv, "GIT_PREFIX="):
			continue
		}
		out = append(out, kv)
	}
	return append(out,
		"LC_ALL=C",
		"GIT_TERMINAL_PROMPT=0",
		"GIT_PAGER=cat",
		"NO_COLOR=1",
	)
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}
