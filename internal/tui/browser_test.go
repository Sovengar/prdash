package tui

import (
	"errors"
	"runtime"
	"strings"
	"testing"
)

// The table is pinned per platform, not taken from the system.
func TestTheOpenerIsChosenPerPlatformAndNotTakenFromTheSystem(t *testing.T) {
	url := "https://github.com/acme/project/pull/1"

	cases := []struct {
		so       string
		wantBin  string
		wantArgs []string
	}{
		{"linux", "xdg-open", []string{url}},
		{"darwin", "open", []string{url}},
		{"windows", "rundll32", []string{"url.dll,FileProtocolHandler", url}},
	}
	for _, c := range cases {
		bin, args := browserCommand(c.so, url)
		if bin != c.wantBin {
			t.Errorf("%s: gave the binary %q, want %q", c.so, bin, c.wantBin)
		}
		if len(args) != len(c.wantArgs) {
			t.Fatalf("%s: gave %d arguments %q, want %d %q", c.so, len(args), args,
				len(c.wantArgs), c.wantArgs)
		}
		for i := range c.wantArgs {
			if args[i] != c.wantArgs[i] {
				t.Errorf("%s: argv[%d] = %q, want %q (full %q)", c.so, i, args[i],
					c.wantArgs[i], args)
			}
		}
	}

	// And what cannot happen on any platform: an interpreter with the URL as code.
	prohibidos := []string{"sh", "bash", "zsh", "cmd", "cmd.exe", "powershell", "pwsh"}
	for _, so := range []string{"linux", "darwin", "windows", "freebsd", "plan9", ""} {
		bin, args := browserCommand(so, url)
		for _, bad := range prohibidos {
			if bin == bad {
				t.Errorf("%s: uses %q, which would interpret the URL", so, bin)
			}
		}
		// The URL always travels as its own argument, never glued to a -c.
		for _, a := range args {
			switch a {
			case "-c", "-Command", "/c":
				t.Errorf("%s: passes %q, which would interpret whatever comes after", so, a)
			}
		}
		if len(args) == 0 {
			t.Errorf("%s: no arguments, it would open nothing", so)
		}
		if args[len(args)-1] != url {
			t.Errorf("%s: the last argument is not the URL: %q", so, args)
		}
	}
}

// The two boundaries of the input.
func TestAnEmptyURLDoesNotOpenAndOneWithSpacesIsNotSplit(t *testing.T) {
	for _, so := range []string{"linux", "darwin", "windows"} {
		bin, args := browserCommand(so, "")
		if bin == "" {
			t.Errorf("%s: empty URL gave an empty binary", so)
		}
		if len(args) != 1 || args[0] != "" {
			t.Logf("%s: browserCommand with an empty URL gave %q; the guard lives in the Cmd", so, args)
		}

		withSpaces := "https://github.com/acme/project/tree/mi rama"
		_, args = browserCommand(so, withSpaces)
		encontrada := 0
		for _, a := range args {
			if a == withSpaces {
				encontrada++
			}
		}
		if encontrada != 1 {
			t.Errorf("%s: a URL with spaces gave %q, and it must travel as one whole "+
				"argument", so, args)
		}
	}

	// And on the OS it is running on right now: the table has to cover it.
	if bin, args := browserCommand(runtime.GOOS, "https://x"); bin == "" || len(args) == 0 {
		t.Errorf("the current OS (%s) is not in the table: bin=%q args=%q", runtime.GOOS, bin, args)
	}
}

// The three things the function does NOT do.
func TestTheURLIsNeitherDecoratedNorClipped(t *testing.T) {
	url := "https://github.com/acme/project/pull/1?tab=readme#diff-123"
	_, args := browserCommand("linux", url)
	if len(args) != 1 || args[0] != url {
		t.Errorf("the URL was decorated or truncated: %q", args)
	}

	double := "https://https://x"
	_, args = browserCommand("linux", double)
	if args[0] != double {
		t.Errorf("a URL with a scheme was rewritten: %q", args[0])
	}

	relative := "/acme/project/pull/1"
	_, args = browserCommand("linux", relative)
	if args[0] != relative {
		t.Errorf("a relative URL was decorated: %q", args[0])
	}
	// And with an odd scheme: as given. Deciding what is valid is not this layer's job.
	odd := "file:///etc/passwd"
	bin, args := browserCommand("linux", odd)
	if args[0] != odd {
		t.Errorf("an odd scheme was rewritten: %q", args[0])
	}
	if strings.ContainsAny(bin, " \t") {
		t.Errorf("the binary carries spaces: %q", bin)
	}
}

// This is the default.
func TestAnEmptyURLSaysThereIsNothingAndRunsNothing(t *testing.T) {
	m := newTestModel(t)
	msg, ok := m.openBrowserCmd("")().(notifyMsg)
	if !ok {
		t.Fatal("the command with an empty URL did not return a notifyMsg")
	}
	if msg.level != levelWarn {
		t.Errorf("level %v, want warn: opening an empty URL is not an error", msg.level)
	}
	if strings.TrimSpace(msg.text) == "" {
		t.Error("the notice came out empty, indistinguishable from not warning at all")
	}
	if strings.HasPrefix(msg.text, "opening ") {
		t.Errorf("the notice says %q, which is the happy-path text with a URL behind it", msg.text)
	}
	// With a URL this path is not taken: the text has to carry the URL.
	opened := []string{}
	m.openURL = func(url string) error {
		opened = append(opened, url)
		return nil
	}
	const url = "https://github.com/acme/project/pull/1"
	withURL := m.openBrowserCmd(url)()
	if notice, isNotice := withURL.(notifyMsg); isNotice &&
		strings.HasPrefix(notice.text, "nothing to open") {
		t.Errorf("with a URL it gave the empty notice: %q", notice.text)
	}
	if len(opened) != 1 || opened[0] != url {
		t.Errorf("opened %v, want exactly [%s]", opened, url)
	}
	if notice, isNotice := withURL.(notifyMsg); !isNotice || notice.text != "opening "+url {
		t.Errorf("the happy-path notice is %v, want %q", withURL, "opening "+url)
	}
	if notice, isNotice := withURL.(notifyMsg); isNotice && notice.level != levelInfo {
		t.Errorf("opening fine is info, neither an error nor a warning: %v", notice.level)
	}

	// And with an empty URL nothing is opened, which is the other side of the guard.
	opened = nil
	empty := m.openBrowserCmd("")()
	if len(opened) != 0 {
		t.Errorf("an empty URL reached the opener: %v", opened)
	}
	// What comes out is the warning above, not a nil: a tea Cmd is always non-nil.
	if notice, isNotice := empty.(notifyMsg); !isNotice || notice.level != levelWarn {
		t.Errorf("an empty URL gave %v, want the warning notice", empty)
	}

	m.openURL = func(string) error { return errors.New("no display") }
	msg, ok = m.openBrowserCmd(url)().(notifyMsg)
	if !ok {
		t.Fatal("an opener that failed did not return a notifyMsg")
	}
	if !strings.Contains(msg.text, "no display") {
		t.Errorf("the notice does not carry the opener failure: %q", msg.text)
	}
	if msg.level != levelError {
		t.Errorf("opening and failing is an error (level %v), not a warning: the user pressing o "+
			"deserves to know that nothing opened", msg.level)
	}
}
