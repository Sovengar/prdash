package tui

import (
	"strconv"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/charmbracelet/x/ansi"

	"prdash/internal/forge/model"
	"prdash/internal/inbox"
	"prdash/internal/testutil"
)

func manyItems(n int) []model.Item {
	items := make([]model.Item, 0, n)
	for i := 1; i <= n; i++ {
		items = append(items, mkItem("github", "github.com", "acme/widget", "Item "+strconv.Itoa(i), i, ""))
	}
	return items
}

func longModel(t *testing.T, n int) Model {
	t.Helper()
	m := newTestModel(t, ghAdapter())
	return send(t, m, page(1, "github", "github.com", model.SectionReview, model.ReviewRequested, manyItems(n), false))
}

func visibleLines(view string) []string {
	return strings.Split(stripANSI(view), "\n")
}

func TestViewBoxesEverySection(t *testing.T) {
	m := newTestModel(t, ghAdapter())
	m = send(t, m, page(1, "github", "github.com", model.SectionReview, model.ReviewRequested, []model.Item{
		mkItem("github", "github.com", "acme/widget", "Add widget", 1, ""),
	}, false))

	lines := visibleLines(m.View().Content)
	if len(lines) != m.height {
		t.Fatalf("the view has %d lines, want %d (the terminal height)", len(lines), m.height)
	}

	for _, want := range []string{"PRDash", "Assigned (1)", "Keybinds"} {
		if !hasBoxTitle(lines, want) {
			t.Errorf("the box %q is missing:\n%s", want, strings.Join(lines, "\n"))
		}
	}
	if hasBoxTitle(lines, "Inbox") {
		t.Errorf("the Inbox title should no longer exist: the legend replaced it:\n%s", strings.Join(lines, "\n"))
	}
	if !hasBoxTitle(lines, "acme/widget#1") {
		t.Errorf("the detail box is not titled with the items reference:\n%s", strings.Join(lines, "\n"))
	}

	tops, bottoms := 0, 0
	for _, l := range lines {
		if strings.HasPrefix(l, "╭") {
			tops++
		}
		if strings.HasPrefix(l, "╰") {
			bottoms++
		}
	}
	if tops != bottoms || tops == 0 {
		t.Errorf("cajas descompensadas: %d aperturas ╭ y %d cierres ╰", tops, bottoms)
	}
}

func hasBoxTitle(lines []string, title string) bool {
	for _, l := range lines {
		if strings.HasPrefix(l, "╭") && strings.Contains(l, " "+title+" ") {
			return true
		}
	}
	return false
}

func TestViewFitsTerminalHeight(t *testing.T) {
	m := longModel(t, 60)
	lines := visibleLines(m.View().Content)
	if len(lines) != m.height {
		t.Errorf("the view has %d lines, want %d (the terminal height)", len(lines), m.height)
	}
	if !hasBoxTitle(lines, "PRDash") {
		t.Errorf("the header box does not title the app: %q", lines[0])
	}
	if !strings.Contains(lines[len(lines)-2], "quit") {
		t.Errorf("the hints are not on the last content line: %q", lines[len(lines)-2])
	}
	if !strings.HasPrefix(lines[len(lines)-1], "╰") {
		t.Errorf("the view does not end at the bottom border of the keybinds box: %q", lines[len(lines)-1])
	}
}

func TestViewBoxesEveryLineAtTerminalWidth(t *testing.T) {
	m := longModel(t, 30)
	for i, l := range visibleLines(m.View().Content) {
		if w := ansi.StringWidth(l); w != m.width {
			t.Errorf("line %d measures %d columns, want %d:\n%q", i, w, m.width, l)
		}
	}
}

// The inbox sorts newest first, so "Item 60" is the first row and "Item 1" the last.
func TestScrollFollowsCursor(t *testing.T) {
	m := longModel(t, 60)
	if m.scroll != 0 {
		t.Fatalf("initial scroll = %d, want 0", m.scroll)
	}

	m = press(t, m, "end")
	if it, ok := m.selected(); !ok || it.Title != "Item 1" {
		t.Fatalf("with end the cursor should be on the last row, not on %q", it.Title)
	}
	view := stripANSI(m.View().Content)
	if !strings.Contains(view, "Item 1") {
		t.Errorf("the cursor row is not visible:\n%s", view)
	}
	if strings.Contains(view, "Item 60") {
		t.Errorf("the list scrolled too far: the first row should no longer be visible:\n%s", view)
	}
	if m.scroll == 0 {
		t.Error("the list did not scroll with the cursor at the end")
	}

	m = press(t, m, "home")
	view = stripANSI(m.View().Content)
	if !strings.Contains(view, "Item 60") {
		t.Errorf("with the cursor at the start the first row must be visible:\n%s", view)
	}
	if strings.Contains(view, "Item 1") {
		t.Errorf("the list did not go back up:\n%s", view)
	}
	if m.scroll != 0 {
		t.Errorf("scroll after home = %d, want 0", m.scroll)
	}
}

