package tool

import (
	"errors"
	"os/exec"
	"strings"
	"testing"
)

// Kind decides what to do with a forge CLI's failure, and the whole package hangs on it: it answers
//"is this retried", "is this a permission" and "what does the user see".

// GitHub answers 403 for a permission AND for a rate limit, so without the text a rate limit is
// classified as a permission.
func TestTheTextWinsOverTheHTTPCode(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want string
	}{
		{
			name: "403 with rate limit text",
			err:  errors.New("gh: API rate limit exceeded for user (HTTP 403)"),
			want: "ratelimit",
		},
		{
			name: "429 with no recognisable text",
			err:  errors.New("gh: something went wrong (HTTP 429)"),
			want: "ratelimit",
		},
		{
			name: "403 permission with no rate limit text",
			err:  errors.New("gh: Resource not accessible by integration (HTTP 403)"),
			want: "permission",
		},
		{
			name: "409 with unmergeable text",
			err:  errors.New("gh: Pull request is not mergeable (HTTP 409)"),
			want: "unmergeable",
		},
		{
			// The obvious reading is the wrong one: "Head branch was modified" is UNMERGEABLE, not a
			//conflict, even though 409 and the word "modified" both point at conflict.
			name: "head branch was modified is unmergeable, not conflict",
			err:  errors.New("gh: Head branch was modified. Review and try the merge again. (HTTP 409)"),
			want: "unmergeable",
		},
		{
			name: "409 with conflict text",
			err:  errors.New("glab: conflict: source branch has been updated (HTTP 409)"),
			want: "conflict",
		},
		{
			name: "404",
			err:  errors.New("gh: Not Found (HTTP 404)"),
			want: "notfound",
		},
		{
			name: "401",
			err:  errors.New("gh: Bad credentials (HTTP 401)"),
			want: "auth",
		},
	}
	for _, c := range cases {
		if got := Kind(c.err); got != c.want {
			t.Errorf("%s: Kind gave %q, want %q", c.name, got, c.want)
		}
	}
	if got := Kind(errors.New("something that looks like nothing")); got == "" {
		t.Error("Kind gave empty for an error with no signal: that reads as not-classified")
	}
	if got := Kind(nil); got != "" {
		t.Errorf("Kind(nil) gave %q, want empty", got)
	}
}

// The order matters for the same reason as the others: a 422 in GitHub is custom deployment
// validation.
func TestSelfRejectionIsLookedAtBeforeTheCode(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want string
	}{
		{
			name: "you cannot approve your own PR",
			err:  errors.New("gh: Can not approve your own pull request (HTTP 422)"),
			want: "selfreview",
		},
		{
			name: "the text is enough, no HTTP",
			err:  errors.New("cannot approve your own pull request"),
			want: "selfreview",
		},
		{
			name: "GitLab's text",
			err:  errors.New("cannot approve your own merge request"),
			want: "selfreview",
		},
	}
	for _, c := range cases {
		if got := Kind(c.err); got != c.want {
			t.Errorf("%s: Kind gave %q, want %q", c.name, got, c.want)
		}
	}
	if got := Kind(errors.New("Can NOT approve your OWN pull request")); got != "selfreview" {
		t.Errorf("with uppercase it gave %q, want selfreview", got)
	}
}

func TestTheErrorsMessageCarriesWhatRanAndHowItEnded(t *testing.T) {
	cases := []struct {
		name string
		e    *Error
		want string
	}{
		{
			name: "with a code",
			e:    &Error{Bin: "gh", Args: []string{"pr", "view", "1"}, Msg: "not found", ExitCode: 1},
			want: "gh pr view 1: not found (exit 1)",
		},
		{
			name: "without a code",
			e:    &Error{Bin: "glab", Args: []string{"mr", "list"}, Msg: "interrupted"},
			want: "glab mr list: interrupted",
		},
		{
			name: "several arguments",
			e:    &Error{Bin: "gh", Args: []string{"api", "-X", "POST", "/x"}, Msg: "bad"},
			want: "gh api -X POST /x: bad",
		},
		{
			name: "no arguments",
			e:    &Error{Bin: "gh", Msg: "something"},
			want: "gh : something",
		},
	}
	for _, c := range cases {
		if got := c.e.Error(); got != c.want {
			t.Errorf("%s: Error() gave %q, want %q", c.name, got, c.want)
		}
		if strings.TrimSpace(c.e.Error()) == "" {
			t.Errorf("%s: empty message", c.name)
		}
	}
}

