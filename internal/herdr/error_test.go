package herdr

import (
	"errors"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"testing"
)

// exitErrorFor devuelve el error que produce exec cuando un proceso sale con
// código. Se lanza de verdad un proceso, porque `*exec.ExitError` solo lleva el
// código a través de su ProcessState y no hay forma de fabricarlo.
//
// Se usa el patrón del proceso ayudante de la librería estándar (re-ejecutar el
// propio binario de test con una env que lo convierte en un programa que sale con
// código) en vez de `sh -c`: así el test no depende de que haya una shell en el
// PATH, que es la misma razón por la que la suite no necesita gh ni glab.
func exitErrorFor(t *testing.T, code int) error {
	t.Helper()
	cmd := exec.Command(os.Args[0], "-test.run=TestProcesoAyudanteQueSaleConCodigo")
	cmd.Env = append(os.Environ(), "PRDASH_TEST_RUN_HELPER=1", "PRDASH_TEST_EXIT_CODE="+strconv.Itoa(code))
	err := cmd.Run()
	if err == nil {
		t.Fatalf("el proceso ayudante salió bien en vez de con %d", code)
	}
	var exit *exec.ExitError
	if !errors.As(err, &exit) {
		t.Fatalf("el proceso ayudante dio %T y no un *exec.ExitError: %v", err, err)
	}
	return err
}

// TestProcesoAyudanteQueSaleConCodigo no hace nada de especial: solo existe para que
// exitErrorFor lo lance y saque el código que le pidan por la env.
func TestProcesoAyudanteQueSaleConCodigo(t *testing.T) {
	if os.Getenv("PRDASH_TEST_RUN_HELPER") == "" {
		t.Skip("solo se ejecuta como proceso ayudante")
	}
	code, err := strconv.Atoi(os.Getenv("PRDASH_TEST_EXIT_CODE"))
	if err != nil || code == 0 {
		os.Exit(1)
	}
	os.Exit(code)
}

// TestNewErrorTomaElCodigoYElMensajeDelServidor: el JSON de stderr de Herdr trae
// dos cosas y las dos se usan: un código estable y un mensaje para humanos.
//
// Son cosas distintas a propósito. El código es lo que un test puede afirmar y lo
// que un usuario puede reportar sin ambigüedad; el mensaje es lo que se lee. Por eso
// `newError` guarda las dos, y por eso se pueden dar por separado: hay servidores que
// mandan código sin mensaje, y otros mandan mensaje sin código.
func TestNewErrorTomaElCodigoYElMensajeDelServidor(t *testing.T) {
	casos := []struct {
		nombre   string
		stderr   string
		wantCode string
		wantMsg  string
	}{
		{
			"código y mensaje anidados",
			`{"error":{"code":"E_NOPE","message":"no existe la rama"}}`,
			"E_NOPE", "no existe la rama",
		},
		{
			"código y mensajes planos",
			`{"code":"E_PLAN","message":"el plan no cabe"}`,
			"E_PLAN", "el plan no cabe",
		},
		// Solo código: el mensaje viene del error de exec, no del servidor. Es el
		// caso que separa el `||` del `&&` de la condición: con solo el código, un
		// `&&` no entraría y el código se perdería.
		{
			"solo código",
			`{"error":{"code":"E_SOLO"}}`,
			"E_SOLO", "exit status 3",
		},
		{
			"solo código plano",
			`{"code":"E_SOLO"}`,
			"E_SOLO", "exit status 3",
		},
		// Solo mensaje: el código se queda vacío, que es lo que la TUI lee como
		// "no hay código". Es el otro caso que separa el `||` del `&&`.
		{
			"solo mensaje",
			`{"error":{"message":"algo falló"}}`,
			"", "algo falló",
		},
		{
			"solo mensaje plano",
			`{"message":"algo falló"}`,
			"", "algo falló",
		},
		// Un mensaje explícitamente vacío con código presente: el mensaje vacío NO
		// puede pisar el mensaje del error de exec. Un `==` en lugar del `!=` lo
		// pisaría con la cadena vacía, y el usuario vería un error sin decir nada.
		{
			"código con mensaje vacío",
			`{"error":{"code":"E_VACIO","message":""}}`,
			"E_VACIO", "exit status 3",
		},
		// JSON que no es de Herdr: no tiene code ni message, así que no es una
		// respuesta de servidor y se muestra tal cual lo imprimió la CLI. Es lo
		// correcto: si Herdr suelta algo que no entendemos, esconderlo detrás de
		// "exit status 1" pierde la única información que había.
		{
			"JSON ajeno",
			`{"otra":"cosa"}`,
			"", `{"otra":"cosa"}`,
		},
		{
			"no es JSON",
			"panic: algo se rompió",
			"", "panic: algo se rompió",
		},
		{
			"stderr vacío",
			"",
			"", "exit status 3",
		},
	}

	for _, c := range casos {
		err := errors.New("exit status 3")
		e := newError([]string{"herdr", "tab", "create"}, err, []byte(c.stderr))

		if e.Code != c.wantCode {
			t.Errorf("%s: el código quedó en %q, want %q", c.nombre, e.Code, c.wantCode)
		}
		if e.Msg != c.wantMsg {
			t.Errorf("%s: el mensaje quedó en %q, want %q", c.nombre, e.Msg, c.wantMsg)
		}
		// Y el mensaje NUNCA queda vacío: un error sin mensaje es un error que no
		// dice nada, y la TUI lo pinta como un fallo sin explicación.
		if e.Msg == "" {
			t.Errorf("%s: el mensaje quedó vacío: un error sin texto no informa de nada", c.nombre)
		}
		// Y el error original se guarda, para poder desenredar el stack.
		if !errors.Is(e.Err, err) {
			t.Errorf("%s: el error original no se guardó", c.nombre)
		}
		// Y los args, que son lo que se repite en el mensaje al usuario.
		if strings.Join(e.Args, " ") != "herdr tab create" {
			t.Errorf("%s: los args quedaron en %q", c.nombre, e.Args)
		}
	}
}

