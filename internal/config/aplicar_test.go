package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestUnaRutaDelConfigSeExpandeYLasQueNoEmpiezanPorTildeNo(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skipf("sin HOME: %v", err)
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "c.toml")
	escribirConfig(t, path, `
data_dir = "~/datos-de-prdash"
clone_dir = "~otro/cosas"
worktree_dir = "~"
`)

	cfg, aviso := LoadFrom(path)
	if aviso != "" {
		t.Fatalf("aviso %q", aviso)
	}
	if quiere := filepath.Join(home, "datos-de-prdash"); cfg.DataDir != quiere {
		t.Errorf("DataDir = %q, want %q (el ~ con separador sí se expande)", cfg.DataDir, quiere)
	}
	if cfg.CloneDir != "~otro/cosas" {
		t.Errorf("CloneDir = %q: un ~ sin separador es un nombre de fichero y se deja como está",
			cfg.CloneDir)
	}
	// A bare `~` DOES expand: p[1] does not exist, so the separator case does not fire.
	if cfg.WorktreeDir != home && cfg.WorktreeDir != "~" {
		t.Errorf("WorktreeDir = %q con un ~ a secas", cfg.WorktreeDir)
	}
}

// For each field a config that sets it and one that omits it, which is what makes the merge
// useful.
func TestLasRutasDelConfigSeAplicanYLasAusentesNoSeTocan(t *testing.T) {
	def := Defaults()

	dir := t.TempDir()
	vacia := filepath.Join(dir, "vacia.toml")
	escribirConfig(t, vacia, "refresh_interval = \"30s\"\n")

	cfg, aviso := LoadFrom(vacia)
	if aviso != "" {
		t.Fatalf("aviso %q", aviso)
	}
	if cfg.DataDir != def.DataDir || cfg.CloneDir != def.CloneDir ||
		cfg.WorktreeDir != def.WorktreeDir {
		t.Errorf("una config sin rutas cambió alguna: %q %q %q",
			cfg.DataDir, cfg.CloneDir, cfg.WorktreeDir)
	}

	lasTres := filepath.Join(dir, "tres.toml")
	escribirConfig(t, lasTres, `
data_dir = "/tmp/datos"
clone_dir = "/tmp/clones"
worktree_dir = "/tmp/worktrees"
`)
	cfg, aviso = LoadFrom(lasTres)
	if aviso != "" {
		t.Fatalf("aviso %q", aviso)
	}
	if cfg.DataDir != "/tmp/datos" || cfg.CloneDir != "/tmp/clones" ||
		cfg.WorktreeDir != "/tmp/worktrees" {
		t.Errorf("no se aplicaron las tres rutas: %q %q %q",
			cfg.DataDir, cfg.CloneDir, cfg.WorktreeDir)
	}
}

// The key is `autoreview`, NOT `auto_review` nor `[auto-review]`.
func TestElAutoReviewSeParseaYElAbsentedejaloComoEstava(t *testing.T) {
	def := Defaults()
	dir := t.TempDir()

	path := filepath.Join(dir, "sin-ar.toml")
	escribirConfig(t, path, "refresh_interval = \"30s\"\n")
	cfg, aviso := LoadFrom(path)
	if aviso != "" {
		t.Fatalf("aviso %q", aviso)
	}
	if cfg.AutoReview.Enabled != def.AutoReview.Enabled {
		t.Errorf("sin [autoreview] el enabled pasó a %v", cfg.AutoReview.Enabled)
	}
	if len(cfg.AutoReview.Allowlist) != len(def.AutoReview.Allowlist) {
		t.Errorf("sin [autoreview] el allowlist cambió de tamaño")
	}

	// Presente y completo.
	path = filepath.Join(dir, "con-ar.toml")
	escribirConfig(t, path, `
[autoreview]
enabled = true
allowlist = ["acme/seguro", "otro/repo"]
`)
	cfg, aviso = LoadFrom(path)
	if aviso != "" {
		t.Fatalf("aviso %q: la clave [autoreview] debería parsear", aviso)
	}
	if !cfg.AutoReview.Enabled {
		t.Error("enabled = true no se aplicó")
	}
	if len(cfg.AutoReview.Allowlist) != 2 {
		t.Fatalf("el allowlist tiene %d entradas, want 2: %v",
			len(cfg.AutoReview.Allowlist), cfg.AutoReview.Allowlist)
	}
	// One config's slice is INDEPENDENT of another's: two LoadFrom of the same file do not share
	//the allowlist's memory.
	otro, aviso := LoadFrom(path)
	if aviso != "" {
		t.Fatal(aviso)
	}
	cfg.AutoReview.Allowlist[0] = "mutado-en-la-primera"
	if otro.AutoReview.Allowlist[0] != "acme/seguro" {
		t.Error("el allowlist se guarda por referencia entre configs: mutarlo escribiría " +
			"en el TOML decodificado de la otra")
	}
	// A nil slice and an empty one are not the same: not in the contract, but a len() that treated
	// them alike would.
	vacio, _ := LoadFrom(path)
	otro.AutoReview.Allowlist = nil
	if len(vacio.AutoReview.Allowlist) == 0 {
		t.Error("vaciar el allowlist de una config vació el de otra")
	}
}

