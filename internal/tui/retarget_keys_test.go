package tui

import (
	"context"
	"strings"
	"testing"
	"time"

	"prdash/internal/forge/model"
	"prdash/internal/testutil"
)

func modelInRetarget(t *testing.T, phase retargetState) Model {
	t.Helper()
	m := newTestModel(t, &testutil.FakeAdapter{ForgeName: "github", HostName: "github.com"})
	// Both all and view are filled because view is DERIVED from all: reapplying the filter rebuilds it from all.
	branches := []string{"main", "release/2.0", "feat/x"}
	m.retarget = retargetPanel{
		state:  phase,
		item:   mkItem("github", "github.com", "acme/widget", "something", 7, ""),
		all:    branches,
		view:   branches,
		cursor: 0,
	}
	return m
}

func TestEscClosesThePopupExceptInTheConfirmationWhereItStepsBack(t *testing.T) {
	for _, phase := range []retargetState{retargetListing, retargetChoosing} {
		m := modelInRetarget(t, phase)
		before := m.branchSeq
		output, _ := pressWithCmd(t, m, "esc")
		got := output

		if got.retarget.state != retargetClosed {
			t.Errorf("esc in phase %d did not close the popup (state=%d)", phase, got.retarget.state)
		}
		if got.branchSeq == before {
			t.Errorf("esc in phase %d did not invalidate the in-flight listing", phase)
		}
	}

	m := modelInRetarget(t, retargetConfirm)
	m.retarget.query = "what the user typed"
	before := m.branchSeq
	output, _ := pressWithCmd(t, m, "esc")
	got := output

	if got.retarget.state != retargetChoosing {
		t.Errorf("esc in the confirmation left the popup at %d, want %d (back to choosing)",
			got.retarget.state, retargetChoosing)
	}
	if got.branchSeq != before {
		t.Error("esc in the confirmation invalidated the listing: going back to the list is not starting " +
			"from zero, and asking for the branches again would be waiting on the forge for no reason")
	}
	if got.retarget.query != "what the user typed" {
		t.Errorf("the filter ended as %q: going back to the list must not drop it",
			got.retarget.query)
	}
}

func TestInTheListingPhaseNoKeyDoesAnythingAndThePopupKeepsWaiting(t *testing.T) {
	for _, key := range []string{"x", "enter", "j", "k", "up", "down", "tab", "?"} {
		m := modelInRetarget(t, retargetListing)
		output, cmd := pressWithCmd(t, m, key)
		got := output

		if got.retarget.state != retargetListing {
			t.Errorf("%q in the listing phase moved the popup to %d", key, got.retarget.state)
		}
		if cmd != nil {
			t.Errorf("%q in the listing phase returned a command: a consumed key cannot "+
				"launch anything", key)
		}
	}
}

func TestInTheConfirmationOnlyEnterDoesSomethingAndEnterClosesThePopupBecauseTheChangeIsInFlight(t *testing.T) {
	m := modelInRetarget(t, retargetConfirm)
	m.retarget.chosen = "release/2.0"
	output, cmd := pressWithCmd(t, m, "enter")

	if cmd != nil {
		t.Error("enter in the confirmation returned a tea.Cmd: the panel puts the request " +
			"in flight and rewrites through the events channel")
	}
	got := output
	if got.retarget.state != retargetClosed {
		t.Errorf("after confirming the popup ended at %d, want closed: what is shown to the "+
			"user is the headers progress notice, not the popup", got.retarget.state)
	}
	if got.retarget.chosen != "" {
		t.Errorf("the chosen branch ended as %q after closing: a second enter could reapply "+
			"the same change", got.retarget.chosen)
	}
	if len(got.toast.texts()) == 0 && !strings.Contains(m.retarget.item.Title, "retarget") {
		t.Error("no progress notice was left after confirming")
	}

	for _, key := range []string{"x", "j", "k", "tab", " "} {
		m2 := modelInRetarget(t, retargetConfirm)
		m2.retarget.chosen = "main"
		out2, cmd2 := pressWithCmd(t, m2, key)
		if out2.retarget.state != retargetConfirm {
			t.Errorf("%q in the confirmation moved the popup to %d", key, out2.retarget.state)
		}
		if cmd2 != nil {
			t.Errorf("%q in the confirmation returned a command", key)
		}
	}
}

