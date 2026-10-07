package sim

import "testing"

// At the exact boundary both dimensions are spent at once: the height rules and the width cap may not take a row away.
func TestFitCellsAtTheExactBoundaryKeepsAllTheRows(t *testing.T) {
	img := solid(5, 29, black)
	for _, box := range []struct{ cols, rows int }{{10, 29}, {20, 58}} {
		cols, rows := FitCells(img, 1, 2, box.cols, box.rows)
		if cols != box.cols || rows != box.rows {
			t.Errorf("a %dx%d box gave %dx%d, want %dx%d: at the exact boundary both "+
				"dimensions are spent at once and the box comes out full",
				box.cols, box.rows, cols, rows, box.cols, box.rows)
		}
	}
}