// TestNewErrorTomaElCodigoDeSalidaSoloSiLoTiene: el código de salida viene del
// error de exec, y solo se lee si el error ES de exec.
//
// Con un error que no viene de un proceso (un binario que no está, por ejemplo) no
// hay código de salida, y el que se pondría es 0, que se lee como "terminó bien".
// Por eso se comprueba el tipo en vez de leer ExitCode() a ciegas.
func TestNewErrorTomaElCodigoDeSalidaSoloSiLoTiene(t *testing.T) {
	// Un error de proceso con código 3: se lee.
	e := newError([]string{"herdr"}, exitErrorFor(t, 3), nil)
	if e.Exit != 3 {
		t.Errorf("con un ExitError(3) quedó Exit=%d, want 3", e.Exit)
	}
	if e.Msg != "exit status 3" {
		t.Errorf("el mensaje quedó en %q", e.Msg)
	}

	// Un error normal: Exit se queda en 0, que es el valor cero, y el mensaje es el
	// del error. Nada que inventar.
	normal := errors.New("exec: \"herdr\": executable file not found in $PATH")
	e = newError([]string{"herdr"}, normal, nil)
	if e.Exit != 0 {
		t.Errorf("con un error normal quedó Exit=%d, want 0: no hay código de salida que leer", e.Exit)
	}
	if e.Msg != normal.Error() {
		t.Errorf("el mensaje quedó en %q, want el del error: %q", e.Msg, normal.Error())
	}
	// Y el mensaje de un error normal es el que dice la verdad ("no está en el
	// PATH"), no un "exit status 0" que no explicaría nada.
	if strings.Contains(e.Msg, "exit status") {
		t.Errorf("un error que no es de proceso dio %q: se inventó un código de salida", e.Msg)
	}
}

// TestNewErrorSeQuedaConLaPrimeraLineaDeStderr: stderr puede traer un volcado
// entero, y lo que se muestra al usuario es una línea.
//
// La primera línea es la del error; lo de debajo suele ser el stack o el log de
// Herdr. Si se metiera todo, el aviso en la TUI sería un bloque que se come media
// pantalla y esconde lo que dice el error real.
func TestNewErrorSeQuedaConLaPrimeraLineaDeStderr(t *testing.T) {
	stderr := "error: no such workspace\ngoroutine 1 [running]:\n\therdr/main.go:42"
	e := newError([]string{"herdr"}, errors.New("exit status 1"), []byte(stderr))

	if e.Msg != "error: no such workspace" {
		t.Errorf("el mensaje quedó en %q, want solo la primera línea", e.Msg)
	}
	if strings.Contains(e.Msg, "goroutine") {
		t.Errorf("el mensaje se tragó el stack: %q", e.Msg)
	}
}

// TestElMensajeDeUnErrorNuncaQuedaVacio: sea cual sea el stderr, el mensaje se
// rellena. Un error sin texto se pinta como un fallo sin explicación, que es peor
// que no pintar el error.
func TestElMensajeDeUnErrorNuncaQuedaVacio(t *testing.T) {
	for _, stderr := range []string{
		"", "   ", "\n\n", "algo", "{}", `{"error":{}}`, `{"code":"","message":""}`,
	} {
		for _, err := range []error{errors.New("exit status 1"), exitErrorFor(t, 7)} {
			e := newError([]string{"herdr"}, err, []byte(stderr))
			if strings.TrimSpace(e.Msg) == "" {
				t.Errorf("stderr=%q err=%v: el mensaje quedó vacío", stderr, err)
			}
		}
	}
}
