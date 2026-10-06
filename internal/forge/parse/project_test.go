package parse

import (
	"strings"
	"testing"
	"time"
)

// The three branches cover three shapes of input.
func TestJoinProjectDoesNotPutOrphanSeparators(t *testing.T) {
	cases := []struct {
		name         string
		owner, piece string
		want         string
	}{
		{"both", "acme", "proj", "acme/proj"},
		{"only owner", "acme", "", "acme"},
		{"only name", "", "proj", "proj"},
		{"neither", "", "", ""},
		{"owner with slashes", "group/sub", "proj", "group/sub/proj"},
		{"name with slashes", "acme", "sub/proj", "acme/sub/proj"},
		{"both with slashes", "a/b", "c/d", "a/b/c/d"},
		{"spaces", " acme ", " proj ", " acme / proj "},
	}
	for _, c := range cases {
		got := joinProject(c.owner, c.piece)
		if got != c.want {
			t.Errorf("%s: joinProject(%q, %q) gave %q, want %q", c.name, c.owner, c.piece, got, c.want)
		}
		// The property that makes the function exist: there is never a slash at the start nor at the end.
		if strings.HasPrefix(got, "/") || strings.HasSuffix(got, "/") {
			t.Errorf("%s: gave %q, with a slash on an edge", c.name, got)
		}
	}
}

// Splitting at the FIRST slash is the error.
func TestSplitProjectSplitsAtTheLastSlashAndNotTheFirst(t *testing.T) {
	cases := []struct {
		name  string
		path  string
		wantA string
		wantB string
	}{
		{"one slash", "acme/proj", "acme", "proj"},
		{"two slashes", "group/sub/proj", "group/sub", "proj"},
		{"three slashes", "a/b/c/proj", "a/b/c", "proj"},
		{"no slash", "proj", "", "proj"},
		{"leading slash", "/group/proj", "group", "proj"},
		{"trailing slash", "group/proj/", "group", "proj"},
		{"slashes on both edges", "/group/proj/", "group", "proj"},
		{"only slashes", "///", "", ""},
		{"empty", "", "", ""},
		// Trim removes slashes, NOT spaces, so the spaces stay inside both halves.
		{"slash with spaces around", " group/sub /proj ", " group/sub ", "proj "},
	}
	for _, c := range cases {
		a, b := splitProject(c.path)
		if a != c.wantA || b != c.wantB {
			t.Errorf("%s: splitProject(%q) gave (%q, %q), want (%q, %q)",
				c.name, c.path, a, b, c.wantA, c.wantB)
		}
		// The right half never carries slashes: it is a project NAME, not a path.
		if strings.Contains(b, "/") {
			t.Errorf("%s: the name %q has slashes", c.name, b)
		}
		// Both halves together rebuild the original without the middle slash.
		if a != "" && b != "" {
			if joined := joinProject(a, b); joined != strings.Trim(c.path, "/") {
				t.Errorf("%s: joinProject of the result gave %q, it does not rebuild %q",
					c.name, joined, strings.Trim(c.path, "/"))
			}
		}
	}
}

// What happens to an unparseable timestamp.
func TestParseTimeOnlyAcceptsRFC3339AndTheRestGivesZero(t *testing.T) {
	good := []string{
		"2026-04-01T12:00:00Z",
		"2026-04-01T12:00:00+02:00",
		"2026-04-01T12:00:00.123456789Z",
	}
	for _, s := range good {
		got := parseTime(s)
		if got.IsZero() {
			t.Errorf("parseTime(%q) gave the zero time", s)
		}
		if got.Year() != 2026 {
			t.Errorf("parseTime(%q) gave the year %d", s, got.Year())
		}
	}

	bad := []struct {
		name  string
		value string
	}{
		{"empty", ""},
		{"only spaces", "   "},
		{"date only", "2026-04-01"},
		{"american format", "04/01/2026"},
		{"epoch in seconds", "1775044800"},
		{"epoch in millis", "1775044800000"},
		{"garbage", "whenever"},
		{"invalid month", "2026-13-01T12:00:00Z"},
		{"text", "<time datetime=\"2026-04-01\"></time>"},
	}
	for _, c := range bad {
		if got := parseTime(c.value); !got.IsZero() {
			t.Errorf("%s: parseTime(%q) gave %v, want the zero time", c.name, c.value, got)
		}
	}

	// The property that distinguishes it from time.Parse: the zero of a failed parse and the zero of
	//a value that is zero differ.
	if parseTime("") != parseTime("garbage") {
		t.Error("the missing value and the unreadable one give different times, and they should not")
	}
	if parseTime("").After(time.Now()) {
		t.Error("the zero time is not the minimum")
	}
}

// The temptation is to split at the FIRST `!`.
func TestProjectFromRefRemovesTheNumberSuffix(t *testing.T) {
	cases := []struct {
		name string
		ref  string
		want string
	}{
		{"with number", "group/proj!12", "group/proj"},
		{"without number", "group/proj", "group/proj"},
		{"with slashes", "a/b/proj!3", "a/b/proj"},
		{"big number", "group/proj!1234", "group/proj"},
		{"empty", "", ""},
		{"only the sign", "!", ""},
		{"with spaces", "group/proj ! 12", "group/proj "},
	}
	for _, c := range cases {
		if got := projectFromRef(c.ref); got != c.want {
			t.Errorf("%s: projectFromRef(%q) gave %q, want %q", c.name, c.ref, got, c.want)
		}
	}
	// What comes out has NO `!`, because it is used as a project path.
	for _, c := range cases {
		if got := projectFromRef(c.ref); strings.Contains(got, "!") {
			t.Errorf("%s: an exclamation mark was left in %q", c.name, got)
		}
	}
}

// The difference with the other extractor.
func TestSplitRepoURLTakesTheProjectOnlyFromTheAPIsPath(t *testing.T) {
	cases := []struct {
		name  string
		url   string
		wantA string
		wantB string
	}{
		{"API of a repo", "https://api.github.com/repos/acme/proj", "acme", "proj"},
		{"API with subgroups", "https://api.github.com/repos/group/sub/proj", "group/sub", "proj"},
		{"self-hosted API", "https://git.umane.example/api/v3/repos/acme/proj", "acme", "proj"},
		// A query string stays glued to the name, which is acceptable because the URL comes from us.
		{"with query", "https://api.github.com/repos/acme/proj?x=1", "acme", "proj?x=1"},
		{"clone url", "https://github.com/acme/proj.git", "", ""},
		{"html url", "https://github.com/acme/proj", "", ""},
		// With no scheme the project still comes out: splitRepoURL only looks for the `/repos/` marker.
		{"no scheme", "api.github.com/repos/acme/proj", "acme", "proj"},
		{"empty", "", "", ""},
	}
	for _, c := range cases {
		a, b := splitRepoURL(c.url)
		if a != c.wantA || b != c.wantB {
			t.Errorf("%s: splitRepoURL(%q) gave (%q, %q), want (%q, %q)",
				c.name, c.url, a, b, c.wantA, c.wantB)
		}
	}

	// The property: with a project, the two halves recompose into a real path.
	a, b := splitRepoURL("https://api.github.com/repos/acme/proj")
	if got := joinProject(a, b); got != "acme/proj" {
		t.Errorf("the project from the URL gave %q, which cannot be recomposed", got)
	}
}
