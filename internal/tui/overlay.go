package tui

import (
	"strings"

	"github.com/charmbracelet/x/ansi"
)

func overlayCentered(content, box string, width int) string {
	if box == "" {
		return content
	}
	lines := strings.Split(content, "\n")
	block := strings.Split(box, "\n")
	block = block[:min(len(block), len(lines))]
	bh := len(block)

	x, y := centeredOrigin(width, len(lines), ansi.StringWidth(block[0]), bh)

	for j := range bh {
		i := y + j
		cell := ansi.Truncate(block[j], width, "")
		cw := ansi.StringWidth(cell)
		line := lines[i]
		lines[i] = ansi.Truncate(line, x, "") + cell + ansi.TruncateLeft(line, x+cw, "")
	}
	return strings.Join(lines, "\n")
}

func centeredOrigin(width, height, boxW, boxH int) (x, y int) {
	x = max(0, (width-boxW)/2)
	y = max(0, (height-boxH)/2)
	return x, y
}
