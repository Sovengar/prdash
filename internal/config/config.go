// Package config carga la configuración XDG de prdash.
//
// El fichero ~/.config/prdash/config.toml es opcional: cualquier clave que
// falte conserva su default. Una config malformada degrada a defaults con un
// warning, sin abortar el arranque. Es el único paquete que lee TOML y el
// entorno XDG.
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/BurntSushi/toml"
)

// FileName es el nombre del fichero de config dentro del directorio XDG.
const FileName = "config.toml"

// DirName es el subdirectorio de prdash bajo $XDG_CONFIG_HOME.
const DirName = "prdash"

// Keybindings mapea nombre de acción → tecla.
type Keybindings map[string]string

// Commands mapea nombre de comando → argv base (separado por espacios).
type Commands map[string]string

// GitHubConfig es la config del forge GitHub.
type GitHubConfig struct {
	Enabled bool
	Host    string
	// CloneBase es el relative URL root de la instancia para clonado/web
	// (p. ej. "git" en un GitHub Enterprise servido en https://host/git/).
	// Vacío significa que la instancia vive en la raíz del host.
	CloneBase string
}

// GitLabConfig es la config del forge GitLab self-managed.
type GitLabConfig struct {
	Enabled bool
	Host    string
	// APIBase documenta el subfolder REST del self-managed (p. ej.
	// "/git/api/v4/"). Además de informativo, se usa para derivar el relative
	// URL root de clonado/web cuando CloneBase está vacío: `glab` resuelve el
	// host y su base API por sí solo, pero el clon por git necesita el prefijo.
	APIBase string
	// CloneBase es un override explícito del relative URL root de clonado/web
	// (p. ej. "git"). Vacío lo deriva de APIBase.
	CloneBase string
	TokenEnv  string // nombre de la variable de entorno del token (lo maneja glab)
}

// ClonePrefix devuelve el relative URL root de clonado/web de un GitHub
// Enterprise configurado en subcarpeta. Normaliza las barras; vacío = raíz.
func (g GitHubConfig) ClonePrefix() string { return normalizeBase(g.CloneBase) }

// ClonePrefix devuelve el relative URL root de clonado/web de la instancia
// GitLab. Prioriza CloneBase; si está vacío lo deriva de APIBase (p. ej.
// "/git/api/v4/" → "git"). Vacío = la instancia vive en la raíz del host.
func (g GitLabConfig) ClonePrefix() string {
	if g.CloneBase != "" {
		return normalizeBase(g.CloneBase)
	}
	return relativeRootFromAPIBase(g.APIBase)
}

// normalizeBase recorta las barras de un relative URL root; vacío = raíz.
func normalizeBase(base string) string { return strings.Trim(base, "/") }

// relativeRootFromAPIBase deriva el relative URL root del api_base REST de
// GitLab quitando el sufijo "api/v4": "/git/api/v4/" → "git", "/api/v4/" → "".
// Devuelve "" si api_base está vacío o no tiene la forma esperada (sin adivinar).
func relativeRootFromAPIBase(apiBase string) string {
	p := normalizeBase(apiBase)
	const rest = "api/v4"
	if p == rest {
		return ""
	}
	if strings.HasSuffix(p, "/"+rest) {
		return normalizeBase(strings.TrimSuffix(p, "/"+rest))
	}
	return ""
}

// BitbucketConfig es la config del forge Bitbucket.
type BitbucketConfig struct {
	Enabled bool
}

// Forges agrupa la config de cada forge.
type Forges struct {
	GitHub    GitHubConfig
	GitLab    GitLabConfig
	Bitbucket BitbucketConfig
}

// Tools es el argv base de las herramientas externas que el orquestador usa.
type Tools struct {
	Tuicr string
	Hunk  string
	Agent string
	GH    string
	Glab  string
}

// AutoReview es la config del modo de auto-review (post-MVP; solo se parsea).
type AutoReview struct {
	Enabled   bool
	Allowlist []string
}

