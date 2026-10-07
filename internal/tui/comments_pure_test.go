package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"prdash/internal/sim"
)

// Text that does not fit has to SAY that something was lost.
func TestClipRunesMarksWhatWasLost(t *testing.T) {
	short := "abcdef"
	cases := []struct {
		n    int
		want string
	}{
		{6, "abcdef"},
		{7, "abcdef"},
		{100, "abcdef"},
		{1, "…"},
		{2, "a…"},
		{3, "ab…"},
		{5, "abcd…"},
		{0, ""},
		{-1, ""},
		{-50, ""},
	}
	for _, c := range cases {
		if got := clipRunes([]rune(short), c.n); got != c.want {
			t.Errorf("clipRunes(%q, %d) = %q, want %q", short, c.n, got, c.want)
		}
	}
	for _, n := range []int{-1, 0, 1, 5} {
		if got := clipRunes(nil, n); got != "" {
			t.Errorf("clipRunes(nil, %d) = %q, want %q", n, got, "")
		}
	}

	multibyte := []rune("ñáé")
	if got := clipRunes(multibyte, 3); got != "ñáé" {
		t.Errorf("clipRunes with 3 runes gave %q, want the whole text: the count is in runes, not bytes", got)
	}
	if got := clipRunes(multibyte, 2); got != "ñ…" {
		t.Errorf("clipRunes with 2 runes gave %q, want %q", got, "ñ…")
	}
	// The result is always printable and measurable: a cut mid-utf-8 sequence breaks the width.
	for n := 1; n <= 8; n++ {
		got := clipRunes([]rune("ñáéíóú"), n)
		if !utf8Valid(got) {
			t.Errorf("clipRunes at %d gave %q, which is not valid utf-8", n, got)
		}
		if w := ansi.StringWidth(got); w > max(n, 1) {
			t.Errorf("clipRunes at %d gave %q of %d columns, which do not fit", n, got, w)
		}
	}
}

func utf8Valid(s string) bool {
	for _, r := range s {
		if r == '�' {
			return false
		}
	}
	return true
}

func TestCommentBoxWidthRespectsTheFloorAndTheIndent(t *testing.T) {
	for outer := -10; outer <= 60; outer++ {
		got := commentBoxWidth(outer)
		want := max(8, outer-2*commentInset)
		if got != want {
			t.Errorf("commentBoxWidth(%d) = %d, want %d", outer, got, want)
		}
	}
	// The floor is exactly 8, not "almost 8": with 9 of outer width the interior is 7.
	for _, outer := range []int{0, 5, 9, 10} {
		if got := commentBoxWidth(outer); got != 8 {
			t.Errorf("commentBoxWidth(%d) = %d, want the floor of 8", outer, got)
		}
	}
	if got := commentBoxWidth(11); got != 9 {
		t.Errorf("commentBoxWidth(11) = %d, want 9: from here on the indent rules over the floor", got)
	}
	for outer := 10; outer <= 40; outer++ {
		if got := commentBoxWidth(outer); got != outer-2 {
			t.Errorf("commentBoxWidth(%d) = %d, want %d", outer, got, outer-2)
		}
	}
}

func TestPadRightAlignsByColumnsNotByBytes(t *testing.T) {
	for _, s := range []string{"", "a", "ab", "one", "a-label long"} {
		for n := 0; n <= 20; n++ {
			got := padRight(s, n)
			if w := ansi.StringWidth(got); w != max(n, ansi.StringWidth(s)) {
				t.Errorf("padRight(%q, %d) mide %d columnas, want %d", s, n, w, max(n, ansi.StringWidth(s)))
			}
			ifTrimmed := strings.TrimRight(got, " ")
			if ifTrimmed != s {
				t.Errorf("padRight(%q, %d) = %q, want the same text with padding", s, n, got)
			}
		}
	}
	if got := padRight("ñ", 5); ansi.StringWidth(got) != 5 {
		t.Errorf("padRight(%q, 5) measures %d columns, want 5: padding counts columns, not bytes", "ñ", ansi.StringWidth(got))
	}
	withColor := "\x1b[31mrojo\x1b[0m"
	if got := padRight(withColor, 10); ansi.StringWidth(got) != 10 {
		t.Errorf("padRight with ANSI measures %d columns, want 10: color codes are not columns", ansi.StringWidth(got))
	}
	if !strings.Contains(padRight(withColor, 10), "\x1b[31m") {
		t.Error("padRight ate the texts color")
	}
}

