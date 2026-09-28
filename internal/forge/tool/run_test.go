package tool

import (
	"context"
	"errors"
	"os/exec"
	"strings"
	"testing"
	"time"
)

// shRunner es un Runner que habla con /bin/sh, que existe en cualquier runner de
// CI y no depende de git ni de gh.
func shRunner(timeout time.Duration) *Runner { return &Runner{Bin: "sh", Timeout: timeout} }

// TestRunAplicaElTimeoutPorDefecto cuando no hay timeout configurado. Sin el
// default, un `gh` colgado dejaría la TUI esperando para siempre: el inbox no
// refresca y el usuario ve una lista congelada sin aviso.
//
// El caso que importa es Timeout=0 (cero explícito, no el que pone New): es el
// valor de un Runner construido a mano, y `<= 0` es lo que lo recoge. Con
// contexto ya vencido, un timeout de 0 es un plazo que pasó hace 30 segundos, así
// que sin el default el comando muere al instante aunque sea trivial.
func TestRunAplicaElTimeoutPorDefecto(t *testing.T) {
	// Cero: el comando trivial tiene que salir bien, o sea que el plazo real era
	// el default y no cero.
	if _, err := shRunner(0).Run(context.Background(), "-c", "echo ok"); err != nil {
		t.Errorf("timeout=0 debería usar el default, pero falló: %v", err)
	}
	// Negativo: un Runner mal formado, que el default también corrige. Con el
	// guard, un plazo negativo se vuelve el default y el comando sale bien; sin
	// él, WithTimeout lo trata como vencido y cancela al instante. Por eso el
	// caso se afirma como ÉXITO, que es lo contrario de lo intuitivo.
	if _, err := shRunner(-time.Second).Run(context.Background(), "-c", "sleep 1"); err != nil {
		t.Errorf("un timeout negativo debería caer al default, pero falló: %v", err)
	}
	// Y Run no toca el Timeout del Runner: si lo mutara, el segundo Run usaría
	// el default como timeout propio y dejaría de ser corregible.
	r := shRunner(0)
	if _, err := r.Run(context.Background(), "-c", "echo ok"); err != nil {
		t.Fatal(err)
	}
	if r.Timeout != 0 {
		t.Errorf("Run mutó el Timeout del Runner: %v", r.Timeout)
	}
	// Un timeout positivo y corto se respeta: el comando duerme más de lo que se
	// le da y muere por plazo, que es la otra mitad del contrato.
	if _, err := shRunner(50*time.Millisecond).Run(context.Background(), "-c", "sleep 5"); err == nil {
		t.Error("un comando que excede el timeout tiene que fallar")
	}
}

// TestRunUsaElMensajeDeStderrYElDeSuUltimoRespaldo: el motivo que ve el usuario
// sale de stderr, y stderr vacío cae al error del propio proceso. Sin ese
// respaldo, un binario que falla sin escribir nada daría un Error con Msg vacío,
// que se clasificaría como "network" sin motivo y dejaría al usuario sin nada
// que hacer.
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

	// stderr vacío: el motivo sale del error del proceso, y no queda vacío.
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

	// stderr solo con blancos: también cuenta como vacío (se hace TrimSpace
	// antes de mirar), así que el motivo sale del error del proceso.
	_, err = shRunner(time.Second).Run(context.Background(), "-c", "echo '   ' >&2; exit 5")
	if !errors.As(err, &cerr) {
		t.Fatalf("error = %T, want *tool.Error", err)
	}
	if cerr.Msg == "" || cerr.Msg == "   " {
		t.Errorf("Msg = %q, want el error del proceso y no los blancos", cerr.Msg)
	}
}

// TestRunDevuelveStdoutAunqueFalle: algunas CLIs de forge traen la respuesta
// válida en stdout y salen con código distinto de cero (`gh pr checks` con un
// check en rojo, entre otros). Tirar esa salida deja al usuario sin el dato que
// sí llegó, y el aviso sin el porqué.
func TestRunDevuelveStdoutAunqueFalle(t *testing.T) {
	out, err := shRunner(time.Second).Run(context.Background(), "-c", "echo 'salida válida'; exit 1")
	if err == nil {
		t.Fatal("se esperaba error")
	}
	if strings.TrimSpace(out) != "salida válida" {
		t.Errorf("stdout = %q, want la salida válida aunque el comando falle", out)
	}
}

// TestKindIgnoraElCodigoHTTPQueNoSeparaNada es la precondición de la cadena de
// clasificación: un texto sin código HTTP debe caer al análisis de texto, y un
// código que kindForHTTP no traduce (200, 418…) también, porque ahí lo único que
// hay es el fraseo.
//
// Si el código 0 (que es lo que devuelve HTTPStatus cuando no encuentra nada) se
// tomara como un código real, todo mensaje sin código se clasificaría por
// kindForHTTP(0) = "" y se perdería el análisis de texto entero.
func TestKindIgnoraElCodigoHTTPQueNoSeparaNada(t *testing.T) {
	// Un texto sin código HTTP se clasifica por su fraseo, no por un 0 ficticio.
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

	// Un código que kindForHTTP no traduce deja pasar el texto. El 418 es el
	// ejemplo limpio: es un código real y no significa nada para prdash.
	if got := Kind(errors.New("HTTP 418: I am a teapot, not authorized")); got != "permission" {
		t.Errorf("con un código sin traducción debe mandar el texto, dio %q", got)
	}
	// Y el caso que de verdad fija el ORDEN: mensajes donde el código HTTP y el
	// texto dicen cosas distintas. Ahí manda el código, y la clase cambia. Con
	// texto en vez de código, un 404 de un conflicto se ofrecería como "refresca"
	// cuando lo que hay que rehacer es un rebase, o un 403 de rate limit se
	// anunciaría como permiso y la acción quedaría deshabilitada para siempre.
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
	// Los que sí, para que la tabla no se vacíe por accidente.
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

// TestHTTPStatusRecomponeElCodigo: el código se lee dígito a dígito de una
// cadena, así que la aritmética del acumulador es lo que lo compose. Un error
// ahí no da un código equivocado: da el código equivocado Y con el mismo aspecto
// (tres dígitos), que es peor, porque un 404 leído como 404 con otro valor
// clasifica el error en la clase de otro.
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
		// Lo que parece un código pero no lo es. Un falso positivo aquí no es un
		// 0: es un código equivocado que clasifica el error en la clase de otro,
		// y por eso el patrón es estrecho a propósito.
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

// TestFirstLineRecortaSinPerderNada: FirstLine es lo que decide qué frase entra
// en el clasificador, y un mensaje de varias líneas del forge trae el motivo
// detrás del argv. Sin recorte, el texto entero acabaría en el aviso.
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

// TestExitCodeLeeElDeLaCadenaYSTecla lo que devuelven: 0 si no hay código en
// error ninguno, y el del subproceso cuando lo hay. Un 0 falso se lee como
// "salió bien" en quien lo use para decidir.
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
	// Y el exec.ExitError crudo, que es lo que devuelve exec sin envolver.
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
	// Se devuelve la causa cruda de exec, que es lo que exec.ExitError quiere.
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
