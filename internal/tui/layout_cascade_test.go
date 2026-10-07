package tui

import "testing"

func TestTheCascadePutsTheDetailBeforeTheBody(t *testing.T) {
	cases := []struct {
		height    int
		wantBody  int
		wantDetal int
		note      string
	}{
		{6, 1, 1, "the detail stays at its minimum, not at zero"},
		{14, 4, 6, "everything hidden: only list and detail, and the detail at its minimum"},
		{16, 3, 6, "the keybinds box comes back with one line"},
		{18, 3, 6, "the keybinds rise to their cap of three"},
		{21, 3, 6, "the header comes back, and the body stays at three"},
		{22, 3, 7, "the detail grows: it is its 40% and there is spare room"},
		{23, 3, 8, "and keeps growing while the body holds three"},
		{25, 3, 10, "the body minimum is three rows: the detail reaches that far"},
		{28, 5, 11, "past that point the body takes the remainder"},
	}

	for _, c := range cases {
		got := computeLayout(c.height, maxHintLines, true)
		if got.bodyLines != c.wantBody || got.detailLines != c.wantDetal {
			t.Errorf("height %d gave body=%d detail=%d, want body=%d detail=%d. %s",
				c.height, got.bodyLines, got.detailLines, c.wantBody, c.wantDetal, c.note)
		}
	}
}

func TestTheDetailRespectsItsMinimumWhileThereIsRoom(t *testing.T) {
	for h := 1; h <= 80; h++ {
		for _, hints := range []int{0, 1, maxHintLines, maxHintLines + 3} {
			got := computeLayout(h, hints, true)

			if got.detailLines < 0 {
				t.Errorf("height %d with %d keybinds gave detail=%d: degrading is removing, "+
					"not staying at negative", h, hints, got.detailLines)
			}

			// The minimum is honoured AS SOON AS THE TERMINAL ALLOWS IT; the threshold is what decides.
			impossible := minDetailRows + detailChrome + listChrome + 1
			if h >= impossible && got.detailLines < minDetailRows {
				t.Errorf("height %d with %d keybinds gave detail=%d, below the minimum %d, "+
					"and the terminal does have room for it (%d needed). With `>=` instead of `>` "+
					"the loop boundary clips one row too many and the card is left without "+
					"its last one",
					h, hints, got.detailLines, minDetailRows, impossible)
			}
			// Below the threshold the detail may drop, and that is correct. My first version said otherwise.
		}
	}
}

// The central body NEVER disappears: the list is what has to be scrollable.
func TestTheCenterBodyNeverDisappears(t *testing.T) {
	for h := 1; h <= 60; h++ {
		for _, hints := range []int{0, 2, maxHintLines} {
			got := computeLayout(h, hints, true)
			if got.bodyLines < 1 {
				t.Errorf("height %d with %d keybinds gave body=%d: the list is what has "+
					"to be scrollable, it cannot be left without rows", h, hints, got.bodyLines)
			}
			if got.detailLines < 0 || got.hintLines < 0 {
				t.Errorf("height %d gave detail=%d and keybinds=%d, negative: degrading is "+
					"removing, not staying at negative", h, got.detailLines, got.hintLines)
			}
		}
	}
}

func TestTheCascadeYieldsInTheDeclaredOrder(t *testing.T) {
	lay := computeLayout(14, maxHintLines, true)
	if !lay.showHeader && lay.detailLines > minDetailRows {
		t.Errorf("the header was hidden with the detail still at %d rows of its minimum "+
			"%d: the order says the detail gives up first", lay.detailLines, minDetailRows)
	}

	layChico := computeLayout(10, maxHintLines, true)
	if !layChico.showKeybinds && layChico.hintLines > 1 {
		t.Errorf("the keybinds box disappeared with %d lines inside: the hints drop "+
			"line by line before disappearing entirely", layChico.hintLines)
	}
}

func TestTheCascadeStopsAsSoonAsItFits(t *testing.T) {
	for h := 1; h <= 80; h++ {
		lay := computeLayout(h, maxHintLines, true)
		reservado := listChrome + detailChrome + lay.detailLines
		if lay.showHeader {
			reservado += headerLines
		}
		if lay.showKeybinds {
			reservado += keybindsChrome + lay.hintLines
		}
		// What is left over: if there were leftovers, the cascade could keep clipping.
		spare := reservado + minListRows - h
		if spare > 0 && (lay.detailLines > minDetailRows || lay.showHeader ||
			lay.hintLines > 1 || lay.showKeybinds) {
			t.Errorf("height %d: %d rows are spare and yet something is left to give up "+
				"(detail=%d header=%v keybinds=%v with %d lines): the cascade stopped too early",
				h, spare, lay.detailLines, lay.showHeader, lay.showKeybinds, lay.hintLines)
		}
		// And the central body keeps the leftover, which is the reason it never disappears.
		if lay.bodyLines != max(1, h-reservado) {
			t.Errorf("height %d: the body ended at %d, and with %d reserved it should be %d",
				h, lay.bodyLines, reservado, max(1, h-reservado))
		}
	}
}

func TestBeforeWindowSizeNothingIsClipped(t *testing.T) {
	empty := computeLayout(0, maxHintLines, false)
	if empty.bodyLines != 0 || empty.detailLines != 0 || empty.hintLines != 0 {
		t.Errorf("with no size it gave %+v, want an empty layout: before the first WindowSizeMsg "+
			"there is no height to distribute", empty)
	}
	zero := computeLayout(0, maxHintLines, true)
	if zero == empty {
		t.Log("with height zero and show the same empty layout is returned; it only asserts " +
			"that there is no crash")
	}
	for _, h := range []int{-1, -40} {
		if got := computeLayout(h, maxHintLines, true); got != (layout{}) {
			t.Errorf("with height %d it gave %+v, want the empty layout", h, got)
		}
	}
}
