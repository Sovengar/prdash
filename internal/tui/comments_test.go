package tui

import (
	"fmt"
	"regexp"
	"strings"
	"testing"
	"time"

	"prdash/internal/forge"
	"prdash/internal/forge/model"
	"prdash/internal/testutil"
)

func commentOf(author, body string) model.Comment {
	return model.Comment{Author: author, Body: body}
}

func modelWithComments(t *testing.T, height int, list []model.Comment, total int) Model {
	t.Helper()
	it := mkItem("github", "github.com", "acme/widget", "Add widget", 42, "")
	gh := ghAdapter()
	gh.Conversations = map[string]forge.CommentPage{
		testutil.ItemKey("acme/widget", 42): {Comments: list, Total: total},
	}
	m := newTestModel(t, gh)
	m.width, m.height = 160, height
	m = send(t, m, page(1, "github", "github.com", model.SectionAuthored, "", []model.Item{it}, false))
	m = showSection(m, model.SectionAuthored)
	return withConversation(t, m, it, forge.CommentPage{Comments: list, Total: total})
}

func withConversation(t *testing.T, m Model, it model.Item, p forge.CommentPage) Model {
	t.Helper()
	id := it.ID()
	if _, asked := m.comments[id]; !asked {
		m.comments[id] = &commentState{} // marked as asked, as the polling does
	}
	return send(t, m, commentsMsg{id: id, page: p})
}

func detailText(t *testing.T, m Model) string {
	t.Helper()
	rows := m.layout().detailLines
	if rows <= 0 {
		t.Fatal("the layout has not reserved rows for the detail: WindowSizeMsg is missing")
	}
	return stripANSI(strings.Join(m.detailLines(mustSelected(t, m), true, rows), "\n"))
}

func mustSelected(t *testing.T, m Model) model.Item {
	t.Helper()
	it, ok := m.selected()
	if !ok {
		t.Fatal("there is no selected item")
	}
	return it
}

func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("it did not happen in time: %s", what)
}

var detailLabels = []string{
	"Item:", "Forge:", "Author:", "Source:", "Target:",
	"State:", "Draft:", "Checks:", "Diff:", "Updated:", "Review:", "Role:",
}

func rowWithField(rows []string, label string) int {
	for i, l := range rows {
		if strings.Contains(stripANSI(l), label) {
			return i
		}
	}
	return -1
}

func hasCommentBox(text string) bool {
	return strings.Contains(text, " "+commentTitle+" ")
}

func rowWithCommentBox(rows []string) int {
	for i, l := range rows {
		if strings.Contains(stripANSI(l), " "+commentTitle+" ") {
			return i
		}
	}
	return -1
}

func TestURLGetsItsOwnFullWidthRow(t *testing.T) {
	const long = "https://umane.emeal.nttdata.com/git/APPCTTI/vsocial/backend/vsocial-api-accions/-/merge_requests/1234"
	it := mkItem("gitlab", "gitlab.example.com", "APPCTTI/vsocial/backend/vsocial-api-accions", "MR", 1234, "")
	it.Author = "someone-else"
	it.URL = long

	gh := ghAdapter()
	gl := &testutil.FakeAdapter{ForgeName: "gitlab", HostName: "gitlab.example.com"}
	m := newTestModel(t, gh, gl)
	m.width, m.height = 160, 45
	m = send(t, m, page(1, "gitlab", "gitlab.example.com", model.SectionReview, model.ReviewRequested, []model.Item{it}, false))
	m = withConversation(t, m, it, forge.CommentPage{
		Comments: []model.Comment{commentOf("alice", "ok for me")}, Total: 1,
	})

	rows := m.detailLines(it, true, m.layout().detailLines)
	urlRow := rowWithField(rows, "URL:")
	if urlRow < 0 {
		t.Fatalf("there is no URL row:\n%s", strings.Join(rows, "\n"))
	}

	plain := stripANSI(rows[urlRow])
	if !strings.Contains(plain, long) {
		t.Errorf("the URL should come out whole, not clipped:\n%s", plain)
	}
	for _, label := range detailLabels {
		if strings.Contains(plain, label) {
			t.Errorf("the URL row shares a row with %q:\n%s", label, plain)
		}
	}
	iComment := rowWithCommentBox(rows)
	if iComment < 0 {
		t.Fatalf("the comments should fit at this height:\n%s", strings.Join(rows, "\n"))
	}
	if urlRow > iComment {
		t.Errorf("the URL (row %d) should come before the comments (row %d)", urlRow, iComment)
	}
}

