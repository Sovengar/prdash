package tui

import (
	"strconv"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"prdash/internal/forge/model"
)

func ghItems(projects ...string) []model.Item {
	items := make([]model.Item, 0, len(projects))
	for i, p := range projects {
		items = append(items, mkItem("github", "github.com", p, "T", 100+i, ""))
	}
	return items
}

func numberedItems(n int, project string) []model.Item {
	items := make([]model.Item, 0, n)
	for i := 1; i <= n; i++ {
		items = append(items, mkItem("github", "github.com", project, "Item "+strconv.Itoa(i), i, ""))
	}
	return items
}

func listText(m Model) string {
	return stripANSI(strings.Join(textOf(m.listLines(m.contentWidth())), "\n"))
}

func TestDefaultSectionIsAssigned(t *testing.T) {
	m := newTestModel(t, ghAdapter())
	m = send(t, m, page(1, "github", "github.com", model.SectionAuthored, "", []model.Item{
		mkItem("github", "github.com", "acme/mine", "Mine item", 1, ""),
	}, false))
	m = send(t, m, page(1, "github", "github.com", model.SectionReview, model.ReviewRequested, []model.Item{
		mkItem("github", "github.com", "acme/assigned", "Assigned item", 2, ""),
	}, false))
	m = send(t, m, page(1, "github", "github.com", model.SectionMentions, "", []model.Item{
		mkItem("github", "github.com", "acme/mentioned", "Mentioned item", 3, ""),
	}, false))

	if m.activeSection != model.SectionReview {
		t.Fatalf("activeSection = %q, want %q (Assigned)", m.activeSection, model.SectionReview)
	}
	view := stripANSI(m.View().Content)
	if !strings.Contains(view, "Assigned item") {
		t.Errorf("the list should show the Assigned items:\n%s", view)
	}
	for _, hidden := range []string{"Mine item", "Mentioned item"} {
		if strings.Contains(view, hidden) {
			t.Errorf("the list shows %q, which belongs to another section:\n%s", hidden, view)
		}
	}
}

func TestCycleThroughEmptySection(t *testing.T) {
	m := newTestModel(t, ghAdapter())
	m = send(t, m, page(1, "github", "github.com", model.SectionReview, model.ReviewRequested, []model.Item{
		mkItem("github", "github.com", "acme/widget", "Assigned", 1, ""),
	}, false))

	m = press(t, m, "tab")
	if m.activeSection != model.SectionMentions {
		t.Fatalf("activeSection = %q, want %q", m.activeSection, model.SectionMentions)
	}
	view := stripANSI(m.View().Content)
	if !strings.Contains(view, "(empty)") {
		t.Errorf("the empty section should mark (empty):\n%s", view)
	}
	if !strings.Contains(view, "Mentioned (0)") {
		t.Errorf("the legend should show 0 for the empty one:\n%s", view)
	}

	m = press(t, m, "tab")
	if m.activeSection != model.SectionAuthored {
		t.Fatalf("activeSection = %q, want %q (sigue ciclando)", m.activeSection, model.SectionAuthored)
	}
}

func TestLegendExactFormatReplacesInbox(t *testing.T) {
	m := newTestModel(t, ghAdapter())
	m = send(t, m, page(1, "github", "github.com", model.SectionAuthored, "", numberedItems(9, "mine/repo"), false))
	m = send(t, m, page(1, "github", "github.com", model.SectionReview, model.ReviewRequested, numberedItems(4, "assigned/repo"), false))

	lines := visibleLines(m.View().Content)
	if !hasBoxTitle(lines, "Mine (9) · Assigned (4) · Mentioned (0)") {
		t.Errorf("the legend does not have the exact format:\n%s", strings.Join(lines, "\n"))
	}
	if hasBoxTitle(lines, "Inbox") {
		t.Errorf("the Inbox title should no longer exist:\n%s", strings.Join(lines, "\n"))
	}
}

func TestLegendHighlightsActiveSection(t *testing.T) {
	m := newTestModel(t, ghAdapter())
	m = send(t, m, page(1, "github", "github.com", model.SectionReview, model.ReviewRequested, []model.Item{
		mkItem("github", "github.com", "acme/widget", "Assigned", 1, ""),
	}, false))

	raw := m.View().Content
	if !strings.Contains(raw, styleLegendActive.Render("Assigned (1)")) {
		t.Errorf("the active section should be highlighted in the legend")
	}
	for _, dim := range []string{"Mine (0)", "Mentioned (0)"} {
		if !strings.Contains(raw, styleDim.Render(dim)) {
			t.Errorf("%q should be dimmed in the legend", dim)
		}
	}
}

