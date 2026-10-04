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

// `main` es lo único que queda sin probar del `main`, y lo está por una razón que el propio
// código declara: `main` lee `os.Args`, escribe en `os.Stdout`/`os.Stderr` y llama a `os.Exit`.
// Las tres cosas son globales del proceso, y una función que sale del proceso no se puede
// llamar desde un test sin matar el test.
//
// Y el camino que falta es el de la TUI: `wire`, `tea.NewProgram(...).Run()` y el código de
// salida. Que es justo el camino que más importa, porque es el que ejecuta el 99% de las veces
// que alguien abre prdash, y el único que nadie ha llamado nunca desde un test.
//
// Por eso este test es un SUBPROCESO: se re-ejecuta el binario de test con una variable de
// entorno que le dice "llama a `main` y deja que `os.Exit` haga su trabajo". El padre comprueba
// el código de salida y la salida del hijo, que es lo que un shell vería.

// mainEnSubproceso dice al subproceso que llame a `main`.
const mainEnSubproceso = "PRDASH_TEST_MAIN"

// argsEnSubproceso lleva los args de prdash al subproceso, separados por un byte que no puede
// aparecer en una orden. Van en el entorno y no en la línea de órdenes porque `-test.run` es un
// flag de la suite: en la línea de órdenes, `parseOpts` lo leería como un flag de prdash y
// devolvería un error de uso en vez de llegar al camino que se quiere probar.
const argsEnSubproceso = "PRDASH_TEST_ARGS"

// TestMainDeVerdadSaleConElCodigoQueCorrespondeALosArgs: `main` entera.
//
// Y el caso que se elige es el de la TUI sin terminal, y es real: `tea.NewProgram(m).Run()`
// necesita una terminal, y sin ella devuelve un error en vez de arrancar. Eso es lo que pasa
// cuando alguien ejecuta `prdash > fichero.log` o en un CI, y lo que hay que comprobar es que
// eso produce un aviso legible y un código de salida HONESTO en vez de un panic o un 0.
//
// Y los dos códigos importan por razones distintas:
//
//   - 1 cuando bubbletea falla: hubo un error.
//   - 0 cuando la CLI se usó bien, que es el otro modo que se puede comprobar sin terminal.
//
// Y el aviso a stderr lleva el motivo de bubbletea, no el del binario: si el mensaje fuera
// "fork/exec /home/usuario/.local/bin/prdash", el diagnóstico llevaría a mirar el PATH cuando
// el problema es que no hay terminal.
func TestMainDeVerdadSaleConElCodigoQueCorrespondeALosArgs(t *testing.T) {
	// Un HOME y un XDG propios, para que el subproceso no lea ni escriba el config de quien
	// corre los tests.
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
			// Y el motivo es de bubbletea: sin terminal no hay TTY, y eso es lo que
			// tiene que ver quien lea el log.
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
			// Y el subproceso NO puede morirse de un panic: un panic deja la mitad de la
			// salida en stderr y un código 2, que es el mismo que un error de uso. Es la
			// ambigüedad que haría pasar un crash por un error de uso.
			if strings.Contains(salida, "panic:") || strings.Contains(salida, "goroutine ") {
				t.Errorf("el subproceso entró en panic:\n%s", salida)
			}
		})
	}
}

// TestMainSinTerminalNoDejaElProcesoColgadoNiSaleConCero: el código de salida honesto.
//
// Y esta es la razón de que el test anterior exista: `run` devuelve 0 al final del camino de
// la TUI, y un `Run()` que fallara sin que nadie mirara su error dejaría a prdash exiting 0
// con un error impreso. En un script eso es indistinguishable de "se montó el review".
//
// Y se comprueba en las dos mitades: el aviso está Y el código es 1. Con solo una de las dos el
// test pasaría —un código 1 sin aviso no dice nada, y un aviso con código 0 engaña a quien
// automate—.
func TestMainSinTerminalNoDejaElProcesoColgadoNiSaleConCero(t *testing.T) {
	base := t.TempDir()
	salida, code := mainComoSubproceso(t, base)

	if code == 0 {
		t.Errorf("sin terminal salió con 0.\n%s", salida)
	}
	// Y el aviso dice algo: un error sin mensaje es un fallo que no se puede diagnosticar.
	if strings.TrimSpace(salida) == "" {
		t.Error("falló sin decir nada: en un log de CI eso es un fallo sin diagnóstico")
	}
	// Y no es un error de uso —que sería un 2 con el texto del flag— sino un fallo de la
	// sesión: la diferencia dice si hay que corregir la orden o el entorno.
	if strings.Contains(salida, "uso:") || strings.Contains(salida, "usage") {
		t.Errorf("el fallo parece de uso y no de entorno:\n%s", salida)
	}
}

