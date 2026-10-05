package tui

import (
	"strings"
	"testing"
	"unicode/utf8"

	"prdash/internal/forge/model"
	"prdash/internal/inbox"
)

func TestPadAndTruncate(t *testing.T) {
	if got := pad("ab", 5); got != "ab   " {
		t.Errorf("pad = %q", got)
	}
	if got := truncate("abcdef", 4); got != "abc…" {
		t.Errorf("truncate = %q", got)
	}
}

func TestRenderCellsPadsBeforeStyle(t *testing.T) {
	cells := []cell{
		{text: "ab", style: styleForge, width: 5},
		{text: "x", style: styleRef, width: 3},
	}
	out := stripANSI(renderCells(cells, newRefLayout(nil, prefixCommon), 100))
	if !strings.HasPrefix(out, "ab   x  ") {
		t.Errorf("renderCells = %q", out)
	}
}

func TestNoCellFillsItsColumn(t *testing.T) {
	it := mkItem("github", "github.com", "APPCITTI/vsocial/backend/mobile-frontend",
		strings.Repeat("t", colTitle+5), 1198, "")
	it.ReviewKind = model.ReviewRequested // without this roleText returns "-"
	lay := newRefLayout([]inbox.Section{section(model.SectionReview, mkItems("APPCITTI/vsocial/backend/mobile-frontend")...)}, prefixCommon)
	cells := itemCells(it, model.SectionReview, "me", lay)

	if got, want := utf8.RuneCountInString(roleText(it, "me")), colRole-1; got != want {
		t.Fatalf("the ROLE text measures %d runes, want %d: the case that overflowed is no longer being tested", got, want)
	}
	if got, want := utf8.RuneCountInString(cells[colRoleIdx].text), colRole-1; got != want {
		t.Fatalf("cell ROLE = %d runes, want %d (colRole - gap)", got, want)
	}
	if got, want := utf8.RuneCountInString(cells[colTitleIdx].text), colTitle-1; got != want {
		t.Fatalf("cell TITLE = %d runes, want %d (colTitle - gap)", got, want)
	}
	if got, want := utf8.RuneCountInString(cells[colRefIdx].text), textWidth(lay.cols[colRefIdx].width); got != want {
		t.Fatalf("cell ITEM = %d runes, want %d (the columns budget)", got, want)
	}
	for i, c := range cells {
		if n := utf8.RuneCountInString(c.text); n > textWidth(c.width) {
			t.Errorf("cell %d (%q) = %d runes, want <= %d (width %d - gap)", i, c.text, n, textWidth(c.width), c.width)
		}
	}
}

func TestColumnsSeparatedByOneSpace(t *testing.T) {
	m := newTestModel(t, ghAdapter())
	m = send(t, m, page(1, "github", "github.com", model.SectionReview, model.ReviewRequested, []model.Item{
		mkItem("github", "github.com", "APPCITTI/vsocial/backend/api-gateway", "api gateway", 1234, ""),
		mkItem("github", "github.com", "APPCITTI/vsocial/backend/mobile-frontend", "movil", 1198, ""),
	}, false))

	lay := newRefLayout([]inbox.Section{{Kind: m.activeSection, Items: m.rows()}}, prefixCommon)
	inner := m.contentWidth() - 2
	var row string
	for _, l := range m.listLines(m.contentWidth()) {
		if line := stripANSI(l.text); strings.Contains(line, "api-gateway#1234") {
			row = line
		}
	}
	if row == "" {
		t.Fatal("the item row was not found")
	}

	runes := []rune(row)
	off := 2
	for _, c := range lay.cols[:fitColumns(lay, inner)] {
		end := off + c.width
		if end > len(runes) {
			t.Fatalf("row %q shorter than the column %q", row, c.title)
		}
		if runes[end-1] != ' ' {
			t.Errorf("the column %q ends in %q, want a separating space at %q", c.title, runes[end-1], row)
		}
		off = end
	}
}

