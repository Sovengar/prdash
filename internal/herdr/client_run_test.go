package herdr

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestRunExecutesTheRealBinary(t *testing.T) {
	dir := t.TempDir()
	log := filepath.Join(dir, "args.log")

	script := "#!/bin/sh\n" +
		"echo \"$@\" >> " + log + "\n" +
		"echo 'esto es stdout'\n" +
		"echo 'esto es stderr' >&2\n" +
		"exit ${FAKE_RC:-0}\n"
	bin := writeBinary(t, dir, "herdr-falso", script)

	c := &Client{Bin: bin, getenv: func(string) string { return "1" }}

	out, errb, err := c.run(context.Background(), "pane", "list", "--json")
	if err != nil {
		t.Fatalf("run returned an error: %v", err)
	}
	if !strings.Contains(string(out), "esto es stdout") {
		t.Errorf("stdout came out as %q", out)
	}
	if !strings.Contains(string(errb), "esto es stderr") {
		t.Errorf("stderr came out as %q", errb)
	}
	// stdout and stderr are NOT mixed: mixed, a CLI warning would look like a successful answer.
	if strings.Contains(string(out), "stderr") {
		t.Errorf("stderr got into stdout: %q", out)
	}

	raw, err := os.ReadFile(log)
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.TrimSpace(string(raw)); got != "pane list --json" {
		t.Errorf("the binary received %q, want %q", got, "pane list --json")
	}

	t.Setenv("FAKE_RC", "3")
	if _, _, err := c.run(context.Background(), "pane", "list"); err == nil {
		t.Error("a binary exiting with code 3 gave no error")
	}
}

func TestDefaultBinaryComesFromTheEnvironmentNotTheField(t *testing.T) {
	t.Setenv("HERDR_BIN_PATH", "")
	if got := defaultBin(); got != "herdr" {
		t.Errorf("without HERDR_BIN_PATH it returned %q, want herdr", got)
	}

	t.Setenv("HERDR_BIN_PATH", "/opt/herdr/bin/herdr")
	if got := defaultBin(); got != "/opt/herdr/bin/herdr" {
		t.Errorf("with HERDR_BIN_PATH it returned %q, want the path from the variable", got)
	}

	c := New()
	if c.Bin != "/opt/herdr/bin/herdr" {
		t.Errorf("New().Bin = %q, want the HERDR_BIN_PATH route", c.Bin)
	}
	if c.getenv == nil {
		t.Error("New() left getenv as nil, so env() falls back to os.Getenv instead of " +
			"using its own reader")
	}
}

// The ONLY read of HERDR_ENV in the whole program.
func TestInHerdrOnlyReadsTheVariable(t *testing.T) {
	cases := []struct {
		value string
		want  bool
		note  string
	}{
		{"1", true, "the exact value"},
		{"", false, "no variable"},
		{"0", false, "zero is not one"},
		{"true", false, `"true" is not "1": the case a boolean Parse would swallow`},
		{"yes", false, "same with yes"},
		{"11", false, "starts with one but is not"},
		{" 1", false, "with a space in front it is not"},
	}
	for _, c := range cases {
		t.Setenv("HERDR_ENV", c.value)
		if got := InHerdr(); got != c.want {
			t.Errorf("HERDR_ENV=%q gave %v, want %v. %s", c.value, got, c.want, c.note)
		}
	}
}