// It derives from the item and the login, so it would show on every render of every one of your PRs
// and repeat what the Role field already says.
func TestSelfDenyTakesNoRoomInTheDetail(t *testing.T) {
	m := newTestModel(t, ghAdapter())
	m.width, m.height = 160, 45
	it := mkItem("github", "github.com", "acme/widget", "Add widget", 42, "")
	it.Author = "Sovengar"
	m = send(t, m, authMsg{cycle: 1, forge: "github", auth: model.AuthState{Forge: "github", OK: true, Login: "Sovengar"}})
	m = send(t, m, page(1, "github", "github.com", model.SectionAuthored, "", []model.Item{it}, false))
	m = showSection(m, model.SectionAuthored)

	if m.selfDenied[it.ID()] == "" {
		t.Fatal("my own item should be vetoed")
	}
	detail := detailText(t, m)
	if strings.Contains(detail, "approve unavailable") {
		t.Errorf("the veto should not paint anything on the card:\n%s", detail)
	}
	if !strings.Contains(detail, "Role:") || !strings.Contains(detail, "own") {
		t.Errorf("the Role field should keep saying it is my own:\n%s", detail)
	}
	m.denied[it.ID()] = "the forge refused the action"
	if !strings.Contains(detailText(t, m), "action disabled") {
		t.Error("the forges denial notice should still be on the card")
	}
}

func TestCommentsLiveInTheirOwnTitledBox(t *testing.T) {
	m := modelWithComments(t, 45, []model.Comment{commentOf("alice", "ok for me")}, 1)
	rows := m.detailLines(mustSelected(t, m), true, m.layout().detailLines)

	top := rowWithCommentBox(rows)
	if top < 0 {
		t.Fatalf("there is no comments box:\n%s", strings.Join(rows, "\n"))
	}
	first := stripANSI(rows[top])
	for _, want := range []string{"╭", commentTitle, "╮"} {
		if !strings.Contains(first, want) {
			t.Errorf("the top border should carry %q:\n%s", want, first)
		}
	}
	bottom := -1
	for i := top + 1; i < len(rows); i++ {
		if strings.HasPrefix(strings.TrimSpace(stripANSI(rows[i])), "╰") {
			bottom = i
			break
		}
	}
	if bottom < 0 {
		t.Fatalf("the box does not close at the bottom:\n%s", strings.Join(rows, "\n"))
	}
	width := m.contentWidth()
	for i := top; i <= bottom; i++ {
		if n := len([]rune(stripANSI(rows[i]))); n != width {
			t.Errorf("line %d of the box measures %d, want %d (the panels width)", i, n, width)
		}
	}
	for i := top; i <= bottom; i++ {
		if strings.Contains(stripANSI(rows[i]), "Comments:") {
			t.Errorf("inside the box the label should not repeat:\n%s", stripANSI(rows[i]))
		}
	}
}

// Against the panel's border its verticals overlap and every row comes out as `||`; a lighter grey
// told them apart but goes yellowish on warm palettes.
func TestBoxIsInsetSoNestingReadsByShape(t *testing.T) {
	m := modelWithComments(t, 45, []model.Comment{commentOf("alice", "ok for me")}, 1)
	rows := m.detailLines(mustSelected(t, m), true, m.layout().detailLines)

	top := rowWithCommentBox(rows)
	if top < 0 {
		t.Fatalf("there is no comments box:\n%s", strings.Join(rows, "\n"))
	}
	for i := top; i < len(rows); i++ {
		plain := stripANSI(rows[i])
		r := []rune(plain)
		if strings.ContainsRune("│╭╰", r[0]) {
			t.Errorf("line %d: the box cannot sit on the panels left border:\n%q", i, plain)
		}
		if last := r[len(r)-1]; strings.ContainsRune("│╮╯", last) {
			t.Errorf("line %d: the box cannot sit on the panels right border:\n%q", i, plain)
		}
		if !strings.HasPrefix(plain, " │") && !strings.HasPrefix(plain, " ╭") && !strings.HasPrefix(plain, " ╰") {
			break // the nesting ends here: what follows is notices or filler
		}
	}
}

func TestCommentBoxSharesTheBorderColor(t *testing.T) {
	m := modelWithComments(t, 45, []model.Comment{commentOf("alice", "ok for me")}, 1)
	rows := strings.Split(m.detailSection(mustSelected(t, m), true, m.layout().detailLines).text, "\n")
	top := rowWithCommentBox(rows)
	if top < 0 {
		t.Fatalf("there is no comments box:\n%s", strings.Join(rows, "\n"))
	}
	panel, box := ansiColors(rows[0]), ansiColors(rows[top])
	if panel != box {
		t.Errorf("the comments box is painted with %q and the panel with %q, and they must be the same",
			box, panel)
	}
	if box == "" {
		t.Error("neither of the two lines carries color: the test would be checking nothing")
	}
}

func ansiColors(line string) string {
	seen := map[string]bool{}
	var out []string
	for _, m := range regexp.MustCompile(`\x1b\[[0-9;]*m`).FindAllString(line, -1) {
		if !seen[m] {
			seen[m] = true
			out = append(out, m)
		}
	}
	return strings.Join(out, " ")
}