func TestPageKeysMoveOneWindow(t *testing.T) {
	m := longModel(t, 60)
	page := m.pageRows()
	if page < 2 {
		t.Fatalf("pageRows = %d, want >= 2", page)
	}

	m = press(t, m, "pgdown")
	if m.cursor != page {
		t.Errorf("cursor after pgdown = %d, want %d", m.cursor, page)
	}
	if m.scroll != page {
		t.Errorf("the window after pgdown = %d, want %d (it must advance a whole page)", m.scroll, page)
	}
	m = press(t, m, "pgup")
	if m.cursor != 0 {
		t.Errorf("cursor after pgup = %d, want 0", m.cursor)
	}
	if m.scroll != 0 {
		t.Errorf("the window after pgup = %d, want 0", m.scroll)
	}

	m = press(t, m, "pgup")
	if m.cursor != 0 {
		t.Errorf("cursor after pgup at the top = %d, want 0", m.cursor)
	}
	for range 5 {
		m = press(t, m, "pgdown")
	}
	if m.cursor != len(m.rows())-1 {
		t.Errorf("cursor after several pgdown = %d, want %d", m.cursor, len(m.rows())-1)
	}
}

// These three cases are the ones that break if the condition is inverted or the boundary moves.
func TestScrollForSettlesTheWindowOnTheCursor(t *testing.T) {
	cases := []struct {
		name                         string
		current, target, total, view int
		want                         int
	}{
		{"the cursor is already visible", 0, 3, 60, 10, 0},
		{"the last row of the window fits", 0, 9, 60, 10, 0},
		{"one above the window does scroll", 0, 10, 60, 10, 1},
		{"the cursor is above", 5, 1, 60, 10, 1},
		{"the cursor is the first line", 5, 0, 60, 10, 0},
		{"the last row sticks to the border", 0, 59, 60, 10, 50},
		{"the content fits whole", 0, 5, 8, 10, 0},
		{"with no row to follow it leaves the scroll", 7, -1, 60, 10, 7},
		{"a negative target does not drag the scroll to zero", 0, -1, 60, 10, 0},
		{"the window is bounded by the content", 40, 40, 12, 10, 2},
		{"a window bigger than the content goes above", 0, 0, 4, 10, 0},
		{"the scroll never stays negative", -5, 0, 60, 10, 0},
		{"the scroll never goes past the end", 99, 59, 60, 10, 50},
		// A zero-height window is degenerate but defined, and tells the `target <= current` boundary mutant.
		{"view zero: the arithmetic stays defined", 5, 5, 10, 0, 6},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := scrollFor(c.current, c.target, c.total, c.view); got != c.want {
				t.Errorf("scrollFor(%d, %d, %d, %d) = %d, want %d",
					c.current, c.target, c.total, c.view, got, c.want)
			}
		})
	}
}

func TestCursorLineLocatesTheCursorRow(t *testing.T) {
	lines := []listLine{
		{text: "prefijo", row: -1},
		{text: "notice", row: -1},
		{text: "header", row: -1},
		{text: "row 0", row: 0},
		{text: "row 1", row: 1},
		{text: "loading more", row: -1},
	}
	cases := map[int]int{0: 3, 1: 4, 2: -1, 99: -1}
	for cursor, want := range cases {
		if got := cursorLine(lines, cursor); got != want {
			t.Errorf("cursorLine(cursor=%d) = %d, want %d", cursor, got, want)
		}
	}
	if got := cursorLine([]listLine{{text: "empty", row: -1}}, 0); got != -1 {
		t.Errorf("cursorLine on a list with no rows = %d, want -1", got)
	}
	if got := cursorLine(nil, 0); got != -1 {
		t.Errorf("cursorLine(nil, 0) = %d, want -1", got)
	}
}