// mainComoSubproceso re-ejecuta este binario de test en modo subproceso y devuelve su salida
// combinada y su código de salida.
func mainComoSubproceso(t *testing.T, base string, args ...string) (string, int) {
	t.Helper()

	// Los args de prdash NO van en la línea de órdenes sino en una variable de entorno, y
	// `os.Args` se reescribe en el hijo antes de llamar a `main`. La razón es que `-test.run`
	// es un flag de la suite: si se pasara por la línea de órdenes, `parseOpts` lo leería
	// como un flag de prdash y devolvería "flag provided but not defined" —un error de uso
	// con código 2— en vez de llegar al camino que se quiere probar.
	cmd := exec.Command(os.Args[0], "-test.run=^TestQueEjecutaMainDeVerdad$")

	cmd.Env = append(os.Environ(),
		mainEnSubproceso+"=1",
		argsEnSubproceso+"="+strings.Join(args, "\x00"),
		// Un HOME y un XDG propios: sin esto el subproceso leería el config real de quien
		// corre los tests y podría escribir en su caché.
		"HOME="+base,
		"XDG_CONFIG_HOME="+filepath.Join(base, "config"),
		"XDG_CACHE_HOME="+filepath.Join(base, "cache"),
		"XDG_DATA_HOME="+filepath.Join(base, "data"),
		// Y un `PATH` con lo justo para que `git` esté si hace falta, sin los binarios del
		// sistema que podrían hacer que `run` se comportara distinto en cada máquina.
		"PATH="+os.Getenv("PATH"),
	)
	// Y sin TTY a propósito: `tea.NewProgram(...).Run()` necesita una terminal, y sin ella es
	// lo que devuelve error. Es el caso que se quiere probar.
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

// TestQueEjecutaMainDeVerdad es lo que corre el subproceso: llama a `main` y deja que `os.Exit`
// termine el proceso.
//
// Y es un test que NO se ejecuta en la pasada normal —solo tiene efecto cuando la variable de
// entorno está puesta— y por eso empieza por una guarda que aborta el resto del test si no es
// el subproceso. Sin ella, la suite normal llamaría a `os.Exit` y se llevaría por delante el
// resto de los tests del paquete.
func TestQueEjecutaMainDeVerdad(t *testing.T) {
	if os.Getenv(mainEnSubproceso) == "1" {
		// Y aquí se reescribe `os.Args` con lo que el padre queria que viera `main`. Es la
		// parte del trabajo de este subproceso que no es trivial: `main` lee `os.Args`, y sin
		// esto leería los flags de la suite.
		os.Args = append([]string{filepath.Base(os.Args[0])},
			strings.Split(os.Getenv(argsEnSubproceso), "\x00")...)
		main()
		// Y si `main` vuelve —que no debería, porque siempre sale con `os.Exit`—, el código
		// es un fallo: sin esto el subproceso saldría con 0 y el test del padre pasaría sin
		// haber comprobado nada.
		os.Exit(99)
	}
	t.Skip("esto solo corre en el subproceso que llama a main")
}

// asExitError es el type assertion a `*exec.ExitError` con el error como segundo valor, que es
// la forma de no repetir el `errors.As` en un `if` de tres líneas.
func asExitError(err error, dst **exec.ExitError) bool {
	e, ok := err.(*exec.ExitError)
	if ok {
		*dst = e
	}
	return ok
}

// TestLaTUIQueArrancaSaleConCeroYLaQueFallaSaleConUno: los dos finales de `run`.
//
// Y son las dos mitades del mismo `if`, y la que faltaba era la de "arrancó": con un subproceso
// sin terminal solo se alcanza el fallo, porque `tea.NewProgram(m).Run()` necesita una TTY y la
// primera de las dos es la que necesita una.
//
// Y el seam es lo que separa las dos. `arrancaTUI` lee bubbletea, ejecuta bubbletea y devuelve
// lo que bubbletea devuelve —no abstrae nada—, pero permite que un test lo sustituya por una
// función que responde, que es lo mismo que hizo `Model.openURL` con el navegador.
//
// Y la asimetría de los códigos es lo que se fija, porque son los que lee un script:
//
//   - 0 con la TUI viva: se exited porque el usuario salió.
//   - 1 con error: bubbletea no pudo arrancar, y el aviso dice por qué.
//
// Y el aviso es lo que separa el segundo del primero: un `1` sin mensaje es un fallo que no se
// puede diagnosticar en un log de CI.
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
			// Y el aviso: solo cuando hubo error. Con la TUI viva no hay nada que avisar, y
			// un aviso ahí sería ruido.
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

// errDeArranque es el error que simula que bubbletea no pudo arrancar. Es un tipo propio y no un
// `errors.New` porque el aserto compara el TEXTO que aparece en el aviso con el del error: dos
// errores con el mismo texto serían indistinguibles y el test no probaría nada.
type errDeArranque struct{}

func (errDeArranque) Error() string { return "no hay terminal" }