// Same reason as in gitcmd: Kind receives an error wrapping an *exec.ExitError.
func TestTheErrorUnwrapsTheCause(t *testing.T) {
	if (&Error{Bin: "gh"}).Unwrap() != nil {
		t.Error("Unwrap returned something without a cause")
	}
	if errors.Unwrap(&Error{Bin: "gh"}) != nil {
		t.Error("errors.Unwrap returned something without a cause")
	}

	cmd := exec.Command("sh", "-c", "exit 9")
	err := cmd.Run()
	if err == nil {
		t.Fatal("expected sh -c 'exit 9' to fail")
	}
	e := &Error{Bin: "gh", Args: []string{"x"}, Msg: "m", Err: err}
	if !errors.Is(e, err) {
		t.Error("errors.Is does not reach the cause")
	}
	var exit *exec.ExitError
	if !errors.As(e, &exit) {
		t.Fatal("errors.As does not reach the *exec.ExitError")
	}
	if exit.ExitCode() != 9 {
		t.Errorf("ExitCode = %d, want 9", exit.ExitCode())
	}
	if !strings.Contains(e.Error(), "m") {
		t.Errorf("the message lost its own text: %q", e.Error())
	}
}

// EXACTLY three digits: a bare `\d+` would read "4040" or a version number as a status.
func TestHTTPStatusOnlyAcceptsAThreeDigitCode(t *testing.T) {
	cases := []struct {
		input string
		want  int
	}{
		{"gh: Not Found (HTTP 404)", 404},
		{"HTTP 429", 429},
		{"(HTTP 503)", 503},
		{"no code", 0},
		{"HTTP 40", 0},
		{"HTTP 4040", 0},
		{"", 0},
		{"error 5000 while loading", 0},
	}
	for _, c := range cases {
		if got := HTTPStatus(c.input); got != c.want {
			t.Errorf("HTTPStatus(%q) gave %d, want %d", c.input, got, c.want)
		}
	}
}

// These are the branches run_test.go's table cannot reach, and the reason is specific.
func TestKindByTextWhenThereIsNoHTTPCodeToSayIt(t *testing.T) {
	for msg, want := range map[string]string{
		"timed out after 30s":                "timeout",
		"request timed out":                  "timeout",
		"authentication failed":              "auth",
		"not logged in: run `gh auth login`": "auth",
		"HTTP request timed out (no code)":   "timeout",
	} {
		if got := Kind(errors.New(msg)); got != want {
			t.Errorf("Kind(%q) = %q, want %q", msg, got, want)
		}
	}

	// Deliberately left out: "could not authenticate with the token" falls into `network`. Writing
	//the test to force that synonym would be fitting the code to the test.
	if got := Kind(errors.New("could not authenticate with the token")); got != "network" {
		t.Errorf("a synonym that is not in the list gave %q; if the list is widened, it is "+
			"widened with a real message and not with a test", got)
	}

	// The control: the two cases that DO carry an HTTP code still go through the code. If the text ran
	//first, a "422 validation error" containing "conflict" would be misread.
	if got := Kind(errors.New("HTTP 422 Unprocessable Entity: conflict on branch")); got != "validation" {
		t.Errorf("with 422 and the word conflict in the body it gave %q, want validation: the "+
			"code wins, and retrying an invalid approve burns quota without fixing anything", got)
	}
}

func TestKindByTextForRateLimitAndConflictWhenThereIsNoHTTPCode(t *testing.T) {
	for msg, want := range map[string]string{
		"abuse detection mechanism triggered":   "ratelimit",
		"You have triggered an abuse detection": "ratelimit",
		"API rate limit exceeded for user":      "ratelimit",
		"already merged":                        "conflict",
		"already closed":                        "conflict",
		"Pull Request is already merged":        "conflict",
		"Merge conflict detected":               "conflict",
	} {
		if got := Kind(errors.New(msg)); got != want {
			t.Errorf("Kind(%q) = %q, want %q", msg, got, want)
		}
	}

	// The asymmetry is why classifying well matters: an "already merged" classed as `network` does not
	//retry; classed as `conflict` it does.
	for _, msg := range []string{"already merged", "abuse detection mechanism triggered"} {
		if got := Kind(errors.New(msg)); got == "network" {
			t.Errorf("%q fell into network, which is the class where nothing is done", msg)
		}
	}
}

// HTTPStatus only recognises a code when it comes with its pattern —"HTTP 404" — so these two
// branches are only reachable with the bare code.
func TestKindWithABareCodeAndNoHTTPPattern(t *testing.T) {
	for msg, want := range map[string]string{
		"429":                 "ratelimit",
		"Error: 429":          "ratelimit",
		"bare 429 here":       "ratelimit",
		"bare 422 here":       "validation",
		"api said 422, sorry": "validation",
	} {
		if code := HTTPStatus(msg); code != 0 {
			t.Errorf("the fixture %q DOES have an HTTP code (%d): it no longer tests the text switch",
				msg, code)
			continue
		}
		if got := Kind(errors.New(msg)); got != want {
			t.Errorf("Kind(%q) = %q, want %q", msg, got, want)
		}
	}
}
