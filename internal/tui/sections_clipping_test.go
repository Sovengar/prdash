package tui

import (
	"strings"
	"testing"
)

func TestWrapHintClipsToMaxHintLinesAndNothingMore(t *testing.T) {
	text := strings.Join(manyLines(), " ")

	for _, width := range []int{6, 8, 10, 12, 16} {
		plain := wrapText(text, width)
		if len(plain) <= maxHintLines {
			t.Fatalf("with width %d the text only gives %d lines, and the test needs more than "+
				"%d to be able to test the clipping", width, len(plain), maxHintLines)
		}
		got := wrapHint(text, width, func(s string) string { return "[" + s + "]" })
		if len(got) != maxHintLines {
			t.Errorf("with width %d it gave %d lines, want %d: the keybinds box is bounded "+
				"and does not grow with the text, because it is a footer",
				width, len(got), maxHintLines)
		}
		// What is kept is the HEAD of the text: the hint is read from the top.
		for i, l := range got {
			want := "[" + plain[i] + "]"
			if l != want {
				t.Errorf("with width %d line %d is %q, want %q: the clipping takes the "+
					"last ones, not the first", width, i, l, want)
			}
		}
	}
}

func TestWrapHintWithWidthOneAndZero(t *testing.T) {
	for _, width := range []int{-10, 0, 1, 2} {
		got := wrapHint("a text with a few words", width, func(s string) string { return s })
		if len(got) == 0 {
			t.Errorf("with width %d it returned no line, and the text does have words", width)
		}
		for i, l := range got {
			if l == "" {
				t.Errorf("with width %d line %d is empty: splitting a real text "+
					"never gives empty lines", width, i)
			}
		}
	}
}

// m.height > 0 IS the show that decides.
func TestTheHeightIsTheThirdArgumentOfTheLayout(t *testing.T) {
	for _, h := range []int{-40, -1, 0} {
		m := newTestModel(t)
		m.height = h
		lay := m.layout()
		if lay != (layout{}) {
			t.Errorf("with height %d the layout gave %+v, want empty: with no known height "+
				"nothing is clipped", h, lay)
		}
	}

	m := newTestModel(t)
	m.height = 40
	if lay := m.layout(); lay.bodyLines < 1 {
		t.Errorf("with height 40 it gave body=%d, want at least 1", lay.bodyLines)
	}
	// The budget passed is the hints ALREADY WRAPPED to the inner width.
	m2 := newTestModel(t)
	m2.height = 40
	wantHoras := len(m2.hintLines())
	if got := m2.layout(); got.hintLines > wantHoras {
		t.Errorf("the layout promises %d keybind lines and the box only has %d",
			got.hintLines, wantHoras)
	}
}

func manyLines() []string {
	out := make([]string, 0, 40)
	for range 40 {
		out = append(out, "atajo")
	}
	return out
}

func TestWrapHintTheTopBudgetIsTheIdentity(t *testing.T) {
	const width = 12
	var palabras []string
	for {
		palabras = append(palabras, "atajo")
		if len(wrapText(strings.Join(palabras, " "), width)) > maxHintLines {
			break
		}
	}
	palabras = palabras[:len(palabras)-1]
	text := strings.Join(palabras, " ")
	plain := wrapText(text, width)
	if len(plain) != maxHintLines {
		t.Fatalf("construction failed: I wanted exactly %d lines and the text gives %d "+
			"(with %d words)", maxHintLines, len(plain), len(palabras))
	}

	got := wrapHint(text, width, func(s string) string { return "[" + s + "]" })
	if len(got) != maxHintLines {
		t.Errorf("with a text of exactly %d lines it gave %d: the clipping at the edge "+
			"has to be the identity", maxHintLines, len(got))
	}
	for i := range got {
		if want := "[" + plain[i] + "]"; got[i] != want {
			t.Errorf("line %d: %q, want %q: at the edge the text is not touched", i, got[i], want)
		}
	}
}

func TestKeybindsTheBudgetEdgeIsTheIdentity(t *testing.T) {
	m := newTestModel(t)
	m.mergeArmed = true
	var all []string
	for width := 1; width <= 60 && len(all) < 2; width++ {
		m.width = width
		all = m.hintLines()
	}
	if len(all) < 2 {
		t.Fatalf("with the merge armed and the narrowest terminal the bar has %d lines, "+
			"and the test needs 2", len(all))
	}

	sec := m.keybindsSection(len(all))
	if sec.lines() != m.keybindsSection(len(all)+1).lines() {
		t.Errorf("with budget %d and with %d the box measures %d and %d rows: at the edge, "+
			"asking for one more line must not change anything",
			len(all), len(all)+1, sec.lines(), m.keybindsSection(len(all)+1).lines())
	}

	previous := m.keybindsSection(len(all)).lines()
	// The NEGATIVE case is what separates max(0, hintLines) from hintLines.
	negative := m.keybindsSection(-1)
	if negative.lines() != m.keybindsSection(0).lines() {
		t.Errorf("with budget -1 the box measures %d rows and with 0 it measures %d: the floor at "+
			"zero has to turn the negative into the same case as zero",
			negative.lines(), m.keybindsSection(0).lines())
	}

	// Zero against one is what separates max(0, ...) from max(1, ...).

	for p := len(all) - 1; p >= 0; p-- {
		current := m.keybindsSection(p).lines()
		if current > previous {
			t.Errorf("with budget %d the box measures %d rows and with %d it measures %d: "+
				"asking for LESS cannot take up more",
				p, current, p+1, previous)
		}
		previous = current
	}
}
