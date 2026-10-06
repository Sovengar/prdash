package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func branchesOfThePopup(t *testing.T, filter string, branches ...string) Model {
	t.Helper()
	m := sizedRetarget(t, 90, 60, branches...)
	if filter != "" {
		m = pressFilter(t, m, filter)
	}
	if len(m.retarget.view) == 0 {
		t.Fatalf("the filter %q left the view empty of %v", filter, m.retarget.all)
	}
	return m
}

func linesOfThePopup(m Model) []string {
	return strings.Split(stripANSI(m.retargetOverlay2()), "\n")
}

// The cursor is the row enter picks.
func TestOnlyOneRowCarriesTheCursorAndItIsTheChosenOne(t *testing.T) {
	branches := []string{"main", "release/2.0", "fix/one", "fix/two", "wip"}
	for cursor := range len(branches) {
		m := branchesOfThePopup(t, "", branches...)
		m.retarget.cursor = cursor
		m.retarget.win = m.retargetWindow()

		expected := ""
		for i, r := range m.retarget.view {
			if r == m.retarget.view[cursor] {
				expected = r
				_ = i
			}
		}
		withCursor := 0
		for _, l := range linesOfThePopup(m) {
			if !strings.Contains(l, "▸") {
				continue
			}
			withCursor++
			if !strings.Contains(l, expected) {
				t.Errorf("cursor %d: the marked row is %q, want the cursors %q", cursor, strings.TrimSpace(l), expected)
			}
		}
		if withCursor != 1 {
			t.Errorf("cursor %d: %d rows marked, want exactamente 1", cursor, withCursor)
		}
		// And the cursor never leaves the view: outside it there would be no row to draw it on.
		if cursor < len(m.retarget.view) && withCursor == 0 {
			t.Errorf("cursor %d: no marked row with a full view", cursor)
		}
	}
}

// Two different signals and the popup has to distinguish them.
func TestTheCursorAndTheCurrentBaseDoNotShareASymbol(t *testing.T) {
	m := branchesOfThePopup(t, "", "main", "other", "tercera")
	m.retarget.cursor = 0
	m.retarget.win = 0
	lines := linesOfThePopup(m)
	var row string
	for _, l := range lines {
		if strings.Contains(l, "▸") {
			row = l
		}
	}
	if row == "" {
		t.Fatalf("there is no marked row: %q", lines)
	}
	if !strings.Contains(row, "▸") {
		t.Errorf("the cursor row does not carry the cursor: %q", row)
	}
	if !strings.Contains(row, "current") {
		t.Errorf("the current base is not marked when the cursor is on it: %q", row)
	}
	marked := 0
	for _, l := range lines {
		if strings.Contains(l, "current") {
			marked++
		}
	}
	if marked != 1 {
		t.Errorf("%d rows marked as current base, want 1: the badge says where you leave from, not which is chosen", marked)
	}
}

func TestTheListAlignsInAColumn(t *testing.T) {
	// A long base and a one-character one, which is what separates an aligned column from a
	// coincidental one.
	m := branchesOfThePopup(t, "", "main", "a-branch-with-name-naïve-and-quite-long", "wip")
	m.retarget.cursor = 1
	m.retarget.win = 0

	// The header fixes the width: every row has to measure the same.
	var widths []int
	for _, l := range linesOfThePopup(m) {
		widths = append(widths, ansi.StringWidth(l))
	}
	for i := 1; i < len(widths); i++ {
		if widths[i] != widths[0] {
			t.Errorf("line %d measures %d columns and line 0 measures %d: the box is off center",
				i, widths[i], widths[0])
		}
	}

	// The current base's suffix lands in the SAME column as the bare name.
	colOfCurrent := -1
	for _, l := range linesOfThePopup(m) {
		i := strings.Index(l, "current")
		if i < 0 {
			continue
		}
		col := ansi.StringWidth(l[:i])
		if colOfCurrent < 0 {
			colOfCurrent = col
		} else if col != colOfCurrent {
			t.Errorf("the suffix falls at column %d and before at %d: the column is not aligned", col, colOfCurrent)
		}
	}
	if colOfCurrent < 0 {
		t.Error("the current bases suffix was not painted")
	}
	// The suffix ENDS at the inner edge, not in an arbitrary column.
	end := colOfCurrent + len("current")
	wantEnd := 1 + max(8, m.retargetBoxWidth()-2)
	if end != wantEnd {
		t.Errorf("the suffix ends at column %d, want %d (the inner border): the padding does not push the suffix", end, wantEnd)
	}
}

