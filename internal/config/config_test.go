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
	got := Defaults().Hints()
	want := []string{
		"q quit", "tab section", "r mount review",
		"a approve", "m merge ×2", "o open", "R refresh", "j/k move", "pgup/dn page",
	}
	if strings.Join(got, " ") != strings.Join(want, " ") {
		t.Fatalf("Hints() = %v, quiero %v", got, want)
	}
}

// TestHintsCubrenTodosLosKeybindings es el guard anti-drift: si se añade una
// acción a DefaultKeybindings() y no se mete en hintOrder, la barra deja de
// decir la verdad sobre qué teclas existen y nadie se entera hasta que alguien
// prueba la tecla y no pasa nada.
func TestHintsCubrenTodosLosKeybindings(t *testing.T) {
	bar := strings.Join(Defaults().Hints(), " ")
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
	bar := strings.Join(cfg.Hints(), " ")
	if !strings.Contains(bar, "b open") || !strings.Contains(bar, "v mount review") {
		t.Errorf("el rebind no llegó a la barra: %v", cfg.Hints())
	}
	if strings.Contains(bar, "o open") || strings.Contains(bar, "m mount review") {
		t.Errorf("la barra sigue mostrando la tecla anterior: %v", cfg.Hints())
	}
}

func TestDefaultKeybindingsCoverActions(t *testing.T) {
	kb := DefaultKeybindings()
	for _, action := range []string{"quit", "refresh", "mount-review", "approve", "merge", "section-next", "open-browser"} {
		if kb[action] == "" {
			t.Errorf("falta keybinding %q", action)
		}
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
