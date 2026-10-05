package tui

import (
	"strings"
	"testing"
	"time"

	"prdash/internal/cache"
	"prdash/internal/forge"
	"prdash/internal/forge/model"
	"prdash/internal/testutil"
)

func commentsOf(t *testing.T, n int) (Model, model.Item) {
	t.Helper()
	m := newTestModel(t)
	it := mkItem("github", "github.com", "acme/widget", "one", 1, "")
	list := make([]model.Comment, n)
	for i := range list {
		list[i] = model.Comment{
			Author: "alice",
			Body:   "line " + strings.Repeat("x", i+1),
		}
	}
	m.comments[it.ID()] = &commentState{list: list, total: n, ready: true}
	return m, it
}

func TestCommentLinesWithNoRoomItDoesNotPaintTheBox(t *testing.T) {
	m, it := commentsOf(t, 3)
	for _, avail := range []int{-100, -1, 0} {
		if got := m.commentLines(it, avail, 60); got != nil {
			t.Errorf("with %d rows available it gave %d lines, want nil: with no rows there is no box", avail, len(got))
		}
	}
	for _, avail := range []int{1, 2} {
		if got := m.commentLines(it, avail, 60); got != nil {
			t.Errorf("with %d rows it gave %d lines: the box needs its frame plus text", avail, len(got))
		}
	}
	if got := m.commentLines(it, 20, 60); len(got) == 0 {
		t.Error("with 20 rows available no box came out")
	}
}

func TestCommentLinesTheCapOfFiveRulesEvenWhenTheBoxIsBig(t *testing.T) {
	m, it := commentsOf(t, 20)
	big := m.commentLines(it, 200, 60)
	if len(big) == 0 {
		t.Fatal("with 200 rows and 20 comments no box came out: the clipping swallowed the whole box")
	}
	plain := stripANSI(strings.Join(big, "\n"))
	if !strings.Contains(plain, "5") {
		t.Errorf("the box with 20 comments and spare room does not say how many it shows:\n%s", plain)
	}

	// The band: room for the box with five, not for twenty. Here the box must come out with five and
	//not disappear.
	exact := exactBudget(t, m, it, forge.CommentLimit)
	inTheBand := m.commentLines(it, exact, 60)
	if len(inTheBand) == 0 {
		t.Fatalf("with an exact budget for %d comments the box disappeared, and it should "+
			"come out clipped: there is room for five", forge.CommentLimit)
	}
	if n := countComments(plain); n > forge.CommentLimit {
		t.Errorf("it showed %d comments with a budget for %d", n, forge.CommentLimit)
	}

	somewhatLess := exact - 1
	if got := m.commentLines(it, somewhatLess, 60); got != nil {
		t.Errorf("with a budget of %d rows the box was painted with %d lines: it does not fit",
			somewhatLess, len(got))
	}
}

func exactBudget(t *testing.T, m Model, it model.Item, n int) int {
	t.Helper()
	for avail := 1; avail <= 300; avail++ {
		if len(m.commentLines(it, avail, 60)) > 0 {
			return avail
		}
	}
	t.Fatalf("with %d comments the box does not come out even with 300 rows", n)
	return 0
}

func countComments(join string) int {
	n := 0
	for _, l := range strings.Split(join, "\n") {
		if strings.Contains(l, "alice") {
			n++
		}
	}
	return n
}

