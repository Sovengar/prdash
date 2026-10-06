package herdr

import (
	"strings"
	"testing"
)

// The CLI's envelope is {"id":...,"result":{...}} and EVERY command returns a .result.

// The envelope itself is a fake server payload: it stays as the server sent it.
const badEnvelope = `esto no es json`

// Each parser goes through decodeResult, so one covering only its own cases exercises a branch the
// other seven share.
func TestParsersRejectAnUnreadableEnvelope(t *testing.T) {
	cases := []struct {
		name  string
		parse func([]byte) error
	}{
		{"worktreeCreated", func(b []byte) error { _, err := parseWorktreeCreated(b); return err }},
		{"worktreeList", func(b []byte) error { _, err := parseWorktreeList(b); return err }},
		{"workspaceCreated", func(b []byte) error { _, err := parseWorkspaceCreated(b); return err }},
		{"tabCreated", func(b []byte) error { _, err := parseTabCreated(b); return err }},
		{"paneSplit", func(b []byte) error { _, err := parsePaneSplit(b); return err }},
		{"paneList", func(b []byte) error { _, err := parsePaneList(b); return err }},
		{"notification", func(b []byte) error {
			_, _, err := parseNotification(b)
			return err
		}},
	}
	for _, c := range cases {
		err := c.parse([]byte(badEnvelope))
		if err == nil {
			t.Errorf("%s gave no error with an unreadable envelope", c.name)
			continue
		}
		if !strings.Contains(err.Error(), "unreadable") {
			t.Errorf("%s gave %q, which does not say the envelope was unreadable", c.name, err)
		}
	}
}

// Valid JSON missing the key that makes the result mean something.
func TestParsersDemandTheirRequiredKey(t *testing.T) {
	cases := []struct {
		name   string
		json   string
		wantIn string
		parse  func([]byte) error
	}{
		{
			name: "worktreeCreated without path",
			json: `{"result":{"workspace":{"workspace_id":"w1"},"worktree":{"branch":"b"}}}`,
			// The error has to NAME the missing key.
			wantIn: "worktree.path",
			parse:  func(b []byte) error { _, err := parseWorktreeCreated(b); return err },
		},
		{
			name:   "workspaceCreated without workspace_id",
			json:   `{"result":{"tab":{"tab_id":"t1"}}}`,
			wantIn: "workspace.workspace_id",
			parse:  func(b []byte) error { _, err := parseWorkspaceCreated(b); return err },
		},
		{
			name:   "paneSplit without pane_id",
			json:   `{"result":{"pane":{"workspace_id":"w1"}}}`,
			wantIn: "pane.pane_id",
			parse:  func(b []byte) error { _, err := parsePaneSplit(b); return err },
		},
	}
	for _, c := range cases {
		err := c.parse([]byte(c.json))
		if err == nil {
			t.Errorf("%s: the parser accepted a result without the required key", c.name)
			continue
		}
		if !strings.Contains(err.Error(), c.wantIn) {
			t.Errorf("%s: the error was %q and does not name %q, which is what has to be fixed",
				c.name, err, c.wantIn)
		}
	}
}

// An empty list is a legitimate reply, not a broken one.
func TestEmptyListIsNotAnError(t *testing.T) {
	panes, err := parsePaneList([]byte(`{"result":{"panes":[]}}`))
	if err != nil {
		t.Fatalf("an empty pane list gave an error: %v", err)
	}
	if len(panes) != 0 {
		t.Errorf("an empty list returned %d panes", len(panes))
	}

	list, err := parseWorktreeList([]byte(`{"result":{"source":{"repo_root":"/r"},"worktrees":[]}}`))
	if err != nil {
		t.Fatalf("an empty worktree list gave an error: %v", err)
	}
	if len(list.Worktrees) != 0 {
		t.Errorf("an empty list returned %d worktrees", len(list.Worktrees))
	}
	if list.RepoRoot != "/r" {
		t.Errorf("with an empty list, RepoRoot = %q, want /r", list.RepoRoot)
	}
}

