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
	if cfg.Forges.GitLab.APIBase != "/git/api/v4/" {
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

func TestHintBarLines(t *testing.T) {
	lines := Defaults().HintBarLines()
	if len(lines) != 2 {
		t.Fatalf("líneas = %d, want 2", len(lines))
	}
	if !strings.Contains(lines[0], "j/k move") || !strings.Contains(lines[1], "refresh") {
		t.Errorf("hints = %v", lines)
	}
}

func TestDefaultKeybindingsCoverActions(t *testing.T) {
	kb := DefaultKeybindings()
	for _, action := range []string{"quit", "refresh", "detail", "mount-review", "approve", "merge", "section-next", "open-browser"} {
		if kb[action] == "" {
			t.Errorf("falta keybinding %q", action)
		}
	}
}

func TestActionForKey(t *testing.T) {
	cfg := Defaults()
	if got := cfg.ActionForKey("r"); got != "refresh" {
		t.Errorf("ActionForKey(r) = %q", got)
	}
	if got := cfg.ActionForKey("a"); got != "approve" {
		t.Errorf("ActionForKey(a) = %q", got)
	}
	if got := cfg.ActionForKey("M"); got != "merge" {
		t.Errorf("ActionForKey(M) = %q", got)
	}
	if got := cfg.ActionForKey("z"); got != "" {
		t.Errorf("ActionForKey(z) = %q, want vacío", got)
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
