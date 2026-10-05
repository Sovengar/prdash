// The active section is composed ENTIRELY as loose lines and then clipped to the window.
// Scrolling the rows alone would leave the header orphaned from its items.
package tui

import (
	"prdash/internal/inbox"
)

type listLine struct {
	text string
	row  int
}

func (m *Model) listLines(inner int) []listLine {
	items := m.rows()
	problems := m.sectionProblems(m.activeSection)
	var lines []listLine

	// One layout for the active section only: sizing every section would waste width on suffixes
	// that are not on screen, and measuring per line would make the table dance as text is written.
	lay := newRefLayout([]inbox.Section{{Kind: m.activeSection, Items: items}}, m.prefixMode)

	// The common path gets its own line instead of repeating per row: that is what leaves room for
	// the ITEM suffix. Without a common path the line is not painted and ITEM carries the full path.
	if prefix := lay.prefixOf(m.activeSection); prefix != "" {
		lines = append(lines, listLine{text: "  " + styleDim.Render("· "+prefix+"/"), row: -1})
	}

	for _, p := range problems {
		lines = append(lines, listLine{text: "  " + styleWarn.Render("⚠ "+p), row: -1})
	}

	switch {
	case len(items) > 0:
		lines = append(lines, listLine{text: "  " + headerLine(lay, inner-2), row: -1})
		for i, it := range items {
			lines = append(lines, listLine{text: m.renderItem(it, m.activeSection, lay, i == m.cursor, inner-2), row: i})
		}
	case len(problems) == 0:
		lines = append(lines, listLine{text: "  " + styleEmpty.Render("(empty)"), row: -1})
	}

	if m.sectionLoadingMore(m.activeSection) {
		lines = append(lines, listLine{text: "  " + styleDim.Render("loading more…"), row: -1})
	}
	return lines
}

func cursorLine(lines []listLine, cursor int) int {
	for i, l := range lines {
		if l.row == cursor {
			return i
		}
	}
	return -1
}

func scrollFor(current, target, total, view int) int {
	if target >= 0 {
		switch {
		case target < current:
			current = target
		case target >= current+view:
			current = target - view + 1
		}
	}
	return min(max(current, 0), max(0, total-view))
}

func (m *Model) syncScroll() {
	lines := m.listLines(m.contentWidth())
	m.scroll = scrollFor(m.scroll, cursorLine(lines, m.cursor), len(lines), m.layout().bodyLines)
}

func visibleList(lines []listLine, scroll, view int) []listLine {
	if view <= 0 || len(lines) == 0 {
		return nil
	}
	start := min(max(scroll, 0), max(0, len(lines)-view))
	return lines[start:min(start+view, len(lines))]
}