func TestForgeBadgeCoversStandardAndSelfHosted(t *testing.T) {
	badge := func(forge, host string) string {
		it := model.NewItem(model.RepoRef{Forge: forge, Host: host, Project: "o/r"}, 1)
		return forgeBadge(it)
	}
	for _, tc := range []struct {
		forge, host, want string
	}{
		{"github", "github.com", "GH"},
		{"gitlab", "gitlab.com", "GLab"},
		{"bitbucket", "bitbucket.org", "BB"},
		{"gitlab", "umane.emeal.nttdata.com", "GLab@umane"},
		{"github", "github.corp.example.com", "GH@github"},
		{"gitlab", "gitserver", "GLab@gitserver"},
		{"github", "", "GH"},
		{"gerrit", "gerrit.example.com", "gerrit@gerrit"},
	} {
		if got := badge(tc.forge, tc.host); got != tc.want {
			t.Errorf("forgeBadge(%q, %q) = %q, want %q", tc.forge, tc.host, got, tc.want)
		}
	}

	it := model.NewItem(model.RepoRef{Forge: "gitlab", Host: "umane.emeal.nttdata.com", Project: "g/p"}, 7)
	if got := forgeLabel(it); got != "gitlab@umane.emeal.nttdata.com" {
		t.Errorf("forgeLabel = %q", got)
	}
}

func TestForgeBadgeFitsColumn(t *testing.T) {
	it := model.NewItem(model.RepoRef{Forge: "gitlab", Host: "empresaurbanisimaziyota.example.com", Project: "g/p"}, 1)
	if got := truncate(forgeBadge(it), colForge); utf8.RuneCountInString(got) != colForge {
		t.Errorf("cell forge = %q (%d runes), want %d", got, utf8.RuneCountInString(got), colForge)
	}
}

func TestNavigationMovesCursor(t *testing.T) {
	m := newTestModel(t, ghAdapter())
	m = send(t, m, page(1, "github", "github.com", model.SectionReview, model.ReviewRequested, []model.Item{
		mkItem("github", "github.com", "acme/widget", "A", 1, ""),
		mkItem("github", "github.com", "acme/widget", "B", 2, ""),
		mkItem("github", "github.com", "acme/widget", "C", 3, ""),
	}, false))

	if m.cursor != 0 {
		t.Fatalf("initial cursor = %d", m.cursor)
	}
	if it, ok := m.selected(); !ok || it.Number != 3 {
		t.Fatalf("initial selection = %+v, want #3 (the most recent)", it)
	}
	m = press(t, m, "down")
	if m.cursor != 1 {
		t.Fatalf("cursor after down = %d", m.cursor)
	}
	if it, ok := m.selected(); !ok || it.Number != 2 {
		t.Fatalf("selection after down = %+v, want #2", it)
	}
	m = press(t, m, "down")
	if m.cursor != 2 {
		t.Fatalf("cursor after the second down = %d", m.cursor)
	}
	m = press(t, m, "down")
	if it, ok := m.selected(); !ok || it.Number != 1 {
		t.Fatalf("selection at the bottom edge = %+v, want #1", it)
	}
	if m.cursor != 2 || m.activeSection != model.SectionReview {
		t.Fatalf("the cursor should stay on the last row of the active section: cursor=%d section=%q", m.cursor, m.activeSection)
	}
}

func TestSectionNextCyclesActiveSection(t *testing.T) {
	m := newTestModel(t, ghAdapter())
	m = send(t, m, page(1, "github", "github.com", model.SectionReview, model.ReviewRequested, []model.Item{
		mkItem("github", "github.com", "acme/widget", "A", 1, ""),
	}, false))

	want := []model.Section{model.SectionMentions, model.SectionAuthored, model.SectionReview, model.SectionMentions}
	for i, w := range want {
		m = press(t, m, "tab")
		if m.activeSection != w {
			t.Fatalf("tab %d: activeSection = %q, want %q", i+1, m.activeSection, w)
		}
	}
}

