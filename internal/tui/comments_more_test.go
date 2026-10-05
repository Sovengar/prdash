package tui

import (
	"strings"
	"testing"

	"prdash/internal/forge/model"
	"prdash/internal/testutil"
)

func TestWithNoRowsNotEvenALoadingNoticeIsPainted(t *testing.T) {
	adapt := &testutil.FakeAdapter{ForgeName: "github", HostName: "github.com"}

	states := []struct {
		name string
		st   *commentState
	}{
		{"without asking", &commentState{}},
		{"loading", &commentState{list: []model.Comment{{Author: "a", Body: "b"}}}},
		{"with error", &commentState{ready: true, err: "boom"}},
		{"without comments", &commentState{ready: true}},
		{"ready with comments", &commentState{ready: true,
			list: []model.Comment{{Author: "a", Body: "b"}}}},
	}

	for _, e := range states {
		for _, avail := range []int{-5, 0} {
			m := newTestModel(t, adapt)
			it := mkItem("github", "github.com", "acme/widget", "one", 1, "")
			st := *e.st
			m.comments[it.ID()] = &st

			if got := m.commentLines(it, avail, m.contentWidth()); len(got) != 0 {
				t.Errorf("state %q with %d rows gave %d lines (%q): with no rows there is no "+
					"box, and loose text is not a box. With the guard set to "+
					"`avail < 0` the states switch returns loading/error before "+
					"reaching the check that absorbs it",
					e.name, avail, len(got),
					firstLineWith(stripANSI(strings.Join(got, " ")), 80))
			}
		}
	}

	// This is what separates `avail <= 0` from `avail <= 1`: with the floor at one, a single row still
	//has room.
	withText := states[1:4]
	for _, e := range withText {
		for _, avail := range []int{1, 2} {
			m := newTestModel(t, adapt)
			it := mkItem("github", "github.com", "acme/widget", "one", 1, "")
			st := *e.st
			m.comments[it.ID()] = &st

			if got := m.commentLines(it, avail, m.contentWidth()); len(got) == 0 {
				t.Errorf("state %q with %d rows gave no line at all: the states "+
					"on their own are one line and with one row there is room for them. With "+
					"the floor at one it is lost, and a row lost from a state on its own "+
					"makes it invisible that the PR has a conversation",
					e.name, avail)
			}
		}
	}
}

func TestTheBoxWidthDiscountsTheIndentAndHasAFloor(t *testing.T) {
	for _, outer := range []int{10, 11, 20, 60, 100, 200} {
		want := outer - 2*commentInset
		if got := commentBoxWidth(outer); got != want {
			t.Errorf("with a room of %d columns the box measures %d, want %d: the indent "+
				"is %d per side and there are two sides", outer, got, want, commentInset)
		}
	}

	for _, outer := range []int{-10, 0, 5, 9} {
		got := commentBoxWidth(outer)
		if got != 8 {
			t.Errorf("with a room of %d columns the box measures %d, want 8: below the "+
				"floor the floor comes out, because an author name that does not fit even clipped "+
				"makes the box unreadable", outer, got)
		}
	}
}

