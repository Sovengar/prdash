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
		t.Fatalf("warning = %q, want empty", warn)
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
		t.Error("bitbucket should come disabled")
	}
	if len(cfg.Roots) != 1 || !strings.HasSuffix(cfg.Roots[0], "dev") {
		t.Errorf("roots = %v", cfg.Roots)
	}
}

func writeConfig(t *testing.T, path, content string) {
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
		t.Fatal("a warning was expected for broken TOML")
	}
	if cfg.RefreshInterval != 60*time.Second {
		t.Errorf("it should fall back to defaults, refresh = %v", cfg.RefreshInterval)
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
		t.Error("github should end up disabled")
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
		t.Errorf("hunk should keep its default, got %q", cfg.Tools.Hunk)
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
		t.Errorf("refresh = %v, want 0 (manual only)", cfg.RefreshInterval)
	}
	if !cfg.Forges.GitHub.Enabled {
		t.Error("github should stay enabled by default")
	}
}

func TestGitLabClonePrefixDerivesFromAPIBase(t *testing.T) {
	cases := []struct {
		name      string
		apiBase   string
		cloneBase string
		want      string
	}{
		{"subfolder", "/git/api/v4/", "", "git"},
		{"root", "/api/v4/", "", ""},
		{"no leading slash", "git/api/v4/", "", "git"},
		{"empty", "", "", ""},
		{"not REST shaped", "/custom/", "", ""},
		{"override wins", "/git/api/v4/", "/custom/", "custom"},
		{"override cleans to root", "/git/api/v4/", "/", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			g := GitLabConfig{APIBase: tc.apiBase, CloneBase: tc.cloneBase}
			if got := g.ClonePrefix(); got != tc.want {
				t.Fatalf("ClonePrefix = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestGitHubClonePrefix(t *testing.T) {
	if got := (GitHubConfig{}).ClonePrefix(); got != "" {
		t.Fatalf("github root = %q", got)
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
	// APIBase's default is "/api/v4/", with the trailing slash: no prefix must be derived.
	if got := Defaults().Forges.GitLab.ClonePrefix(); got != "" {
		t.Fatalf("gitlab default ClonePrefix = %q, want empty (root)", got)
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
		t.Errorf("entries without ~/ must not change: %v", got)
	}
}

// The merge treats "not set" and "set to empty" as different things on purpose.
func TestTheEmptyPathsDoNotWipeTheDefaults(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	writeConfig(t, path, `
data_dir = ""
clone_dir = ""
worktree_dir = ""
roots = [""]
`)
	cfg, warn := LoadFrom(path)
	if warn != "" {
		t.Fatalf("a config with empty paths is not an error: %s", warn)
	}
	def := Defaults()
	if cfg.DataDir != def.DataDir {
		t.Errorf("an empty data_dir should not clobber the default: %q vs %q", cfg.DataDir, def.DataDir)
	}
	if cfg.CloneDir != def.CloneDir {
		t.Errorf("an empty clone_dir should not clobber the default: %q vs %q", cfg.CloneDir, def.CloneDir)
	}
	if cfg.WorktreeDir != def.WorktreeDir {
		t.Errorf("an empty worktree_dir should not clobber the default: %q vs %q", cfg.WorktreeDir, def.WorktreeDir)
	}
	other := t.TempDir()
	path2 := filepath.Join(dir, "other.toml")
	writeConfig(t, path2, "data_dir = \""+other+"\"\n")
	cfg2, _ := LoadFrom(path2)
	if cfg2.DataDir != other {
		t.Errorf("data_dir with a value = %q, want %q", cfg2.DataDir, other)
	}
}

// The three forges do not merge alike: GitHub and GitLab have their own block.
func TestBitbucketEnabledAppliesAsTheOnlyForgeOfTheBlock(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "off.toml")
	writeConfig(t, path, "[forge.bitbucket]\nenabled = false\n")
	cfg, _ := LoadFrom(path)
	if cfg.Forges.Bitbucket.Enabled {
		t.Error("enabled = false should turn bitbucket off")
	}

	path = filepath.Join(dir, "on.toml")
	writeConfig(t, path, "[forge.bitbucket]\nenabled = true\n")
	cfg, _ = LoadFrom(path)
	if !cfg.Forges.Bitbucket.Enabled {
		t.Error("enabled = true should turn bitbucket on")
	}

	// The block without `enabled`: the default stays false, which is what the second `&&` is for.
	path = filepath.Join(dir, "bare.toml")
	writeConfig(t, path, "[forge.bitbucket]\n")
	cfg, _ = LoadFrom(path)
	if cfg.Forges.Bitbucket.Enabled {
		t.Error("a block without enabled should not turn the forge on")
	}
	// A block with other fields neither: the condition is about `enabled`, not about the block existing.
	path = filepath.Join(dir, "empty.toml")
	writeConfig(t, path, "[forge.bitbucket]\nclone_base = \"https://bitbucket.example.com\"\n")
	cfg, warn := LoadFrom(path)
	if warn != "" {
		t.Errorf("a block without enabled is not a broken config: %s", warn)
	}
	if cfg.Forges.Bitbucket.Enabled {
		t.Error("a block with other fields should not turn the forge on")
	}
}

// The keybindings merge ignores an empty value on purpose: a keys map is written per action, and
// an empty one would delete the shortcut.
func TestTheEmptyKeybindingsDoNotDisableShortcuts(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "kb.toml")
	writeConfig(t, path, `
[keybindings]
"quit" = ""
"merge" = "x"
`)
	cfg, warn := LoadFrom(path)
	if warn != "" {
		t.Fatalf("broken config: %s", warn)
	}
	def := DefaultKeybindings()
	if cfg.Keybindings["quit"] != def["quit"] {
		t.Errorf("quit = %q, want the default %q: an empty value does not disable the shortcut", cfg.Keybindings["quit"], def["quit"])
	}
	// A real value DOES overwrite.
	if cfg.Keybindings["merge"] != "x" {
		t.Errorf("merge = %q, want \"x\"", cfg.Keybindings["merge"])
	}
	for action, key := range cfg.Keybindings {
		if key == "" {
			t.Errorf("the action %q ended up without a key: %v", action, cfg.Keybindings)
		}
	}
}

// expand must only convert a real `~/`. The cases it does NOT touch are half the contract.
func TestExpandOnlyTouchesATildeAtTheStart(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skipf("no home: %v", err)
	}
	cases := map[string]string{
		"~/dev":     filepath.Join(home, "dev"),
		"~/":        filepath.Join(home),
		"~":         "~",         // a bare ~ is not converted
		"~user/dev": "~user/dev", // another user's home: unknown
		"~x":        "~x",        // the second letter is not a separator
		"a~b":       "a~b",       // the ~ is not at the start
		"/a~/b":     "/a~/b",     // same, inside an absolute path
		"~a/b":      "~a/b",      // second letter not a separator
		"~a":        "~a",        // same, without a slash
		"":          "",          // empty is left alone
		"/abs":      "/abs",      // no tilde
		"relative":  "relative",  // no tilde
		"dev/":      "dev/",      // no tilde
		"~~/dev":    "~~/dev",    // double tilde: the second is not a separator
	}
	for in, want := range cases {
		if got := expand(in); got != want {
			t.Errorf("expand(%q) = %q, want %q", in, got, want)
		}
	}
	if got := expand("~/"); got != filepath.Join(home) {
		t.Errorf(`expand("~/") = %q, want %q`, got, filepath.Join(home))
	}
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
	// `tools.<name>` defines the binary; `commands.<name>` the full argv.
	cfg.Tools.Agent = "claude"
	if got := cfg.ToolArgs("agent"); len(got) != 1 || got[0] != "claude" {
		t.Fatalf("agent tools = %v", got)
	}
	cfg.Commands["agent"] = "claude --model opus"
	if got := cfg.ToolArgs("agent"); len(got) != 3 || got[0] != "claude" || got[2] != "opus" {
		t.Fatalf("agent commands = %v", got)
	}
	if got := cfg.ToolArgs("unknown"); got != nil {
		t.Fatalf("unknown tool = %v", got)
	}
}

func TestPaneOverride(t *testing.T) {
	cfg := Defaults()
	if _, ok := cfg.PaneOverride("hunk"); ok {
		t.Fatal("hunk should not come with an override by default")
	}
	cfg.Commands["hunk"] = "hunk diff develop...HEAD --watch"
	got, ok := cfg.PaneOverride("hunk")
	if !ok {
		t.Fatal("hunk should have an override")
	}
	want := []string{"hunk", "diff", "develop...HEAD", "--watch"}
	if strings.Join(got, " ") != strings.Join(want, " ") {
		t.Fatalf("override = %v, want %v", got, want)
	}
	cfg.Commands["agent"] = "   "
	if _, ok := cfg.PaneOverride("agent"); ok {
		t.Fatal("a blank value is not an override")
	}
}

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
			t.Fatalf("%s should have an override", key)
		}
		if strings.Join(got, " ") != want {
			t.Fatalf("%s = %q, want %q", key, strings.Join(got, " "), want)
		}
	}
}

