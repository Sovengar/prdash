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
		Tuicr:  Tool{Argv: []string{"tuicr"}},
		Hunk:   Tool{Argv: []string{"hunk"}},
		Agent:  Tool{Argv: []string{"opencode"}},
		Editor: Tool{Argv: []string{"vi"}},
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
	if len(p.Tabs) != 2 || p.Tabs[0].Label != LabelReview || p.Tabs[1].Label != LabelEdit {
		t.Fatalf("tabs = %+v, quiero Review y Edit", p.Tabs)
	}
	if p.PaneCount() != 4 {
		t.Fatalf("panes = %d, quiero 4", p.PaneCount())
	}

	review, edit := p.Tabs[0].Panes, p.Tabs[1].Panes
	if len(review) != 2 || len(edit) != 2 {
		t.Fatalf("panes por tab = %d/%d, quiero 2/2", len(review), len(edit))
	}

	tuicr := review[0]
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

	// El editor acompaña a la review en su tab; el diff y el agente comparten el
	// de trabajo.
	if review[1].Kind != KindEditor {
		t.Errorf("segundo pane de Review = %v, quiero el editor", review[1].Kind)
	}
	if edit[0].Kind != KindHunk || edit[1].Kind != KindAgent {
		t.Errorf("panes de Edit = %v", edit)
	}
}

// Los panes que no son el primero de su tab se abren con una división a la
// derecha: el segundo se lee junto a lo que comenta.
func TestBuildSplitsPanesToTheRight(t *testing.T) {
	p := Build(sampleItem(), sampleWorktree(), sampleTools(), Env{})

	for _, tab := range p.Tabs {
		for i, pane := range tab.Panes {
			want := DirRight
			if i == 0 {
				want = DirReuse
			}
			if pane.Dir != want {
				t.Errorf("pane %s del tab %s: Dir = %q, quiero %q", pane.Kind, tab.Label, pane.Dir, want)
			}
		}
	}
}

// El editor no se omite aunque no se pueda probar su disponibilidad: su orden
// puede ser una función del shell (`vi` -> `nvim .`) que no está en el PATH.
func TestBuildKeepsEditorWithoutAvailability(t *testing.T) {
	p := Build(sampleItem(), sampleWorktree(), sampleTools(), Env{Available: map[string]bool{}})

	editor, ok := paneOfKind(p, KindEditor)
	if !ok {
		t.Fatalf("el editor no debería omitirse nunca: %+v", p)
	}
	if strings.Join(editor.Argv, " ") != "vi" {
		t.Fatalf("argv del editor = %v", editor.Argv)
	}
	if _, ok := paneOfKind(p, KindTuicr); ok {
		t.Fatal("una herramienta sin binario sí debe omitirse")
	}
}

// Un tab que se queda sin panes no se monta: abrir una pestaña en blanco es
// ruido que el usuario tendría que cerrar a mano. El tab de Edit es el único que
// puede vaciarse, porque el de Review siempre conserva su editor.
func TestBuildDropsEmptyTab(t *testing.T) {
	env := Env{Available: map[string]bool{"tuicr": true}}
	p := Build(sampleItem(), sampleWorktree(), sampleTools(), env)

	if len(p.Tabs) != 1 || p.Tabs[0].Label != LabelReview {
		t.Fatalf("tabs = %+v, quiero solo Review", p.Tabs)
	}
}

func TestBuildMissingBinaryOmitsPane(t *testing.T) {
	env := Env{Available: map[string]bool{"tuicr": true, "agent": true}}
	p := Build(sampleItem(), sampleWorktree(), sampleTools(), env)

	if p.PaneCount() != 3 {
		t.Fatalf("panes = %d, quiero 3", p.PaneCount())
	}
	if _, ok := paneOfKind(p, KindHunk); ok {
		t.Fatal("el pane de Hunk debería omitirse")
	}
	if !hasWarning(p.Warnings, "Hunk") {
		t.Fatalf("falta el aviso de Hunk: %v", p.Warnings)
	}
}

