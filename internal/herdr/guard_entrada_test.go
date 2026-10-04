package herdr

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Estas son las salidas del runner que no son de contenido sino de ENTRADA: un argv vacío, una
// salida que no es el JSON esperado, y un timeout.
//
// Y las dos primeras son las que se olvidan: el `empty argv` es un guard de una línea que no se
// dispara nunca con un plan real —el plan siempre trae comando—, y la salida ilegible es lo que
// pasa cuando Herdr cambia de versión o cuando otra cosa contesta en el socket.
//
// Y el timeout tiene una propiedad que solo se ve ejecutando de verdad, y es la que se
// comprueba aquí: con `Timeout` a cero se usa el valor por defecto, y ese camino no se puede
// tocar con un `execFn` porque el `execFn` se va por delante de todo.

// TestUnArgvVacioSeNiegaAntesDeLlamarAHerdr: el guard de `PaneRun`.
//
// Y es un guard que no se dispara nunca con un plan real —el plan siempre trae comando—, lo
// que lo hace perfecto para colarse. Y lo que protege es peor que un comando mal formado: un
// argv vacío se une en una sola línea de shell y sale como una línea EN BLANCO, que el shell
// del pane ejecuta como un Enter más. El usuario vería el pane aceptar la tecla y no pasaría
// nada, que es indistinguible de que tuicr no arrancó.
//
// Y el motivo tiene que decir "argv vacío" y no nada: sin él, el aviso de la TUI sería
// "could not run" sin más, y el usuario buscaría el problema en Herdr.
func TestUnArgvVacioSeNiegaAntesDeLlamarAHerdr(t *testing.T) {
	// Un Herdr que funciona, para que lo que falle sea el guard y no el entorno.
	f := &fakeCLI{env: map[string]string{"HERDR_ENV": "1"}}
	f.respond = func([]string) ([]byte, []byte, error) {
		return []byte(`{"id":"x","result":{"type":"ok"}}`), nil, nil
	}

	for _, argv := range [][]string{nil, {}} {
		err := f.client().PaneRun(context.Background(), "w1:p1", argv)
		if err == nil {
			t.Errorf("un argv %v dio nil: se mandaría una línea en blanco al shell del pane",
				argv)
			continue
		}
		if !strings.Contains(err.Error(), "argv") {
			t.Errorf("el error %q no dice que el argv está vacío", err)
		}
	}

	// Y el caso bueno, que es el control: con argv sí llega a Herdr.
	f2 := &fakeCLI{env: map[string]string{"HERDR_ENV": "1"}}
	f2.respond = func([]string) ([]byte, []byte, error) {
		return []byte(`{"id":"x","result":{"type":"ok"}}`), nil, nil
	}
	if err := f2.client().PaneRun(context.Background(), "w1:p1",
		[]string{"tuicr", "pr", "7"}); err != nil {
		t.Errorf("un argv con contenido dio error: %v", err)
	}
	if !f2.called("pane", "run") {
		t.Error("un argv con contenido no llegó a la CLI: el guard no es lo que lo paró")
	}
	// Y el argv viaja como UNA sola cadena, no partido en varios argumentos.
	//
	// Y aquí hay que decir quién cita los argumentos, porque `PaneRun` NO lo hace y parece que
	// sí: el `strings.Join(argv, " ")` no cita nada. El que cita es `paneCommand`, en el layout,
	// con `shellQuote`, y por eso `PaneRun` no puede citar sin duplicar las comillas de lo que
	// llega. Mi primera versión daba por hecho que `PaneRun` citaba y por eso buscaba comillas
	// ahí; la cita está probada en `shellquote_test.go` y en `layout_degrada_test.go`.
	//
	// Lo que se comprueba aquí es el transporte: lo que `PaneRun` manda a la CLI es una única
	// cadena con el argv entero dentro, porque Herdr la mete en el shell del pane tal cual y
	// un argv partido sería un comando distinto.
	argv := []string{"opencode", "run", "con espacios y comillas"}
	f3 := &fakeCLI{env: map[string]string{"HERDR_ENV": "1"}}
	f3.respond = func(args []string) ([]byte, []byte, error) {
		// La primera llamada es `--version`, que sale del `guard` al preguntar por
		// `Available()`. Solo se mira la de `pane run`: comprobar el argv en la de la versión
		// da un falso negativo, porque ahí no hay argv que mirar.
		if len(args) == 0 || args[0] == "--version" {
			return []byte(`{"id":"x","result":{"type":"ok"}}`), nil, nil
		}
		unido := strings.Join(argv, " ")
		found := false
		for _, a := range args {
			if a == unido {
				found = true
			}
		}
		if !found {
			t.Errorf("el argv llegó partido en %v: se habría perdido la frase con espacios", args)
		}
		return []byte(`{"id":"x","result":{"type":"ok"}}`), nil, nil
	}
	if err := f3.client().PaneRun(context.Background(), "w1:p1", argv); err != nil {
		t.Errorf("el argv con espacios dio error: %v", err)
	}
}

