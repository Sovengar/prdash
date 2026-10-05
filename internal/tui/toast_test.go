package tui

import (
	"strings"
	"testing"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"prdash/internal/forge/model"
	"prdash/internal/state"
)

func toastTestModel(t *testing.T) (Model, *time.Time) {
	t.Helper()
	m := newTestModel(t, ghAdapter())
	now := time.Date(2026, 9, 26, 10, 0, 0, 0, time.UTC)
	m.toast.now = func() time.Time { return now }
	return m, &now
}

func TestToastAppearsAndExpires(t *testing.T) {
	m, now := toastTestModel(t)
	m = press(t, m, "a") // with no selection: it launches a notice

	if len(toastTexts(m)) == 0 {
		t.Fatal("there should be a notice after the keypress")
	}
	m = send(t, m, toastTickMsg{})
	if len(toastTexts(m)) != 1 {
		t.Fatalf("the notice should not expire early: %q", toastTexts(m))
	}
	*now = now.Add(toastDuration + time.Second)
	m = send(t, m, toastTickMsg{})
	if got := toastTexts(m); len(got) != 0 {
		t.Fatalf("the notice should have expired: %q", got)
	}
}

func TestToastStacksInOrder(t *testing.T) {
	m, _ := toastTestModel(t)
	m.toast.show("first", toastInfo)
	m.toast.show("second", toastError)
	if got := m.toast.last(); got != "second" {
		t.Errorf("last() = %q, want %q", got, "second")
	}
	if len(m.toast.texts()) != 2 {
		t.Errorf("texts() = %q, want 2 avisos", m.toast.texts())
	}
}

func TestToastIgnoresEmptyMessage(t *testing.T) {
	m, _ := toastTestModel(t)
	m.toast.show("", toastInfo)
	if got := m.toast.texts(); len(got) != 0 {
		t.Errorf("an empty notice should not stack: %q", got)
	}
}

func TestToastOverlaysView(t *testing.T) {
	m, _ := toastTestModel(t)
	item := mkItem("github", "github.com", "acme/widget", "Add widget", 1, "")
	item.Author = "someone"
	m = send(t, m, page(1, "github", "github.com", model.SectionReview, model.ReviewRequested, []model.Item{item}, false))

	view := stripANSI(m.View().Content)
	if !strings.Contains(view, "Add widget") {
		t.Fatalf("the base view should still be there:\n%s", view)
	}

	m.toast.show("you cannot approve your own PR/MR", toastWarning)
	overlaid := stripANSI(m.View().Content)
	if !strings.Contains(overlaid, "you cannot approve your own PR/MR") {
		t.Fatalf("the notice should be overlaid:\n%s", overlaid)
	}
	if !strings.Contains(overlaid, "Add widget") {
		t.Fatalf("the overlay must not erase the background view:\n%s", overlaid)
	}
}

// The delicate invariant: compositing the overlay over lines must not widen them, or the
// whole table misaligns.
func TestToastDoesNotBreakColumnWidths(t *testing.T) {
	m, _ := toastTestModel(t)
	item := mkItem("github", "github.com", "acme/widget", "Add widget", 1, "")
	item.Author = "someone"
	m = send(t, m, page(1, "github", "github.com", model.SectionReview, model.ReviewRequested, []model.Item{item}, false))
	before := strings.Split(m.View().Content, "\n")

	m.toast.show("you cannot approve your own PR/MR", toastWarning)
	after := strings.Split(m.View().Content, "\n")
	if len(after) != len(before) {
		t.Fatalf("the overlay added lines: %d -> %d", len(before), len(after))
	}
	for i, l := range after {
		if w := ansi.StringWidth(l); w > m.outerWidth() {
			t.Fatalf("line %d measures %d columns, exceeds %d:\n%q", i, w, m.outerWidth(), stripANSI(l))
		}
	}
	if stripANSI(after[0]) != stripANSI(before[0]) {
		t.Errorf("the overlay moved the header:\n%q\n%q", stripANSI(before[0]), stripANSI(after[0]))
	}
}

func frameOf(view string) []string {
	out := make([]string, 0)
	for _, l := range strings.Split(stripANSI(view), "\n") {
		if l == "" {
			out = append(out, "")
			continue
		}
		out = append(out, string([]rune(l)[0])+string([]rune(l)[len([]rune(l))-1]))
	}
	return out
}

