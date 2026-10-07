package github

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The two adapters look alike and are not interchangeable. One of the three differences is SILENT.

// The argv-vs-variable split is what is checked, because they are two different strategies.
func TestNewFallsBackToDefaultsAndTheHostGoesInTheArgvNotTheEnvironment(t *testing.T) {
	a := New("", "")
	if a.Host() != "github.com" {
		t.Errorf("no host gave %q, want github.com", a.Host())
	}
	if a.Forge() != ForgeName {
		t.Errorf("Forge gave %q, want %q", a.Forge(), ForgeName)
	}
	if a.runner == nil {
		t.Fatal("New left the runner nil")
	}
	// The prompt disabled, which is what stops gh from opening a browser to authenticate.
	if !has(a.runner.Extra, "GH_PROMPT_DISABLED=1") {
		t.Errorf("the runner does not carry GH_PROMPT_DISABLED: %v", a.runner.Extra)
	}
	// And NO host variable: on GitHub the host goes in the argv.
	for _, v := range a.runner.Extra {
		if strings.Contains(v, "HOST") {
			t.Errorf("the GitHub runner carries %q: the host goes in the argv, which is the only "+
				"thing `gh` accepts in every command", v)
		}
	}
	if a.runner.Bin != "gh" {
		t.Errorf("no binary gave %q, want gh", a.runner.Bin)
	}

	corporate := New("git.umane.example", "")
	if corporate.Host() != "git.umane.example" {
		t.Errorf("with a host gave %q", corporate.Host())
	}
	// The exact argv is read from the script because gh also accepts --hostname, so inspecting the adapter is not enough.
	echo := filepath.Join(t.TempDir(), "gh-echo")
	log := filepath.Join(t.TempDir(), "argv")
	if err := os.WriteFile(echo, []byte("#!/bin/sh\necho \"$@\" > "+log+"\necho ok\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if auth := New("git.umane.example", echo).Auth(context.Background()); !auth.OK {
		t.Fatalf("a good session gave OK=false: %s", auth.Reason)
	}
	received, err := os.ReadFile(log)
	if err != nil {
		t.Fatalf("the script never got to run: %v", err)
	}
	argv := strings.TrimSpace(string(received))
	if !strings.Contains(argv, "--hostname git.umane.example") {
		t.Errorf("the call does not carry the host in the argv: %q", argv)
	}
	if !strings.HasPrefix(argv, "auth status") {
		t.Errorf("the call is not `auth status`: %q", argv)
	}
	if count(strings.Fields(argv), "git.umane.example") != 1 {
		t.Errorf("the host appears %d times in %q", count(strings.Fields(argv), "git.umane.example"), argv)
	}

	if New("", "/opt/gh").runner.Bin != "/opt/gh" {
		t.Error("a custom binary was not used")
	}
}

// The failure's reason is the CLI's text verbatim: it is the only thing that says why.
func TestAuthDistinguishesSessionAndBadToken(t *testing.T) {
	dir := t.TempDir()

	good := script(t, dir, "gh-ok", `#!/bin/sh
echo "github.com"
echo "  ✓ Logged in to github.com account user (GH_TOKEN)"
exit 0
`)
	auth := New("github.com", good).Auth(context.Background())
	if !auth.OK {
		t.Fatalf("a good session gave OK=false: %s", auth.Reason)
	}
	if auth.Login != "user" {
		t.Errorf("login %q, want user", auth.Login)
	}
	if auth.Forge != ForgeName {
		t.Errorf("the state does not bring the forge name: %+v", auth)
	}

	bad := script(t, dir, "gh-ko", `#!/bin/sh
echo "gh: To use GitHub CLI in a GitHub Actions workflow, set the GH_TOKEN" >&2
exit 4
`)
	auth = New("github.com", bad).Auth(context.Background())
	if auth.OK {
		t.Error("a bad session gave OK=true")
	}
	if !strings.Contains(auth.Reason, "GH_TOKEN") {
		t.Errorf("the reason %q does not bring what the CLI said", auth.Reason)
	}
	if auth.Login != "" {
		t.Errorf("a bad session brings login %q", auth.Login)
	}

	auth = New("github.com", filepath.Join(dir, "does-not-exist")).Auth(context.Background())
	if auth.OK {
		t.Error("a nonexistent binary gave OK=true")
	}
	if strings.TrimSpace(auth.Reason) == "" {
		t.Error("a nonexistent binary gave an empty Reason")
	}

	// Unexpected output with a zero exit is a valid session with an empty login; treating it otherwise would invent a failure.
	strange := script(t, dir, "gh-strange", "#!/bin/sh\necho 'something else'\nexit 0\n")
	auth = New("github.com", strange).Auth(context.Background())
	if !auth.OK {
		t.Errorf("unexpected output gave OK=false: %s", auth.Reason)
	}
	if auth.Login != "" {
		t.Errorf("unexpected output gave login %q", auth.Login)
	}
}

// The regex demands a space after "account": `gh auth status` has an "Active account: true" line with the same word.
func TestTheLoginIsNotConfusedWithTheActiveAccountLine(t *testing.T) {
	cases := []struct {
		name   string
		output string
		want   string
	}{
		{"full format", "  ✓ Logged in to github.com account user (GH_TOKEN)", "user"},
		{"dots in the login", "account j.perez (GH_TOKEN)", "j.perez"},
		{"active account before the login",
			"  Active account: true\n  ✓ Logged in to github.com account user (GH_TOKEN)", "user"},
		{"only active account", "  Active account: true", ""},
		{"without the word", "something else", ""},
		{"empty", "", ""},
		{"only spaces", "   \n  ", ""},
		// A token glued with no space gives "user": the regex's character class stops at the parenthesis.
		{"token glued", "account user(GH_TOKEN)", "user"},
	}
	for _, c := range cases {
		got := loginFromAuthStatus(c.output)
		if got != c.want {
			t.Errorf("%s: gave %q, want %q", c.name, got, c.want)
		}
		// It never returns "true", which would turn the viewer into a concrete account.
		if got == "true" {
			t.Errorf("%s: returned \"true\" as the login", c.name)
		}
	}
}

func script(t *testing.T, dir, name, body string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

func has(xs []string, want string) bool { return count(xs, want) > 0 }

func count(xs []string, want string) int {
	n := 0
	for _, x := range xs {
		if x == want {
			n++
		}
	}
	return n
}