// TestUnaSalidaQueNoEsElJSONEsperadoNoSeConvierteEnUnaListaVacia: `parseWorktreeList`.
//
// Y es el fallo que se ve como "no hay worktrees" en vez de como "Herdr dijo otra cosa". Un
// `worktree list` que responde algo distinto —un aviso por stderr con código 0, una versión que
// cambió el campo— devuelve una lista VACÍA sin error, y el usuario ve que su review ya no
// está montado cuando en realidad no se pudo saber.
//
// Y el caso elegido es JSON que no parsea, no JSON válido con otra forma: `WorktreeList` ya
// propaga el error de la CLI, así que la línea que falta es la del parseo.
//
// Y hay un caso que NO da error y conviene tener en la cabeza: un JSON bien formado con la forma
// de otra respuesta —`{"result":{"type":"ok"}}`— devuelve una lista VACÍA sin error, porque el
// parseo solo lee los campos que conoce y los que faltan dan cero. Se ve igual que "no hay
// worktrees", y por eso los casos de esta tabla usan algo que no parsea: son los que
// distinguishes "no se pudo" de "no hay".
func TestUnaSalidaQueNoEsElJSONEsperadoNoSeConvierteEnUnaListaVacia(t *testing.T) {
	for _, c := range []struct {
		nombre string
		salida string
	}{
		{"html de un proxy", "<html>no</html>"},
		{"json truncado", `{"result":{"worktrees":[`},
		{"lista en vez de objeto", `[]`},
		{"vacío", ""},
	} {
		f := &fakeCLI{env: map[string]string{"HERDR_ENV": "1"}}
		f.respond = func([]string) ([]byte, []byte, error) {
			return []byte(c.salida), nil, nil
		}

		list, err := f.client().WorktreeList(context.Background(), "/repo")
		if err == nil {
			t.Errorf("%s: dio nil, y se pintaría como \"no hay worktrees\"", c.nombre)
			continue
		}
		if list != nil {
			t.Errorf("%s: devolvió %+v además del error", c.nombre, list)
		}
	}
}

