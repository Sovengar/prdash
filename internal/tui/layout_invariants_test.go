package tui

import (
	"strings"
	"testing"
)

// The parts have to add up to the terminal height, because the view fills it exactly and any
// mismatch pushes the hints or the header off screen.
func TestComputeLayoutTheSumIsTheHeight(t *testing.T) {
	for _, show := range []bool{true, false} {
		for _, hintAvailable := range []int{-1, 0, 1, 2, 3, 4, 10} {
			for height := -3; height <= 80; height++ {
				lay := computeLayout(height, hintAvailable, show)

				if !show {
					if lay != (layout{}) {
						t.Errorf("show=false gave %+v, want the empty view: before the first "+
							"WindowSizeMsg nothing is clipped", lay)
					}
					continue
				}
				if height <= 0 {
					if lay != (layout{}) {
						t.Errorf("with height %d it gave %+v, want the empty view", height, lay)
					}
					continue
				}

				if lay.bodyLines < 1 {
					t.Errorf("height %d, keybinds %d: the center body ended at %d: "+
						"it cannot disappear", height, hintAvailable, lay.bodyLines)
				}

				// THE FORMULA: everything reserved plus the body gives the exact height, except when the height
				//is below what is reserved.
				reservado := listChrome + detailChrome + lay.detailLines
				if lay.showHeader {
					reservado += headerLines
				}
				if lay.showKeybinds {
					reservado += keybindsChrome + lay.hintLines
				}
				wants := max(1, height-reservado)
				if lay.bodyLines != wants {
					t.Errorf("height %d, keybinds %d: body %d with %d reserved, want %d. "+
						"The mismatch pushes a box off screen, and the symptom shows up in another box",
						height, hintAvailable, lay.bodyLines, reservado, wants)
				}
				// No height is NEGATIVE. A detail at -1 lines is not a small detail: the renderer sees an
				//invalid box.
				if lay.detailLines < 0 {
					t.Errorf("height %d, keybinds %d: the detail ended at %d lines: a negative height is not a height",
						height, hintAvailable, lay.detailLines)
				}
				if lay.hintLines < 0 {
					t.Errorf("height %d, keybinds %d: the keybinds ended at %d lines", height, hintAvailable, lay.hintLines)
				}
				if lay.showKeybinds && lay.hintLines < 1 {
					t.Errorf("height %d, keybinds %d: the keybinds box is visible with %d lines",
						height, hintAvailable, lay.hintLines)
				}
				if lay.bodyLines > minListRows && lay.detailLines < minDetailRows {
					t.Errorf("height %d, keybinds %d: the detail dropped to %d with the center body at %d, "+
						"which can still hold its minimum of %d",
						height, hintAvailable, lay.detailLines, lay.bodyLines, minDetailRows)
				}

				// When the height works out, the sum is exact.
				if height > reservado {
					if got := lay.bodyLines + reservado; got != height {
						t.Errorf("altura %d, atajos %d: body %d + reservado %d = %d, want %d",
							height, hintAvailable, lay.bodyLines, reservado, got, height)
					}
				}
			}
		}
	}
}

// The cascade degrades IN ORDER and each step has a floor respected until there is nothing left.
func TestComputeLayoutRespectsTheFloorsBeforeRemovingThem(t *testing.T) {
	for _, height := range []int{5, 6, 7, 8, 9, 10, 12, 15, 18, 20, 24, 30, 40, 60, 80} {
		for _, hintAvailable := range []int{0, 1, 2, 3, 5} {
			lay := computeLayout(height, hintAvailable, true)
			if lay.showKeybinds && lay.hintLines < 1 {
				t.Errorf("height %d, keybinds %d: the keybinds box is visible with %d lines: "+
					"a visible box with no content is a gap", height, hintAvailable, lay.hintLines)
			}
			if lay.hintLines > maxHintLines {
				t.Errorf("height %d, keybinds %d: the keybinds ask for %d lines, more than the cap %d: "+
					"the box never asks for more lines than will be painted",
					height, hintAvailable, lay.hintLines, maxHintLines)
			}
			if lay.showKeybinds && hintAvailable > 0 && lay.hintLines > hintAvailable {
				t.Errorf("height %d, keybinds %d: the box asks for %d lines and there are only %d",
					height, hintAvailable, lay.hintLines, hintAvailable)
			}
		}
	}
}

// One fewer terminal line yields exactly one more thing given up, in the cascade's order.
func TestComputeLayoutGivesUpMoreAsTheHeightDrops(t *testing.T) {
	for _, hintAvailable := range []int{0, 1, 2, 3, 5} {
		previous := computeLayout(5, hintAvailable, true)
		for height := 6; height <= 80; height++ {
			current := computeLayout(height, hintAvailable, true)

			if current.detailLines < previous.detailLines {
				t.Errorf("height %d: the detail dropped to %d from %d when going up from %d: "+
					"growing cannot remove", height, current.detailLines, previous.detailLines, height-1)
			}
			if previous.showHeader && !current.showHeader {
				t.Errorf("height %d: the header disappeared when going up from %d", height, height-1)
			}
			if previous.showKeybinds && !current.showKeybinds {
				t.Errorf("height %d: the keybinds disappeared when going up from %d", height, height-1)
			}
			if current.hintLines < previous.hintLines && !current.showKeybinds {
			} else if current.hintLines < previous.hintLines {
				t.Errorf("height %d: the keybinds went from %d to %d when going up: growing cannot remove",
					height, previous.hintLines, current.hintLines)
			}
			if current.detailLines-previous.detailLines > 1 {
				t.Errorf("height %d: the detail jumped from %d to %d with one more terminal line",
					height, previous.detailLines, current.detailLines)
			}
			previous = current
		}
	}
}

