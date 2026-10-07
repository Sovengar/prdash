package tui

import (
	"strings"
	"testing"
	"unicode/utf8"

	"prdash/internal/forge/model"
	"prdash/internal/inbox"
)

func TestRefLeafReducesThePathToTheLeaf(t *testing.T) {
	for _, tc := range []struct {
		name  string
		items []model.Item
		want  []string
	}{
		{"with a long subgroup", mkItems("APPCITTI/vsocial/backend/api-gateway"), []string{"api-gateway#100"}},
		{"github owner/repo", mkItems("acme/widget"), []string{"widget#100"}},
		{"no subgroup", mkItems("myrepo"), []string{"myrepo#100"}},
		{"several items", mkItems("g/a/one", "g/a/two", "h/b/three"), []string{"one#100", "two#101", "three#102"}},
		{"empty project", mkItems("", "g/p"), []string{"#100", "p#101"}},
	} {
		for i, it := range tc.items {
			if got := refLeaf(it); got != tc.want[i] {
				t.Errorf("%s[%d]: refLeaf = %q, want %q", tc.name, i, got, tc.want[i])
			}
		}
	}
}

func TestPrefixModeCyclingReturnsToTheOrigin(t *testing.T) {
	m := prefixCommon
	for i, want := range []prefixMode{prefixFull, prefixLeaf, prefixCommon, prefixFull} {
		m = m.next()
		if m != want {
			t.Fatalf("press %d: mode = %v, want %v", i+1, m, want)
		}
	}
}

func TestPrefixModeStringIsTheHintsName(t *testing.T) {
	for mode, want := range map[prefixMode]string{
		prefixCommon: "common",
		prefixFull:   "full",
		prefixLeaf:   "leaf",
	} {
		if got := mode.String(); got != want {
			t.Errorf("%d.String() = %q, want %q", mode, got, want)
		}
	}
}

func TestNewRefLayoutDeclaresPrefixOnlyInCommon(t *testing.T) {
	sections := []inbox.Section{section(model.SectionReview, mkItems(
		"APPCITTI/vsocial/backend/api-gateway",
		"APPCITTI/vsocial/backend/web-app",
	)...)}
	if got := newRefLayout(sections, prefixCommon).prefixOf(model.SectionReview); got != "APPCITTI/vsocial/backend" {
		t.Errorf("prefijo en common = %q, want %q", got, "APPCITTI/vsocial/backend")
	}
	for _, mode := range []prefixMode{prefixFull, prefixLeaf} {
		if got := newRefLayout(sections, mode).prefixOf(model.SectionReview); got != "" {
			t.Errorf("prefix in %v = %q, want empty (there is no prefix to declare)", mode, got)
		}
	}
}

func TestNewRefLayoutSizesITEMByMode(t *testing.T) {
	sections := []inbox.Section{section(model.SectionReview,
		mkItems("APPCITTI/vsocial/backend/api-gateway", "APPCITTI/vsocial/web-app")...)}

	for _, tc := range []struct {
		mode prefixMode
		want int
		why  string
	}{
		{prefixCommon, 24, "the longest suffix + gap"},
		{prefixFull, itemWidthCap, "the full reference + gap, capped at the ceiling"},
		{prefixLeaf, 16, "the longest sheet + gap"},
	} {
		if got := newRefLayout(sections, tc.mode).cols[colRefIdx].width; got != tc.want {
			t.Errorf("%v: width of ITEM = %d, want %d (%s)", tc.mode, got, tc.want, tc.why)
		}
	}
}

