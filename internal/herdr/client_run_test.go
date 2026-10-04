package herdr

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestRunSaleAlBinarioDeVerdad: `run` ejecuta el binario, captura stdout y stderr por
// separado, y devuelve el fallo.
//
// Y este es el agujero que mas cubre de golpe, porque TODOS los demas tests del paquete
// pasan por `execFn` y `run` es justo la rama que esquivan. Lo que se esta probando aqui es
// la concha: el timeout, el binario por defecto, el entorno que se hereda y que stdout y
// stderr no se mezclen.
//
// Y no se puede cambiar `execFn` desde fuera del paquete, asi que un test externo a este
// paquete no llegaria aqui nunca. Es una de las razones por las que los tests de este
// fichero son `package herdr` y no `package herdr_test`.
func TestRunSaleAlBinarioDeVerdad(t *testing.T) {
	dir := t.TempDir()
	log := filepath.Join(dir, "args.log")

	// Un binario falso que escribe lo que se le pasó, dice otra cosa por stderr, y
	// sale con el código que se le indique.
	script := "#!/bin/sh\n" +
		"echo \"$@\" >> " + log + "\n" +
		"echo 'esto es stdout'\n" +
		"echo 'esto es stderr' >&2\n" +
		"exit ${FAKE_RC:-0}\n"
	bin := escribirBinario(t, dir, "herdr-falso", script)

	c := &Client{Bin: bin, getenv: func(string) string { return "1" }}

	out, errb, err := c.run(context.Background(), "pane", "list", "--json")
	if err != nil {
		t.Fatalf("run devolvio error: %v", err)
	}
	if !strings.Contains(string(out), "esto es stdout") {
		t.Errorf("stdout salio %q", out)
	}
	if !strings.Contains(string(errb), "esto es stderr") {
		t.Errorf("stderr salio %q", errb)
	}
	// Y stdout y stderr NO se mezclan: si se mezclaran, un aviso de la CLI aparecería
	// como si fuera contenido, y al parsear el JSON sería un error de formato en vez de
	// el aviso que es.
	if strings.Contains(string(out), "stderr") {
		t.Errorf("stderr se metio en stdout: %q", out)
	}

	// Los argumentos llegan enteros y en orden, que es lo que forma el argv.
	crudo, err := os.ReadFile(log)
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.TrimSpace(string(crudo)); got != "pane list --json" {
		t.Errorf("al binario le llego %q, want %q", got, "pane list --json")
	}

	// Y el codigo de salida distinto de cero es un error, con stderr en el mensaje.
	t.Setenv("FAKE_RC", "3")
	if _, _, err := c.run(context.Background(), "pane", "list"); err == nil {
		t.Error("un binario que sale con codigo 3 no dio error")
	}
}

// TestElBinarioPorDefectoSaleDelEntornoYNoDelCampo: sin `Bin` puesto, `run` usa
// HERDR_BIN_PATH, y sin esa variable usa el nombre canonico.
//
// Y la via canonica es `HERDR_BIN_PATH` y no "herdr" a secas, y el motivo esta en el
// comentario del codigo: apunta al binario REALMENTE en ejecucion, con lo que el socket y
// los pipes son los correctos. Un `herdr` del PATH puede ser otra version, y con ella el
// socket no es el de esta sesion — que es el fallo que hace que Herdr parezca roto sin
// estarlo.
func TestElBinarioPorDefectoSaleDelEntornoYNoDelCampo(t *testing.T) {
	// Sin nada puesto: "herdr".
	t.Setenv("HERDR_BIN_PATH", "")
	if got := defaultBin(); got != "herdr" {
		t.Errorf("sin HERDR_BIN_PATH devolvio %q, want herdr", got)
	}

	// Con la variable: la variable, y no un nombre fijo.
	t.Setenv("HERDR_BIN_PATH", "/opt/herdr/bin/herdr")
	if got := defaultBin(); got != "/opt/herdr/bin/herdr" {
		t.Errorf("con HERDR_BIN_PATH devolvio %q, want la ruta de la variable", got)
	}

	// Y `New` sin binario toma ese mismo.
	c := New()
	if c.Bin != "/opt/herdr/bin/herdr" {
		t.Errorf("New().Bin = %q, want la ruta de HERDR_BIN_PATH", c.Bin)
	}
	if c.getenv == nil {
		t.Error("New() dejo getenv a nil, con lo que env() cae a os.Getenv en vez de " +
			"usar el lector propio")
	}
}

