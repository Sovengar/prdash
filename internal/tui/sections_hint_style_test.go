package tui

import (
	"strings"
	"testing"
)

func TestHintLinesPaintTheKeyWithTheCursorStyleAndKeepTheDescriptionDim(t *testing.T) {
	m := newTestModel(t)
	m.width = 400

	lines := m.hintLines()
	joined := strings.Join(lines, "\n")
	hints := m.cfg.Hints(m.dynamicHints())
	plain := strings.Join(wrapText(strings.Join(hints, hintSep), m.contentWidth()), "\n")

	if got := stripANSI(joined); got != plain {
		t.Fatalf("the hints changed after painting:\n got %q\nwant %q", got, plain)
	}

	for _, h := range hints {
		key, label, ok := strings.Cut(h, " ")
		if !ok {
			continue
		}
		if !strings.Contains(joined, styleCursor.Render(key)) {
			t.Errorf("the key %q is not painted with the cursor style: %q", key, joined)
		}
		for _, word := range strings.Fields(label) {
			if !strings.Contains(joined, styleHint.Render(word)) {
				t.Errorf("the description word %q is not painted with the hint style: %q", word, joined)
			}
		}
	}
	if !strings.Contains(joined, styleHint.Render("·")) {
		t.Errorf("the separator is not painted with the hint style: %q", joined)
	}
}

func TestHintLinesPaintKeysAtTheStartOfWrappedLines(t *testing.T) {
	m := newTestModel(t)
	hints := m.cfg.Hints(m.dynamicHints())
	keys := make(map[string]bool, len(hints))
	for _, h := range hints {
		key, _, _ := strings.Cut(h, " ")
		keys[key] = true
	}

	for width := 40; width <= 200; width++ {
		m.width = width
		lines := m.hintLines()
		wantPlain := wrapText(strings.Join(hints, hintSep), m.contentWidth())
		wantPlain = wantPlain[:max(0, min(maxHintLines, len(wantPlain)))]

		if len(lines) != len(wantPlain) {
			t.Fatalf("width %d: got %d hint lines, want %d", width, len(lines), len(wantPlain))
		}
		for i, line := range lines {
			plain := stripANSI(line)
			if plain != wantPlain[i] {
				t.Fatalf("width %d line %d: painted hint is %q, want %q", width, i, plain, wantPlain[i])
			}
			for _, word := range strings.Fields(plain) {
				if keys[word] && !strings.Contains(line, styleCursor.Render(word)) {
					t.Errorf("width %d line %d: key %q lost its accent after wrapping: %q", width, i, word, line)
				}
			}
		}
	}
}

func TestMergeConfirmKeepsTheWholeLineWarning(t *testing.T) {
	m := newTestModel(t)
	m.mergeArmed = true

	for _, line := range m.hintLines() {
		plain := stripANSI(line)
		if want := styleWarn.Render(plain); line != want {
			t.Errorf("the merge confirmation changed its style: got %q, want %q", line, want)
		}
	}
}
