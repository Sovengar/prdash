package tui

import (
	"strings"
	"testing"
)

func TestClipTopWithZeroRowsLeavesTheWholeDetail(t *testing.T) {
	three := []string{"state: open", "review: aprobado", "rol: autor"}

	for _, rows := range []int{-5, 0} {
		if got := clipTop(three, rows); len(got) != len(three) {
			t.Errorf("with %d rows the detail ended at %d lines, want %d: with no room to "+
				"clip it does not clip. With zero passing both checks the "+
				"clip returns an empty slice and the card is left blank",
				rows, len(got), len(three))
		}
	}

	if got := clipTop(three, 1); len(got) != 1 || got[0] != "rol: autor" {
		t.Errorf("with one row it ended at %v, want the last: the clip is from the top so "+
			"the end of the detail survives", got)
	}

	for _, rows := range []int{3, 4, 100} {
		if got := clipTop(three, rows); len(got) != len(three) {
			t.Errorf("with %d rows for %d lines it ended at %d: there is nothing to clip",
				rows, len(three), len(got))
		}
	}
}

func TestTheNoticeSticksToTheBottomOnTheLastRow(t *testing.T) {
	const rowCount, width = 12, 40

	rows := make([]bool, rowCount)
	for i := range rows {
		rows[i] = true
	}
	base := make([]string, rowCount)
	for i := range base {
		base[i] = strings.Repeat("·", width)
	}

	box := strings.Join([]string{
		"+--------------+",
		"| a notice     |",
		"+--------------+",
	}, "\n")

	got := overlayToasts(strings.Join(base, "\n"), []string{box}, width, rows)
	lines := strings.Split(got, "\n")

	if len(lines) != rowCount {
		t.Fatalf("the overlay changed the number of rows: %d, want %d. The notice is "+
			"painted over the background by clipping it, not by adding rows", len(lines), rowCount)
	}

	last := lines[rowCount-1]
	if !strings.Contains(last, "+--------------+") {
		t.Errorf("the last row is %q and does not carry the notices border: the block has "+
			"to end on the last row of the view", firstLineWith(last, 60))
	}
	if !strings.Contains(lines[rowCount-3], "+--------------+") {
		t.Errorf("the third from last row is %q and does not carry the notices top border",
			firstLineWith(lines[rowCount-3], 60))
	}
	if !strings.Contains(lines[rowCount-2], "a notice") {
		t.Errorf("the second to last row is %q and does not carry the notices text",
			firstLineWith(lines[rowCount-2], 60))
	}

	rowsWithGap := make([]bool, rowCount)
	copy(rowsWithGap, rows)
	rowsWithGap[rowCount-1] = false // the last row is somebody elses border
	got = overlayToasts(strings.Join(base, "\n"), []string{box}, width, rowsWithGap)
	lines = strings.Split(got, "\n")
	if strings.Contains(lines[rowCount-1], "a notice") {
		t.Errorf("the notice was painted on the last row, which is border: %q",
			firstLineWith(lines[rowCount-1], 60))
	}
	// It moves exactly enough: the block needs THREE rows and with the last one closed only two fit.
	if !strings.Contains(lines[rowCount-3], "a notice") {
		t.Errorf("with the last row closed the notice should rise one row and land on "+
			"the third from the bottom, and it ended up as %q on that row",
			firstLineWith(lines[rowCount-3], 60))
	}
	if strings.Contains(lines[rowCount-2], "a notice") {
		t.Errorf("the notice stayed stuck to the background with the last row closed: %q",
			firstLineWith(lines[rowCount-2], 60))
	}
}
