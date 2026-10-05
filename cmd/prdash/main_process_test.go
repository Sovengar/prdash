package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

const mainInSubprocess = "PRDASH_TEST_MAIN"

// The args go in the ENVIRONMENT, not the command line, because -test.run would be read as a
// prdash flag.
const argsInSubprocess = "PRDASH_TEST_ARGS"

func TestRealMainExitsWithTheCodeMatchingTheArgs(t *testing.T) {
	// Its own HOME and XDG, so the subprocess does not read the config of whoever runs the tests.
	base := t.TempDir()

	for _, c := range []struct {
		name string
		args []string
		code int
		want string
	}{
		{
			name: "the TUI without a terminal warns and exits with 1",
			args: nil,
			code: 1,
			want: "prdash:",
		},
		{
			name: "print mode exits with 0",
			args: []string{"--print"},
			code: 0,
			want: "",
		},
		{
			name: "an unknown flag is a usage error",
			args: []string{"--does-not-exist"},
			code: 2,
			want: "",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			output, code := mainAsSubprocess(t, base, c.args...)

			if code != c.code {
				t.Errorf("exit code is %d, want %d.\n%s", code, c.code, output)
			}
			if c.want != "" && !strings.Contains(output, c.want) {
				t.Errorf("the output does not contain %q:\n%s", c.want, output)
			}
			// The subprocess must NOT die of a panic: that leaves half the output in stderr and the exit
			// code says nothing.
			if strings.Contains(output, "panic:") || strings.Contains(output, "goroutine ") {
				t.Errorf("the subprocess panicked:\n%s", output)
			}
		})
	}
}

// This is why the previous test exists: run returns 0 at the end of the happy path.
func TestMainWithoutTerminalLeavesNoHungProcessAndDoesNotExitZero(t *testing.T) {
	base := t.TempDir()
	output, code := mainAsSubprocess(t, base)

	if code == 0 {
		t.Errorf("without a terminal it exited with 0.\n%s", output)
	}
	// The warning says something: an error with no message is a failure that cannot be diagnosed.
	if strings.TrimSpace(output) == "" {
		t.Error("it failed without saying anything: in a CI log that is an undiagnosable failure")
	}
	// Not a usage error —that would be a 2 with the flag's text— but a session failure.
	if strings.Contains(output, "uso:") || strings.Contains(output, "usage") {
		t.Errorf("the failure looks like a usage error and not an environment one:\n%s", output)
	}
}

func mainAsSubprocess(t *testing.T, base string, args ...string) (string, int) {
	t.Helper()

	// prdash's args do NOT go on the command line; os.Args is rewritten in the child before main.
	cmd := exec.Command(os.Args[0], "-test.run=^TestThatRunsTheRealMain$")

	cmd.Env = append(os.Environ(),
		mainInSubprocess+"=1",
		argsInSubprocess+"="+strings.Join(args, "\x00"),
		"HOME="+base,
		"XDG_CONFIG_HOME="+filepath.Join(base, "config"),
		"XDG_CACHE_HOME="+filepath.Join(base, "cache"),
		"XDG_DATA_HOME="+filepath.Join(base, "data"),
		"PATH="+os.Getenv("PATH"),
	)
	cmd.Stdin = nil

	out, err := cmd.CombinedOutput()
	code := 0
	if err != nil {
		var exitErr *exec.ExitError
		if ok := asExitError(err, &exitErr); ok {
			code = exitErr.ExitCode()
		} else {
			t.Fatalf("could not run the subprocess: %v", err)
		}
	}
	return string(out), code
}

// It does NOT run in the normal pass.
func TestThatRunsTheRealMain(t *testing.T) {
	if os.Getenv(mainInSubprocess) == "1" {
		os.Args = append([]string{filepath.Base(os.Args[0])},
			strings.Split(os.Getenv(argsInSubprocess), "\x00")...)
		main()
		os.Exit(99)
	}
	t.Skip("this only runs in the subprocess that calls main")
}

func asExitError(err error, dst **exec.ExitError) bool {
	e, ok := err.(*exec.ExitError)
	if ok {
		*dst = e
	}
	return ok
}

// Both halves of the same if, and the missing one was the "started" half: with a terminal-less
// subprocess only the failure was reachable.
func TestTheTUIThatStartsExitsZeroAndTheOneThatFailsExitsOne(t *testing.T) {
	xdg := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", xdg)
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())

	original := startTUI
	t.Cleanup(func() { startTUI = original })

	for _, c := range []struct {
		name    string
		failErr error
		code    int
	}{
		{"the TUI starts and the user quits", nil, 0},
		{"the TUI cannot start", startError{}, 1},
	} {
		t.Run(c.name, func(t *testing.T) {
			startTUI = func(tea.Model) error { return c.failErr }

			var stdout, stderr bytes.Buffer
			code := run(nil, &stdout, &stderr)

			if code != c.code {
				t.Errorf("code = %d, want %d.\nstdout: %s\nstderr: %s",
					code, c.code, stdout.String(), stderr.String())
			}
			// The warning only when there was an error: with the TUI alive there is nothing to warn about.
			if c.failErr == nil && strings.TrimSpace(stderr.String()) != "" {
				t.Errorf("with the TUI alive it warned: %q", stderr.String())
			}
			if c.failErr != nil {
				if !strings.Contains(stderr.String(), "prdash:") {
					t.Errorf("stderr = %q, want the warning with the program prefix", stderr.String())
				}
				if !strings.Contains(stderr.String(), c.failErr.Error()) {
					t.Errorf("stderr = %q, it does not carry the cause of the failure", stderr.String())
				}
			}
		})
	}
}

type startError struct{}

func (startError) Error() string { return "no terminal" }
