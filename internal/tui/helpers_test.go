package tui

import (
	"testing"

	"prdash/internal/forge/model"
	"prdash/internal/testutil"
)

// The identity is forge, host, project and number.
func TestFindItemMatchesByIdentityAndDoesNotMixItemsFromDifferentForges(t *testing.T) {
	gh := mkItem("github", "github.com", "acme/widget", "one", 7, "")
	gl := mkItem("gitlab", "gitlab.example.com", "other/widget", "other", 7, "")
	items := []model.Item{gh, gl}

	got, ok := findItem(items, gh.ID())
	if !ok || got.ID() != gh.ID() {
		t.Errorf("findItem of the first returned (%v, %v)", got.Number, ok)
	}
	got, ok = findItem(items, gl.ID())
	if !ok || got.ID() != gl.ID() {
		t.Errorf("findItem of the second returned (%v, %v): %+v", got.Number, ok, got)
	}

	// An id that is not there gives the zero value: a bare model.Item{} would look like a real item.
	id := gh.ID()
	id.Project = "no/exists"
	got, ok = findItem(items, id)
	if ok {
		t.Error("findItem of a missing id gave ok")
	}
	if got.ID() != (model.ID{}) {
		t.Errorf("findItem returned %+v instead of the zero value", got.ID())
	}
	if _, ok := findItem(nil, gh.ID()); ok {
		t.Error("findItem over nil gave ok")
	}
}

func TestTheSectionCycleCyclesAndTheActiveSectionIsRemembered(t *testing.T) {
	// The positions map is created in New, so the nil check in setActiveSection is redundant.
	m := newTestModel(t)

	start := m.activeSection
	for range sectionCycle {
		m.cycleSection()
	}
	if m.activeSection != start {
		t.Errorf("a full turn of the cycle ended at %v, want %v", m.activeSection, start)
	}

	m2 := newTestModel(t, &testutil.FakeAdapter{ForgeName: "github", HostName: "github.com"})
	k := streamKey{forge: "github", section: model.SectionReview, kind: model.ReviewRequested}
	m2.streams[k] = &stream{}
	for _, n := range []int{1, 2, 3} {
		it := mkItem("github", "github.com", "acme/widget", "t", n, "")
		it.Section = model.SectionReview
		m2.streams[k].items = append(m2.streams[k].items, it)
	}
	m2.rebuild()
	m2.cursor = 2
	m2.scroll = 1

	m2.cycleSection() // to another section, with no items: the cursor is clamped to zero
	m2.setActiveSection(model.SectionReview)
	if m2.cursor != 2 {
		t.Errorf("back at review the cursor ended at %d, want 2: the position is remembered "+
			"per section and is not reset", m2.cursor)
	}
	// The scroll is remembered in memory, but with three items in a forty-row terminal there is nothing to scroll.

	m2.setActiveSection(model.SectionMentions)
	if m2.cursor != 0 || m2.scroll != 0 {
		t.Errorf("a new section ended at cursor=%d scroll=%d, want 0 and 0",
			m2.cursor, m2.scroll)
	}

	var zero Model
	zero.setActiveSection(model.SectionReview)
	if len(zero.pos) != 1 {
		t.Errorf("setActiveSection on a zero model left %d positions, want 1: "+
			"writing to a nil map is a PANIC, not a visible error", len(zero.pos))
	}

	m2.activeSection = "an-invented-section"
	if got := m2.sectionCycleIndex(); got != 0 {
		t.Errorf("a section outside the cycle gave index %d, want 0", got)
	}
	m2.cycleSection()
}

func TestRequestCommentsRejectsTheThreeNegativesAndAlwaysRearmsTheTick(t *testing.T) {
	for _, c := range []struct {
		name    string
		prepara func(*testing.T) Model
	}{
		{"no selection", func(t *testing.T) Model {
			return newTestModel(t, &testutil.FakeAdapter{ForgeName: "github", HostName: "github.com"})
		}},
		{"unknown forge for the item", func(t *testing.T) Model {
			// The item is selected with its forge present and then removed: building a gitlab-only model never arrived.
			m := newTestModel(t, &testutil.FakeAdapter{ForgeName: "github", HostName: "github.com"})
			m = withSelection(t, m, mkItem("github", "github.com", "acme/widget", "one", 7, ""))
			if _, ok := m.selected(); !ok {
				t.Fatal("useless fixture: the item was not left selected")
			}
			delete(m.byForge, "github")
			return m
		}},
		{"conversation already loaded", func(t *testing.T) Model {
			m := newTestModel(t, &testutil.FakeAdapter{ForgeName: "github", HostName: "github.com"})
			it := mkItem("github", "github.com", "acme/widget", "one", 7, "")
			m = withSelection(t, m, it)
			m.comments[it.ID()] = &commentState{list: []model.Comment{{Author: "x", Body: "y"}}, ready: true}
			return m
		}},
	} {
		t.Run(c.name, func(t *testing.T) {
			m := c.prepara(t)
			if cmd := m.requestComments(); cmd == nil {
				t.Error("the tick was not rearmed: the chain is short and the next selection " +
					"change would go unnoticed")
			}
			if cmd := m.commentsCmd(); cmd == nil {
				t.Error("commentsCmd returned nil")
			}
		})
	}
}