// What broke the dash closing the bottom line is that the count's style ends with a reset, and a reset
// does not restore what was there: it takes the border's grey with it.
func TestEveryBorderGlyphIsPainted(t *testing.T) {
	m := modelWithComments(t, 45, []model.Comment{commentOf("alice", "ok for me")}, 3)
	rows := strings.Split(m.detailSection(mustSelected(t, m), true, m.layout().detailLines).text, "\n")
	for i, l := range rows {
		if bad := unpaintedBorderGlyphs(l); bad != "" {
			t.Errorf("line %d: these border glyphs come out unpainted, with the terminals default color: %q\n%q",
				i, bad, stripANSI(l))
		}
	}
}

// The glyphs are multibyte, so the line is split into escape runs and text runs; what is looked for is a
// glyph in a run that follows a reset and precedes another style.
func unpaintedBorderGlyphs(line string) string {
	const glyphs = "─│╭╮╰╯"
	seg := regexp.MustCompile(`\x1b\[[0-9;]*m|[^\x1b]+`)
	var out []rune
	styled, seen := false, false
	for _, s := range seg.FindAllString(line, -1) {
		if s[0] == '\x1b' {
			styled = s != "\x1b[m" && s != "\x1b[0m"
			seen = true
			continue
		}
		if seen && !styled {
			for _, r := range s {
				if strings.ContainsRune(glyphs, r) {
					out = append(out, r)
				}
			}
		}
	}
	return string(out)
}

func TestNoBoxWhenThereAreNoComments(t *testing.T) {
	m := modelWithComments(t, 45, nil, 0)
	detail := stripANSI(strings.Join(m.detailLines(mustSelected(t, m), true, m.layout().detailLines), "\n"))
	if hasCommentBox(detail) {
		t.Errorf("with no comments there should be no box:\n%s", detail)
	}
	if !strings.Contains(detail, "Comments:") || !strings.Contains(detail, "none") {
		t.Errorf("the field line should stay as it was:\n%s", detail)
	}
}

// One of the box's two rows is a border, and a clipped box does not look clipped: it looks like the
// PR only has that comment.
func TestBoxIsAllOrNothing(t *testing.T) {
	list := []model.Comment{commentOf("alice", "one"), commentOf("bob", "two"), commentOf("carol", "three")}
	for _, tc := range []struct {
		name    string
		avail   int
		wantBox bool
	}{
		{"the three plus the two borders fit", commentChrome + 3, true},
		{"one row short for a comment", commentChrome + 2, false},
		{"only the borders fit", commentChrome, false},
		{"not even a border fits", 1, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := modelWithComments(t, 45, list, 3)
			rows := m.commentLines(mustSelected(t, m), tc.avail, m.contentWidth())
			got := len(rows) > 0
			if got != tc.wantBox {
				t.Errorf("with avail=%d the box=%v, want %v", tc.avail, got, tc.wantBox)
			}
			if !got {
				return
			}
			if n := len(rows); n != tc.avail {
				t.Errorf("the box takes %d rows and the budget was %d", n, tc.avail)
			}
		})
	}
}

func TestBoxNeverOverflowsItsBudget(t *testing.T) {
	for _, height := range []int{24, 30, 40, 45, 60, 80} {
		for _, n := range []int{1, 3, 5} {
			list := make([]model.Comment, n)
			for i := range list {
				list[i] = commentOf("u", strings.Repeat("text long ", 20))
			}
			m := modelWithComments(t, height, list, n)
			rows := m.layout().detailLines
			it := mustSelected(t, m)
			for _, denied := range []bool{false, true} {
				if denied {
					m.denied[it.ID()] = "no permission"
				}
				lines := m.detailLines(it, true, rows)
				if len(lines) > rows {
					t.Fatalf("height %d with %d comments: the detail returned %d rows for a panel of %d (denied=%v)",
						height, n, len(lines), rows, denied)
				}
			}
		}
	}
}

func TestURLRowDoesNotCostHeight(t *testing.T) {
	m := modelWithComments(t, 45, []model.Comment{commentOf("alice", "ok for me")}, 1)
	rows := m.detailLines(mustSelected(t, m), true, m.layout().detailLines)

	urlRow := rowWithField(rows, "URL:")
	if urlRow < 0 {
		t.Fatalf("there is no URL row:\n%s", strings.Join(rows, "\n"))
	}
	if got := len(detailLabels); got != 12 {
		t.Fatalf("the test waits 12 grid labels, has %d", got)
	}
	seen := map[string]bool{}
	for _, l := range rows {
		plain := stripANSI(l)
		for _, label := range detailLabels {
			if strings.Contains(plain, label) {
				seen[label] = true
			}
		}
	}
	for _, label := range detailLabels {
		if !seen[label] {
			t.Errorf("the card lost the field %q when giving its row to the URL:\n%s",
				label, strings.Join(rows, "\n"))
		}
	}

	gridRows := 0
	for i, l := range rows {
		if i == urlRow {
			continue
		}
		plain := stripANSI(l)
		for _, label := range detailLabels {
			if strings.Contains(plain, label) {
				gridRows++
				break
			}
		}
	}
	if gridRows != 6 {
		t.Errorf("the grid takes %d rows, want 6 (12 fields in two columns):\n%s",
			gridRows, strings.Join(rows, "\n"))
	}
}

