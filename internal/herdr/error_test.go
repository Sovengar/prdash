package herdr

import (
	"errors"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"testing"
)

// A real process is spawned because *exec.ExitError only carries the code.
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

// Herdr's stderr JSON carries two things and both are used: a stable code and a message for humans.
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
		// Code only: the message comes from the exec error, not the server.
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
		// An explicitly empty message with a code present: the empty message must NOT overwrite the one
		// from the error.
		{
			"código con mensaje vacío",
			`{"error":{"code":"E_VACIO","message":""}}`,
			"E_VACIO", "exit status 3",
		},
		// JSON that is not Herdr's: no code and no message, so it is shown as the CLI printed it.
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
		// The message is NEVER empty: an error with no message says nothing.
		if e.Msg == "" {
			t.Errorf("%s: el mensaje quedó vacío: un error sin texto no informa de nada", c.nombre)
		}
		// The original error is kept, so the stack can be unwound.
		if !errors.Is(e.Err, err) {
			t.Errorf("%s: el error original no se guardó", c.nombre)
		}
		if strings.Join(e.Args, " ") != "herdr tab create" {
			t.Errorf("%s: los args quedaron en %q", c.nombre, e.Args)
		}
	}
}

// The exit code comes from the exec error and is only read if the error IS an exec one.
func TestNewErrorTomaElCodigoDeSalidaSoloSiLoTiene(t *testing.T) {
	e := newError([]string{"herdr"}, exitErrorFor(t, 3), nil)
	if e.Exit != 3 {
		t.Errorf("con un ExitError(3) quedó Exit=%d, want 3", e.Exit)
	}
	if e.Msg != "exit status 3" {
		t.Errorf("el mensaje quedó en %q", e.Msg)
	}

	normal := errors.New("exec: \"herdr\": executable file not found in $PATH")
	e = newError([]string{"herdr"}, normal, nil)
	if e.Exit != 0 {
		t.Errorf("con un error normal quedó Exit=%d, want 0: no hay código de salida que leer", e.Exit)
	}
	if e.Msg != normal.Error() {
		t.Errorf("el mensaje quedó en %q, want el del error: %q", e.Msg, normal.Error())
	}
	// The message of an ordinary error is the true one ("not in PATH"), not a fabricated text.
	if strings.Contains(e.Msg, "exit status") {
		t.Errorf("un error que no es de proceso dio %q: se inventó un código de salida", e.Msg)
	}
}

// The first line is shown, not the whole dump.
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
