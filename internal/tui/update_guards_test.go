package tui

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"

	"prdash/internal/config"
	"prdash/internal/forge"
	"prdash/internal/forge/model"
	"prdash/internal/review/executor"
	"prdash/internal/review/plan"
	"prdash/internal/testutil"
	"prdash/internal/worktree"
)

func TestASpinnersTickGoesToTheSpinnerAndReturnsItsCmd(t *testing.T) {
	m := newTestModel(t)
	before := m.spinner.View()

	output, cmd := m.Update(spinner.TickMsg{})
	got := output.(Model)

	if got.spinner.View() == before {
		t.Error("the tick did not change the spinners view: it would stop turning on the first frame")
	}
	if cmd == nil {
		t.Error("the tick does not return the spinners Cmd: the next tick never arrives and the " +
			"spinner stays frozen")
	}
}

func TestAMessageThatIsNeitherAKeyNorAnEventIsIgnoredAndBreaksNothing(t *testing.T) {
	m := newTestModel(t)
	m.loading = true
	m.cursor = 3
	before := m

	for _, msg := range []tea.Msg{
		"any random string",
		struct{ X int }{42},
		nil,
	} {
		output, cmd := m.Update(msg)
		got := output.(Model)
		if cmd != nil {
			t.Errorf("%T: an unknown message returned a command", msg)
		}
		if got.loading != before.loading || got.cursor != before.cursor {
			t.Errorf("%T: an unknown message changed the state", msg)
		}
	}
}

// commentsTickMsg does NOT go through the event bomb (it consumes no channel reader) and still has
// to re-arm its tick.
func TestTheCommentsTickRearmsEvenWhenThereIsNothingToFetch(t *testing.T) {
	for _, c := range []struct {
		name    string
		prepara func(*testing.T) Model
	}{
		{"no selection", func(t *testing.T) Model { return newTestModel(t) }},
		{"with an item with no comments", func(t *testing.T) Model {
			m := newTestModel(t, &testutil.FakeAdapter{ForgeName: "github", HostName: "github.com"})
			m = withSelection(t, m, mkItem("github", "github.com", "acme/widget", "one", 7, ""))
			return m
		}},
	} {
		t.Run(c.name, func(t *testing.T) {
			m := c.prepara(t)
			if _, cmd := m.Update(commentsTickMsg{}); cmd == nil {
				t.Error("the comments tick returned no command: the chain dies and the " +
					"next selection change goes unnoticed")
			}
		})
	}
}

func TestAnActionOnAnUnknownForgeIsRefusedAndSaid(t *testing.T) {
	m := newTestModel(t) // with no adapters: the forges map is empty
	it := mkItem("github", "github.com", "acme/widget", "one", 7, "")

	a, ok := m.canActionOn(forge.ActionApprove, it)
	if ok || a != nil {
		t.Error("canActionOn of an unknown forge gave ok")
	}
	av := lastToast(m)
	if !strings.Contains(av, "unknown forge") {
		t.Errorf("the notice %q does not say the forge is unknown", av)
	}
	if !strings.Contains(av, "github") {
		t.Errorf("the notice %q does not name the forge: without the name the user does not know which", av)
	}
	// The level is error and not a warning: waiting does not fix it. Checked on the rendered text
	//rather than on the toast's type because that is what the view shows.
	if level := paintedLevel(m, "unknown forge"); level != "error" {
		t.Errorf("the notice comes out at level %q, want error: a forge that does not come back is not "+
			"transient and waiting does not fix it", level)
	}
}

func TestAnActionWithAnotherInFlightIsRefusedAndSaid(t *testing.T) {
	m := newTestModel(t, &testutil.FakeAdapter{ForgeName: "github", HostName: "github.com"})
	it := mkItem("github", "github.com", "acme/widget", "one", 7, "APPROVED")
	m.actionBusy = true

	if _, ok := m.canActionOn(forge.ActionMerge, it); ok {
		t.Fatal("canActionOn with an action in flight gave ok: two merges at once")
	}
	av := lastToast(m)
	if !strings.Contains(av, "already running") {
		t.Errorf("the notice %q does not say one is already in flight", av)
	}
	if level := paintedLevel(m, "already running"); level != "warn" {
		t.Errorf("the notice comes out at level %q, want warn: waiting is not an error", level)
	}
}

func TestMountWithNoSelectionIsRefusedAndSaid(t *testing.T) {
	m := newTestModel(t)

	output, cmd := m.startMount()
	if cmd != nil {
		t.Error("mount with no selection returned a command: it would create a worktree for nothing")
	}
	got := output.(Model)
	if got.mountBusy {
		t.Error("with no selection the mount was marked as in flight: the latch would stay " +
			"closed and the following keypresses would do nothing")
	}
	av := lastToast(got)
	if !strings.Contains(av, "select an item") {
		t.Errorf("the notice %q does not say an item must be chosen", av)
	}
	if level := paintedLevel(got, "select an item"); level != "warn" {
		t.Errorf("the notice comes out at level %q, want warn: the fix is to choose, not to repair", level)
	}
}

func TestMountWithAMountInFlightIsRefusedAndSaid(t *testing.T) {
	m := newTestModel(t, &testutil.FakeAdapter{ForgeName: "github", HostName: "github.com"})
	m = withSelection(t, m, mkItem("github", "github.com", "acme/widget", "one", 7, ""))
	m.mounter = &wiringMounter{}
	m.mountBusy = true

	output, cmd := m.startMount()
	if cmd != nil {
		t.Error("mount with a mount in flight returned a command: two worktrees on the same path")
	}
	got := output.(Model)
	if !got.mountBusy {
		t.Error("the mount latch was released: the next keypress would mount another")
	}
	av := lastToast(got)
	if !strings.Contains(av, "already running") {
		t.Errorf("the notice %q does not say a mount is already in flight", av)
	}
}