// Same asymmetry as the simulation, same reason: esc closes the popup and stays, q leaves.
func TestQAndQuitCloseAndAbandonThePopup(t *testing.T) {
	for _, phase := range []retargetState{retargetListing, retargetChoosing, retargetConfirm} {
		for _, key := range []string{"q", "ctrl+c"} {
			m := modelInRetarget(t, phase)
			output, cmd := pressWithCmd(t, m, key)
			got := output

			if got.retarget.state != retargetClosed {
				t.Errorf("%q in phase %d did not close the popup", key, phase)
			}
			if cmd == nil {
				t.Errorf("%q in phase %d did not ask to leave the TUI", key, phase)
			}
		}
	}
}

func TestWithAnEmptyFilterJAndKNavigateAndWithAFilterTheyTypeItsLetters(t *testing.T) {
	// `k` is tested with the cursor mid-list because clampRetargetCursor CLAMPS instead of wrapping.
	m := modelInRetarget(t, retargetChoosing)
	if before := m.retarget.cursor; pressModel(t, m, "j").retarget.cursor == before {
		t.Error("with an empty filter, j did not move the cursor")
	}
	mAbove := modelInRetarget(t, retargetChoosing)
	mAbove.retarget.cursor = 2
	if before := mAbove.retarget.cursor; pressModel(t, mAbove, "k").retarget.cursor == before {
		t.Error("with an empty filter, k did not move the cursor")
	}
	mFirst := modelInRetarget(t, retargetChoosing)
	if got := pressModel(t, mFirst, "k").retarget.cursor; got != 0 {
		t.Errorf("k on the first left the cursor at %d, want 0: the clipping does not wrap", got)
	}
	m2 := modelInRetarget(t, retargetChoosing)
	if before := m2.retarget.cursor; pressModel(t, m2, "down").retarget.cursor == before {
		t.Error("with an empty filter, down did not move the cursor")
	}

	m3 := modelInRetarget(t, retargetChoosing)
	m3.retarget.query = "re"
	before := m3.retarget.cursor
	output := pressModel(t, m3, "j")
	if output.retarget.cursor != before {
		t.Error("with a filter typed, j moved the cursor: it would type and navigate at once")
	}
	if !strings.Contains(output.retarget.query, "j") {
		t.Errorf("with a filter typed, j did not end up in the filter: it is %q", output.retarget.query)
	}

	m4 := modelInRetarget(t, retargetChoosing)
	m4.retarget.query = "release"
	deleted := pressModel(t, m4, "ctrl+u")
	if deleted.retarget.query != "" {
		t.Errorf("ctrl+u left the filter as %q", deleted.retarget.query)
	}
	// The cursor is back at ZERO because applyQuery returns it to the top and k/j clamp rather than wrap.
	toNavigate := pressModelRetarget(t, deleted, "j")
	toNavigate.retarget.cursor = 0
	if afterVar := pressModelRetarget(t, toNavigate, "j"); afterVar.retarget.cursor != 1 {
		t.Errorf("after ctrl+u, j does not navigate again: the cursor ended at %d, want 1. It stays "+
			"as a letter with the empty filter", afterVar.retarget.cursor)
	}
	if afterVar := pressModelRetarget(t, toNavigate, "j"); afterVar.retarget.query != "" {
		t.Errorf("j with the empty filter typed %q", afterVar.retarget.query)
	}
	if len(deleted.retarget.view) != 3 {
		t.Errorf("after ctrl+u %d visible branches stay, want the 3: the filter was removed but "+
			"the list ate them", len(deleted.retarget.view))
	}
}

func TestEnterWithNoSelectionDoesNothing(t *testing.T) {
	m := modelInRetarget(t, retargetChoosing)
	m.retarget.view = nil
	m.retarget.cursor = 0

	output, cmd := pressWithCmd(t, m, "enter")
	got := output

	if got.retarget.state != retargetChoosing {
		t.Errorf("enter with no selection moved the popup to %d", got.retarget.state)
	}
	if got.retarget.chosen != "" {
		t.Errorf("enter with no selection chose %q: a base change that nobody "+
			"asked for would be applied", got.retarget.chosen)
	}
	if cmd != nil {
		t.Error("enter with no selection returned a command: there is nothing to confirm")
	}
}

