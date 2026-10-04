package main

import (
	"strings"
	"testing"

	"prdash/internal/config"
	"prdash/internal/forge/model"
	"prdash/internal/review/plan"
)

func TestPaneToolUsesVerbatimOverride(t *testing.T) {
	cfg := config.Defaults()
	cfg.Commands["hunk"] = "hunk diff develop...HEAD --watch"

	tool := paneTool(cfg, "hunk")
	if !tool.Override {
		t.Fatalf("hunk debería resolverse como override: %+v", tool)
	}
	if got := strings.Join(tool.Argv, " "); got != "hunk diff develop...HEAD --watch" {
		t.Fatalf("argv = %q", got)
	}
}

func TestPaneToolFallsBackToToolsBase(t *testing.T) {
	cfg := config.Defaults()
	cfg.Tools.Hunk = "myhunk"

	tool := paneTool(cfg, "hunk")
	if tool.Override {
		t.Fatalf("sin `[commands]` no debería marcarse override: %+v", tool)
	}
	if got := strings.Join(tool.Argv, " "); got != "myhunk" {
		t.Fatalf("argv = %q", got)
	}
}

func TestToolAvailabilityReportsMissingBinary(t *testing.T) {
	tools := plan.Tools{
		Tuicr:  plan.Tool{Argv: []string{"tuicr"}},
		Hunk:   plan.Tool{Argv: []string{"definitely-not-a-real-binary-xyz"}},
		Agent:  plan.Tool{Argv: []string{"opencode"}},
		Editor: plan.Tool{Argv: []string{"vi"}},
	}
	if toolAvailability(tools)[string(plan.KindHunk)] {
		t.Fatal("un binario ausente debería reportarse como no disponible")
	}
}

func TestToolAvailabilityDoesNotGateTheEditor(t *testing.T) {
	tools := plan.Tools{
		Tuicr:  plan.Tool{Argv: []string{"tuicr"}},
		Hunk:   plan.Tool{Argv: []string{"hunk"}},
		Agent:  plan.Tool{Argv: []string{"opencode"}},
		Editor: plan.Tool{Argv: []string{"vi"}},
	}
	if _, ok := toolAvailability(tools)[string(plan.KindEditor)]; ok {
		t.Fatal("el editor no debería pasar por el chequeo de disponibilidad")
	}

	pl := plan.Build(model.Item{}, plan.Worktree{Path: "/wt"}, tools, plan.Env{Available: map[string]bool{}})
	if pl.PaneCount() != 1 {
		t.Fatalf("panes = %d, quiero solo el editor: %+v", pl.PaneCount(), pl)
	}
	if len(pl.Tabs) != 1 || pl.Tabs[0].Panes[0].Kind != plan.KindEditor {
		t.Fatalf("tabs = %+v", pl.Tabs)
	}
}

func TestClonePrefixesOf(t *testing.T) {
	cfg := config.Config{
		Forges: config.Forges{
			GitHub: config.GitHubConfig{Host: "github.com"},
			GitLab: config.GitLabConfig{Host: "gitlab.example.com", APIBase: "/git/api/v4/"},
		},
	}
	got := clonePrefixesOf(cfg)
	if len(got) != 1 || got["gitlab.example.com"] != "git" {
		t.Fatalf("prefixes = %v", got)
	}
	if _, ok := got["github.com"]; ok {
		t.Fatalf("github no debería tener prefijo: %v", got)
	}
}

func TestClonePrefixesOfHonorsExplicitOverride(t *testing.T) {
	cfg := config.Config{
		Forges: config.Forges{
			GitHub: config.GitHubConfig{Host: "github.enterprise.com", CloneBase: "/ent/"},
			GitLab: config.GitLabConfig{Host: "gitlab.example.com", APIBase: "/api/v4/"},
		},
	}
	got := clonePrefixesOf(cfg)
	if got["github.enterprise.com"] != "ent" {
		t.Fatalf("github enterprise = %v", got)
	}
	if _, ok := got["gitlab.example.com"]; ok {
		t.Fatalf("gitlab raíz no debería tener prefijo: %v", got)
	}
}