func TestVisibleListBoundsTheWindowToTheContent(t *testing.T) {
	lines := make([]listLine, 10)
	for i := range lines {
		lines[i] = listLine{row: i}
	}

	got := visibleList(lines, 3, 4)
	if len(got) != 4 || got[0].row != 3 || got[3].row != 6 {
		t.Errorf("visibleList(3, 4) = %d lines starting at %d", len(got), got[0].row)
	}
	got = visibleList(lines, 99, 4)
	if len(got) != 4 || got[0].row != 6 {
		t.Errorf("visibleList(99, 4) = %d lines starting at %d, want 4 from 6", len(got), got[0].row)
	}
	six := lines[:6]
	got = visibleList(six, 0, 8)
	if len(got) != 6 || got[0].row != 0 {
		t.Errorf("visibleList(6 lines, view 8) = %d lines, want 6 from 0", len(got))
	}
	got = visibleList(six, 3, 3)
	if len(got) != 3 || got[0].row != 3 {
		t.Errorf("visibleList(6 lines, 3, 3) = %d lines from %d, want 3 from 3", len(got), got[0].row)
	}
	got = visibleList(lines, -5, 3)
	if len(got) != 3 || got[0].row != 0 {
		t.Errorf("visibleList(-5, 3) = %d lines starting at %d, want 3 from 0", len(got), got[0].row)
	}
	got = visibleList(lines, 99, 3)
	if len(got) != 3 || got[0].row != 7 {
		t.Errorf("visibleList(99, 3) = %d lines starting at %d, want 3 from 7", len(got), got[0].row)
	}
	if visibleList(lines, 0, 0) != nil || visibleList(nil, 0, 5) != nil {
		t.Error("with no window or no lines visibleList should return nil")
	}
}

// Rows are numbered with `row` so the cursor and scroll find them; the scroll depends on it matching.
func TestListLinesComposeTheBodyInOrder(t *testing.T) {
	m := newTestModel(t, ghAdapter())
	m = send(t, m, page(1, "github", "github.com", model.SectionReview, model.ReviewRequested, []model.Item{
		mkItem("github", "github.com", "acme/widget", "One", 1, ""),
		mkItem("github", "github.com", "acme/widget", "Two", 2, ""),
	}, false))

	lines := m.listLines(m.contentWidth())
	var rows []int
	for _, l := range lines {
		rows = append(rows, l.row)
	}
	want := []int{-1, -1, 0, 1}
	if len(rows) != len(want) {
		t.Fatalf("lines = %v, want %v", rows, want)
	}
	for i := range want {
		if rows[i] != want[i] {
			t.Fatalf("rows = %v, want %v (row %d must be row %d)", rows, want, i, want[i])
		}
	}

	i0 := cursorLine(lines, 0)
	if i0 < 0 || lines[i0].row != 0 {
		t.Fatalf("cursorLine(0) = %d", i0)
	}
	withCursor := stripANSI(lines[i0].text)
	withoutCursor := stripANSI(lines[cursorLine(lines, 1)].text)
	if withCursor == withoutCursor {
		t.Errorf("the cursor row is not distinguishable from the other:\n%s", withCursor)
	}

	// row: -1, so neither the cursor nor the scroll can mistake it for an item.
	m2 := newTestModel(t, ghAdapter())
	m2 = send(t, m2, page(1, "github", "github.com", model.SectionReview, model.ReviewRequested, []model.Item{
		mkItem("github", "github.com", "acme/widget", "One", 1, ""),
	}, true))
	lines = m2.listLines(m2.contentWidth())
	last := lines[len(lines)-1]
	if last.row != -1 || !strings.Contains(stripANSI(last.text), "loading more") {
		t.Errorf("the last line = %+v, want the pagination indicator with row -1", last)
	}
}