func TestAnEmptyListingSaysThereAreNoBranchesAndDoesNotOpenTheSelector(t *testing.T) {
	m := modelInRetarget(t, retargetListing)
	m.branchSeq = 5

	output := send(t, m, branchesMsg{seq: 5, key: repoKeyOf(m), names: nil})
	got := output

	if strings.TrimSpace(got.retarget.errMsg) == "" {
		t.Error("an empty listing left no message: the popup would look like it waits on the forge")
	}
	if !strings.Contains(got.retarget.errMsg, "no branches") {
		t.Errorf("the message %q does not say there are no branches", got.retarget.errMsg)
	}
	if got.retarget.state == retargetChoosing {
		t.Error("the selector opened with no branches: an empty searcher would be shown")
	}
}

func TestAStaleListingIsDiscardedAndNotStored(t *testing.T) {
	m := modelInRetarget(t, retargetListing)
	m.branchSeq = 7

	output := send(t, m, branchesMsg{seq: 3, key: repoKeyOf(m), names: []string{"main", "vieja"}})
	got := output

	if _, exists := got.branchCache[repoKeyOf(got)]; exists {
		t.Error("a stale listing was stored in the cache: the popup would open with branches " +
			"from ten minutes ago")
	}
	if strings.Contains(got.retarget.errMsg, "no branches") {
		t.Error("a stale listing set an error message: it is not a failure, it is a result " +
			"that no longer belongs to anyone")
	}
}

func TestAStaleListingWithErrorIsNeitherStoredNorShown(t *testing.T) {
	m := modelInRetarget(t, retargetChoosing)
	m.branchSeq = 7
	m.retarget.errMsg = ""

	output := send(t, m, branchesMsg{
		seq: 3, key: repoKeyOf(m), errMsg: "the forge does not answer",
	})
	got := output

	if got.retarget.errMsg != "" {
		t.Errorf("an error of a stale request was shown: %q", got.retarget.errMsg)
	}
	if _, exists := got.branchCache[repoKeyOf(got)]; exists {
		t.Error("a stale listing with an error was stored in the cache")
	}
}

// storeBranches has a LAZY map initialisation that a Model from New does not trigger.
func TestAListingIsCachedAndTheSecondRequestDoesNotAskAgain(t *testing.T) {
	m := modelInRetarget(t, retargetListing)
	m.branchSeq = 5
	key := repoKeyOf(m)

	output := send(t, m, branchesMsg{seq: 5, key: key, names: []string{"main", "feat/x"}})
	got := output

	saved, exists := got.branchCache[key]
	if !exists {
		t.Fatalf("the listing was not stored in the cache; the map is %v", got.branchCache)
	}
	if len(saved.names) != 2 || saved.names[0] != "main" {
		t.Errorf("the cache stores %v", saved.names)
	}
	if saved.fetchedAt.IsZero() {
		t.Error("the cache does not note when it was requested: there is no way to know if it expires")
	}
	if !saved.fetchedAt.After(time.Time{}) {
		t.Error("the cache date is older than nothing")
	}
	if got.retarget.state != retargetChoosing {
		t.Errorf("after the listing the popup ended at %d, want %d", got.retarget.state, retargetChoosing)
	}
}

func repoKeyOf(m Model) repoKey {
	return keyOf(m.retarget.item)
}

var _ = context.Background
var _ = model.RepoRef{}

func pressModelRetarget(t *testing.T, m Model, key string) Model {
	t.Helper()
	return pressModel(t, m, key)
}

// Reachable because Model is built zero-valued in one place.
func TestStoreBranchesOnAZeroValueModelDoesNotPanic(t *testing.T) {
	var m Model
	m.storeBranches(repoKey{forge: "github", host: "github.com", project: "o/r"},
		[]string{"main"})

	if len(m.branchCache) != 1 {
		t.Errorf("the map ended with %d entries, want 1: storeBranches did not initialize it",
			len(m.branchCache))
	}
}