func TestLastGridRowCarriesTheActionFields(t *testing.T) {
	m := newTestModel(t, ghAdapter())
	m.width, m.height = 160, 45
	m = send(t, m, page(1, "github", "github.com", model.SectionAuthored, "", []model.Item{
		mkItem("github", "github.com", "acme/widget", "Add widget", 42, "CHANGES_REQUESTED"),
	}, false))
	m = showSection(m, model.SectionAuthored)
	it := mustSelected(t, m)

	joined := stripANSI(strings.Join(m.detailLines(it, true, 4), "\n"))
	for _, want := range []string{"Review:", "changes requested", "Role:"} {
		if !strings.Contains(joined, want) {
			t.Errorf("the clipping lost %q, which is what says whether the action applies:\n%s", want, joined)
		}
	}
}

func TestAllocateGivesTheLeftoverToWhoseNeedsIt(t *testing.T) {
	cases := []struct {
		name   string
		need   []int
		budget int
		want   []int
	}{
		{"caben enteros", []int{1, 4, 1, 1, 1}, 10, []int{1, 4, 1, 1, 1}},
		{"a spare row", []int{1, 4, 1, 1, 1}, 8, []int{1, 4, 1, 1, 1}},
		{"one row short", []int{1, 4, 1, 1, 1}, 7, []int{1, 3, 1, 1, 1}},
		{"all largos", []int{4, 4, 4, 4, 4}, 11, []int{3, 2, 2, 2, 2}},
		{"a very long one", []int{4, 1, 1, 1, 1}, 6, []int{2, 1, 1, 1, 1}},
		{"no budget", []int{1, 4}, 1, []int{1, 1}},
		{"a single one", []int{4}, 1, []int{1}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := allocate(tc.need, tc.budget)
			if len(got) != len(tc.want) {
				t.Fatalf("allocate returned %d quotas, want %d", len(got), len(tc.want))
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Errorf("cuota %d = %d, want %d (reparto completo: %v)", i, got[i], tc.want[i], got)
				}
			}
			sum := 0
			for i, n := range got {
				if n > tc.need[i] {
					t.Errorf("quota %d (%d) exceeds what it needs (%d)", i, n, tc.need[i])
				}
				if n < 1 {
					t.Errorf("quota %d is 0: the comment would disappear", i)
				}
				sum += n
			}
			if tc.budget >= len(tc.need) && sum > tc.budget {
				t.Errorf("the split adds up to %d and the budget is %d", sum, tc.budget)
			}
		})
	}
}

func TestAllocateIsDeterministic(t *testing.T) {
	first := allocate([]int{2, 2, 2}, 4)
	if first[0] != 2 || first[1] != 1 || first[2] != 1 {
		t.Errorf("with a leftover row the first one always takes it: %v", first)
	}
	for range 50 {
		got := allocate([]int{2, 2, 2}, 4)
		for i := range got {
			if got[i] != first[i] {
				t.Fatalf("allocate is not deterministic: %v != %v", got, first)
			}
		}
	}
}

func TestLongCommentKeepsItsRowsWhenOthersAreShort(t *testing.T) {
	m := modelWithComments(t, 45, []model.Comment{
		commentOf("alice", "one liner"),
		commentOf("bob", "the timeout is 30x too high\n\nI would put it at 2s like the rest of the endpoints, "+
			"otherwise the pool exhausts and the requests die in the queue without ever logging"),
		commentOf("carol", "one liner"),
		commentOf("dave", "one liner"),
		commentOf("erin", "one liner"),
	}, 5)
	m = send(t, m, authMsg{cycle: 1, forge: "github", auth: model.AuthState{Forge: "github", OK: true, Login: "reviewer"}})
	m = send(t, m, page(1, "github", "github.com", model.SectionAuthored, "", []model.Item{
		mkItem("github", "github.com", "acme/widget", "Add widget", 42, ""),
	}, false))
	m = withConversation(t, m, mustSelected(t, m), forge.CommentPage{
		Comments: []model.Comment{
			commentOf("alice", "one liner"),
			commentOf("bob", "the timeout is 30x too high\n\nI would put it at 2s like the rest of the endpoints, "+
				"otherwise the pool exhausts and the requests die in the queue without ever logging"),
			commentOf("carol", "one liner"),
			commentOf("dave", "one liner"),
			commentOf("erin", "one liner"),
		},
		Total: 5,
	})

	detail := detailText(t, m)
	if !strings.Contains(detail, "I would put it at 2s") {
		t.Errorf("the long comment should take more than its first line:\n%s", detail)
	}
	for _, want := range []string{"alice:", "carol:", "dave:", "erin:"} {
		if !strings.Contains(detail, want) {
			t.Errorf("the rows split should not sacrifice %q:\n%s", want, detail)
		}
	}
}

