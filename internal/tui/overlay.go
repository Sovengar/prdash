// Boxes drawn over the view, by the toasts' technique: the background is not recomposed, it is
// clipped. Clipping with ansi.Truncate keeps the base line's colour codes; splicing by index does not.
package tui

import (
	"strings"

	"github.com/charmbracelet/x/ansi"
)

// Unlike the toasts, a popup IS clipped vertically: it is modal and its border matters. The background
// is not clipped, which is the whole point of a popup.
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

// Its own function because the overlay and the graphics layer have to agree: computed separately
// they land in different rectangles, and only when they coincided. The floor at zero does the whole
// job — a box larger than the area is pinned to the corner.
func centeredOrigin(width, height, boxW, boxH int) (x, y int) {
	x = max(0, (width-boxW)/2)
	y = max(0, (height-boxH)/2)
	return x, y
}
