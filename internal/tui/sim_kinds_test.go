package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"prdash/internal/sim"
)

// offeringSimulator adds the optional capability report to the usual fake.
type offeringSimulator struct {
	*fakeSimulator
	offered []sim.Kind
}

func (s offeringSimulator) Kinds() []sim.Kind { return s.offered }

func TestTheChooserOffersRebaseWhenGitSimSupportsIt(t *testing.T) {
	f := offeringSimulator{
		fakeSimulator: &fakeSimulator{available: true},
		offered:       []sim.Kind{sim.KindMerge, sim.KindRebase},
	}
	m := simModel(t, f)
	m = press(t, m, "v")

	if len(m.sim.kinds) != 2 || m.sim.kinds[1] != sim.KindRebase {
		t.Fatalf("kinds = %v, want merge and rebase", m.sim.kinds)
	}
	if text := viewText(m); !strings.Contains(text, "rebase") {
		t.Errorf("the selector does not offer rebase:\n%s", text)
	}
}

func TestEnterRendersTheRebaseWhenThatIsTheChoice(t *testing.T) {
	f := offeringSimulator{
		fakeSimulator: &fakeSimulator{available: true},
		offered:       []sim.Kind{sim.KindMerge, sim.KindRebase},
	}
	m := simModel(t, f)
	m = press(t, m, "v")
	m = press(t, m, "j")
	m = press(t, m, "enter")

	if m.sim.kind != sim.KindRebase {
		t.Errorf("kind = %q, want rebase", m.sim.kind)
	}
}

func TestAnEmptyCapabilityReportFallsBackToTheDefaultList(t *testing.T) {
	f := offeringSimulator{fakeSimulator: &fakeSimulator{available: true}}
	m := simModel(t, f)
	m = press(t, m, "v")

	if len(m.sim.kinds) != 1 || m.sim.kinds[0] != sim.KindMerge {
		t.Errorf("kinds = %v, want the default merge only", m.sim.kinds)
	}
}

// Panels opened directly by a test carry no list; the cursor must not index a nil slice.
func TestAPanelWithoutAListUsesTheDefaultKinds(t *testing.T) {
	m, _ := modelInSim(t, simChoosing)

	if got := m.simKindsNow(); len(got) != len(simKinds) || got[0] != simKinds[0] {
		t.Errorf("simKindsNow = %v, want %v", got, simKinds)
	}
	got, cmd := pressWithCmd(t, m, "enter")
	if cmd != nil || got.sim.state != simRendering {
		t.Errorf("enter on a panel without a list left state = %d", got.sim.state)
	}
}

func htmlPage(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "sim.html")
	if err := os.WriteFile(path, []byte("<html></html>"), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func openerStub(t *testing.T, name, arg string) (bin, log string) {
	t.Helper()
	bin, log = t.TempDir(), filepath.Join(t.TempDir(), "opened.log")
	script := "#!/bin/sh\necho \"$" + arg + "\" >> \"" + log + "\"\n"
	if err := os.WriteFile(filepath.Join(bin, name), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return bin, log
}

func waitOpened(t *testing.T, log, want string) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if data, err := os.ReadFile(log); err == nil && strings.Contains(string(data), want) {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	data, _ := os.ReadFile(log)
	t.Fatalf("opener got %q, want %q", data, want)
}

func TestOOpensThePageWithTerminalBrowser(t *testing.T) {
	page := htmlPage(t)
	bin, log := openerStub(t, "terminal-browser", "2")
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))

	m := simModel(t, &fakeSimulator{available: true})
	m.sim.state = simShowing
	m.sim.image = page
	m.sim.img = nil

	_, cmd := pressWithCmd(t, m, "o")
	if cmd == nil {
		t.Fatal("o returned no command")
	}
	if msg, ok := cmd().(notifyMsg); !ok || !strings.Contains(msg.text, "opened") {
		t.Errorf("o answered %#v, want an opened notice", msg)
	}
	waitOpened(t, log, page)
}

func TestOWithoutTerminalBrowserFallsBackToTheDesktop(t *testing.T) {
	page := htmlPage(t)
	bin, log := openerStub(t, "xdg-open", "1")
	t.Setenv("PATH", bin)

	m := simModel(t, &fakeSimulator{available: true})
	m.sim.state = simShowing
	m.sim.image = page
	m.sim.img = nil

	_, cmd := pressWithCmd(t, m, "o")
	if cmd == nil {
		t.Fatal("o returned no command")
	}
	_ = cmd()
	waitOpened(t, log, page)
}

func TestOWithoutAnyOpenerWarns(t *testing.T) {
	page := htmlPage(t)
	t.Setenv("PATH", t.TempDir())

	m := simModel(t, &fakeSimulator{available: true})
	m.sim.state = simShowing
	m.sim.image = page
	m.sim.img = nil

	_, cmd := pressWithCmd(t, m, "o")
	if cmd == nil {
		t.Fatal("o returned no command")
	}
	msg, ok := cmd().(notifyMsg)
	if !ok || !strings.Contains(msg.text, "simulate:") {
		t.Errorf("o answered %#v, want a warning naming the failure", msg)
	}
}

func TestAFailingTerminalBrowserStartsNothingAndWarns(t *testing.T) {
	page := htmlPage(t)
	bin := t.TempDir()
	if err := os.WriteFile(filepath.Join(bin, "terminal-browser"), []byte("no shebang"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin)

	m := simModel(t, &fakeSimulator{available: true})
	m.sim.state = simShowing
	m.sim.image = page
	m.sim.img = nil

	_, cmd := pressWithCmd(t, m, "o")
	msg, ok := cmd().(notifyMsg)
	if !ok || !strings.Contains(msg.text, "simulate:") {
		t.Errorf("o answered %#v, want a warning", msg)
	}
}