// Config es la configuración resuelta de prdash.
type Config struct {
	Roots           []string
	RefreshInterval time.Duration
	DataDir         string
	CloneDir        string
	WorktreeDir     string
	Forges          Forges
	Tools           Tools
	AutoReview      AutoReview
	Keybindings     Keybindings
	Commands        Commands
}

// fileConfig refleja el TOML crudo del disco, con punteros para distinguir
// "ausente" (conservar default) de "valor cero".
type fileConfig struct {
	Roots           []string          `toml:"roots"`
	RefreshInterval *string           `toml:"refresh_interval"`
	DataDir         *string           `toml:"data_dir"`
	CloneDir        *string           `toml:"clone_dir"`
	WorktreeDir     *string           `toml:"worktree_dir"`
	Forge           *forgeFile        `toml:"forge"`
	Tools           *toolsFile        `toml:"tools"`
	AutoReview      *autoReviewFile   `toml:"autoreview"`
	Keybindings     map[string]string `toml:"keybindings"`
	Commands        map[string]string `toml:"commands"`
}

type forgeFile struct {
	GitHub    *githubFile    `toml:"github"`
	GitLab    *gitlabFile    `toml:"gitlab"`
	Bitbucket *bitbucketFile `toml:"bitbucket"`
}

type githubFile struct {
	Enabled   *bool   `toml:"enabled"`
	Host      *string `toml:"host"`
	CloneBase *string `toml:"clone_base"`
}

type gitlabFile struct {
	Enabled   *bool   `toml:"enabled"`
	Host      *string `toml:"host"`
	APIBase   *string `toml:"api_base"`
	CloneBase *string `toml:"clone_base"`
	TokenEnv  *string `toml:"token_env"`
}

type bitbucketFile struct {
	Enabled *bool `toml:"enabled"`
}

type toolsFile struct {
	Tuicr *string `toml:"tuicr"`
	Hunk  *string `toml:"hunk"`
	Agent *string `toml:"agent"`
	GH    *string `toml:"gh"`
	Glab  *string `toml:"glab"`
}

type autoReviewFile struct {
	Enabled   *bool    `toml:"enabled"`
	Allowlist []string `toml:"allowlist"`
}

// Load lee la config del path estándar XDG. Devuelve la config resuelta y un
// warning (posiblemente vacío) para notificar en la UI. Nunca falla: ante
// cualquier error usa defaults.
func Load() (Config, string) {
	path, err := Path()
	if err != nil {
		return Defaults(), ""
	}
	return LoadFrom(path)
}

// LoadFrom resuelve la config desde un fichero concreto (testeable).
func LoadFrom(path string) (Config, string) {
	cfg := Defaults()

	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, "" // sin fichero, defaults silenciosos
		}
		return cfg, fmt.Sprintf("config: %v", err)
	}

	var fc fileConfig
	if _, err := toml.Decode(string(raw), &fc); err != nil {
		return cfg, fmt.Sprintf("config: %v", err)
	}

	if fc.Roots != nil {
		cfg.Roots = expandAll(fc.Roots) // sustituye, no añade
	}
	if fc.RefreshInterval != nil {
		if d, err := time.ParseDuration(*fc.RefreshInterval); err == nil && d >= 0 {
			cfg.RefreshInterval = d
		}
	}
	if fc.DataDir != nil && *fc.DataDir != "" {
		cfg.DataDir = expand(*fc.DataDir)
	}
	if fc.CloneDir != nil && *fc.CloneDir != "" {
		cfg.CloneDir = expand(*fc.CloneDir)
	}
	if fc.WorktreeDir != nil && *fc.WorktreeDir != "" {
		cfg.WorktreeDir = expand(*fc.WorktreeDir)
	}
	if fc.Forge != nil {
		mergeForges(&cfg.Forges, fc.Forge)
	}
	if fc.Tools != nil {
		mergeString(fc.Tools.Tuicr, &cfg.Tools.Tuicr)
		mergeString(fc.Tools.Hunk, &cfg.Tools.Hunk)
		mergeString(fc.Tools.Agent, &cfg.Tools.Agent)
		mergeString(fc.Tools.GH, &cfg.Tools.GH)
		mergeString(fc.Tools.Glab, &cfg.Tools.Glab)
	}
	if fc.AutoReview != nil {
		if fc.AutoReview.Enabled != nil {
			cfg.AutoReview.Enabled = *fc.AutoReview.Enabled
		}
		if fc.AutoReview.Allowlist != nil {
			cfg.AutoReview.Allowlist = append([]string(nil), fc.AutoReview.Allowlist...)
		}
	}
	// Keybindings: merge sobre defaults (el usuario solo sobreescribe lo que cambia).
	for k, v := range fc.Keybindings {
		if v != "" {
			cfg.Keybindings[k] = v
		}
	}
	// Commands: merge sobre defaults.
	for k, v := range fc.Commands {
		if v != "" {
			cfg.Commands[k] = v
		}
	}
	return cfg, ""
}