// `enabled = false` is a LINE and not a loose boolean because GitLab is the only forge where
// disabling it changes the inbox.
func TestDesactivarUnForgeEnElConfigDesapareceDelInboxYNoSoloLoMarca(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "forges.toml")

	// GitLab disabled: before this test nothing disabled it, and the branch was dead.
	escribirConfig(t, path, `
[forge.gitlab]
enabled = false
`)
	cfg, aviso := LoadFrom(path)
	if aviso != "" {
		t.Fatalf("aviso %q", aviso)
	}
	def := Defaults()
	if cfg.Forges.GitLab.Enabled {
		t.Error("enabled = false en [forge.gitlab] no desactivó el forge")
	}
	if !def.Forges.GitLab.Enabled {
		t.Fatal("el fixture no sirve: GitLab viene activado por defecto")
	}
	// Y GitHub no se toca: cada forge es independiente y desactivar uno no desactiva el otro.
	if !cfg.Forges.GitHub.Enabled {
		t.Error("desactivar GitLab desactivó GitHub")
	}

	// The inverse: a default of disabled would be re-enabled by `enabled = true`.
	escribirConfig(t, path, `
[forge.gitlab]
enabled = true
`)
	cfg, _ = LoadFrom(path)
	if !cfg.Forges.GitLab.Enabled {
		t.Error("enabled = true en [forge.gitlab] no activó el forge")
	}

	escribirConfig(t, path, `
[forge.bitbucket]
enabled = false
`)
	cfg, _ = LoadFrom(path)
	if cfg.Forges.Bitbucket.Enabled {
		t.Error("enabled = false en [forge.bitbucket] no desactivó el forge")
	}

	escribirConfig(t, path, `
[forge.github]
host = "github.example.com"
clone_base = "/tmp/bases"
`)
	cfg, _ = LoadFrom(path)
	def = Defaults()
	if cfg.Forges.GitHub.Enabled != def.Forges.GitHub.Enabled {
		t.Error("poner host y clone_base cambió el enabled de GitHub")
	}
	if cfg.Forges.GitHub.Host != "github.example.com" {
		t.Errorf("Host = %q, want el override", cfg.Forges.GitHub.Host)
	}
	if cfg.Forges.GitHub.CloneBase != "/tmp/bases" {
		t.Errorf("CloneBase = %q, want el override", cfg.Forges.GitHub.CloneBase)
	}

	// An empty string does NOT replace: that is the difference between "I did not set it" and "I set
	// it empty".
	escribirConfig(t, path, `
[forge.github]
host = ""
`)
	cfg, _ = LoadFrom(path)
	if cfg.Forges.GitHub.Host != def.Forges.GitHub.Host {
		t.Errorf("un host vacío sustituyó al default: %q", cfg.Forges.GitHub.Host)
	}
}

// The other half of the guard, and the one that makes the previous useful.
func TestUnForgeAusenteDelConfigNoTocaNada(t *testing.T) {
	def := Defaults()
	dir := t.TempDir()
	path := filepath.Join(dir, "uno.toml")

	escribirConfig(t, path, "[forge.github]\nhost = \"otro.example.com\"\n")
	cfg, aviso := LoadFrom(path)
	if aviso != "" {
		t.Fatalf("aviso %q", aviso)
	}
	if cfg.Forges.GitLab != def.Forges.GitLab {
		t.Error("un [forge.github] cambió GitLab")
	}
	if cfg.Forges.Bitbucket != def.Forges.Bitbucket {
		t.Error("un [forge.github] cambió Bitbucket")
	}

	// An unknown forge name is ignored in silence: the TOML decoder has no field for it.
	escribirConfig(t, path, "[forge.svn]\nenabled = true\n")
	cfg, aviso = LoadFrom(path)
	if aviso != "" {
		t.Errorf("un forge desconocido dio aviso %q, y el decoder lo ignora sin quejarse", aviso)
	}
	if !mismasSalvoAviso(cfg, def) {
		t.Error("un forge desconocido dejó la config en otra cosa que los defaults")
	}
}
