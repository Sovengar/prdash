package tui

import (
	"fmt"
	"os"
	"slices"
	"strings"
	"testing"
	"time"
)

func TestMain(m *testing.M) {
	done := make(chan struct{})
	go func() {
		next := 0
		paintHintLine("q quit", []bool{true}, &next)
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		fmt.Fprintln(os.Stderr, "paintHintLine did not terminate")
		os.Exit(1)
	}
	os.Exit(m.Run())
}

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

func TestHintKeyMarksMarkOnlyTheFirstWordOfEachHint(t *testing.T) {
	tests := []struct {
		name  string
		hints []string
		want  []bool
	}{
		{"no hints", nil, []bool{}},
		{"one hint", []string{"q quit"}, []bool{true, false}},
		{"multi-word description", []string{"tab select the next section"}, []bool{true, false, false, false, false}},
		{"several hints", []string{"q quit", "j/k move"}, []bool{true, false, false, true, false}},
		{"empty hint between", []string{"", "q quit"}, []bool{false, true, false}},
		{"blank hint between", []string{"  \t ", "q quit"}, []bool{false, true, false}},
		{"only empty", []string{""}, []bool{}},
		{"only blank", []string{"   "}, []bool{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := hintKeyMarks(tt.hints); !slices.Equal(got, tt.want) {
				t.Errorf("hintKeyMarks(%q) = %v, want %v", tt.hints, got, tt.want)
			}
		})
	}
}

func TestPaintHintLinePaintsOnlyMarkedTokens(t *testing.T) {
	tests := []struct {
		name  string
		line  string
		marks []bool
		want  string
	}{
		{
			"marked token",
			"q quit",
			[]bool{true},
			styleCursor.Render("q") + " " + styleHint.Render("quit"),
		},
		{
			"leading space",
			" q quit",
			[]bool{true},
			" " + styleCursor.Render("q") + " " + styleHint.Render("quit"),
		},
		{
			"marker exhausts before the line",
			"q quit now",
			[]bool{true},
			styleCursor.Render("q") + " " + styleHint.Render("quit") + " " + styleHint.Render("now"),
		},
		{
			"more markers than words",
			"q",
			[]bool{true, false, true},
			styleCursor.Render("q"),
		},
		{
			"no markers",
			"q quit",
			nil,
			styleHint.Render("q") + " " + styleHint.Render("quit"),
		},
		{
			"empty line",
			"",
			[]bool{true},
			"",
		},
		{
			"blank line",
			"   ",
			[]bool{true},
			"   ",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			next := 0
			if got := paintHintLine(tt.line, tt.marks, &next); got != tt.want {
				t.Errorf("paintHintLine(%q, %v) = %q, want %q", tt.line, tt.marks, got, tt.want)
			}
		})
	}
}

func TestWrapHintKeysKeepsThePlainLineWhenThereAreNoHints(t *testing.T) {
	got := wrapHintKeys("", nil, 80)
	if len(got) != 1 || got[0] != "" {
		t.Errorf("wrapHintKeys with no hints = %q, want one empty line", got)
	}
}

func TestPaintHintLineTerminates(t *testing.T) {
	type painted struct {
		line string
		next int
	}
	done := make(chan painted, 1)
	go func() {
		next := 0
		done <- painted{paintHintLine("q quit", []bool{true}, &next), next}
	}()

	select {
	case got := <-done:
		want := styleCursor.Render("q") + " " + styleHint.Render("quit")
		if got.line != want || got.next != 2 {
			t.Errorf("paintHintLine = %q, next %d; want %q, next 2", got.line, got.next, want)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("paintHintLine did not terminate")
	}
}
