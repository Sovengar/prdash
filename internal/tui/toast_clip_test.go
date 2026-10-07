package tui

import (
	"strings"
	"testing"
)

// The box does not enter whole but does not vanish: the rows that fit are painted, and a warning that disappears says nothing.
func TestOverlayToastsABoxTallerThanTheViewIsPaintedClipped(t *testing.T) {
	const width, rowCount = 40, 2
	before, rows := testView(width, rowCount)

	m := newToastManager()
	m.show("careful", toastWarning)
	boxes := m.blocks(width)
	if bh := len(strings.Split(boxes[0], "\n")); bh <= rowCount {
		t.Fatalf("the box is %d rows tall in a view of %d: there is nothing to clip", bh, rowCount)
	}

	after := overlayToasts(before, boxes, width, rows)

	tocadas := rowsWithTheBox(t, before, after)
	if len(tocadas) != rowCount {
		t.Fatalf("a box taller than the view touched %d rows (%v), want %d: "+
			"the rows that fit have to be painted", len(tocadas), tocadas, rowCount)
	}
	for i, row := range tocadas {
		if row != i {
			t.Errorf("the touched row %d is %d, want %d: the clip keeps the top of the box", i, row, i)
		}
	}
	if !strings.Contains(strings.Split(after, "\n")[rowCount-1], toastIcon(toastWarning)) {
		t.Errorf("the last row does not carry the notice: %q", strings.Split(after, "\n")[rowCount-1])
	}
}