func TestToastDoesNotStepOnBorders(t *testing.T) {
	m, _ := toastTestModel(t)
	item := mkItem("github", "github.com", "acme/widget", "Add widget", 1, "")
	item.Author = "someone"
	m = send(t, m, page(1, "github", "github.com", model.SectionReview, model.ReviewRequested, []model.Item{item}, false))

	before := frameOf(m.View().Content)
	m.toast.show("you cannot approve your own PR/MR", toastWarning)
	after := frameOf(m.View().Content)

	if len(after) != len(before) {
		t.Fatalf("the overlay changed the number of lines: %d -> %d", len(before), len(after))
	}
	for i := range before {
		if after[i] != before[i] {
			t.Errorf("line %d: the frame went from %q to %q; the notice stepped on a border:\n%s",
				i, before[i], after[i], stripANSI(m.View().Content))
		}
	}
}

// The hints box does not give up its interior: it is the help the user has to read.
func TestToastDoesNotCoverTheHelp(t *testing.T) {
	m, _ := toastTestModel(t)
	m = send(t, m, page(1, "github", "github.com", model.SectionReview, model.ReviewRequested, []model.Item{
		mkItem("github", "github.com", "acme/widget", "Add widget", 1, ""),
	}, false))

	m.toast.show("merged !42 into main", toastSuccess)
	view := stripANSI(m.View().Content)
	for _, l := range strings.Split(view, "\n") {
		if !strings.Contains(l, "q quit") {
			continue
		}
		if !strings.Contains(l, "j/k move") {
			t.Errorf("the keybind line was covered by a notice: %q", l)
		}
	}
	if !strings.Contains(view, "j/k move") {
		t.Errorf("the keybinds disappeared with the notice:\n%s", view)
	}
}

func TestToastStacksSeveralNotices(t *testing.T) {
	m, _ := toastTestModel(t)
	m = send(t, m, page(1, "github", "github.com", model.SectionReview, model.ReviewRequested, []model.Item{
		mkItem("github", "github.com", "acme/widget", "Add widget", 1, ""),
	}, false))
	before := frameOf(m.View().Content)

	m.toast.show("first notice", toastInfo)
	m.toast.show("second notice", toastError)
	m.toast.show("third notice", toastSuccess)
	view := stripANSI(m.View().Content)

	for _, want := range []string{"first notice", "second notice", "third notice"} {
		if !strings.Contains(view, want) {
			t.Errorf("the notice %q is missing:\n%s", want, view)
		}
	}
	after := frameOf(m.View().Content)
	for i := range before {
		if after[i] != before[i] {
			t.Errorf("line %d: the frame went from %q to %q with 3 live notices", i, before[i], after[i])
		}
	}
}

func TestViewRowsMatchTheLines(t *testing.T) {
	m, _ := toastTestModel(t)
	v := m.compose(m.layout(), m.listSection(m.layout()), m.detailSection(model.Item{}, false, m.layout().detailLines))
	if got, want := len(v.rows), len(strings.Split(v.text, "\n")); got != want {
		t.Errorf("rows = %d, want %d (one per line)", got, want)
	}
	for i, ok := range v.rows {
		l := stripANSI(strings.Split(v.text, "\n")[i])
		border := strings.HasPrefix(l, "╭") || strings.HasPrefix(l, "╰")
		if border && ok {
			t.Errorf("line %d is a border and should not admit a notice: %q", i, l)
		}
	}
}

func TestLandRowLooksForTheLowestGap(t *testing.T) {
	rows := []bool{false, false, false, true, true, true}
	if base, ok := landRow(rows, 5, 3); !ok || base != 5 {
		t.Errorf("landRow = (%d, %v), want (5, true): the lowest gap is 3..5", base, ok)
	}
	if _, ok := landRow(rows, 5, 4); ok {
		t.Error("landRow found room for 4 rows in 3 free ones")
	}
	above := []bool{true, true, true, false, false, false}
	if base, ok := landRow(above, 2, 3); !ok || base != 2 {
		t.Errorf("landRow(above, 2, 3) = (%d, %v), want (2, true): the anchor bounds the search", base, ok)
	}
	if _, ok := landRow(above, 1, 3); ok {
		t.Error("landRow went above the anchor")
	}
	if _, ok := landRow([]bool{false, false, false}, 2, 2); ok {
		t.Error("landRow painted over rows that take no notice")
	}
	if _, ok := landRow([]bool{true, true, true}, -5, 3); ok {
		t.Error("landRow accepted a negative anchor")
	}
}

func TestToastWrapsAndStaysBounded(t *testing.T) {
	m, _ := toastTestModel(t)
	long := strings.Repeat("very long toast message ", 12)
	m.toast.show(long, toastError)
	blocks := m.toast.blocks(80)
	if len(blocks) != 1 {
		t.Fatalf("a notice is a box, want 1: %d", len(blocks))
	}
	boxLines := strings.Split(blocks[0], "\n")
	if len(boxLines) < 2 {
		t.Fatalf("a long message should take several lines: %d", len(boxLines))
	}
	for _, l := range boxLines {
		if w := ansi.StringWidth(l); w > toastMaxWidth {
			t.Errorf("box line with %d columns, exceeds %d: %q", w, toastMaxWidth, l)
		}
	}
	if w := ansi.StringWidth(strings.Split(m.toast.blocks(30)[0], "\n")[0]); w > 30 {
		t.Errorf("the box did not respect the available room: %d columns", w)
	}
}

