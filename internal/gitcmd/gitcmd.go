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
// forzar mensajes en inglés y añade modo no interactivo (sin prompt de
// credenciales, sin pager, sin color).
func Env() []string {
	env := os.Environ()
	out := env[:0]
	for _, kv := range env {
		switch {
		case strings.HasPrefix(kv, "LC_ALL="),
			strings.HasPrefix(kv, "LANG="),
			strings.HasPrefix(kv, "LANGUAGE="),
			strings.HasPrefix(kv, "LC_MESSAGES="):
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
