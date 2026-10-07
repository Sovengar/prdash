package tui

import (
	"math/rand"
	"strings"
	"testing"
	"unicode/utf8"

	"prdash/internal/forge/model"
	"prdash/internal/inbox"
)

func mkItems(projects ...string) []model.Item {
	items := make([]model.Item, 0, len(projects))
	for i, p := range projects {
		items = append(items, mkItem("gitlab", "gitlab.com", p, "T", 100+i, ""))
	}
	return items
}

func section(kind model.Section, items ...model.Item) inbox.Section {
	return inbox.Section{Kind: kind, Items: items}
}

func TestSectionPrefix(t *testing.T) {
	for _, tc := range []struct {
		name  string
		items []model.Item
		want  string
	}{
		{"subgroup long", mkItems("APPCITTI/vsocial/backend/api-gateway", "APPCITTI/vsocial/backend/web-app"), "APPCITTI/vsocial/backend"},
		{"short in the common group", mkItems("APPCITTI/vsocial/a/x", "APPCITTI/vsocial/b/y"), "APPCITTI/vsocial"},
		{"nothing in common", mkItems("a/one", "b/two"), ""},
		{"github owner/repo", mkItems("acme/widget", "acme/lib"), "acme"},
		{"projects identicos", mkItems("g/p", "g/p"), "g"},
		{"a single item", mkItems("APPCITTI/vsocial/backend/api"), ""},
		{"no subgroups", mkItems("a", "b"), ""},
		{"prefijos parciales", mkItems("APPCITTI/vs/x", "APPCITTI/vsocial/y"), "APPCITTI"},
		{"project empty", mkItems("", "g/p"), ""},
		{"same path, different host", mkItems("g/p", "g/p"), "g"},
	} {
		if got := sectionPrefix(tc.items); got != tc.want {
			t.Errorf("%s: sectionPrefix = %q, want %q", tc.name, got, tc.want)
		}
	}
}

func TestRefSuffixRemovesThePrefixFromTheHeader(t *testing.T) {
	items := mkItems("APPCITTI/vsocial/backend/api-gateway", "APPCITTI/vsocial/backend/web-app")
	prefix := sectionPrefix(items)

	got := refSuffix(items[0], prefix)
	if want := "api-gateway#100"; got != want {
		t.Errorf("refSuffix = %q, want %q", got, want)
	}
	if full := prefix + "/" + got; full != refLabel(items[0]) {
		t.Errorf("%q + %q = %q, want %q", prefix, got, full, refLabel(items[0]))
	}
	if got := refSuffix(items[0], ""); got != refLabel(items[0]) {
		t.Errorf("without prefix: refSuffix = %q, want %q", got, refLabel(items[0]))
	}
	if got := refSuffix(items[0], "other/group"); got != refLabel(items[0]) {
		t.Errorf("prefijo ajeno: refSuffix = %q, want %q", got, refLabel(items[0]))
	}
}

func TestTruncateTail(t *testing.T) {
	for _, tc := range []struct {
		s    string
		w    int
		want string
	}{
		{"short", 10, "short"},
		{"abcdefgh", 4, "…fgh"},
		{"abc", 1, "…"},
		{"abc", 0, ""},
		{"api-gateway#1234", 6, "…#1234"},
		{"api-gateway#1234", 8, "…ay#1234"},
	} {
		if got := truncateTail(tc.s, tc.w); got != tc.want {
			t.Errorf("truncateTail(%q, %d) = %q, want %q", tc.s, tc.w, got, tc.want)
		}
	}
}