func TestListLinesEmptyPaintsTheEmptyState(t *testing.T) {
	m := newTestModel(t, ghAdapter())
	m = send(t, m, page(1, "github", "github.com", model.SectionReview, model.ReviewRequested, nil, false))

	lines := m.listLines(m.contentWidth())
	if len(lines) != 1 {
		t.Fatalf("lines = %d (%v), want only the empty state one", len(lines), lines)
	}
	if lines[0].row != -1 || !strings.Contains(stripANSI(lines[0].text), "(empty)") {
		t.Errorf("line = %+v, want the empty state with row -1", lines[0])
	}
	m = send(t, m, pageMsg{
		cycle:    1,
		key:      streamKey{forge: "github", section: model.SectionReview, kind: model.ReviewRequested},
		warnings: []model.Warning{{Forge: "github", Section: model.SectionReview, Kind: "network", Msg: "dial tcp: timeout"}},
	})
	lines = m.listLines(m.contentWidth())
	joined := ""
	for _, l := range lines {
		joined += stripANSI(l.text)
	}
	if !strings.Contains(joined, "github: could not be queried") {
		t.Errorf("with a notice the forges notice should be painted:\n%s", joined)
	}
	if strings.Contains(joined, "(empty)") {
		t.Errorf("with a notice the empty state should not be painted too:\n%s", joined)
	}
	if lines[0].row != -1 {
		t.Errorf("the notice should be the first line with row -1, it is %+v", lines[0])
	}
}

// Not a test of syncScroll's `view <= 0` guard: it cannot be reached, noted so nobody re-adds it.
func TestSyncScrollDoesNotBreakInTinyTerminals(t *testing.T) {
	m := longModel(t, 60)
	m = press(t, m, "end")
	if it, ok := m.selected(); !ok || it.Title != "Item 1" {
		t.Fatalf("with end the cursor should be on the last row, not on %q", it.Title)
	}

	for _, h := range []int{40, 12, 8, 5, 3, 1} {
		m.height = h
		m.syncScroll()
		lay := m.layout()
		if lay.bodyLines < 1 {
			t.Fatalf("height=%d: bodyLines = %d, but the layout guarantees >= 1", h, lay.bodyLines)
		}
		lines := m.listLines(m.contentWidth())
		if m.scroll < 0 || m.scroll > max(0, len(lines)-1) {
			t.Errorf("height=%d: scroll = %d out of range with %d lines", h, m.scroll, len(lines))
		}
		if cl := cursorLine(lines, m.cursor); cl >= 0 {
			vis := visibleList(lines, m.scroll, lay.bodyLines)
			found := false
			for _, v := range vis {
				if v.row == m.cursor {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("height=%d: the cursor row is not in the window (line %d of %d, %d visible)",
					h, cl, len(lines), len(vis))
			}
		}
	}
}

// pad on a string that already fits must add nothing: it would push the cell past its column.
func TestPadAndTruncateMeasureInRunesAndInWidths(t *testing.T) {
	if got := pad("ab", 4); got != "ab  " {
		t.Errorf("pad(ab,4) = %q, want \"ab  \"", got)
	}
	if got := pad("ab", 2); got != "ab" {
		t.Errorf("pad with the exact string = %q, want %q (it does not overpad)", got, "ab")
	}
	if got := pad("abcd", 2); got != "abcd" {
		t.Errorf("pad of a longer string = %q, want unchanged (clipping wins)", got)
	}
	if got := utf8.RuneCountInString(pad("áé", 4)); got != 4 {
		t.Errorf("pad with accents = %d runes, want 4", got)
	}
	if got := pad("👍", 3); utf8.RuneCountInString(got) != 3 {
		t.Errorf("pad with an emoji = %d runes, want 3", utf8.RuneCountInString(got))
	}

	if got := truncate("abcdef", 4); got != "abc…" {
		t.Errorf("truncate(abcdef,4) = %q, want \"abc…\"", got)
	}
	if got := utf8.RuneCountInString(truncate("abcdefgh", 5)); got != 5 {
		t.Errorf("truncate must give EXACTLY 5 runes, gave %d", got)
	}
	if got := truncate("abc", 3); got != "abc" {
		t.Errorf("truncate of what exactly fits = %q, want unchanged", got)
	}
	if got := truncate("abc", 10); got != "abc" {
		t.Errorf("truncate of what has spare room = %q, want unchanged", got)
	}
	if got := truncate("abc", 0); got != "" {
		t.Errorf("truncate(abc,0) = %q, want empty", got)
	}
	if got := truncate("abc", 1); got != "…" {
		t.Errorf("truncate(abc,1) = %q, want only the ellipsis (two columns in one)", got)
	}
	if got := truncate("abc", -1); got != "" {
		t.Errorf("truncate with negative width = %q, want empty", got)
	}
	if got := truncate("áéíóú", 3); got != "áé…" {
		t.Errorf("truncate with accents = %q, want \"áé…\"", got)
	}
}

// Shared with the graphics layer on purpose: separate positions would land in different rectangles.
func TestCenteredOriginPlacesTheBox(t *testing.T) {
	cases := []struct {
		name                      string
		width, height, boxW, boxH int
		wantX, wantY              int
	}{
		{"box centered in a larger area", 20, 10, 4, 4, 8, 3},
		{"box that fills the area", 10, 10, 10, 10, 0, 0},
		{"taller than the area", 20, 3, 4, 8, 8, 0},
		{"wider than the area", 3, 10, 10, 2, 0, 4},
		{"one row of difference", 20, 11, 4, 4, 8, 3},
		{"odd above", 21, 11, 4, 4, 8, 3},
		{"area of one line", 20, 1, 4, 1, 8, 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			x, y := centeredOrigin(c.width, c.height, c.boxW, c.boxH)
			if x != c.wantX || y != c.wantY {
				t.Errorf("centeredOrigin(%d,%d,%d,%d) = %d,%d, want %d,%d",
					c.width, c.height, c.boxW, c.boxH, x, y, c.wantX, c.wantY)
			}
			if x < 0 || y < 0 {
				t.Errorf("origen negative: x=%d y=%d", x, y)
			}
		})
	}
}

