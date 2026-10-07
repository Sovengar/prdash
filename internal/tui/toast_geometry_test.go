package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

// What is asserted here is NOT visible from the painted box: a column more or less does not show.
func TestToastGeometryBoundsTheWidth(t *testing.T) {
	icon := toastIcon(toastInfo)

	for _, message := range []string{"", "hola", strings.Repeat("x", 40), strings.Repeat("y", 200)} {
		for _, available := range []int{0, 5, 9, 20, 50, 100, 1000} {
			width, wrapAt := toastGeometry(message, icon, available)

			if width > toastMaxWidth {
				t.Errorf("message of %d columns with %d available gave a width of %d, want <= %d",
					len(message), available, width, toastMaxWidth)
			}
			// The free space OVERRIDES the minimum: with 9 free columns the box is 9 even if the minimum is larger.
			if available > toastMinAvailable && width > available {
				t.Errorf("with %d free columns the box measures %d: it overflows", available, width)
			}
			if available >= toastMinWidth && width < toastMinWidth {
				t.Errorf("with %d free columns the box measures %d, below the minimum %d",
					available, width, toastMinWidth)
			}

			// The wrap width is EXACTLY the usable width minus the icon gap, with no floor of its own, hence 6 (8-2) not 4.
			want := max(width-toastFrame, toastMinInner) - toastIconGap
			if wrapAt != want {
				t.Errorf("with a width of %d the text splits at %d, want %d (the usable minus the icons gap)",
					width, wrapAt, want)
			}
			if wrapAt < toastMinInner-toastIconGap {
				t.Errorf("with a width of %d the text splits at %d, below %d: it comes out in unreadable chunks",
					width, wrapAt, toastMinInner-toastIconGap)
			}
		}
	}
}

// A warning that covers the view hides what the user came to look at.
func TestToastGeometryRespectsTheFreeSpace(t *testing.T) {
	icon := toastIcon(toastInfo)
	message := strings.Repeat("x", 30)

	for _, available := range []int{30, 40, 50, 59} {
		width, _ := toastGeometry(message, icon, available)
		if width > available {
			t.Errorf("with %d free columns the box measures %d: it invades the view", available, width)
		}
	}

	debajo, _ := toastGeometry(message, icon, 0)
	for _, available := range []int{-5, 0, 1, toastMinAvailable} {
		width, _ := toastGeometry(message, icon, available)
		if width != debajo {
			t.Errorf("with %d free columns it gave a width of %d and without gap it gives %d: below the minimum %d it should not change",
				available, width, debajo, toastMinAvailable)
		}
	}

	// The floor holds even with the narrowest box there is: the text never wraps below 6 columns.
	for available := -5; available <= toastMinWidth; available++ {
		if _, wrapAt := toastGeometry(message, icon, available); wrapAt < toastMinInner-toastIconGap {
			t.Errorf("with %d free columns the text splits at %d, want >= %d",
				available, wrapAt, toastMinInner-toastIconGap)
		}
	}

	exact, _ := toastGeometry(message, icon, toastMinAvailable+1)
	if exact == debajo {
		t.Errorf("with %d free columns it gave the same width as without gap: the threshold is not where it says",
			toastMinAvailable+1)
	}
}

func TestToastGeometryTheTextFitsInTheBox(t *testing.T) {
	icon := toastIcon(toastInfo)
	for length := 0; length <= toastMaxWidth; length++ {
		message := ""
		for i := 0; i < length; i++ {
			if i > 0 {
				message += " "
			}
			message += "z"
		}
		width, wrapAt := toastGeometry(message, icon, 1000)

		for _, line := range wrapText(message, wrapAt) {
			if len(line) > wrapAt {
				t.Errorf("a message of %d columns gave a line of %d splitting at %d",
					length, len(line), wrapAt)
			}
			// The icon's width is measured in COLUMNS, not len(), which would count its three bytes.
			if ansi.StringWidth(icon)+1+len(line) > width-toastFrame {
				t.Errorf("a message of %d columns gave a first line of %d plus the icon in a box of %d",
					length, len(line), width)
			}
		}
		// The box is painted at exactly the geometry width; the three-byte border means len() counts bytes and a 24-wide box measures 72.
		for _, line := range strings.Split(stripANSI(renderToast(t, message, 1000)), "\n") {
			if lineWidth := ansi.StringWidth(line); lineWidth != width {
				t.Errorf("a message of %d columns gave a line of %d in a box of %d: geometry and painting do not match",
					length, lineWidth, width)
			}
		}
	}

	// The wrapper breaks BY WORDS, so a word wider than the box comes out whole and the box clips it: breaking a word misreads.
	for length := toastMaxWidth; length < toastMaxWidth+20; length++ {
		message := strings.Repeat("z", length)
		width, _ := toastGeometry(message, icon, 1000)
		lines := wrapText(message, max(width-toastFrame, toastMinInner)-toastIconGap)
		if len(lines) != 1 || len(lines[0]) != length {
			t.Errorf("a word of %d columns was split into %d chunks: a word is not split",
				length, len(lines))
		}
		for _, line := range strings.Split(stripANSI(renderToast(t, message, 1000)), "\n") {
			if lineWidth := ansi.StringWidth(line); lineWidth != width {
				t.Errorf("a word of %d columns gave a line of %d in a box of %d: the box left",
					length, lineWidth, width)
			}
		}
	}
}

