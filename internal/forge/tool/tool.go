// Package tool ejecuta las CLIs de forge por subproceso con un entorno
// homogéneo: locale inglés, sin interacción, timeout y clasificación de
// errores. Es la única capa que lanza procesos; vive fuera del núcleo puro.
package tool

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

// DefaultTimeout es el límite por invocación de una CLI de forge.
const DefaultTimeout = 30 * time.Second

// Runner ejecuta un binario con timeout y entorno no interactivo.
type Runner struct {
	Bin     string
	Timeout time.Duration
	Extra   []string // variables extra del forge (p. ej. GH_PROMPT_DISABLED=1)
}

// New construye un Runner con timeout por defecto y las variables extra dadas.
func New(bin string, extra ...string) *Runner {
	return &Runner{Bin: bin, Timeout: DefaultTimeout, Extra: extra}
}

// Run ejecuta el binario con los args dados y devuelve stdout. Ante un fallo
// devuelve un error que resume la primera línea de stderr.
func (r *Runner) Run(ctx context.Context, args ...string) (string, error) {
	timeout := r.Timeout
	if timeout <= 0 {
		timeout = DefaultTimeout
	}
	cctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(cctx, r.Bin, args...)
	cmd.Env = Env(r.Extra...)
	var out, errb bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errb
	if err := cmd.Run(); err != nil {
		msg := FirstLine(strings.TrimSpace(errb.String()))
		if msg == "" {
			msg = err.Error()
		}
		return out.String(), fmt.Errorf("%s %s: %s", r.Bin, strings.Join(args, " "), msg)
	}
	return out.String(), nil
}

// Env compone el entorno del subproceso: descarta el locale del usuario para
// forzar mensajes en inglés, y añade modo no interactivo más las variables
// extra del forge.
func Env(extra ...string) []string {
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
	out = append(out, "LC_ALL=C", "GIT_TERMINAL_PROMPT=0", "NO_COLOR=1")
	return append(out, extra...)
}

// Kind clasifica un error de CLI en la clase de warning correspondiente.
func Kind(err error) string {
	if err == nil {
		return ""
	}
	msg := strings.ToLower(err.Error())
	switch {
	case strings.Contains(msg, "deadline exceeded"), strings.Contains(msg, "timed out"):
		return "timeout"
	case strings.Contains(msg, "rate limit"), strings.Contains(msg, "429"), strings.Contains(msg, "abuse"):
		return "ratelimit"
	case strings.Contains(msg, "409"), strings.Contains(msg, "conflict"), strings.Contains(msg, "not mergeable"),
		strings.Contains(msg, "already closed"), strings.Contains(msg, "already merged"):
		return "conflict"
	case strings.Contains(msg, "401"), strings.Contains(msg, "unauthorized"),
		strings.Contains(msg, "not logged"), strings.Contains(msg, "authentication failed"):
		return "auth"
	case strings.Contains(msg, "403"), strings.Contains(msg, "forbidden"),
		strings.Contains(msg, "not authorized"), strings.Contains(msg, "insufficient"),
		strings.Contains(msg, "permission"), strings.Contains(msg, "must have push"):
		return "permission"
	case strings.Contains(msg, "404"), strings.Contains(msg, "not found"):
		return "notfound"
	default:
		return "network"
	}
}

// FirstLine recorta un mensaje a su primera línea.
func FirstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}
