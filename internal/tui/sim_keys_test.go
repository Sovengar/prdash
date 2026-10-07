package tui

import (
	"context"
	"strings"
	"testing"

	"prdash/internal/forge/model"
	"prdash/internal/sim"
	"prdash/internal/testutil"
)

type stubSimulator struct {
	available bool
	calls     int
}

func (s *stubSimulator) Available() bool { return s.available }

func (s *stubSimulator) Simulate(_ context.Context, _ model.Item, _ sim.Kind) (sim.Result, error) {
	s.calls++
	return sim.Result{}, nil
}

func modelInSim(t *testing.T, phase simState) (Model, *stubSimulator) {
	t.Helper()
	fake := &stubSimulator{available: true}
	m := newTestModel(t, &testutil.FakeAdapter{ForgeName: "github", HostName: "github.com"})
	m.simulator = fake
	m.sim.state = phase
	m.sim.item = mkItem("github", "github.com", "acme/widget", "something", 7, "")
	return m, fake
}

// The sweep is exhaustive on purpose: a transition table with a hole is a keypress that does nothing, with no error.
func TestEveryKeyInEveryPhaseClosesOrAdvancesAndAlwaysEndsOnSomethingClosable(t *testing.T) {
	keys := []string{
		"q", "ctrl+c", "esc", "o", "enter", "up", "down", "j", "k", "tab", "right", "left",
		"x", "space", "1", "?",
	}
	phases := []struct {
		name  string
		phase simState
	}{
		{"eligiendo", simChoosing},
		{"renderizando", simRendering},
		{"ensenando", simShowing},
	}

	for _, f := range phases {
		for _, key := range keys {
			m, _ := modelInSim(t, f.phase)
			got, _ := pressWithCmd(t, m, key)

			if esc, _ := pressWithCmd(t, m, "esc"); esc.sim.state != simClosed {
				t.Errorf("%s + %q: ESC did not close the popup (state=%d)",
					f.name, key, esc.sim.state)
			}

			// Closing leaves the panel EMPTY, with no image and no cells, so reopening does not show the previous simulation.
			advances := f.phase == simChoosing && key == "enter"
			if !advances && got.sim.state != simClosed {
				t.Errorf("%s + %q: did not close the popup (state=%d)", f.name, key, got.sim.state)
			}
			if got.sim.image != "" || len(got.sim.cells) != 0 {
				t.Errorf("%s + %q: popup residue was left: image=%q cells=%d",
					f.name, key, got.sim.image, len(got.sim.cells))
			}

			alreadyClosed := got
			alreadyClosed.closeSim()
			if alreadyClosed.sim.state != simClosed || alreadyClosed.sim.image != "" {
				t.Errorf("%s + %q: closing an already closed popup left residue", f.name, key)
			}
		}
	}
}

// The asymmetry is the point: esc closes the popup and stays in the TUI, q closes it and leaves.
func TestQAndQuitCloseAndCancelTheRenderAndAbortTheTUI(t *testing.T) {
	for _, key := range []string{"q", "ctrl+c"} {
		m, fake := modelInSim(t, simRendering)
		output, cmd := pressWithCmd(t, m, key)
		got := output

		if got.sim.state != simClosed {
			t.Errorf("%q: did not close the popup (state=%d)", key, got.sim.state)
		}
		if cmd == nil {
			t.Errorf("%q: did not return a command, so the TUI does not exit", key)
		}
		if got.simSeq == 0 {
			t.Errorf("%q: did not invalidate the render in flight (simSeq=%d)", key, got.simSeq)
		}
		if fake.calls != 0 {
			t.Errorf("%q: launched %d renders on its way out", key, fake.calls)
		}
	}
}

