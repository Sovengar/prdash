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
	// The floor is 2 as a max rather than an `if`: the old branch made width exactly 2
	// indistinguishable from `width <= 2`, and with max there is no comparison to mutate at the boundary.
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
	// The title is ALWAYS truncated, without asking: `ansi.Truncate` is the identity when the text
	// fits. The width is RE-MEASURED because `titleWidth = innerWidth` lied once truncation left it shorter.
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
		// Clip and pad without asking: both questions were identities at their boundary. The padding needs
		// no guard because the clip above guarantees a non-negative difference, and `strings.Repeat` with a
		// negative count PANICS — the order of the two lines is what prevents it.
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