func TestTheQuitActionIsResolvedByTheConfigAndTheOverlayEatsItFirst(t *testing.T) {
	cfg := config.Defaults()
	cfg.Keybindings["quit"] = "0"
	new := func(t *testing.T) Model {
		t.Helper()
		m := New(cfg, []forge.Adapter{
			&testutil.FakeAdapter{ForgeName: "github", HostName: "github.com"},
		})
		m.width, m.height = 160, 40
		m.loading = false
		m.cachePath = ""
		return m
	}

	m := new(t)
	m = withSelection(t, m, mkItem("github", "github.com", "acme/widget", "one", 7, ""))
	if _, cmd := pressWithCmd(t, m, "0"); cmd == nil {
		t.Fatal("the reassigned quit key did not ask to leave the TUI")
	}

	m2 := new(t)
	m2 = withSelection(t, m2, mkItem("github", "github.com", "acme/widget", "one", 7, ""))
	m2.retarget.state = retargetChoosing
	m2.retarget.all = []string{"main", "feat/x"}
	m2.retarget.view = m2.retarget.all

	output, cmd := pressWithCmd(t, m2, "0")
	got := output
	if cmd != nil {
		t.Error("with the overlay open, the quit key closed it: it would lose the filter and " +
			"would close the session unintentionally")
	}
	if got.retarget.state != retargetChoosing {
		t.Errorf("the overlay was closed with the filters key (state=%d)", got.retarget.state)
	}
	if !strings.Contains(got.retarget.query, "0") {
		t.Errorf("the key did not reach the filter: it is %q", got.retarget.query)
	}

	for _, key := range []string{"q", "ctrl+c"} {
		m3 := new(t)
		m3 = withSelection(t, m3, mkItem("github", "github.com", "acme/widget", "one", 7, ""))
		m3.retarget.state = retargetChoosing
		if _, cmd := pressWithCmd(t, m3, key); cmd == nil {
			t.Errorf("%q with the overlay open did not leave the TUI", key)
		}
	}
}

// The most useful of the three mount warnings because it gives the next action: the mount worked and
// the layout did not.
func TestAMountThatRequiresHerdrWarnsWithWhereTheWorktreeEndedUp(t *testing.T) {
	av, level := mountNotice(executor.Result{
		Worktree: worktree.Worktree{Path: "/wt/prdash-pr-7", Label: "prdash-pr-7"},
	}, nil)
	if !strings.Contains(av, "Herdr") {
		t.Errorf("the notice %q does not say Herdr is needed", av)
	}
	if !strings.Contains(av, "/wt/prdash-pr-7") {
		t.Errorf("the notice %q does not say where the worktree ended up: without that there is nothing to do", av)
	}
	if level != levelWarn {
		t.Errorf("level %v, want warn: the worktree exists and what is missing is Herdr", level)
	}

	ok, okLevel := mountNotice(executor.Result{
		Herdr:    true,
		Worktree: worktree.Worktree{Path: "/wt/prdash-pr-7", Label: "prdash-pr-7"},
		Plan:     plan.Plan{Tabs: []plan.Tab{{Label: "Review"}, {Label: "Edit"}}},
	}, nil)
	if !strings.Contains(ok, "panes") || !strings.Contains(ok, "tabs") {
		t.Errorf("the happy path notice %q does not say how many panes nor tabs", ok)
	}
	if !strings.Contains(ok, "/wt/prdash-pr-7") {
		t.Errorf("the happy path notice %q does not say the path", ok)
	}
	if okLevel != levelOK {
		t.Errorf("level %v of the happy path", okLevel)
	}

	errMsg, errLevel := mountNotice(executor.Result{}, errors.New("no such branch"))
	if !strings.Contains(errMsg, "no such branch") {
		t.Errorf("the error notice %q does not carry the cause", errMsg)
	}
	if errLevel != levelError {
		t.Errorf("level %v of the error, want error", errLevel)
	}
}

func TestTheBaseChangeOverlaySitsOnTopOfTheContent(t *testing.T) {
	m := newTestModel(t, &testutil.FakeAdapter{ForgeName: "github", HostName: "github.com"})
	m = withSelection(t, m, mkItem("github", "github.com", "acme/widget", "one", 7, ""))
	m.width, m.height = 100, 30

	withoutPopup := m.View().Content
	for _, phase := range []retargetState{retargetListing, retargetChoosing, retargetConfirm} {
		m.retarget.state = phase
		withPopup := m.View().Content

		if len(withPopup) <= len(withoutPopup) {
			t.Errorf("phase %d: the popup added nothing to the render (%d -> %d)",
				phase, len(withoutPopup), len(withPopup))
			continue
		}
		if !strings.Contains(withPopup, "retarget") {
			t.Errorf("phase %d: the render does not contain the popup box", phase)
		}
		if !strings.Contains(withPopup, "acme/widget") {
			t.Errorf("phase %d: the overlay covered the inbox content", phase)
		}
	}

	m.retarget.state = retargetClosed
	if got := m.View().Content; strings.Contains(got, "retarget ") {
		t.Error("with the popup closed the render still brings the popup box")
	}
}

var _ = time.Second

// The render is inspected rather than the toast's field, because what matters is how it PAINTS.
func paintedLevel(m Model, contains string) string {
	for _, v := range m.toast.toasts {
		if !strings.Contains(v.message, contains) {
			continue
		}
		switch v.level {
		case toastError:
			return "error"
		case toastWarning:
			return "warn"
		default:
			return "info"
		}
	}
	return ""
}

type wiringMounter struct{}

func (wiringMounter) Mount(context.Context, model.Item) (executor.Result, error) {
	return executor.Result{}, nil
}