// My first version asserted something false: that esc keeps the render in flight and q invalidates it; both invalidate it.
func TestEscAndQuitDifferOnlyInThatQuitLeavesTheTUI(t *testing.T) {
	m, _ := modelInSim(t, simRendering)
	m.simSeq = 7

	for _, key := range []string{"esc", "q", "ctrl+c"} {
		got, cmd := pressWithCmd(t, m, key)
		if got.simSeq == 7 {
			t.Errorf("%q did not invalidate the render in flight: its image would appear over an "+
				"inbox the user has already gone back to working on", key)
		}
		if got.sim.state != simClosed {
			t.Errorf("%q did not close the popup", key)
		}
		sale := cmd != nil
		if sale != (key != "esc") {
			t.Errorf("%q: asks exit? = %v", key, sale)
		}
	}
}

// o is the only key that does NOT close: while choosing or rendering it closes, with an image it opens it.
func TestOOpensTheImageOnlyWhenThereIsOneAndClosesItOtherwise(t *testing.T) {
	m, _ := modelInSim(t, simShowing)
	m.sim.image = "/tmp/imagen-de-prueba.jpg"
	output, cmd := pressWithCmd(t, m, "o")
	got := output

	if cmd == nil {
		t.Error("o with an image returned no command: it would open nothing")
	}
	if got.sim.state == simClosed {
		t.Error("o with an image closed the popup: the popup closes when opening the viewer and the " +
			"image disappears from the view too early")
	}

	m2, _ := modelInSim(t, simShowing)
	m2.sim.image = ""
	out2, cmd2 := pressWithCmd(t, m2, "o")
	if out2.sim.state != simClosed {
		t.Error("o with no image did not close the popup")
	}
	if cmd2 != nil {
		t.Error("o with no image returned a command: it would run a viewer on an empty path")
	}

	for _, phase := range []simState{simChoosing, simRendering} {
		m3, _ := modelInSim(t, phase)
		out3, cmd3 := pressWithCmd(t, m3, "o")
		if out3.sim.state != simClosed {
			t.Errorf("o in phase %d did not close", phase)
		}
		if cmd3 != nil {
			t.Errorf("o in phase %d returned a command with no image", phase)
		}
	}
}

func TestEnterInTheChoicePhaseStartsTheRenderAndInTheOthersCloses(t *testing.T) {
	m, _ := modelInSim(t, simChoosing)
	m.sim.cursor = 0

	output, cmd := pressWithCmd(t, m, "enter")
	got := output

	if got.sim.state != simRendering {
		t.Fatalf("enter did not move to the rendering phase (state=%d)", got.sim.state)
	}
	// The command is nil on purpose: the render is a goroutine publishing on the events channel, not a tea.Cmd.
	if cmd != nil {
		t.Error("enter returned a tea.Cmd: the render would run in the update loop and freeze " +
			"the keyboard with the popup up")
	}
	if got.sim.kind != simKinds[0] {
		t.Errorf("enter rendered %q, and the cursor was at 0 (%q)", got.sim.kind, simKinds[0])
	}
	if got.sim.image != "" {
		t.Error("the panel kept the previous image when starting a new render")
	}

	for _, phase := range []simState{simRendering, simShowing} {
		m2, fake2 := modelInSim(t, phase)
		out2, cmd2 := pressWithCmd(t, m2, "enter")
		if out2.sim.state != simClosed {
			t.Errorf("enter in phase %d did not close", phase)
		}
		if cmd2 != nil {
			t.Errorf("enter in phase %d returned a command", phase)
		}
		if fake2.calls != 0 {
			t.Errorf("enter in phase %d rendered", phase)
		}
	}
}

// The `default` exists because an unforeseen key arrives, and a keypress that does nothing leaves the popup stuck.
func TestAKeyThatIsNotAMovementClosesThePopupAndDoesNotHangThere(t *testing.T) {
	for _, key := range []string{"x", "space", "1", "?", "F5", "ctrl+n"} {
		m, fake := modelInSim(t, simChoosing)
		output, cmd := pressWithCmd(t, m, key)
		got := output
		if got.sim.state != simClosed {
			t.Errorf("%q in the choice phase left the popup open", key)
		}
		if cmd != nil {
			t.Errorf("%q in the choice phase returned a command: closing launches nothing", key)
		}
		if fake.calls != 0 {
			t.Errorf("%q launched a render", key)
		}
	}
}