func TestCommentsShowBelowTheFields(t *testing.T) {
	m := modelWithComments(t, 45, []model.Comment{
		commentOf("alice", "please add a test for the retry path"),
		commentOf("bob", "the timeout is 30x too high"),
	}, 2)

	detail := detailText(t, m)
	iField := strings.Index(detail, "State:")
	iComment := strings.Index(detail, " "+commentTitle+" ")
	if iField < 0 || iComment < 0 {
		t.Fatalf("blocks are missing in the detail:\n%s", detail)
	}
	if iComment < iField {
		t.Errorf("the comments should come after the card:\n%s", detail)
	}
	for _, want := range []string{"alice:", "please add a test for the retry path", "bob:", "the timeout is 30x too high"} {
		if !strings.Contains(detail, want) {
			t.Errorf("%q is missing in the detail:\n%s", want, detail)
		}
	}
}

func TestFieldsStayInTwoColumns(t *testing.T) {
	m := modelWithComments(t, 60, []model.Comment{commentOf("alice", "one")}, 1)
	lines := m.detailLines(mustSelected(t, m), true, m.layout().detailLines)
	joined := stripANSI(strings.Join(lines, "\n"))

	if !strings.Contains(joined, "Item:") {
		t.Fatalf("the grid row is not there:\n%s", joined)
	}
	row := ""
	for _, l := range strings.Split(joined, "\n") {
		if strings.Contains(l, "Item:") {
			row = l
			break
		}
	}
	if !strings.Contains(row, "Forge:") {
		t.Errorf("Item and Forge should go on the same grid row, not stacked:\n%s", row)
	}
	if fields := strings.Count(joined, ":"); fields < 13 {
		t.Errorf("fields were lost in the grid (%d labels):\n%s", fields, joined)
	}
	if rows := strings.Count(joined, "Item:"); rows != 1 {
		t.Errorf("Item should come out once, not %d", rows)
	}
}

func TestCommentsCapAtFive(t *testing.T) {
	var many []model.Comment
	for i := 0; i < 9; i++ {
		many = append(many, commentOf(fmt.Sprintf("u%d", i), fmt.Sprintf("body %d", i)))
	}
	m := modelWithComments(t, 60, many, 9)
	detail := detailText(t, m)
	for i := 0; i < 9; i++ {
		body := fmt.Sprintf("body %d", i)
		want := i < forge.CommentLimit
		if got := strings.Contains(detail, body); got != want {
			t.Errorf("%q present=%v, want %v (only the first %d)", body, got, want, forge.CommentLimit)
		}
	}
}

func TestCommentsAnnounceThereAreMore(t *testing.T) {
	list := []model.Comment{
		commentOf("alice", "one"), commentOf("bob", "two"), commentOf("carol", "three"),
		commentOf("dave", "four"), commentOf("erin", "five"),
	}
	// A known viewer means the item is not the user's and there are no action warnings, which is what
	// actually leaves the panel room: the own-approval veto no longer takes rows.
	m := modelWithComments(t, 45, list, 23)
	m = send(t, m, authMsg{cycle: 1, forge: "github", auth: model.AuthState{Forge: "github", OK: true, Login: "reviewer"}})
	it := mustSelected(t, m)
	m = send(t, m, page(1, "github", "github.com", model.SectionAuthored, "", []model.Item{it}, false))
	m = withConversation(t, m, it, forge.CommentPage{Comments: list, Total: 23})

	detail := detailText(t, m)
	if !strings.Contains(detail, "5 of 23") {
		t.Errorf("with 23 comments the card should say which ones it shows:\n%s", detail)
	}
}

