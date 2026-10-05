package sim

import "testing"

// The exact boundary: the image's ratio makes `rows` of them measure EXACTLY the box width, so both
// dimensions are spent at once. The height rules there —the box comes out full— and the width cap
// may not quietly take a row away from it.
func TestFitCellsAtTheExactBoundaryKeepsAllTheRows(t *testing.T) {
	// 2*5/29 columns per row: 29 rows ask for exactly 10 columns, 58 for 20.
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
