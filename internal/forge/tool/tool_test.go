package tool

import (
	"errors"
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
