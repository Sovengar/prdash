package tool

import (
	"context"
	"errors"
	"os/exec"
	"strings"
	"testing"
	"time"
)

func shRunner(timeout time.Duration) *Runner { return &Runner{Bin: "sh", Timeout: timeout} }

// Without the default, a hung `gh` leaves the TUI frozen.
func TestRunAplicaElTimeoutPorDefecto(t *testing.T) {
	if _, err := shRunner(0).Run(context.Background(), "-c", "echo ok"); err != nil {
		t.Errorf("timeout=0 debería usar el default, pero falló: %v", err)
	}
	// Negative: a malformed Runner, which the default also fixes.
	if _, err := shRunner(-time.Second).Run(context.Background(), "-c", "sleep 1"); err != nil {
		t.Errorf("un timeout negativo debería caer al default, pero falló: %v", err)
	}
	// Run does not touch the Runner's Timeout: mutating it would make the second Run use the
	// default instead of the configured one.
	r := shRunner(0)
	if _, err := r.Run(context.Background(), "-c", "echo ok"); err != nil {
		t.Fatal(err)
	}
	if r.Timeout != 0 {
		t.Errorf("Run mutó el Timeout del Runner: %v", r.Timeout)
	}
	if _, err := shRunner(50*time.Millisecond).Run(context.Background(), "-c", "sleep 5"); err == nil {
		t.Error("un comando que excede el timeout tiene que fallar")
	}
}

func TestRunUsaElMensajeDeStderrYElDeSuUltimoRespaldo(t *testing.T) {
	// stderr presente: manda stderr, y solo su primera línea.
	_, err := shRunner(time.Second).Run(context.Background(), "-c", "echo 'el motivo real' >&2; exit 3")
	var cerr *Error
	if !errors.As(err, &cerr) {
		t.Fatalf("error = %T, want *tool.Error", err)
	}
	if cerr.Msg != "el motivo real" {
		t.Errorf("Msg = %q, want el stderr", cerr.Msg)
	}
	if cerr.ExitCode != 3 {
		t.Errorf("ExitCode = %d, want 3", cerr.ExitCode)
	}

	_, err = shRunner(time.Second).Run(context.Background(), "-c", "exit 7")
	if !errors.As(err, &cerr) {
		t.Fatalf("error = %T, want *tool.Error", err)
	}
	if cerr.Msg == "" {
		t.Error("sin stderr el motivo no puede quedar vacío")
	}
	if cerr.ExitCode != 7 {
		t.Errorf("ExitCode = %d, want 7", cerr.ExitCode)
	}

	// Whitespace-only stderr counts as empty too, because it is trimmed before the check.
	_, err = shRunner(time.Second).Run(context.Background(), "-c", "echo '   ' >&2; exit 5")
	if !errors.As(err, &cerr) {
		t.Fatalf("error = %T, want *tool.Error", err)
	}
	if cerr.Msg == "" || cerr.Msg == "   " {
		t.Errorf("Msg = %q, want el error del proceso y no los blancos", cerr.Msg)
	}
}

// Some forge CLIs print a valid answer on stdout and exit non-zero.
func TestRunDevuelveStdoutAunqueFalle(t *testing.T) {
	out, err := shRunner(time.Second).Run(context.Background(), "-c", "echo 'salida válida'; exit 1")
	if err == nil {
		t.Fatal("se esperaba error")
	}
	if strings.TrimSpace(out) != "salida válida" {
		t.Errorf("stdout = %q, want la salida válida aunque el comando falle", out)
	}
}

