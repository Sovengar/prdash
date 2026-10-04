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

func TestLoadFromDegradaADefaultsYAvisa(t *testing.T) {
	dir := t.TempDir()

	comoCarpeta := filepath.Join(dir, "config-carpeta")
	if err := os.MkdirAll(comoCarpeta, 0o755); err != nil {
		t.Fatal(err)
	}

	sinPermiso := filepath.Join(dir, "config-sin-permiso")
	if err := os.WriteFile(sinPermiso, []byte("[general]\n"), 0o000); err != nil {
		t.Fatal(err)
	}
	if _, err := os.ReadFile(sinPermiso); err == nil {
		t.Skip("el usuario puede leer un fichero en modo 000: el caso de EACCES no aplica aquí")
	}

	conBucle := filepath.Join(dir, "config-bucle")
	if err := os.Symlink(conBucle, conBucle); err != nil {
		t.Fatalf("crear el enlace que se apunta a si mismo: %v", err)
	}

	for _, c := range []struct {
		nombre string
		path   string
	}{
		{"directorio en vez de fichero", comoCarpeta},
		{"fichero sin permiso de lectura", sinPermiso},
		{"enlace que se apunta a si mismo", conBucle},
	} {
		cfg, aviso := LoadFrom(c.path)

		if aviso == "" {
			t.Errorf("%s: LoadFrom degrado SIN aviso", c.nombre)
			continue
		}
		if !strings.HasPrefix(aviso, "config:") {
			t.Errorf("%s: el aviso %q no lleva el prefijo config:", c.nombre, aviso)
		}
		def := Defaults()
		if !mismasSalvoAviso(cfg, def) {
			t.Errorf("%s: la config de vuelta no es la de defaults", c.nombre)
		}
	}
}

// The asymmetry with the above is deliberate and reads backwards from how it sounds: an absent
// config is the normal case and warns nothing, an unreadable one warns.
func TestUnConfigAusenteDegradaEnSilencioYUnoVacioTambien(t *testing.T) {
	dir := t.TempDir()

	cfg, aviso := LoadFrom(filepath.Join(dir, "no-existe.toml"))
	if aviso != "" {
		t.Errorf("un config ausente dio aviso %q: se avisaría en cada arranque", aviso)
	}
	if !mismasSalvoAviso(cfg, Defaults()) {
		t.Error("un config ausente no devolvió los defaults")
	}

	vacio := filepath.Join(dir, "vacio.toml")
	if err := os.WriteFile(vacio, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, aviso = LoadFrom(vacio)
	if aviso != "" {
		t.Errorf("un config vacío dio aviso %q", aviso)
	}
	if !mismasSalvoAviso(cfg, Defaults()) {
		t.Error("un config vacío no devolvió los defaults")
	}
}

// The most expensive of the four failures, which is why the important assertion is not the one
// about the error.
func TestUnTOMLQueNoParseaAvisaYNoSeQuedaConMitadDeLoLeido(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "roto.toml")
	escribirConfig(t, path, `
roots = ["/tmp/un-sitio-que-no-debe-aplicarse"]
data_dir = "/otro"
refresh_interval = "10s"
esto-no-es-una-llave
`)

	cfg, aviso := LoadFrom(path)

	if aviso == "" {
		t.Fatal("un TOML malformado dio nil: la config rota se aplicaría en silencio")
	}
	if !strings.Contains(aviso, "toml") && !strings.Contains(aviso, "config:") {
		t.Errorf("el aviso %q no dice que es del config", aviso)
	}
	def := Defaults()
	if len(cfg.Roots) != len(def.Roots) || len(cfg.Roots) == 0 {
		t.Errorf("la config se quedó a medias: %d raíces, las de por defecto son %d",
			len(cfg.Roots), len(def.Roots))
	}
	for _, r := range cfg.Roots {
		if r == "/tmp/un-sitio-que-no-debe-aplicarse" {
			t.Error("se aplicó un roots del TOML que no llegó a parsear entero")
		}
	}
	if cfg.DataDir == "/otro" {
		t.Error("se aplicó un data_dir de un TOML que no llegó a parsear entero")
	}
}

