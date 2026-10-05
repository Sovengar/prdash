package parse

import (
	"strings"
	"testing"
)

func TestParseGLBranchesReadsEachPage(t *testing.T) {
	ndjson := `{"name":"main","commit":{"id":"abc"}}
{"name":"release/2.0"}
{"name":"fix/hunk-pane-argv","default":true}`

	names, err := ParseGLBranches(ndjson)
	if err != nil {
		t.Fatalf("ParseGLBranches = %v, want no error", err)
	}
	if got := strings.Join(names, ","); got != "main,release/2.0,fix/hunk-pane-argv" {
		t.Errorf("ParseGLBranches = %q, want the three branches in order", got)
	}
}

// A line that cannot be understood does not throw away the whole listing.
func TestParseGLBranchesSkipsTheStrangeLine(t *testing.T) {
	ndjson := "garbage\n{\"name\":\"main\"}\n"

	names, err := ParseGLBranches(ndjson)
	if err != nil {
		t.Fatalf("ParseGLBranches = %v, want the strange line not to take down the read", err)
	}
	if got := strings.Join(names, ","); got != "main" {
		t.Errorf("ParseGLBranches = %q, want only the readable branch", got)
	}
}

func TestParseGLBranchesWarnsWhenNothingWasRead(t *testing.T) {
	for _, in := range []string{"garbage\n", "[]\n", "{\"commit\":{}}\n"} {
		names, err := ParseGLBranches(in)
		if err == nil {
			t.Errorf("ParseGLBranches(%q) = %v, want error", in, names)
		}
	}
}

func TestParseGLBranchesCarriesTheCause(t *testing.T) {
	for _, in := range []string{"garbage\n", "[]\nnot-json\n"} {
		names, err := ParseGLBranches(in)
		if err == nil {
			t.Fatalf("ParseGLBranches(%q) = %v, want error", in, names)
		}
		if !strings.HasPrefix(err.Error(), "could not read the branch list: ") {
			t.Errorf("ParseGLBranches(%q) = %q, want the reason with the unmarshal cause", in, err)
		}
	}
}

func TestParseGLBranchesWithoutACauseDoesNotInventOne(t *testing.T) {
	names, err := ParseGLBranches(`{"commit":{"id":"abc"}}`)
	if err == nil {
		t.Fatalf("ParseGLBranches = %v, want error", names)
	}
	if got := err.Error(); got != "could not read the branch list" {
		t.Errorf("error = %q, want the warning without a cause", got)
	}
}

func TestParseGLBranchesAcceptsTrulyEmpty(t *testing.T) {
	names, err := ParseGLBranches("")
	if err != nil || len(names) != 0 {
		t.Errorf("ParseGLBranches(\"\") = %v, %v; want an empty list with no error", names, err)
	}
}

func TestParseGHBranchesSplitsLines(t *testing.T) {
	names := ParseGHBranches("main\nrelease/2.0\n\n  main  \n")
	if got := strings.Join(names, "|"); got != "main|release/2.0|main" {
		t.Errorf("ParseGHBranches = %q, want the three names with the whitespace trimmed", got)
	}
}
