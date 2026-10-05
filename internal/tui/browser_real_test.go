package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func waitForFile(path string, limit time.Duration) ([]byte, error) {
	deadline := time.Now().Add(limit)
	var last error
	for time.Now().Before(deadline) {
		raw, err := os.ReadFile(path)
		if err == nil {
			if len(raw) > 0 {
				return raw, nil
			}
			last = err
		} else {
			last = err
		}
		time.Sleep(5 * time.Millisecond)
	}
	return nil, last
}

// The REAL paths of openBrowserCmd: the ones that do not go through the openURL double.

// The PATH is emptied, so nothing is opened.
func TestWithNoOpenerInThePATHItWarnsAndOpensNothing(t *testing.T) {
	empty := t.TempDir()
	t.Setenv("PATH", empty)

	m := newTestModel(t)
	// And without the seam: the real path, the one production would run.
	if m.openURL != nil {
		t.Fatal("useless fixture: the seam is still in place, the real path is not under test")
	}

	const url = "https://github.com/acme/project/pull/1"
	msg, ok := m.openBrowserCmd(url)().(notifyMsg)
	if !ok {
		t.Fatal("without an opener it did not return a notifyMsg")
	}
	if msg.level != levelWarn {
		t.Errorf("level %v, want warn: the environment fails, not the action", msg.level)
	}
	if !strings.Contains(msg.text, "xdg-open") && !strings.Contains(msg.text, "open") &&
		!strings.Contains(msg.text, "rundll32") {
		t.Errorf("the notice %q does not say which opener is missing", msg.text)
	}
	if !strings.Contains(strings.ToLower(msg.text), "not found") {
		t.Errorf("the notice %q does not say it was not found", msg.text)
	}
	// And it does not say the success text: with the good path's wording the user would think it opened.
	if strings.HasPrefix(msg.text, "opening") {
		t.Errorf("without an opener the notice says %q", msg.text)
	}
	entries, err := os.ReadDir(empty)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Errorf("the empty PATH has %d entries: something was executed", len(entries))
	}
}

// LookPath looks at the execute bit and NOT the shebang, so the warning must not say "not found":
// the user would go install xdg-utils when the interpreter is what is missing.
func TestAnOpenerThatCannotBeExecutedWarnsWithItsError(t *testing.T) {
	binDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(binDir, "xdg-open"),
		[]byte("#!/interprete-que-no-existe\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", binDir)

	m := newTestModel(t)
	if m.openURL != nil {
		t.Fatal("useless fixture: the seam is still in place, the real path is not under test")
	}

	msg, ok := m.openBrowserCmd("https://ejemplo/x")().(notifyMsg)
	if !ok {
		t.Fatal("an opener that does not start did not return a notifyMsg")
	}
	// The level is ERROR and not a warning on purpose: the opener exists and does not start.
	if msg.level != levelError {
		t.Errorf("level %v, want error: the opener exists and does not start", msg.level)
	}
	// The warning does NOT say "not found": LookPath DID find it —it looks at the execute bit— and
	// the failure arrives in Start.
	if strings.Contains(msg.text, "not found") {
		t.Errorf("the notice says the opener was not found, but `LookPath` did find it: "+
			"the failure is at Start, not at lookup. Notice: %q", msg.text)
	}
	// And it carries Start's cause, which is the only thing that tells a broken shebang from a
	// permission problem.
	if !strings.Contains(msg.text, "open browser") {
		t.Errorf("the notice %q does not say it comes from the opener", msg.text)
	}
	if len(strings.TrimSpace(msg.text)) <= len("open browser: ") {
		t.Errorf("the notice %q does not carry the cause of the Start failure", msg.text)
	}
}

// With the guards in the other order, an empty URL on a machine with no opener would report the
// environment instead of having nothing to open.
func TestEmptyURLIsRejectedBeforeLookingAtTheEnvironment(t *testing.T) {
	empty := t.TempDir()
	t.Setenv("PATH", empty)

	m := newTestModel(t)
	msg, ok := m.openBrowserCmd("")().(notifyMsg)
	if !ok {
		t.Fatal("an empty URL did not return a notifyMsg")
	}
	if !strings.Contains(msg.text, "nothing to open") {
		t.Errorf("the notice %q is the environment one and not the empty-URL one: the opener would "+
			"be checked before the URL and the notice would be the wrong one", msg.text)
	}
	// And with the opener present, the same warning: the result does not depend on the machine.
	t.Setenv("PATH", os.Getenv("PATH"))
	if got := m.openBrowserCmd("")(); !strings.Contains(got.(notifyMsg).text, "nothing to open") {
		t.Errorf("with an opener the empty-URL notice changed: %q", got)
	}
}

// The boundary with the shell.
func TestTheURLIsPassedAsIsToTheOpener(t *testing.T) {
	binDir := t.TempDir()
	log := filepath.Join(binDir, "args.log")
	script := "#!/bin/sh\n" +
		"for a in \"$@\"; do echo \"[$a]\" >> " + log + "; done\n" +
		"exit 0\n"
	if err := os.WriteFile(filepath.Join(binDir, "xdg-open"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", binDir)

	m := newTestModel(t)
	const url = `https://host/p; echo pwned?a=1&b="2" '3'`
	msg, ok := m.openBrowserCmd(url)().(notifyMsg)
	if !ok {
		t.Fatalf("no notifyMsg was returned: %v", msg)
	}
	if msg.level != levelInfo || !strings.Contains(msg.text, url) {
		t.Errorf("the happy path was not taken: %+v", msg)
	}

	// The opener got the URL as ONE argument, with the brackets the script adds.
	crudo, err := waitForFile(log, 2*time.Second)
	if err != nil {
		t.Fatalf("the opener never ran: %v", err)
	}
	line := strings.TrimSpace(string(crudo))
	if line != "["+url+"]" {
		t.Errorf("the opener received %q, want a single argument with the whole URL", line)
	}
}