func TestMutatingOperationsAreVetoedOutsideHerdr(t *testing.T) {
	calls := 0
	seen := [][]string{}
	c := &Client{
		Bin:    "must-not-run",
		getenv: func(k string) string { return "" },
		execFn: func(context.Context, ...string) ([]byte, []byte, error) {
			calls++
			return nil, nil, nil
		},
	}

	if err := c.WorktreeRemove(context.Background(), "w1", true); err == nil {
		t.Error("worktree remove outside Herdr gave no error")
	}
	if err := c.WorkspaceClose(context.Background(), "w1", false); err == nil {
		t.Error("workspace close outside Herdr gave no error")
	}
	if err := c.PaneFocus(context.Background(), "left"); err == nil {
		t.Error("pane focus outside Herdr gave no error")
	}
	if calls != 0 {
		t.Errorf("%d calls to the binary ran while outside Herdr: the veto has to go "+
			"BEFORE reaching out to the CLI, not after", calls)
	}

	inside := &Client{
		Bin: "herdr",
		getenv: func(k string) string {
			if k == "HERDR_ENV" {
				return "1"
			}
			return ""
		},
		execFn: func(_ context.Context, args ...string) ([]byte, []byte, error) {
			seen = append(seen, args)
			return []byte(`{"id":"cli:ok","result":{"type":"ok"}}`), nil, nil
		},
	}
	if err := inside.PaneFocus(context.Background(), "left"); err != nil {
		t.Errorf("inside Herdr, pane focus failed: %v", err)
	}
	// Counting carefully is the trap: the guard's Available() is also a call to the binary.
	if len(seen) != 2 {
		t.Errorf("inside Herdr %d calls were made, want 2 (version + operation): "+
			"%v", len(seen), seen)
	}
	if len(seen) == 2 && !strings.Contains(strings.Join(seen[1], " "), "focus") {
		t.Errorf("the second call was %v and is not the operation", seen[1])
	}
}

func TestPaneFocusWithoutDirectionGoesRight(t *testing.T) {
	var seen [][]string
	newClient := func() *Client {
		return &Client{
			Bin:    "herdr",
			getenv: func(k string) string { return "1" },
			execFn: func(_ context.Context, args ...string) ([]byte, []byte, error) {
				seen = append(seen, args)
				return []byte(`{"id":"cli:ok","result":{"type":"ok"}}`), nil, nil
			},
		}
	}

	if err := newClient().PaneFocus(context.Background(), ""); err != nil {
		t.Fatalf("pane focus without direction: %v", err)
	}
	if !containsArg(seen, "right") {
		t.Errorf("without a direction %v was sent and none carries right", seen)
	}

	seen = nil
	if err := newClient().PaneFocus(context.Background(), "up"); err != nil {
		t.Fatalf("pane focus with direction: %v", err)
	}
	if !containsArg(seen, "up") {
		t.Errorf("with direction up %v was sent and none carries up", seen)
	}
}

func TestRunTimeoutApplies(t *testing.T) {
	dir := t.TempDir()
	bin := writeBinary(t, dir, "herdr-colgado", "#!/bin/sh\nsleep 5\n")

	c := &Client{Bin: bin, getenv: func(string) string { return "1" }, Timeout: 50 * time.Millisecond}

	start := time.Now()
	_, _, err := c.run(context.Background(), "pane", "list")
	elapsed := time.Since(start)

	if err == nil {
		t.Error("a hung binary gave no error")
	}
	if elapsed > 2*time.Second {
		t.Errorf("run took %s with a timeout of 50ms: the context is not cutting the "+
			"call", elapsed)
	}
	// The default floor is proved by what run computes, not by waiting 30s.
	bin1s := writeBinary(t, dir, "herdr-corto", "#!/bin/sh\nsleep 1\n")
	c.Bin = bin1s
	c.Timeout = 0
	start = time.Now()
	if _, _, err := c.run(context.Background(), "pane", "list"); err != nil {
		t.Errorf("with Timeout at zero and a binary that sleeps 1s it gave an error: %v", err)
	}
	if elapsed := time.Since(start); elapsed < 900*time.Millisecond {
		t.Errorf("with Timeout at zero run took %s: the 50ms timeout of the previous "+
			"case was applied instead of the default floor", elapsed)
	}
}

// containsArg looks through ALL the calls, because the guard's first one is `--version`.
func containsArg(calls [][]string, arg string) bool {
	for _, c := range calls {
		for _, a := range c {
			if a == arg {
				return true
			}
		}
	}
	return false
}

func writeBinary(t *testing.T, dir, name, body string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(body), 0o755); err != nil {
		t.Fatalf("writing %s: %v", path, err)
	}
	return path
}
