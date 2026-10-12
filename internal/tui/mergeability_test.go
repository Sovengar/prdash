package tui

import (
	"strings"
	"testing"

	"prdash/internal/forge/model"
	"prdash/internal/inbox"
)

// The lazy value the inbox list carried as UNKNOWN must be resolved before the
// arm-time gate reads it.
func TestMergeabilityUnknownIsResolvedOnRefresh(t *testing.T) {
	it := gatedItems(model.MergeRulesAll())[0]
	it.TargetBranch = "main"
	f := newMergeFixture(t, it)

	conflicted := it
	conflicted.Mergeable = model.Mergeability{Known: true, Conflicted: true}
	f.adp.ItemStates[stateKey(it)] = conflicted

	m := f.m
	m.requestMergeability()
	ev := waitMount(t, m)
	msg, ok := ev.(mergeabilityMsg)
	if !ok {
		t.Fatalf("the resolution returned %T, want mergeabilityMsg", ev)
	}
	m = send(t, m, msg)

	if view := stripANSI(m.View().Content); !strings.Contains(view, "✗") {
		t.Errorf("the row does not mark the resolved conflict:\n%s", view)
	}
	m = press(t, m, "m")
	if m.mergeArmed {
		t.Error("the resolved conflict should refuse arming")
	}
	if !strings.Contains(toasts(m), "conflicts with main") {
		t.Errorf("the notice does not name the conflict: %q", toasts(m))
	}
}

// An UNKNOWN is not a green light: the arm still happens (the forge decides),
// but the user is told instead of being left to the post-hoc refusal.
func TestMergeOnUnknownMergeabilityWarnsAtArmTime(t *testing.T) {
	f := newMergeFixture(t, gatedItems(model.MergeRulesAll())...)

	m := press(t, f.m, "m")
	if !m.mergeArmed {
		t.Fatal("an unknown mergeability should not block arming")
	}
	if !strings.Contains(toasts(m), "has not computed the mergeability") {
		t.Errorf("the arm-time warning is missing: %q", toasts(m))
	}
}

func TestRequestMergeabilitySkipsKnownItemsAndNoSelection(t *testing.T) {
	items := gatedItems(model.MergeRulesAll())
	items[0].Mergeable = model.Mergeability{Known: true}
	f := newMergeFixture(t, items...)
	f.m.requestMergeability()
	if len(f.m.mergeAttempts) != 0 {
		t.Errorf("a known item was checked: %v", f.m.mergeAttempts)
	}

	empty := newTestModel(t, ghAdapter())
	empty.requestMergeability()
	if len(empty.mergeAttempts) != 0 {
		t.Errorf("with no selection it checked something: %v", empty.mergeAttempts)
	}
}

func TestRequestMergeabilityDoesNotStackWhileOneIsInFlight(t *testing.T) {
	it := gatedItems(model.MergeRulesAll())[0]
	f := newMergeFixture(t, it)

	f.m.requestMergeability()
	f.m.requestMergeability()
	if got := f.m.mergeAttempts[it.ID()]; got != 1 {
		t.Fatalf("attempts = %d, want 1: a second request was stacked onto the one in flight", got)
	}
}

func TestRequestMergeabilityStopsAtTheAttemptCap(t *testing.T) {
	it := gatedItems(model.MergeRulesAll())[0]
	f := newMergeFixture(t, it)
	f.m.mergeAttempts[it.ID()] = maxMergeabilityChecks

	f.m.requestMergeability()
	if f.m.mergeInFlight[it.ID()] {
		t.Error("a capped item was checked again: the forge could be called forever")
	}
	if got := f.m.mergeAttempts[it.ID()]; got != maxMergeabilityChecks {
		t.Fatalf("attempts = %d, want them untouched at the cap", got)
	}
}

func TestRequestMergeabilitySkipsAForgeWithoutAnAdapter(t *testing.T) {
	it := gatedItems(model.MergeRulesAll())[0]
	f := newMergeFixture(t, it)
	delete(f.m.byForge, it.Forge)

	f.m.requestMergeability()
	if len(f.m.mergeAttempts) != 0 {
		t.Errorf("with no adapter wired it still requested: %v", f.m.mergeAttempts)
	}
}

