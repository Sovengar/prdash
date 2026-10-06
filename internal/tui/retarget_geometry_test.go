package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

func sizedRetarget(t *testing.T, w, h int, branches ...string) Model {
	t.Helper()
	m, _ := retargetFixture(t, branches...)
	return send(t, m, tea.WindowSizeMsg{Width: w, Height: h})
}

// The popup draws ON TOP, in both dimensions.
func TestTheRetargetPopupRespectsTheTerminalInBothDimensions(t *testing.T) {
	for _, w := range []int{20, 40, 64, 80, 200} {
		for _, h := range []int{5, 8, 12, 20, 60} {
			m := sizedRetarget(t, w, h, "main", "release/2.0", "fix/one")
			box := m.retargetOverlay2()
			if box == "" {
				continue
			}
			lines := strings.Split(stripANSI(box), "\n")
			width := m.retargetBoxWidth()
			for i, l := range lines {
				if got := ansi.StringWidth(l); got != width {
					t.Errorf("terminal %dx%d: line %d measures %d columns and the box %d: %q",
						w, h, i, got, width, l)
				}
			}
			// The width never exceeds the popup's design width however wide the terminal.
			if width > retargetChooserWidth {
				t.Errorf("terminal %dx%d: box of %d columns, want <= %d", w, h, width, retargetChooserWidth)
			}
			if want := min(m.contentWidth(), retargetChooserWidth); width != want {
				t.Errorf("terminal %dx%d: box of %d columns, want %d (content bounded)", w, h, width, want)
			}
		}
	}
}

// The number of branch rows is what decides the height.
func TestThePopupsRowsFitBetweenTheMarginAndTheCeiling(t *testing.T) {
	branches := []string{"main", "a/1", "a/2", "a/3", "a/4", "a/5", "a/6", "a/7", "a/8", "a/9", "a/10", "a/11", "a/12", "a/13"}
	for _, h := range []int{4, 6, 8, 10, 14, 20, 40, 100} {
		m := sizedRetarget(t, 80, h, branches...)
		got := m.retargetRows()

		if got < retargetMinRows {
			t.Errorf("height %d: %d rows, want >= %d (with no rows the popup looks empty)", h, got, retargetMinRows)
		}
		if got > retargetRows {
			t.Errorf("altura %d: %d rows, want <= %d", h, got, retargetRows)
		}
		want := h - 2*retargetMargin - retargetChrome
		if want < retargetMinRows {
			want = retargetMinRows
		}
		if want > retargetRows {
			want = retargetRows
		}
		if got != want {
			t.Errorf("height %d: %d rows, want %d (h - 2*margin - frame, bounded to [%d, %d])",
				h, got, want, retargetMinRows, retargetRows)
		}
	}
}

// Filter and arrows share the same window calculation.
func TestMovingTheCursorRecalculatesTheWindow(t *testing.T) {
	m := sizedRetarget(t, 80, 60, "a", "b", "c", "d", "e")
	if win := m.retargetWindow(); win != 0 {
		t.Errorf("with the whole list win = %d, want 0", win)
	}

	rows := m.retargetRows()
	m.retarget.view = seqBranches(rows)
	m.retarget.cursor, m.retarget.win = 0, 0
	if win := m.retargetWindow(); win != 0 {
		t.Errorf("list of %d rows with window of %d and cursor above: win = %d, want 0: the first branch would be hidden",
			rows, rows, win)
	}

	m.retarget.view = seqBranches(rows + 4)
	for cursor := 0; cursor < rows+4; cursor++ {
		m.retarget.cursor, m.retarget.win = cursor, 0
		// This is what moveRetargetCursor does: the arithmetic lives in retargetWindow and the render
		//reads m.retarget.win.
		m.retarget.win = m.retargetWindow()
		visibles, start := m.retargetVisible()
		if cursor < start || cursor >= start+len(visibles) {
			t.Errorf("cursor %d: the window [%d, %d) does not contain it (visible %d)", cursor, start, start+len(visibles), len(visibles))
		}
		if start+len(visibles) > len(m.retarget.view) {
			t.Errorf("cursor %d: the window [%d, %d) goes past the %d branches", cursor, start, start+len(visibles), len(m.retarget.view))
		}
	}

	m.retarget.cursor, m.retarget.win = rows+3, 0
	m.moveRetargetCursor(-(rows + 3))
	if m.retarget.cursor != 0 {
		t.Errorf("cursor = %d after going all the way up, want 0", m.retarget.cursor)
	}
	if m.retarget.win != 0 {
		t.Errorf("win = %d after going all the way up, want 0", m.retarget.win)
	}
	m.retarget.view = []string{"a", "b"}
	m.retarget.cursor, m.retarget.win = 1, 0
	m.moveRetargetCursor(5)
	if m.retarget.win != 0 {
		t.Errorf("list of 2 rows with window of %d: win = %d, want 0", rows, m.retarget.win)
	}
}