// A consequence of rebase being excluded: with a single strategy there is no menu, so enter must not swallow the choice.
func TestWithASingleStrategyTheArrowsDoNothingAndEnterDoesNotEatTheChoice(t *testing.T) {
	if len(simKinds) != 1 {
		t.Skipf("there are now %d strategies and this test is for the single one case", len(simKinds))
	}

	m, _ := modelInSim(t, simChoosing)
	for _, key := range []string{"up", "down", "left", "right", "j", "k", "tab"} {
		m2 := m
		if moved := m2.moveSimCursor(key); moved {
			t.Errorf("%q: moveSimCursor said it moved the cursor with a single strategy", key)
		}
	}
	if m.sim.cursor != 0 {
		t.Errorf("the cursor ended at %d with a single strategy", m.sim.cursor)
	}
	if pressModel(t, m, "enter").sim.state != simRendering {
		t.Error("enter does not launch the render with a single strategy")
	}
}

func TestOpeningTheSimulatorDeniesTheThreeThingsAndNamesThem(t *testing.T) {
	m, _ := modelInSim(t, simClosed)
	m.simulator = &stubSimulator{available: false}
	m = withSelection(t, m, mkItem("github", "github.com", "acme/widget", "something", 7, ""))
	if _, cmd := m.openSimulator(); cmd != nil {
		t.Error("without git-sim it returned a command")
	}
	if av := lastToast(m); !strings.Contains(av, "git-sim") {
		t.Errorf("without git-sim it does not warn about git-sim: %q", av)
	}

	m2, _ := modelInSim(t, simClosed)
	if _, cmd := m2.openSimulator(); cmd != nil {
		t.Error("with no selection it returned a command")
	}
	if av := lastToast(m2); !strings.Contains(av, "select an item") {
		t.Errorf("with no selection it does not warn: %q", av)
	}

	m3, _ := modelInSim(t, simClosed)
	withoutBase := mkItem("github", "github.com", "acme/widget", "something", 7, "")
	withoutBase.TargetBranch = ""
	m3 = withSelection(t, m3, withoutBase)
	if _, cmd := m3.openSimulator(); cmd != nil {
		t.Error("with no target branch it returned a command")
	}
	av := lastToast(m3)
	if !strings.Contains(av, "target branch") {
		t.Errorf("with no target branch it does not warn about that: %q", av)
	}
	if !strings.Contains(av, refLabel(withoutBase)) {
		t.Errorf("the notice does not name the item: %q", av)
	}

	m4, _ := modelInSim(t, simClosed)
	withSpaces := mkItem("github", "github.com", "acme/widget", "something", 7, "")
	withSpaces.TargetBranch = "   "
	m4 = withSelection(t, m4, withSpaces)
	if _, cmd := m4.openSimulator(); cmd != nil {
		t.Error("with a target branch of only spaces it returned a command")
	}
	if av := lastToast(m4); !strings.Contains(av, "target branch") {
		t.Errorf("a target branch of spaces does not count as empty: %q", av)
	}
}

// It enters through a pageMsg rather than withItems+rebuild, because selected() reads m.rows(), the VISIBLE section.
func withSelection(t *testing.T, m Model, it model.Item) Model {
	t.Helper()
	return send(t, m,
		page(m.cycle, it.Forge, it.Ref.Host, model.SectionReview, model.ReviewRequested,
			[]model.Item{it}, false))
}

func pressModel(t *testing.T, m Model, key string) Model {
	t.Helper()
	got, _ := pressWithCmd(t, m, key)
	return got
}
