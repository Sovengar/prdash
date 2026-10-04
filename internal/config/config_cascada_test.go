package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Load never fails: that is the project rule. A TUI that does not start because the file has a typo is
//worse than one that starts with defaults.

func TestLoadSinFicheroNoDiceNada(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "no-existe.toml")

	cfg, warn := LoadFrom(path)
	if warn != "" {
		t.Errorf("sin fichero dio aviso %q: el primer arranque tiene que ser silencioso", warn)
	}
	if len(cfg.Keybindings) == 0 {
		t.Error("sin fichero devolvio una config sin keybindings en vez de los defaults")
	}
}

// The case that gets confused with the previous one, because the RESULT is the same —the defaults—.
func TestUnFicheroQueExisteYNoSeLeeAvisa(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("como root los permisos de lectura no impiden leer")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	if err := os.WriteFile(path, []byte("x = 1\n"), 0o000); err != nil {
		t.Fatal(err)
	}

	cfg, warn := LoadFrom(path)
	if warn == "" {
		t.Fatal("un fichero que existe y no se puede leer dio aviso vacio: el problema " +
			"pasa desapercibido")
	}
	if !strings.Contains(warn, "config") {
		t.Errorf("el aviso %q no dice de donde viene", warn)
	}
	if len(cfg.Keybindings) == 0 {
		t.Error("un config ilegible devolvio una config sin keybindings en vez de los defaults")
	}
}

