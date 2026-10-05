package tui

import "testing"

func TestLandRowPicksTheLowestRowThatFits(t *testing.T) {
	opened := func(n int) []bool {
		rows := make([]bool, n)
		for i := range rows {
			rows[i] = true
		}
		return rows
	}

	base, ok := landRow(opened(10), 9, 3)
	if !ok {
		t.Fatal("a box of 3 rows in a window of 10 found no room, and there is spare gap")
	}
	if base != 9 {
		t.Errorf("a box of 3 rows in a window of 10 landed on row %d, want 9 (the lowest)", base)
	}
	if fromVar := base - 3 + 1; fromVar != 7 {
		t.Errorf("with the base at %d it occupies from %d, want 7", base, fromVar)
	}

	if _, ok := landRow(opened(3), 2, 10); ok {
		t.Error("a box of 10 rows found room in a window of 3")
	}

	if base, ok := landRow(opened(10), 9, 1); !ok || base != 9 {
		t.Errorf("a box of 1 row went to %d (ok=%v), want 9", base, ok)
	}
}

// The anchor is the limit: a warning already in place cannot move again.
func TestLandRowDoesNotLeaveTheAnchor(t *testing.T) {
	opened := func(n int) []bool {
		rows := make([]bool, n)
		for i := range rows {
			rows[i] = true
		}
		return rows
	}
	rows := opened(20)

	base, ok := landRow(rows, 4, 3)
	if !ok {
		t.Fatal("a box of 3 rows with the anchor at 4 found no room, and there is spare fit")
	}
	if base > 4 {
		t.Errorf("with the anchor at 4 the box landed at %d: it went past the anchor", base)
	}
	if base != 4 {
		t.Errorf("with the anchor at 4 and spare room the box landed at %d, want 4", base)
	}
	if _, ok := landRow(rows, 1, 3); ok {
		t.Error("a box of 3 rows found room with the anchor at 1: it does not fit above the anchor")
	}
	if base, ok := landRow(rows, 2, 3); !ok || base != 2 {
		t.Errorf("with the anchor at 2 the box of 3 rows went to %d (ok=%v), want 2", base, ok)
	}
}

func TestLandRowGoesAboveWhatItCannotAdmit(t *testing.T) {
	rows := make([]bool, 10)
	for i := range rows {
		rows[i] = true
	}
	rows[7], rows[8], rows[9] = false, false, false

	base, ok := landRow(rows, 9, 3)
	if !ok {
		t.Fatal("a box of 3 rows found no room with the 3 below closed, and the 7 above are free")
	}
	if base != 6 {
		t.Errorf("the box landed on row %d, want 6 (exactly above the closed ones)", base)
	}
	for i := base - 3 + 1; i <= base; i++ {
		if !rows[i] {
			t.Errorf("the box occupies row %d, which takes no notices: it splits a frame", i)
		}
	}

	rows2 := make([]bool, 10)
	for i := range rows2 {
		rows2[i] = true
	}
	rows2[8], rows2[9] = false, false
	if base, ok := landRow(rows2, 9, 2); !ok || base != 7 {
		t.Errorf("with a gap of 2 rows below the box of 2 went to %d (ok=%v), want 7", base, ok)
	}

	none := make([]bool, 5)
	if _, ok := landRow(none, 4, 3); ok {
		t.Error("a box of 3 rows found room in a window that admits no row")
	}
}

// ALL the rows have to admit it: one closed row is enough to refuse.
func TestNoticesAreAdmittedAllOrNothing(t *testing.T) {
	opened := func(n int) []bool {
		rows := make([]bool, n)
		for i := range rows {
			rows[i] = true
		}
		return rows
	}

	if !admitsWarning(opened(10), 0, 10) {
		t.Error("ten open rows do not admit themselves: the function is broken")
	}
	for _, closed := range []int{0, 3, 6, 9} {
		rows := opened(10)
		rows[closed] = false
		if admitsWarning(rows, 0, 10) {
			t.Errorf("with row %d closed, the whole block was admitted: half a notice breaks the frame just like a whole one", closed)
		}
	}
	if admitsWarning(opened(10), 8, 5) {
		t.Error("a block that escapes at the bottom was admitted: row 12 does not exist")
	}
	if admitsWarning(opened(10), -3, 2) {
		t.Error("a block with a negative index was admitted")
	}
	if !admitsWarning(opened(10), 8, 2) {
		t.Error("a block that ends exactly on the last row was not admitted")
	}
	if admitsWarning(opened(10), 9, 2) {
		t.Error("a block that ends one row past the last was admitted")
	}
	if !admitsWarning(opened(10), 5, 0) {
		t.Error("a block of zero rows should be admitted: nothing opposes it")
	}
	if admitsWarning(nil, 0, 1) {
		t.Error("a block over an empty slice was admitted")
	}
}

func TestLandRowWithABoxOfHeightZero(t *testing.T) {
	rows := make([]bool, 10)
	for i := range rows {
		rows[i] = true
	}
	for _, bh := range []int{0, -1, -10} {
		if base, ok := landRow(rows, 9, bh); ok {
			t.Errorf("a box of %d rows was placed at %d: there is nothing to paint", bh, base)
		}
	}
	if _, ok := landRow(rows, -1, 3); ok {
		t.Error("a box found room with a negative anchor")
	}
}
