package plan

import (
	"strconv"
	"strings"

	"prdash/internal/forge/model"
)

type Kind string

const (
	KindTuicr  Kind = "tuicr"
	KindHunk   Kind = "hunk"
	KindAgent  Kind = "agent"
	KindEditor Kind = "editor"
)

const (
	DirReuse = ""
	DirRight = "right"
	DirDown  = "down"
)

const (
	LabelReview = "Review"
	LabelEdit   = "Edit"
)

type Pane struct {
	Kind  Kind
	Label string
	Dir   string
	Cwd   string
	Argv  []string
	Env   []string
}

type Tab struct {
	Label string
	Panes []Pane
}

func (t Tab) Cwd() string {
	if len(t.Panes) == 0 {
		return ""
	}
	return t.Panes[0].Cwd
}

type Plan struct {
	Tabs     []Tab
	Warnings []string
}

func (p Plan) PaneCount() int {
	n := 0
	for _, t := range p.Tabs {
		n += len(t.Panes)
	}
	return n
}

type Worktree struct {
	Path   string
	Branch string
	Label  string
}

const (
	defaultTuicrBin = "tuicr"
	defaultHunkBin  = "hunk"
	defaultAgentBin = "opencode"
	defaultEditBin  = "vi"
)

type Tool struct {
	Argv     []string
	Override bool
}

type Tools struct {
	Tuicr  Tool
	Hunk   Tool
	Agent  Tool
	Editor Tool
}

func (t Tools) Binary(kind Kind) string {
	switch kind {
	case KindTuicr:
		return t.Tuicr.binary(defaultTuicrBin)
	case KindHunk:
		return t.Hunk.binary(defaultHunkBin)
	case KindAgent:
		return t.Agent.binary(defaultAgentBin)
	case KindEditor:
		return t.Editor.binary(defaultEditBin)
	}
	return ""
}

func (t Tool) effective(defaultBin string, extra ...string) []string {
	if t.Override {
		return append([]string(nil), t.Argv...)
	}
	base := t.Argv
	if len(base) == 0 {
		base = []string{defaultBin}
	}
	argv := make([]string, 0, len(base)+len(extra))
	argv = append(argv, base...)
	argv = append(argv, extra...)
	return argv
}

func (t Tool) binary(defaultBin string) string {
	if len(t.Argv) > 0 {
		return t.Argv[0]
	}
	if t.Override {
		return ""
	}
	return defaultBin
}

type Env struct {
	Available map[string]bool
}

func Build(pr model.Item, wt Worktree, tools Tools, env Env) Plan {
	var p Plan

	compose := func(kind Kind, label, dir string, tool Tool, defaultBin string, extra ...string) (Pane, bool) {
		argv := tool.effective(defaultBin, extra...)
		if len(argv) == 0 || strings.TrimSpace(argv[0]) == "" {
			p.Warnings = append(p.Warnings, label+" has no configured command: pane omitted")
			return Pane{}, false
		}
		return Pane{
			Kind:  kind,
			Label: label,
			Dir:   dir,
			Cwd:   wt.Path,
			Argv:  argv,
			Env:   paneEnv(pr, wt),
		}, true
	}

	add := func(tab *Tab, kind Kind, label, dir string, tool Tool, defaultBin string, extra ...string) {
		if !env.available(string(kind)) {
			p.Warnings = append(p.Warnings, label+" is not installed: pane omitted")
			return
		}
		if pane, ok := compose(kind, label, dir, tool, defaultBin, extra...); ok {
			tab.Panes = append(tab.Panes, pane)
		}
	}

	review := Tab{Label: LabelReview}
	add(&review, KindTuicr, "TUICR", DirReuse, tools.Tuicr, defaultTuicrBin, "pr", ReviewTarget(pr))
	if editor, ok := compose(KindEditor, "Editor", DirRight, tools.Editor, defaultEditBin); ok {
		review.Panes = append(review.Panes, editor)
	}
	edit := Tab{Label: LabelEdit}
	add(&edit, KindHunk, "Hunk", DirReuse, tools.Hunk, defaultHunkBin, "diff")
	add(&edit, KindAgent, "Agent", DirRight, tools.Agent, defaultAgentBin)
	p.Tabs = nonEmpty(review, edit)
	return p
}

func nonEmpty(tabs ...Tab) []Tab {
	out := make([]Tab, 0, len(tabs))
	for _, t := range tabs {
		if len(t.Panes) > 0 {
			out = append(out, t)
		}
	}
	return out
}

func ReviewTarget(pr model.Item) string {
	if pr.URL != "" {
		return pr.URL
	}
	return pr.Ref.Project + "#" + strconv.Itoa(pr.Number)
}

func (e Env) available(kind string) bool {
	if e.Available == nil {
		return true
	}
	return e.Available[kind]
}

func paneEnv(pr model.Item, wt Worktree) []string {
	env := []string{
		"PRDASH_REPO=" + pr.Ref.Project,
		"PRDASH_NUMBER=" + strconv.Itoa(pr.Number),
		"PRDASH_WORKTREE=" + wt.Path,
	}
	if wt.Branch != "" {
		env = append(env, "PRDASH_BRANCH="+wt.Branch)
	}
	if pr.TargetBranch != "" {
		env = append(env, "PRDASH_BASE="+pr.TargetBranch)
	}
	if pr.URL != "" {
		env = append(env, "PRDASH_URL="+pr.URL)
	}
	return env
}