// Free on the border, so it is not the first thing lost when the panel is tight.
func TestCountLivesInTheBorder(t *testing.T) {
	list := []model.Comment{
		commentOf("alice", "one"), commentOf("bob", "two"), commentOf("carol", "three"),
		commentOf("dave", "four"), commentOf("erin", "five"),
	}
	m := modelWithComments(t, 45, list, 23)
	it := mustSelected(t, m)
	// The forge's warning tightens the panel (a blank line plus text) to check that the count no longer
	// depends on the budget. The own-approval veto no longer takes rows, so this is the only way to squeeze.
	m.denied[it.ID()] = "the forge refused the action"
	m = withConversation(t, m, it, forge.CommentPage{Comments: list, Total: 23})

	detail := detailText(t, m)
	if !hasCommentBox(detail) {
		t.Fatalf("the box should come out:\n%s", detail)
	}
	rows := strings.Split(detail, "\n")
	bottom := -1
	for i, l := range rows {
		if strings.HasPrefix(strings.TrimSpace(l), "╰") {
			bottom = i
			break
		}
	}
	if bottom < 0 {
		t.Fatalf("the box does not close at the bottom:\n%s", detail)
	}
	row := rows[bottom]
	if !strings.HasSuffix(strings.TrimSpace(row), "╯") {
		t.Fatalf("the count should be on the boxes bottom border:\n%s", detail)
	}
	if !strings.HasSuffix(strings.TrimRight(row, " ╯"), "─") {
		t.Errorf("the border should follow after the count, not leave it loose:\n%s", row)
	}
	if !strings.Contains(row, "5 of 23") {
		t.Errorf("the bottom border should carry the count, not the body:\n%s", row)
	}
	for _, want := range []string{"alice:", "bob:", "carol:", "dave:", "erin:"} {
		if !strings.Contains(detail, want) {
			t.Errorf("%q should not drop:\n%s", want, detail)
		}
	}
}

func TestCountAlwaysSaysHowManyOfHowMany(t *testing.T) {
	m := modelWithComments(t, 45, []model.Comment{commentOf("alice", "one"), commentOf("bob", "two")}, 2)
	detail := detailText(t, m)
	if !strings.Contains(detail, "2 of 2") {
		t.Errorf("with everything in view the count also says the total:\n%s", detail)
	}
	if strings.Contains(detail, commentHint) {
		t.Errorf("with no comments outside there is nothing to open:\n%s", detail)
	}
	if !hasCommentBox(detail) {
		t.Errorf("the comments box should come out:\n%s", detail)
	}
}

func TestCommentHeadlineSkipsBotBoilerplate(t *testing.T) {
	m := modelWithComments(t, 45, []model.Comment{
		commentOf("ssf-bot", "<!-- ssf: origin=acme/widget#553 -->\n\nattached the agent to the session"),
	}, 1)
	detail := detailText(t, m)
	if !strings.Contains(detail, "attached the agent to the session") {
		t.Errorf("the real comment text was not painted:\n%s", detail)
	}
	if strings.Contains(detail, "ssf: origin") {
		t.Errorf("the bots boilerplate was painted:\n%s", detail)
	}
}

func TestCommentBodySplitsOverAvailableRows(t *testing.T) {
	body := "the retry has no backoff and that is why the service goes down when " +
		"the dependency is slow, and the timeout is thirty times what it " +
		"should be, so the pool is exhausted and the requests die queued " +
		"before logging anything"
	m := modelWithComments(t, 60, []model.Comment{commentOf("alice", body)}, 1)
	detail := detailText(t, m)

	if !strings.Contains(detail, "the retry has no backoff") {
		t.Fatalf("the comment was not painted:\n%s", detail)
	}
	if strings.Count(detail, "alice:") != 1 {
		t.Errorf("the author should come out only once:\n%s", detail)
	}
	rows := 0
	for _, l := range strings.Split(detail, "\n") {
		if strings.Contains(l, "backoff") || strings.Contains(l, "dependencia") ||
			strings.Contains(l, "thirty times") || strings.Contains(l, "exhausted") {
			rows++
		}
	}
	if rows < 2 {
		t.Errorf("with a terminal height the body should take several rows, it took %d:\n%s", rows, detail)
	}
	for _, l := range strings.Split(detail, "\n") {
		if n := len([]rune(l)); n > m.contentWidth() {
			t.Errorf("row of %d runes, more than the usable width %d:\n%q", n, m.contentWidth(), l)
		}
	}
}

// A tall terminal is needed for the block to fit: at 24 rows the grid already takes the panel.
func TestCommentCutIsMarked(t *testing.T) {
	long := strings.Repeat("palabra ", 400)
	m := modelWithComments(t, 45, []model.Comment{commentOf("alice", long)}, 1)
	detail := detailText(t, m)
	if !hasCommentBox(detail) {
		t.Fatalf("at 45 rows the comments should fit:\n%s", detail)
	}
	if !strings.Contains(detail, "…") {
		t.Errorf("a comment that does not fit should end in …:\n%s", detail)
	}
	for _, l := range strings.Split(detail, "\n") {
		if n := len([]rune(l)); n > m.contentWidth() {
			t.Errorf("row of %d runes, more than the usable width %d:\n%q", n, m.contentWidth(), l)
		}
	}
}