// TestInHerdrSoloMiraLaVariable: dentro de Herdr es HERDR_ENV=1, y cualquier otra cosa es
// que no.
//
// Y esta es la UNICA lectura de HERDR_ENV en todo el programa, que es lo que hace que la
// degradación sea coherente: si dos sitios la leyeran distinta, uno diría que estamos
// dentro y otro que no, y la operación se intentaría a medias.
//
// Y el caso que no se puede confundir: `"true"`, `"yes"` y `"0"` NO son estar dentro. Un
// bool.Parse o un chequeo de presencia los tomarían por verdadero, y el resultado sería
// intentar montar un review fuera de Herdr.
func TestInHerdrSoloMiraLaVariable(t *testing.T) {
	casos := []struct {
		valor string
		want  bool
		nota  string
	}{
		{"1", true, "el valor exacto"},
		{"", false, "sin variable"},
		{"0", false, "cero no es uno"},
		{"true", false, `"true" no es "1": es el caso que un Parse booleano se tragaria`},
		{"yes", false, "lo mismo con yes"},
		{"11", false, "empieza por uno pero no es"},
		{" 1", false, "con espacio delante no es"},
	}
	for _, c := range casos {
		t.Setenv("HERDR_ENV", c.valor)
		if got := InHerdr(); got != c.want {
			t.Errorf("HERDR_ENV=%q dio %v, want %v. %s", c.valor, got, c.want, c.nota)
		}
	}
}

// TestLasOperacionesMutantesSeVetanFueraDeHerdr: worktree remove, workspace close y pane
// focus no tocan la sesion si Herdr no esta disponible.
//
// Y esto no es un detalle defensivo: estas tres son las que MODIFICAN estado. Un worktree
// eliminado de mas es trabajo perdido del usuario, y un workspace cerrado con un review
// dentro deja el review sin sitio.
//
// El veto es el `guard`, que mira disponibilidad antes de salir. Y las lecturas —list y
// version— NO pasan por ahi, que es la diferencia: una lectura que falla no destruye nada,
// y vetarla solo haria que la app pareciera no tener nada que mostrar.
func TestLasOperacionesMutantesSeVetanFueraDeHerdr(t *testing.T) {
	// Fuera de Herdr: vetadas, y sin tocar el binario.
	llamadas := 0
	vistos := [][]string{}
	c := &Client{
		Bin:    "no-debe-ejecutarse",
		getenv: func(k string) string { return "" },
		execFn: func(context.Context, ...string) ([]byte, []byte, error) {
			llamadas++
			return nil, nil, nil
		},
	}

	if err := c.WorktreeRemove(context.Background(), "w1", true); err == nil {
		t.Error("worktree remove fuera de Herdr no dio error")
	}
	if err := c.WorkspaceClose(context.Background(), "w1", false); err == nil {
		t.Error("workspace close fuera de Herdr no dio error")
	}
	if err := c.PaneFocus(context.Background(), "left"); err == nil {
		t.Error("pane focus fuera de Herdr no dio error")
	}
	if llamadas != 0 {
		t.Errorf("se ejecutaron %d llamadas al binario estando fuera de Herdr: el veto "+
			"tiene que ir ANTES de salir a la CLI, no despues", llamadas)
	}

	// Y dentro de Herdr, con versión bastante, pasan. Sin esto el test de arriba
	// probaría que todo está vetado siempre, que es un fallo distinto.
	dentro := &Client{
		Bin: "herdr",
		getenv: func(k string) string {
			if k == "HERDR_ENV" {
				return "1"
			}
			return ""
		},
		execFn: func(_ context.Context, args ...string) ([]byte, []byte, error) {
			vistos = append(vistos, args)
			return []byte(`{"id":"cli:ok","result":{"type":"ok"}}`), nil, nil
		},
	}
	if err := dentro.PaneFocus(context.Background(), "left"); err != nil {
		t.Errorf("dentro de Herdr, pane focus falló: %v", err)
	}
	// Y aquí hay que contar BIEN, que es la trampa del test: `guard` llama a
	// `Available()`, que consulta la versión, y esa consulta ES una llamada al binario.
	// La primera versión de este test contaba una y veía dos, y la conclusión fácil —
	// «el guard llama dos veces»— era falsa: una es `--version` y la otra la operación.
	if len(vistos) != 2 {
		t.Errorf("dentro de Herdr se hicieron %d llamadas, want 2 (version + operacion): "+
			"%v", len(vistos), vistos)
	}
	if len(vistos) == 2 && !strings.Contains(strings.Join(vistos[1], " "), "focus") {
		t.Errorf("la segunda llamada fue %v y no es la operación", vistos[1])
	}
}

