package tui

import "testing"

func heightOf(lay layout) int {
	n := lay.bodyLines + listChrome + detailChrome + lay.detailLines
	if lay.showHeader {
		n += headerLines
	}
	if lay.showKeybinds {
		n += keybindsChrome + lay.hintLines
	}
	return n
}

// The invariant the whole view rests on: each box gives up its height in turn.
func TestComputeLayoutFillsTheHeight(t *testing.T) {
	for _, height := range []int{10, 14, 20, 24, 30, 40, 60, 120} {
		lay := computeLayout(height, 1, true)
		if got := heightOf(lay); got != height {
			t.Errorf("computeLayout(%d) adds up to %d lines, want %d (%+v)", height, got, height, lay)
		}
		if lay.bodyLines < 1 {
			t.Errorf("computeLayout(%d) leaves the center body at %d: without it there is nothing to look at", height, lay.bodyLines)
		}
	}
}

func TestComputeLayoutDetailFortyPercent(t *testing.T) {
	for _, tc := range []struct{ height, detail int }{
		{40, 16},
		{30, 12},
		{24, 9},
	} {
		lay := computeLayout(tc.height, 1, true)
		if lay.detailLines != tc.detail {
			t.Errorf("computeLayout(%d) detail = %d, want %d", tc.height, lay.detailLines, tc.detail)
		}
		if !lay.showHeader || !lay.showKeybinds {
			t.Errorf("computeLayout(%d) = %+v; with %d lines the four boxes fit", tc.height, lay, tc.height)
		}
	}
}

func TestComputeLayoutDegradesWithoutLosingTheBody(t *testing.T) {
	lay := computeLayout(14, 1, true)
	if lay.showHeader {
		t.Errorf("computeLayout(14) = %+v; the header box should drop", lay)
	}
	if lay.bodyLines < minListRows {
		t.Errorf("center body = %d, want >= %d", lay.bodyLines, minListRows)
	}
	if lay.detailLines < minDetailRows {
		t.Errorf("detail = %d, want >= %d", lay.detailLines, minDetailRows)
	}

	lay = computeLayout(10, 1, true)
	if lay.showHeader || lay.showKeybinds {
		t.Errorf("computeLayout(10) = %+v; in 10 lines only body and panel fit", lay)
	}
	if lay.bodyLines < 1 || lay.detailLines < 1 {
		t.Errorf("computeLayout(10) = %+v; body and panel cannot be left at zero", lay)
	}
}

func TestComputeLayoutClipsHintsBeforeHidingThem(t *testing.T) {
	lay := computeLayout(40, 3, true)
	if lay.showKeybinds && lay.hintLines != maxHintLines {
		t.Errorf("with spare room the hints = %d, want %d (%+v)", lay.hintLines, maxHintLines, lay)
	}
	lay = computeLayout(40, 1, true)
	if lay.hintLines != 1 {
		t.Errorf("hints = %d, want 1 (%+v)", lay.hintLines, lay)
	}
	lay = computeLayout(9, 1, true)
	if lay.showKeybinds {
		t.Errorf("computeLayout(9) = %+v; the keybinds box should drop", lay)
	}
	if lay.hintLines != 0 {
		t.Errorf("hints = %d after hiding the box, want 0", lay.hintLines)
	}
}

func TestComputeLayoutWithNoHeightItDoesNotClip(t *testing.T) {
	for _, show := range []bool{false, true} {
		lay := computeLayout(0, 1, show)
		if lay.bodyLines != 0 || lay.detailLines != 0 {
			t.Errorf("computeLayout(0, show=%v) = %+v; want ceros", show, lay)
		}
		if lay.showHeader || lay.showKeybinds {
			t.Errorf("computeLayout(0, show=%v) = %+v; with no height nothing is decided", show, lay)
		}
	}
}