// `[tools].editor` sets the editor's ORDER; `[commands].editor` replaces it entirely.
func TestEditorToolAndOverride(t *testing.T) {
	if got := strings.Join(Defaults().ToolArgs("editor"), " "); got != "vi" {
		t.Fatalf("default editor = %q, want %q", got, "vi")
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
		t.Fatalf("Hints() = %v, want %v", got, want)
	}
}

func TestHintsWithDynamicState(t *testing.T) {
	cfg := Defaults()
	bar := strings.Join(cfg.Hints(HintState{"prefix-mode": "full"}), " ")
	if !strings.Contains(bar, "p prefix: full") {
		t.Errorf("the bar does not name the current mode: %v", cfg.Hints(HintState{"prefix-mode": "full"}))
	}

	bar = strings.Join(cfg.Hints(HintState{"does-not-exist": "something"}), " ")
	if strings.Contains(bar, "something") {
		t.Errorf("a state for an absent action added an entry: %v", cfg.Hints(HintState{"does-not-exist": "something"}))
	}

	cfg.Keybindings["prefix-mode"] = "P"
	bar = strings.Join(cfg.Hints(HintState{"prefix-mode": "leaf"}), " ")
	if !strings.Contains(bar, "P prefix: leaf") {
		t.Errorf("the rebind was not combined with the state: %v", cfg.Hints(HintState{"prefix-mode": "leaf"}))
	}
	if strings.Contains(bar, "p prefix") {
		t.Errorf("the bar still shows the previous key: %v", cfg.Hints(HintState{"prefix-mode": "leaf"}))
	}
}