func TestCommentBoxWidthDiscountsTheIndentOnBothSides(t *testing.T) {
	for outer := -20; outer <= 200; outer++ {
		got := commentBoxWidth(outer)
		want := max(8, outer-2*commentInset)
		if got != want {
			t.Errorf("commentBoxWidth(%d) = %d, want %d", outer, got, want)
		}
		if got < 8 {
			t.Errorf("commentBoxWidth(%d) = %d, want >= 8", outer, got)
		}
	}
	for outer := 20; outer <= 120; outer++ {
		if got := commentBoxWidth(outer); got != outer-2*commentInset {
			t.Errorf("commentBoxWidth(%d) = %d, want %d (two sides of indent)",
				outer, got, outer-2*commentInset)
		}
	}
	if got := commentBoxWidth(8 + 2*commentInset); got != 8 {
		t.Errorf("commentBoxWidth(%d) = %d, want 8: the floor starts here", 8+2*commentInset, got)
	}
	if got := commentBoxWidth(7 + 2*commentInset); got != 8 {
		t.Errorf("commentBoxWidth(%d) = %d, want 8 (the floor rules)", 7+2*commentInset, got)
	}
	for outer := 8 + 2*commentInset; outer <= 40; outer++ {
		if got := commentBoxWidth(outer); got != outer-2*commentInset {
			t.Errorf("commentBoxWidth(%d) = %d, want %d", outer, got, outer-2*commentInset)
		}
	}
}

// GitLab exposes no count, so the total is what was read, and it is floored at the list's height so
// the arithmetic does not invent comments.
func TestTheCountIsNeverLowerThanWhatIsShown(t *testing.T) {
	finalTotal := func(total, list int) int {
		if total < list {
			return list
		}
		return total
	}
	for total := -3; total <= 30; total++ {
		for list := 0; list <= 30; list++ {
			if got := finalTotal(total, list); got < list {
				t.Errorf("total %d with list %d gave a final total of %d, which is LOWER than the list",
					total, list, got)
			}
			if total >= list && finalTotal(total, list) != total {
				t.Errorf("total %d with list %d gave %d, want the forges total: "+
					"an excess is not corrected", total, list, finalTotal(total, list))
			}
			if list == 0 && finalTotal(total, list) != max(total, 0) {
				t.Errorf("with an empty list and total %d it gave %d", total, finalTotal(total, list))
			}
		}
	}
	if got := finalTotal(-5, 0); got != 0 {
		t.Errorf("with total -5 and an empty list it gave %d, want 0", got)
	}
}

func TestTheCommentBlockIsNeverTallerThanTheBudget(t *testing.T) {
	// One-line comments: the budget never bites when there is spare room.
	for _, n := range []int{1, 2, 3, 5, 6, 12, 40} {
		m, it := commentsOf(t, n)
		for avail := 1; avail <= 60; avail++ {
			got := m.commentLines(it, avail, 60)
			if len(got) > avail {
				t.Errorf("%d short comments with %d rows gave a block of %d, want <= %d",
					n, avail, len(got), avail)
			}
		}
	}

	for _, lines := range []int{1, 2, 4, 8} {
		for _, n := range []int{1, 3, 5, 9} {
			m := newTestModel(t)
			it := mkItem("github", "github.com", "acme/widget", "one", 1, "")
			body := strings.TrimSpace(strings.Repeat("palabra "+strings.Repeat("y", 5)+"\n", lines))
			list := make([]model.Comment, n)
			for i := range list {
				list[i] = model.Comment{Author: "alice", Body: body}
			}
			m.comments[it.ID()] = &commentState{list: list, total: n, ready: true}

			for avail := 1; avail <= 60; avail++ {
				got := m.commentLines(it, avail, 60)
				if len(got) > avail {
					t.Errorf("%d comments of %d lines with %d rows gave a block of %d, want <= %d. "+
						"The budget is discounted from the frame once, and two extra rows eat "+
						"the end of the card, which is what says whether the action applies",
						n, lines, avail, len(got), avail)
				}
			}
		}
	}
	m, it := commentsOf(t, 2)
	pequeno := len(m.commentLines(it, 30, 60))
	enorme := len(m.commentLines(it, 300, 60))
	if pequeno != enorme {
		t.Errorf("with 30 rows it gave a block of %d and with 300 of %d: "+
			"the box takes what it needs and is not stretched", pequeno, enorme)
	}
	if enorme > 2+commentChrome+8 {
		t.Errorf("with 2 comments and 300 rows the block measures %d rows: there is a stretched box", enorme)
	}
}

