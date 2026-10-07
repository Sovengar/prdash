package bordered

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

func widthOf(t *testing.T, s string) int {
	t.Helper()
	return ansi.StringWidth(s)
}

func lines(s string) []string { return strings.Split(s, "\n") }

func TestTheOuterWidthIsTheOneRequested(t *testing.T) {
	contents := []string{
		"",
		"one line",
		"two\nlines",
		"three\nshort\nlines",
		"one line\n\nand a gap in the middle",
	}
	for _, width := range []int{2, 3, 4, 5, 10, 38, 80} {
		for _, content := range contents {
			got := RenderWithTitle(Rounded(), nil, "T", content, width)
			for i, l := range lines(got) {
				if w := widthOf(t, l); w != width {
					t.Errorf("width %d, content %q, row %d: measures %d, want %d. "+
						"The requested width is an equality, not a bound: the layout has "+
						"already reserved it and one extra column unbalances the box",
						width, content, i, w, width)
				}
			}
		}
	}
}

func TestTheInteriorWidthIsTheOuterOneMinusTwo(t *testing.T) {
	for _, width := range []int{2, 3, 4, 8, 40} {
		got := RenderWithTitle(Rounded(), nil, "", "x", width)
		rows := lines(got)
		if len(rows) < 3 {
			t.Fatalf("width %d: the result has %d rows, and the test needs the top one, "+
				"one of content and the bottom one", width, len(rows))
		}
		content := rows[1]
		left, right := Rounded().Left, Rounded().Right
		interior := strings.TrimSuffix(strings.TrimPrefix(content, left), right)
		want := width - 2
		if w := widthOf(t, interior); w != want {
			t.Errorf("width %d: the interior measures %d, want %d (the outer one minus the "+
				"two borders)", width, w, want)
		}
	}
}

// Below two there is no box, so the width goes up to two.
func TestTheMinimumWidthIsTwo(t *testing.T) {
	for _, requested := range []int{-40, -1, 0, 1} {
		got := RenderWithTitle(Rounded(), nil, "T", "c", requested)
		for i, l := range lines(got) {
			if w := widthOf(t, l); w != 2 {
				t.Errorf("requested %d, row %d: measures %d, want 2. Below two the borders do "+
					"not fit, and the floor is what avoids having to check it in the layout",
					requested, i, w)
			}
		}
	}

	if rows := len(lines(RenderWithTitle(Rounded(), nil, "", "", 2))); rows != 3 {
		t.Errorf("with the minimum width there are %d rows, want 3", rows)
	}
}

func TestTheTitleIsClippedToTheInteriorAndDoesNotOverflowIt(t *testing.T) {
	titles := []string{
		"",
		"t",
		"title",
		"a title far longer than any interior",
		"unicode: áéíóúñ",
	}
	for _, width := range []int{2, 3, 4, 6, 12, 30} {
		for _, title := range titles {
			got := RenderWithTitle(Rounded(), nil, title, "c", width)
			rows := lines(got)
			for i, l := range rows {
				if w := widthOf(t, l); w != width {
					t.Errorf("width %d, title %q, row %d: measures %d, want %d. A title "+
						"that overflows the interior breaks the whole box", width, title, i, w, width)
				}
			}
			interior := width - 2
			painted := ansi.StringWidth(stripBorder(rows[0]))
			if painted > interior {
				t.Errorf("width %d, title %q: %d title characters were painted into an "+
					"interior of %d", width, title, painted, interior)
			}
		}
	}
}

func stripBorder(l string) string {
	b := Rounded()
	return strings.TrimSuffix(strings.TrimPrefix(l, b.TopLeft), b.TopRight)
}

func TestTheTitleAlignmentRespectsTheWidth(t *testing.T) {
	for _, width := range []int{6, 7, 8, 9, 10, 11, 20} {
		interior := width - 2
		for _, title := range []string{"ab", "abc", "abcd", "abcde"} {
			tw := ansi.StringWidth(title)
			if tw > interior {
				continue // here the title is clipped and the alignment stops mattering
			}
			for _, alignCase := range []struct {
				name  string
				align int
			}{{"left", AlignLeft}, {"center", AlignCenter}, {"right", AlignRight}} {
				got := RenderWithTitles(Rounded(), nil, title, alignCase.align, "", AlignLeft, "", width)
				row := lines(got)[0]
				if w := widthOf(t, row); w != width {
					t.Errorf("width %d, title %q, %s: the row measures %d, want %d",
						width, title, alignCase.name, w, width)
					continue
				}
				interiorLine := stripBorder(row)
				pos := strings.Index(interiorLine, title)
				if pos < 0 {
					t.Errorf("width %d, title %q, %s: the title does not appear in the line %q",
						width, title, alignCase.name, interiorLine)
					continue
				}
				pos = ansi.StringWidth(interiorLine[:pos])
				rest := interior - tw
				switch alignCase.align {
				case AlignRight:
					if pos != rest {
						t.Errorf("width %d, title %q, right: starts at column %d, "+
							"want %d (the whole padding on the left)", width, title, pos, rest)
					}
				case AlignCenter:
					if pos != rest/2 {
						t.Errorf("width %d, title %q, center: starts at column %d, "+
							"want %d (half the padding on the left)", width, title, pos, rest/2)
					}
				default:
					if pos != 0 {
						t.Errorf("width %d, title %q, left: starts at column %d, "+
							"want 0", width, title, pos)
					}
				}
			}
		}
	}
}

