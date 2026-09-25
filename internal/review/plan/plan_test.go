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
	return Tools{
		Tuicr: Tool{Argv: []string{"tuicr"}},
		Hunk:  Tool{Argv: []string{"hunk"}},
		Agent: Tool{Argv: []string{"opencode"}},
	}
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
	tools.Agent = Tool{Override: true} // override vacío = sin comando
	p := Build(sampleItem(), sampleWorktree(), tools, Env{})

	if len(p.Panes) != 2 {
		t.Fatalf("panes = %d, quiero 2", len(p.Panes))
	}
	if !hasWarning(p.Warnings, "Agente") {
		t.Fatalf("falta el aviso del agente: %v", p.Warnings)
	}
}

// El pane de Hunk debe mostrar el diff del PR, no exportar una sesión viva.
func TestBuildHunkDiffsTargetBranch(t *testing.T) {
	it := sampleItem()
	it.TargetBranch = "main"
	p := Build(it, sampleWorktree(), sampleTools(), Env{})

	hunk, ok := paneOfKind(p, KindHunk)
	if !ok {
		t.Fatalf("falta el pane de Hunk: %+v", p.Panes)
	}
	want := []string{"hunk", "diff", "main...HEAD"}
	if strings.Join(hunk.Argv, " ") != strings.Join(want, " ") {
		t.Fatalf("argv Hunk = %v, quiero %v", hunk.Argv, want)
	}
	if contains(hunk.Argv, "session") {
		t.Fatalf("el pane de Hunk no debe usar `session review`: %v", hunk.Argv)
	}
}

func TestBuildHunkWithoutTargetBranchFallsBackToWorkingTree(t *testing.T) {
	it := sampleItem() // sin TargetBranch
	p := Build(it, sampleWorktree(), sampleTools(), Env{})

	hunk, ok := paneOfKind(p, KindHunk)
	if !ok {
		t.Fatalf("falta el pane de Hunk: %+v", p.Panes)
	}
	want := []string{"hunk", "diff"}
	if strings.Join(hunk.Argv, " ") != strings.Join(want, " ") {
		t.Fatalf("argv Hunk = %v, quiero %v", hunk.Argv, want)
	}
}

func TestBuildInjectsBaseEnv(t *testing.T) {
	it := sampleItem()
	it.TargetBranch = "develop"
	p := Build(it, sampleWorktree(), sampleTools(), Env{})

	for _, pane := range p.Panes {
		if !contains(pane.Env, "PRDASH_BASE=develop") {
			t.Fatalf("pane %s sin PRDASH_BASE=develop: %v", pane.Kind, pane.Env)
		}
	}
}

func paneOfKind(p Plan, kind Kind) (Pane, bool) {
	for _, pane := range p.Panes {
		if pane.Kind == kind {
			return pane, true
		}
	}
	return Pane{}, false
}

// Un override de `[commands]` es el argv completo del pane: verbatim, sin
// añadirle los argumentos por defecto del ítem.
func TestBuildOverrideArgvVerbatim(t *testing.T) {
	cases := []struct {
		name  string
		tools Tools
		kind  Kind
		want  []string
	}{
		{
			name:  "tuicr",
			tools: Tools{Tuicr: Tool{Argv: []string{"tuicr", "pr", "--url", "x"}, Override: true}},
			kind:  KindTuicr,
			want:  []string{"tuicr", "pr", "--url", "x"},
		},
		{
			name:  "hunk",
			tools: Tools{Hunk: Tool{Argv: []string{"hunk", "diff", "develop...HEAD", "--watch"}, Override: true}},
			kind:  KindHunk,
			want:  []string{"hunk", "diff", "develop...HEAD", "--watch"},
		},
		{
			name:  "agent",
			tools: Tools{Agent: Tool{Argv: []string{"claude", "--model", "opus"}, Override: true}},
			kind:  KindAgent,
			want:  []string{"claude", "--model", "opus"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := Build(sampleItem(), sampleWorktree(), tc.tools, Env{})
			pane, ok := paneOfKind(p, tc.kind)
			if !ok {
				t.Fatalf("falta el pane %s: %+v", tc.kind, p.Panes)
			}
			if strings.Join(pane.Argv, " ") != strings.Join(tc.want, " ") {
				t.Fatalf("argv = %v, quiero %v (verbatim, sin extras)", pane.Argv, tc.want)
			}
		})
	}
}

func TestToolsBinaryPrefersOverride(t *testing.T) {
	tools := Tools{
		Tuicr: Tool{Argv: []string{"mytui", "--x"}, Override: true},
		Hunk:  Tool{Argv: []string{"hunk"}},
		Agent: Tool{},
	}
	if got := tools.Binary(KindTuicr); got != "mytui" {
		t.Fatalf("Binary(Tuicr) = %q", got)
	}
	if got := tools.Binary(KindHunk); got != "hunk" {
		t.Fatalf("Binary(Hunk) = %q", got)
	}
	if got := tools.Binary(KindAgent); got != "opencode" {
		t.Fatalf("Binary(Agent) = %q", got)
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
