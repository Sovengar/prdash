package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func testView(width, rowCount int) (string, []bool) {
	rows := make([]bool, rowCount)
	text := make([]string, rowCount)
	for i := range rows {
		rows[i] = true
		text[i] = strings.Repeat(".", width)
	}
	return strings.Join(text, "\n"), rows
}

func rowsWithTheBox(t *testing.T, before, after string) []int {
	t.Helper()
	a := strings.Split(before, "\n")
	b := strings.Split(after, "\n")
	if len(a) != len(b) {
		t.Fatalf("the overlay changed the number of rowCount: %d -> %d", len(a), len(b))
	}
	var tocadas []int
	for i := range a {
		if a[i] != b[i] {
			tocadas = append(tocadas, i)
		}
	}
	return tocadas
}

func TestOverlayToastsFallAsLowAsTheyCan(t *testing.T) {
	const width, rowCount = 40, 10
	before, rows := testView(width, rowCount)

	box := "╭──────────╮\n│ alert    │\n╰──────────╯"
	after := overlayToasts(before, []string{box}, width, rows)

	tocadas := rowsWithTheBox(t, before, after)
	if len(tocadas) != 3 {
		t.Fatalf("a box of 3 rowCount touched %d rowCount (%v), want 3: the box was split or repeated",
			len(tocadas), tocadas)
	}
	for i, row := range tocadas {
		if row != rowCount-3+i {
			t.Errorf("the box touched row %d at position %d, want %d: a notice falls as low as it can",
				row, i, rowCount-3+i)
		}
	}

	lines := strings.Split(after, "\n")
	if !strings.Contains(lines[rowCount-3], "╭") {
		t.Errorf("the first row of the box should carry the top corner: %q", lines[rowCount-3])
	}
	if !strings.Contains(lines[rowCount-1], "╰") {
		t.Errorf("the last row of the box does not carry the bottom corner: %q", lines[rowCount-1])
	}
	if !strings.Contains(lines[rowCount-2], "alert") {
		t.Errorf("row %d should carry the notices text: %q", rowCount-2, lines[rowCount-2])
	}

	want := width - ansi.StringWidth("╭──────────╮") - 1
	if col := strings.Index(lines[rowCount-3], "╭"); col != want {
		t.Errorf("the box starts at column %d, want %d: the right air is one column wide", col, want)
	}
}

func TestOverlayToastsStackUpwards(t *testing.T) {
	const width, rowCount = 40, 12
	before, rows := testView(width, rowCount)

	boxA := []string{"╭──╮", "│ A│", "╰──╯"}
	boxB := []string{"╭──╮", "│ B│", "╰──╯"}

	after := overlayToasts(before, []string{strings.Join(boxA, "\n"), strings.Join(boxB, "\n")}, width, rows)

	tocadas := rowsWithTheBox(t, before, after)
	if len(tocadas) != 6 {
		t.Fatalf("two boxes of 3 rowCount touched %d rowCount (%v), want 6: they stepped on each other", len(tocadas), tocadas)
	}

	if a := strings.Split(after, "\n")[rowCount-2]; !strings.Contains(a, "A") {
		t.Errorf("row %d should carry notice A (the first goes at the bottom): %q", rowCount-2, a)
	}
	if b := strings.Split(after, "\n")[rowCount-5]; !strings.Contains(b, "B") {
		t.Errorf("row %d should carry notice B (the second goes on top): %q", rowCount-5, b)
	}
	if middle := strings.Split(after, "\n")[rowCount-4]; strings.Contains(middle, "A") || strings.Contains(middle, "B") {
		t.Errorf("row %d between the two notices has something: %q", rowCount-4, middle)
	}

	for i, row := range tocadas {
		want := rowCount - 6 + i
		if row != want {
			t.Errorf("the touched row %d is %d, want %d: the notices stack with no gaps nor overlaps",
				i, row, want)
		}
	}
}