// The comments' budget is what is left after the header, and what is not comments is discounted.
func TestCommentBudgetDiscountsWhatIsNotComments(t *testing.T) {
	for rows := -5; rows <= 60; rows++ {
		for grid := 0; grid <= 20; grid++ {
			for url := 0; url <= 4; url++ {
				for avisos := 0; avisos <= 5; avisos++ {
					got := commentBudget(rows, grid, url, avisos)
					want := rows - grid - url - 2 - avisos
					if got != want {
						t.Fatalf("commentBudget(%d,%d,%d,%d) = %d, want %d",
							rows, grid, url, avisos, got, want)
					}
				}
			}
		}
	}

	if got := commentBudget(20, 6, 1, 0); got != 11 {
		t.Errorf("commentBudget(20, 6, 1, 0) = %d, want 11 (20 - 6 - 1 - 2 - 0)", got)
	}
	if commentBudget(20, 6, 1, 0) <= commentBudget(19, 6, 1, 0) {
		t.Error("more rows do not give more budget: the budget has to grow with the rows")
	}
	if got := commentBudget(3, 20, 4, 5); got >= 0 {
		t.Errorf("with a header bigger than the panel it gave %d of budget, want negative", got)
	}
	headerOnly := commentBudget(18, 0, 0, 0)
	if headerOnly != 16 {
		t.Errorf("with 18 rows, 0 fields, 0 url and 0 notices it gave %d, want 16: the title and the gap are 2", headerOnly)
	}
	base := commentBudget(20, 5, 1, 2)
	for _, c := range []struct {
		name string
		got  int
		want int
	}{
		{"more fields", commentBudget(20, 6, 1, 2), base - 1},
		{"more notices", commentBudget(20, 5, 1, 3), base - 1},
		{"more url", commentBudget(20, 5, 2, 2), base - 1},
		{"fewer rows", commentBudget(19, 5, 1, 2), base - 1},
	} {
		if c.got != c.want {
			t.Errorf("%s: gave %d, want %d (one more part takes a row away)", c.name, c.got, c.want)
		}
	}
}

// Clipped from the TOP, so the END of the detail —state, review and role — stays visible.
func TestClipTopClipsFromTheTopNotFromTheBottom(t *testing.T) {
	lines := []string{"a", "b", "c", "d", "e"}

	for _, rows := range []int{5, 6, 100} {
		if got := clipTop(lines, rows); len(got) != len(lines) || &got[0] != &lines[0] {
			t.Errorf("clipTop with %d rows gave %q, want the five intact", rows, got)
		}
	}
	if got := clipTop(lines, 3); !equalStrings(got, []string{"c", "d", "e"}) {
		t.Errorf("clipTop with 3 rows gave %q, want [c d e]: it clips from the top", got)
	}
	if got := clipTop(lines, 1); !equalStrings(got, []string{"e"}) {
		t.Errorf("clipTop with 1 row gave %q, want [e]", got)
	}
	if got := clipTop(lines, 5); len(got) != 5 {
		t.Errorf("clipTop with exactly 5 rows gave %d, want 5", len(got))
	}
	for _, rows := range []int{0, -1, -100} {
		if got := clipTop(lines, rows); len(got) != len(lines) {
			t.Errorf("clipTop with %d rows gave %d lines, want the %d intact",
				rows, len(got), len(lines))
		}
	}
	for _, rows := range []int{-5, 0, 1, 5} {
		if got := clipTop(nil, rows); len(got) != 0 {
			t.Errorf("clipTop(nil, %d) gave %d lines, want none", rows, len(got))
		}
	}
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestDetailGridSplitsIntoTwoColumns(t *testing.T) {
	forVar := func(fields ...string) []detailField {
		fs := make([]detailField, len(fields))
		for i, c := range fields {
			fs[i] = detailField{key: c, value: "v"}
		}
		return fs
	}

	for _, c := range []struct {
		name   string
		fields []detailField
		rows   int
	}{
		{"none", nil, 0},
		{"one", forVar("a"), 1},
		{"two", forVar("a", "b"), 1},
		{"three", forVar("a", "b", "c"), 2},
		{"cuatro", forVar("a", "b", "c", "d"), 2},
		{"cinco", forVar("a", "b", "c", "d", "e"), 3},
		{"six", forVar("a", "b", "c", "d", "e", "f"), 3},
	} {
		got := detailGrid(c.fields, 80)
		if len(got) != c.rows {
			t.Errorf("%s: %d fields gave %d rows, want %d", c.name, len(c.fields), len(got), c.rows)
		}
		for i, l := range got {
			if strings.Contains(l, "\n") {
				t.Errorf("%s: row %d has a jump inside: %q", c.name, i, l)
			}
		}
	}
}
