package bordered

import (
	"image/color"
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

func TestRenderWithTitleExactWidthAndCorners(t *testing.T) {
	out := RenderWithTitle(Rounded(), lipgloss.Color("238"), " prdash ", "hello", 20)
	lines := strings.Split(out, "\n")
	if len(lines) != 3 {
		t.Fatalf("lines = %d, want 3 (border + content + border)", len(lines))
	}
	for i, l := range lines {
		if w := ansi.StringWidth(l); w != 20 {
			t.Errorf("line %d width = %d, want 20: %q", i, w, l)
		}
	}
	top := ansi.Strip(lines[0])
	if !strings.HasPrefix(top, "╭") || !strings.HasSuffix(top, "╮") {
		t.Errorf("wrong top corners: %q", top)
	}
	if !strings.Contains(top, " prdash ") {
		t.Errorf("title not embedded in the top line: %q", top)
	}
	bot := ansi.Strip(lines[2])
	if !strings.HasPrefix(bot, "╰") || !strings.HasSuffix(bot, "╯") {
		t.Errorf("wrong bottom corners: %q", bot)
	}
}

// Content wider than the interior is clipped, not re-wrapped: wrapping would break the geometry.
func TestRenderWithTitleClipsWithoutWrapping(t *testing.T) {
	longText := strings.Repeat("x", 100)
	out := RenderWithTitle(Rounded(), nil, "", longText, 12)
	lines := strings.Split(out, "\n")
	if len(lines) != 3 {
		t.Fatalf("lines = %d, want 3 (the clip does not add lines)", len(lines))
	}
	for i, l := range lines {
		if w := ansi.StringWidth(l); w != 12 {
			t.Errorf("line %d width = %d, want 12", i, w)
		}
	}
	if got := ansi.StringWidth(ansi.Strip(lines[1])); got != 12 {
		t.Errorf("clipped content width = %d, want 12", got)
	}
}

func TestRenderWithTitleClipKeepsANSI(t *testing.T) {
	content := "\x1b[31m" + strings.Repeat("ab", 40) + "\x1b[0m"
	out := RenderWithTitle(Rounded(), nil, "", content, 10)
	lines := strings.Split(out, "\n")
	if len(lines) != 3 {
		t.Fatalf("lines = %d, want 3", len(lines))
	}
	if w := ansi.StringWidth(lines[1]); w != 10 {
		t.Errorf("ANSI width = %d, want 10: %q", w, lines[1])
	}
	if !strings.Contains(lines[1], "\x1b[") {
		t.Errorf("the content's ANSI was lost: %q", lines[1])
	}
}

func TestBorderWithNoCharacterFillsWithSpace(t *testing.T) {
	emptyBorder := lipgloss.Border{TopLeft: "┌", Top: "", TopRight: "┐", Left: "│", Right: "│", BottomLeft: "└", Bottom: "", BottomRight: "┘"}
	out := RenderWithTitle(emptyBorder, nil, "", "hello", 8)
	lines := strings.Split(out, "\n")
	if len(lines) != 3 {
		t.Fatalf("lines = %d, want 3", len(lines))
	}
	if got := ansi.Strip(lines[0]); got != "┌      ┐" {
		t.Errorf("top line = %q, want \"┌      ┐\" (space fill)", got)
	}
	if got := ansi.Strip(lines[2]); got != "└      ┘" {
		t.Errorf("bottom line = %q, want \"└      ┘\"", got)
	}
	for i, l := range lines {
		if w := ansi.StringWidth(l); w != 8 {
			t.Errorf("line %d width = %d, want 8: %q", i, w, l)
		}
	}

	// Without side borders it is a space too, or the content would stick to the frame.
	withoutSides := lipgloss.Border{TopLeft: "+", Top: "-", TopRight: "+", BottomLeft: "+", Bottom: "-", BottomRight: "+"}
	out = RenderWithTitle(withoutSides, nil, "", "hello", 8)
	lines = strings.Split(out, "\n")
	if got := ansi.Strip(lines[1]); got != " hello  " {
		t.Errorf("interior line = %q, want \" hello  \" (side spaces)", got)
	}
}

func TestTheInteriorIsPaddedToTheExactWidth(t *testing.T) {
	for _, inner := range []string{"x", "short", "exact!!", "too long and then some"} {
		for _, width := range []int{6, 8, 12, 20} {
			out := RenderWithTitle(Rounded(), nil, "", inner, width)
			for i, l := range strings.Split(out, "\n") {
				if w := ansi.StringWidth(l); w != width {
					t.Errorf("content %q and width %d: line %d measures %d, want %d: %q",
						inner, width, i, w, width, ansi.Strip(l))
				}
			}
		}
	}
	out := RenderWithTitle(Rounded(), nil, "", "abcdefghij", 6)
	lines := strings.Split(out, "\n")
	if got := ansi.Strip(lines[1]); got != "│abcd│" {
		t.Errorf("clipped content = %q, want \"│abcd│\"", got)
	}
}

func TestATitleWiderThanTheInteriorIsClipped(t *testing.T) {
	for _, width := range []int{4, 6, 8, 12} {
		out := RenderWithTitle(Rounded(), nil, "a really quite long title indeed", "", width)
		for i, l := range strings.Split(out, "\n") {
			if w := ansi.StringWidth(l); w != width {
				t.Errorf("width %d: line %d measures %d, want %d: %q", width, i, w, width, ansi.Strip(l))
			}
		}
		out = RenderWithTitle(Rounded(), nil, "abc", "", width)
		for i, l := range strings.Split(out, "\n") {
			if w := ansi.StringWidth(l); w != width {
				t.Errorf("width %d, exact title: line %d measures %d, want %d", width, i, w, width)
			}
		}
	}
}

func TestTheBorderColorIsApplied(t *testing.T) {
	withColor := color.RGBA{R: 0x1b, G: 0x2b, B: 0x34, A: 0xff}
	out := RenderWithTitle(Rounded(), withColor, "t", "hello", 10)
	lines := strings.Split(out, "\n")
	if len(lines) != 3 {
		t.Fatalf("lines = %d, want 3", len(lines))
	}
	// With a colour there is an ANSI sequence: without one the border is invisible on a dark terminal.
	for i, l := range lines {
		if !strings.Contains(l, "\x1b[") {
			t.Errorf("line %d has no ANSI with a border colour: %q", i, ansi.Strip(l))
		}
		// And the width does NOT change with the style: that is why the measurement is in plain text.
		if w := ansi.StringWidth(l); w != 10 {
			t.Errorf("line %d width = %d, want 10: the style must not change the width", i, w)
		}
	}
	withStyle := lipgloss.NewStyle().Bold(true).Render("hello")
	out = RenderWithTitle(Rounded(), withColor, "t", withStyle, 10)
	if !strings.Contains(out, "\x1b[1m") && !strings.Contains(out, "\x1b[") {
		t.Errorf("the content lost its style: %q", out)
	}

	plain := ansi.Strip(RenderWithTitle(Rounded(), nil, "t", "hello", 10))
	if plain != ansi.Strip(out) {
		t.Errorf("with and without colour the plain text should be the same:\n%q\n%q", plain, ansi.Strip(out))
	}
}

func TestRenderWithTitleMinimumWidth(t *testing.T) {
	out := RenderWithTitle(Rounded(), nil, "long title", "x", 1)
	for i, l := range strings.Split(out, "\n") {
		if w := ansi.StringWidth(l); w != 2 {
			t.Errorf("line %d width = %d, want 2 (clamp)", i, w)
		}
	}
}

func TestRenderWithTitleEmptyContent(t *testing.T) {
	out := RenderWithTitle(Rounded(), nil, "", "", 8)
	lines := strings.Split(out, "\n")
	if len(lines) != 3 {
		t.Fatalf("lines = %d, want 3", len(lines))
	}
	if got := ansi.Strip(lines[1]); got != "│      │" {
		t.Errorf("empty interior line = %q, want border + interior fill", got)
	}
}

func TestRenderWithTitleLongTitle(t *testing.T) {
	out := RenderWithTitle(Rounded(), nil, strings.Repeat("t", 40), "x", 10)
	for i, l := range strings.Split(out, "\n") {
		if w := ansi.StringWidth(l); w != 10 {
			t.Errorf("line %d width = %d, want 10: %q", i, w, l)
		}
	}
}

func TestRenderWithTitlesBottomCaption(t *testing.T) {
	out := RenderWithTitles(Rounded(), nil, " top ", AlignLeft, " bottom ", AlignRight, "c", 24)
	lines := strings.Split(out, "\n")
	bot := ansi.Strip(lines[len(lines)-1])
	if !strings.Contains(bot, " bottom ") {
		t.Errorf("bottom caption missing: %q", bot)
	}
	if !strings.HasSuffix(bot, "╯") {
		t.Errorf("bottom right corner missing: %q", bot)
	}
	for i, l := range lines {
		if w := ansi.StringWidth(l); w != 24 {
			t.Errorf("line %d width = %d, want 24", i, w)
		}
	}
}

// The content's padding is the caller's, not the box's: a line of spaces looks empty.
func TestRenderWithTitlePadsToTheGivenHeight(t *testing.T) {
	out := RenderWithTitle(Rounded(), nil, "", strings.Repeat("\n", 2), 8)
	if got := len(strings.Split(out, "\n")); got != 5 {
		t.Errorf("lines = %d, want 5 (2 borders + 3 of content)", got)
	}
}