func TestLegendCountsDedupedSection(t *testing.T) {
	m := newTestModel(t, ghAdapter())
	dup := mkItem("github", "github.com", "acme/widget", "Dup", 1, "")
	m = send(t, m, page(1, "github", "github.com", model.SectionAuthored, "", []model.Item{dup}, false))
	m = send(t, m, page(1, "github", "github.com", model.SectionReview, model.ReviewRequested, []model.Item{dup}, false))

	view := stripANSI(m.View().Content)
	if !strings.Contains(view, "Mine (1) · Assigned (0) · Mentioned (0)") {
		t.Errorf("the legend does not reflect the deduplication by authority:\n%s", view)
	}
}

func TestPrefixFollowsActiveSection(t *testing.T) {
	m := newTestModel(t, ghAdapter())
	m = send(t, m, page(1, "github", "github.com", model.SectionAuthored, "", ghItems("mine/group/api-a", "mine/group/api-b"), false))
	m = send(t, m, page(1, "github", "github.com", model.SectionReview, model.ReviewRequested, ghItems("app/vsocial/backend/api-gateway", "app/vsocial/backend/web-app"), false))

	if got := listText(m); !strings.Contains(got, "app/vsocial/backend/") || strings.Contains(got, "mine/group/") {
		t.Errorf("with Assigned active the list should show its prefix, not Mine:\n%s", got)
	}

	m = press(t, m, "tab")
	m = press(t, m, "tab")
	if m.activeSection != model.SectionAuthored {
		t.Fatalf("activeSection = %q, want %q", m.activeSection, model.SectionAuthored)
	}
	if got := listText(m); !strings.Contains(got, "mine/group/") || strings.Contains(got, "app/vsocial/backend/") {
		t.Errorf("with Mine active the list should show its prefix:\n%s", got)
	}
}

func TestPrefixKeepsTableWidth(t *testing.T) {
	for _, width := range []int{70, 124, 200} {
		m := newTestModel(t, ghAdapter())
		m.width, m.height = width, 40
		m = send(t, m, page(1, "github", "github.com", model.SectionReview, model.ReviewRequested, ghItems(
			"APPCITTI/vsocial/backend/api-gateway",
			"APPCITTI/vsocial/backend/web-app",
		), false))

		view := m.View().Content
		for i, l := range visibleLines(view) {
			if w := ansi.StringWidth(l); w != m.width {
				t.Errorf("width %d: line %d measures %d columns:\n%q", width, i, w, l)
			}
		}
		if !strings.Contains(stripANSI(view), "api-gateway#100") {
			t.Errorf("width %d: the ITEM cell lost the sheet and the number:\n%s", width, stripANSI(view))
		}
	}
}

func TestRememberCursorAndScrollPerSection(t *testing.T) {
	m := newTestModel(t, ghAdapter())
	m.width, m.height = 160, 24
	m = send(t, m, page(1, "github", "github.com", model.SectionReview, model.ReviewRequested, manyItems(60), false))
	m = send(t, m, page(1, "github", "github.com", model.SectionAuthored, "", []model.Item{
		mkItem("github", "github.com", "acme/mine-a", "Mine A", 1, ""),
		mkItem("github", "github.com", "acme/mine-b", "Mine B", 2, ""),
	}, false))

	m = press(t, m, "end")
	assigned := sectionPos{cursor: m.cursor, scroll: m.scroll}
	if assigned.cursor == 0 || assigned.scroll == 0 {
		t.Fatalf("the case needs cursor and scroll moved in Assigned: %+v", assigned)
	}

	m = press(t, m, "tab")
	m = press(t, m, "tab")
	if m.activeSection != model.SectionAuthored {
		t.Fatalf("activeSection = %q, want %q", m.activeSection, model.SectionAuthored)
	}
	if m.cursor != 0 || m.scroll != 0 {
		t.Fatalf("Mine must open at its default position: cursor=%d scroll=%d", m.cursor, m.scroll)
	}
	m = press(t, m, "down")
	mine := sectionPos{cursor: m.cursor, scroll: m.scroll}

	m = press(t, m, "tab")
	if m.activeSection != model.SectionReview {
		t.Fatalf("activeSection = %q, want %q", m.activeSection, model.SectionReview)
	}
	if m.cursor != assigned.cursor || m.scroll != assigned.scroll {
		t.Errorf("Assigned did not recover its position: cursor=%d scroll=%d, want %+v", m.cursor, m.scroll, assigned)
	}
	m = press(t, m, "tab")
	m = press(t, m, "tab")
	if m.cursor != mine.cursor || m.scroll != mine.scroll {
		t.Errorf("Mine did not recover its position: cursor=%d scroll=%d, want %+v", m.cursor, m.scroll, mine)
	}
}