func TestALongNameNeitherOverflowsNorStepsOnTheSuffix(t *testing.T) {
	long := "feature/" + strings.Repeat("name", 20)
	m := branchesOfThePopup(t, "", "main", long, "wip")
	m.retarget.cursor = 1
	m.retarget.win = 0
	inner := max(8, m.retargetBoxWidth()-2)

	var widths []int
	vistos := map[string]bool{}
	for _, l := range linesOfThePopup(m) {
		widths = append(widths, ansi.StringWidth(l))
		for _, r := range []string{"main", "wip"} {
			if strings.Contains(l, r) {
				vistos[r] = true
			}
		}
	}
	for i := 1; i < len(widths); i++ {
		if widths[i] != widths[0] {
			t.Errorf("line %d measures %d columns and line 0 measures %d with a huge name: it leaves the box", i, widths[i], widths[0])
		}
	}
	for _, r := range []string{"main", "wip"} {
		if !vistos[r] {
			t.Errorf("with a huge name in the list, %q disappeared from the box", r)
		}
	}
	if !strings.Contains(stripANSI(m.retargetOverlay2()), "feature/") {
		t.Error("the clipped name lost its beginning, which is what makes it recognisable")
	}
	_ = inner
}

func TestHowManyRowsArePaintedNotOneMoreNorOneLess(t *testing.T) {
	many := []string{"main"}
	for i := range 40 {
		many = append(many, "branch/"+string(rune('a'+i%26))+string(rune('a'+i/26)))
	}
	for _, h := range []int{10, 14, 20, 40} {
		m := sizedRetarget(t, 90, h, many...)
		want := min(m.retargetRows(), len(m.retarget.view))
		if got := countBranchRows(m); got != want {
			t.Errorf("height %d: %d branch rows painted, want %d (the ones the box reserved: %d)",
				h, got, want, m.retargetRows())
		}
	}
	few := []string{"main", "other", "wip"}
	for _, h := range []int{10, 20, 40} {
		m := sizedRetarget(t, 90, h, few...)
		if got := countBranchRows(m); got != len(few) {
			t.Errorf("height %d with %d branches: %d painted, want %d", h, len(few), got, len(few))
		}
	}
}

func countBranchRows(m Model) int {
	n := 0
	for _, l := range linesOfThePopup(m) {
		if strings.Contains(l, "▸") || strings.Contains(l, retargetCurrentSuffix) {
			n++
			continue
		}
		for _, r := range m.retarget.view {
			if r != "" && strings.Contains(l, r) && !strings.Contains(l, "from ") {
				n++
				break
			}
		}
	}
	return n
}

// The header names the base even when it does not know it.
func TestTheHeaderNamesTheBaseEvenWhenItDoesNotKnowIt(t *testing.T) {
	m := branchesOfThePopup(t, "", "main", "other")
	if !strings.Contains(stripANSI(m.retargetOverlay2()), "from main") {
		t.Errorf("the header should say which base you leave from: %q", stripANSI(m.retargetOverlay2()))
	}

	m.retarget.item.TargetBranch = ""
	txt := stripANSI(m.retargetOverlay2())
	if !strings.Contains(txt, "from unknown") {
		t.Errorf("with no known base the header should say unknown: %q", txt)
	}
	if strings.Contains(txt, "from  ") || strings.Contains(txt, "from ·") {
		t.Errorf("with no known base the header left a gap: %q", txt)
	}
	// And with no known base no row carries a "current" marker.
	if strings.Contains(txt, "current") {
		t.Errorf("with no known base the base badge should not come out: %q", txt)
	}
}