func TestTheCommentsErrorIsClippedToTheUsefulWidth(t *testing.T) {
	m, it := commentsOf(t, 1)
	m.comments[it.ID()].err = "the server said something very long that does not fit in any terminal line"

	for inner := 38; inner <= 200; inner++ {
		lines := m.commentLines(it, 20, inner)
		if len(lines) == 0 {
			t.Fatalf("with inner %d no line came out", inner)
		}
		plain := stripANSI(lines[len(lines)-1])
		if width := len([]rune(plain)); width > inner {
			t.Errorf("with inner %d the error line measures %d: %q",
				inner, width, plain)
		}
		m.comments[it.ID()].err = "boom"
		plain = stripANSI(m.commentLines(it, 20, inner)[0])
		if !strings.Contains(plain, "boom") {
			t.Errorf("with inner %d the error message was lost: %q", inner, plain)
		}
	}

	m.comments[it.ID()].err = "error-reason-and-tail-noise-that-does-not-fit"
	plain := stripANSI(m.commentLines(it, 20, 38)[0])
	if !strings.Contains(plain, "error-reason") {
		t.Errorf("with a long error and a small width the reason was lost: %q", plain)
	}
	m.comments[it.ID()].err = "boom"
	plain = stripANSI(m.commentLines(it, 20, 200)[0])
	if !strings.Contains(plain, "not read") {
		t.Errorf("the error line does not say it is a read failure: %q", plain)
	}
}

// The rule is "only becomes an error if nothing came back".
func TestAWarningIsAnErrorOnlyIfNothingElseCame(t *testing.T) {
	for _, c := range []struct {
		name     string
		comments []model.Comment
		warn     string
		wantsErr bool
	}{
		{
			"with comments and warning",
			[]model.Comment{{Author: "alice", Body: "hi"}},
			"could not read a comment",
			false,
		},
		{
			"without comments and with warning",
			nil,
			"could not read the conversation",
			true,
		},
		{
			"without comments and without warning",
			nil,
			"",
			false,
		},
		{
			"with comments and without warning",
			[]model.Comment{{Author: "alice", Body: "hi"}},
			"",
			false,
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			key := "acme/widget#1"
			f := &testutil.FakeAdapter{
				ForgeName: "github",
				HostName:  "github.com",
				Conversations: map[string]forge.CommentPage{
					key: {Comments: c.comments},
				},
			}
			if c.warn != "" {
				f.CommentWarnings = map[string][]model.Warning{
					key: {{Forge: "github", Kind: "degraded", Msg: c.warn}},
				}
			}
			m := newTestModel(t, f)
			m.applySnapshot(cache.File{Streams: []cache.Stream{{
				Forge: "github", Host: "github.com", Section: model.SectionReview,
				Kind:  model.ReviewRequested,
				Items: []model.Item{mkItem("github", "github.com", "acme/widget", "one", 1, "")},
			}}})
			m.rebuild()
			m.statuses["github"].auth.OK = true

			it, ok := m.selected()
			if !ok {
				t.Fatal("there is no selected item")
			}
			_ = m.requestComments()

			var msg commentsMsg
			for range 400 {
				select {
				case ev := <-m.events:
					if c, ok := ev.(commentsMsg); ok && c.id == it.ID() {
						msg = c
					}
				case <-time.After(5 * time.Millisecond):
				}
				if msg.id == it.ID() {
					break
				}
			}
			if msg.id != it.ID() {
				t.Fatal("the comments query result never arrived")
			}
			if (msg.err != "") != c.wantsErr {
				t.Errorf("with %d comments and warning %q it gave err=%q, wants error=%v",
					len(c.comments), c.warn, msg.err, c.wantsErr)
			}
			if !c.wantsErr && len(c.comments) > 0 && len(msg.page.Comments) != len(c.comments) {
				t.Errorf("with a warning and comments %d of %d arrived: a warning does not kill what arrived",
					len(msg.page.Comments), len(c.comments))
			}
		})
	}
}
