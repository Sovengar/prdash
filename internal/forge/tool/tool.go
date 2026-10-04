// Package tool ejecuta las CLIs de forge por subproceso con un entorno
// homogéneo: locale inglés, sin interacción, timeout y clasificación de
// errores. Es la única capa que lanza procesos; vive fuera del núcleo puro.
package tool

import (
	"bytes"
	"context"
	"encoding/json"
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

// pipeCloseGrace es el margen para cerrar las tuberías de salida tras caducar el
// contexto. Sin ella el timeout mata al proceso pero `cmd.Run()` sigue esperando a que
// un hijo que heredó los descriptores suelte la tubería. Mismo motivo y mismo arreglo que
// en `internal/herdr`.
const pipeCloseGrace = 250 * time.Millisecond

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
	// Sin esto el timeout no corta: ver `pipeCloseGrace`.
	cmd.WaitDelay = pipeCloseGrace
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
//
// El orden de las comprobaciones es una tabla de precedencias entre señales que se
// contradicen, y no un orden arbitrario:
//
//  1. Texto de rate limit, antes que el código HTTP, porque GitHub usa 403 tanto para
//     permiso como para rate limit.
//  2. Texto de no integrable, antes que el código HTTP, por el mismo conflicto: GitHub
//     responde 409 tanto a un rechazo de merge como a un estado que se resuelve
//     refrescando.
//  3. Texto de autorrechazo, antes que el código HTTP, porque GitHub responde 422 a
//     "no puedes aprobar tu propio PR" y 422 es la clase de validación de contenido.
//
// Ese tercer punto estuvo en el sitio equivocado durante un tiempo, con el comentario
// diciendo lo contrario del código: `isSelfReviewText` se comprobaba DESPUÉS del código
// HTTP, así que nunca veía nada. Con el texto solo —sin el "(HTTP 422)" que gh añade al
// stderr— la clasificación era correcta, y con el texto real de GitHub salía
// "validation". Y "validation" no es permiso, así que la TUI no registraba la denegación
// y volvía a intentarlo cada vez que se pulsaba la tecla, contra un PR que no se puede
// aprobar nunca. El aserto que fija esto es el caso con el código HTTP incluido, no el
// que se lo quita.
//
//  4. Código HTTP, que es lo único que queda cuando el texto no dice nada.
//  5. Texto restante, como último recurso.
func Kind(err error) string {
	if err == nil {
		return ""
	}
	msg := strings.ToLower(err.Error())

	if isRateLimitText(msg) {
		return "ratelimit"
	}
	// El texto de "no integrable" se mira antes que el código HTTP por la misma
	// razón que el de rate limit, y por el mismo conflicto de vocabulario: GitHub
	// responde 409 a un rechazo de merge por ramas y también a un estado que sí
	// se resuelve refrescando, así que el código no distingue y el texto sí.
	if isUnmergeableText(msg) {
		return "unmergeable"
	}
	// El rechazo de auto-aprobación es la única razón por la que un forge veta aprobar,
	// y no es un fallo de red ni un conflicto de estado. Antes que el código HTTP, que
	// con 422 lo clasificaría como validación —ver el doc de la función.
	if isSelfReviewText(msg) {
		return "selfreview"
	}
	if code := HTTPStatus(err.Error()); code != 0 {
		if k := kindForHTTP(code); k != "" {
			return k
		}
	}

	switch {
	case strings.Contains(msg, "deadline exceeded"), strings.Contains(msg, "timed out"):
		return "timeout"
	case strings.Contains(msg, "rate limit"), strings.Contains(msg, "429"), strings.Contains(msg, "abuse"):
		return "ratelimit"
	case strings.Contains(msg, "409"), strings.Contains(msg, "conflict"),
		strings.Contains(msg, "already closed"), strings.Contains(msg, "already merged"):
		return "conflict"
	// El 422 es "la petición está bien pero su contenido no vale": un base que no
	// existe, un título vacío, una etiqueta mal formada. Es su propia clase y no
	// cabe en ninguna de las de arriba. Antes caía en `network`, porque el texto
	// que llega por stderr no tiene ni "conflict" ni "not found" —el motivo de
	// verdad va en el cuerpo JSON—, y el clasificador se quedaba sin nada.
	//
	// No es conflicto (un refresco no lo arregla: el valor sigue siendo malo) ni
	// permiso (eso dejaría la acción deshabilitada para siempre). Es un error de
	// la llamada, que es lo que la TUI enseña sin prometerle nada a nadie.
	case strings.Contains(msg, "422"):
		return "validation"
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

// isUnmergeableText reconoce el rechazo de un merge porque el forge no puede
// crearlo con las ramas como están.
//
// Cubre las redacciones de GitHub ("Pull request …#6 is not mergeable: the merge
// commit cannot be cleanly created", y el 409 "Head branch was modified. Review
// and try the merge again.") y las de GitLab ("The merge request cannot merge",
// "You need to rebase", "Branch is not up to date"). Un texto que no se
// reconoce cae en el cubo genérico, que es el comportamiento correcto ante lo
// desconocido: se clasifica peor, no se miente mejor.
func isUnmergeableText(lower string) bool {
	return strings.Contains(lower, "not mergeable") ||
		strings.Contains(lower, "cannot be cleanly created") ||
		strings.Contains(lower, "head branch was modified") ||
		strings.Contains(lower, "cannot merge") ||
		strings.Contains(lower, "cannot be merged") ||
		strings.Contains(lower, "need to rebase") ||
		strings.Contains(lower, "not up to date")
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
	case code == 422:
		return "validation"
	case code == 429:
		return "ratelimit"
	case code >= 500:
		return "network"
	default:
		return ""
	}
}

// APIMessage saca el motivo que devuelve la API en el cuerpo de una respuesta con
// salida distinta de cero.
//
// Existe porque las CLIs de forge no lo pasan por stderr: a stderr llega una línea
// con el argv entero (`gh api -X PATCH repos/o/r/pulls/1 -f base=x: gh: Validation
// Failed (HTTP 422)`) y el motivo de verdad va en el cuerpo JSON. Sin esto, el
// motivo que ve el usuario es el comando que falló y no por qué falló, y en un 422
// la diferencia es enorme: `Proposed base branch 'main2' was not found` es
// accionable y `Validation Failed` no lo es.
//
// Prefiere `errors[].message` sobre `message` porque las APIs los usan para cosas
// distintas: GitHub escribe `Validation Failed` en el primero y el detalle de qué
// campo no valía en el segundo, así que al revés se enseñaría el genérico. Cae a
// `message` cuando no hay `errors`, que es lo que hace GitLab.
func APIMessage(body string) string {
	var payload struct {
		Message any `json:"message"`
		Errors  []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}
	if err := json.Unmarshal([]byte(body), &payload); err != nil {
		return ""
	}
	for _, e := range payload.Errors {
		if msg := strings.TrimSpace(e.Message); msg != "" {
			return msg
		}
	}
	// El `message` de GitLab puede ser un objeto cuando el error es de campo, y un
	// objeto no es un motivo que se pueda enseñar: solo se acepta como string.
	if msg, ok := payload.Message.(string); ok {
		return strings.TrimSpace(msg)
	}
	return ""
}

// FirstLine recorta un mensaje a su primera línea.
func FirstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}
