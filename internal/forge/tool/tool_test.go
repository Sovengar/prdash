package tool

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEnvIsNonInteractive(t *testing.T) {
	joined := strings.Join(Env("GH_PROMPT_DISABLED=1"), "\n")
	for _, want := range []string{"LC_ALL=C", "GIT_TERMINAL_PROMPT=0", "NO_COLOR=1", "GH_PROMPT_DISABLED=1"} {
		if !strings.Contains(joined, want) {
			t.Errorf("Env no contiene %q", want)
		}
	}
	count := 0
	for _, kv := range Env() {
		if strings.HasPrefix(kv, "LC_ALL=") {
			count++
		}
	}
	if count != 1 {
		t.Errorf("LC_ALL aparece %d veces, want 1", count)
	}
}

func TestKind(t *testing.T) {
	cases := map[string]string{
		"context deadline exceeded":   "timeout",
		"HTTP 429 Too Many Requests":  "ratelimit",
		"API rate limit exceeded":     "ratelimit",
		"401 Unauthorized":            "auth",
		"not logged into any host":    "auth",
		"403 Forbidden":               "permission",
		"you must have push access":   "permission",
		"404 Not Found":               "notfound",
		"409 conflict: not mergeable": "conflict",
		"connection refused":          "network",
	}
	for msg, want := range cases {
		if got := Kind(errors.New(msg)); got != want {
			t.Errorf("Kind(%q) = %q, want %q", msg, got, want)
		}
	}
	if Kind(nil) != "" {
		t.Error("Kind(nil) debería ser vacío")
	}
}

func TestFirstLine(t *testing.T) {
	if got := FirstLine("uno\ndos"); got != "uno" {
		t.Errorf("FirstLine = %q", got)
	}
}

// TestKindHTTPFirst cubre M5: el código HTTP manda sobre el fraseo.
func TestKindHTTPFirst(t *testing.T) {
	cases := map[string]string{
		"glab api: HTTP 403: Forbidden":         "permission",
		"HTTP 404: Not Found":                   "notfound",
		"http/1.1 429 Too Many Requests":        "ratelimit",
		"server returned status code 401":       "auth",
		"HTTP 409: Conflict while merging":      "conflict",
		"HTTP 503 Service Unavailable":          "network",
		"401 Unauthorized":                      "auth",
		"you must have push access to the repo": "permission",
		"could not resolve to a PullRequest":    "network",
	}
	for msg, want := range cases {
		if got := Kind(errors.New(msg)); got != want {
			t.Errorf("Kind(%q) = %q, want %q", msg, got, want)
		}
	}
}

func TestErrorPreservesCauseAndExitCode(t *testing.T) {
	cause := errors.New("boom")
	err := &Error{Bin: "gh", Args: []string{"api"}, ExitCode: 8, Msg: "pending", Err: cause}
	if ExitCode(err) != 8 {
		t.Errorf("ExitCode = %d", ExitCode(err))
	}
	if !errors.Is(err, cause) {
		t.Error("el error debe preservar la causa (%w / Unwrap)")
	}
	if !strings.Contains(err.Error(), "exit 8") {
		t.Errorf("el mensaje debería incluir el exit code: %q", err.Error())
	}
}

// TestRunKeepsStdoutOnNonZeroExit cubre C8: `gh pr checks` sale con exit != 0
// pero trae JSON válido; el runner debe devolverlo igualmente.
func TestRunKeepsStdoutOnNonZeroExit(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "fake")
	if err := os.WriteFile(script, []byte("#!/bin/sh\necho '[{\"name\":\"ci\",\"state\":\"PENDING\",\"bucket\":\"pending\"}]'\nexit 8\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	out, err := New(script).Run(context.Background(), "pr", "checks")
	if err == nil {
		t.Fatal("se esperaba error por exit 8")
	}
	if ExitCode(err) != 8 {
		t.Errorf("ExitCode = %d, want 8", ExitCode(err))
	}
	if !strings.Contains(out, "pending") {
		t.Errorf("stdout debería conservarse pese al exit != 0: %q", out)
	}
}

// TestKindRateLimit403 cubre H-1: GitHub usa 403 tanto para permiso como para
// primary/secondary rate limit; el texto delata el límite.
func TestKindRateLimit403(t *testing.T) {
	cases := map[string]string{
		"HTTP 403: API rate limit exceeded for user ID 123":         "ratelimit",
		"gh: HTTP 403: You have exceeded a secondary rate limit":    "ratelimit",
		"HTTP 403: You have triggered an abuse detection mechanism": "ratelimit",
		"HTTP 403: Forbidden":                              "permission",
		"HTTP 403: Resource not accessible by integration": "permission",
	}
	for msg, want := range cases {
		if got := Kind(errors.New(msg)); got != want {
			t.Errorf("Kind(%q) = %q, want %q", msg, got, want)
		}
	}
}
