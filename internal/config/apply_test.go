package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestAConfigPathIsExpandedAndTheOnesNotStartingWithTildeAreNot(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skipf("no HOME: %v", err)
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "c.toml")
	writeConfig(t, path, `
data_dir = "~/data-of-prdash"
clone_dir = "~other-stuff"
worktree_dir = "~"
`)

	cfg, warn := LoadFrom(path)
	if warn != "" {
		t.Fatalf("warn %q", warn)
	}
	if want := filepath.Join(home, "data-of-prdash"); cfg.DataDir != want {
		t.Errorf("DataDir = %q, want %q (a ~ with a separator does expand)", cfg.DataDir, want)
	}
	if cfg.CloneDir != "~other-stuff" {
		t.Errorf("CloneDir = %q: a ~ without a separator is a file name and is left alone",
			cfg.CloneDir)
	}
	// A bare `~` DOES expand: p[1] does not exist, so the separator case does not fire.
	if cfg.WorktreeDir != home && cfg.WorktreeDir != "~" {
		t.Errorf("WorktreeDir = %q with a bare ~", cfg.WorktreeDir)
	}
}

// For each field a config that sets it and one that omits it, which is what makes the merge
// useful.
func TestTheConfigPathsAreAppliedAndTheMissingOnesAreNotTouched(t *testing.T) {
	def := Defaults()

	dir := t.TempDir()
	empty := filepath.Join(dir, "empty.toml")
	writeConfig(t, empty, "refresh_interval = \"30s\"\n")

	cfg, warn := LoadFrom(empty)
	if warn != "" {
		t.Fatalf("warn %q", warn)
	}
	if cfg.DataDir != def.DataDir || cfg.CloneDir != def.CloneDir ||
		cfg.WorktreeDir != def.WorktreeDir {
		t.Errorf("a config without paths changed one of them: %q %q %q",
			cfg.DataDir, cfg.CloneDir, cfg.WorktreeDir)
	}

	allThree := filepath.Join(dir, "three.toml")
	writeConfig(t, allThree, `
data_dir = "/tmp/data"
clone_dir = "/tmp/clones"
worktree_dir = "/tmp/worktrees"
`)
	cfg, warn = LoadFrom(allThree)
	if warn != "" {
		t.Fatalf("warn %q", warn)
	}
	if cfg.DataDir != "/tmp/data" || cfg.CloneDir != "/tmp/clones" ||
		cfg.WorktreeDir != "/tmp/worktrees" {
		t.Errorf("the three paths were not applied: %q %q %q",
			cfg.DataDir, cfg.CloneDir, cfg.WorktreeDir)
	}
}

// The key is `autoreview`, NOT `auto_review` nor `[auto-review]`.
func TestAutoReviewParsesAndItsAbsenceLeavesThingsAsTheyWere(t *testing.T) {
	def := Defaults()
	dir := t.TempDir()

	path := filepath.Join(dir, "no-ar.toml")
	writeConfig(t, path, "refresh_interval = \"30s\"\n")
	cfg, warn := LoadFrom(path)
	if warn != "" {
		t.Fatalf("warn %q", warn)
	}
	if cfg.AutoReview.Enabled != def.AutoReview.Enabled {
		t.Errorf("without [autoreview] the enabled changed to %v", cfg.AutoReview.Enabled)
	}
	if len(cfg.AutoReview.Allowlist) != len(def.AutoReview.Allowlist) {
		t.Errorf("without [autoreview] the allowlist changed size")
	}

	// Present and complete.
	path = filepath.Join(dir, "with-ar.toml")
	writeConfig(t, path, `
[autoreview]
enabled = true
allowlist = ["acme/seguro", "other/repo"]
`)
	cfg, warn = LoadFrom(path)
	if warn != "" {
		t.Fatalf("warn %q: the [autoreview] key should parse", warn)
	}
	if !cfg.AutoReview.Enabled {
		t.Error("enabled = true was not applied")
	}
	if len(cfg.AutoReview.Allowlist) != 2 {
		t.Fatalf("the allowlist has %d entries, want 2: %v",
			len(cfg.AutoReview.Allowlist), cfg.AutoReview.Allowlist)
	}
	// One config's slice is INDEPENDENT of another's: two LoadFrom of the same file do not share
	//the allowlist's memory.
	other, warn := LoadFrom(path)
	if warn != "" {
		t.Fatal(warn)
	}
	cfg.AutoReview.Allowlist[0] = "mutated-in-the-first"
	if other.AutoReview.Allowlist[0] != "acme/seguro" {
		t.Error("the allowlist is stored by reference between configs: mutating it would write " +
			"into the other one's decoded TOML")
	}
	// A nil slice and an empty one are not the same: not in the contract, but a len() that treated
	// them alike would.
	empty, _ := LoadFrom(path)
	other.AutoReview.Allowlist = nil
	if len(empty.AutoReview.Allowlist) == 0 {
		t.Error("emptying one config's allowlist emptied the other one's")
	}
}

