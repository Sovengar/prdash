package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

const mainEnSubproceso = "PRDASH_TEST_MAIN"

// The args go in the ENVIRONMENT, not the command line, because -test.run would be read as a
// prdash flag.
const argsEnSubproceso = "PRDASH_TEST_ARGS"

func TestMainDeVerdadSaleConElCodigoQueCorrespondeALosArgs(t *testing.T) {
	// Its own HOME and XDG, so the subprocess does not read the config of whoever runs the tests.
	base := t.TempDir()

	for _, c := range []struct {
		nombre string
		args   []string
		code   int
		quiere string
	}{
		{
			nombre: "la TUI sin terminal avisa y sale con 1",
			args:   nil,
			code:   1,
			quiere: "prdash:",
		},
		{
			nombre: "el modo print sale con 0",
			args:   []string{"--print"},
			code:   0,
			quiere: "",
		},
		{
			nombre: "un flag desconocido es un error de uso",
			args:   []string{"--no-existe"},
			code:   2,
			quiere: "",
		},
	} {
		t.Run(c.nombre, func(t *testing.T) {
			salida, code := mainComoSubproceso(t, base, c.args...)

			if code != c.code {
				t.Errorf("el código de salida es %d, want %d.\n%s", code, c.code, salida)
			}
			if c.quiere != "" && !strings.Contains(salida, c.quiere) {
				t.Errorf("la salida no contiene %q:\n%s", c.quiere, salida)
			}
			// The subprocess must NOT die of a panic: that leaves half the output in stderr and the exit
			// code says nothing.
			if strings.Contains(salida, "panic:") || strings.Contains(salida, "goroutine ") {
				t.Errorf("el subproceso entró en panic:\n%s", salida)
			}
		})
	}
}

// This is why the previous test exists: run returns 0 at the end of the happy path.
func TestMainSinTerminalNoDejaElProcesoColgadoNiSaleConCero(t *testing.T) {
	base := t.TempDir()
	salida, code := mainComoSubproceso(t, base)

	if code == 0 {
		t.Errorf("sin terminal salió con 0.\n%s", salida)
	}
	// The warning says something: an error with no message is a failure that cannot be diagnosed.
	if strings.TrimSpace(salida) == "" {
		t.Error("falló sin decir nada: en un log de CI eso es un fallo sin diagnóstico")
	}
	// Not a usage error —that would be a 2 with the flag's text— but a session failure.
	if strings.Contains(salida, "uso:") || strings.Contains(salida, "usage") {
		t.Errorf("el fallo parece de uso y no de entorno:\n%s", salida)
	}
}

func mainComoSubproceso(t *testing.T, base string, args ...string) (string, int) {
	t.Helper()

	// prdash's args do NOT go on the command line; os.Args is rewritten in the child before main.
	cmd := exec.Command(os.Args[0], "-test.run=^TestQueEjecutaMainDeVerdad$")

	cmd.Env = append(os.Environ(),
		mainEnSubproceso+"=1",
		argsEnSubproceso+"="+strings.Join(args, "\x00"),
		"HOME="+base,
		"XDG_CONFIG_HOME="+filepath.Join(base, "config"),
		"XDG_CACHE_HOME="+filepath.Join(base, "cache"),
		"XDG_DATA_HOME="+filepath.Join(base, "data"),
		"PATH="+os.Getenv("PATH"),
	)
	cmd.Stdin = nil

	out, err := cmd.CombinedOutput()
	codigo := 0
	if err != nil {
		var salidaErr *exec.ExitError
		if ok := asExitError(err, &salidaErr); ok {
			codigo = salidaErr.ExitCode()
		} else {
			t.Fatalf("no se pudo ejecutar el subproceso: %v", err)
		}
	}
	return string(out), codigo
}

// It does NOT run in the normal pass.
func TestQueEjecutaMainDeVerdad(t *testing.T) {
	if os.Getenv(mainEnSubproceso) == "1" {
		os.Args = append([]string{filepath.Base(os.Args[0])},
			strings.Split(os.Getenv(argsEnSubproceso), "\x00")...)
		main()
		os.Exit(99)
	}
	t.Skip("esto solo corre en el subproceso que llama a main")
}

func asExitError(err error, dst **exec.ExitError) bool {
	e, ok := err.(*exec.ExitError)
	if ok {
		*dst = e
	}
	return ok
}

// Both halves of the same if, and the missing one was the "started" half: with a terminal-less
// subprocess only the failure was reachable.
func TestLaTUIQueArrancaSaleConCeroYLaQueFallaSaleConUno(t *testing.T) {
	xdg := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", xdg)
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())

	original := arrancaTUI
	t.Cleanup(func() { arrancaTUI = original })

	for _, c := range []struct {
		nombre string
		falla  error
		code   int
	}{
		{"la TUI arranca y el usuario sale", nil, 0},
		{"la TUI no puede arrancar", errDeArranque{}, 1},
	} {
		t.Run(c.nombre, func(t *testing.T) {
			arrancaTUI = func(tea.Model) error { return c.falla }

			var stdout, stderr bytes.Buffer
			code := run(nil, &stdout, &stderr)

			if code != c.code {
				t.Errorf("código = %d, want %d.\nstdout: %s\nstderr: %s",
					code, c.code, stdout.String(), stderr.String())
			}
			// The warning only when there was an error: with the TUI alive there is nothing to warn about.
			if c.falla == nil && strings.TrimSpace(stderr.String()) != "" {
				t.Errorf("con la TUI viva se avisó: %q", stderr.String())
			}
			if c.falla != nil {
				if !strings.Contains(stderr.String(), "prdash:") {
					t.Errorf("stderr = %q, want el aviso con el prefijo del programa", stderr.String())
				}
				if !strings.Contains(stderr.String(), c.falla.Error()) {
					t.Errorf("stderr = %q, no trae el motivo del fallo", stderr.String())
				}
			}
		})
	}
}

type errDeArranque struct{}

func (errDeArranque) Error() string { return "no hay terminal" }
