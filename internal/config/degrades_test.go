package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// LoadFrom's contract is not "load the config" but DEGRADE: anything that fails returns the
//defaults.

func TestLoadFromDegradesToDefaultsAndWarns(t *testing.T) {
	dir := t.TempDir()

	asFolder := filepath.Join(dir, "config-folder")
	if err := os.MkdirAll(asFolder, 0o755); err != nil {
		t.Fatal(err)
	}

	noPermission := filepath.Join(dir, "config-no-permission")
	if err := os.WriteFile(noPermission, []byte("[general]\n"), 0o000); err != nil {
		t.Fatal(err)
	}
	if _, err := os.ReadFile(noPermission); err == nil {
		t.Skip("the user can read a file in mode 000: the EACCES case does not apply here")
	}

	selfLoop := filepath.Join(dir, "config-loop")
	if err := os.Symlink(selfLoop, selfLoop); err != nil {
		t.Fatalf("creating the link that points at itself: %v", err)
	}

	for _, c := range []struct {
		name string
		path string
	}{
		{"a directory instead of a file", asFolder},
		{"a file without read permission", noPermission},
		{"a link that points at itself", selfLoop},
	} {
		cfg, warn := LoadFrom(c.path)

		if warn == "" {
			t.Errorf("%s: LoadFrom degraded WITHOUT a warning", c.name)
			continue
		}
		if !strings.HasPrefix(warn, "config:") {
			t.Errorf("%s: the warning %q does not carry the config: prefix", c.name, warn)
		}
		def := Defaults()
		if !sameIgnoringWarning(cfg, def) {
			t.Errorf("%s: the config that came back is not the default one", c.name)
		}
	}
}

// The asymmetry with the above is deliberate and reads backwards from how it sounds: an absent
// config is the normal case and warns nothing, an unreadable one warns.
func TestAMissingConfigDegradesSilentlyAndAnEmptyOneDoesToo(t *testing.T) {
	dir := t.TempDir()

	cfg, warn := LoadFrom(filepath.Join(dir, "does-not-exist.toml"))
	if warn != "" {
		t.Errorf("a missing config gave warn %q: it would warn on every start", warn)
	}
	if !sameIgnoringWarning(cfg, Defaults()) {
		t.Error("a missing config did not return the defaults")
	}

	empty := filepath.Join(dir, "empty.toml")
	if err := os.WriteFile(empty, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, warn = LoadFrom(empty)
	if warn != "" {
		t.Errorf("an empty config gave warn %q", warn)
	}
	if !sameIgnoringWarning(cfg, Defaults()) {
		t.Error("an empty config did not return the defaults")
	}
}

// The most expensive of the four failures, which is why the important assertion is not the one
// about the error.
func TestATOMLThatDoesNotParseWarnsAndDoesNotKeepHalfOfWhatItRead(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "broken.toml")
	writeConfig(t, path, `
roots = ["/tmp/a-site-that-must-not-apply"]
data_dir = "/other"
refresh_interval = "10s"
this-is-not-a-key
`)

	cfg, warn := LoadFrom(path)

	if warn == "" {
		t.Fatal("a malformed TOML gave nil: the broken config would be applied silently")
	}
	if !strings.Contains(warn, "toml") && !strings.Contains(warn, "config:") {
		t.Errorf("the warning %q does not say it comes from the config", warn)
	}
	def := Defaults()
	if len(cfg.Roots) != len(def.Roots) || len(cfg.Roots) == 0 {
		t.Errorf("the config was left half-applied: %d roots, the defaults have %d",
			len(cfg.Roots), len(def.Roots))
	}
	for _, r := range cfg.Roots {
		if r == "/tmp/a-site-that-must-not-apply" {
			t.Error("a roots from the TOML that never finished parsing was applied")
		}
	}
	if cfg.DataDir == "/other" {
		t.Error("a data_dir from a TOML that never finished parsing was applied")
	}
}

