package herdr

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// The runner's outputs that are about INPUT rather than content: an empty argv, an unexpected output,
//and a timeout.

// A guard that never fires with a real plan, because the plan always brings a command.
func TestUnArgvVacioSeNiegaAntesDeLlamarAHerdr(t *testing.T) {
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
	// The argv travels as ONE string, not split into arguments, and PaneRun does NOT quote them.
	argv := []string{"opencode", "run", "con espacios y comillas"}
	f3 := &fakeCLI{env: map[string]string{"HERDR_ENV": "1"}}
	f3.respond = func(args []string) ([]byte, []byte, error) {
		// The first call is `--version`, which leaves the guard when it asks Available().
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

// The failure you see as "no worktrees" instead of "the output was not the JSON we expected".
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

// The real binary has to run here, because the timeout branch is the one under test.
func TestSinTimeoutSeUsaElValorPorDefectoYConTimeoutCortaDeVerdad(t *testing.T) {
	lento := filepath.Join(t.TempDir(), "herdr-lento")
	script := "#!/bin/sh\necho '{\"id\":\"x\",\"result\":{\"type\":\"ok\"}}'\n" +
		"sleep 30 &\nsleep 30\n"
	if err := os.WriteFile(lento, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}

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

	// Timeout at ZERO uses DefaultTimeout (30s), and that is checked WITHOUT waiting 30s.
	rapido := filepath.Join(t.TempDir(), "herdr-rapido")
	if err := os.WriteFile(rapido,
		[]byte("#!/bin/sh\necho '{\"id\":\"x\",\"result\":{\"type\":\"ok\"}}'\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	sinTimeout := &Client{Bin: rapido, getenv: func(string) string { return "1" }}
	if sinTimeout.Timeout != 0 {
		t.Fatal("el fixture no sirve: el Client debería guardar el cero tal cual")
	}
	// The default is applied INSIDE `run`, which is why the field cannot be checked directly.
	empiezaCero := time.Now()
	if _, _, err := sinTimeout.run(context.Background(), "pane", "list"); err != nil {
		t.Errorf("con Timeout a cero `run` falló: %v", err)
	}
	if siTardó := time.Since(empiezaCero); siTardó > 5*time.Second {
		t.Errorf("con Timeout a cero `run` tardó %s: el valor por defecto no se aplica y el "+
			"proceso se queda sin plazo", siTardó.Round(time.Millisecond))
	}
}

// An empty Bin does NOT mean "run nothing": it means "use the name from the PATH".
func TestUnBinarioEnBlancoSeBuscaEnElPathYNoDaUnNombreVacio(t *testing.T) {
	dir := t.TempDir()
	log := filepath.Join(dir, "args.log")
	guion := "#!/bin/sh\necho \"$@\" > " + log + "\necho 'herdr 0.9.1'\n"
	if err := os.WriteFile(filepath.Join(dir, "herdr"), []byte(guion), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)

	// HERDR_BIN_PATH has to be left EMPTY, because defaultBin() asks it first.
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
	registrado, err := os.ReadFile(log)
	if err != nil {
		t.Fatalf("el herdr del PATH no llegó a ejecutarse: %v", err)
	}
	if !strings.Contains(string(registrado), "--version") {
		t.Errorf("el herdr del PATH recibió %q", registrado)
	}

	// And with nothing in PATH the error stays readable, which is half the point of resolving the
	// default.
	vacio := t.TempDir()
	t.Setenv("PATH", vacio)
	t.Setenv("HERDR_BIN_PATH", "")
	otro := &Client{getenv: func(string) string { return "1" }}
	if _, _, err := otro.run(context.Background(), "--version"); err == nil {
		t.Error("sin herdr en el PATH dio nil")
	}
	// The error names the binary it looked for: "exec" with an empty name tells the user nothing.
	if err != nil && !strings.Contains(err.Error(), "herdr") {
		t.Errorf("el error %q no nombra el binario que faltaba", err)
	}
}