// The precondition of the classification chain.
func TestKindIgnoraElCodigoHTTPQueNoSeparaNada(t *testing.T) {
	// Text with no HTTP code is classified by its wording, not by a fake 0.
	for msg, want := range map[string]string{
		"401 Unauthorized":           "auth",
		"404 Not Found":              "notfound",
		"403 Forbidden":              "permission",
		"you must have push access":  "permission",
		"context deadline exceeded":  "timeout",
		"something entirely unknown": "network",
	} {
		if got := Kind(errors.New(msg)); got != want {
			t.Errorf("Kind(%q) = %q, want %q", msg, got, want)
		}
	}

	// A code kindForHTTP does not translate lets the text through; 418 is the clean case.
	if got := Kind(errors.New("HTTP 418: I am a teapot, not authorized")); got != "permission" {
		t.Errorf("con un código sin traducción debe mandar el texto, dio %q", got)
	}
	// The case that really pins the ORDER: messages where the HTTP code and the text disagree.
	discrepan := map[string]string{
		"HTTP 404: merge conflict":            "notfound",
		"HTTP 401: forbidden":                 "auth",
		"HTTP 403: you must have push access": "permission",
		"HTTP 429: conflict while merging":    "ratelimit",
		"HTTP 503: 404 not found":             "network",
		"HTTP 422: permission denied":         "validation",
		"HTTP 409: you must have push access": "conflict",
		"HTTP 500: 401 Unauthorized":          "network",
	}
	for msg, want := range discrepan {
		if got := Kind(errors.New(msg)); got != want {
			t.Errorf("Kind(%q) = %q, want %q (el código HTTP manda sobre el texto)", msg, got, want)
		}
	} // Y KindForHTTP translation: los que no tienen traducción devuelven vacío.
	for _, code := range []int{0, 200, 201, 204, 301, 418, 451, 499} {
		if got := kindForHTTP(code); got != "" {
			t.Errorf("kindForHTTP(%d) = %q, want \"\" (sin traducción, que el texto decida)", code, got)
		}
	}
	for code, want := range map[int]string{
		401: "auth", 403: "permission", 404: "notfound",
		409: "conflict", 422: "validation", 429: "ratelimit",
		500: "network", 503: "network", 599: "network",
	} {
		if got := kindForHTTP(code); got != want {
			t.Errorf("kindForHTTP(%d) = %q, want %q", code, got, want)
		}
	}
}

func TestHTTPStatusRecomponeElCodigo(t *testing.T) {
	cases := map[string]int{
		"HTTP 404: Not Found":                   404,
		"HTTP 503 Service Unavailable":          503,
		"status code 401":                       401,
		"status: 429":                           429,
		"http/1.1 200 OK":                       200,
		"HTTP 000":                              0,
		"sin codigo":                            0,
		"":                                      0,
		"HTTP 40":                               0, // incompleto: no se inventa
		"status code 40a":                       0,
		"el puerto 8080 no es un código":        0,
		"HTTP 403 sin texto de estado":          403,
		"se receiving 200 pero no es un status": 0,
		"status code: 422 con dos puntos":       422,
		// Something that looks like a code but is not. A false positive here is not a 0: it is a code
		// printed as if it were one.
		"X-Status-Code: 409 en una cabecera": 0,
		"status_code 404 con guion bajo":     0,
		"HTTP 40x4 no es un código":          0,
	}
	for msg, want := range cases {
		if got := HTTPStatus(msg); got != want {
			t.Errorf("HTTPStatus(%q) = %d, want %d", msg, got, want)
		}
	}
}

func TestFirstLineRecortaSinPerderNada(t *testing.T) {
	cases := map[string]string{
		"uno":                "uno",
		"uno\ndos":           "uno",
		"\nuno":              "", // la primera línea está vacía: no hay motivo
		"uno\n\ndos":         "uno",
		"uno\n":              "uno",
		"":                   "",
		"\n":                 "",
		"con\ntres\nlineas":  "con",
		"sin salto de linea": "sin salto de linea",
	}
	for in, want := range cases {
		if got := FirstLine(in); got != want {
			t.Errorf("FirstLine(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestExitCodeLeeElDeLaCadenaYSTecla(t *testing.T) {
	if got := ExitCode(nil); got != 0 {
		t.Errorf("ExitCode(nil) = %d, want 0", got)
	}
	if got := ExitCode(errors.New("un error cualquiera")); got != 0 {
		t.Errorf("ExitCode(otro error) = %d, want 0", got)
	}
	if got := ExitCode(&Error{ExitCode: 42}); got != 42 {
		t.Errorf("ExitCode(*Error) = %d, want 42", got)
	}
	// Un *Error envuelto en otro error sigue dando su código: es el contrato de
	// Unwrap, y quien lo envuelve no debe perder la información.
	if got := ExitCode(errWrap{&Error{ExitCode: 9}}); got != 9 {
		t.Errorf("ExitCode(envuelto) = %d, want 9", got)
	}
	var exitErr *exec.ExitError
	if errors.As(shRunFailure(t, 6), &exitErr) {
		if got := ExitCode(exitErr); got != 6 {
			t.Errorf("ExitCode(exec.ExitError) = %d, want 6", got)
		}
	}
}

type errWrap struct{ err error }

func (e errWrap) Error() string { return "envuelto: " + e.err.Error() }
func (e errWrap) Unwrap() error { return e.err }

func shRunFailure(t *testing.T, code int) error {
	t.Helper()
	_, err := shRunner(time.Second).Run(context.Background(), "-c", "exit "+itoa(code))
	if err == nil {
		t.Fatal("se esperaba fallo")
	}
	var cerr *Error
	if !errors.As(err, &cerr) {
		t.Fatalf("error = %T, want *tool.Error", err)
	}
	return cerr.Err
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}
