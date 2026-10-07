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
		t.Fatalf("with no expected warnings, got %v", p.Warnings)
	}
	if len(p.Tabs) != 2 || p.Tabs[0].Label != LabelReview || p.Tabs[1].Label != LabelEdit {
		t.Fatalf("tabs = %+v, want Review and Edit", p.Tabs)
	}
	if p.PaneCount() != 4 {
		t.Fatalf("panes = %d, want 4", p.PaneCount())
	}

	review, edit := p.Tabs[0].Panes, p.Tabs[1].Panes
	if len(review) != 2 || len(edit) != 2 {
		t.Fatalf("panes per tab = %d/%d, want 2/2", len(review), len(edit))
	}

	tuicr := review[0]
	if tuicr.Kind != KindTuicr || tuicr.Label != "TUICR" {
		t.Fatalf("TUICR pane = %+v", tuicr)
	}
	if tuicr.Cwd != sampleWorktree().Path {
		t.Errorf("cwd = %q, want the worktree", tuicr.Cwd)
	}
	wantArgv := []string{"tuicr", "pr", "https://github.com/acme/widget/pull/7"}
	if strings.Join(tuicr.Argv, " ") != strings.Join(wantArgv, " ") {
		t.Errorf("TUICR argv = %v, want %v", tuicr.Argv, wantArgv)
	}
	if !contains(tuicr.Env, "PRDASH_WORKTREE="+sampleWorktree().Path) {
		t.Errorf("env without PRDASH_WORKTREE: %v", tuicr.Env)
	}

	if review[1].Kind != KindEditor {
		t.Errorf("the second Review pane = %v, want the editor", review[1].Kind)
	}
	if edit[0].Kind != KindHunk || edit[1].Kind != KindAgent {
		t.Errorf("Edit panes = %v", edit)
	}
}

func TestBuildSplitsPanesToTheRight(t *testing.T) {
	p := Build(sampleItem(), sampleWorktree(), sampleTools(), Env{})

	for _, tab := range p.Tabs {
		for i, pane := range tab.Panes {
			want := DirRight
			if i == 0 {
				want = DirReuse
			}
			if pane.Dir != want {
				t.Errorf("pane %s of tab %s: Dir = %q, want %q", pane.Kind, tab.Label, pane.Dir, want)
			}
		}
	}
}

// The editor is not omitted even without availability: its ORDER may be useful.
func TestBuildKeepsEditorWithoutAvailability(t *testing.T) {
	p := Build(sampleItem(), sampleWorktree(), sampleTools(), Env{Available: map[string]bool{}})

	editor, ok := paneOfKind(p, KindEditor)
	if !ok {
		t.Fatalf("the editor should never be omitted: %+v", p)
	}
	if strings.Join(editor.Argv, " ") != "vi" {
		t.Fatalf("editor argv = %v", editor.Argv)
	}
	if _, ok := paneOfKind(p, KindTuicr); ok {
		t.Fatal("a tool with no binary must be omitted")
	}
}

// A tab left with no panes is not mounted: opening a blank tab is noise.
func TestBuildDropsEmptyTab(t *testing.T) {
	env := Env{Available: map[string]bool{"tuicr": true}}
	p := Build(sampleItem(), sampleWorktree(), sampleTools(), env)

	if len(p.Tabs) != 1 || p.Tabs[0].Label != LabelReview {
		t.Fatalf("tabs = %+v, want only Review", p.Tabs)
	}
}

func TestBuildMissingBinaryOmitsPane(t *testing.T) {
	env := Env{Available: map[string]bool{"tuicr": true, "agent": true}}
	p := Build(sampleItem(), sampleWorktree(), sampleTools(), env)

	if p.PaneCount() != 3 {
		t.Fatalf("panes = %d, want 3", p.PaneCount())
	}
	if _, ok := paneOfKind(p, KindHunk); ok {
		t.Fatal("the Hunk pane should be omitted")
	}
	if !hasWarning(p.Warnings, "Hunk") {
		t.Fatalf("the Hunk warning is missing: %v", p.Warnings)
	}
}

func TestBuildEmptyArgvOmitsPane(t *testing.T) {
	tools := sampleTools()
	tools.Agent = Tool{Override: true} // empty override = no command
	p := Build(sampleItem(), sampleWorktree(), tools, Env{})

	if _, ok := paneOfKind(p, KindAgent); ok {
		t.Fatal("the agent pane should be omitted")
	}
	if len(p.Tabs[1].Panes) != 1 {
		t.Fatalf("the Edit tab is left with Hunk alone: %+v", p.Tabs[1])
	}
	if !hasWarning(p.Warnings, "Agent") {
		t.Fatalf("the agent warning is missing: %v", p.Warnings)
	}
}

