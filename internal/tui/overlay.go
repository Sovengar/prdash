// Boxes drawn over the view, by the same technique the toasts use: the background view is not recomposed,
// it is clipped where the box needs text room. Clipping with ansi.Truncate/TruncateLeft instead of
// splicing by index keeps the base line's colour codes; the other way, whatever survived on the right
// would come out in the terminal's foreground colour.
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

// Its own function because two things need it and they have to agree: the overlay that clips the view and
// the graphics layer that places the image. If each computed its own position the frame and the image
// would land in different rectangles, and only when they happened to coincide — the worst thing that can
// happen to a positioning bug.
//
// The floor at zero does the whole job: a box larger than the area cannot be centred, so it is pinned to
// the corner. There used to be an `if` after this that could never fire: with the box inside the area y
// comes out at (height-boxH)/2, and adding boxH gives (height+boxH)/2, which is at most the area exactly
// when boxH <= height, which is this case's premise; with a taller box height-boxH is negative, y is
// zero by the floor, and the `if` left it alone. Verified by removing it: the suite stays green with y
// forced to the middle of the area.
func centeredOrigin(width, height, boxW, boxH int) (x, y int) {
	x = max(0, (width-boxW)/2)
	y = max(0, (height-boxH)/2)
	return x, y
}