// TestSinTimeoutSeUsaElValorPorDefectoYConTimeoutCortaDeVerdad: el `run` de verdad.
//
// Y aquí hay que ejecutar el binario de verdad, porque la rama del timeout por defecto está
// DESPUÉS del `execFn`, así que con el doble de siempre se salta entera.
//
// Y el motivo por el que hace falta el binario real es el del propio `WaitDelay`: matar el
// proceso no basta, porque un hijo que hereda los descriptores mantiene a `cmd.Run()` esperando.
// Con un script que abre un descriptor y se queda vivo, un timeout SIN `WaitDelay` se mide en
// segundos y no en milisegundos —medido: 5,00s con un timeout de 50ms—.
//
// Y el caso del `Timeout` a cero es el otro: sin él, un `Client` construido a mano se quedaría
// esperando el tiempo por defecto de `exec`, que es cero, es decir "para siempre".
func TestSinTimeoutSeUsaElValorPorDefectoYConTimeoutCortaDeVerdad(t *testing.T) {
	// Un script que imprime y se queda vivo con un descriptor abierto: es el caso que hace
	// que matar el proceso no baste.
	lento := filepath.Join(t.TempDir(), "herdr-lento")
	script := "#!/bin/sh\necho '{\"id\":\"x\",\"result\":{\"type\":\"ok\"}}'\n" +
		"sleep 30 &\nsleep 30\n"
	if err := os.WriteFile(lento, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}

	// Con timeout explícito y corto: corta, y corta RÁPIDO. El techo del aserto es amplio a
	// propósito —un techo corto convierte un CI lento en un test rojo que dice lo
	// contrario— y el suelo no: si no cortara, tardaría cinco segundos.
	conTimeout := &Client{
		Bin:     lento,
		Timeout: 50 * time.Millisecond,
		getenv:  func(k string) string { return "1" },
	}
	empieza := time.Now()
	_, _, err := conTimeout.run(context.Background(), "pane", "list")
	siTardó := time.Since(empieza)
	if err == nil {
		t.Error("un binario que se queda vivo dio nil con un timeout de 50ms")
	}
	if siTardó > 3*time.Second {
		t.Errorf("el timeout tardó %s en cortar: `WaitDelay` no está cortando de verdad y "+
			"el proceso se queda esperando a los hijos que heredaron los descriptores",
			siTardó.Round(time.Millisecond))
	}

	// Y con `Timeout` a CERO: se usa `DefaultTimeout`, que son 30s. Y eso se comprueba SIN
	// esperar 30 segundos, porque la prueba de que el default se aplica no necesita que el
	// proceso dure: necesita que `run` termine, y un script rápido termina con cualquier plazo.
	//
	// Mi primera versión reutilizó el script de 30 segundos aquí y el test tardó 30,31s en
	// total. El código hacía bien —el default es 30s— y el fixture estaba mal.
	rapido := filepath.Join(t.TempDir(), "herdr-rapido")
	if err := os.WriteFile(rapido,
		[]byte("#!/bin/sh\necho '{\"id\":\"x\",\"result\":{\"type\":\"ok\"}}'\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	sinTimeout := &Client{Bin: rapido, getenv: func(string) string { return "1" }}
	if sinTimeout.Timeout != 0 {
		t.Fatal("el fixture no sirve: el Client debería guardar el cero tal cual")
	}
	// El default se aplica DENTRO de `run`, y por eso no se puede comprobar mirando el campo:
	// si se aplicara en el constructor, el `Client` guardaría 30s.
	empiezaCero := time.Now()
	if _, _, err := sinTimeout.run(context.Background(), "pane", "list"); err != nil {
		t.Errorf("con Timeout a cero `run` falló: %v", err)
	}
	if siTardó := time.Since(empiezaCero); siTardó > 5*time.Second {
		t.Errorf("con Timeout a cero `run` tardó %s: el valor por defecto no se aplica y el "+
			"proceso se queda sin plazo", siTardó.Round(time.Millisecond))
	}
}

// TestUnBinarioEnBlancoSeBuscaEnElPathYNoDaUnNombreVacio: `run` sin `Bin`.
//
// Y el `Bin` vacío NO significa "ejecuta nada": significa "usa el nombre por defecto", y sin
// esta rama un `Client` construido a mano —que es lo que hace la suite y lo que haría un
// embedding— intentaría ejecutar la cadena vacía, que falla con un error que no dice nada.
//
// Y el nombre por defecto sale del PATH, así que el test pone un `herdr` de mentira AHÍ y
// comprueba que es el que se ejecuta. Con `Bin` puesto a mano el camino se saltaría entero,
// que es lo que hacía la primera versión de este test.
//
// Y el caso que hace que esto no sea trivial: si el PATH no tiene ningún `herdr`, el error tiene
// que seguir siendo legible —"no such file"— y no un fallo de execve con un nombre vacío.
func TestUnBinarioEnBlancoSeBuscaEnElPathYNoDaUnNombreVacio(t *testing.T) {
	// Un PATH con un `herdr` de mentira, que es el que el nombre por defecto tiene que encontrar.
	dir := t.TempDir()
	log := filepath.Join(dir, "args.log")
	guion := "#!/bin/sh\necho \"$@\" > " + log + "\necho 'herdr 0.9.1'\n"
	if err := os.WriteFile(filepath.Join(dir, "herdr"), []byte(guion), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)

	// Y `HERDR_BIN_PATH` tiene que quedar VACÍA, porque `defaultBin()` la consulta primero: con
	// la variable puesta de antes en el entorno del proceso —y en esta máquina hay un herdr de
	// verdad— el PATH no se miraría y el test mediría el binario real.
	t.Setenv("HERDR_BIN_PATH", "")

	c := &Client{getenv: func(string) string { return "1" }}
	if c.Bin != "" {
		t.Fatal("el fixture no sirve: el Client debería arrancar con el binario vacío")
	}
	out, _, err := c.run(context.Background(), "--version")
	if err != nil {
		t.Fatalf("sin Bin no se encontró el herdr del PATH: %v", err)
	}
	if !strings.Contains(string(out), "0.9.1") {
		t.Errorf("la salida es %q, no la del binario del PATH", out)
	}
	// Y la prueba directa de que el nombre por defecto se usó: el `herdr` del PATH registró la
	// llamada.
	registrado, err := os.ReadFile(log)
	if err != nil {
		t.Fatalf("el herdr del PATH no llegó a ejecutarse: %v", err)
	}
	if !strings.Contains(string(registrado), "--version") {
		t.Errorf("el herdr del PATH recibió %q", registrado)
	}

	// Y sin nada en el PATH: el error tiene que seguir siendo legible, que es la mitad de lo
	// que arregla el nombre por defecto.
	vacio := t.TempDir()
	t.Setenv("PATH", vacio)
	t.Setenv("HERDR_BIN_PATH", "")
	otro := &Client{getenv: func(string) string { return "1" }}
	if _, _, err := otro.run(context.Background(), "--version"); err == nil {
		t.Error("sin herdr en el PATH dio nil")
	}
	// El error nombra el binario que se buscó: "exec" con un nombre vacío no le dice al
	// usuario que le falta el programa.
	if err != nil && !strings.Contains(err.Error(), "herdr") {
		t.Errorf("el error %q no nombra el binario que faltaba", err)
	}
}
