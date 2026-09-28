package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestLoadFromMissingFileReturnsDefaultsSilently(t *testing.T) {
	cfg, warn := LoadFrom(filepath.Join(t.TempDir(), "nope.toml"))
	if warn != "" {
		t.Fatalf("warning = %q, want vacío", warn)
	}
	if cfg.RefreshInterval != 60*time.Second {
		t.Errorf("refresh = %v", cfg.RefreshInterval)
	}
	if !cfg.Forges.GitHub.Enabled || cfg.Forges.GitHub.Host != "github.com" {
		t.Errorf("github = %+v", cfg.Forges.GitHub)
	}
	if cfg.Forges.GitLab.APIBase != "/api/v4/" {
		t.Errorf("api_base = %q", cfg.Forges.GitLab.APIBase)
	}
	if cfg.Forges.Bitbucket.Enabled {
		t.Error("bitbucket debería venir deshabilitado")
	}
	if len(cfg.Roots) != 1 || !strings.HasSuffix(cfg.Roots[0], "dev") {
		t.Errorf("roots = %v", cfg.Roots)
	}
}

// escribirConfig deja un config.toml en path. Los tests de merge necesitan un
// fichero de verdad porque el parseo del TOML es parte de lo que se prueba: un
// mapa a mano saltaría justo la parte donde un valor mal formado se degrada.
func escribirConfig(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestLoadFromBrokenFileReturnsDefaultsWithWarning(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(path, []byte("roots = [unclosed"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, warn := LoadFrom(path)
	if warn == "" {
		t.Fatal("se esperaba warning por TOML roto")
	}
	if cfg.RefreshInterval != 60*time.Second {
		t.Errorf("debería caer a defaults, refresh = %v", cfg.RefreshInterval)
	}
}

func TestLoadFromMergesOverrides(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	content := `
roots = ["~/work", "/srv/code"]
refresh_interval = "30s"
data_dir = "~/data"

[forge.github]
enabled = false
host = "github.enterprise.com"

[forge.gitlab]
host = "gitlab.acme.io"
api_base = "/custom/api/v4/"

[tools]
agent = "claude"

[keybindings]
refresh = "R"

[commands]
gh = "gh --hostname github.enterprise.com"
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, warn := LoadFrom(path)
	if warn != "" {
		t.Fatalf("warning = %q", warn)
	}

	if cfg.RefreshInterval != 30*time.Second {
		t.Errorf("refresh = %v", cfg.RefreshInterval)
	}
	if len(cfg.Roots) != 2 || !strings.HasSuffix(cfg.Roots[0], "work") || cfg.Roots[1] != "/srv/code" {
		t.Errorf("roots = %v", cfg.Roots)
	}
	if cfg.Forges.GitHub.Enabled {
		t.Error("github debería quedar deshabilitado")
	}
	if cfg.Forges.GitHub.Host != "github.enterprise.com" {
		t.Errorf("github host = %q", cfg.Forges.GitHub.Host)
	}
	if cfg.Forges.GitLab.APIBase != "/custom/api/v4/" {
		t.Errorf("api_base = %q", cfg.Forges.GitLab.APIBase)
	}
	if cfg.Forges.GitLab.Host != "gitlab.acme.io" {
		t.Errorf("gitlab host = %q", cfg.Forges.GitLab.Host)
	}
	if cfg.Tools.Agent != "claude" {
		t.Errorf("agent = %q", cfg.Tools.Agent)
	}
	if cfg.Tools.Hunk != "hunk" {
		t.Errorf("hunk debería conservar su default, got %q", cfg.Tools.Hunk)
	}
	if cfg.KeyFor("refresh") != "R" {
		t.Errorf("keybinding refresh = %q", cfg.KeyFor("refresh"))
	}
	if got := cfg.CmdArgs("gh"); len(got) != 3 || got[0] != "gh" {
		t.Errorf("cmd gh = %v", got)
	}
}

func TestLoadFromPartialKeepsOtherDefaults(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(path, []byte(`refresh_interval = "0s"`), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, warn := LoadFrom(path)
	if warn != "" {
		t.Fatalf("warning = %q", warn)
	}
	if cfg.RefreshInterval != 0 {
		t.Errorf("refresh = %v, want 0 (solo manual)", cfg.RefreshInterval)
	}
	if !cfg.Forges.GitHub.Enabled {
		t.Error("github debería seguir habilitado por default")
	}
}

func TestGitLabClonePrefixDerivesFromAPIBase(t *testing.T) {
	cases := []struct {
		name      string
		apiBase   string
		cloneBase string
		want      string
	}{
		{"subcarpeta", "/git/api/v4/", "", "git"},
		{"raíz", "/api/v4/", "", ""},
		{"sin barra inicial", "git/api/v4/", "", "git"},
		{"vacío", "", "", ""},
		{"sin forma REST", "/custom/", "", ""},
		{"override gana", "/git/api/v4/", "/custom/", "custom"},
		{"override limpia a raíz", "/git/api/v4/", "/", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			g := GitLabConfig{APIBase: tc.apiBase, CloneBase: tc.cloneBase}
			if got := g.ClonePrefix(); got != tc.want {
				t.Fatalf("ClonePrefix = %q, quiero %q", got, tc.want)
			}
		})
	}
}

func TestGitHubClonePrefix(t *testing.T) {
	if got := (GitHubConfig{}).ClonePrefix(); got != "" {
		t.Fatalf("github raíz = %q", got)
	}
	if got := (GitHubConfig{CloneBase: "/ent/"}).ClonePrefix(); got != "ent" {
		t.Fatalf("github enterprise = %q", got)
	}
}

func TestLoadFromCloneBaseOverrides(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	content := `
[forge.github]
host = "github.enterprise.com"
clone_base = "/ent/"

[forge.gitlab]
host = "gitlab.acme.io"
clone_base = "repo"
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, warn := LoadFrom(path)
	if warn != "" {
		t.Fatalf("warning = %q", warn)
	}
	if got := cfg.Forges.GitHub.ClonePrefix(); got != "ent" {
		t.Errorf("github clone_base = %q", got)
	}
	if got := cfg.Forges.GitLab.ClonePrefix(); got != "repo" {
		t.Errorf("gitlab clone_base = %q", got)
	}
}

func TestDefaultsClonePrefix(t *testing.T) {
	// El default de APIBase es "/api/v4/" (GitLab estándar en la raíz): no
	// debe derivar prefijo, para no clonar mal un GitLab en raíz.
	if got := Defaults().Forges.GitLab.ClonePrefix(); got != "" {
		t.Fatalf("gitlab default ClonePrefix = %q, quiero vacío (raíz)", got)
	}
	if got := Defaults().Forges.GitHub.ClonePrefix(); got != "" {
		t.Fatalf("github default ClonePrefix = %q", got)
	}
}
func TestLoadFromInvalidDurationKeepsDefault(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(path, []byte(`refresh_interval = "nope"`), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, _ := LoadFrom(path)
	if cfg.RefreshInterval != 60*time.Second {
		t.Errorf("refresh = %v", cfg.RefreshInterval)
	}
}

func TestPathRespectsXDG(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	path, err := Path()
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(dir, DirName, FileName)
	if path != want {
		t.Fatalf("path = %q, want %q", path, want)
	}
}

func TestExpandAll(t *testing.T) {
	home, _ := os.UserHomeDir()
	got := expandAll([]string{"~/dev", "/abs", "~"})
	if got[0] != filepath.Join(home, "dev") {
		t.Errorf("got[0] = %q", got[0])
	}
	if got[1] != "/abs" || got[2] != "~" {
		t.Errorf("entradas sin ~/ no deben cambiar: %v", got)
	}
}

// TestLasRutasVaciasNoBorranLosDefaults: el patrón de merge del config trata
// "no puesto" y "puesto a vacío" como cosas distintas a propósito, y en las rutas
// esa diferencia se nota. Un puntero nil (la clave no está en el TOML) no toca
// nada. Un puntero a "" SÍ se aplica… salvo en las rutas, donde una cadena
// vacía no es "usa la ruta por defecto" sino "no hay ruta", que deja al resolver
// de repos y al de worktrees sin sitio donde escribir y rompe en runtime.
//
// Se afirma en los dos sentidos: nil conserva, "" conserva en las rutas. Lo
// segundo es una decisión de diseño, no un accidente, y por eso tiene test.
func TestLasRutasVaciasNoBorranLosDefaults(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	escribirConfig(t, path, `
data_dir = ""
clone_dir = ""
worktree_dir = ""
roots = [""]
`)
	cfg, warn := LoadFrom(path)
	if warn != "" {
		t.Fatalf("un config con rutas vacías no es un error: %s", warn)
	}
	def := Defaults()
	if cfg.DataDir != def.DataDir {
		t.Errorf("data_dir vacío no debería pisar el default: %q vs %q", cfg.DataDir, def.DataDir)
	}
	if cfg.CloneDir != def.CloneDir {
		t.Errorf("clone_dir vacío no debería pisar el default: %q vs %q", cfg.CloneDir, def.CloneDir)
	}
	if cfg.WorktreeDir != def.WorktreeDir {
		t.Errorf("worktree_dir vacío no debería pisar el default: %q vs %q", cfg.WorktreeDir, def.WorktreeDir)
	}
	// Y una ruta de verdad sí se aplica, para que el caso anterior no se lea
	// como "las rutas del config se ignoran siempre".
	otro := t.TempDir()
	path2 := filepath.Join(dir, "otro.toml")
	escribirConfig(t, path2, "data_dir = \""+otro+"\"\n")
	cfg2, _ := LoadFrom(path2)
	if cfg2.DataDir != otro {
		t.Errorf("data_dir con valor = %q, want %q", cfg2.DataDir, otro)
	}
}

// TestBitbucketEnabledSeAplicaSiendoElUnicoForgeDelBloque: los tres forges no se
// mergean igual. GitHub y GitLab tienen bloque propio, así que la condición
// es "el bloque está". Bitbucket solo tiene `enabled`, y la condición es doble
// (`bloque != nil && enabled != nil`) porque sin la segunda mitad un
// `[forge.bitbucket]` sin `enabled` pondría el forge a false sin que el usuario
// lo pidiera. Afirmar el `false` explícito es lo que distingue una cosa de la
// otra.
func TestBitbucketEnabledSeAplicaSiendoElUnicoForgeDelBloque(t *testing.T) {
	dir := t.TempDir()
	// Con `enabled = false` explícito: el forge se apaga.
	path := filepath.Join(dir, "off.toml")
	escribirConfig(t, path, "[forge.bitbucket]\nenabled = false\n")
	cfg, _ := LoadFrom(path)
	if cfg.Forges.Bitbucket.Enabled {
		t.Error("enabled = false debería apagar bitbucket")
	}

	// Con `enabled = true`: se enciende.
	path = filepath.Join(dir, "on.toml")
	escribirConfig(t, path, "[forge.bitbucket]\nenabled = true\n")
	cfg, _ = LoadFrom(path)
	if !cfg.Forges.Bitbucket.Enabled {
		t.Error("enabled = true debería encender bitbucket")
	}

	// Con el bloque pero sin `enabled`: el default se queda, que es false. Sin el
	// segundo `&&` de la condición, un bloque a secas desreferenciaría un puntero
	// nil y el arranque de prdash acabaría en un panic por config ausente.
	path = filepath.Join(dir, "bare.toml")
	escribirConfig(t, path, "[forge.bitbucket]\n")
	cfg, _ = LoadFrom(path)
	if cfg.Forges.Bitbucket.Enabled {
		t.Error("un bloque sin enabled no debería encender el forge")
	}
	// Y un bloque con otros campos tampoco: la condición es sobre `enabled`, no
	// sobre "hay bloque de bitbucket".
	path = filepath.Join(dir, "vacio.toml")
	escribirConfig(t, path, "[forge.bitbucket]\nclone_base = \"https://bitbucket.example.com\"\n")
	cfg, warn := LoadFrom(path)
	if warn != "" {
		t.Errorf("un bloque sin enabled no es un config roto: %s", warn)
	}
	if cfg.Forges.Bitbucket.Enabled {
		t.Error("un bloque con otros campos no debería encender el forge")
	}
}

// TestLosKeybindingsVaciosNoDesactivanAtajos: el merge de keybindings ignora el
// valor vacío a propósito. Un mapa de atajos se escribe porDelta —"quiero otra
// tecla"— y un valor vacío no significa "sin tecla" sino "no he cambiado esto":
// aceptarlo borraría el atajo por defecto de la acción, que es un item que
// disappears de la TUI sin que nadie lo haya pedido. Es la diferencia entre un
// config que degrada y uno que desactiva.
//
// Y un valor de verdad sí sobreescribe: si no, el filtro no haría nada.
func TestLosKeybindingsVaciosNoDesactivanAtajos(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "kb.toml")
	escribirConfig(t, path, `
[keybindings]
"quit" = ""
"merge" = "x"
`)
	cfg, warn := LoadFrom(path)
	if warn != "" {
		t.Fatalf("config roto: %s", warn)
	}
	def := DefaultKeybindings()
	// El vacío conserva el atajo por defecto de esa acción.
	if cfg.Keybindings["quit"] != def["quit"] {
		t.Errorf("quit = %q, want el default %q: un valor vacío no desactiva el atajo", cfg.Keybindings["quit"], def["quit"])
	}
	// El valor de verdad sobreescribe.
	if cfg.Keybindings["merge"] != "x" {
		t.Errorf("merge = %q, want \"x\"", cfg.Keybindings["merge"])
	}
	// Y ninguna acción se queda sin tecla.
	for accion, tecla := range cfg.Keybindings {
		if tecla == "" {
			t.Errorf("la acción %q quedó sin tecla: %v", accion, cfg.Keybindings)
		}
	}
}

// TestExpandSoloTocaElTilDEInicial: expand solo debe convertir un `~/` de
// verdad. Los casos que NO toca son la mitad del contrato y los que más
// duelen al romperse:
//
//   - "~" a secas es el home, no un prefijo: convertirlo daría home + "" y
//     una ruta que parece la del home pero no lo es.
//   - "~user/dev" es el home de OTRO usuario, que expand() no sabe resolver y
//     no debe tocar: tocarlo lo convertiría en un path del home actual con un
//     directorio "user" dentro, que es un path válido y por tanto silencioso.
//   - "~x" tampoco: la segunda letra tiene que ser un separador, o un
//     directorio que empieza por ~ se convierte sin querer.
//   - Una "~" al final o en medio no es prefijo: "a~b" y "/a~/b" son rutas
//     literales.
func TestExpandSoloTocaElTilDEInicial(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skipf("sin home: %v", err)
	}
	cases := map[string]string{
		"~/dev":     filepath.Join(home, "dev"),
		"~/":        filepath.Join(home),
		"~":         "~",         // a secas no se convierte
		"~user/dev": "~user/dev", // es el home de otro usuario: no se sabe
		"~x":        "~x",        // la segunda letra no es separador
		"a~b":       "a~b",       // la ~ no está al principio
		"/a~/b":     "/a~/b",     // idem, dentro de una ruta absoluta
		"~a/b":      "~a/b",      // segunda letra no separador
		"~a":        "~a",        // idem sin barra
		"":          "",          // vacío se deja
		"/abs":      "/abs",      // sin tilde
		"relative":  "relative",  // sin tilde
		"dev/":      "dev/",      // sin tilde
		"~~/dev":    "~~/dev",    // doble tilde: la segunda no es separador
	}
	for in, want := range cases {
		if got := expand(in); got != want {
			t.Errorf("expand(%q) = %q, want %q", in, got, want)
		}
	}
	// Un "~/" de dos caracteres es el caso mínimo que sí se convierte: el borde
	// de len(p) < 2.
	if got := expand("~/"); got != filepath.Join(home) {
		t.Errorf(`expand("~/") = %q, want %q`, got, filepath.Join(home))
	}
	// Y un solo carácter no puede ser prefijo: no hay segunda letra que mirar.
	if got := expand("~"); got != "~" {
		t.Errorf(`expand("~") = %q, want "~"`, got)
	}
}

func TestToolArgs(t *testing.T) {
	cfg := Defaults()
	if got := cfg.ToolArgs("agent"); len(got) != 1 || got[0] != "opencode" {
		t.Fatalf("agent default = %v", got)
	}
	if got := cfg.ToolArgs("tuicr"); len(got) != 1 || got[0] != "tuicr" {
		t.Fatalf("tuicr default = %v", got)
	}
	// `tools.<name>` define el binario; `commands.<name>` el argv completo.
	cfg.Tools.Agent = "claude"
	if got := cfg.ToolArgs("agent"); len(got) != 1 || got[0] != "claude" {
		t.Fatalf("agent tools = %v", got)
	}
	cfg.Commands["agent"] = "claude --model opus"
	if got := cfg.ToolArgs("agent"); len(got) != 3 || got[0] != "claude" || got[2] != "opus" {
		t.Fatalf("agent commands = %v", got)
	}
	if got := cfg.ToolArgs("desconocido"); got != nil {
		t.Fatalf("herramienta desconocida = %v", got)
	}
}

func TestPaneOverride(t *testing.T) {
	cfg := Defaults()
	if _, ok := cfg.PaneOverride("hunk"); ok {
		t.Fatal("hunk no debería traer override por defecto")
	}
	cfg.Commands["hunk"] = "hunk diff develop...HEAD --watch"
	got, ok := cfg.PaneOverride("hunk")
	if !ok {
		t.Fatal("hunk debería tener override")
	}
	want := []string{"hunk", "diff", "develop...HEAD", "--watch"}
	if strings.Join(got, " ") != strings.Join(want, " ") {
		t.Fatalf("override = %v, quiero %v", got, want)
	}
	cfg.Commands["agent"] = "   "
	if _, ok := cfg.PaneOverride("agent"); ok {
		t.Fatal("un valor en blanco no es override")
	}
}

// Un `[commands]` del fichero XDG llega verbatim a las tres claves de pane.
func TestLoadFromReadsPaneCommands(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	body := "[commands]\ntuicr = \"tuicr pr\"\nhunk = \"hunk diff main...HEAD\"\nagent = \"claude --model opus\"\n"
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, warn := LoadFrom(path)
	if warn != "" {
		t.Fatalf("warning = %q", warn)
	}
	for key, want := range map[string]string{
		"tuicr": "tuicr pr",
		"hunk":  "hunk diff main...HEAD",
		"agent": "claude --model opus",
	} {
		got, ok := cfg.PaneOverride(key)
		if !ok {
			t.Fatalf("%s debería tener override", key)
		}
		if strings.Join(got, " ") != want {
			t.Fatalf("%s = %q, quiero %q", key, strings.Join(got, " "), want)
		}
	}
}

// `[tools].editor` fija la orden del editor; `[commands].editor` la sustituye
// entera. El default es `vi` porque es un comando de shell (típicamente el que
// expande a `nvim .`), no un binario que prdash pueda localizar.
func TestEditorToolAndOverride(t *testing.T) {
	if got := strings.Join(Defaults().ToolArgs("editor"), " "); got != "vi" {
		t.Fatalf("editor por defecto = %q, quiero %q", got, "vi")
	}

	path := filepath.Join(t.TempDir(), "config.toml")
	body := "[tools]\neditor = \"nvim .\"\n[commands]\neditor = \"nvim README.md\"\n"
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, warn := LoadFrom(path)
	if warn != "" {
		t.Fatalf("warning = %q", warn)
	}
	if cfg.Tools.Editor != "nvim ." {
		t.Fatalf("tools.editor = %q", cfg.Tools.Editor)
	}
	got, ok := cfg.PaneOverride("editor")
	if !ok || strings.Join(got, " ") != "nvim README.md" {
		t.Fatalf("commands.editor = %v (override=%v)", got, ok)
	}
}

func TestHints(t *testing.T) {
	got := Defaults().Hints(nil)
	want := []string{
		"q quit", "tab section", "r mount review",
		"a approve", "m merge ×2", "v simulate", "o open", "R refresh",
		"p prefix", "e edit base", "j/k move", "pgup/dn page",
	}
	if strings.Join(got, " ") != strings.Join(want, " ") {
		t.Fatalf("Hints() = %v, quiero %v", got, want)
	}
}

// TestHintsConEstadoDinamico fija la costura por la que la barra nombra el modo
// de prefijo actual. Sin estado, la entrada sale como cualquier otra ("p
// prefix"); con estado, la etiqueta lo nombra. El fragmento lo pone quien tiene
// el estado —la TUI— pero lo une y decide el formato la config, que es quien
// sabe qué etiqueta tiene cada acción.
func TestHintsConEstadoDinamico(t *testing.T) {
	cfg := Defaults()
	bar := strings.Join(cfg.Hints(HintState{"prefix-mode": "full"}), " ")
	if !strings.Contains(bar, "p prefix: full") {
		t.Errorf("la barra no nombra el modo actual: %v", cfg.Hints(HintState{"prefix-mode": "full"}))
	}

	// Un fragmento de una acción que no está en la barra no puede inventar una
	// entrada nueva: hintOrder sigue siendo la única fuente de la lista.
	bar = strings.Join(cfg.Hints(HintState{"no-existe": "algo"}), " ")
	if strings.Contains(bar, "algo") {
		t.Errorf("un estado de una acción ausente añadió una entrada: %v", cfg.Hints(HintState{"no-existe": "algo"}))
	}

	// El rebind y el estado se combinan: la tecla sale de [keybindings] y el
	// nombre del modo, del estado de la vista.
	cfg.Keybindings["prefix-mode"] = "P"
	bar = strings.Join(cfg.Hints(HintState{"prefix-mode": "leaf"}), " ")
	if !strings.Contains(bar, "P prefix: leaf") {
		t.Errorf("el rebind no se combinó con el estado: %v", cfg.Hints(HintState{"prefix-mode": "leaf"}))
	}
	if strings.Contains(bar, "p prefix") {
		t.Errorf("la barra sigue mostrando la tecla anterior: %v", cfg.Hints(HintState{"prefix-mode": "leaf"}))
	}
}

// TestHintsCubrenTodosLosKeybindings es el guard anti-drift: si se añade una
// acción a DefaultKeybindings() y no se mete en hintOrder, la barra deja de
// decir la verdad sobre qué teclas existen y nadie se entera hasta que alguien
// prueba la tecla y no pasa nada.
func TestHintsCubrenTodosLosKeybindings(t *testing.T) {
	bar := strings.Join(Defaults().Hints(nil), " ")
	for action, key := range DefaultKeybindings() {
		if !strings.Contains(bar, key+" ") {
			t.Errorf("la acción %q (tecla %q) no sale en la barra de hints", action, key)
		}
	}
}

// TestHintsSiguenElRebind: la barra se deriva de [keybindings], no de una lista
// de teclas fija escrita a mano.
func TestHintsSiguenElRebind(t *testing.T) {
	cfg := Defaults()
	cfg.Keybindings["open-browser"] = "b"
	cfg.Keybindings["mount-review"] = "v"
	bar := strings.Join(cfg.Hints(nil), " ")
	if !strings.Contains(bar, "b open") || !strings.Contains(bar, "v mount review") {
		t.Errorf("el rebind no llegó a la barra: %v", cfg.Hints(nil))
	}
	if strings.Contains(bar, "o open") || strings.Contains(bar, "m mount review") {
		t.Errorf("la barra sigue mostrando la tecla anterior: %v", cfg.Hints(nil))
	}
}

func TestDefaultKeybindingsCoverActions(t *testing.T) {
	kb := DefaultKeybindings()
	for _, action := range []string{"quit", "refresh", "mount-review", "approve", "merge", "section-next", "open-browser", "prefix-mode", "retarget"} {
		if kb[action] == "" {
			t.Errorf("falta keybinding %q", action)
		}
	}
}

// TestDefaultKeybindingsNoSeRepiten vigila que dos acciones no compartan tecla. Es
// un fallo silencioso: `ActionForKey` resuelve por orden alfabético de acción, así
// que la colisión no rompe nada visible, solo deja una de las dos acciones
// imposible de alcanzar y sin que nada diga cuál.
//
// No lo había y lo pediu `retarget`: elegir `e` fue mirar la lista entera, y nada
// impedía que mañana otra acción elija la misma.
func TestDefaultKeybindingsNoSeRepiten(t *testing.T) {
	owner := map[string]string{}
	for action, key := range DefaultKeybindings() {
		if key == "" {
			continue
		}
		if otra, ok := owner[key]; ok {
			t.Errorf("las acciones %q y %q comparten la tecla %q", otra, action, key)
		}
		owner[key] = action
	}
}

func TestActionForKey(t *testing.T) {
	cfg := Defaults()
	if got := cfg.ActionForKey("R"); got != "refresh" {
		t.Errorf("ActionForKey(R) = %q", got)
	}
	if got := cfg.ActionForKey("r"); got != "mount-review" {
		t.Errorf("ActionForKey(r) = %q", got)
	}
	if got := cfg.ActionForKey("a"); got != "approve" {
		t.Errorf("ActionForKey(a) = %q", got)
	}
	if got := cfg.ActionForKey("m"); got != "merge" {
		t.Errorf("ActionForKey(m) = %q", got)
	}
	if got := cfg.ActionForKey("z"); got != "" {
		t.Errorf("ActionForKey(z) = %q, want vacío", got)
	}
}

// TestDefaultKeybindingsNoColisionan es el invariante que hace legible el
// esquema r=review / R=refresh / m=merge: si dos acciones comparten tecla,
// ActionForKey resuelve una por orden alfabético y la otra queda muerta sin que
// nada lo diga.
func TestDefaultKeybindingsNoColisionan(t *testing.T) {
	kb := DefaultKeybindings()
	owner := map[string]string{}
	for action, key := range kb {
		if other, dup := owner[key]; dup {
			t.Errorf("tecla %q compartida por %q y %q", key, other, action)
		}
		owner[key] = action
	}
}

func TestActionForKeyHonorsOverride(t *testing.T) {
	cfg := Defaults()
	cfg.Keybindings["refresh"] = "R"
	if got := cfg.ActionForKey("R"); got != "refresh" {
		t.Errorf("ActionForKey(R) = %q", got)
	}
	if got := cfg.ActionForKey("r"); got == "refresh" {
		t.Error("la tecla vieja no debería seguir mapeada tras el override")
	}
}