func TestAllocateSplitsTheRowsRegardlessOfArrivalOrder(t *testing.T) {
	cases := []struct {
		name   string
		need   []int
		budget int
		want   []int
	}{
		{
			"spare for all: each one asks for its own",
			[]int{1, 2, 3}, 10,
			[]int{1, 2, 3},
		},
		{
			"exact: nobody receives more than asked",
			[]int{2, 2}, 4,
			[]int{2, 2},
		},
		{
			// The one with least does NOT get it: it goes to whoever has least AND STILL WANTS more.
			"the one with the least gets nothing if it already has all its text",
			[]int{3, 1}, 4,
			[]int{3, 1},
		},
		{
			// The levelling is real: one asking a lot and two asking little all end up the same.
			"the big one does not eat the budget",
			[]int{4, 2, 2}, 8,
			[]int{4, 2, 2},
		},
		{
			"nobody absorbs more: spare budget and it is not distributed",
			[]int{1, 1}, 10,
			[]int{1, 1},
		},
		{
			"one alone, with spare budget",
			[]int{4}, 10,
			[]int{4},
		},
		{
			"one alone, with a short budget",
			[]int{4}, 3,
			[]int{3},
		},
		{
			// The budget can be SMALLER than the number of comments, and then everyone gets one row.
			"the budget is smaller than the comments: all to one row, and it goes past",
			[]int{5, 5, 5}, 2,
			[]int{1, 1, 1},
		},
		{
			"zero budget: all to one row anyway",
			[]int{5, 5}, 0,
			[]int{1, 1},
		},
		{
			"nothing to distribute",
			nil, 10,
			nil,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := allocate(c.need, c.budget)
			if !sameInt(got, c.want) {
				t.Errorf("allocate(%v, %d) = %v, want %v", c.need, c.budget, got, c.want)
			}
			// The sum never goes over the budget, UNLESS the budget is smaller than the number of comments.
			sum := 0
			for _, r := range got {
				sum += r
			}
			if limit := max(c.budget, len(c.need)); sum > limit {
				t.Errorf("allocate(%v, %d) distributed %d rows, more than the limit of %d",
					c.need, c.budget, sum, limit)
			}
			for i, r := range got {
				if c.need[i] > 0 && r < 1 {
					t.Errorf("allocate(%v, %d)[%d] = %d: a comment needs at least one row to be seen",
						c.need, c.budget, i, r)
				}
			}
			for i, r := range got {
				if r > c.need[i] {
					t.Errorf("allocate(%v, %d)[%d] = %d, more than it asked", c.need, c.budget, i, r)
				}
			}
		})
	}

	// The result does not depend on input order: the same numbers in another order allocate the same.
	base := allocate([]int{6, 1, 1, 6, 1}, 12)
	for _, perm := range [][]int{
		{1, 6, 1, 6, 1},
		{1, 1, 6, 1, 6},
		{6, 6, 1, 1, 1},
	} {
		got := allocate(perm, 12)
		if !sameMultiset(base, got) {
			t.Errorf("allocate with the same needs in another order gave a different split: %v vs %v", base, got)
		}
	}
	if base[1] == 1 && base[2] == 1 && base[0] == 6 {
		t.Error("the split concentrates the budget in the first ones: that only depends on arrival order")
	}
}

func sameInt(a, b []int) bool {
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

func sameMultiset(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	copy := append([]int(nil), a...)
	for _, v := range b {
		en := -1
		for i, c := range copy {
			if c == v {
				en = i
				break
			}
		}
		if en < 0 {
			return false
		}
		copy = append(copy[:en], copy[en+1:]...)
	}
	return true
}

func TestTheSelectorCursorWrapsAroundBothWays(t *testing.T) {
	restore := simKinds
	simKinds = []sim.Kind{sim.KindMerge, sim.KindRebase, sim.Kind("other")}
	t.Cleanup(func() { simKinds = restore })

	// Three kinds on purpose: with only two, cursor±1 would both land on the one and wrap would be untestable.
	n := len(simKinds)
	if n < 3 {
		t.Fatalf("3 kinds are needed for +1 and -1 to be distinguishable, there are %d", n)
	}
	m := simModel(t, &fakeSimulator{available: true})
	m = press(t, m, "v")

	m.sim.cursor = 0
	for i := range n {
		m = press(t, m, "j")
		if want := (i + 1) % n; m.sim.cursor != want {
			t.Fatalf("after %d down steps the cursor = %d, want %d", i+1, m.sim.cursor, want)
		}
	}

	m.sim.cursor = 0
	m = press(t, m, "k")
	if m.sim.cursor != n-1 {
		t.Errorf("up from the first gave %d, want %d (the last): a negative index here is a panic", m.sim.cursor, n-1)
	}
	m.sim.cursor = 0
	for i := range n {
		m = press(t, m, "k")
		if want := (n - 1 - i) % n; m.sim.cursor != want {
			t.Fatalf("after %d up steps the cursor = %d, want %d", i+1, m.sim.cursor, want)
		}
	}

	for _, alias := range []string{"j", "right", "tab"} {
		m.sim.cursor = 0
		m = press(t, m, alias)
		if m.sim.cursor != 1 {
			t.Errorf("key %q gave the cursor %d, want 1 (it is an alias of down)", alias, m.sim.cursor)
		}
	}
	m.sim.cursor = 0
	m = press(t, m, "k")
	if m.sim.cursor != n-1 {
		t.Errorf("key k gave the cursor %d, want %d (it is an alias of up)", m.sim.cursor, n-1)
	}
}
