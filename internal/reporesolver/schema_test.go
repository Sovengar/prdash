package reporesolver

import "testing"

func testHosts() map[string]string {
	return map[string]string{"github.com": "github"}
}

// An entry starting with `://` is neither a schemed URL nor SCP, and is not normalised as if it
// were.
func TestAMissingSchemeDoesNotSlipThroughTheSCPBranch(t *testing.T) {
	cases := []struct {
		raw  string
		note string
	}{
		{"://git@github.com:acme/widget.git", "missing scheme with an SCP shape behind it: the case " +
			"the `>= 0` condition was put there to close"},
		{"://github.com/acme/widget", "missing scheme, no SCP behind it"},
		{"://git@gitlab.com:group/proj", "another host, so it is not a github-only thing"},
	}
	for _, c := range cases {
		ref, ok := ParseRemoteURL(c.raw, testHosts(), nil)
		if ok {
			t.Errorf("%q normalised to %+v: it should not. Without a scheme it is not a git URL, "+
				"and letting it through the SCP branch produces a repo that EXISTS and on which "+
				"work would be done. %s", c.raw, ref, c.note)
		}
	}

	// The good side of the same boundary: a scheme at position one or more goes through the URL branch
	//and is normalised. With `> 0` this would also work, which is why it is pinned.
	for _, raw := range []string{"https://github.com/acme/widget", "ssh://git@github.com/acme/widget.git"} {
		ref, ok := ParseRemoteURL(raw, testHosts(), nil)
		if !ok {
			t.Errorf("%q did not normalise and should have: it has a scheme", raw)
			continue
		}
		if ref.Host != "github.com" {
			t.Errorf("%q gave host %q, want github.com", raw, ref.Host)
		}
	}
}

// The condition is `at > 0`, which demands something before the @.
func TestAnSCPWithAnEmptyUserIsNotARemote(t *testing.T) {
	cases := []string{
		"@github.com:acme/widget",
		"@github.com",
		"git@github.com",       // no path behind it
		"git@github.com:acme",  // a single path segment
		"git@github.com:acme/", // empty segment at the end
		"git@github.com:/acme", // empty segment at the start
	}
	for _, raw := range cases {
		if ref, ok := ParseRemoteURL(raw, testHosts(), nil); ok {
			t.Errorf("%q normalised to %+v and should not: without a user, without a colon or with a "+
				"single-segment path it is not a repo", raw, ref)
		}
	}

	for _, raw := range []string{"git@github.com:acme/widget", "git@github.com:acme/widget.git"} {
		ref, ok := ParseRemoteURL(raw, testHosts(), nil)
		if !ok {
			t.Errorf("%q did not normalise and should have", raw)
			continue
		}
		if ref.Host != "github.com" || ref.Project != "acme/widget" {
			t.Errorf("%q gave %+v", raw, ref)
		}
	}
}