func TestRefreshKeepsSectionPosition(t *testing.T) {
	m := newTestModel(t, ghAdapter())
	m.width, m.height = 160, 24
	m = send(t, m, page(m.cycle, "github", "github.com", model.SectionReview, model.ReviewRequested, manyItems(60), false))
	m = press(t, m, "end")
	before := sectionPos{cursor: m.cursor, scroll: m.scroll}

	m = send(t, m, page(m.cycle, "github", "github.com", model.SectionReview, model.ReviewRequested, manyItems(60), false))
	if m.cursor != before.cursor || m.scroll != before.scroll {
		t.Errorf("the refresh moved the position: cursor=%d scroll=%d, want %+v", m.cursor, m.scroll, before)
	}

	m = send(t, m, page(m.cycle, "github", "github.com", model.SectionReview, model.ReviewRequested, manyItems(3), false))
	if m.cursor > len(m.rows())-1 || m.cursor < 0 {
		t.Errorf("cursor = %d outside the %d new rows", m.cursor, len(m.rows()))
	}
	if m.scroll != 0 {
		t.Errorf("scroll = %d, want 0: the new content fits whole", m.scroll)
	}
}

func TestActiveEmptyShowsEmptyAndZero(t *testing.T) {
	m := newTestModel(t, ghAdapter())
	view := stripANSI(m.View().Content)
	if !strings.Contains(view, "(empty)") {
		t.Errorf("the active empty one should mark (empty):\n%s", view)
	}
	if !strings.Contains(view, "Assigned (0)") {
		t.Errorf("the legend should show 0 on the active one:\n%s", view)
	}
}

func TestLoadingMoreOnlyOnActiveSection(t *testing.T) {
	m := newTestModel(t, ghAdapter())
	m = send(t, m, page(1, "github", "github.com", model.SectionAuthored, "", []model.Item{
		mkItem("github", "github.com", "acme/mine", "Mine", 1, ""),
	}, true))
	m = send(t, m, page(1, "github", "github.com", model.SectionReview, model.ReviewRequested, []model.Item{
		mkItem("github", "github.com", "acme/assigned", "Assigned", 2, ""),
	}, false))

	if view := stripANSI(m.View().Content); strings.Contains(view, "loading more…") {
		t.Errorf("the indicator of a non-active section must not be painted:\n%s", view)
	}

	m = press(t, m, "tab")
	m = press(t, m, "tab")
	if m.activeSection != model.SectionAuthored {
		t.Fatalf("activeSection = %q, want %q", m.activeSection, model.SectionAuthored)
	}
	if view := stripANSI(m.View().Content); !strings.Contains(view, "loading more…") {
		t.Errorf("back at the paginating section, the indicator reappears:\n%s", view)
	}
}

func TestWarningsOnlyOnActiveSection(t *testing.T) {
	m := newTestModel(t, ghAdapter())
	m = send(t, m, pageMsg{cycle: 1, key: streamKey{forge: "github", section: model.SectionAuthored}, warnings: []model.Warning{
		{Forge: "github", Section: model.SectionAuthored, Kind: "network", Msg: "boom"},
	}})
	m = send(t, m, page(1, "github", "github.com", model.SectionReview, model.ReviewRequested, []model.Item{
		mkItem("github", "github.com", "acme/assigned", "Assigned", 2, ""),
	}, false))

	view := stripANSI(m.View().Content)
	if strings.Contains(view, "could not be queried") {
		t.Errorf("the notice of a non-active section must not be painted:\n%s", view)
	}
	if !strings.Contains(view, "Mine (0)") {
		t.Errorf("the legend should keep counting the section with a notice:\n%s", view)
	}

	m = press(t, m, "tab")
	m = press(t, m, "tab")
	if view := stripANSI(m.View().Content); !strings.Contains(view, "could not be queried") {
		t.Errorf("on tabbing to the section with the notice, it should be painted:\n%s", view)
	}
}

func TestNarrowTerminalTruncatesLegendKeepsBorderWidth(t *testing.T) {
	m := newTestModel(t, ghAdapter())
	m.width, m.height = 34, 20
	m = send(t, m, page(1, "github", "github.com", model.SectionAuthored, "", numberedItems(9, "mine/repo"), false))
	m = send(t, m, page(1, "github", "github.com", model.SectionReview, model.ReviewRequested, numberedItems(4, "assigned/repo"), false))

	lines := visibleLines(m.View().Content)
	for i, l := range lines {
		if w := ansi.StringWidth(l); w != m.width {
			t.Errorf("line %d measures %d columns, want %d:\n%q", i, w, m.width, l)
		}
	}
	top := ""
	for _, l := range lines {
		if strings.HasPrefix(l, "╭") && strings.Contains(l, "Mine (") {
			top = l
		}
	}
	if top == "" {
		t.Fatalf("the legend line was not found:\n%s", strings.Join(lines, "\n"))
	}
	if strings.Contains(top, "Mentioned (0)") {
		t.Errorf("at 34 columns the legend should be clipped:\n%q", top)
	}
}
