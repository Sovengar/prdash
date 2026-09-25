// Package tool ejecuta las CLIs de forge por subproceso con un entorno
// homogéneo: locale inglés, sin interacción, timeout y clasificación de
// errores. Es la única capa que lanza procesos; vive fuera del núcleo puro.
package tool

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"regexp"
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

// Error es el fallo de una CLI, con el código de salida y la causa preservada
// (Unwrap) para poder clasificarlo sin depender solo del texto.
type Error struct {
	Bin      string
	Args     []string
	ExitCode int
	Msg      string
	Err      error
}

// Error compone el mensaje del fallo incluyendo el código de salida.
func (e *Error) Error() string {
	base := fmt.Sprintf("%s %s: %s", e.Bin, strings.Join(e.Args, " "), e.Msg)
	if e.ExitCode != 0 {
		return fmt.Sprintf("%s (exit %d)", base, e.ExitCode)
	}
	return base
}

// Unwrap expone la causa subyacente (p. ej. *exec.ExitError).
func (e *Error) Unwrap() error { return e.Err }

// Run ejecuta el binario con los args dados y devuelve stdout. Ante un fallo
// devuelve stdout igualmente (algunas CLIs, como `gh pr checks`, traen salida
// válida con exit != 0) más un *Error con el código de salida.
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
		cerr := &Error{Bin: r.Bin, Args: args, Msg: msg, Err: err}
		var exit *exec.ExitError
		if errors.As(err, &exit) {
			cerr.ExitCode = exit.ExitCode()
		}
		return out.String(), cerr
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

// ExitCode devuelve el código de salida de un error de CLI, o 0.
func ExitCode(err error) int {
	var cerr *Error
	if errors.As(err, &cerr) {
		return cerr.ExitCode
	}
	var exit *exec.ExitError
	if errors.As(err, &exit) {
		return exit.ExitCode()
	}
	return 0
}

// Re compila los patrones de código HTTP presentes en el stderr de las CLIs.
var (
	httpCodeRe   = regexp.MustCompile(`(?i)\bhttp(?:/\d(?:\.\d)?)?\s+(\d{3})\b`)
	statusCodeRe = regexp.MustCompile(`(?i)\bstatus(?:\s+code)?[:\s]+(\d{3})\b`)
)

// HTTPStatus extrae el código HTTP de un mensaje de error, o 0 si no hay.
func HTTPStatus(msg string) int {
	for _, re := range []*regexp.Regexp{httpCodeRe, statusCodeRe} {
		if m := re.FindStringSubmatch(msg); m != nil {
			code := 0
			for _, r := range m[1] {
				code = code*10 + int(r-'0')
			}
			return code
		}
	}
	return 0
}

// Kind clasifica un error de CLI en la clase de warning correspondiente.
// El texto de rate limit se mira antes que el código HTTP porque GitHub usa
// 403 tanto para permiso como para primary/secondary rate limit; después manda
// el código HTTP y el texto queda como último recurso.
func Kind(err error) string {
	if err == nil {
		return ""
	}
	msg := strings.ToLower(err.Error())

	if isRateLimitText(msg) {
		return "ratelimit"
	}
	if code := HTTPStatus(err.Error()); code != 0 {
		if k := kindForHTTP(code); k != "" {
			return k
		}
	}

	// El rechazo de auto-aprobación se mira antes que el resto: es la única
	// razón por la que un forge veta aprobar, y no es un fallo de red ni un
	// conflicto de estado.
	if isSelfReviewText(msg) {
		return "selfreview"
	}

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

// isSelfReviewText reconoce el rechazo de aprobar un PR/MR propio. Cubre las
// redacciones de GitHub ("Can not approve your own pull request") y GitLab
// ("cannot approve your own merge request").
func isSelfReviewText(lower string) bool {
	return strings.Contains(lower, "approve your own")
}

// isRateLimitText reconoce los textos de límite de peticiones que GitHub y
// GitLab devuelven con 403 o 429.
func isRateLimitText(lower string) bool {
	return strings.Contains(lower, "rate limit") ||
		strings.Contains(lower, "abuse") ||
		strings.Contains(lower, "too many requests")
}

// kindForHTTP traduce un código HTTP a la clase de warning.
func kindForHTTP(code int) string {
	switch {
	case code == 401:
		return "auth"
	case code == 403:
		return "permission"
	case code == 404:
		return "notfound"
	case code == 409:
		return "conflict"
	case code == 429:
		return "ratelimit"
	case code >= 500:
		return "network"
	default:
		return ""
	}
}

// FirstLine recorta un mensaje a su primera línea.
func FirstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}
