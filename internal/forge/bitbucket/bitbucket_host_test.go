package bitbucket

import "testing"

// This is not a detail: the adapter is for a host that is NOT in the default config.
func TestTheDefaultHostOnlyAppliesWithoutAHost(t *testing.T) {
	cases := []struct {
		input, want string
		note        string
	}{
		{"", "bitbucket.org", "no host: Bitbucket's, which is the only place this " +
			"adapter talks to"},
		{"bb.example.com", "bb.example.com", "custom host: self-hosted Bitbucket, which " +
			"is exactly what the parameter exists for"},
		{"bitbucket.org", "bitbucket.org", "the default written by hand: doesn't matter"},
		{"192.168.1.10:7990", "192.168.1.10:7990", "host with port, which is what " +
			"happens on an instance behind a proxy"},
	}
	for _, c := range cases {
		if got := New(c.input).Host(); got != c.want {
			t.Errorf("New(%q).Host() = %q, want %q. %s", c.input, got, c.want, c.note)
		}
	}

	// Nothing beyond that can be asserted: the adapter is inert, everything answers "not implemented".
}
