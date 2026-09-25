package plan

import (
	"strings"
	"testing"

	"prdash/internal/forge/model"
)

func sampleItem() model.Item {
	ref := model.RepoRef{Forge: "github", Host: "github.com", Project: "acme/widget", Owner: "acme", Name: "widget"}
	it := model.NewItem(ref, 7)
	it.URL = "https://github.com/acme/widget/pull/7"
	return it
}

func sampleTools() Tools {
	return Tools{Tuicr: []string{"tuicr"}, Hunk: []string{"hunk"}, Agent: []string{"opencode"}}
}

func sampleWorktree() Worktree {
	return Worktree{Path: "/tmp/wt/prdash-pr-7", Branch: "prdash/pr-7", Label: "prdash-pr-7"}
}

func TestBuildAllToolsPresent(t *testing.T) {
	p := Build(sampleItem(), sampleWorktree(), sampleTools(), Env{})

	if len(p.Warnings) != 0 {
		t.Fatalf("sin avisos esperados, got %v", p.Warnings)
	}
	if len(p.Panes) != 3 {
		t.Fatalf("panes = %d, quiero 3", len(p.Panes))
	}
	tuicr := p.Panes[0]
	if tuicr.Kind != KindTuicr || tuicr.Label != "TUICR" {
		t.Fatalf("pane TUICR = %+v", tuicr)
	}
	if tuicr.Cwd != sampleWorktree().Path {
		t.Errorf("cwd = %q, quiero el worktree", tuicr.Cwd)
	}
	wantArgv := []string{"tuicr", "pr", "https://github.com/acme/widget/pull/7"}
	if strings.Join(tuicr.Argv, " ") != strings.Join(wantArgv, " ") {
		t.Errorf("argv TUICR = %v, quiero %v", tuicr.Argv, wantArgv)
	}
	if !contains(tuicr.Env, "PRDASH_WORKTREE="+sampleWorktree().Path) {
		t.Errorf("env sin PRDASH_WORKTREE: %v", tuicr.Env)
	}
	if p.Panes[1].Kind != KindHunk || p.Panes[2].Kind != KindAgent {
		t.Errorf("orden de panes = %v", p.Panes)
	}
}

func TestBuildMissingBinaryOmitsPane(t *testing.T) {
	env := Env{Available: map[string]bool{"tuicr": true, "agent": true}}
	p := Build(sampleItem(), sampleWorktree(), sampleTools(), env)

	if len(p.Panes) != 2 {
		t.Fatalf("panes = %d, quiero 2", len(p.Panes))
	}
	for _, pane := range p.Panes {
		if pane.Kind == KindHunk {
			t.Fatal("el pane de Hunk debería omitirse")
		}
	}
	if !hasWarning(p.Warnings, "Hunk") {
		t.Fatalf("falta el aviso de Hunk: %v", p.Warnings)
	}
}

func TestBuildEmptyArgvOmitsPane(t *testing.T) {
	tools := sampleTools()
	tools.Agent = nil
	p := Build(sampleItem(), sampleWorktree(), tools, Env{})

	if len(p.Panes) != 2 {
		t.Fatalf("panes = %d, quiero 2", len(p.Panes))
	}
	if !hasWarning(p.Warnings, "Agente") {
		t.Fatalf("falta el aviso del agente: %v", p.Warnings)
	}
}

func TestReviewTargetFallsBackToProjectNumber(t *testing.T) {
	it := sampleItem()
	it.URL = ""
	if got := ReviewTarget(it); got != "acme/widget#7" {
		t.Fatalf("ReviewTarget = %q", got)
	}
}

func contains(list []string, want string) bool {
	for _, s := range list {
		if s == want {
			return true
		}
	}
	return false
}

func hasWarning(warns []string, needle string) bool {
	for _, w := range warns {
		if strings.Contains(w, needle) {
			return true
		}
	}
	return false
}