func TestSectionNextHonorsRebind(t *testing.T) {
	m := newTestModel(t, ghAdapter())
	m.cfg.Keybindings["section-next"] = "n"
	m = send(t, m, page(1, "github", "github.com", model.SectionReview, model.ReviewRequested, []model.Item{
		mkItem("github", "github.com", "acme/widget", "C", 3, ""),
	}, false))

	m = press(t, m, "n")
	if m.activeSection != model.SectionMentions {
		t.Fatalf("n should cycle the active section: activeSection = %q", m.activeSection)
	}
	m = press(t, m, "tab")
	if m.activeSection != model.SectionMentions {
		t.Fatalf("tab moved the section even though it was rebound: activeSection = %q", m.activeSection)
	}
}

func TestAuthoredOnlyShowsOwnItems(t *testing.T) {
	m := newTestModel(t, ghAdapter())
	m = send(t, m, page(1, "github", "github.com", model.SectionAuthored, "", []model.Item{mkItem("github", "github.com", "acme/widget", "Mine", 1, "")}, false))
	m = send(t, m, page(1, "github", "github.com", model.SectionReview, model.ReviewRequested, []model.Item{mkItem("github", "github.com", "acme/lib", "Ajeno", 2, "")}, false))

	authored := m.sectionItems(model.SectionAuthored)
	if len(authored) != 1 || authored[0].Title != "Mine" {
		t.Fatalf("authored = %+v", authored)
	}
}

func TestInitReturnsCmd(t *testing.T) {
	m := newTestModel(t, ghAdapter())
	if m.Init() == nil {
		t.Fatal("Init should return a Cmd")
	}
}

func TestCompactCount(t *testing.T) {
	cases := map[int]string{
		0: "0", 7: "7", 999: "999",
		1000: "1.0k", 1234: "1.2k", 9499: "9.5k", 9949: "9.9k",
		9950: "10k", 9999: "10k", 10000: "10k", 12345: "12k", 999999: "999k",
	}
	for in, want := range cases {
		if got := compactCount(in); got != want {
			t.Errorf("compactCount(%d) = %q, want %q", in, got, want)
		}
	}
	if got := utf8.RuneCountInString(diffColumnText(model.DiffStat{Additions: 9999, Deletions: 9999, Known: true})); got > textWidth(colDiff) {
		t.Errorf("the worst case takes %d runes and the column gives %d", got, textWidth(colDiff))
	}
}

func TestDiffColumnTextUnknownIsDash(t *testing.T) {
	if got := diffColumnText(model.DiffStat{}); got != "-" {
		t.Errorf("diffColumnText with no data = %q, want %q", got, "-")
	}
	if got := diffColumnText(model.DiffStat{Known: true}); got != "+0 -0" {
		t.Errorf("diffColumnText of an empty PR = %q, want %q (known, zero lines)", got, "+0 -0")
	}
	if got := diffColumnText(model.DiffStat{Additions: 381, Deletions: 36, Known: true}); got != "+381 -36" {
		t.Errorf("diffColumnText = %q, want %q", got, "+381 -36")
	}
}

func TestDiffColumnAppearsOnlyOnWideTerminals(t *testing.T) {
	item := mkItem("github", "github.com", "APPCITTI/vsocial/backend/vsocial-api-actuacions", "fix", 1015, "")
	item.Diff = model.DiffStat{Additions: 42, Deletions: 1, Files: 2, Known: true}

	render := func(width int) (header, row string) {
		m := newTestModel(t, ghAdapter())
		m.width, m.height = width, 40
		m = send(t, m, page(1, "github", "github.com", model.SectionReview, model.ReviewRequested, []model.Item{item}, false))
		lay := newRefLayout([]inbox.Section{{Kind: m.activeSection, Items: m.rows()}}, prefixCommon)
		header = stripANSI(headerLine(lay, m.contentWidth()-2))
		for _, l := range m.listLines(m.contentWidth()) {
			if line := stripANSI(l.text); strings.Contains(line, "vsocial-api-actuacions#1015") {
				row = line
			}
		}
		return header, row
	}

	_, narrow := render(124)
	if strings.Contains(narrow, "+42 -1") {
		t.Errorf("at 124 the DIFF column should not fit, but the row carries it:\n%s", narrow)
	}
	header, wide := render(200)
	if !strings.Contains(wide, "+42 -1") {
		t.Errorf("at 200 the row should carry the diffstat:\n%s", wide)
	}
	if !strings.Contains(header, "DIFF") {
		t.Errorf("at 200 the header should carry the DIFF column:\n%s", header)
	}
}