func TestCommentsAreFirstToGo(t *testing.T) {
	list := []model.Comment{commentOf("alice", "one"), commentOf("bob", "two"), commentOf("carol", "three")}
	gh := ghAdapter()
	gh.Conversations = map[string]forge.CommentPage{testutil.ItemKey("acme/widget", 42): {Comments: list, Total: 3}}
	m := newTestModel(t, gh)
	m.width, m.height = 160, 24
	it := mkItem("github", "github.com", "acme/widget", "Add widget", 42, "")
	m = send(t, m, page(1, "github", "github.com", model.SectionAuthored, "", []model.Item{it}, false))
	m = withConversation(t, m, it, forge.CommentPage{Comments: list, Total: 3})

	detail := stripANSI(strings.Join(m.detailLines(it, true, 9), "\n"))
	if strings.Contains(detail, "alice:") {
		t.Errorf("at 9 rows the comments should drop:\n%s", detail)
	}
	for _, want := range []string{"Add widget", "Item:", "State:", "Role:"} {
		if !strings.Contains(detail, want) {
			t.Errorf("the card should not lose %q when the comments drop:\n%s", want, detail)
		}
	}
}

func TestDetailAlwaysFitsThePanel(t *testing.T) {
	for _, height := range []int{20, 24, 30, 40, 45, 60, 80} {
		t.Run(fmt.Sprintf("height %d", height), func(t *testing.T) {
			var list []model.Comment
			for i := 0; i < 12; i++ {
				list = append(list, commentOf(fmt.Sprintf("u%d", i), strings.Repeat("text long ", 30)))
			}
			m := modelWithComments(t, height, list, 12)
			rows := m.layout().detailLines
			it := mustSelected(t, m)
			for _, denied := range []bool{false, true} {
				if denied {
					m.denied[it.ID()] = "no permission"
				}
				lines := m.detailLines(it, true, rows)
				if len(lines) > rows {
					t.Fatalf("the detail returned %d rows for a panel of %d (denied=%v):\n%s",
						len(lines), rows, denied, strings.Join(lines, "\n"))
				}
			}
		})
	}
}

func TestCommentsStates(t *testing.T) {
	it := mkItem("github", "github.com", "acme/widget", "Add widget", 42, "")

	cases := []struct {
		name string
		set  func(m *Model)
		want string
		not  string
	}{
		{
			name: "en vuelo",
			set: func(m *Model) {
				m.comments[it.ID()] = &commentState{}
			},
			want: "loading",
		},
		{
			name: "without asking",
			set:  func(m *Model) {},
			not:  "loading",
		},
		{
			name: "without comments",
			set: func(m *Model) {
				m.comments[it.ID()] = &commentState{ready: true}
			},
			want: "none", not: "loading",
		},
		{
			name: "read failure",
			set: func(m *Model) {
				m.comments[it.ID()] = &commentState{ready: true, err: "rate limited"}
			},
			want: "rate limited", not: "loading",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := newTestModel(t, ghAdapter())
			m.width, m.height = 160, 45
			m = send(t, m, page(1, "github", "github.com", model.SectionAuthored, "", []model.Item{it}, false))
			tc.set(&m)
			detail := stripANSI(strings.Join(m.detailLines(it, true, m.layout().detailLines), "\n"))
			if !strings.Contains(detail, tc.want) {
				t.Errorf("the panel should say %q:\n%s", tc.want, detail)
			}
			if tc.not != "" && strings.Contains(detail, tc.not) {
				t.Errorf("the panel should not say %q:\n%s", tc.not, detail)
			}
		})
	}
}

func TestCommentFailIsNotCachedAsEmpty(t *testing.T) {
	it := mkItem("github", "github.com", "acme/widget", "Add widget", 42, "")
	gh := ghAdapter()
	gh.CommentWarnings = map[string][]model.Warning{
		testutil.ItemKey("acme/widget", 42): {{Kind: "ratelimit", Msg: "API rate limit exceeded"}},
	}
	m := newTestModel(t, gh)
	m.width, m.height = 160, 45
	m = send(t, m, page(1, "github", "github.com", model.SectionAuthored, "", []model.Item{it}, false))
	m = showSection(m, model.SectionAuthored)
	m = send(t, m, commentsMsg{id: it.ID(), err: "API rate limit exceeded"})

	st := m.comments[it.ID()]
	if st == nil || !st.ready || st.err == "" {
		t.Fatalf("the failure should be recorded as an error: %+v", st)
	}
	if strings.Contains(detailText(t, m), "none") {
		t.Errorf("a failure cannot be painted as \"there are no comments\":\n%s", detailText(t, m))
	}
}

