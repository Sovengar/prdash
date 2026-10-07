package herdr

import (
	"errors"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"testing"
)

// A real process is spawned because *exec.ExitError only carries the code.
func exitErrorFor(t *testing.T, code int) error {
	t.Helper()
	cmd := exec.Command(os.Args[0], "-test.run=TestHelperProcessExitsWithCode")
	cmd.Env = append(os.Environ(), "PRDASH_TEST_RUN_HELPER=1", "PRDASH_TEST_EXIT_CODE="+strconv.Itoa(code))
	err := cmd.Run()
	if err == nil {
		t.Fatalf("the helper process exited cleanly instead of with %d", code)
	}
	var exit *exec.ExitError
	if !errors.As(err, &exit) {
		t.Fatalf("the helper process gave %T instead of an *exec.ExitError: %v", err, err)
	}
	return err
}

func TestHelperProcessExitsWithCode(t *testing.T) {
	if os.Getenv("PRDASH_TEST_RUN_HELPER") == "" {
		t.Skip("only runs as a helper process")
	}
	code, err := strconv.Atoi(os.Getenv("PRDASH_TEST_EXIT_CODE"))
	if err != nil || code == 0 {
		os.Exit(1)
	}
	os.Exit(code)
}

func TestNewErrorTakesCodeAndMessageFromTheServer(t *testing.T) {
	cases := []struct {
		name     string
		stderr   string
		wantCode string
		wantMsg  string
	}{
		{
			"nested code and message",
			`{"error":{"code":"E_NOPE","message":"branch does not exist"}}`,
			"E_NOPE", "branch does not exist",
		},
		{
			"flat code and message",
			`{"code":"E_PLAN","message":"the plan does not fit"}`,
			"E_PLAN", "the plan does not fit",
		},
		{
			"code only",
			`{"error":{"code":"E_SOLO"}}`,
			"E_SOLO", "exit status 3",
		},
		{
			"flat code only",
			`{"code":"E_SOLO"}`,
			"E_SOLO", "exit status 3",
		},
		{
			"message only",
			`{"error":{"message":"algo falló"}}`,
			"", "algo falló",
		},
		{
			"flat message only",
			`{"message":"algo falló"}`,
			"", "algo falló",
		},
		// An empty message must NOT overwrite the one from the error.
		{
			"code with empty message",
			`{"error":{"code":"E_VACIO","message":""}}`,
			"E_VACIO", "exit status 3",
		},
		{
			"foreign JSON",
			`{"other":"cosa"}`,
			"", `{"other":"cosa"}`,
		},
		{
			"not JSON",
			"panic: something broke",
			"", "panic: something broke",
		},
		{
			"empty stderr",
			"",
			"", "exit status 3",
		},
	}

	for _, c := range cases {
		err := errors.New("exit status 3")
		e := newError([]string{"herdr", "tab", "create"}, err, []byte(c.stderr))

		if e.Code != c.wantCode {
			t.Errorf("%s: the code ended up as %q, want %q", c.name, e.Code, c.wantCode)
		}
		if e.Msg != c.wantMsg {
			t.Errorf("%s: the message ended up as %q, want %q", c.name, e.Msg, c.wantMsg)
		}
		// The message is NEVER empty: an error with no message says nothing.
		if e.Msg == "" {
			t.Errorf("%s: the message ended up empty: an error with no text reports nothing", c.name)
		}
		// The original error is kept, so the stack can be unwound.
		if !errors.Is(e.Err, err) {
			t.Errorf("%s: the original error was not kept", c.name)
		}
		if strings.Join(e.Args, " ") != "herdr tab create" {
			t.Errorf("%s: the args ended up as %q", c.name, e.Args)
		}
	}
}

func TestNewErrorTakesExitCodeOnlyWhenItHasOne(t *testing.T) {
	e := newError([]string{"herdr"}, exitErrorFor(t, 3), nil)
	if e.Exit != 3 {
		t.Errorf("with an ExitError(3) it ended up as Exit=%d, want 3", e.Exit)
	}
	if e.Msg != "exit status 3" {
		t.Errorf("the message ended up as %q", e.Msg)
	}

	normal := errors.New("exec: \"herdr\": executable file not found in $PATH")
	e = newError([]string{"herdr"}, normal, nil)
	if e.Exit != 0 {
		t.Errorf("with an ordinary error it ended up as Exit=%d, want 0: there is no exit code to read", e.Exit)
	}
	if e.Msg != normal.Error() {
		t.Errorf("the message ended up as %q, want the error's one: %q", e.Msg, normal.Error())
	}
	// The message of an ordinary error is the true one ("not in PATH"), not a fabricated text.
	if strings.Contains(e.Msg, "exit status") {
		t.Errorf("an error that is not from a process gave %q: an exit code was invented", e.Msg)
	}
}

func TestNewErrorKeepsOnlyTheFirstStderrLine(t *testing.T) {
	stderr := "error: no such workspace\ngoroutine 1 [running]:\n\therdr/main.go:42"
	e := newError([]string{"herdr"}, errors.New("exit status 1"), []byte(stderr))

	if e.Msg != "error: no such workspace" {
		t.Errorf("the message ended up as %q, want only the first line", e.Msg)
	}
	if strings.Contains(e.Msg, "goroutine") {
		t.Errorf("the message swallowed the stack: %q", e.Msg)
	}
}

func TestErrorMessageNeverEndsUpEmpty(t *testing.T) {
	for _, stderr := range []string{
		"", "   ", "\n\n", "algo", "{}", `{"error":{}}`, `{"code":"","message":""}`,
	} {
		for _, err := range []error{errors.New("exit status 1"), exitErrorFor(t, 7)} {
			e := newError([]string{"herdr"}, err, []byte(stderr))
			if strings.TrimSpace(e.Msg) == "" {
				t.Errorf("stderr=%q err=%v: the message ended up empty", stderr, err)
			}
		}
	}
}
