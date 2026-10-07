package tui

import (
	"fmt"
	"strings"
	"testing"
	"unicode/utf8"

	"prdash/internal/forge"
	"prdash/internal/forge/model"
)

func TestTheArrowsFollowTheListInBothDirections(t *testing.T) {
	items := make([]model.Item, 4)
	for i := range items {
		items[i] = mkItem("github", "github.com", "acme/widget", "One", i+1, "")
	}
	n := len(items)
	new := func() Model {
		return send(t, newTestModel(t, ghAdapter()),
			page(1, "github", "github.com", model.SectionReview, model.ReviewRequested, items, false))
	}

	m := new()
	if m.cursor != 0 {
		t.Fatalf("the initial cursor = %d, want 0", m.cursor)
	}
	for i := range items {
		m = press(t, m, "down")
		want := min(i+1, n-1)
		if m.cursor != want {
			t.Fatalf("after %d down steps the cursor = %d, want %d", i+1, m.cursor, want)
		}
	}
	m = press(t, m, "down")
	if m.cursor != n-1 {
		t.Errorf("down on the last gave %d, want %d: the list is not circular", m.cursor, n-1)
	}
	m = press(t, m, "j")
	if m.cursor != n-1 {
		t.Errorf("j on the last gave %d, want %d", m.cursor, n-1)
	}

	m = new()
	m.cursor = n - 1
	m = press(t, m, "up")
	if m.cursor != n-2 {
		t.Fatalf("up from the last gave %d, want %d", m.cursor, n-2)
	}
	m = press(t, m, "k")
	if m.cursor != n-3 {
		t.Errorf("k gave the cursor %d, want %d", m.cursor, n-3)
	}
	m.cursor = 1
	m = press(t, m, "up")
	if m.cursor != 0 {
		t.Errorf("up from the second row gave %d, want 0", m.cursor)
	}
	m = press(t, m, "up")
	if m.cursor != 0 {
		t.Errorf("up on the first gave %d, want 0: the list is not circular", m.cursor)
	}

	m = new()
	m.cursor = n - 1
	for want := n - 2; want >= 0; want-- {
		m = press(t, m, "up")
		if m.cursor != want {
			t.Fatalf("going back up: the cursor = %d, want %d", m.cursor, want)
		}
	}
}

func TestWithNoRowsNotEvenANoticeIsPainted(t *testing.T) {
	m := modelWithComments(t, 45, []model.Comment{commentOf("alice", "hola")}, 1)
	it := mustSelected(t, m)
	it.ID()
	denied := it
	deniedID := denied.ID()
	m.denied[deniedID] = "no permission"
	inner := m.contentWidth()

	for _, avail := range []int{-5, -1, 0, 1} {
		if got := m.commentLines(it, avail, inner); len(got) != 0 {
			t.Errorf("with avail=%d it returned %d lines, want 0: with no room nothing is painted, not even a notice",
				avail, len(got))
		}
	}
	if got := m.commentLines(it, commentChrome-1, inner); len(got) != 0 {
		t.Errorf("with avail=%d it returned %d lines, want 0: less than the frame and the box does not fit",
			commentChrome-1, len(got))
	}
}

// GitLab does not expose the count.
func TestTheCountNeverInventsMoreThanAreShown(t *testing.T) {
	cases := []struct {
		name  string
		total int
		want  int
	}{
		{"total greater than the list", 9, 9},
		{"total equal to the list", 3, 3},
		{"total smaller than the list", 1, 3},
		{"zero total with a list", 0, 3},
		{"total negative", -7, 3},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			list := []model.Comment{commentOf("alice", "one"), commentOf("bob", "two"), commentOf("carol", "three")}
			m := modelWithComments(t, 45, list, c.total)
			it := mustSelected(t, m)
			id := it.ID()
			st := m.comments[id]
			if st == nil {
				t.Fatal("there is no comments state")
			}
			if st.total != c.want {
				t.Errorf("the total stayed at %d, want %d: the count cannot be lower than what is shown", st.total, c.want)
			}
			// The border counts from the list, not from the inverted total.
			rows := m.commentLines(it, 30, m.contentWidth())
			if len(rows) == 0 {
				t.Fatal("the box was not painted")
			}
			legend := stripANSI(strings.Join(rows, "\n"))
			if !strings.Contains(legend, "3 of "+itoaSmall(c.want)) {
				t.Errorf("the border does not say 3 of %d: %q", c.want, legend)
			}
		})
	}
}