// Per-field degradation is where "degrade with a warning" does NOT apply: an invalid
// `refresh_interval` is one bad key, not a broken file.
func TestAnInvalidValueIsIgnoredAndAValidOneIsApplied(t *testing.T) {
	dir := t.TempDir()
	def := Defaults()

	for _, c := range []struct {
		name    string
		toml    string
		want    time.Duration
		explain string
	}{
		{"unparseable duration", `refresh_interval = "quince minutos"`, def.RefreshInterval,
			"a badly written duration is ignored and the default wins"},
		{"negative duration", `refresh_interval = "-5s"`, def.RefreshInterval,
			"a negative interval is not applied: the tick would be requested in the past"},
		{"empty duration", `refresh_interval = ""`, def.RefreshInterval,
			"an empty duration is not a duration"},
		{"valid duration", `refresh_interval = "90s"`, 90 * time.Second,
			"a valid duration IS applied"},
		{"zero", `refresh_interval = "0s"`, 0,
			"zero IS applied: it is a manual refresh, and that is what the user asked for"},
	} {
		path := filepath.Join(dir, "c.toml")
		writeConfig(t, path, c.toml)

		cfg, warn := LoadFrom(path)
		if warn != "" {
			t.Errorf("%s: it gave warn %q, and a typo is not a broken config",
				c.name, warn)
		}
		if cfg.RefreshInterval != c.want {
			t.Errorf("%s: RefreshInterval = %v, want %v (%s)",
				c.name, cfg.RefreshInterval, c.want, c.explain)
		}
	}
}

// The guard exists because the code writes `fc.DataDir != nil && *fc.DataDir != ""`.
func TestAnEmptyStringInAPathIsIgnoredAndDoesNotLeaveItEmpty(t *testing.T) {
	dir := t.TempDir()
	def := Defaults()
	path := filepath.Join(dir, "c.toml")

	writeConfig(t, path, `
data_dir = ""
clone_dir = ""
worktree_dir = ""
`)
	cfg, warn := LoadFrom(path)
	if warn != "" {
		t.Errorf("it gave warn %q", warn)
	}
	if cfg.DataDir != def.DataDir {
		t.Errorf("DataDir = %q with an empty data_dir in the config, want the default %q",
			cfg.DataDir, def.DataDir)
	}
	if cfg.CloneDir != def.CloneDir {
		t.Errorf("CloneDir = %q with an empty clone_dir, want %q", cfg.CloneDir, def.CloneDir)
	}
	if cfg.WorktreeDir != def.WorktreeDir {
		t.Errorf("WorktreeDir = %q with an empty worktree_dir, want %q",
			cfg.WorktreeDir, def.WorktreeDir)
	}
	for name, value := range map[string]string{
		"DataDir": cfg.DataDir, "CloneDir": cfg.CloneDir, "WorktreeDir": cfg.WorktreeDir,
	} {
		if value == "" {
			t.Errorf("%s ended up empty: it would be a path relative to the working directory", name)
		}
	}
}

func TestAHintWithAnEmptyKeyIsNotPainted(t *testing.T) {
	cfg := Defaults()

	orphan := cfg.Hints(HintState{"nonexistent-action": "text"})
	if len(orphan) != len(cfg.Hints(nil)) {
		t.Errorf("an action with no key added %d entries to the bar",
			len(orphan)-len(cfg.Hints(nil)))
	}

	blank := Defaults()
	for _, action := range []string{"refresh", "approve", "merge", "quit"} {
		blank.Keybindings[action] = ""
	}
	for _, h := range blank.Hints(nil) {
		if strings.TrimSpace(h) == "" {
			t.Errorf("a hint with an empty key was painted as %q", h)
		}
		if strings.HasPrefix(h, " ") {
			t.Errorf("the hint %q starts with a gap: the key disappeared but the gap stayed", h)
		}
	}
}

func sameIgnoringWarning(a, b Config) bool {
	if len(a.Roots) != len(b.Roots) {
		return false
	}
	for i := range a.Roots {
		if a.Roots[i] != b.Roots[i] {
			return false
		}
	}
	if a.RefreshInterval != b.RefreshInterval || a.DataDir != b.DataDir ||
		a.CloneDir != b.CloneDir || a.WorktreeDir != b.WorktreeDir {
		return false
	}
	if a.Forges != b.Forges {
		return false
	}
	if len(a.Keybindings) != len(b.Keybindings) {
		return false
	}
	for k, v := range a.Keybindings {
		if b.Keybindings[k] != v {
			return false
		}
	}
	if len(a.Commands) != len(b.Commands) {
		return false
	}
	for k, v := range a.Commands {
		if b.Commands[k] != v {
			return false
		}
	}
	return len(a.Hints(nil)) == len(b.Hints(nil))
}
