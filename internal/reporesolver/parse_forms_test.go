package reporesolver

import (
	"testing"

	"prdash/internal/forge/model"
)

func TestParseRemoteURLClassifiesFormsBeforeNormalizing(t *testing.T) {
	hosts := map[string]string{"github.com": "github", "gitlab.example.com": "gitlab"}

	cases := []struct {
		name string
		raw  string
		ok   bool
	}{
		{"with https scheme", "https://github.com/acme/widget.git", true},
		{"with ssh scheme", "ssh://git@github.com/acme/widget.git", true},
		{"scp", "git@github.com:acme/widget.git", true},
		{"scp with a one-character user", "a@github.com:acme/widget.git", true},

		{"scp with an empty user", "@github.com:acme/widget.git", false},
		{"scp with only an at-sign", "@", false},
		{"at-sign first and colon", "@github.com:acme/widget", false},

		{"empty scheme with colon", "://github.com:acme/widget", false},
		{"empty scheme and at-sign", "://git@github.com/acme/widget", false},
		{"only an empty scheme", "://", false},

		{"at-sign without colon", "git@github.com", false},
		{"at-sign and slash", "git@github.com/acme/widget", false},
		{"at-sign and nothing", "git@", false},

		{"local path", "/home/u/dev/widget", false},
		{"relative path", "../widget", false},
		{"only a colon", "acme:widget", false},
		{"only one word", "widget", false},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, ok := ParseRemoteURL(c.raw, hosts, nil)
			if ok != c.ok {
				t.Fatalf("ParseRemoteURL(%q) gave ok=%v, want %v (ref %+v)", c.raw, ok, c.ok, got)
			}
			// And when it says no it does not return half a repo: a half-filled RepoRef would paint as
			// real.
			if !ok && got != (model.RepoRef{}) {
				t.Errorf("ParseRemoteURL(%q) said no but returned %+v", c.raw, got)
			}
		})
	}
}

// The separators being looked for can appear BEFORE where they belong.
func TestParseRemoteURLIsNotConfusedByASeparatorInTheWrongPlace(t *testing.T) {
	hosts := map[string]string{"github.com": "github"}

	// Each entry with its exact result, because what is asserted is that the separator decides.
	cases := []struct {
		raw  string
		want model.RepoRef
	}{
		// With a scheme the scheme wins, and its `@` is the user, not an SCP separator.
		{"https://git@github.com/acme/widget.git",
			model.RepoRef{Forge: "github", Host: "github.com", Project: "acme/widget", Owner: "acme", Name: "widget"}},
		// An `@` inside the path is legal in a git path and is NOT a user.
		{"https://github.com/acme/wi@get.git",
			model.RepoRef{Forge: "github", Host: "github.com", Project: "acme/wi@get", Owner: "acme", Name: "wi@get"}},
		// With a port in the host: the port stays with the host and is not read as an SCP separator.
		{"ssh://git@github.com:22/acme/widget.git",
			model.RepoRef{Forge: "github", Host: "github.com", Project: "acme/widget", Owner: "acme", Name: "widget"}},
		// A `:` inside an SCP path: the separator is the FIRST one and the rest are part of the path.
		{"git@github.com:acme/wi:get.git",
			model.RepoRef{Forge: "github", Host: "github.com", Project: "acme/wi:get", Owner: "acme", Name: "wi:get"}},
	}

	for _, c := range cases {
		got, ok := ParseRemoteURL(c.raw, hosts, nil)
		if !ok {
			t.Errorf("ParseRemoteURL(%q) said no, and it is one of the three forms", c.raw)
			continue
		}
		if got != c.want {
			t.Errorf("ParseRemoteURL(%q) gave %+v, want %+v: an extra separator is not a missing one",
				c.raw, got, c.want)
		}
	}
}