// With the filter off the box says how many branches the repo has.
func TestTheBranchCountDistinguishesFilteredFromUnfiltered(t *testing.T) {
	branches := []string{"main", "fix/one", "fix/two", "wip", "other"}
	complete := []string{"main", "fix/one", "fix/two", "wip", "other"}

	m := branchesOfThePopup(t, "", complete...)
	txt := stripANSI(m.retargetOverlay2())
	if !strings.Contains(txt, "5 branches") {
		t.Errorf("without a filter the box should say how many there are: %q", txt)
	}
	if strings.Contains(txt, "match") {
		t.Errorf("without a filter there should be no match formula: %q", txt)
	}

	m = branchesOfThePopup(t, "fix", complete...)
	txt = stripANSI(m.retargetOverlay2())
	if !strings.Contains(txt, "2 of 5 branches match") {
		t.Errorf("with a filter the box should say how many match out of how many there are: %q", txt)
	}

	// And the singular: "1 branch", because a popup that says "1 branches" looks broken.
	if got := pluralBranches(1); got != "1 branch" {
		t.Errorf("pluralBranches(1) = %q, want %q", got, "1 branch")
	}
	if got := pluralBranches(0); got != "0 branches" {
		t.Errorf("pluralBranches(0) = %q, want %q", got, "0 branches")
	}
	if got := pluralBranches(2); got != "2 branches" {
		t.Errorf("pluralBranches(2) = %q, want %q", got, "2 branches")
	}
	m = branchesOfThePopup(t, "wip", complete...)
	if !strings.Contains(stripANSI(m.retargetOverlay2()), "1 of 5 branches match") {
		t.Errorf("with a single match: %q", stripANSI(m.retargetOverlay2()))
	}
	_ = branches
}

// Without a filter the field shows a dim placeholder.
func TestTheFilterFieldDistinguishesThePlaceholderFromTheText(t *testing.T) {
	m := branchesOfThePopup(t, "", "main", "other")
	txt := stripANSI(m.retargetOverlay2())
	if !strings.Contains(txt, "type to search") {
		t.Errorf("without a filter the field should offer the placeholder: %q", txt)
	}

	m = branchesOfThePopup(t, "ot", "main", "other")
	txt = stripANSI(m.retargetOverlay2())
	if strings.Contains(txt, "type to search") {
		t.Errorf("with a filter set the placeholder should not remain: %q", txt)
	}
	if !strings.Contains(txt, "ot") {
		t.Errorf("with a filter the field should show what was typed: %q", txt)
	}

	// A filter of only spaces trims to empty, so the view is the WHOLE list.
	m.retarget.query = "   "
	m.applyQuery()
	if len(m.retarget.view) != len(m.retarget.all) {
		t.Errorf("a filter of only spaces gave %d of %d branches: the filter is trimmed and does not leave the view empty",
			len(m.retarget.view), len(m.retarget.all))
	}
	if !strings.Contains(stripANSI(m.retargetOverlay2()), "2 of 2 branches match") {
		t.Errorf("a filter of only spaces is not filtering anything: %q", stripANSI(m.retargetOverlay2()))
	}
	// And a one-character query, which is where a `query == ""` written by mistake would break.
	for _, q := range []string{"x", "f", "1"} {
		m.retarget.query = q
		txt := stripANSI(m.retargetOverlay2())
		if strings.Contains(txt, "type to search") || !strings.Contains(txt, q) {
			t.Errorf("with the filter %q the box does not show what was typed: %q", q, txt)
		}
	}
}

// A filter that matches nothing leaves the list empty and the box says so.
func TestThePopupSaysWhenItFindsNothing(t *testing.T) {
	m := branchesOfThePopup(t, "", "main", "other")
	m.retarget.query = "noexiste"
	m.applyQuery()
	txt := stripANSI(m.retargetOverlay2())
	if !strings.Contains(txt, "no branch matches the filter") {
		t.Errorf("a filter with no matches should explain it: %q", txt)
	}
	// The forge's error has its own place: it is another cause with another repair.
	m.retarget.query = ""
	m.retarget.errMsg = "gh: Not Found (HTTP 404)"
	txt = stripANSI(m.retargetOverlay2())
	if !strings.Contains(txt, "404") {
		t.Errorf("the forges error should be visible: %q", txt)
	}
	if strings.Contains(txt, "no branch matches") {
		t.Errorf("with a forges error the filter message should not come out: %q", txt)
	}
	// The error WINS over the list: a repo that errored has no list to show.
	m.retarget.view = []string{"what would have been left before"}
	if txt = stripANSI(m.retargetOverlay2()); !strings.Contains(txt, "404") {
		t.Errorf("with an error and a list, the box should show the error: %q", txt)
	}
}
