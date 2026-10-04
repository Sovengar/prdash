package herdr

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestRunSaleAlBinarioDeVerdad(t *testing.T) {
	dir := t.TempDir()
	log := filepath.Join(dir, "args.log")

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
	// stdout and stderr are NOT mixed: mixed, a CLI warning would look like a successful answer.
	if strings.Contains(string(out), "stderr") {
		t.Errorf("stderr se metio en stdout: %q", out)
	}

	crudo, err := os.ReadFile(log)
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.TrimSpace(string(crudo)); got != "pane list --json" {
		t.Errorf("al binario le llego %q, want %q", got, "pane list --json")
	}

	t.Setenv("FAKE_RC", "3")
	if _, _, err := c.run(context.Background(), "pane", "list"); err == nil {
		t.Error("un binario que sale con codigo 3 no dio error")
	}
}

func TestElBinarioPorDefectoSaleDelEntornoYNoDelCampo(t *testing.T) {
	t.Setenv("HERDR_BIN_PATH", "")
	if got := defaultBin(); got != "herdr" {
		t.Errorf("sin HERDR_BIN_PATH devolvio %q, want herdr", got)
	}

	t.Setenv("HERDR_BIN_PATH", "/opt/herdr/bin/herdr")
	if got := defaultBin(); got != "/opt/herdr/bin/herdr" {
		t.Errorf("con HERDR_BIN_PATH devolvio %q, want la ruta de la variable", got)
	}

	c := New()
	if c.Bin != "/opt/herdr/bin/herdr" {
		t.Errorf("New().Bin = %q, want la ruta de HERDR_BIN_PATH", c.Bin)
	}
	if c.getenv == nil {
		t.Error("New() dejo getenv a nil, con lo que env() cae a os.Getenv en vez de " +
			"usar el lector propio")
	}
}

// The ONLY read of HERDR_ENV in the whole program.
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

func TestLasOperacionesMutantesSeVetanFueraDeHerdr(t *testing.T) {
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
	// Counting carefully is the trap: guard calls Available(), which queries the version, and THAT is
	// a call to the binary.
	if len(vistos) != 2 {
		t.Errorf("dentro de Herdr se hicieron %d llamadas, want 2 (version + operacion): "+
			"%v", len(vistos), vistos)
	}
	if len(vistos) == 2 && !strings.Contains(strings.Join(vistos[1], " "), "focus") {
		t.Errorf("la segunda llamada fue %v y no es la operación", vistos[1])
	}
}

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

	vistos = nil
	if err := nuevo().PaneFocus(context.Background(), "up"); err != nil {
		t.Fatalf("pane focus con dirección: %v", err)
	}
	if !contieneArg(vistos, "up") {
		t.Errorf("con dirección up se mandó %v y ninguna lleva up", vistos)
	}
}

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
	// The default floor applies when Timeout is empty, and it is proved by checking what run computes
	//rather than by waiting 30s.
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

// contieneArg looks through ALL the calls, because the guard's first one is `--version`.
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

func escribirBinario(t *testing.T, dir, nombre, cuerpo string) string {
	t.Helper()
	ruta := filepath.Join(dir, nombre)
	if err := os.WriteFile(ruta, []byte(cuerpo), 0o755); err != nil {
		t.Fatalf("escribir %s: %v", ruta, err)
	}
	return ruta
}
