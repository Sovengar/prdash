package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Load never fails: a TUI that does not start over a typo is worse than one with defaults.
func TestLoadWithoutAFileSaysNothing(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "does-not-exist.toml")

	cfg, warn := LoadFrom(path)
	if warn != "" {
		t.Errorf("without a file it gave warn %q: the first start has to be silent", warn)
	}
	if len(cfg.Keybindings) == 0 {
		t.Error("with no file it returned a config without keybindings instead of the defaults")
	}
}

func TestAFileThatExistsAndCannotBeReadWarns(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("as root read permissions do not prevent reading")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	if err := os.WriteFile(path, []byte("x = 1\n"), 0o000); err != nil {
		t.Fatal(err)
	}

	cfg, warn := LoadFrom(path)
	if warn == "" {
		t.Fatal("a file that exists and cannot be read gave an empty warning: the problem " +
			"goes unnoticed")
	}
	if !strings.Contains(warn, "config") {
		t.Errorf("the warning %q does not say where it comes from", warn)
	}
	if len(cfg.Keybindings) == 0 {
		t.Error("an unreadable config returned a config without keybindings instead of the defaults")
	}
}

func TestLoadThroughTheXDGPath(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)

	path, err := Path()
	if err != nil {
		t.Fatalf("Path: %v", err)
	}
	if filepath.Dir(path) != filepath.Join(dir, DirName) {
		t.Errorf("Path gave %q, want it inside %q", path, filepath.Join(dir, DirName))
	}
	if filepath.Base(path) != FileName {
		t.Errorf("Path gave the file %q, want %q", filepath.Base(path), FileName)
	}

	if err := os.MkdirAll(filepath.Join(dir, DirName), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("[keybindings]\nquit = \"Q\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, warn := Load()
	if warn != "" {
		t.Errorf("Load warned %q on a valid config", warn)
	}
	if got := cfg.KeyFor("quit"); got != "Q" {
		t.Errorf("Load did not read the quit override: %q", got)
	}

	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("HOME", "")
	cfg, _ = Load()
	if len(cfg.Keybindings) == 0 {
		t.Error("without HOME nor XDG_CONFIG_HOME, Load returned a config without keybindings")
	}
}

func TestKeyForFallsBackToTheDefaultAndNotToEmpty(t *testing.T) {
	cfg := Defaults()

	for action := range DefaultKeybindings() {
		if got := cfg.KeyFor(action); got == "" {
			t.Errorf("action %q has an empty key with the default config", action)
		}
	}
	cfg.Keybindings["quit"] = "Z"
	if got := cfg.KeyFor("quit"); got != "Z" {
		t.Errorf("with an override, KeyFor gave %q, want Z", got)
	}
	if got := cfg.KeyFor("made-up-action"); got != "" {
		t.Errorf("an unknown action gave key %q, want empty", got)
	}
}

func TestActionForKeyInvertsTheMapAndIsStable(t *testing.T) {
	cfg := Defaults()

	if got := cfg.ActionForKey(cfg.KeyFor("quit")); got != "quit" {
		t.Errorf("ActionForKey gave %q, want quit", got)
	}
	if got := cfg.ActionForKey(""); got != "" {
		t.Errorf("the empty key gave action %q, want empty", got)
	}
	if got := cfg.ActionForKey("Ctrl+Alt+Impossible"); got != "" {
		t.Errorf("an unbound key gave action %q, want empty", got)
	}

	cfg.Keybindings["a"] = "X"
	cfg.Keybindings["b"] = "X"
	first := cfg.ActionForKey("X")
	for i := 0; i < 100; i++ {
		if got := cfg.ActionForKey("X"); got != first {
			t.Fatalf("ActionForKey gave %q on call %d and %q on the first", got, i, first)
		}
	}
	if first != "a" && first != "b" {
		t.Errorf("ActionForKey gave %q, which is neither of the two actions", first)
	}
}

// strings.Fields, not Split(" ").
func TestCmdArgsSplitsTheCommandOnSpaces(t *testing.T) {
	cfg := Defaults()

	cfg.Commands["myagent"] = "  tuicr   review  "
	got := cfg.CmdArgs("myagent")
	want := []string{"tuicr", "review"}
	if len(got) != len(want) {
		t.Fatalf("with odd spacing it gave %q, want %q", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("argv[%d] = %q, want %q (full %q)", i, got[i], want[i], got)
		}
	}
	for _, a := range got {
		if a == "" {
			t.Errorf("an empty argument came out of %q", got)
		}
	}

	// An empty argv is the signal of "no command for this action".
	if got := cfg.CmdArgs("nonexistent-action"); len(got) != 0 {
		t.Errorf("an action with no command gave %q, want an empty argv", got)
	}
	cfg.Commands["blank"] = "   "
	if got := cfg.CmdArgs("blank"); len(got) != 0 {
		t.Errorf("a command of only spaces gave %q", got)
	}
}

// The whole function: "did not configure it" differs from "configured it to nothing".
func TestThePaneOverrideIsNotAnEmptyValue(t *testing.T) {
	cfg := Defaults()

	if _, ok := cfg.PaneOverride("hunk"); ok {
		t.Error("with nothing configured it gave an override")
	}
	cfg.Commands["hunk"] = "hunk --model o3"
	argv, ok := cfg.PaneOverride("hunk")
	if !ok {
		t.Fatal("a configured command gave override=false")
	}
	if len(argv) != 3 || argv[0] != "hunk" || argv[2] != "o3" {
		t.Errorf("the override gave %q", argv)
	}
	for _, empty := range []string{"", "   ", "\t\n "} {
		cfg.Commands["hunk"] = empty
		if _, ok := cfg.PaneOverride("hunk"); ok {
			t.Errorf("a command of %q gave override=true", empty)
		}
	}
}

// The order is a decision: commands.<name> wins over tools.<name> over the default.
func TestToolArgsPrioritisesTheOverrideAndThenTheBinary(t *testing.T) {
	cfg := Defaults()

	// The defaults are NOT the tool's name: agent comes out as opencode and editor as vi.
	defaults := map[string]string{
		"tuicr": "tuicr", "hunk": "hunk", "agent": "opencode", "editor": "vi",
	}
	for name, want := range defaults {
		got := cfg.ToolArgs(name)
		if len(got) == 0 {
			t.Errorf("%q unconfigured gave an empty argv", name)
			continue
		}
		if got[0] != want {
			t.Errorf("%q unconfigured gave %q, want it to start with %q", name, got, want)
		}
	}

	cfg.Tools.Tuicr = "/opt/tuicr --dark"
	got := cfg.ToolArgs("tuicr")
	if got[0] != "/opt/tuicr" || len(got) != 2 {
		t.Errorf("with tools.tuicr it gave %q", got)
	}

	// `--model o3` is TWO fields, which a naive single-arg split gets wrong.
	cfg.Commands["tuicr"] = "tuicr review --model o3"
	got = cfg.ToolArgs("tuicr")
	want := []string{"tuicr", "review", "--model", "o3"}
	if len(got) != len(want) {
		t.Fatalf("with commands.tuicr it gave %q, want %q", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("argv[%d] = %q, want %q (full %q)", i, got[i], want[i], got)
		}
	}

	cfg.Commands["tuicr"] = "  "
	got = cfg.ToolArgs("tuicr")
	if got[0] != "/opt/tuicr" {
		t.Errorf("a blank commands.tuicr gave %q, and tools should still win", got)
	}
}

func TestFieldsOrUsesTheFallbackOnlyWhenEmpty(t *testing.T) {
	cases := []struct {
		raw      string
		fallback string
		want     string
	}{
		{"", "hunk", "hunk"},
		{"   ", "hunk", "hunk"},
		{"\t\n", "hunk", "hunk"},
		{"/opt/hunk", "hunk", "/opt/hunk"},
		{"/opt/hunk --dark", "hunk", "/opt/hunk"},
		{"with  spaces   inside", "hunk", "with"},
	}
	for _, c := range cases {
		got := fieldsOr(c.raw, c.fallback)
		if len(got) == 0 {
			t.Errorf("fieldsOr(%q, %q) gave an empty argv", c.raw, c.fallback)
			continue
		}
		if got[0] != c.want {
			t.Errorf("fieldsOr(%q, %q) gave %q, want it to start with %q", c.raw, c.fallback, got, c.want)
		}
	}
}