func itoaSmall(n int) string {
	if n == 0 {
		return "0"
	}
	isNeg := n < 0
	if isNeg {
		n = -n
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	if isNeg {
		return "-" + string(b)
	}
	return string(b)
}

func TestTheBoxTakesWhatItNeedsAndNoMore(t *testing.T) {
	one := []model.Comment{commentOf("alice", "one")}
	three := []model.Comment{commentOf("alice", "one"), commentOf("bob", "two"), commentOf("carol", "three")}

	m1 := modelWithComments(t, 45, one, 1)
	it1 := mustSelected(t, m1)
	if rows := m1.commentLines(it1, commentChrome+1, m1.contentWidth()); len(rows) != commentChrome+1 {
		t.Errorf("exact budget: the box takes %d rows, want %d", len(rows), commentChrome+1)
	}
	for _, avail := range []int{commentChrome + 2, commentChrome + 4, 12, 30} {
		rows := m1.commentLines(it1, avail, m1.contentWidth())
		if len(rows) > avail {
			t.Errorf("with avail=%d the box takes %d rows: it goes past the budget", avail, len(rows))
		}
		if want := commentChrome + 1; len(rows) != want {
			t.Errorf("with avail=%d the box takes %d rows, want %d: it does not pad the extra room", avail, len(rows), want)
		}
	}

	m3 := modelWithComments(t, 45, three, 3)
	it3 := mustSelected(t, m3)
	if rows := m3.commentLines(it3, commentChrome+3, m3.contentWidth()); len(rows) != commentChrome+3 {
		t.Errorf("exact budget for 3 comments: %d rows, want %d", len(rows), commentChrome+3)
	}
	if rows := m3.commentLines(it3, commentChrome+2, m3.contentWidth()); len(rows) != 0 {
		t.Errorf("with 3 comments and one row less the box was painted half (%d rows)", len(rows))
	}
	for _, avail := range []int{commentChrome + 3, commentChrome + 5, commentChrome + 9, 40} {
		if rows := m3.commentLines(it3, avail, m3.contentWidth()); len(rows) > avail {
			t.Errorf("with avail=%d and 3 comments the box takes %d: it goes past", avail, len(rows))
		}
	}
}

func TestTheBodySplitsAtTheBoxWidthNotAtTheTerminals(t *testing.T) {
	phrase := strings.Repeat("x", 400)
	for _, inner := range []int{20, 30, 40, 60, 80, 120, 200} {
		m := modelWithComments(t, 45, []model.Comment{commentOf("alice", phrase)}, 1)
		it := mustSelected(t, m)
		rows := m.commentLines(it, 30, inner)
		if len(rows) == 0 {
			t.Errorf("inner %d: the box was not painted", inner)
			continue
		}
		for i, l := range rows {
			if w := utf8.RuneCountInString(stripANSI(l)); w > inner {
				t.Errorf("inner %d: row %d measures %d columns, want <= %d: the body does not fit in its box",
					inner, i, w, inner)
			}
		}
	}
	body := strings.Repeat("palabra ", 60)
	rowsOf := func(inner int) int {
		m := modelWithComments(t, 45, []model.Comment{commentOf("alice", body)}, 1)
		it := mustSelected(t, m)
		return len(m.commentLines(it, 30, inner))
	}
	estrechas, wide := rowsOf(20), rowsOf(200)
	if estrechas <= wide {
		t.Errorf("with the narrow box %d rows come out and with the wide one %d: the body width is not ruling",
			estrechas, wide)
	}
	t.Logf("narrow %d rows, wide %d (cap per comment: %d)", estrechas, wide, maxCommentLines)
}

func TestTheLegendSaysHowManyAreShownOfHowManyThereAre(t *testing.T) {
	list := make([]model.Comment, 3)
	for i := range list {
		list[i] = commentOf("alice", "one")
	}
	m := modelWithComments(t, 45, list, 3)
	it := mustSelected(t, m)
	rows := m.commentLines(it, 30, m.contentWidth())
	if len(rows) == 0 {
		t.Fatal("the box was not painted")
	}
	plain := stripANSI(strings.Join(rows, "\n"))
	if !strings.Contains(plain, "3 of 3") {
		t.Errorf("with everything seen the border should say 3 of 3: %q", plain)
	}

	m2 := modelWithComments(t, 45, list, 12)
	it2 := mustSelected(t, m2)
	plain = stripANSI(strings.Join(m2.commentLines(it2, 30, m2.contentWidth()), "\n"))
	if !strings.Contains(plain, "3 of 12") {
		t.Errorf("with 12 in the total the border should say 3 of 12: %q", plain)
	}
	if !strings.Contains(plain, commentHint) {
		t.Errorf("with 12 in the total the hint that there are more should come out: %q", plain)
	}

	// With a narrow gap the hint is dropped but the count stays, because the number is the datum.
	m3 := modelWithComments(t, 45, list, 12)
	it3 := mustSelected(t, m3)
	plain = stripANSI(strings.Join(m3.commentLines(it3, 30, 12), "\n"))
	if !strings.Contains(plain, "3 of 12") {
		t.Errorf("with the narrow box the count cannot drop: %q", plain)
	}
	// The pure legend, unpainted, for the arithmetic that decides whether the hint fits.
	const (
		exactSpace = 45
		shortSpace = 44
	)
	legendWidth := len([]rune(fmt.Sprintf("%d of %d", 3, 12) + commentHint))
	for _, c := range []struct {
		shown, total, outer int
		wantHint            bool
	}{
		{3, 3, exactSpace, false},
		{3, 12, exactSpace, true},
		{3, 12, shortSpace, false},
		{3, 12, 60, true},
		{1, 1, exactSpace, false},
		{5, 5, exactSpace, false},
		{3, 12, 8, false},
	} {
		got := commentLegend(c.shown, c.total, c.outer)
		if !strings.Contains(stripANSI(got), itoaSmall(c.shown)+" of "+itoaSmall(c.total)) {
			t.Errorf("commentLegend(%d, %d, %d) = %q, it does not say the count",
				c.shown, c.total, c.outer, stripANSI(got))
		}
		if hasHint := strings.Contains(stripANSI(got), commentHint); hasHint != c.wantHint {
			t.Errorf("commentLegend(%d, %d, %d) %s the hint, want it to %s (the legend and the hint measure %d runes and the gap is %d)",
				c.shown, c.total, c.outer,
				map[bool]string{true: "trae", false: "no trae"}[hasHint],
				map[bool]string{true: "brings it", false: "does not bring it"}[c.wantHint],
				legendWidth,
				commentBoxWidth(c.outer)-commentBoxBorder-legendGap)
		}
	}
}

func TestTheCapOfFiveAppliesAndIsSaid(t *testing.T) {
	for _, n := range []int{1, 4, 5, 6, 20} {
		list := make([]model.Comment, n)
		for i := range list {
			list[i] = commentOf("alice", "one")
		}
		m := modelWithComments(t, 80, list, n)
		it := mustSelected(t, m)
		rows := m.commentLines(it, 60, m.contentWidth())
		if len(rows) == 0 {
			t.Errorf("with %d comments the box was not painted", n)
			continue
		}
		plain := stripANSI(strings.Join(rows, "\n"))
		want := n
		if want > forge.CommentLimit {
			want = forge.CommentLimit
		}
		if !strings.Contains(plain, itoaSmall(want)+" of "+itoaSmall(n)) {
			t.Errorf("with %d comments the border should say %d of %d, no more: %q", n, want, n, plain)
		}
		// And the ones not shown do not appear: it is a cap, not an arbitrary trim.
		for i := forge.CommentLimit; i < n; i++ {
			if strings.Contains(plain, "comment-"+itoaSmall(i)) {
				t.Errorf("with %d comments %d was shown, which is above the cap of %d", n, i, forge.CommentLimit)
			}
		}
	}
}
