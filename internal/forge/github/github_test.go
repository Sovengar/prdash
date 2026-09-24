package github

import (
	"strings"
	"testing"
)

func TestSearchQueryCoversQualifiers(t *testing.T) {
	cases := map[string]string{
		"author:@me":           `is:pr is:open author:@me`,
		"review-requested:@me": `is:pr is:open review-requested:@me`,
		"mentions:@me":         `is:pr is:open mentions:@me`,
	}
	for qualifier, want := range cases {
		q := searchQuery(qualifier)
		if !strings.Contains(q, want) {
			t.Errorf("searchQuery(%q) no contiene %q:\n%s", qualifier, want, q)
		}
		if !strings.Contains(q, "reviewDecision") {
			t.Errorf("searchQuery(%q) debería pedir reviewDecision", qualifier)
		}
		if !strings.Contains(q, "statusCheckRollup") {
			t.Errorf("searchQuery(%q) debería pedir los checks", qualifier)
		}
	}
}

func TestToolEnvIsNonInteractive(t *testing.T) {
	env := toolEnv()
	joined := strings.Join(env, "\n")
	for _, want := range []string{"LC_ALL=C", "GIT_TERMINAL_PROMPT=0", "GH_PROMPT_DISABLED=1"} {
		if !strings.Contains(joined, want) {
			t.Errorf("toolEnv no contiene %q", want)
		}
	}
	// LC_ALL no debe quedar duplicado por el entorno del usuario.
	count := 0
	for _, kv := range env {
		if strings.HasPrefix(kv, "LC_ALL=") {
			count++
		}
	}
	if count != 1 {
		t.Errorf("LC_ALL aparece %d veces, want 1", count)
	}
}

func TestClassify(t *testing.T) {
	cases := map[string]string{
		"gh api: ... context deadline exceeded": "timeout",
		"gh api: HTTP 401 Unauthorized":         "auth",
		"gh api: connection refused":            "network",
	}
	for msg, want := range cases {
		if got := classify(errString(msg)); got != want {
			t.Errorf("classify(%q) = %q, want %q", msg, got, want)
		}
	}
}

type errString string

func (e errString) Error() string { return string(e) }