func TestOverlayCenteredClipsTheBoxThatDoesNotFit(t *testing.T) {
	// A background wider than the scroll: the overlay clips where the box needs room rather than deleting the line.
	background := "aaaa\naaaa\naaaa\naaaa"
	got := overlayCentered(background, "1\n2\n3\n4\n5\n6", 10)
	lines := strings.Split(got, "\n")
	if len(lines) != 4 {
		t.Fatalf("lines = %d, want 4 (the background does not grow):\n%s", len(lines), got)
	}
	for i, want := range []string{"aaaa1", "aaaa2", "aaaa3", "aaaa4"} {
		if lines[i] != want {
			t.Errorf("line %d = %q, want %q (the box is painted over the background)", i, lines[i], want)
		}
	}
	if got := overlayCentered(background, "", 10); got != background {
		t.Errorf("empty box = %q, want the view intact", got)
	}
	got = overlayCentered("aaaaaaaaaa", "0123456789", 6)
	line := strings.Split(got, "\n")[0]
	if line != "012345aaaa" {
		t.Errorf("line = %q, want \"012345aaaa\" (box clipped to the width, background behind)", line)
	}
}

// Header and rows share the layout on purpose, so writing over the table does not move a column.
func TestTheRowPaintsOnlyTheCellsThatFit(t *testing.T) {
	m := longModel(t, 3)
	lay := newRefLayout([]inbox.Section{{Kind: m.activeSection, Items: m.rows()}}, m.prefixMode)

	for width := 20; width <= 140; width++ {
		m.width = width
		inner := m.contentWidth()
		want := fitColumns(lay, inner-2)
		wantW := 2
		for _, c := range lay.cols[:want] {
			wantW += c.width
		}

		for _, l := range m.listLines(inner) {
			if l.row < 0 {
				continue
			}
			if got := ansi.StringWidth(stripANSI(l.text)); got != wantW {
				t.Fatalf("width %d: row %d measures %d columns, want %d (those of the %d cells that fit)",
					width, l.row, got, wantW, want)
			}
		}
	}
}

// The `inner-2` listLines passes to the header shows in which columns appear, not in the width.
func TestTheHeaderShowsOnlyTheColumnsThatFit(t *testing.T) {
	m := longModel(t, 6)
	for width := 20; width <= 140; width += 2 {
		m.width = width
		inner := m.contentWidth()
		lay := newRefLayout([]inbox.Section{{Kind: m.activeSection, Items: m.rows()}}, m.prefixMode)
		want := fitColumns(lay, inner-2)

		lines := m.listLines(inner)
		hi := -1
		for i, l := range lines {
			if l.row == 0 {
				hi = i
			}
		}
		if hi < 1 {
			t.Fatalf("width %d: I cannot find the first row in %d lines", width, len(lines))
		}
		header := stripANSI(lines[hi-1].text)

		for i := range want {
			if !strings.Contains(header, lay.cols[i].title) {
				t.Fatalf("width %d: the first %d columns are missing in the header, which says %q", width, want, header)
			}
		}
		if want < len(lay.cols) && strings.Contains(header, lay.cols[want].title) {
			t.Fatalf("width %d: column %d (%q) does not fit and should not be in the header: %q",
				width, want, lay.cols[want].title, header)
		}
	}
}