func TestBuildEmptyArgvOmitsPane(t *testing.T) {
	tools := sampleTools()
	tools.Agent = Tool{Override: true} // override vacío = sin comando
	p := Build(sampleItem(), sampleWorktree(), tools, Env{})

	if _, ok := paneOfKind(p, KindAgent); ok {
		t.Fatal("el pane del agente debería omitirse")
	}
	if len(p.Tabs[1].Panes) != 1 {
		t.Fatalf("el tab de Edit se queda con Hunk solo: %+v", p.Tabs[1])
	}
	if !hasWarning(p.Warnings, "Agente") {
		t.Fatalf("falta el aviso del agente: %v", p.Warnings)
	}
}

// El pane de Hunk muestra el diff del WORKING TREE, no el del PR/MR. El pane
// vive al lado del editor y del agente: lo que se revisa es lo que se está
// tocando, y el diff del PR es un objetivo fijo que no se mueve mientras
// editas. El diff del PR sigue disponible por `[commands].hunk`.
func TestBuildHunkDiffsTheWorkingTree(t *testing.T) {
	it := sampleItem()
	it.TargetBranch = "main"
	p := Build(it, sampleWorktree(), sampleTools(), Env{})

	hunk, ok := paneOfKind(p, KindHunk)
	if !ok {
		t.Fatalf("falta el pane de Hunk: %+v", p)
	}
	want := []string{"hunk", "diff"}
	if strings.Join(hunk.Argv, " ") != strings.Join(want, " ") {
		t.Fatalf("argv Hunk = %v, quiero %v (working tree, sin revspec)", hunk.Argv, want)
	}
}

// La rama destino sigue llegando al pane por el entorno, aunque ya no sea un
// argumento del diff: el agente la necesita para push/PR.
func TestBuildInjectsBaseEnv(t *testing.T) {
	it := sampleItem()
	it.TargetBranch = "develop"
	p := Build(it, sampleWorktree(), sampleTools(), Env{})

	for _, tab := range p.Tabs {
		for _, pane := range tab.Panes {
			if !contains(pane.Env, "PRDASH_BASE=develop") {
				t.Fatalf("pane %s sin PRDASH_BASE=develop: %v", pane.Kind, pane.Env)
			}
		}
	}
}

func TestTabCwdIsTheFirstPaneCwd(t *testing.T) {
	p := Build(sampleItem(), sampleWorktree(), sampleTools(), Env{})

	for _, tab := range p.Tabs {
		if tab.Cwd() != sampleWorktree().Path {
			t.Fatalf("cwd del tab %s = %q, quiero el worktree", tab.Label, tab.Cwd())
		}
	}
	if (Tab{}).Cwd() != "" {
		t.Fatal("un tab sin panes no tiene cwd")
	}
}

func paneOfKind(p Plan, kind Kind) (Pane, bool) {
	for _, tab := range p.Tabs {
		for _, pane := range tab.Panes {
			if pane.Kind == kind {
				return pane, true
			}
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
		{
			name:  "editor",
			tools: Tools{Editor: Tool{Argv: []string{"nvim", "README.md"}, Override: true}},
			kind:  KindEditor,
			want:  []string{"nvim", "README.md"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := Build(sampleItem(), sampleWorktree(), tc.tools, Env{})
			pane, ok := paneOfKind(p, tc.kind)
			if !ok {
				t.Fatalf("falta el pane %s: %+v", tc.kind, p)
			}
			if strings.Join(pane.Argv, " ") != strings.Join(tc.want, " ") {
				t.Fatalf("argv = %v, quiero %v (verbatim, sin extras)", pane.Argv, tc.want)
			}
		})
	}
}

func TestToolsBinaryPrefersOverride(t *testing.T) {
	tools := Tools{
		Tuicr:  Tool{Argv: []string{"mytui", "--x"}, Override: true},
		Hunk:   Tool{Argv: []string{"hunk"}},
		Agent:  Tool{},
		Editor: Tool{Argv: []string{"vi"}},
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
	if got := tools.Binary(KindEditor); got != "vi" {
		t.Fatalf("Binary(Editor) = %q", got)
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