// Having no variable set at all is a container environment.
func TestLoadPorLaRutaDeXDG(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)

	path, err := Path()
	if err != nil {
		t.Fatalf("Path: %v", err)
	}
	if filepath.Dir(path) != filepath.Join(dir, DirName) {
		t.Errorf("Path dio %q, want dentro de %q", path, filepath.Join(dir, DirName))
	}
	if filepath.Base(path) != FileName {
		t.Errorf("Path dio el fichero %q, want %q", filepath.Base(path), FileName)
	}

	if err := os.MkdirAll(filepath.Join(dir, DirName), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("[keybindings]\nquit = \"Q\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, warn := Load()
	if warn != "" {
		t.Errorf("Load aviso %q con un config valido", warn)
	}
	if got := cfg.KeyFor("quit"); got != "Q" {
		t.Errorf("Load no leyo el override de quit: %q", got)
	}

	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("HOME", "")
	cfg, _ = Load()
	if len(cfg.Keybindings) == 0 {
		t.Error("sin HOME ni XDG_CONFIG_HOME, Load devolvio una config sin keybindings")
	}
}

// The fallback is a REAL default, not an empty string.
func TestKeyForCaeAlDefaultYNoAlVacio(t *testing.T) {
	cfg := Defaults()

	for action := range DefaultKeybindings() {
		if got := cfg.KeyFor(action); got == "" {
			t.Errorf("la accion %q tiene tecla vacia con la config por defecto", action)
		}
	}
	cfg.Keybindings["quit"] = "Z"
	if got := cfg.KeyFor("quit"); got != "Z" {
		t.Errorf("con override, KeyFor dio %q, want Z", got)
	}
	if got := cfg.KeyFor("accion-inventada"); got != "" {
		t.Errorf("una accion desconocida dio tecla %q, want vacio", got)
	}
}

// Two actions on the same key, resolved deterministically.
func TestActionForKeyInvierteElMapaYEsEstable(t *testing.T) {
	cfg := Defaults()

	if got := cfg.ActionForKey(cfg.KeyFor("quit")); got != "quit" {
		t.Errorf("ActionForKey dio %q, want quit", got)
	}
	if got := cfg.ActionForKey(""); got != "" {
		t.Errorf("la tecla vacia dio accion %q, want vacio", got)
	}
	if got := cfg.ActionForKey("Ctrl+Alt+Imposible"); got != "" {
		t.Errorf("una tecla sin asignar dio accion %q, want vacio", got)
	}

	cfg.Keybindings["a"] = "X"
	cfg.Keybindings["b"] = "X"
	primera := cfg.ActionForKey("X")
	for i := 0; i < 100; i++ {
		if got := cfg.ActionForKey("X"); got != primera {
			t.Fatalf("ActionForKey dio %q en la llamada %d y %q en la primera", got, i, primera)
		}
	}
	if primera != "a" && primera != "b" {
		t.Errorf("ActionForKey dio %q, que no es ninguna de las dos acciones", primera)
	}
}

// strings.Fields, not Split(" ").
func TestCmdArgsParteElComandoPorEspacios(t *testing.T) {
	cfg := Defaults()

	cfg.Commands["agente"] = "  tuicr   review  "
	got := cfg.CmdArgs("agente")
	want := []string{"tuicr", "review"}
	if len(got) != len(want) {
		t.Fatalf("con espacios raros dio %q, want %q", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("argv[%d] = %q, want %q (completo %q)", i, got[i], want[i], got)
		}
	}
	for _, a := range got {
		if a == "" {
			t.Errorf("salió un argumento vacío en %q", got)
		}
	}

	// Pinned as empty, because an empty argv here is the signal of "no command for this action".
	if got := cfg.CmdArgs("accion-que-no-existe"); len(got) != 0 {
		t.Errorf("una accion sin comando dio %q, want argv vacio", got)
	}
	cfg.Commands["vacio"] = "   "
	if got := cfg.CmdArgs("vacio"); len(got) != 0 {
		t.Errorf("un comando de solo espacios dio %q", got)
	}
}

// That is the whole function: the difference between "the user did not configure it" and "the user
// configured it to nothing".
func TestElOverrideDelPaneNoEsUnValorVacio(t *testing.T) {
	cfg := Defaults()

	if _, ok := cfg.PaneOverride("hunk"); ok {
		t.Error("sin configurar dio override")
	}
	cfg.Commands["hunk"] = "hunk --model o3"
	argv, ok := cfg.PaneOverride("hunk")
	if !ok {
		t.Fatal("un comando configurado dio override=false")
	}
	if len(argv) != 3 || argv[0] != "hunk" || argv[2] != "o3" {
		t.Errorf("el override dio %q", argv)
	}
	for _, vacio := range []string{"", "   ", "\t\n "} {
		cfg.Commands["hunk"] = vacio
		if _, ok := cfg.PaneOverride("hunk"); ok {
			t.Errorf("un comando de %q dio override=true", vacio)
		}
	}
}

// The order is a decision: commands.<name> wins over tools.<name> over the default.
func TestToolArgsPriorizaElOverrideYLuegoElBinario(t *testing.T) {
	cfg := Defaults()

	// The defaults are NOT the tool's name: agent comes out as opencode and editor as vi.
	porDefecto := map[string]string{
		"tuicr": "tuicr", "hunk": "hunk", "agent": "opencode", "editor": "vi",
	}
	for name, want := range porDefecto {
		got := cfg.ToolArgs(name)
		if len(got) == 0 {
			t.Errorf("%q sin configurar dio argv vacio", name)
			continue
		}
		if got[0] != want {
			t.Errorf("%q sin configurar dio %q, want que empiece por %q", name, got, want)
		}
	}

	cfg.Tools.Tuicr = "/opt/tuicr --dark"
	got := cfg.ToolArgs("tuicr")
	if got[0] != "/opt/tuicr" || len(got) != 2 {
		t.Errorf("con tools.tuicr dio %q", got)
	}

	// `--model o3` is TWO fields, not one with a space in it, which is exactly what a naive single-arg
	//split gets wrong.
	cfg.Commands["tuicr"] = "tuicr review --model o3"
	got = cfg.ToolArgs("tuicr")
	want := []string{"tuicr", "review", "--model", "o3"}
	if len(got) != len(want) {
		t.Fatalf("con commands.tuicr dio %q, want %q", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("argv[%d] = %q, want %q (completo %q)", i, got[i], want[i], got)
		}
	}

	cfg.Commands["tuicr"] = "  "
	got = cfg.ToolArgs("tuicr")
	if got[0] != "/opt/tuicr" {
		t.Errorf("un commands.tuicr en blanco dio %q, y deberia seguir mandando tools", got)
	}
}

// The case separating the two branches of a naive `== ""` is whitespace.
func TestFieldsOrUsaElFallbackSoloSiEstaVacio(t *testing.T) {
	casos := []struct {
		raw      string
		fallback string
		want     string
	}{
		{"", "hunk", "hunk"},
		{"   ", "hunk", "hunk"},
		{"\t\n", "hunk", "hunk"},
		{"/opt/hunk", "hunk", "/opt/hunk"},
		{"/opt/hunk --dark", "hunk", "/opt/hunk"},
		{"con  espacios   dentro", "hunk", "con"},
	}
	for _, c := range casos {
		got := fieldsOr(c.raw, c.fallback)
		if len(got) == 0 {
			t.Errorf("fieldsOr(%q, %q) dio argv vacio", c.raw, c.fallback)
			continue
		}
		if got[0] != c.want {
			t.Errorf("fieldsOr(%q, %q) dio %q, want que empiece por %q", c.raw, c.fallback, got, c.want)
		}
	}
}