// Per-field degradation is where "degrade with a warning" does NOT apply: an invalid
// `refresh_interval` is one bad key, not a broken file.
func TestUnValorInvalidoSeIgnoraYUnoValidoSeAplica(t *testing.T) {
	dir := t.TempDir()
	def := Defaults()

	for _, c := range []struct {
		nombre   string
		toml     string
		want     time.Duration
		explicar string
	}{
		{"duración no parseable", `refresh_interval = "quince minutos"`, def.RefreshInterval,
			"una duración mal escrita se ignora y el default manda"},
		{"duración negativa", `refresh_interval = "-5s"`, def.RefreshInterval,
			"un intervalo negativo no se aplica: el tick se pediría en el pasado"},
		{"duración vacía", `refresh_interval = ""`, def.RefreshInterval,
			"una duración vacía no es una duración"},
		{"duración válida", `refresh_interval = "90s"`, 90 * time.Second,
			"una duración válida sí se aplica"},
		{"cero", `refresh_interval = "0s"`, 0,
			"cero SÍ se aplica: es un refresco manual, y es lo que el usuario pidió"},
	} {
		path := filepath.Join(dir, "c.toml")
		escribirConfig(t, path, c.toml)

		cfg, aviso := LoadFrom(path)
		if aviso != "" {
			t.Errorf("%s: dio aviso %q, y un error tipográfico no es un config roto",
				c.nombre, aviso)
		}
		if cfg.RefreshInterval != c.want {
			t.Errorf("%s: RefreshInterval = %v, want %v (%s)",
				c.nombre, cfg.RefreshInterval, c.want, c.explicar)
		}
	}
}

// The guard exists because the code writes `fc.DataDir != nil && *fc.DataDir != ""`.
func TestUnaCadenaVaciaEnUnaRutaSeIgnoraYNoLaDejaVacia(t *testing.T) {
	dir := t.TempDir()
	def := Defaults()
	path := filepath.Join(dir, "c.toml")

	escribirConfig(t, path, `
data_dir = ""
clone_dir = ""
worktree_dir = ""
`)
	cfg, aviso := LoadFrom(path)
	if aviso != "" {
		t.Errorf("dio aviso %q", aviso)
	}
	if cfg.DataDir != def.DataDir {
		t.Errorf("DataDir = %q con un data_dir vacío en el config, want el default %q",
			cfg.DataDir, def.DataDir)
	}
	if cfg.CloneDir != def.CloneDir {
		t.Errorf("CloneDir = %q con un clone_dir vacío, want %q", cfg.CloneDir, def.CloneDir)
	}
	if cfg.WorktreeDir != def.WorktreeDir {
		t.Errorf("WorktreeDir = %q con un worktree_dir vacío, want %q",
			cfg.WorktreeDir, def.WorktreeDir)
	}
	for nombre, valor := range map[string]string{
		"DataDir": cfg.DataDir, "CloneDir": cfg.CloneDir, "WorktreeDir": cfg.WorktreeDir,
	} {
		if valor == "" {
			t.Errorf("%s quedó vacía: sería un path relativo al directorio de trabajo", nombre)
		}
	}
}

func TestUnaPistaConTeclaVaciaNoSePinta(t *testing.T) {
	cfg := Defaults()

	conHuerfana := cfg.Hints(HintState{"accion-que-no-existe": "texto"})
	if len(conHuerfana) != len(cfg.Hints(nil)) {
		t.Errorf("una acción sin tecla añadió %d entradas a la barra",
			len(conHuerfana)-len(cfg.Hints(nil)))
	}

	conVacia := Defaults()
	for _, accion := range []string{"refresh", "approve", "merge", "quit"} {
		conVacia.Keybindings[accion] = ""
	}
	for _, h := range conVacia.Hints(nil) {
		if strings.TrimSpace(h) == "" {
			t.Errorf("una pista con la tecla vacía se pintó como %q", h)
		}
		if strings.HasPrefix(h, " ") {
			t.Errorf("la pista %q empieza por un hueco: la tecla desapareció y el hueco quedó", h)
		}
	}
}

func mismasSalvoAviso(a, b Config) bool {
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
