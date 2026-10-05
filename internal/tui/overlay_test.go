package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func viewOf(rows, width int) string {
	lines := make([]string, 0, rows)
	for i := range rows {
		label := "···" + string(rune('0'+i))
		lines = append(lines, label+strings.Repeat("·", max(0, width-ansi.StringWidth(label))))
	}
	return strings.Join(lines, "\n")
}

func plain(s string) string { return ansi.Strip(s) }

func TestOverlayKeepsTheBackgroundAroundThePopup(t *testing.T) {
	content := viewOf(9, 20)
	box := "AAA\nBBB\nCCC"

	got := plain(overlayCentered(content, box, 20))
	lines := strings.Split(got, "\n")
	if len(lines) != 9 {
		t.Fatalf("the view changed height: %d lines, want 9", len(lines))
	}
	for i, l := range lines {
		want := "···" + string(rune('0'+i)) + strings.Repeat("·", 16)
		if l == want {
			continue // row intacta
		}
		if i >= 3 && i <= 5 {
			continue // the ones the popup covers
		}
		t.Errorf("row %d = %q, want %q", i, l, want)
	}
	for i, want := range []string{"AAA", "BBB", "CCC"} {
		if row := strings.Split(got, "\n")[3+i]; !strings.Contains(row, want) {
			t.Errorf("row %d = %q, no contains %q", 3+i, row, want)
		}
	}
}

func TestOverlayCentersHorizontally(t *testing.T) {
	got := plain(overlayCentered(strings.Repeat("x", 21), "MID", 21))
	row := strings.Split(got, "\n")[0]
	if !strings.HasPrefix(row, "xxxx") || !strings.HasSuffix(row, "xxxx") {
		t.Errorf("row = %q, want the popup centered", row)
	}
	if !strings.Contains(row, "MID") {
		t.Errorf("row = %q, does not contain the popup", row)
	}
}

func TestOverlayPreservesLineWidth(t *testing.T) {
	content := viewOf(7, 20)
	for _, box := range []string{"A", "AAA\nBBB\nCCC", strings.Repeat("W", 40)} {
		got := overlayCentered(content, box, 20)
		for i, l := range strings.Split(got, "\n") {
			if w := ansi.StringWidth(l); w != 20 {
				t.Errorf("box %q: row %d mide %d, want 20", box, i, w)
			}
		}
	}
}

func TestOverlayCropsABoxTallerThanTheView(t *testing.T) {
	content := viewOf(3, 20)
	box := "1\n2\n3\n4\n5"

	got := plain(overlayCentered(content, box, 20))
	if lines := strings.Split(got, "\n"); len(lines) != 3 {
		t.Fatalf("lines = %d, want 3", len(lines))
	}
	if !strings.Contains(got, "3") || strings.Contains(got, "4") {
		t.Errorf("it did not clip at the bottom:\n%s", got)
	}
}

func TestOverlayWithoutBoxIsIdentity(t *testing.T) {
	content := viewOf(5, 20)
	if got := overlayCentered(content, "", 20); got != content {
		t.Error("an empty popup altered the view")
	}
}

// The base line is clipped with ansi, not with byte indexes.
func TestOverlayKeepsBaseColorsRightOfThePopup(t *testing.T) {
	base := "\x1b[31m" + strings.Repeat("r", 20) + "\x1b[0m"
	got := overlayCentered(base, "PP", 20)

	if !strings.Contains(got, "\x1b[31m") {
		t.Error("the base line lost its color sequence")
	}
	if !strings.HasSuffix(got, "\x1b[0m") {
		t.Errorf("the line does not keep the style close: %q", got)
	}
}