// The anti-drift guard: an action added to DefaultKeybindings and not to hintOrder makes the bar
// lie.
func TestHintsCoverAllKeybindings(t *testing.T) {
	bar := strings.Join(Defaults().Hints(nil), " ")
	for action, key := range DefaultKeybindings() {
		if !strings.Contains(bar, key+" ") {
			t.Errorf("action %q (key %q) does not show up in the hints bar", action, key)
		}
	}
}

func TestHintsFollowTheRebind(t *testing.T) {
	cfg := Defaults()
	cfg.Keybindings["open-browser"] = "b"
	cfg.Keybindings["mount-review"] = "v"
	bar := strings.Join(cfg.Hints(nil), " ")
	if !strings.Contains(bar, "b open") || !strings.Contains(bar, "v mount review") {
		t.Errorf("the rebind did not reach the bar: %v", cfg.Hints(nil))
	}
	if strings.Contains(bar, "o open") || strings.Contains(bar, "m mount review") {
		t.Errorf("the bar still shows the previous key: %v", cfg.Hints(nil))
	}
}

func TestDefaultKeybindingsCoverActions(t *testing.T) {
	kb := DefaultKeybindings()
	for _, action := range []string{"quit", "refresh", "mount-review", "approve", "merge", "section-next", "open-browser", "prefix-mode", "retarget"} {
		if kb[action] == "" {
			t.Errorf("missing keybinding %q", action)
		}
	}
}

// Two actions sharing a key is a silent failure: ActionForKey resolves alphabetically by
// action, so which one wins is arbitrary.
func TestDefaultKeybindingsAreNotRepeated(t *testing.T) {
	owner := map[string]string{}
	for action, key := range DefaultKeybindings() {
		if key == "" {
			continue
		}
		if other, ok := owner[key]; ok {
			t.Errorf("actions %q and %q share the key %q", other, action, key)
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
		t.Errorf("ActionForKey(z) = %q, want empty", got)
	}
}

func TestDefaultKeybindingsDoNotCollide(t *testing.T) {
	kb := DefaultKeybindings()
	owner := map[string]string{}
	for action, key := range kb {
		if other, dup := owner[key]; dup {
			t.Errorf("key %q shared by %q and %q", key, other, action)
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
		t.Error("the old key should not stay mapped after the override")
	}
}