func TestSetNoticeMapsLevels(t *testing.T) {
	m, _ := toastTestModel(t)
	m.setNotice("hola", levelNone)
	if got := m.toast.texts(); len(got) != 0 {
		t.Errorf("levelNone should not launch a notice: %q", got)
	}
	for _, lvl := range []noticeLevel{levelInfo, levelOK, levelWarn, levelError} {
		m.toast.toasts = nil
		m.setNotice("message", lvl)
		if len(m.toast.texts()) != 1 {
			t.Errorf("level %v did not launch a notice", lvl)
		}
	}
}

// The tick comes from the clock, not the channel, so it cannot re-arm a reader.
func TestToastTickDoesNotTouchTheEventChannel(t *testing.T) {
	m, _ := toastTestModel(t)
	before := m.readers
	out, cmd := m.Update(toastTickMsg{})
	after := out.(Model)
	if after.readers != before {
		t.Errorf("the toast tick touched the readers: %d -> %d", before, after.readers)
	}
	if cmd == nil {
		t.Error("the tick should rearm")
	}
}

func TestToastInViewOfDetail(t *testing.T) {
	m, _ := toastTestModel(t)
	item := mkItem("github", "github.com", "acme/widget", "Add widget", 1, "")
	m = send(t, m, page(1, "github", "github.com", model.SectionAuthored, "", []model.Item{item}, false))
	m.toast.show(state.SelfReviewReason, toastWarning)
	if !strings.Contains(stripANSI(m.View().Content), "you cannot approve your own") {
		t.Fatal("the notice should be visible over the detail panel")
	}
}

func TestToastBorderMatchesLevel(t *testing.T) {
	m, _ := toastTestModel(t)
	want := map[toastLevel]string{
		toastSuccess: "✓", toastError: "✗", toastInfo: "ℹ", toastWarning: "⚠",
	}
	for lvl, icon := range want {
		if got := toastIcon(lvl); got != icon {
			t.Errorf("toastIcon(%d) = %q, want %q", lvl, got, icon)
		}
		block := m.toast.render(toast{message: "x", level: lvl, created: time.Now(), duration: time.Second}, 80)
		if !strings.Contains(block, icon) {
			t.Errorf("the box of level %d does not carry its icon: %q", lvl, block)
		}
		if !strings.Contains(block, lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).Render("x")) &&
			!strings.Contains(block, "│") {
			t.Errorf("the box of level %d does not look like a box: %q", lvl, block)
		}
	}
}

// replace must find its target even when it is not the newest.
func TestToastReplaceFindsAnOlderToast(t *testing.T) {
	m, now := toastTestModel(t)
	m.toast.show("merge ok", toastSuccess)
	m.toast.show("an action is running", toastInfo) // newer: the merge is not the last one

	*now = now.Add(time.Second)
	m.toast.replace("merge ok", "merge ok · worktree removed", toastSuccess)

	got := m.toast.texts()
	if len(got) != 2 {
		t.Fatalf("texts = %q, want 2 (the old replaced, the new intact)", got)
	}
	if got[0] != "merge ok · worktree removed" {
		t.Errorf("the old notice should be replaced in place: %q", got)
	}
	if got[1] != "an action is running" {
		t.Errorf("the newest notice should not be touched: %q", got)
	}
	if !m.toast.toasts[0].created.Equal(*now) {
		t.Errorf("the replaced notices TTL should restart: created=%v, now=%v", m.toast.toasts[0].created, *now)
	}
}

func TestToastReplaceAppendsWhenTheTargetIsGone(t *testing.T) {
	m, now := toastTestModel(t)
	m.toast.show("merge ok", toastSuccess)

	*now = now.Add(toastDuration + time.Second)
	m.toast.update()
	m.toast.show("refreshed", toastInfo)
	if got := m.toast.texts(); len(got) != 1 || got[0] != "refreshed" {
		t.Fatalf("setup: texts = %q, want only the new notice", got)
	}

	m.toast.replace("merge ok", "merge ok · worktree removed", toastSuccess)
	got := m.toast.texts()
	if len(got) != 2 || got[0] != "refreshed" || got[1] != "merge ok · worktree removed" {
		t.Fatalf("texts = %q, want the new notice stacked at the end", got)
	}
}
