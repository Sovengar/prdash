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

const FileName = "config.toml"

const DirName = "prdash"

type Keybindings map[string]string

// A pane key (`tuicr`, `hunk`, `agent`, `editor`) takes the value as the pane's COMPLETE argv, verbatim.
type Commands map[string]string

type GitHubConfig struct {
	Enabled   bool
	Host      string
	CloneBase string
}

type GitLabConfig struct {
	Enabled   bool
	Host      string
	APIBase   string
	CloneBase string
	TokenEnv  string // name of the token env var (glab reads it)
}

func (g GitHubConfig) ClonePrefix() string { return normalizeBase(g.CloneBase) }

func (g GitLabConfig) ClonePrefix() string {
	if g.CloneBase != "" {
		return normalizeBase(g.CloneBase)
	}
	return relativeRootFromAPIBase(g.APIBase)
}

func normalizeBase(base string) string { return strings.Trim(base, "/") }

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

type BitbucketConfig struct {
	Enabled bool
}

type Forges struct {
	GitHub    GitHubConfig
	GitLab    GitLabConfig
	Bitbucket BitbucketConfig
}

type Tools struct {
	Tuicr  string
	Hunk   string
	Agent  string
	Editor string
	GH     string
	Glab   string
}

type AutoReview struct {
	Enabled   bool
	Allowlist []string
}

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
	Tuicr  *string `toml:"tuicr"`
	Hunk   *string `toml:"hunk"`
	Agent  *string `toml:"agent"`
	Editor *string `toml:"editor"`
	GH     *string `toml:"gh"`
	Glab   *string `toml:"glab"`
}

type autoReviewFile struct {
	Enabled   *bool    `toml:"enabled"`
	Allowlist []string `toml:"allowlist"`
}

func Load() (Config, string) {
	path, err := Path()
	if err != nil {
		return Defaults(), ""
	}
	return LoadFrom(path)
}

func LoadFrom(path string) (Config, string) {
	cfg := Defaults()

	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, "" // no file: silent defaults
		}
		return cfg, fmt.Sprintf("config: %v", err)
	}

	var fc fileConfig
	if _, err := toml.Decode(string(raw), &fc); err != nil {
		return cfg, fmt.Sprintf("config: %v", err)
	}

	if fc.Roots != nil {
		cfg.Roots = expandAll(fc.Roots) // replaces, does not append
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
		mergeString(fc.Tools.Editor, &cfg.Tools.Editor)
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
	for k, v := range fc.Keybindings {
		if v != "" {
			cfg.Keybindings[k] = v
		}
	}
	for k, v := range fc.Commands {
		if v != "" {
			cfg.Commands[k] = v
		}
	}
	return cfg, ""
}

func Path() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, DirName, FileName), nil
}

func DefaultKeybindings() Keybindings {
	return Keybindings{
		"quit":         "q",
		"refresh":      "R",
		"mount-review": "r",
		"approve":      "a",
		"merge":        "m",
		"simulate":     "v",
		"section-next": "tab",
		"open-browser": "o",
		"prefix-mode":  "p",
		"retarget":     "e",
	}
}

func DefaultCommands() Commands {
	return Commands{
		"gh":   "gh",
		"glab": "glab",
	}
}

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
			GitLab:    GitLabConfig{Enabled: true, Host: "gitlab.example.com", APIBase: "/api/v4/"},
			Bitbucket: BitbucketConfig{Enabled: false},
		},
		Tools: Tools{
			Tuicr:  "tuicr",
			Hunk:   "hunk",
			Agent:  "opencode",
			Editor: "vi",
			GH:     "gh",
			Glab:   "glab",
		},
		AutoReview:  AutoReview{Allowlist: []string{}},
		Keybindings: DefaultKeybindings(),
		Commands:    DefaultCommands(),
	}
}

func (c Config) KeyFor(action string) string {
	if k, ok := c.Keybindings[action]; ok {
		return k
	}
	return DefaultKeybindings()[action]
}

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

func (c Config) CmdArgs(action string) []string {
	raw, ok := c.Commands[action]
	if !ok {
		raw = DefaultCommands()[action]
	}
	return strings.Fields(raw)
}

func (c Config) PaneOverride(name string) ([]string, bool) {
	raw, ok := c.Commands[name]
	if !ok || strings.TrimSpace(raw) == "" {
		return nil, false
	}
	return strings.Fields(raw), true
}

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
	case "editor":
		return fieldsOr(c.Tools.Editor, "vi")
	}
	return nil
}

func fieldsOr(raw, fallback string) []string {
	if strings.TrimSpace(raw) == "" {
		return strings.Fields(fallback)
	}
	return strings.Fields(raw)
}

type hint struct {
	action string
	key    string
	label  string
}

var hintOrder = []hint{
	{action: "quit", label: "quit"},
	{action: "section-next", label: "section"},
	{action: "mount-review", label: "mount review"},
	{action: "approve", label: "approve"},
	{action: "merge", label: "merge ×2"},
	{action: "simulate", label: "simulate"},
	{action: "open-browser", label: "open"},
	{action: "refresh", label: "refresh"},
	{action: "prefix-mode", label: "prefix"},
	{action: "retarget", label: "edit base"},
	{key: "j/k", label: "move"},
	{key: "pgup/dn", label: "page"},
}

type HintState map[string]string

func (c Config) Hints(state HintState) []string {
	out := make([]string, 0, len(hintOrder))
	for _, h := range hintOrder {
		key := h.key
		label := h.label
		if h.action != "" {
			key = c.KeyFor(h.action)
			if dyn, ok := state[h.action]; ok && dyn != "" {
				label += ": " + dyn
			}
		}
		if key == "" {
			continue
		}
		out = append(out, key+" "+label)
	}
	return out
}

func expandAll(in []string) []string {
	out := make([]string, 0, len(in))
	for _, p := range in {
		out = append(out, expand(p))
	}
	return out
}

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
