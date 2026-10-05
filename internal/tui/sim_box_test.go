package tui

import (
	"slices"
	"strings"
	"testing"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"

	"prdash/internal/sim"
)

// The padding decides the box's HEIGHT.
func TestPadLinesFillsUpToTheAskedHeight(t *testing.T) {
	for _, have := range []int{0, 1, 3, 5, 12, 40} {
		lines := make([]string, have)
		for i := range lines {
			lines[i] = "x"
		}
		for _, want := range []int{0, 1, 3, 5, 12, 40} {
			got := padLines(lines, want)

			if len(got) < have {
				t.Errorf("padLines with %d lines and target %d returned %d: content was lost",
					have, want, len(got))
			}
			if len(got) < want {
				t.Errorf("padLines with %d lines and target %d returned %d: it did not pad",
					have, want, len(got))
			}
			for i := have; i < len(got); i++ {
				if got[i] != "" {
					t.Errorf("padding line %d is %q, want an empty line", i, got[i])
				}
			}
			for i := range have {
				if got[i] != "x" {
					t.Errorf("line %d became %q, want the original content", i, got[i])
				}
			}
		}
	}
	for _, want := range []int{-5, -1, 0} {
		if got := padLines([]string{"a", "b"}, want); len(got) != 2 {
			t.Errorf("padLines with target %d returned %d lines, want the 2 that were there", want, len(got))
		}
		if got := padLines(nil, want); len(got) != 0 {
			t.Errorf("padLines(nil, %d) returned %d lines, want none", want, len(got))
		}
	}
	// The input slice is not touched: padding copies, it does not mutate.
	orig := []string{"a", "b"}
	padLines(orig, 10)
	if len(orig) != 2 || orig[0] != "a" || orig[1] != "b" {
		t.Errorf("padLines mutated the input slice: %q", orig)
	}
}

// The sim popup's height is FIXED.
func TestTheSimBoxMeasuresWhatTheFillCommands(t *testing.T) {
	restore := simKinds
	simKinds = []sim.Kind{sim.KindMerge, sim.KindRebase, sim.Kind("other")}
	t.Cleanup(func() { simKinds = restore })

	for _, h := range []int{20, 30, 45, 60} {
		m := showSim(t, solidSim(4, 3, black))
		m = send(t, m, tea.WindowSizeMsg{Width: 100, Height: h})
		_, want := m.simBox()

		for _, cells := range []int{0, 1, 5} {
			m.sim.cells = make([]string, cells)
			for i := range m.sim.cells {
				m.sim.cells[i] = "cell"
			}
			lines := strings.Split(stripANSI(m.simImageBox()), "\n")
			if len(lines) != want {
				t.Errorf("terminal of %d rows with %d cells: the box measures %d, want %d: the padding never arrived",
					h, cells, len(lines), want)
			}
		}

		m.sim.cells = make([]string, 40)
		for i := range m.sim.cells {
			m.sim.cells[i] = "cell"
		}
		lines := strings.Split(stripANSI(m.simImageBox()), "\n")
		if wants := 40 + simChrome; len(lines) != wants {
			t.Errorf("with 40 cells in a box of %d the box measures %d, want %d: padding is not clipping",
				want, len(lines), wants)
		}
	}
}

// The cursor is the row enter picks.
func TestOnlyOneRowOfTheSelectorCarriesTheCursor(t *testing.T) {
	restore := simKinds
	simKinds = []sim.Kind{sim.KindMerge, sim.KindRebase, sim.Kind("other")}
	t.Cleanup(func() { simKinds = restore })

	m := simModel(t, &fakeSimulator{available: true})
	m = press(t, m, "v")

	for cursor := range len(simKinds) {
		m.sim.cursor = cursor
		marked := 0
		var withMarks string
		for _, l := range strings.Split(stripANSI(m.simChooserBox()), "\n") {
			if !strings.Contains(l, "▸") {
				continue
			}
			marked++
			withMarks = l
		}
		if marked != 1 {
			t.Errorf("cursor %d: %d rows marked, want exactamente 1", cursor, marked)
		}
		if marked == 1 && !strings.Contains(withMarks, string(simKinds[cursor])) {
			t.Errorf("cursor %d: the marked row is %q, want the one of %q", cursor, strings.TrimSpace(withMarks), simKinds[cursor])
		}
	}
}

// Each strategy's label is padded.
func TestTheSelectorFlowStartsAtAFixedColumn(t *testing.T) {
	restore := simKinds
	simKinds = []sim.Kind{sim.KindMerge, sim.KindRebase, sim.Kind("12345678")}
	t.Cleanup(func() { simKinds = restore })

	m := simModel(t, &fakeSimulator{available: true})
	m = press(t, m, "v")

	box := stripANSI(m.simChooserBox())
	// The flow's start is measured by the branch NAME, not the arrow.
	flujos := []int{}
	for kind, branch := range map[sim.Kind]string{
		sim.KindMerge:        "main",
		sim.KindRebase:       "feat/x",
		sim.Kind("12345678"): "main",
	} {
		l := lineWithKind(t, box, kind)
		i := strings.Index(l, branch)
		if i < 0 {
			t.Fatalf("the row of %q does not name %q: %q", kind, branch, strings.TrimSpace(l))
		}
		flujos = append(flujos, utf8.RuneCountInString(l[:i]))
	}
	slices.Sort(flujos)
	for i := 1; i < len(flujos); i++ {
		if flujos[i] != flujos[0] {
			t.Errorf("the flow of %q starts at column %d and the one of %q at %d: the label is not padded the same",
				simKinds[i], flujos[i], simKinds[0], flujos[0])
		}
	}
	// All three rows land in the SAME column.
	if flujos[0] != flujos[1] || flujos[1] != flujos[2] {
		t.Errorf("the flows start at columns %v: the label is not padded to the same width", flujos)
	}
}

// The merge and rebase arrows point in opposite directions.
func TestTheMergeArrowAndTheRebaseOnePointInOppositeDirections(t *testing.T) {
	restore := simKinds
	simKinds = []sim.Kind{sim.KindMerge, sim.KindRebase}
	t.Cleanup(func() { simKinds = restore })

	m := simModel(t, &fakeSimulator{available: true})
	m = press(t, m, "v")

	box := stripANSI(m.simChooserBox())
	merge := lineWithKind(t, box, sim.KindMerge)
	if !strings.Contains(merge, "←") {
		t.Errorf("the merge row should use the arrow that brings the branch to the base, gave %q", strings.TrimSpace(merge))
	}
	rebase := lineWithKind(t, box, sim.KindRebase)
	if !strings.Contains(rebase, "→") {
		t.Errorf("the rebase row should use the arrow that takes the base to the branch, gave %q", strings.TrimSpace(rebase))
	}
	if strings.Contains(merge, "→") || strings.Contains(rebase, "←") {
		t.Errorf("the two operations share a direction: merge=%q rebase=%q", strings.TrimSpace(merge), strings.TrimSpace(rebase))
	}
	for _, l := range []string{merge, rebase} {
		if !strings.Contains(l, "main") {
			t.Errorf("row %q does not name the base: without the two branches the arrow says nothing", strings.TrimSpace(l))
		}
	}
}

func lineWithKind(t *testing.T, box string, kind sim.Kind) string {
	t.Helper()
	for _, l := range strings.Split(box, "\n") {
		if strings.Contains(l, string(kind)) {
			return l
		}
	}
	t.Fatalf("there is no row for %q in:\n%s", kind, box)
	return ""
}