// Accepting both forms is because the exact server shape is unverified.
func TestParseServerErrorAcceptsBothForms(t *testing.T) {
	cases := []struct {
		name     string
		stderr   string
		wantCode string
		wantMsg  string
	}{
		{
			name:     "nested",
			stderr:   `{"error":{"code":"pane_gone","message":"the pane no longer exists"}}`,
			wantCode: "pane_gone",
			wantMsg:  "the pane no longer exists",
		},
		{
			name:     "flat",
			stderr:   `{"code":"pane_gone","message":"the pane no longer exists"}`,
			wantCode: "pane_gone",
			wantMsg:  "the pane no longer exists",
		},
		{
			name: "nested with code and no message",
			// The empty message is a real case: a server error may carry only the code.
			stderr:   `{"error":{"code":"timeout"}}`,
			wantCode: "timeout",
			wantMsg:  "",
		},
		{
			name:     "json that is not a server error",
			stderr:   `{"result":{"ok":true}}`,
			wantCode: "",
			wantMsg:  "",
		},
		{
			name:     "json array",
			stderr:   `[1,2,3]`,
			wantCode: "",
			wantMsg:  "",
		},
		{
			name:     "free text",
			stderr:   `error: no such pane`,
			wantCode: "",
			wantMsg:  "",
		},
		{
			name:     "empty",
			stderr:   `   `,
			wantCode: "",
			wantMsg:  "",
		},
	}
	for _, c := range cases {
		code, msg := parseServerError([]byte(c.stderr))
		if code != c.wantCode || msg != c.wantMsg {
			t.Errorf("%s: parseServerError gave (%q, %q), want (%q, %q)",
				c.name, code, msg, c.wantCode, c.wantMsg)
		}
	}
}

func TestRPCErrorWithoutMessageIsTheCode(t *testing.T) {
	if got := (&rpcError{Code: "timeout"}).Error(); got != "timeout" {
		t.Errorf("an rpcError without a message gave %q, want timeout", got)
	}
	if got := (&rpcError{Code: "timeout", Message: "slow"}).Error(); got != "timeout: slow" {
		t.Errorf("an rpcError with a message gave %q, want \"timeout: slow\"", got)
	}
}

// Both halves are used, which is why both are parsed.
func TestParseVersionSeparatesTheNumberFromTheText(t *testing.T) {
	cases := []struct {
		input  string
		want   Version
		wantOK bool
	}{
		{"herdr 0.9.1-preview.7\n", Version{Major: 0, Minor: 9, Patch: 1}, true},
		{"herdr v1.2.3\n", Version{Major: 1, Minor: 2, Patch: 3}, true},
		{"0.10.0", Version{Major: 0, Minor: 10, Patch: 0}, true},
		{"herdr 2.0.0", Version{Major: 2, Minor: 0, Patch: 0}, true},
		{"herdr unknown", Version{}, false},
		{"", Version{}, false},
		{"herdr", Version{}, false},
		// Two components is not a version. Returning 0.9 would invent a patch that is not there.
		{"herdr 0.9", Version{}, false},
	}
	for _, c := range cases {
		got, ok := parseVersion([]byte(c.input))
		if ok != c.wantOK {
			t.Errorf("parseVersion(%q) gave ok=%v, want %v", c.input, ok, c.wantOK)
			continue
		}
		if !c.wantOK {
			continue
		}
		if got.Major != c.want.Major || got.Minor != c.want.Minor || got.Patch != c.want.Patch {
			t.Errorf("parseVersion(%q) gave %d.%d.%d, want %d.%d.%d", c.input,
				got.Major, got.Minor, got.Patch, c.want.Major, c.want.Minor, c.want.Patch)
		}
		if strings.ContainsAny(got.Raw, "\n") {
			t.Errorf("parseVersion(%q) left the newline in Raw=%q", c.input, got.Raw)
		}
		if got.Raw == "" {
			t.Errorf("parseVersion(%q) returned an empty Raw: the diagnosis needs it", c.input)
		}
	}
}

// The clipping to the first line.
func TestFirstLineKeepsTheFirstOne(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		{"important warning\nanother thing\n", "important warning"},
		{"  warning  \n  another  ", "warning"},
		{"only one line", "only one line"},
		{"", ""},
		{"\n\nthe third", "the third"},
		{"with\n", "with"},
	}
	for _, c := range cases {
		if got := firstLine(c.input); got != c.want {
			t.Errorf("firstLine(%q) gave %q, want %q", c.input, got, c.want)
		}
	}
}

func TestHaveGraphicsTargetRequiresBoth(t *testing.T) {
	cases := []struct {
		socket, pane string
		want         bool
	}{
		{"/tmp/sock", "p1", true},
		{"", "p1", false},
		{"/tmp/sock", "", false},
		{"", "", false},
	}
	for _, c := range cases {
		if got := haveGraphicsTarget(c.socket, c.pane); got != c.want {
			t.Errorf("haveGraphicsTarget(%q, %q) gave %v, want %v", c.socket, c.pane, got, c.want)
		}
	}
}