// The DIFF column carries two colours in the same cell.
func TestDiffSpansColoursOnlyTheDigits(t *testing.T) {
	cases := []struct {
		plain string
		want  []string // text of each span; nil = no spans (all plain)
	}{
		{"+381 -36", []string{"+381", " ", "-36"}},
		{"+1.2k -6.7k", []string{"+1.2k", " ", "-6.7k"}},
		{"+381 -36 (11 files)", []string{"+381", " ", "-36", " (11 files)"}},
		{"+0 -0", []string{"+0", " ", "-0"}},
		{"-", nil},
		{"no changes", nil},
		{"unknown (forge did not report it)", nil},
		{"+381", nil}, // truncated: without the deletions side
		{"+abc -def", nil},
		{"381 36", nil}, // without a sign it is not a diffstat
	}
	for _, c := range cases {
		spans := diffSpans(c.plain)
		if c.want == nil {
			if spans != nil {
				t.Errorf("diffSpans(%q) = %+v, want nil (no color)", c.plain, spans)
			}
			continue
		}
		var got []string
		for _, s := range spans {
			got = append(got, s.text)
		}
		if len(got) != len(c.want) {
			t.Errorf("diffSpans(%q) = %q, want %q", c.plain, got, c.want)
			continue
		}
		for i := range got {
			if got[i] != c.want[i] {
				t.Errorf("diffSpans(%q)[%d] = %q, want %q", c.plain, i, got[i], c.want[i])
			}
		}
	}
}

func TestDiffSpansSideColours(t *testing.T) {
	spans := diffSpans("+381 -36")
	if len(spans) != 3 {
		t.Fatalf("spans = %d, want 3", len(spans))
	}
	if got := spans[0].style.Render("x"); got != styleDiffAdd.Render("x") {
		t.Errorf("the added part should go in styleDiffAdd: %q", got)
	}
	if got := spans[2].style.Render("x"); got != styleDiffDel.Render("x") {
		t.Errorf("the removed part should go in styleDiffDel: %q", got)
	}
}

func TestRenderCellSpansKeepWidth(t *testing.T) {
	c := diffCell(model.DiffStat{Additions: 1589, Deletions: 474, Files: 23, Known: true}, colDiff)
	out := renderCell(c)
	if got := utf8.RuneCountInString(stripANSI(out)); got != colDiff {
		t.Errorf("the cell takes %d runes, want %d: %q", got, colDiff, stripANSI(out))
	}
	if got := strings.TrimRight(stripANSI(out), " "); got != "+1.6k -474" {
		t.Errorf("cell = %q, want %q", got, "+1.6k -474")
	}
	if !strings.Contains(out, "\x1b[") {
		t.Errorf("the cell should carry color: %q", out)
	}
	unknown := renderCell(diffCell(model.DiffStat{}, colDiff))
	if got := strings.TrimRight(stripANSI(unknown), " "); got != "-" {
		t.Errorf("unknown cell = %q, want %q", got, "-")
	}
	if got := utf8.RuneCountInString(stripANSI(unknown)); got != colDiff {
		t.Errorf("the unknown cell takes %d runes, want %d", got, colDiff)
	}
}

func TestStyleDiffTextDoesNotAlterTheWidth(t *testing.T) {
	for _, plain := range []string{"+381 -36", "+381 -36 (11 files)", "-", "no changes", "unknown (forge did not report it)"} {
		if got := stripANSI(styleDiffText(plain)); got != plain {
			t.Errorf("styleDiffText(%q) on plain = %q, want identical", plain, got)
		}
	}
}