func TestOverlayToastsDoNotBreakAFrame(t *testing.T) {
	const width, rowCount = 40, 10
	before, rows := testView(width, rowCount)
	for i := rowCount - 4; i < rowCount; i++ {
		rows[i] = false
	}

	box := []string{"╭──╮", "│ A│", "╰──╯"}
	after := overlayToasts(before, []string{strings.Join(box, "\n")}, width, rows)

	tocadas := rowsWithTheBox(t, before, after)
	if len(tocadas) == 0 {
		t.Fatal("the box was not painted anywhere, and there was spare room above")
	}
	for _, row := range tocadas {
		if !rows[row] {
			t.Errorf("the box touched row %d, which takes no notices: it splits a frame", row)
		}
		if row >= rowCount-4 {
			t.Errorf("the box went down to row %d, which is border: %v", row, tocadas)
		}
	}
	if len(tocadas) != 3 || tocadas[0] != rowCount-4-3 {
		t.Errorf("the box landed on rowCount %v, want [%d, %d, %d]: the lowest without stepping on borders",
			tocadas, rowCount-7, rowCount-6, rowCount-5)
	}
}

// landRow's boundary.
func TestOverlayToastsABoxAsTallAsTheWindow(t *testing.T) {
	const width, rowCount = 40, 10
	before, rows := testView(width, rowCount)

	lines := make([]string, rowCount)
	for i := range lines {
		lines[i] = "│" + strings.Repeat("·", width-2) + "│"
	}
	lines[0] = "╭" + strings.Repeat("─", width-2) + "╮"
	lines[rowCount-1] = "╰" + strings.Repeat("─", width-2) + "╯"
	box := strings.Join(lines, "\n")

	after := overlayToasts(before, []string{box}, width, rows)

	tocadas := rowsWithTheBox(t, before, after)
	if len(tocadas) != rowCount {
		t.Fatalf("a box of %d rowCount in a window of %d touched %d rowCount (%v), want %d: the box does not enter whole",
			rowCount, rowCount, len(tocadas), tocadas, rowCount)
	}
	for i, row := range tocadas {
		if row != i {
			t.Errorf("the touched row %d is %d, want %d: the box has to paint up to the top", i, row, i)
		}
	}
	if !strings.Contains(strings.Split(after, "\n")[0], "╭") {
		t.Errorf("the first row does not carry the top corner: the box left without limit")
	}
	if !strings.Contains(strings.Split(after, "\n")[rowCount-1], "╰") {
		t.Errorf("the last row does not carry the bottom corner")
	}
}

// The box's width comes from its FIRST line.
func TestOverlayToastsTheBoxIsAlwaysARectangle(t *testing.T) {
	m := newToastManager()
	for _, level := range []toastLevel{toastSuccess, toastError, toastInfo, toastWarning, toastLevel(9)} {
		for _, message := range []string{"short", strings.Repeat("word ", 20), "with\nbreaks"} {
			for available := 0; available <= 120; available += 7 {
				lines := strings.Split(stripANSI(m.render(toast{message: message, level: level}, available)), "\n")
				want := ansi.StringWidth(lines[0])
				for i, l := range lines {
					if got := ansi.StringWidth(l); got != want {
						t.Errorf("level %d, message of %d, %d free: line %d measures %d and the first %d: "+
							"the box has to be a rectangle",
							level, len(message), available, i, got, want)
						break
					}
				}
			}
		}
	}
}

func TestOverlayToastsWithNoRoomItIsNotPainted(t *testing.T) {
	const width, rowCount = 40, 6
	before, rows := testView(width, rowCount)
	for i := range rows {
		rows[i] = false
	}

	box := []string{"╭──╮", "│ A│", "╰──╯"}
	after := overlayToasts(before, []string{strings.Join(box, "\n")}, width, rows)

	if after != before {
		t.Errorf("the box was painted in a view that admits no row:\n%s", after)
	}
	opened := func() []bool {
		r := make([]bool, rowCount)
		r[0], r[4] = true, true
		return r
	}
	one := []string{"1"}
	two := []string{"2"}
	three := []string{"3"}
	after = overlayToasts(before, []string{one[0], two[0], three[0]}, width, opened())
	lines := strings.Split(after, "\n")
	if !strings.Contains(lines[4], "1") {
		t.Errorf("row 4 does not carry notice 1: %q", lines[4])
	}
	if !strings.Contains(lines[0], "2") {
		t.Errorf("row 0 does not carry notice 2: %q", lines[0])
	}
	if strings.Contains(lines[0], "3") || strings.Contains(lines[4], "3") {
		t.Error("notice 3 was painted with no room: the oldest one should be discarded")
	}
}