func TestRequestMergeabilitySkipsAnUnauthenticatedForge(t *testing.T) {
	it := gatedItems(model.MergeRulesAll())[0]
	f := newMergeFixture(t, it)
	f.m = send(t, f.m, authMsg{cycle: f.m.cycle, forge: it.Forge, auth: model.AuthState{Forge: it.Forge, OK: false}})

	f.m.requestMergeability()
	if len(f.m.mergeAttempts) != 0 {
		t.Errorf("an unauthenticated forge was called: %v", f.m.mergeAttempts)
	}
}

func TestApplyMergeabilityResetsTheAttemptsOnARealValue(t *testing.T) {
	it := gatedItems(model.MergeRulesAll())[0]
	f := newMergeFixture(t, it)
	m := f.m
	m.requestMergeability()

	resolved := it
	resolved.Mergeable = model.Mergeability{Known: true}
	m = send(t, m, mergeabilityMsg{id: it.ID(), item: resolved})
	if got := m.mergeAttempts[it.ID()]; got != 0 {
		t.Fatalf("attempts = %d, want 0 after a resolved value", got)
	}
	if m.mergeInFlight[it.ID()] {
		t.Error("the in-flight flag should clear when the result arrives")
	}
}

// The marker is a claim, so it is painted only for a conflict the forge
// reported: neither UNKNOWN nor MERGEABLE get one. It is the row's own gutter,
// not text inside the title.
func TestMarkCellMarksOnlyAReportedConflict(t *testing.T) {
	it := mkItem("github", "github.com", "acme/widget", "Add widget", 1, "")
	for _, tc := range []struct {
		name string
		m    model.Mergeability
		want string
	}{
		{"unknown", model.Mergeability{}, ""},
		{"mergeable", model.Mergeability{Known: true}, ""},
		{"conflict", model.Mergeability{Known: true, Conflicted: true}, "✗"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			it.Mergeable = tc.m
			if got := markCell(it, colMark).text; got != tc.want {
				t.Fatalf("marker = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestMergeableDetailDistinguishesTheThreeStates(t *testing.T) {
	it := mkItem("github", "github.com", "acme/widget", "Add widget", 1, "")
	it.TargetBranch = "main"
	cases := []struct {
		name string
		m    model.Mergeability
		want string
	}{
		{"unknown", model.Mergeability{}, "unknown"},
		{"mergeable", model.Mergeability{Known: true}, "yes"},
		{"conflict", model.Mergeability{Known: true, Conflicted: true}, "no (conflicts with main)"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			it.Mergeable = tc.m
			if got := mergeableDetail(it); got != tc.want {
				t.Fatalf("mergeableDetail = %q, want %q", got, tc.want)
			}
		})
	}

	it.Mergeable = model.Mergeability{Known: true, Conflicted: true}
	it.TargetBranch = "  "
	if got := mergeableDetail(it); got != "no (conflicts with the target)" {
		t.Fatalf("an unnamed target = %q, want the generic wording", got)
	}
}

// The marker must not ride in the title column.
func TestConflictMarkerIsNotInTheTitleCell(t *testing.T) {
	it := mkItem("github", "github.com", "acme/widget", "Add widget", 1, "")
	it.Mergeable = model.Mergeability{Known: true, Conflicted: true}
	lay := newRefLayout([]inbox.Section{section(model.SectionReview, it)}, prefixCommon)
	cells := itemCells(it, model.SectionReview, "me", lay)

	if got := cells[colTitleIdx].text; got != "Add widget" {
		t.Fatalf("title cell = %q, want the plain title", got)
	}
	if got := cells[colMarkIdx].text; got != "✗" {
		t.Fatalf("marker cell = %q, want the conflict marker", got)
	}
}

func TestApplyMergeabilityIgnoresAnEmptyItem(t *testing.T) {
	f := newMergeFixture(t, gatedItems(model.MergeRulesAll())...)
	before := len(f.m.rows())
	f.m.applyMergeability(mergeabilityMsg{})
	if got := len(f.m.rows()); got != before {
		t.Fatalf("rows = %d, want %d: an empty item must not be inserted", got, before)
	}
}