func seqBranches(n int) []string {
	out := make([]string, 0, n)
	for i := range n {
		out = append(out, "branch/"+string(rune('a'+i%26))+string(rune('a'+i/26)))
	}
	return out
}

func TestTheConfirmationArrowFallsInAFixedColumn(t *testing.T) {
	column := func(base string) int {
		t.Helper()
		m, _ := retargetFixture(t, base, "destino")
		m.retarget.item.TargetBranch = base
		m.retarget.cursor = 1
		m.retarget.chosen = "destino"
		m.retarget.state = retargetConfirm

		box := stripANSI(m.retargetConfirmBox())
		wantCol := 2 + (m.retargetBoxWidth()-6)/2
		seen := -1
		for _, l := range strings.Split(box, "\n") {
			i := strings.Index(l, "→")
			if i < 0 {
				continue
			}
			col := ansi.StringWidth(l[:i])
			if ansi.StringWidth(base) <= (m.retargetBoxWidth()-6)/2 && col != wantCol {
				t.Errorf("base %q: the arrow falls at column %d, want %d (half box, the bases padding): %q",
					base, col, wantCol, l)
			}
			if !strings.Contains(l, "→ "+m.retarget.chosen) {
				t.Errorf("base %q: the arrow is not stuck to the target: %q", base, l)
			}
			seen = col
		}
		if seen < 0 {
			t.Fatalf("base %q: the box has no arrow: %q", base, box)
		}
		return seen
	}

	want := column("main")
	for _, base := range []string{"main", "release/2.0", "feature/x", "x", "fix/hunk"} {
		if got := column(base); got != want {
			t.Errorf("the arrows column depends on the base: %q gave %d and %q gave %d", "main", want, base, got)
		}
	}
	larga := "feature/" + strings.Repeat("x", 40)
	if column(larga) <= want {
		t.Errorf("a base of %d columns should push the arrow past %d", len(larga), want)
	}
}

func TestTheConfirmationBoxNamesTheTwoBranches(t *testing.T) {
	m, _ := retargetFixture(t, "main", "release/2.0")
	m.retarget.cursor = 1
	m.retarget.chosen = "release/2.0"
	m.retarget.state = retargetConfirm

	box := stripANSI(m.retargetConfirmBox())
	for _, want := range []string{"main", "release/2.0", "enter apply", "esc back"} {
		if !strings.Contains(box, want) {
			t.Errorf("the box should contain %q: %q", want, box)
		}
	}
	// Without it the box only lists the keys and the user cannot tell what stopped being true.
	if !strings.Contains(box, "recomputed") {
		t.Errorf("the box should say it is recalculated: %q", box)
	}

	withoutBase := m
	withoutBase.retarget.item.TargetBranch = ""
	box = stripANSI(withoutBase.retargetConfirmBox())
	if !strings.Contains(box, "unknown") {
		t.Errorf("with no known base the box should say unknown: %q", box)
	}
	if strings.Contains(box, " →  ") || strings.Contains(box, "(→") {
		t.Errorf("with no known base the box left a gap in the arrow: %q", box)
	}
}

// The in-progress warning is the only thing that distinguishes one retarget from another.
func TestRetargetProgressNoticeDistinguishesTheTwoBranches(t *testing.T) {
	withBase := stripANSI(retargetProgressNotice("main", "release/2.0"))
	if !strings.Contains(withBase, "main") || !strings.Contains(withBase, "release/2.0") {
		t.Errorf("the notice should name the two branches: %q", withBase)
	}
	withoutBase := stripANSI(retargetProgressNotice("", "release/2.0"))
	if !strings.Contains(withoutBase, "release/2.0") {
		t.Errorf("with no source base the notice should name the target: %q", withoutBase)
	}
	if strings.Contains(withoutBase, "( →") || strings.Contains(withoutBase, "(→  ") {
		t.Errorf("with no source base the notice left a gap: %q", withoutBase)
	}
	if withBase == withoutBase {
		t.Errorf("with and without a source base the notice is the same: %q", withBase)
	}
}