// Path devuelve la ruta del fichero de config respetando $XDG_CONFIG_HOME.
func Path() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, DirName, FileName), nil
}

// DefaultKeybindings devuelve el mapa de teclas por defecto.
func DefaultKeybindings() Keybindings {
	return Keybindings{
		"quit":         "q",
		"refresh":      "r",
		"detail":       "enter",
		"mount-review": "m",
		"approve":      "a",
		"merge":        "M",
		"section-next": "tab",
		"open-browser": "o",
	}
}

// DefaultCommands devuelve el argv base por defecto de las CLIs de forge.
func DefaultCommands() Commands {
	return Commands{
		"gh":   "gh",
		"glab": "glab",
	}
}

// Defaults construye la config por defecto.
func Defaults() Config {
	home, _ := os.UserHomeDir()
	share := filepath.Join(home, ".local", "share", "prdash")
	return Config{
		Roots:           expandAll([]string{"~/dev"}),
		RefreshInterval: 60 * time.Second,
		DataDir:         filepath.Join(share, "repos"),
		CloneDir:        filepath.Join(share, "repos"),
		WorktreeDir:     filepath.Join(share, "worktrees"),
		Forges: Forges{
			GitHub:    GitHubConfig{Enabled: true, Host: "github.com"},
			GitLab:    GitLabConfig{Enabled: true, Host: "gitlab.example.com", APIBase: "/git/api/v4/"},
			Bitbucket: BitbucketConfig{Enabled: false},
		},
		Tools: Tools{
			Tuicr: "tuicr",
			Hunk:  "hunk",
			Agent: "opencode",
			GH:    "gh",
			Glab:  "glab",
		},
		AutoReview:  AutoReview{Allowlist: []string{}},
		Keybindings: DefaultKeybindings(),
		Commands:    DefaultCommands(),
	}
}

// KeyFor devuelve la tecla configurada para una acción, o el default.
func (c Config) KeyFor(action string) string {
	if k, ok := c.Keybindings[action]; ok {
		return k
	}
	return DefaultKeybindings()[action]
}

// ActionForKey devuelve la acción configurada para una tecla, o "" si ninguna.
// Se resuelve sobre el mapa ya fusionado (defaults + overrides del usuario).
func (c Config) ActionForKey(key string) string {
	if key == "" {
		return ""
	}
	actions := make([]string, 0, len(c.Keybindings))
	for action := range c.Keybindings {
		actions = append(actions, action)
	}
	sort.Strings(actions)
	for _, action := range actions {
		if c.Keybindings[action] == key {
			return action
		}
	}
	return ""
}

// CmdArgs devuelve el argv base de un comando, separado por espacios. Es API
// reservada para el orquestador F2 (comandos de tuicr/hunk/agente configurables).
func (c Config) CmdArgs(action string) []string {
	raw, ok := c.Commands[action]
	if !ok {
		raw = DefaultCommands()[action]
	}
	return strings.Fields(raw)
}