// Hunk's pane shows the WORKING TREE diff, not the PR's: it sits next to the editor.
func TestBuildHunkDiffsTheWorkingTree(t *testing.T) {
	it := sampleItem()
	it.TargetBranch = "main"
	p := Build(it, sampleWorktree(), sampleTools(), Env{})

	hunk, ok := paneOfKind(p, KindHunk)
	if !ok {
		t.Fatalf("the Hunk pane is missing: %+v", p)
	}
	want := []string{"hunk", "diff"}
	if strings.Join(hunk.Argv, " ") != strings.Join(want, " ") {
		t.Fatalf("Hunk argv = %v, want %v (working tree, no revspec)", hunk.Argv, want)
	}
}

// The target branch still reaches the pane through the environment, not as an argument.
func TestBuildInjectsBaseEnv(t *testing.T) {
	it := sampleItem()
	it.TargetBranch = "develop"
	p := Build(it, sampleWorktree(), sampleTools(), Env{})

	for _, tab := range p.Tabs {
		for _, pane := range tab.Panes {
			if !contains(pane.Env, "PRDASH_BASE=develop") {
				t.Fatalf("pane %s without PRDASH_BASE=develop: %v", pane.Kind, pane.Env)
			}
		}
	}
}

func TestTabCwdIsTheFirstPaneCwd(t *testing.T) {
	p := Build(sampleItem(), sampleWorktree(), sampleTools(), Env{})

	for _, tab := range p.Tabs {
		if tab.Cwd() != sampleWorktree().Path {
			t.Fatalf("cwd of tab %s = %q, want the worktree", tab.Label, tab.Cwd())
		}
	}
	if (Tab{}).Cwd() != "" {
		t.Fatal("a tab with no panes has no cwd")
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

// A `[commands]` override is the pane's WHOLE argv: verbatim, with nothing appended.
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
				t.Fatalf("the pane %s is missing: %+v", tc.kind, p)
			}
			if strings.Join(pane.Argv, " ") != strings.Join(tc.want, " ") {
				t.Fatalf("argv = %v, want %v (verbatim, no extras)", pane.Argv, tc.want)
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

func TestBinaryOfAnUnknownKindReturnsEmptyNotAnotherKindsBinary(t *testing.T) {
	tools := Tools{}

	for _, kind := range []Kind{Kind("made-up"), Kind("")} {
		if got := tools.Binary(kind); got != "" {
			t.Errorf("the Kind %q gave binary %q: a pane would be launched with the tool of "+
				"another kind instead of with nothing", string(kind), got)
		}
	}

	known := map[Kind]string{
		KindTuicr:  "tuicr",
		KindHunk:   "hunk",
		KindAgent:  "opencode",
		KindEditor: "vi",
	}
	seen := map[string]Kind{}
	for kind, want := range known {
		got := tools.Binary(kind)
		if got == "" {
			t.Errorf("the Kind %q gave an empty binary: that Kind does have a default", string(kind))
			continue
		}
		if got != want {
			t.Errorf("the Kind %q gave %q, want %q", string(kind), got, want)
		}
		if other, dup := seen[got]; dup {
			t.Errorf("the Kinds %q and %q share binary %q: a `default` that copied another's "+
				"would go unnoticed", string(other), string(kind), got)
		}
		seen[got] = kind
	}

	// A configured Argv replaces its own, not another's: that is what allows one override per tool.
	own := Tools{
		Tuicr:  Tool{Argv: []string{"/opt/mytui", "pr", "7"}},
		Editor: Tool{Argv: []string{"nvim"}},
	}
	// What is compared is the FIRST element of the argv, because that is what is looked up in PATH.
	if got := own.Binary(KindTuicr); got != "/opt/mytui" {
		t.Errorf("the tuicr argv gave %q, want /opt/mytui: it is the first element, which is "+
			"what is checked to decide whether it is available", got)
	}
	if got := own.Binary(KindEditor); got != "nvim" {
		t.Errorf("the editor argv gave %q, want nvim", got)
	}
	if got := own.Binary(KindHunk); got != "hunk" {
		t.Errorf("the hunk Kind changed to %q with argv of other panes set", got)
	}

	// An OVERRIDE with an EMPTY argv is an explicit DISABLE: it returns empty instead of the binary.
	disabled := Tools{Agent: Tool{Override: true}}
	if got := disabled.Binary(KindAgent); got != "" {
		t.Errorf("a disabled pane gave binary %q: it would be launched the same as if it were not "+
			"disabled on purpose", got)
	}
	if got := disabled.Binary(KindEditor); got != "vi" {
		t.Errorf("disabling the agent changed the editor to %q", got)
	}
	disabledWithArgv := Tools{Agent: Tool{Argv: []string{"x"}, Override: true}}
	if got := disabledWithArgv.Binary(KindAgent); got != "x" {
		t.Errorf("with argv present `Override` does not win: gave %q, want x", got)
	}
}
