package tool

import (
	"context"
	"errors"
	"os/exec"
	"strings"
	"testing"
	"time"
)

func shRunner(timeout time.Duration) *Runner { return &Runner{Bin: "sh", Timeout: timeout} }

// Without the default, a hung `gh` leaves the TUI frozen.
func TestRunAppliesTheDefaultTimeout(t *testing.T) {
	if _, err := shRunner(0).Run(context.Background(), "-c", "echo ok"); err != nil {
		t.Errorf("timeout=0 should use the default, but it failed: %v", err)
	}
	// Negative: a malformed Runner, which the default also fixes.
	if _, err := shRunner(-time.Second).Run(context.Background(), "-c", "sleep 1"); err != nil {
		t.Errorf("a negative timeout should fall back to the default, but it failed: %v", err)
	}
	// Run does not touch the Runner's Timeout: mutating it would make the second Run use the
	// default instead of the configured one.
	r := shRunner(0)
	if _, err := r.Run(context.Background(), "-c", "echo ok"); err != nil {
		t.Fatal(err)
	}
	if r.Timeout != 0 {
		t.Errorf("Run mutated the Runner's Timeout: %v", r.Timeout)
	}
	if _, err := shRunner(50*time.Millisecond).Run(context.Background(), "-c", "sleep 5"); err == nil {
		t.Error("a command that exceeds the timeout must fail")
	}
}

func TestRunUsesStderrMessageAndItsLastFallback(t *testing.T) {
	_, err := shRunner(time.Second).Run(context.Background(), "-c", "echo 'the real reason' >&2; exit 3")
	var toolErr *Error
	if !errors.As(err, &toolErr) {
		t.Fatalf("error = %T, want *tool.Error", err)
	}
	if toolErr.Msg != "the real reason" {
		t.Errorf("Msg = %q, want the stderr", toolErr.Msg)
	}
	if toolErr.ExitCode != 3 {
		t.Errorf("ExitCode = %d, want 3", toolErr.ExitCode)
	}

	_, err = shRunner(time.Second).Run(context.Background(), "-c", "exit 7")
	if !errors.As(err, &toolErr) {
		t.Fatalf("error = %T, want *tool.Error", err)
	}
	if toolErr.Msg == "" {
		t.Error("with no stderr the reason cannot end up empty")
	}
	if toolErr.ExitCode != 7 {
		t.Errorf("ExitCode = %d, want 7", toolErr.ExitCode)
	}

	// Whitespace-only stderr counts as empty too, because it is trimmed before the check.
	_, err = shRunner(time.Second).Run(context.Background(), "-c", "echo '   ' >&2; exit 5")
	if !errors.As(err, &toolErr) {
		t.Fatalf("error = %T, want *tool.Error", err)
	}
	if toolErr.Msg == "" || toolErr.Msg == "   " {
		t.Errorf("Msg = %q, want the process's error and not the blanks", toolErr.Msg)
	}
}

// Some forge CLIs print a valid answer on stdout and exit non-zero.
func TestRunReturnsStdoutEvenWhenItFails(t *testing.T) {
	out, err := shRunner(time.Second).Run(context.Background(), "-c", "echo 'valid output'; exit 1")
	if err == nil {
		t.Fatal("an error was expected")
	}
	if strings.TrimSpace(out) != "valid output" {
		t.Errorf("stdout = %q, want the valid output even though the command failed", out)
	}
}

