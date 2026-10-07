package tui

import (
	"context"
	"image"
	"strings"
	"testing"

	"prdash/internal/forge/model"
	"prdash/internal/herdr"
	"prdash/internal/testutil"
	"prdash/internal/worktree"
)

func TestWiringReturnsExactlyWhatTheModelHasWired(t *testing.T) {
	full := newTestModel(t)
	full.mounter = &wiringMounter{}
	full.simulator = &stubSimulator{available: true}
	full.graphics = &wiringGraphics{}
	full.reviewLookup = &wiringReviewLookup{}
	full.reviewRemover = &wiringReviewRemover{}

	w := full.Wiring()
	if w.Mounter == nil || w.Simulator == nil || w.Graphics == nil ||
		w.ReviewLookup == nil || w.ReviewRemover == nil {
		t.Errorf("Wiring lost some dependency: %+v", w)
	}
	// And they are the SAME instances, which is what makes the accessor worth having.
	if w.Simulator != full.simulator {
		t.Error("Wiring returned another Simulator: a copying accessor is useless for checking the wiring")
	}
	if w.Graphics != full.graphics {
		t.Error("Wiring returned another Graphics")
	}

	empty := newTestModel(t)
	empty.mounter = nil
	empty.simulator = nil
	empty.graphics = nil
	wv := empty.Wiring()
	if wv.Mounter != nil || wv.Simulator != nil || wv.Graphics != nil ||
		wv.ReviewLookup != nil || wv.ReviewRemover != nil {
		t.Errorf("Wiring on an empty model returned something: %+v", wv)
	}
}

func TestWithNoKnownHeightTheScrollAdvancesSixRows(t *testing.T) {
	m := newTestModel(t)
	m.height = 0

	if rows := m.pageRows(); rows != 6 {
		t.Errorf("with no known height pageRows = %d, want 6", rows)
	}
	m.scroll = 0
	m.scroll += m.pageRows()
	if m.scroll != 6 {
		t.Errorf("the scroll ended at %d", m.scroll)
	}

	m2 := newTestModel(t)
	m2.height = 40
	if rows := m2.pageRows(); rows == 6 {
		t.Error("with a known height it still uses the six row default")
	}
}

func TestAForgeThatFinishesLoadingStopsBeingInLoading(t *testing.T) {
	m := modelWithLoadingStatuses(t)

	output := send(t, m, forgeDoneMsg{cycle: 3, forge: "github"})
	got := output
	if got.statuses["github"].loading {
		t.Error("github forgeDoneMsg did not turn off its loading: that forges spinner would never stop")
	}
	if !got.statuses["gitlab"].loading {
		t.Error("github forgeDoneMsg turned off gitlab loading, which is still loading")
	}

	unknown := send(t, m, forgeDoneMsg{cycle: 3, forge: "bitbucket"})
	if _, ok := unknown.statuses["bitbucket"]; ok {
		t.Error("a forgeDoneMsg for an unknown forge created it in the map")
	}

	// A STALE one touches neither, and with a NEW model, because the statuses map is fresh.
	stale := send(t, modelWithLoadingStatuses(t), forgeDoneMsg{cycle: 1, forge: "github"})
	if !stale.statuses["github"].loading || !stale.statuses["gitlab"].loading {
		t.Error("a stale forgeDoneMsg turned off a loading: it would kill the spinner of a forge " +
			"that is really still loading")
	}
}

func TestOpeningTheBrowserWithoutURLIsRefusedAndReported(t *testing.T) {
	m := newTestModel(t, &testutil.FakeAdapter{ForgeName: "github", HostName: "github.com"})
	output, cmd := pressWithCmd(t, m, "o")
	got := output
	if cmd != nil {
		t.Error("opening with no selection returned a command: it would run a viewer on nothing")
	}
	// The warning does not separate "no selection" from "the item has no URL": one guard.
	if av := lastToast(got); !strings.Contains(av, "no URL") {
		t.Errorf("the notice %q does not say there is nothing to open", av)
	}

	m2 := newTestModel(t, &testutil.FakeAdapter{ForgeName: "github", HostName: "github.com"})
	withoutURL := mkItem("github", "github.com", "acme/widget", "one", 7, "")
	withoutURL.URL = ""
	m2 = withSelection(t, m2, withoutURL)

	out2, cmd2 := pressWithCmd(t, m2, "o")
	got2 := out2
	if cmd2 != nil {
		t.Error("opening an item with no URL returned a command")
	}
	if av := lastToast(got2); !strings.Contains(av, "no URL") {
		t.Errorf("the notice %q does not say the item has no URL", av)
	}
	if lastToastLevel(got2) != toastWarning {
		t.Errorf("level %v, want warn", lastToastLevel(got2))
	}
}

type wiringGraphics struct{}

func (wiringGraphics) Available() bool                     { return false }
func (wiringGraphics) CellSize(context.Context) (int, int) { return 1, 2 }
func (wiringGraphics) SetImage(context.Context, string, image.Image, herdr.Placement) error {
	return nil
}
func (wiringGraphics) Clear(context.Context, string) error { return nil }

type wiringReviewLookup struct{}

func (wiringReviewLookup) ActiveReview(model.Item) (worktree.Worktree, bool) {
	return worktree.Worktree{}, false
}

type wiringReviewRemover struct{}

func (wiringReviewRemover) RemoveReview(context.Context, model.Item) (bool, string, error) {
	return false, "", nil
}

func modelWithLoadingStatuses(t *testing.T) Model {
	t.Helper()
	m := newTestModel(t, &testutil.FakeAdapter{ForgeName: "github", HostName: "github.com"})
	m.cycle = 3
	m.statuses = map[string]*forgeStatus{
		"github": {forge: "github", host: "github.com", loading: true},
		"gitlab": {forge: "gitlab", host: "gitlab.example.com", loading: true},
	}
	return m
}