func TestRefCellTextByMode(t *testing.T) {
	withGroup := mkItems("APPCITTI/vsocial/backend/api-gateway", "APPCITTI/vsocial/backend/web-app")
	prefix := sectionPrefix(withGroup)
	if got, want := refCellText(withGroup[0], prefixCommon, prefix), "api-gateway#100"; got != want {
		t.Errorf("common = %q, want %q", got, want)
	}
	if got, want := refCellText(withGroup[0], prefixFull, ""), "APPCITTI/vsocial/backend/api-gateway#100"; got != want {
		t.Errorf("full = %q, want %q", got, want)
	}
	if got, want := refCellText(withGroup[0], prefixLeaf, ""), "api-gateway#100"; got != want {
		t.Errorf("leaf = %q, want %q", got, want)
	}

	withoutGroup := mkItems("acme/one", "other/one")
	if p := sectionPrefix(withoutGroup); p != "" {
		t.Fatalf("the fixture should have no common prefix, has %q", p)
	}
	cellCommon := refCellText(withoutGroup[0], prefixCommon, "")
	cellFull := refCellText(withoutGroup[0], prefixFull, "")
	if cellCommon != cellFull || cellCommon != "acme/one#100" {
		t.Errorf("degradation: common = %q, full = %q, want both %q", cellCommon, cellFull, "acme/one#100")
	}
}

func TestFullClipsTheTailAndKeepsTheNumber(t *testing.T) {
	items := mkItems("APPCITTI/vsocial/backend/api-gateway", "APPCITTI/vsocial/web-app")
	lay := newRefLayout([]inbox.Section{section(model.SectionReview, items...)}, prefixFull)
	cell := itemCells(items[0], model.SectionReview, "me", lay)[colRefIdx].text

	if want := "…/vsocial/backend/api-gateway#100"; cell != want {
		t.Errorf("cell ITEM = %q, want %q", cell, want)
	}
	if got := utf8.RuneCountInString(cell); got > itemWidthCap {
		t.Errorf("cell = %q (%d runes), exceeds the cap %d", cell, got, itemWidthCap)
	}
}

func TestLeafDoesNotDisambiguateRepeatedLeaves(t *testing.T) {
	items := mkItems("acme/one", "other/one")
	lay := newRefLayout([]inbox.Section{section(model.SectionReview, items...)}, prefixLeaf)

	if got := itemCells(items[0], model.SectionReview, "me", lay)[colRefIdx].text; got != "one#100" {
		t.Errorf("cell of the first = %q, want %q", got, "one#100")
	}
	if got := itemCells(items[1], model.SectionReview, "me", lay)[colRefIdx].text; got != "one#101" {
		t.Errorf("cell of the second = %q, want %q", got, "one#101")
	}
}

func TestRefCellTextDoesNotRepeatThePathInTheLine(t *testing.T) {
	items := mkItems("acme/one", "other/one")
	lay := newRefLayout([]inbox.Section{section(model.SectionReview, items...)}, prefixCommon)
	if got := lay.prefixOf(model.SectionReview); got != "" {
		t.Fatalf("prefix = %q, want empty", got)
	}
	cell := itemCells(items[0], model.SectionReview, "me", lay)[colRefIdx].text
	if got := strings.Count(cell, "acme"); got != 1 {
		t.Errorf("the cell says %q: the group appears %d times, want 1", cell, got)
	}
}

func TestFullLosesTheITEMColumnOnVeryNarrowTerminals(t *testing.T) {
	items := mkItems("APPCITTI/vsocial/backend/api-gateway", "APPCITTI/vsocial/web-app")
	segs := []inbox.Section{section(model.SectionReview, items...)}

	const threshold = 48 // colForge(14) + itemWidthCap(34)
	for _, tc := range []struct {
		mode      prefixMode
		inner     int
		wantsITEM bool
	}{
		{prefixCommon, 38, true},
		{prefixLeaf, 38, true},
		{prefixFull, 47, false},
		{prefixFull, threshold, true},
	} {
		lay := newRefLayout(segs, tc.mode)
		exists := strings.Contains(titlesOf(lay, tc.inner), "ITEM")
		if exists != tc.wantsITEM {
			t.Errorf("%v a %d columnas interiores: ITEM presente = %v, want %v (columnas: %s)",
				tc.mode, tc.inner, exists, tc.wantsITEM, titlesOf(lay, tc.inner))
		}
	}
}

func titlesOf(l refLayout, inner int) string {
	var out []string
	for _, c := range l.cols[:fitColumns(l, inner)] {
		out = append(out, c.title)
	}
	return strings.Join(out, ",")
}