// The precondition of the classification chain.
func TestKindIgnoresTheHTTPCodeThatSeparatesNothing(t *testing.T) {
	// Text with no HTTP code is classified by its wording, not by a fake 0.
	for msg, want := range map[string]string{
		"401 Unauthorized":           "auth",
		"404 Not Found":              "notfound",
		"403 Forbidden":              "permission",
		"you must have push access":  "permission",
		"context deadline exceeded":  "timeout",
		"something entirely unknown": "network",
	} {
		if got := Kind(errors.New(msg)); got != want {
			t.Errorf("Kind(%q) = %q, want %q", msg, got, want)
		}
	}

	// A code kindForHTTP does not translate lets the text through; 418 is the clean case.
	if got := Kind(errors.New("HTTP 418: I am a teapot, not authorized")); got != "permission" {
		t.Errorf("with a code without a translation the text must win, it gave %q", got)
	}
	// The case that really pins the ORDER: messages where the HTTP code and the text disagree.
	discrepancies := map[string]string{
		"HTTP 404: merge conflict":            "notfound",
		"HTTP 401: forbidden":                 "auth",
		"HTTP 403: you must have push access": "permission",
		"HTTP 429: conflict while merging":    "ratelimit",
		"HTTP 503: 404 not found":             "network",
		"HTTP 422: permission denied":         "validation",
		"HTTP 409: you must have push access": "conflict",
		"HTTP 500: 401 Unauthorized":          "network",
	}
	for msg, want := range discrepancies {
		if got := Kind(errors.New(msg)); got != want {
			t.Errorf("Kind(%q) = %q, want %q (the HTTP code wins over the text)", msg, got, want)
		}
	} // And KindForHTTP translation: those with no translation return empty.
	for _, code := range []int{0, 200, 201, 204, 301, 418, 451, 499} {
		if got := kindForHTTP(code); got != "" {
			t.Errorf("kindForHTTP(%d) = %q, want \"\" (no translation, let the text decide)", code, got)
		}
	}
	for code, want := range map[int]string{
		401: "auth", 403: "permission", 404: "notfound",
		409: "conflict", 422: "validation", 429: "ratelimit",
		500: "network", 503: "network", 599: "network",
	} {
		if got := kindForHTTP(code); got != want {
			t.Errorf("kindForHTTP(%d) = %q, want %q", code, got, want)
		}
	}
}

func TestHTTPStatusRebuildsTheCode(t *testing.T) {
	cases := map[string]int{
		"HTTP 404: Not Found":                  404,
		"HTTP 503 Service Unavailable":         503,
		"status code 401":                      401,
		"status: 429":                          429,
		"http/1.1 200 OK":                      200,
		"HTTP 000":                             0,
		"no code":                              0,
		"":                                     0,
		"HTTP 40":                              0, // incomplete: it is not invented
		"status code 40a":                      0,
		"port 8080 is not a code":              0,
		"HTTP 403 without a status text":       403,
		"receiving 200 but it is not a status": 0,
		"status code: 422 with a colon":        422,
		// Something that looks like a code but is not. A false positive here is not a 0: it is a code
		// printed as if it were one.
		"X-Status-Code: 409 in a header":  0,
		"status_code 404 with underscore": 0,
		"HTTP 40x4 is not a code":         0,
	}
	for msg, want := range cases {
		if got := HTTPStatus(msg); got != want {
			t.Errorf("HTTPStatus(%q) = %d, want %d", msg, got, want)
		}
	}
}

func TestFirstLineTrimsWithoutLosingAnything(t *testing.T) {
	cases := map[string]string{
		"one":                "one",
		"one\ntwo":           "one",
		"\none":              "", // the first line is empty: there is no reason
		"one\n\ntwo":         "one",
		"one\n":              "one",
		"":                   "",
		"\n":                 "",
		"with\nthree\nlines": "with",
		"without a newline":  "without a newline",
	}
	for in, want := range cases {
		if got := FirstLine(in); got != want {
			t.Errorf("FirstLine(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestExitCodeReadsFromTheChainAndTheExitError(t *testing.T) {
	if got := ExitCode(nil); got != 0 {
		t.Errorf("ExitCode(nil) = %d, want 0", got)
	}
	if got := ExitCode(errors.New("some error")); got != 0 {
		t.Errorf("ExitCode(other error) = %d, want 0", got)
	}
	if got := ExitCode(&Error{ExitCode: 42}); got != 42 {
		t.Errorf("ExitCode(*Error) = %d, want 42", got)
	}
	if got := ExitCode(errWrap{&Error{ExitCode: 9}}); got != 9 {
		t.Errorf("ExitCode(wrapped) = %d, want 9", got)
	}
	var exitErr *exec.ExitError
	if errors.As(shRunFailure(t, 6), &exitErr) {
		if got := ExitCode(exitErr); got != 6 {
			t.Errorf("ExitCode(exec.ExitError) = %d, want 6", got)
		}
	}
}

type errWrap struct{ err error }

func (e errWrap) Error() string { return "wrapped: " + e.err.Error() }
func (e errWrap) Unwrap() error { return e.err }

func shRunFailure(t *testing.T, code int) error {
	t.Helper()
	_, err := shRunner(time.Second).Run(context.Background(), "-c", "exit "+itoa(code))
	if err == nil {
		t.Fatal("a failure was expected")
	}
	var toolErr *Error
	if !errors.As(err, &toolErr) {
		t.Fatalf("error = %T, want *tool.Error", err)
	}
	return toolErr.Err
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}