func TestNewRefLayoutSizesITEMByContent(t *testing.T) {
	lay := newRefLayout([]inbox.Section{
		section(model.SectionReview, mkItems("g/one", "g/two")...),
	}, prefixCommon)
	if got, want := lay.cols[colRefIdx].width, len("one#100")+1; got != want {
		t.Errorf("width of ITEM = %d, want %d (the longest suffix + gap)", got, want)
	}
	if got := lay.prefixOf(model.SectionReview); got != "g" {
		t.Errorf("review prefix = %q, want %q", got, "g")
	}
	if got := lay.prefixOf(model.SectionAuthored); got != "" {
		t.Errorf("prefix of a missing section = %q, want empty", got)
	}

	lay = newRefLayout([]inbox.Section{
		section(model.SectionReview, mkItems(
			"g/a-naïve-service-with-a-very-long-name",
			"g/other-naïve-service-with-a-very-long-name",
		)...),
	}, prefixCommon)
	if got := lay.cols[colRefIdx].width; got != itemWidthCap {
		t.Errorf("width of ITEM = %d, want the cap %d", got, itemWidthCap)
	}

	lay = newRefLayout(nil, prefixCommon)
	if got := lay.cols[colRefIdx].width; got != itemWidthMin {
		t.Errorf("width of ITEM with no items = %d, want %d", got, itemWidthMin)
	}
}

func TestItemCellsKeepTheNumberWhenClipped(t *testing.T) {
	items := mkItems(
		"APPCITTI/vsocial/backend/a-naïve-service-with-a-very-long-name",
		"APPCITTI/vsocial/backend/other-naïve-service-with-a-very-long-name",
	)
	lay := newRefLayout([]inbox.Section{section(model.SectionReview, items...)}, prefixCommon)
	ref := itemCells(items[0], model.SectionReview, "me", lay)[colRefIdx].text

	if !strings.HasSuffix(ref, "#100") {
		t.Errorf("cell ITEM = %q, want the suffix %q", ref, "#100")
	}
	if got := utf8.RuneCountInString(ref); got > itemWidthCap {
		t.Errorf("cell ITEM = %q (%d runes), exceeds the cap %d", ref, got, itemWidthCap)
	}
	if !strings.HasPrefix(ref, "…") {
		t.Errorf("cell ITEM = %q, want the tail clipping (prefix %q)", ref, "…")
	}
	// What is lost from the front is the group, which the header already declares.
	if got := lay.prefixOf(model.SectionReview); got != "APPCITTI/vsocial/backend" {
		t.Errorf("prefix = %q, want the group that paid for the clipping", got)
	}
}

func TestListLinesShowThePrefixOnAFixedLine(t *testing.T) {
	m := newTestModel(t, ghAdapter())
	m = send(t, m, page(1, "github", "github.com", model.SectionReview, model.ReviewRequested, mkItems(
		"APPCITTI/vsocial/backend/api-gateway",
		"APPCITTI/vsocial/backend/web-app",
	), false))

	var prefixLine, row string
	var all []string
	for _, l := range m.listLines(m.contentWidth()) {
		line := stripANSI(l.text)
		all = append(all, line)
		switch {
		case strings.Contains(line, "APPCITTI/vsocial/backend/"):
			prefixLine = line
		case strings.Contains(line, "api-gateway#100"):
			row = line
		}
	}
	if prefixLine == "" {
		t.Errorf("the common prefix line is missing:\n%s", strings.Join(all, "\n"))
	}
	if got := strings.Count(strings.Join(all, "\n"), "APPCITTI/vsocial/backend/"); got != 1 {
		t.Errorf("the prefix appears %d times, want 1 (only its line):\n%s", got, strings.Join(all, "\n"))
	}
	if strings.Contains(row, "APPCITTI") {
		t.Errorf("row = %q, want only the suffix", row)
	}
	if !strings.Contains(row, "api-gateway#100") {
		t.Errorf("row = %q, want %q untruncated", row, "api-gateway#100")
	}
	// No inner section header with the title and the count: the legend lives elsewhere.
	if joined := strings.Join(all, "\n"); strings.Contains(joined, "Assigned (2)") {
		t.Errorf("the list should not carry a section header:\n%s", joined)
	}
}