func TestTheContentIsClippedAndPaddedToTheInterior(t *testing.T) {
	for _, width := range []int{4, 6, 10, 20} {
		interior := width - 2
		for _, size := range []int{0, 1, interior - 1, interior, interior + 1, interior * 2} {
			if size < 0 {
				continue
			}
			content := strings.Repeat("x", size)
			got := RenderWithTitle(Rounded(), nil, "", content, width)
			rows := lines(got)
			if len(rows) != 3 {
				t.Fatalf("width %d, content of %d: %d rows, want 3", width, size, len(rows))
			}
			row := rows[1]
			if w := widthOf(t, row); w != width {
				t.Errorf("width %d, content of %d: the row measures %d, want %d. The "+
					"padding is what makes the box opaque, and the clip what keeps it in",
					width, size, w, width)
				continue
			}
			interiorLine := row
			b := Rounded()
			interiorLine = strings.TrimPrefix(interiorLine, b.Left)
			interiorLine = strings.TrimSuffix(interiorLine, b.Right)
			want := strings.Repeat("x", min(size, interior)) +
				strings.Repeat(" ", max(0, interior-size))
			if interiorLine != want {
				t.Errorf("width %d, content of %d: the interior is %q, want %q",
					width, size, interiorLine, want)
			}
		}
	}
}

func TestEmptyContentGivesARowWithAnEmptyInterior(t *testing.T) {
	for _, content := range []string{"", "\n", "\n\n"} {
		expected := strings.Count(content, "\n") + 1
		got := RenderWithTitle(Rounded(), nil, "", content, 12)
		rows := lines(got)
		if len(rows) != expected+2 {
			t.Errorf("content %q: %d rows, want %d (the content rows plus the two "+
				"borders)", content, len(rows), expected+2)
		}
		for i, l := range rows {
			if widthOf(t, l) != 12 {
				t.Errorf("content %q, row %d: measures %d, want 12", content, i, widthOf(t, l))
			}
		}
		for i := 1; i < len(rows)-1; i++ {
			interior := strings.TrimSuffix(strings.TrimPrefix(rows[i], Rounded().Left), Rounded().Right)
			if strings.TrimSpace(interior) != "" {
				t.Errorf("content %q, row %d: the interior is %q and has something in it",
					content, i, interior)
			}
		}
	}
}

func TestABorderWithoutFillCharactersUsesSpaces(t *testing.T) {
	b := lipgloss.Border{
		TopLeft: "+", Top: "", TopRight: "+",
		Left: "|", BottomLeft: "+", Bottom: "", BottomRight: "+", Right: "|",
	}
	got := RenderWithTitle(b, nil, "t", "c", 10)
	for i, l := range lines(got) {
		if w := widthOf(t, l); w != 10 {
			t.Errorf("row %d with a border without fill measures %d, want 10: the empty "+
				"fill has to fall back to a space", i, w)
		}
	}
}

// strings.Repeat with a negative count panics; the clip guarantees the difference is never negative.
func TestThePaddingCannotAskForTooFewSpaces(t *testing.T) {
	contents := []string{
		"",
		"\x1b[31m\x1b[0m",
		"\x1b[31m\x1b[0m\x1b[32m",
		"\x1b[31mtext\x1b[0m",
		"\x1b[31m\x1b[0m\n\x1b[32m",
		"\x1b[31m" + strings.Repeat("x", 100) + "\x1b[0m",
		"\x1b[1m\x1b[4m\x1b[31mábc\x1b[0m",
		"áéíóú" + "\x1b[0m",
	}
	for _, width := range []int{2, 3, 4, 5, 8, 20, 40} {
		for _, content := range contents {
			var (
				got string
				pan bool
			)
			// The panic is caught to say WHICH case caused it, instead of the test dying.
			func() {
				defer func() {
					if r := recover(); r != nil {
						pan = true
					}
				}()
				got = RenderWithTitle(Rounded(), nil, "t", content, width)
			}()
			if pan {
				t.Errorf("width %d, content %q: the padding asked for too few spaces and "+
					"blew up. The clip is what guarantees it does not happen, and that has "+
					"to be checked and not assumed", width, content)
				continue
			}
			for i, l := range lines(got) {
				if w := widthOf(t, l); w != width {
					t.Errorf("width %d, content %q, row %d: measures %d, want %d",
						width, content, i, w, width)
				}
			}
		}
	}
}