func TestFitColumnsAlwaysLeavesForge(t *testing.T) {
	m := longModel(t, 3)
	lay := newRefLayout([]inbox.Section{{Kind: m.activeSection, Items: m.rows()}}, m.prefixMode)
	total := len(lay.cols)
	if total < 2 {
		t.Fatalf("the layout needs at least FORGE and another column, has %d", total)
	}

	if got := fitColumns(lay, 100_000); got != total {
		t.Errorf("with spare width %d columns fit, gave %d", total, got)
	}
	usado := 0
	for k := 1; k <= total; k++ {
		border := usado + lay.cols[k-1].width
		if got := fitColumns(lay, border); got != k {
			t.Errorf("with the exact width of %d columns %d fit, gave %d", k, k, got)
		}
		if k > 1 {
			if got := fitColumns(lay, border-1); got != k-1 {
				t.Errorf("with width of %d columns %d fit and with %d %d fit, gave %d", k, k, k-1, k-1, got)
			}
		}
		usado = border
	}
	for _, w := range []int{0, 1, -5, -100} {
		if got := fitColumns(lay, w); got != 1 {
			t.Errorf("fitColumns(%d) = %d, want 1 (FORGE always fits)", w, got)
		}
	}
}

func TestTheCursorMarkerGoesInItsOwnRow(t *testing.T) {
	m := longModel(t, 4)

	for cursor := range 4 {
		m.cursor = cursor
		lines := m.listLines(m.contentWidth())
		for _, l := range lines {
			if l.row < 0 {
				continue
			}
			plain := stripANSI(l.text)
			marked := strings.Contains(plain, "▸")
			if want := l.row == cursor; marked != want {
				t.Errorf("row %d with cursor at %d: marked=%v, want %v (%q)", l.row, cursor, marked, want, plain)
			}
		}
	}
}

// Both items share UpdatedAt, so their order is deterministic by number.
func TestDetailPaneShowsSelectedItem(t *testing.T) {
	m := newTestModel(t, ghAdapter(), &testutil.FakeAdapter{ForgeName: "gitlab", HostName: "gitlab.example.com"})
	first := mkItem("github", "github.com", "acme/widget", "GitHub PR", 1, "")
	second := mkItem("gitlab", "gitlab.example.com", "grp/proj", "GitLab MR", 2, "")
	first.UpdatedAt = time.Unix(1000, 0)
	second.UpdatedAt = time.Unix(1000, 0)
	m = send(t, m, page(1, "github", "github.com", model.SectionReview, model.ReviewRequested, []model.Item{first}, false))
	m = send(t, m, page(1, "gitlab", "gitlab.example.com", model.SectionReview, model.ReviewRequested, []model.Item{second}, false))

	view := stripANSI(m.View().Content)
	for _, want := range []string{"GitHub PR", "github@github.com", "Item", "Author", "State"} {
		if !strings.Contains(view, want) {
			t.Errorf("the panel does not contain %q:\n%s", want, view)
		}
	}

	m = press(t, m, "down")
	view = stripANSI(m.View().Content)
	for _, want := range []string{"GitLab MR", "gitlab@gitlab.example.com"} {
		if !strings.Contains(view, want) {
			t.Errorf("after moving the cursor the panel does not contain %q:\n%s", want, view)
		}
	}
	if strings.Contains(view, "github@github.com") {
		t.Errorf("the panel still shows the previous items forge:\n%s", view)
	}
}

func TestDetailPaneEmptyWithoutSelection(t *testing.T) {
	m := newTestModel(t, ghAdapter())
	view := stripANSI(m.View().Content)
	if !strings.Contains(view, "no selection") {
		t.Errorf("with an empty inbox the panel should warn:\n%s", view)
	}
	if !hasBoxTitle(visibleLines(m.View().Content), "Detail") {
		t.Errorf("the panel box should be titled Detail with no selection:\n%s", view)
	}
}