func TestListLinesWithNoPrefixKeepThePath(t *testing.T) {
	render := func(project string) []string {
		m := newTestModel(t, ghAdapter())
		m = send(t, m, page(1, "github", "github.com", model.SectionReview, model.ReviewRequested, mkItems(project), false))
		lines := make([]string, 0, 8)
		for _, l := range m.listLines(m.contentWidth()) {
			lines = append(lines, stripANSI(l.text))
		}
		return lines
	}

	if !containsSubstring(render("g/p"), "g/p#100") {
		t.Fatal("a short path in an items section was not painted whole")
	}

	var cell string
	for _, line := range render("APPCITTI/vsocial/backend/api-gateway") {
		if strings.Contains(line, "APPCITTI") {
			t.Errorf("with no common prefix there must be no prefix line: %q", line)
		}
		if strings.Contains(line, "api-gateway#100") {
			cell = line
		}
	}
	if cell == "" {
		t.Fatal("no line painted the items suffix")
	}
	if !strings.Contains(cell, "…") {
		t.Errorf("cell = %q, want the tail clipping", cell)
	}
}

func containsSubstring(lines []string, s string) bool {
	for _, l := range lines {
		if strings.Contains(l, s) {
			return true
		}
	}
	return false
}

func TestRefColInvariantes(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	seg := func() string {
		return strings.Repeat(string(rune('a'+rng.Intn(26))), 1+rng.Intn(12))
	}
	kinds := []model.Section{model.SectionAuthored, model.SectionReview, model.SectionMentions}
	for round := range 300 {
		sections := make([]inbox.Section, 1+rng.Intn(3))
		for secIdx := range sections {
			group := seg()
			if rng.Intn(4) == 0 {
				group = seg()
			}
			for range 1 + rng.Intn(5) {
				parts := []string{group}
				for range 1 + rng.Intn(4) {
					parts = append(parts, seg())
				}
				sections[secIdx].Kind = kinds[secIdx]
				sections[secIdx].Items = append(sections[secIdx].Items,
					mkItem("gitlab", "gitlab.com", strings.Join(parts, "/"), "T", 1+rng.Intn(9999), ""))
			}
		}

		// The invariants are checked in ALL THREE modes, not just common: tail clipping is only correct in common.
		for _, mode := range []prefixMode{prefixCommon, prefixFull, prefixLeaf} {
			lay := newRefLayout(sections, mode)
			w := lay.cols[colRefIdx].width
			if w < itemWidthMin || w > itemWidthCap {
				t.Fatalf("round %d in %v: width of ITEM = %d, outside [%d, %d]", round, mode, w, itemWidthMin, itemWidthCap)
			}
			if mode != prefixCommon {
				for _, sec := range sections {
					if p := lay.prefixOf(sec.Kind); p != "" {
						t.Fatalf("round %d in %v: section %v declares prefix %q, want empty", round, mode, sec.Kind, p)
					}
				}
			}
			for _, sec := range sections {
				prefix := lay.prefixOf(sec.Kind)
				for _, it := range sec.Items {
					full := refLabel(it)
					want := refCellText(it, mode, prefix)
					if want == "" {
						t.Fatalf("round %d in %v: empty label for %q", round, mode, full)
					}
					if prefix != "" {
						if recon := strings.TrimPrefix(full, prefix+"/"); recon == full {
							t.Fatalf("round %d in %v: the prefix %q does not apply to %q", round, mode, prefix, full)
						} else if recon != want {
							t.Fatalf("round %d in %v: cell %q, want the suffix %q of %q", round, mode, want, recon, full)
						}
					}
					cell := itemCells(it, sec.Kind, "me", lay)[colRefIdx].text
					if cell == "" {
						t.Fatalf("round %d in %v: empty cell for %q", round, mode, full)
					}
					cut := truncateTail(want, textWidth(w))
					if want == cut {
						if cell != want {
							t.Fatalf("round %d en %v: cell %q, want %q", round, mode, cell, want)
						}
						continue
					}
					if !strings.HasPrefix(cell, "…") {
						t.Fatalf("round %d in %v: cell clipped %q, want the prefix %q", round, mode, cell, "…")
					}
					tail := want[max(0, len(want)-(textWidth(w)-1)):]
					if !strings.HasSuffix(cell, tail) {
						t.Fatalf("round %d in %v: cell %q loses the tail %q of %q", round, mode, cell, tail, want)
					}
				}
			}
		}
	}
}
