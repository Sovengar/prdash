package herdr

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// A guard that never fires with a real plan, because the plan always brings a command.
func TestEmptyArgvIsRefusedBeforeCallingHerdr(t *testing.T) {
	f := &fakeCLI{env: map[string]string{"HERDR_ENV": "1"}}
	f.respond = func([]string) ([]byte, []byte, error) {
		return []byte(`{"id":"x","result":{"type":"ok"}}`), nil, nil
	}

	for _, argv := range [][]string{nil, {}} {
		err := f.client().PaneRun(context.Background(), "w1:p1", argv)
		if err == nil {
			t.Errorf("an argv of %v gave nil: a blank line would be sent to the pane's shell",
				argv)
			continue
		}
		if !strings.Contains(err.Error(), "argv") {
			t.Errorf("the error %q does not say the argv is empty", err)
		}
	}

	f2 := &fakeCLI{env: map[string]string{"HERDR_ENV": "1"}}
	f2.respond = func([]string) ([]byte, []byte, error) {
		return []byte(`{"id":"x","result":{"type":"ok"}}`), nil, nil
	}
	if err := f2.client().PaneRun(context.Background(), "w1:p1",
		[]string{"tuicr", "pr", "7"}); err != nil {
		t.Errorf("an argv with content gave an error: %v", err)
	}
	if !f2.called("pane", "run") {
		t.Error("an argv with content never reached the CLI: the guard is not what stopped it")
	}
	// The argv travels as ONE string, not split into arguments, and PaneRun does NOT quote them.
	argv := []string{"opencode", "run", "with spaces and quotes"}
	f3 := &fakeCLI{env: map[string]string{"HERDR_ENV": "1"}}
	f3.respond = func(args []string) ([]byte, []byte, error) {
		// The first call is `--version`, which leaves the guard when it asks Available().
		if len(args) == 0 || args[0] == "--version" {
			return []byte(`{"id":"x","result":{"type":"ok"}}`), nil, nil
		}
		joined := strings.Join(argv, " ")
		found := false
		for _, a := range args {
			if a == joined {
				found = true
			}
		}
		if !found {
			t.Errorf("the argv arrived split as %v: the phrase with spaces would have been lost", args)
		}
		return []byte(`{"id":"x","result":{"type":"ok"}}`), nil, nil
	}
	if err := f3.client().PaneRun(context.Background(), "w1:p1", argv); err != nil {
		t.Errorf("the argv with spaces gave an error: %v", err)
	}
}

// The failure you see as "no worktrees" instead of "the output was not the JSON we expected".
func TestOutputThatIsNotTheExpectedJSONDoesNotBecomeAnEmptyList(t *testing.T) {
	for _, c := range []struct {
		name   string
		output string
	}{
		{"proxy HTML", "<html>no</html>"},
		{"truncated JSON", `{"result":{"worktrees":[`},
		{"list instead of object", `[]`},
		{"empty", ""},
	} {
		f := &fakeCLI{env: map[string]string{"HERDR_ENV": "1"}}
		f.respond = func([]string) ([]byte, []byte, error) {
			return []byte(c.output), nil, nil
		}

		list, err := f.client().WorktreeList(context.Background(), "/repo")
		if err == nil {
			t.Errorf("%s: it gave nil, and it would be painted as \"there are no worktrees\"", c.name)
			continue
		}
		if list != nil {
			t.Errorf("%s: it returned %+v besides the error", c.name, list)
		}
	}
}

// The real binary has to run here, because the timeout branch is the one under test.
func TestWithoutTimeoutTheDefaultIsUsedAndWithTimeoutItReallyCuts(t *testing.T) {
	slow := filepath.Join(t.TempDir(), "herdr-lento")
	script := "#!/bin/sh\necho '{\"id\":\"x\",\"result\":{\"type\":\"ok\"}}'\n" +
		"sleep 30 &\nsleep 30\n"
	if err := os.WriteFile(slow, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}

	withTimeout := &Client{
		Bin:     slow,
		Timeout: 50 * time.Millisecond,
		getenv:  func(k string) string { return "1" },
	}
	start := time.Now()
	_, _, err := withTimeout.run(context.Background(), "pane", "list")
	took := time.Since(start)
	if err == nil {
		t.Error("a binary that stays alive gave nil with a timeout of 50ms")
	}
	if took > 3*time.Second {
		t.Errorf("the timeout took %s to cut: `WaitDelay` is not really cutting and "+
			"the process waits for the children that inherited the descriptors",
			took.Round(time.Millisecond))
	}

	// Timeout at ZERO uses DefaultTimeout (30s), and that is checked WITHOUT waiting 30s.
	fast := filepath.Join(t.TempDir(), "herdr-rapido")
	if err := os.WriteFile(fast,
		[]byte("#!/bin/sh\necho '{\"id\":\"x\",\"result\":{\"type\":\"ok\"}}'\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	withoutTimeout := &Client{Bin: fast, getenv: func(string) string { return "1" }}
	if withoutTimeout.Timeout != 0 {
		t.Fatal("the fixture is useless: the Client should keep the zero as it is")
	}
	// The default is applied INSIDE `run`, which is why the field cannot be checked directly.
	startZero := time.Now()
	if _, _, err := withoutTimeout.run(context.Background(), "pane", "list"); err != nil {
		t.Errorf("with Timeout at zero `run` failed: %v", err)
	}
	if took := time.Since(startZero); took > 5*time.Second {
		t.Errorf("with Timeout at zero `run` took %s: the default is not applied and the "+
			"process is left without a deadline", took.Round(time.Millisecond))
	}
}

// An empty Bin does NOT mean "run nothing": it means "use the name from the PATH".
func TestBlankBinaryIsLookedUpInPathAndGivesNoEmptyName(t *testing.T) {
	dir := t.TempDir()
	log := filepath.Join(dir, "args.log")
	script := "#!/bin/sh\necho \"$@\" > " + log + "\necho 'herdr 0.9.1'\n"
	if err := os.WriteFile(filepath.Join(dir, "herdr"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)

	// HERDR_BIN_PATH has to be left EMPTY, because defaultBin() asks it first.
	t.Setenv("HERDR_BIN_PATH", "")

	c := &Client{getenv: func(string) string { return "1" }}
	if c.Bin != "" {
		t.Fatal("the fixture is useless: the Client should start with an empty binary")
	}
	out, _, err := c.run(context.Background(), "--version")
	if err != nil {
		t.Fatalf("without Bin the herdr from the PATH was not found: %v", err)
	}
	if !strings.Contains(string(out), "0.9.1") {
		t.Errorf("the output is %q, not the PATH binary's one", out)
	}
	logged, err := os.ReadFile(log)
	if err != nil {
		t.Fatalf("the herdr from the PATH never got executed: %v", err)
	}
	if !strings.Contains(string(logged), "--version") {
		t.Errorf("the herdr from the PATH received %q", logged)
	}

	// With nothing in PATH the error stays readable: half the point of resolving the default.
	empty := t.TempDir()
	t.Setenv("PATH", empty)
	t.Setenv("HERDR_BIN_PATH", "")
	another := &Client{getenv: func(string) string { return "1" }}
	if _, _, err := another.run(context.Background(), "--version"); err == nil {
		t.Error("with no herdr in the PATH it gave nil")
	}
	// The error names the binary it looked for: "exec" with an empty name tells the user nothing.
	if err != nil && !strings.Contains(err.Error(), "herdr") {
		t.Errorf("the error %q does not name the missing binary", err)
	}
}
