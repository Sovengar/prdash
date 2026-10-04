// Package bordered draws boxes with a rounded border and the title embedded in the top line. Content
// is clipped (ANSI-aware) to the inner width rather than re-wrapped, because wrapping would break the
// height the layout computed.
package bordered

import (
	"image/color"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

const (
	AlignLeft = iota
	AlignCenter
	AlignRight
)

func Rounded() lipgloss.Border { return lipgloss.RoundedBorder() }

// sin leyenda inferior.
func RenderWithTitle(border lipgloss.Border, borderFg color.Color, title, content string, width int) string {
	return RenderWithTitles(border, borderFg, title, AlignLeft, "", AlignLeft, content, width)
}

func RenderWithTitles(border lipgloss.Border, borderFg color.Color, topTitle string, topAlign int, bottomTitle string, bottomAlign int, content string, width int) string {
	// The floor is 2, expressed as a max rather than the old `if width < 2 { width = 2 }`: the old
	// branch made width exactly 2 indistinguishable from `width <= 2`, since both leave it at 2 and
	// there is no input that separates them. With max there is no comparison to mutate at the boundary.
	// Below 2 the two borders do not fit, and the floor is what lets the layout not check anything.
	width = max(2, width)

	innerWidth := width - 2 // width >= 2 → nunca negativo

	var style *ansi.Style
	if borderFg != nil {
		s := ansi.NewStyle().ForegroundColor(borderFg)
		style = &s
	}

	var b strings.Builder
	b.WriteString(borderLine(style, border.TopLeft, border.Top, border.TopRight, innerWidth, topAlign, topTitle))
	b.WriteString("\n")
	for _, line := range contentLines(style, border.Left, border.Right, content, innerWidth) {
		b.WriteString(line)
		b.WriteString("\n")
	}
	b.WriteString(borderLine(style, border.BottomLeft, border.Bottom, border.BottomRight, innerWidth, bottomAlign, bottomTitle))
	return b.String()
}

func borderLine(style *ansi.Style, left, fill, right string, innerWidth, align int, title string) string {
	if fill == "" {
		fill = " "
	}
	// The title is ALWAYS truncated, without asking: `ansi.Truncate` is visibly the identity when the
	// text already fits, so the question only existed to have a branch to mutate at a boundary where
	// truncating and not truncating give the same text and the same width.
	// The width is RE-MEASURED rather than assumed: measuring is what makes the bottom padding come out
	// right, and `titleWidth = innerWidth` lied if truncation had left the title shorter than that.
	title = ansi.Truncate(title, innerWidth, "")
	titleWidth := ansi.StringWidth(title)

	remaining := innerWidth - titleWidth
	var leftPad, rightPad int
	switch align {
	case AlignRight:
		leftPad = remaining
	case AlignCenter:
		leftPad = remaining / 2
		rightPad = remaining - leftPad
	default:
		rightPad = remaining
	}

	return styled(style, left) +
		styled(style, strings.Repeat(fill, leftPad)) +
		styled(style, title) +
		styled(style, strings.Repeat(fill, rightPad)) +
		styled(style, right)
}

func contentLines(style *ansi.Style, leftChar, rightChar, content string, innerWidth int) []string {
	if leftChar == "" {
		leftChar = " "
	}
	if rightChar == "" {
		rightChar = " "
	}

	raw := strings.Split(content, "\n") // siempre >= 1 elemento
	lines := make([]string, 0, len(raw))
	for _, line := range raw {
		// Clip and pad without asking, for the same reason as the title: both questions (`w > innerWidth`
		// and `pad > 0`) were identities at their boundary.
		// Padding needs no guard because the clip above guarantees the difference is never negative, and
		// `strings.Repeat` with a negative count PANICS: the guard was not decorative, it was what
		// prevented the panic, and now the order of the two lines does.
		line = ansi.Truncate(line, innerWidth, "")
		line += strings.Repeat(" ", innerWidth-ansi.StringWidth(line))
		lines = append(lines, styled(style, leftChar)+line+styled(style, rightChar))
	}
	return lines
}

func styled(style *ansi.Style, s string) string {
	if style == nil {
		return s
	}
	return style.Styled(s)
}