// `enabled = false` is a LINE and not a loose boolean because GitLab is the only forge where
// disabling it changes the inbox.
func TestDisablingAForgeInConfigMakesItDisappearFromTheInboxAndNotJustGetMarked(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "forges.toml")

	// GitLab disabled: before this test nothing disabled it, and the branch was dead.
	writeConfig(t, path, `
[forge.gitlab]
enabled = false
`)
	cfg, warn := LoadFrom(path)
	if warn != "" {
		t.Fatalf("warn %q", warn)
	}
	def := Defaults()
	if cfg.Forges.GitLab.Enabled {
		t.Error("enabled = false in [forge.gitlab] did not disable the forge")
	}
	if !def.Forges.GitLab.Enabled {
		t.Fatal("the fixture is useless: GitLab ships enabled by default")
	}
	// And GitHub is untouched: each forge is independent and disabling one does not disable the other.
	if !cfg.Forges.GitHub.Enabled {
		t.Error("disabling GitLab disabled GitHub")
	}

	// The inverse: a default of disabled would be re-enabled by `enabled = true`.
	writeConfig(t, path, `
[forge.gitlab]
enabled = true
`)
	cfg, _ = LoadFrom(path)
	if !cfg.Forges.GitLab.Enabled {
		t.Error("enabled = true in [forge.gitlab] did not enable the forge")
	}

	writeConfig(t, path, `
[forge.bitbucket]
enabled = false
`)
	cfg, _ = LoadFrom(path)
	if cfg.Forges.Bitbucket.Enabled {
		t.Error("enabled = false in [forge.bitbucket] did not disable the forge")
	}

	writeConfig(t, path, `
[forge.github]
host = "github.example.com"
clone_base = "/tmp/bases"
`)
	cfg, _ = LoadFrom(path)
	def = Defaults()
	if cfg.Forges.GitHub.Enabled != def.Forges.GitHub.Enabled {
		t.Error("setting host and clone_base changed GitHub's enabled")
	}
	if cfg.Forges.GitHub.Host != "github.example.com" {
		t.Errorf("Host = %q, want the override", cfg.Forges.GitHub.Host)
	}
	if cfg.Forges.GitHub.CloneBase != "/tmp/bases" {
		t.Errorf("CloneBase = %q, want the override", cfg.Forges.GitHub.CloneBase)
	}

	// An empty string does NOT replace: that is the difference between "I did not set it" and "I set
	// it empty".
	writeConfig(t, path, `
[forge.github]
host = ""
`)
	cfg, _ = LoadFrom(path)
	if cfg.Forges.GitHub.Host != def.Forges.GitHub.Host {
		t.Errorf("an empty host replaced the default: %q", cfg.Forges.GitHub.Host)
	}
}

// The other half of the guard, and the one that makes the previous useful.
func TestAMissingForgeInConfigTouchesNothing(t *testing.T) {
	def := Defaults()
	dir := t.TempDir()
	path := filepath.Join(dir, "one.toml")

	writeConfig(t, path, "[forge.github]\nhost = \"other.example.com\"\n")
	cfg, warn := LoadFrom(path)
	if warn != "" {
		t.Fatalf("warn %q", warn)
	}
	if cfg.Forges.GitLab != def.Forges.GitLab {
		t.Error("a [forge.github] changed GitLab")
	}
	if cfg.Forges.Bitbucket != def.Forges.Bitbucket {
		t.Error("a [forge.github] changed Bitbucket")
	}

	// An unknown forge name is ignored in silence: the TOML decoder has no field for it.
	writeConfig(t, path, "[forge.svn]\nenabled = true\n")
	cfg, warn = LoadFrom(path)
	if warn != "" {
		t.Errorf("an unknown forge gave warn %q, and the decoder ignores it without complaining", warn)
	}
	if !sameIgnoringWarning(cfg, def) {
		t.Error("an unknown forge left the config as something other than the defaults")
	}
}
