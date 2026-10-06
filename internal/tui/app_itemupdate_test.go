package tui

import (
	"testing"
	"time"

	"prdash/internal/forge/model"
)

// Without it, merging a PR would leave it looking unmerged until the next refresh.

// The failure mergeItem exists for.
func TestAnItemThatIsNotThereIsAddedAndTheSectionIsNotLost(t *testing.T) {
	m := newTestModel(t)
	original := model.Item{
		Section: model.SectionReview, ReviewKind: model.ReviewRequested,
		Forge: "github", Host: "github.com", Number: 7,
		Ref: model.RepoRef{Project: "o/r"}, Title: "before", UpdatedAt: time.Now(),
	}
	withItems(t, &m, original)
	m.rebuild()

	// The re-read: the forge answers the state but does not know which section the item came from.
	reread := original
	reread.Section = ""
	reread.ReviewKind = ""
	reread.Title = "after"
	m.applyItemUpdate(reread)

	it, ok := findItem(allTheItems(m), original.ID())
	if !ok {
		t.Fatalf("the item disappeared after updating it: %v", allTheItems(m))
	}
	if it.Title != "after" {
		t.Errorf("the title was not updated: %q", it.Title)
	}
	if it.Section != model.SectionReview {
		t.Errorf("the section was lost: %q", it.Section)
	}
	if it.ReviewKind != model.ReviewRequested {
		t.Errorf("the review kind was lost: %q", it.ReviewKind)
	}
	// And it stays in the stream it was in, because the user had it in front of them.
	withItems := 0
	for _, s := range m.streams {
		withItems += len(s.items)
	}
	if withItems != 1 {
		t.Errorf("there are %d items in the streams after updating, want 1", withItems)
	}
}

// The other side of that.
func TestAnItemThatIsNotThereIsAddedToTheRightStream(t *testing.T) {
	m := newTestModel(t)
	m.rebuild()

	new := model.Item{
		Section: model.SectionMentions, ReviewKind: "",
		Forge: "gitlab", Host: "gitlab.com", Number: 3,
		Ref: model.RepoRef{Project: "o/r"}, Title: "new",
	}
	m.applyItemUpdate(new)

	it, ok := findItem(allTheItems(m), new.ID())
	if !ok {
		t.Fatalf("the added item does not appear: %v", allTheItems(m))
	}
	if it.Title != "new" || it.Forge != "gitlab" {
		t.Errorf("the added item did not arrive whole: %+v", it)
	}
	enReview := false
	for k, s := range m.streams {
		for _, streamed := range s.items {
			if streamed.ID() == new.ID() && k.section != model.SectionMentions {
				enReview = true
			}
		}
	}
	if enReview {
		t.Error("the item went into a stream that is not its own")
	}

	other := new
	other.Forge = "github"
	other.Host = "github.com"
	m.applyItemUpdate(other)
	total := 0
	for _, s := range m.streams {
		total += len(s.items)
	}
	if total != 2 {
		t.Errorf("two forges with the same number ended with %d items, want 2", total)
	}
}

// The three things an update must not do.
func TestUpdateNeitherDuplicatesNorDeletesTheRest(t *testing.T) {
	m := newTestModel(t)
	base := model.Item{
		Section: model.SectionReview, ReviewKind: model.ReviewRequested,
		Forge: "github", Host: "github.com", Number: 1,
		Ref: model.RepoRef{Project: "o/r"}, Title: "one",
	}
	two := base
	two.Number = 2
	two.Title = "two"
	three := base
	three.Number = 3
	three.Title = "three"
	withItems(t, &m, base, two, three)
	m.rebuild()
	before := len(allTheItems(m))

	updated := base
	updated.Title = "one changed"
	updated.State = "MERGED"
	m.applyItemUpdate(updated)

	if len(allTheItems(m)) != before {
		t.Errorf("after updating there are %d items and there were %d", len(allTheItems(m)), before)
	}
	vistos := 0
	for _, it := range allTheItems(m) {
		if it.ID() == base.ID() {
			vistos++
			if it.Title != "one changed" || it.State != "MERGED" {
				t.Errorf("the item was not left updated: %+v", it)
			}
		}
	}
	if vistos != 1 {
		t.Errorf("the item appears %d times after updating it", vistos)
	}
	titles := map[string]bool{}
	for _, it := range allTheItems(m) {
		titles[it.Title] = true
	}
	for _, wantTitle := range []string{"one changed", "two", "three"} {
		if !titles[wantTitle] {
			t.Errorf("after updating %q is missing; left %v", wantTitle, titles)
		}
	}
}

// Both halves, and the default matters as much as the cases.
func TestMergeItemKeepsWhatTheForgeDoesNotKnowAndChangesWhatItDoes(t *testing.T) {
	old := model.Item{Section: model.SectionReview, ReviewKind: model.ReviewRequested, Title: "old"}

	fresh := model.Item{Title: "fresh"}
	merged := mergeItem(old, fresh)
	if merged.Section != model.SectionReview || merged.ReviewKind != model.ReviewRequested {
		t.Errorf("it did not keep what the reread item did not bring: %+v", merged)
	}
	if merged.Title != "fresh" {
		t.Errorf("the title is not the reread one: %q", merged.Title)
	}

	// With a section set: the re-read's wins, and that is the retarget.
	fresh = model.Item{Title: "fresh", Section: model.SectionAuthored, ReviewKind: ""}
	merged = mergeItem(old, fresh)
	if merged.Section != model.SectionAuthored {
		t.Errorf("with its own section the old one was put: %q", merged.Section)
	}
	if merged.ReviewKind != model.ReviewRequested {
		t.Errorf("keeping the section prevented keeping the kind: %q", merged.ReviewKind)
	}

	fresh = model.Item{Title: "fresh", ReviewKind: model.ReviewAssigned}
	merged = mergeItem(old, fresh)
	if merged.Section != model.SectionReview || merged.ReviewKind != model.ReviewAssigned {
		t.Errorf("no mezclo bien: %+v", merged)
	}

	merged = mergeItem(model.Item{}, model.Item{Title: "fresh"})
	if merged.Section != "" || merged.ReviewKind != "" {
		t.Errorf("with an empty old item it invented something: %+v", merged)
	}
}

// The difference between "no" and "not known".
func TestOtherwiseTheDetailDoesNotShowADash(t *testing.T) {
	if got := yesNo(true); got != "yes" {
		t.Errorf("yesNo(true) gave %q", got)
	}
	if got := yesNo(false); got != "no" {
		t.Errorf("yesNo(false) gave %q, want \"no\": a false is an answer, not an absence", got)
	}
	// The difference with orDash is real: a false does NOT print as a dash.
	if yesNo(false) == orDash("") {
		t.Error("yesNo(false) comes out the same as orDash of an empty string: they are different things")
	}
	if got := orDash(""); got != "-" {
		t.Errorf("orDash(\"\") gave %q", got)
	}
	if got := orDash("something"); got != "something" {
		t.Errorf("orDash with text gave %q", got)
	}
}

func withItems(t *testing.T, m *Model, items ...model.Item) {
	t.Helper()
	for _, it := range items {
		k := streamKey{forge: it.Forge, section: it.Section, kind: it.ReviewKind}
		s := m.streams[k]
		if s == nil {
			s = &stream{}
			m.streams[k] = s
		}
		s.items = append(s.items, it)
	}
}

func allTheItems(m Model) []model.Item {
	var out []model.Item
	for _, s := range m.streams {
		out = append(out, s.items...)
	}
	return out
}
