package tui

import (
	"strings"
	"testing"

	"prdash/internal/forge/model"
)

// The merge confirmation is the warning that has to list the modes.
func TestMergeConfirmNamesTheModesTheRepoAllows(t *testing.T) {
	base := func(t *testing.T, rules model.MergeRules) Model {
		m2 := newTestModel(t, ghAdapter())
		it := mkItem("github", "github.com", "acme/widget", "One", 1, "")
		it.Merge = rules
		m2 = send(t, m2, page(1, "github", "github.com", model.SectionReview, model.ReviewRequested, []model.Item{it}, false))
		return m2
	}

	m := base(t, model.MergeRulesAll())
	txt := stripANSI(m.mergeConfirmText())
	for _, want := range []string{"press the mode", "esc cancel"} {
		if !strings.Contains(txt, want) {
			t.Errorf("the box should say %q: %q", want, txt)
		}
	}
	for _, want := range []string{"r rebase", "m merge commit", "s squash"} {
		if !strings.Contains(txt, want) {
			t.Errorf("with the three modes it should come out %q: %q", want, txt)
		}
	}
	if n := strings.Count(txt, "·"); n != 3 {
		t.Errorf("with three modes there are three separators, got %d: %q", n, txt)
	}

	// One known mode: ONLY that one.
	m = base(t, model.MergeRules{Known: true, MergeCommit: true})
	txt = stripANSI(m.mergeConfirmText())
	if !strings.Contains(txt, "m merge commit") {
		t.Errorf("with a single known mode that one should come out: %q", txt)
	}
	for _, no := range []string{"rebase", "squash"} {
		if strings.Contains(txt, no) {
			t.Errorf("a repository that only allows merge commit should not offer %q: %q", no, txt)
		}
	}

	// Unknown rules offer all three: not knowing is not the same as forbidding.
	m = base(t, model.MergeRules{})
	txt = stripANSI(m.mergeConfirmText())
	for _, want := range []string{"rebase", "merge commit", "squash"} {
		if !strings.Contains(txt, want) {
			t.Errorf("with no known rules the three should be offered: %q", txt)
		}
	}

	m = base(t, model.MergeRules{Known: true})
	txt = stripANSI(m.mergeConfirmText())
	if !strings.Contains(txt, "the repository allows no merge strategy") {
		t.Errorf("a repo with no strategies should say so: %q", txt)
	}
	m = base(t, model.MergeRulesAll())
	txt = stripANSI(m.mergeConfirmText())
	if !strings.Contains(txt, "merge acme") {
		t.Errorf("the box should name the item: %q", txt)
	}
	if !strings.Contains(txt, "delete") {
		t.Errorf("the box should say whether the branch is deleted: %q", txt)
	}
}

func TestMergeConfirmSaysSoftBlockBeforehand(t *testing.T) {
	m := newTestModel(t, ghAdapter())
	it := mkItem("github", "github.com", "acme/widget", "One", 1, "")
	m = send(t, m, page(1, "github", "github.com", model.SectionReview, model.ReviewRequested, []model.Item{it}, false))

	withoutBlock := stripANSI(m.mergeConfirmText())
	if strings.Contains(withoutBlock, "anyway") {
		t.Errorf("with no block it should not say \"anyway\": %q", withoutBlock)
	}

	m.mergeBlockReason = "CI is failing (2 of 5 checks)"
	withBlock := stripANSI(m.mergeConfirmText())
	if !strings.Contains(withBlock, "CI is failing (2 of 5 checks)") {
		t.Errorf("the block notice should come out: %q", withBlock)
	}
	if !strings.Contains(withBlock, "anyway") {
		t.Errorf("with a soft block it should say you can still press: %q", withBlock)
	}
	if !strings.Contains(withBlock, "esc cancel") {
		t.Errorf("with a block there is still output: %q", withBlock)
	}
	if strings.Index(withBlock, "CI is failing") > strings.Index(withBlock, "press the mode") {
		t.Errorf("the notice should come before the keys: %q", withBlock)
	}
}

// The ITEM column's prefix is the last directory component's parent.
func TestSectionPrefixDoesNotEatTheLastPathPart(t *testing.T) {
	cases := []struct {
		name  string
		projs []string
		want  string
	}{
		{"same repo", []string{"acme/widget", "acme/widget"}, "acme"},
		{"common group", []string{"grp/proj", "grp/other"}, "grp"},
		{"common group of three", []string{"a/b", "a/c", "a/d"}, "a"},
		{"subgroups distintos", []string{"grp/sub/proj", "grp/other/proj"}, "grp"},
		{"different groups", []string{"acme/widget", "other/widget"}, ""},
		{"one without slash", []string{"widget", "acme/widget"}, ""},
		{"only one", []string{"acme/widget"}, ""},
		{"nothing in common among three", []string{"a/x", "b/y", "c/z"}, ""},
		{"only the sheet is equal", []string{"x/widget", "y/widget"}, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			items := make([]model.Item, 0, len(c.projs))
			for i, proj := range c.projs {
				items = append(items, model.NewItem(model.RepoRef{Forge: "github", Host: "github.com", Project: proj}, i+1))
			}
			if got := sectionPrefix(items); got != c.want {
				t.Errorf("sectionPrefix(%v) = %q, want %q", c.projs, got, c.want)
			}
		})
	}
	if got := sectionPrefix(nil); got != "" {
		t.Errorf("sectionPrefix(nil) = %q, want \"\"", got)
	}
	if got := sectionPrefix([]model.Item{}); got != "" {
		t.Errorf("sectionPrefix([]) = %q, want \"\"", got)
	}
}

// The hints are the help and cannot take over the screen.
func TestWrapHintBoundsTheNumberOfLines(t *testing.T) {
	plain := func(s string) string { return stripANSI(s) }

	got := wrapHint("one", 80, plain)
	if len(got) != 1 || got[0] != "one" {
		t.Errorf("a hint gave %q, want [\"one\"]", got)
	}

	for _, w := range []int{3, 4, 10} {
		got = wrapHint("one two", w, plain)
		for _, l := range got {
			if len(l) > w {
				t.Errorf("width %d: the line %q overflows", w, l)
			}
		}
	}

	long := strings.TrimSpace(strings.Repeat("palabra ", 200))
	got = wrapHint(long, 400, plain)
	if len(got) > maxHintLines {
		t.Errorf("a huge hint gave %d lines, want <= %d", len(got), maxHintLines)
	}
	if len(got) > 0 && !strings.HasPrefix(long, got[0]) {
		t.Errorf("the clipping should cut from the end, the first line is %q", got[0])
	}

	got = wrapHint("one two three", 4, func(s string) string { return "[" + s + "]" })
	for _, l := range got {
		if !strings.HasPrefix(l, "[") || !strings.HasSuffix(l, "]") {
			t.Errorf("the line is not dressed: %q", l)
		}
	}

	if got = wrapHint("", 20, plain); len(got) != 1 {
		t.Errorf("empty text gave %q, want one line", got)
	}
}
