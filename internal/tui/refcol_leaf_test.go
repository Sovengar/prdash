package tui

import (
	"strings"
	"testing"

	"prdash/internal/forge/model"
)

func TestRefLeafCutsAtTheLastSeparator(t *testing.T) {
	cases := []struct {
		project string
		number  int
		want    string
	}{
		{"acme/widget", 7, "widget#7"},
		{"group/sub/project", 12, "project#12"},
		{"a/b/c/d", 1, "d#1"},
		{"widget", 7, "widget#7"},
		{"", 7, "#7"},
		{"/project", 7, "project#7"},
		{"/a/b", 3, "b#3"},
		{"/", 7, "#7"},
		{"a/", 7, "#7"},
		{"acme/widget", 0, "widget#0"},
		{"acme/widget", -1, "widget#-1"},
		{"acme/widget", 12345, "widget#12345"},
	}

	for _, c := range cases {
		got := refLeaf(model.Item{Number: c.number, Ref: model.RepoRef{Project: c.project}})
		if got != c.want {
			t.Errorf("refLeaf(%q, %d) = %q, want %q", c.project, c.number, got, c.want)
		}
	}

	// And the leaf NEVER comes out with a slash: the leaf of a path is not a path.
	for _, project := range []string{"a", "a/b", "a/b/c", "/a", "/a/b", "/"} {
		sheet := refLeaf(model.Item{Number: 1, Ref: model.RepoRef{Project: project}})
		stay := strings.TrimSuffix(sheet, "#1")
		if strings.Contains(stay, "/") {
			t.Errorf("refLeaf(%q) gave %q: the sheet of a path carries no slashes", project, sheet)
		}
	}

	// And the number is always there, because without it the cell does not identify the item.
	for _, project := range []string{"", "a", "a/b", "/a"} {
		sheet := refLeaf(model.Item{Number: 42, Ref: model.RepoRef{Project: project}})
		if !strings.HasSuffix(sheet, "#42") {
			t.Errorf("refLeaf(%q) gave %q, want the number behind it: without it the cell does not identify the item",
				project, sheet)
		}
	}
}

func TestSectionPrefixWithProjectsOfDifferentDepth(t *testing.T) {
	cases := []struct {
		name     string
		projects []string
		want     string
	}{
		{"same depth", []string{"acme/a", "acme/b"}, "acme"},
		{"profundidad distinta", []string{"a/b/c", "a/b/d"}, "a/b"},
		{"one without slashes", []string{"widget", "acme/a"}, ""},
		{"one without slashes first", []string{"widget", "acme/a"}, ""},
		{"both without slashes", []string{"a", "b"}, ""},
		{"three profundidades", []string{"x/y/z", "x/y", "x/y"}, "x"},
		// The prefix has NO trailing slash: whoever joins it adds the separator.
		{"the prefix has no trailing slash", []string{"a/b/c", "a/b/d"}, "a/b"},
		{"a single common segment", []string{"a/b", "a/c"}, "a"},
		{"nothing in common", []string{"a/b", "c/d"}, ""},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			items := make([]model.Item, len(c.projects))
			for i, p := range c.projects {
				items[i] = model.Item{Number: i + 1, Ref: model.RepoRef{Project: p}}
			}
			if got := sectionPrefix(items); got != c.want {
				t.Errorf("sectionPrefix(%v) = %q, want %q", c.projects, got, c.want)
			}
		})
	}

	// With a single item there is no prefix: putting the name there would duplicate the row.
	if got := sectionPrefix([]model.Item{{Ref: model.RepoRef{Project: "acme/widget"}}}); got != "" {
		t.Errorf("sectionPrefix with one item gave %q, want empty", got)
	}
}
