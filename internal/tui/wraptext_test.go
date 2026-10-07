package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func TestWrapTextDoesNotTouchWhatAlreadyFits(t *testing.T) {
	for _, tc := range []struct{ text, wants string }{
		{"ab cd", "ab cd"},
		{"a  b", "a  b"},
		{"a   b   c", "a   b   c"},
		{"a\tb", "a\tb"},
		{"ab  cd", "ab  cd"},
		{"  with  LEADING  and  tail  ", "  with  LEADING  and  tail  "},
	} {
		for _, max := range []int{1, 4, 5, 6, 10, 100} {
			if ansi.StringWidth(tc.text) > max {
				continue
			}
			got := wrapText(tc.text, max)
			if len(got) != 1 {
				t.Errorf("wrapText(%q, %d) returned %d lines, want 1: the text fits", tc.text, max, len(got))
				continue
			}
			if got[0] != tc.wants {
				t.Errorf("wrapText(%q, %d) returned %q, want %q: text that fits is not touched, not even a space",
					tc.text, max, got[0], tc.wants)
			}
		}
	}

	const text = "a  b  c"
	const width = 7
	if got := wrapText(text, width); len(got) != 1 || got[0] != text {
		t.Errorf("a text of %d columns with width %d gave %q, want the line untouched",
			ansi.StringWidth(text), width, got)
	}
	if got := wrapText(text, width-1); len(got) == 1 && got[0] == text {
		t.Errorf("a text of %d columns with width %d came back intact: it should not, it does not fit",
			ansi.StringWidth(text), width-1)
	}
}

func TestWrapTextJoinsTheWordThatWouldCloseTheWidth(t *testing.T) {
	const max = 5

	got := wrapText("ab cd ef", max)
	if len(got) != 2 {
		t.Fatalf("wrapText(%q, %d) returned %q, want 2 lines: the second closes the exact width",
			"ab cd ef", max, got)
	}
	if got[0] != "ab cd" {
		t.Errorf("the first line is %q, want %q: the word that closes the width has to be joined", got[0], "ab cd")
	}
	if got[1] != "ef" {
		t.Errorf("the second line is %q, want %q", got[1], "ef")
	}

	if got := wrapText("ab cd ef", max-1); len(got) != 3 {
		t.Errorf("wrapText(%q, %d) returned %q, want 3 lines: one column less and the word no longer closes",
			"ab cd ef", max-1, got)
	}

	got = wrapText("ab cd ef gh", max)
	if len(got) != 2 || got[0] != "ab cd" || got[1] != "ef gh" {
		t.Errorf("wrapText(%q, %d) = %q, want [ab cd, ef gh]", "ab cd ef gh", max, got)
	}
}

func TestWrapTextNonPositiveWidthDoesNotSplit(t *testing.T) {
	for _, max := range []int{-20, -1, 0} {
		for _, text := range []string{"ab cd ef", "one word", "a  b  c", ""} {
			got := wrapText(text, max)
			if len(got) != 1 {
				t.Errorf("wrapText(%q, %d) returned %q, want a single line: with a non-positive width it does not split",
					text, max, got)
				continue
			}
			if got[0] != text {
				t.Errorf("wrapText(%q, %d) returned %q: without splitting, the text has to come back as given",
					text, max, got[0])
			}
		}
	}
	// Empty or spaces only gives ONE line, not none: an empty list would make the caller print an extra blank line.
	for _, text := range []string{"", "   ", "\t"} {
		if got := wrapText(text, 10); len(got) != 1 {
			t.Errorf("wrapText(%q, 10) returned %q, want one line", text, got)
		}
	}
	if got := wrapText("    ", 2); len(got) != 1 || got[0] != "    " {
		t.Errorf("wrapText(%q, 2) = %q, want the text intact in one line", "    ", got)
	}
}

func TestWrapTextEveryLineFitsAndNoWordIsSplit(t *testing.T) {
	for max := 1; max <= 40; max++ {
		for _, text := range []string{
			"ab cd ef gh ij",
			"a very long word that does not fit in any way at all",
			strings.Repeat("x ", 30) + "y",
			"short",
			"  spaces   at   the   beginning  and   at   the   end  ",
		} {
			for _, line := range wrapText(text, max) {
				if ansi.StringWidth(line) > max {
					if len(strings.Fields(line)) != 1 {
						t.Errorf("wrapText(%.20q, %d) gave line %q, which does not fit and is not a single word",
							text, max, line)
					}
				}
				if strings.TrimSpace(line) == "" && strings.TrimSpace(text) != "" {
					t.Errorf("wrapText(%.20q, %d) gave a line of pure spaces: %q", text, max, line)
				}
			}
			juntas := wrapText(text, max)
			for _, palabra := range strings.Fields(text) {
				found := false
				for _, line := range juntas {
					if palabra == line || strings.Contains(line, " "+palabra+" ") ||
						strings.HasPrefix(line, palabra+" ") || strings.HasSuffix(line, " "+palabra) {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("wrapText(%.20q, %d) lost or split the word %q", text, max, palabra)
				}
			}
		}
	}
}