func TestCommentsFetchedOnceForTheSelectedItem(t *testing.T) {
	m := newTestModel(t, ghAdapter())
	m.width, m.height = 160, 45
	m = send(t, m, page(1, "github", "github.com", model.SectionAuthored, "", []model.Item{
		mkItem("github", "github.com", "acme/widget", "One", 1, ""),
		mkItem("github", "github.com", "acme/widget", "Two", 2, ""),
	}, false))
	m = showSection(m, model.SectionAuthored)
	gh := m.byForge["github"].(*testutil.FakeAdapter)

	// The first call is awaited before counting, or the test would be measuring the scheduler.
	for range 3 {
		m.requestComments()
	}
	waitFor(t, "the first comments query", func() bool { return gh.CommentCallCount() >= 1 })
	if got := gh.CommentCallCount(); got != 1 {
		t.Fatalf("Comments called %d times, want 1 (the ticks cannot duplicate the query)", got)
	}

	m = send(t, m, commentsMsg{id: mustSelected(t, m).ID(), page: forge.CommentPage{
		Comments: []model.Comment{commentOf("alice", "hi")}, Total: 1,
	}})
	m.requestComments()
	if got := gh.CommentCallCount(); got != 1 {
		t.Errorf("Comments called %d times after caching, want 1", got)
	}
}

func TestCommentsNotAskedWithoutSession(t *testing.T) {
	m := newTestModel(t, ghAdapter())
	m.width, m.height = 160, 45
	m = send(t, m, page(1, "github", "github.com", model.SectionAuthored, "", []model.Item{
		mkItem("github", "github.com", "acme/widget", "One", 1, ""),
	}, false))
	m = showSection(m, model.SectionAuthored)
	m.statuses["github"].auth = model.AuthState{Forge: "github", OK: false, Reason: "bad token"}
	gh := m.byForge["github"].(*testutil.FakeAdapter)

	m.requestComments()
	if got := gh.CommentCallCount(); got != 0 {
		t.Errorf("Comments called %d times with no session, want 0", got)
	}
	if _, marked := m.comments[mustSelected(t, m).ID()]; marked {
		t.Error("a query that is not launched should not be marked as asked")
	}
	if detail := detailText(t, m); strings.Contains(detail, "loading") {
		t.Errorf("with no query in flight it should not say loading:\n%s", detail)
	}
}

func TestCommentsInvalidatedByAction(t *testing.T) {
	it := mkItem("github", "github.com", "acme/widget", "Add widget", 42, "")
	gh := ghAdapter()
	m := newTestModel(t, gh)
	m.width, m.height = 160, 45
	m = send(t, m, page(1, "github", "github.com", model.SectionAuthored, "", []model.Item{it}, false))
	m = showSection(m, model.SectionAuthored)
	m = send(t, m, commentsMsg{id: it.ID(), page: forge.CommentPage{
		Comments: []model.Comment{commentOf("alice", "hi")}, Total: 1,
	}})
	if _, ok := m.comments[it.ID()]; !ok {
		t.Fatal("the conversation should be cached before the action")
	}

	m.applyAction(forge.Outcome{Kind: forge.ActionApprove, ID: it.ID(), OK: true, Item: it, HasItem: true}, m.cycle)
	if _, ok := m.comments[it.ID()]; ok {
		t.Error("an action should invalidate the items cached conversation")
	}
	_ = m.requestComments()
	waitFor(t, "the query after invalidating", func() bool { return gh.CommentCallCount() >= 1 })
	if got := gh.CommentCallCount(); got != 1 {
		t.Errorf("Comments called %d times after invalidating, want 1", got)
	}
}

func TestCommentsSurviveRefresh(t *testing.T) {
	it := mkItem("github", "github.com", "acme/widget", "Add widget", 42, "")
	m := newTestModel(t, ghAdapter())
	m.width, m.height = 160, 45
	m = send(t, m, page(1, "github", "github.com", model.SectionAuthored, "", []model.Item{it}, false))
	m = send(t, m, commentsMsg{id: it.ID(), page: forge.CommentPage{
		Comments: []model.Comment{commentOf("alice", "hi")}, Total: 1,
	}})

	m.cycle = 2
	m = send(t, m, page(2, "github", "github.com", model.SectionAuthored, "", []model.Item{it}, false))
	if _, ok := m.comments[it.ID()]; !ok {
		t.Error("the inbox refresh should not kill the cached conversation")
	}
}

func TestCommentTotalNeverBelowShown(t *testing.T) {
	it := mkItem("github", "github.com", "acme/widget", "Add widget", 42, "")
	m := newTestModel(t, ghAdapter())
	m.width, m.height = 160, 45
	m = send(t, m, page(1, "github", "github.com", model.SectionAuthored, "", []model.Item{it}, false))
	m = showSection(m, model.SectionAuthored)
	m = send(t, m, commentsMsg{id: it.ID(), page: forge.CommentPage{
		Comments: []model.Comment{commentOf("a", "1"), commentOf("b", "2"), commentOf("c", "3")}, Total: 1,
	}})

	detail := detailText(t, m)
	if strings.Contains(detail, "of 1") || strings.Contains(detail, "3 of 1") {
		t.Errorf("a total below what is shown must not be painted:\n%s", detail)
	}
	if st := m.comments[it.ID()]; st.total < 3 {
		t.Errorf("total = %d, want >= 3 (what is shown)", st.total)
	}
}