func TestToastGeometryTheEstimateIsTheSumOfItsParts(t *testing.T) {
	for length := 0; length <= toastMaxWidth; length++ {
		for _, level := range []toastLevel{toastSuccess, toastError, toastInfo, toastWarning} {
			message := strings.Repeat("z", length)
			icon := toastIcon(level)

			width, _ := toastGeometry(message, icon, 1000)

			want := length + ansi.StringWidth(icon) + toastIconSpace + toastFrame
			want = min(max(want, toastMinWidth), toastMaxWidth)
			if width != want {
				t.Errorf("a message of %d columns with icon %q gave a width of %d, want %d (the text, the icon, its space and the frame)",
					length, icon, width, want)
			}
		}
	}
}

// Every current icon is one column, so a hardcoded 1 would give the same answer today and break tomorrow.
func TestToastGeometryTheIconPushesAccordingToItsWidth(t *testing.T) {
	short := strings.Repeat("z", 30)
	for _, icon := range []string{"i", "ii", "iii", "✨", "⚠️"} {
		want := 30 + ansi.StringWidth(icon) + toastIconSpace + toastFrame
		want = min(max(want, toastMinWidth), toastMaxWidth)
		if width, _ := toastGeometry(short, icon, 1000); width != want {
			t.Errorf("with an icon of %d columns it gave a width of %d, want %d: the icon did not push",
				ansi.StringWidth(icon), width, want)
		}
	}
	a, _ := toastGeometry(short, "ii", 1000)
	b, _ := toastGeometry(short, "⚠️", 1000)
	if a != b {
		t.Errorf("two icons of the same size gave different widths: %d and %d", a, b)
	}
	narrow, _ := toastGeometry(short, "i", 1000)
	width, _ := toastGeometry(short, "✨✨", 1000)
	if width < narrow {
		t.Errorf("an icon of 4 columns gave a box of %d and one of 1 gave %d: the icon only pushes",
			width, narrow)
	}
}

func renderToast(t *testing.T, message string, available int) string {
	t.Helper()
	m := newToastManager()
	return m.render(toast{message: message, level: toastInfo}, available)
}

// The air is what separates "a warning on top of the view" from "one more border".
func TestToastColumnIsGluedToTheRightWithAir(t *testing.T) {
	for width := 0; width <= 200; width += 1 {
		for _, bw := range []int{1, 5, 24, 60, 100, 200} {
			x := toastColumn(width, bw)

			if x < 0 {
				t.Errorf("toastColumn(%d, %d) = %d: a negative column leaves", width, bw, x)
			}
			if bw <= width-1 && x+bw > width-1 {
				t.Errorf("toastColumn(%d, %d) = %d: the box ends at column %d and the view has %d, it leaves to the right",
					width, bw, x, x+bw, width)
			}
			if bw < width-1 && x != width-bw-1 {
				t.Errorf("toastColumn(%d, %d) = %d, want %d: the right air is one column wide",
					width, bw, x, width-bw-1)
			}
			if bw >= width-1 && x != 0 {
				t.Errorf("toastColumn(%d, %d) = %d with no room for the air: it should go to 0", width, bw, x)
			}
			if x != max(width-bw-1, 0) {
				t.Errorf("toastColumn(%d, %d) = %d, want %d", width, bw, x, max(width-bw-1, 0))
			}
		}
	}
}

// The +1 is the edge that matters: the last row (index anchor) is the lowest, so anchor+1 rows fit from it counting up.
func TestToastBlockHeightNeverExceedsWhatIsLeft(t *testing.T) {
	// anchor starts at 0 on purpose: it comes from len(lines)-1 over a strings.Split, which always returns at least one line.
	for anchor := 0; anchor <= 50; anchor++ {
		for bh := 0; bh <= 50; bh++ {
			got := toastBlockHeight(bh, anchor)

			if got > anchor+1 {
				t.Errorf("toastBlockHeight(%d, %d) = %d with room for %d: it leaves at the top",
					bh, anchor, got, anchor+1)
			}
			if got > bh {
				t.Errorf("toastBlockHeight(%d, %d) = %d: it invented rows", bh, anchor, got)
			}
			if got != min(bh, anchor+1) {
				t.Errorf("toastBlockHeight(%d, %d) = %d, want %d", bh, anchor, got, min(bh, anchor+1))
			}
		}
	}
	window := 3
	if got := toastBlockHeight(window, window-1); got != window {
		t.Errorf("a box of %d rows in a window of %d ended at %d: it does not fit exactly, which is the case that must fit",
			window, window, got)
	}
}