// ToolArgs devuelve el argv configurable de una herramienta del orquestador.
// Prioriza `commands.<name>` (argv completo, permite flags), luego el binario de
// `tools.<name>` y por último el default del propio nombre. Devuelve nil (pane
// omitido con aviso) si no hay nada configurado.
func (c Config) ToolArgs(name string) []string {
	if raw, ok := c.Commands[name]; ok && strings.TrimSpace(raw) != "" {
		return strings.Fields(raw)
	}
	switch name {
	case "tuicr":
		return fieldsOr(c.Tools.Tuicr, "tuicr")
	case "hunk":
		return fieldsOr(c.Tools.Hunk, "hunk")
	case "agent":
		return fieldsOr(c.Tools.Agent, "opencode")
	}
	return nil
}

// fieldsOr parte raw en argv; si está vacío usa el fallback.
func fieldsOr(raw, fallback string) []string {
	if strings.TrimSpace(raw) == "" {
		return strings.Fields(fallback)
	}
	return strings.Fields(raw)
}

// hintLabels es la etiqueta corta de cada acción en la barra de hints.
var hintLabels = map[string]string{
	"refresh":      "r refresh",
	"detail":       "enter detail",
	"mount-review": "m mount review",
	"approve":      "a approve",
	"merge":        "M merge",
	"section-next": "tab section",
	"open-browser": "o open",
	"quit":         "q quit",
}

// HintBarLines devuelve las líneas de hints, separadas por " · ", derivadas
// de los keybindings configurados. Es API reservada: la TUI compone hoy su
// barra con las acciones realmente disponibles.
func (c Config) HintBarLines() []string {
	first := []string{"j/k move"}
	second := []string{}
	for _, action := range []string{
		"section-next", "refresh", "detail", "mount-review", "approve", "merge", "open-browser", "quit",
	} {
		key, ok := c.Keybindings[action]
		if !ok {
			continue
		}
		label, ok := hintLabels[action]
		if !ok {
			label = key
		}
		hint := key + " " + strings.TrimPrefix(label, key+" ")
		if action == "section-next" {
			first = append(first, hint)
			continue
		}
		second = append(second, hint)
	}
	return []string{strings.Join(first, " · "), strings.Join(second, " · ")}
}

// expandAll expande `~/` a home en cada entrada.
func expandAll(in []string) []string {
	out := make([]string, 0, len(in))
	for _, p := range in {
		out = append(out, expand(p))
	}
	return out
}

// expand expande un `~/` inicial a home. Si no se puede resolver home, deja
// la entrada tal cual.
func expand(p string) string {
	if len(p) < 2 || p[0] != '~' || (p[1] != '/' && p[1] != filepath.Separator) {
		return p
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return p
	}
	return filepath.Join(home, p[2:])
}

func mergeString(src *string, dst *string) {
	if src != nil && *src != "" {
		*dst = *src
	}
}

// mergeForges aplica los overrides del TOML sobre los defaults de forges.
func mergeForges(dst *Forges, src *forgeFile) {
	if src.GitHub != nil {
		if src.GitHub.Enabled != nil {
			dst.GitHub.Enabled = *src.GitHub.Enabled
		}
		mergeString(src.GitHub.Host, &dst.GitHub.Host)
		mergeString(src.GitHub.CloneBase, &dst.GitHub.CloneBase)
	}
	if src.GitLab != nil {
		if src.GitLab.Enabled != nil {
			dst.GitLab.Enabled = *src.GitLab.Enabled
		}
		mergeString(src.GitLab.Host, &dst.GitLab.Host)
		mergeString(src.GitLab.APIBase, &dst.GitLab.APIBase)
		mergeString(src.GitLab.CloneBase, &dst.GitLab.CloneBase)
		mergeString(src.GitLab.TokenEnv, &dst.GitLab.TokenEnv)
	}
	if src.Bitbucket != nil && src.Bitbucket.Enabled != nil {
		dst.Bitbucket.Enabled = *src.Bitbucket.Enabled
	}
}