func TestDetailPaneFitsShortScreenInTwoColumns(t *testing.T) {
	m := newTestModel(t, ghAdapter())
	m.width, m.height = 120, 24
	m = send(t, m, page(1, "github", "github.com", model.SectionReview, model.ReviewRequested, []model.Item{
		mkItem("github", "github.com", "acme/widget", "Add widget", 1, ""),
	}, false))

	detail := m.layout().detailLines
	if detail != 9 {
		t.Fatalf("panel of %d lines, want 9 (the 40%% of 24)", detail)
	}
	it, ok := m.selected()
	lines := m.detailLines(it, ok, detail)
	if len(lines) != detail {
		t.Fatalf("the panel returned %d lines, want %d", len(lines), detail)
	}
	joined := stripANSI(strings.Join(lines, "\n"))
	for _, want := range []string{"Add widget", "Item:", "Forge:", "github@github.com", "Author:", "State:", "Role:"} {
		if !strings.Contains(joined, want) {
			t.Errorf("the two column panel lost %q:\n%s", want, joined)
		}
	}
}

func TestDetailPaneClipsOnlyWhenTooSmall(t *testing.T) {
	m := longModel(t, 1)
	it, ok := m.selected()
	clipped := m.detailLines(it, ok, 4)
	if len(clipped) != 4 {
		t.Fatalf("the clipping returned %d lines, want 4", len(clipped))
	}
	joined := stripANSI(strings.Join(clipped, "\n"))
	for _, want := range []string{"Review:", "Role:"} {
		if !strings.Contains(joined, want) {
			t.Errorf("the clipping lost %q:\n%s", want, joined)
		}
	}
	if strings.Contains(joined, "Forja:") || strings.Contains(joined, "Item:") {
		t.Errorf("the clipping should have dropped the details header:\n%s", joined)
	}
}

func TestDetailShowsDiffStat(t *testing.T) {
	it := mkItem("gitlab", "gitlab.example.com", "grp/proj", "MR", 1015, "")
	it.Diff = model.DiffStat{Additions: 42, Deletions: 1, Files: 2, Known: true}
	m := newTestModel(t, ghAdapter(), &testutil.FakeAdapter{ForgeName: "gitlab", HostName: "gitlab.example.com"})
	m = send(t, m, page(1, "gitlab", "gitlab.example.com", model.SectionReview, model.ReviewRequested, []model.Item{it}, false))

	view := stripANSI(m.View().Content)
	if !strings.Contains(view, "+42 -1 (2 files)") {
		t.Errorf("the detail should show the aggregated diffstat:\n%s", view)
	}
}

func TestDetailDiffUnknownAndEmpty(t *testing.T) {
	cases := []struct {
		name string
		d    model.DiffStat
		want string
	}{
		{"no data", model.DiffStat{}, "unknown (forge did not report it)"},
		{"no changes", model.DiffStat{Known: true}, "no changes"},
		{"one file", model.DiffStat{Additions: 3, Deletions: 0, Files: 1, Known: true}, "+3 -0 (1 file)"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			it := mkItem("github", "github.com", "acme/widget", "PR", 1, "")
			it.Diff = c.d
			m := newTestModel(t, ghAdapter())
			m = send(t, m, page(1, "github", "github.com", model.SectionReview, model.ReviewRequested, []model.Item{it}, false))
			view := stripANSI(m.View().Content)
			if !strings.Contains(view, c.want) {
				t.Errorf("the detail should show %q:\n%s", c.want, view)
			}
		})
	}
}

func TestDetailDropsDiffBeforeLosingTheTitle(t *testing.T) {
	detail := func(rows int) string {
		m := newTestModel(t, ghAdapter())
		m.width, m.height = 120, 24
		it := mkItem("github", "github.com", "acme/widget", "Add widget", 1, "")
		it.Diff = model.DiffStat{Additions: 42, Deletions: 1, Files: 2, Known: true}
		m = send(t, m, page(1, "github", "github.com", model.SectionReview, model.ReviewRequested, []model.Item{it}, false))
		m.denied[it.ID()] = "the forge refused the action"
		return stripANSI(strings.Join(m.detailPane(rows), "\n"))
	}

	tight := detail(9)
	if strings.Contains(tight, "Diff:") {
		t.Errorf("at 9 rows the diffstat should drop before the title:\n%s", tight)
	}
	if !strings.Contains(tight, "Add widget") {
		t.Errorf("the title should not drop because of the diffstat:\n%s", tight)
	}
	if !strings.Contains(tight, "Role:") {
		t.Errorf("the role says whether the action applies and cannot drop:\n%s", tight)
	}

	if roomy := detail(11); !strings.Contains(roomy, "+42 -1 (2 files)") {
		t.Errorf("at 11 rows the diffstat should be there:\n%s", roomy)
	}
}
