package gitlab

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The runner's host is what is checked, not the struct's.
func TestNewFallsBackToDefaultsAndTheHostGoesInTheEnvironment(t *testing.T) {
	a := New("", "")
	if a.Host() != "gitlab.example.com" {
		t.Errorf("no host gave %q, want the example self-managed: a default that was "+
			"gitlab.com would send the calls to a place the user did not configure",
			a.Host())
	}
	if a.Forge() != ForgeName {
		t.Errorf("Forge gave %q, want %q", a.Forge(), ForgeName)
	}
	if a.runner == nil {
		t.Fatal("New left the runner nil")
	}
	if got := a.runner.Extra; !contains(got, "GITLAB_HOST=gitlab.example.com") {
		t.Errorf("the runner does not carry GITLAB_HOST: %v", got)
	}
	if got := a.runner.Extra; !contains(got, "GLAB_NO_PROMPT=1") {
		t.Errorf("the runner does not carry GLAB_NO_PROMPT: without it glab can ask for a token "+
			"in a process with no terminal and hang: %v", got)
	}

	selfManaged := New("git.umane.example", "")
	if selfManaged.Host() != "git.umane.example" {
		t.Errorf("with a host gave %q", selfManaged.Host())
	}
	if got := selfManaged.runner.Extra; !contains(got, "GITLAB_HOST=git.umane.example") {
		t.Errorf("the runner does not carry the configured host: %v", got)
	}

	custom := New("", "/opt/glab")
	if got := custom.runner.Bin; got != "/opt/glab" {
		t.Errorf("with a binary gave %q", got)
	}
	// And the default.
	if got := New("", "").runner.Bin; got != "glab" {
		t.Errorf("with no binary gave %q, want glab", got)
	}
}

func TestAuthDistinguishesSessionAndBadToken(t *testing.T) {
	dir := t.TempDir()

	// GitLab's format is "as <login>", not GitHub's "account <login>".
	good := glabScript(t, dir, "glab-ok", `#!/bin/sh
echo "Logged in to git.umane.example as glab (GLAB_TOKEN)"
exit 0
`)
	a := New("git.umane.example", good)
	auth := a.Auth(context.Background())
	if !auth.OK {
		t.Fatalf("a good session gave OK=false: %s", auth.Reason)
	}
	if auth.Forge != ForgeName {
		t.Errorf("the state does not bring the forge name: %+v", auth)
	}
	if auth.Login != "glab" {
		t.Errorf("login %q, want glab", auth.Login)
	}

	// The reason is the FIRST line of stderr, not the last and not all of them.
	bad := glabScript(t, dir, "glab-ko", `#!/bin/sh
echo "401 Unauthorized" >&2
echo "detail that is not shown" >&2
exit 1
`)
	auth = New("git.umane.example", bad).Auth(context.Background())
	if auth.OK {
		t.Error("a bad session gave OK=true")
	}
	if strings.TrimSpace(auth.Reason) == "" {
		t.Error("a bad session gave an empty Reason")
	}
	if !strings.Contains(auth.Reason, "401") {
		t.Errorf("the reason %q does not bring what the CLI said", auth.Reason)
	}
	if auth.Login != "" {
		t.Errorf("a bad session brings login %q", auth.Login)
	}

	auth = New("git.umane.example", filepath.Join(dir, "does-not-exist")).Auth(context.Background())
	if auth.OK {
		t.Error("a nonexistent binary gave OK=true")
	}
	if strings.TrimSpace(auth.Reason) == "" {
		t.Error("a nonexistent binary gave an empty Reason")
	}

	// A good session with unexpected output: OK=true and an empty login, and that is the honest
	// answer.
	strange := glabScript(t, dir, "glab-strange", "#!/bin/sh\necho 'something else'\nexit 0\n")
	auth = New("git.umane.example", strange).Auth(context.Background())
	if !auth.OK {
		t.Errorf("unexpected output gave OK=false: %s", auth.Reason)
	}
	if auth.Login != "" {
		t.Errorf("unexpected output gave login %q, want empty", auth.Login)
	}
}

// The regex points at "Logged in to <host> as <login>".
func TestTheLoginIsExtractedFromGlabsFormat(t *testing.T) {
	cases := []struct {
		name   string
		output string
		want   string
	}{
		{"full format", "Logged in to git.umane.example as glab (GLAB_TOKEN)", "glab"},
		{"dots in the login", "as j.perez (GLAB_TOKEN)", "j.perez"},
		{"hyphen in the login", "as my-user (GLAB_TOKEN)", "my-user"},
		{"without the token path", "Logged in to git.umane.example as glab", "glab"},
		{"active account", "  Active account: true\n  Logged in to x as glab (Y)", "glab"},
		{"without the word as", "something else", ""},
		{"empty", "", ""},
		{"only spaces", "   \n  ", ""},
		{"login with a glued parenthesis", "as user(GLAB_TOKEN)", "user"},
	}
	for _, c := range cases {
		got := loginFromAuthStatus(c.output)
		if got != c.want {
			t.Errorf("%s: gave %q, want %q", c.name, got, c.want)
		}
		// It never prints text containing the token, which is what would break the comparison.
		if strings.ContainsAny(got, "()") {
			t.Errorf("%s: the login carries parentheses: %q", c.name, got)
		}
	}
}

func glabScript(t *testing.T, dir, name, body string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

func contains(xs []string, want string) bool {
	for _, x := range xs {
		if x == want {
			return true
		}
	}
	return false
}