// "In its place" is the part that matters: the last row is recomposed, not clipped.
func TestTheCutRowIsRecomposedInItsPlace(t *testing.T) {
	adapt := &testutil.FakeAdapter{ForgeName: "github", HostName: "github.com"}
	m := newTestModel(t, adapt)
	it := mkItem("github", "github.com", "acme/widget", "one", 1, "")

	paragraphs := make([]string, 10)
	for i := range paragraphs {
		paragraphs[i] = strings.Repeat("filler word for the phrase ", 4)
	}
	body := strings.Join(paragraphs, "\n\n")
	const inner = 40
	m.comments[it.ID()] = &commentState{ready: true, list: []model.Comment{
		{Author: "alice", Body: body},
	}, total: 1}

	for _, rows := range []int{1, 2, 3, 4, 5, 6} {
		got := m.commentLines(it, commentChrome+rows, inner)
		if len(got) == 0 {
			t.Fatalf("with %d rows no block came out at all", rows)
		}
		content := len(got) - commentChrome
		if content > rows {
			t.Errorf("with %d rows the block painted %d of content: it went past the length",
				rows, content)
		}
		plain := stripANSI(strings.Join(got, "\n"))
		if !strings.Contains(plain, "…") {
			t.Errorf("with %d rows the block comes out with no cut mark: %q", rows,
				firstLineWith(plain, 160))
		}
		// The LAST row of the CONTENT, not of the box: the box's last row is the bottom border with the
		//legend.
		withTheMarks := []string{}
		for _, l := range strings.Split(plain, "\n") {
			if strings.Contains(l, "…") {
				withTheMarks = append(withTheMarks, l)
			}
		}
		if len(withTheMarks) != 1 {
			t.Errorf("with %d rows there are %d rows with the cut mark, want 1: the cut "+
				"is marked once, on the last row that was painted", rows, len(withTheMarks))
			continue
		}
		lastOfText := ""
		for _, l := range strings.Split(plain, "\n") {
			if strings.Contains(l, "─") {
				continue
			}
			lastOfText = l
		}
		if !strings.Contains(lastOfText, "…") {
			t.Errorf("with %d rows the cut mark ended at %q and the last row of "+
				"text is %q: the mark has to fall on the last row that was painted",
				rows, withTheMarks[0], firstLineWith(lastOfText, 90))
		}

		// commentRow only branches on `idx == 0`, so the index passed to the recompose decides whether
		//that row still carries the author's name.
		firstOfText := ""
		for _, l := range strings.Split(plain, "\n") {
			if strings.Contains(l, "─") {
				continue
			}
			firstOfText = l
			break
		}
		if !strings.Contains(firstOfText, "alice") {
			t.Errorf("with %d rows the first text row is %q and does not carry the author: "+
				"it is row zero and only row zero carries it", rows,
				firstLineWith(firstOfText, 90))
		}
		if rows > 1 && strings.Contains(lastOfText, "alice") {
			t.Errorf("with %d rows the last row also carries the author: %q. With more "+
				"than one row only the first carries it, and seeing it twice makes you think "+
				"there are two comments", rows, firstLineWith(lastOfText, 90))
		}
	}
}

func TestTheSplitIsTheSameWhenTheBudgetIsExact(t *testing.T) {
	cases := []struct {
		need   []int
		budget int
		note   string
	}{
		{[]int{1, 1, 1}, 3, "three of one row with an exact budget"},
		{[]int{1, 5}, 6, "one short y one long, presupuesto exacto"},
		{[]int{5, 1}, 6, "the long one first: the split does not depend on the order"},
		{[]int{1, 1, 1, 1, 1}, 5, "the five of one row"},
		{[]int{3, 3, 3}, 9, "three equal ones with an exact budget"},
		{[]int{10, 1, 1, 1}, 13, "one huge and three of one"},
		{[]int{2, 2, 2, 2, 2}, 10, "five of two rows"},
	}
	for _, c := range cases {
		got := allocate(c.need, c.budget)
		if len(got) != len(c.need) {
			t.Errorf("%s: allocate returned %d rows for %d comments",
				c.note, len(got), len(c.need))
			continue
		}
		for i := range got {
			if got[i] != c.need[i] {
				t.Errorf("%s: allocate returned %v, want %v. With the exact budget "+
					"the split has to be the identity, or the splits `>` needs "+
					"a `>=` that does not exist", c.note, got, c.need)
				break
			}
		}
	}
}

// allocate starts at one row per comment, so a comment asking for ZERO rows would be left with a
// dangling index.
func TestNobodyAsksForZeroRows(t *testing.T) {
	bodies := []model.Comment{
		{Author: "alice"},
		{Author: "alice", Body: ""},
		{Author: "alice", Body: "\n\n\n"},
		{Author: "alice", Body: "   \n  \n"},
		{Author: "", Body: ""},
		{Author: "alice", Body: "one line"},
	}
	for _, c := range bodies {
		for _, lines := range []int{1, 2, 5, 20} {
			for _, width := range []int{12, 38, 60} {
				got := commentBody(c, lines, commentBodyWidth(width))
				if len(got) == 0 {
					t.Errorf("comment %+v with %d rows and width %d returned NOTHING: a "+
						"empty row reads as a deleted comment, and the floor to one "+
						"row is also what makes the split with an exact budget "+
						"be the identity",
						c, lines, width)
				}
				if len(got) > lines && lines >= 1 {
					t.Errorf("comment %+q with a cap of %d rows returned %d",
						c.Body, lines, len(got))
				}
			}
		}
	}
}

func firstLineWith(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n])
}