// TestPaneFocusSinDireccionVaADerecha: sin dirección, el foco va a la derecha, que es la
// lectura de un diff junto a lo que lo comenta.
//
// Y es el mismo suelo que la dirección de división del layout: lo que no se dice es "a la
// derecha", porque derecha es lo que se espera y lo que se lee. Un foco a la izquierda
// por defecto pondría el diff donde estaba la conversación.
func TestPaneFocusSinDireccionVaADerecha(t *testing.T) {
	var vistos [][]string
	nuevo := func() *Client {
		return &Client{
			Bin:    "herdr",
			getenv: func(k string) string { return "1" },
			execFn: func(_ context.Context, args ...string) ([]byte, []byte, error) {
				vistos = append(vistos, args)
				return []byte(`{"id":"cli:ok","result":{"type":"ok"}}`), nil, nil
			},
		}
	}

	if err := nuevo().PaneFocus(context.Background(), ""); err != nil {
		t.Fatalf("pane focus sin dirección: %v", err)
	}
	if !contieneArg(vistos, "right") {
		t.Errorf("sin dirección se mandó %v y ninguna lleva right", vistos)
	}

	// Y con dirección explícita, esa se respeta.
	vistos = nil
	if err := nuevo().PaneFocus(context.Background(), "up"); err != nil {
		t.Fatalf("pane focus con dirección: %v", err)
	}
	if !contieneArg(vistos, "up") {
		t.Errorf("con dirección up se mandó %v y ninguna lleva up", vistos)
	}
}

// TestElTimeoutDeRunSeAplica: la llamada a la CLI no se queda colgada para siempre.
//
// Y esto importa por lo que hay detrás: si `run` no tuviera timeout y el binario se
// colgara —un Herdr con el socket atascado, un pane que no responde— el bubbletea loop se
// quedaría esperando ese mensaje y la TUI dejaría de responder al teclado, sin que se ve
// por qué.
//
// El test usa `sleep`, que es de los pocos comandos que están en cualquier sitio, y mide
// que el contexto corta antes que el proceso. Con un timeout de 50ms y un binario que
// duerme un segundo, la diferencia entre "cortó" y "no cortó" es de un orden de magnitud y
// no cabe en el margen del reloj.
func TestElTimeoutDeRunSeAplica(t *testing.T) {
	dir := t.TempDir()
	bin := escribirBinario(t, dir, "herdr-colgado", "#!/bin/sh\nsleep 5\n")

	c := &Client{Bin: bin, getenv: func(string) string { return "1" }, Timeout: 50 * time.Millisecond}

	inicio := time.Now()
	_, _, err := c.run(context.Background(), "pane", "list")
	elapsed := time.Since(inicio)

	if err == nil {
		t.Error("un binario colgado no dio error")
	}
	if elapsed > 2*time.Second {
		t.Errorf("run tardó %s con un timeout de 50ms: el contexto no está cortando la "+
			"llamada", elapsed)
	}
	// Y el suelo por defecto se aplica cuando el campo Timeout viene vacío. La forma de
	// probarlo NO es esperar a los 30 s de DefaultTimeout, sino comprobar lo contrario:
	// que con Timeout a cero la llamada NO se corta a los 50 ms del caso anterior.
	//
	// Y por eso el assert va en la dirección contraria a la del primer caso. Es tentador
	// escribir "tarda menos de 2 s" y sería falso: con el suelo de 30 s y un binario que
	// duerme 1 s, lo correcto es que tarde 1 s. Un timeout de 50 ms no se parece en nada a
	// un suelo de 30 s, así que el margen entre 1 s y 2 s separa las dos implementaciones
	// sin depender de la precisión del reloj.
	bin1s := escribirBinario(t, dir, "herdr-corto", "#!/bin/sh\nsleep 1\n")
	c.Bin = bin1s
	c.Timeout = 0
	inicio = time.Now()
	if _, _, err := c.run(context.Background(), "pane", "list"); err != nil {
		t.Errorf("con Timeout a cero y un binario que duerme 1s dio error: %v", err)
	}
	if elapsed := time.Since(inicio); elapsed < 900*time.Millisecond {
		t.Errorf("con Timeout a cero run tardó %s: se aplicó el timeout de 50ms del caso "+
			"anterior en vez del suelo por defecto", elapsed)
	}
}

// contieneArg mira entre TODAS las llamadas, porque la primera que hace el guard es la de
// `--version` y no lleva el argumento que se está mirando.
func contieneArg(llamadas [][]string, arg string) bool {
	for _, c := range llamadas {
		for _, a := range c {
			if a == arg {
				return true
			}
		}
	}
	return false
}

// escribirBinario crea un script ejecutable y devuelve su ruta.
func escribirBinario(t *testing.T, dir, nombre, cuerpo string) string {
	t.Helper()
	ruta := filepath.Join(dir, nombre)
	if err := os.WriteFile(ruta, []byte(cuerpo), 0o755); err != nil {
		t.Fatalf("escribir %s: %v", ruta, err)
	}
	return ruta
}
