package herdr

import (
	"errors"
	"os"
	"testing"
)

const fixtureWorktreeCreated = `{
  "id": "cli:worktree:create",
  "result": {
    "type": "worktree_created",
    "workspace": {"workspace_id": "w18", "label": "prdash-pr-7"},
    "tab": {"tab_id": "w18:t1"},
    "root_pane": {"pane_id": "w18:p1"},
    "worktree": {
      "path": "/home/u/.herdr/worktrees/prdash/feat-x",
      "branch": "prdash/pr-7",
      "label": "repo",
      "is_linked_worktree": true,
      "is_prunable": false,
      "open_workspace_id": "w18"
    }
  }
}`

const fixtureWorktreeList = `{
  "id": "cli:worktree:list",
  "result": {
    "type": "worktree_list",
    "source": {
      "repo_key": "/repo/.git",
      "repo_name": "repo",
      "repo_root": "/repo",
      "source_checkout_path": "/repo",
      "source_workspace_id": "w17"
    },
    "worktrees": [
      {"path": "/repo", "branch": "main", "label": "repo", "is_linked_worktree": false, "is_prunable": false, "open_workspace_id": "w17"},
      {"path": "/repo.wt/prdash-pr-7", "branch": "prdash/pr-7", "label": "prdash-pr-7", "is_linked_worktree": true, "is_prunable": false, "open_workspace_id": ""}
    ]
  }
}`

const fixtureWorkspaceCreated = `{
  "id": "cli:workspace:create",
  "result": {
    "type": "workspace_created",
    "workspace": {"workspace_id": "w20", "label": "review"},
    "tab": {"tab_id": "w20:t1"},
    "root_pane": {"pane_id": "w20:p1"}
  }
}`

const fixtureTabCreated = `{
  "id": "cli:tab:create",
  "result": {
    "type": "tab_created",
    "tab": {"tab_id": "w20:t2"},
    "root_pane": {"pane_id": "w20:p5"}
  }
}`

const fixturePaneSplit = `{
  "id": "cli:pane:split",
  "result": {
    "type": "pane_info",
    "pane": {"pane_id": "w18:p2", "workspace_id": "w18", "tab_id": "w18:t1", "cwd": "/repo.wt/prdash-pr-7", "label": ""}
  }
}`

const fixturePaneList = `{
  "id": "cli:pane:list",
  "result": {
    "type": "pane_list",
    "panes": [
      {"pane_id": "w18:p1", "workspace_id": "w18", "tab_id": "w18:t1", "cwd": "/repo.wt/prdash-pr-7", "label": "TUICR"},
      {"pane_id": "w18:p2", "workspace_id": "w18", "tab_id": "w18:t1", "cwd": "/repo.wt/prdash-pr-7", "label": "Hunk"}
    ]
  }
}`

const fixtureNotification = `{
  "id": "cli:notification:show",
  "result": {"type": "notification_show", "shown": true, "reason": ""}
}`

// A misplaced `>` disables or enables a capability wrongly: the versioning decides usability.
func TestVersionAtLeastComparesCascade(t *testing.T) {
	min := Version{Major: 0, Minor: 9, Patch: 3}
	cases := []struct {
		name string
		v    Version
		want bool
	}{
		{"the exact minimum", Version{0, 9, 3, ""}, true},
		{"patch above", Version{0, 9, 4, ""}, true},
		{"minor above, patch below", Version{0, 10, 0, ""}, true},
		{"major above with everything below", Version{1, 0, 0, ""}, true},
		{"patch below", Version{0, 9, 2, ""}, false},
		{"minor below", Version{0, 8, 9, ""}, false},
		{"major below", Version{0, 0, 0, ""}, false},
		{"all zero against all zero", Version{0, 0, 0, ""}, true},
	}
	for _, c := range cases[:len(cases)-1] {
		t.Run(c.name, func(t *testing.T) {
			if got := c.v.AtLeast(min); got != c.want {
				t.Errorf("Version(%s).AtLeast(%s) = %v, want %v", c.v, min, got, c.want)
			}
		})
	}
	// The 0.0.0 minimum is not a rare case: it is what arrives when the version cannot be read.
	if !(Version{}).AtLeast(Version{}) {
		t.Error("0.0.0 should meet the 0.0.0 minimum: it is the degradation when the version is not read")
	}
	if (Version{}).AtLeast(min) {
		t.Error("0.0.0 should not meet a 0.9.3 minimum")
	}
	// And a huge version qualifies, which is what makes Herdr's 0.x versioning useless here.
	if !(Version{99, 0, 0, ""}).AtLeast(min) {
		t.Error("a very high major should meet any 0.x minimum")
	}
	// Only a positive-major minimum reaches the major-below return: 0.x minimums fall through to the minor comparison.
	if (Version{0, 9, 3, ""}).AtLeast(Version{1, 0, 0, ""}) {
		t.Error("0.9.3 should not meet a 1.0.0 minimum")
	}
}

func TestErrorComposesTheMessageWithWhatIsThere(t *testing.T) {
	cases := []struct {
		name string
		err  *Error
		want string
	}{
		{
			"message only",
			&Error{Args: []string{"pane", "list"}, Msg: "pane not found"},
			"herdr [pane list]: pane not found",
		},
		{
			"with server code",
			&Error{Args: []string{"pane", "list"}, Msg: "pane not found", Code: "E_NOENT"},
			"herdr [pane list]: pane not found [E_NOENT]",
		},
		{
			"with exit code",
			&Error{Args: []string{"pane", "list"}, Msg: "pane not found", Exit: 2},
			"herdr [pane list]: pane not found (exit 2)",
		},
		{
			"with both",
			&Error{Args: []string{"pane", "list"}, Msg: "pane not found", Exit: 2, Code: "E_NOENT"},
			"herdr [pane list]: pane not found [E_NOENT] (exit 2)",
		},
		{
			// An exit 0 with a non-zero code is odd but possible when the process is killed.
			"without args",
			&Error{Msg: "boom"},
			"herdr []: boom",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := c.err.Error(); got != c.want {
				t.Errorf("Error() = %q, want %q", got, c.want)
			}
		})
	}
	// The cause is preserved by Unwrap: classifying without depending on the text.
	cause := os.ErrNotExist
	err := &Error{Msg: "x", Err: cause}
	if !errors.Is(err, cause) {
		t.Error("errors.Is should find the cause through the *Error")
	}
	if (&Error{Msg: "x"}).Unwrap() != nil {
		t.Error("without a cause, Unwrap should return nil")
	}
}

func TestParseWorktreeCreated(t *testing.T) {
	info, err := parseWorktreeCreated([]byte(fixtureWorktreeCreated))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if info.WorkspaceID != "w18" || info.TabID != "w18:t1" || info.RootPaneID != "w18:p1" {
		t.Fatalf("container = %+v", info)
	}
	if info.Path != "/home/u/.herdr/worktrees/prdash/feat-x" || info.Branch != "prdash/pr-7" {
		t.Fatalf("worktree = %+v", info)
	}
	// The ownership label comes from the workspace (--label); worktree.label is the worktree's name.
	if info.WorkspaceLabel != "prdash-pr-7" || info.Label != "repo" {
		t.Fatalf("labels = %+v", info)
	}
	if !info.IsLinkedWorktree || info.OpenWorkspaceID != "w18" {
		t.Fatalf("flags = %+v", info)
	}
}

func TestParseWorktreeList(t *testing.T) {
	list, err := parseWorktreeList([]byte(fixtureWorktreeList))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if list.RepoRoot != "/repo" || len(list.Worktrees) != 2 {
		t.Fatalf("list = %+v", list)
	}
	if list.Worktrees[1].Path != "/repo.wt/prdash-pr-7" || !list.Worktrees[1].IsLinkedWorktree {
		t.Fatalf("worktree[1] = %+v", list.Worktrees[1])
	}
}

func TestParseWorkspaceAndTabAndPane(t *testing.T) {
	ws, err := parseWorkspaceCreated([]byte(fixtureWorkspaceCreated))
	if err != nil || ws.WorkspaceID != "w20" || ws.RootPaneID != "w20:p1" {
		t.Fatalf("workspace = %+v, err=%v", ws, err)
	}
	tab, err := parseTabCreated([]byte(fixtureTabCreated))
	if err != nil || tab.TabID != "w20:t2" || tab.RootPaneID != "w20:p5" {
		t.Fatalf("tab = %+v, err=%v", tab, err)
	}
	pane, err := parsePaneSplit([]byte(fixturePaneSplit))
	if err != nil || pane.PaneID != "w18:p2" || pane.Cwd != "/repo.wt/prdash-pr-7" {
		t.Fatalf("pane = %+v, err=%v", pane, err)
	}
	panes, err := parsePaneList([]byte(fixturePaneList))
	if err != nil || len(panes) != 2 || panes[1].Label != "Hunk" {
		t.Fatalf("panes = %+v, err=%v", panes, err)
	}
}

func TestParseNotification(t *testing.T) {
	shown, reason, err := parseNotification([]byte(fixtureNotification))
	if err != nil || !shown || reason != "" {
		t.Fatalf("notification shown=%v reason=%q err=%v", shown, reason, err)
	}
}

func TestParseVersion(t *testing.T) {
	v, ok := parseVersion([]byte("herdr 0.9.1-preview.2026-09-21-0ff0f27e2226\n"))
	if !ok || v.Major != 0 || v.Minor != 9 || v.Patch != 1 {
		t.Fatalf("version = %+v ok=%v", v, ok)
	}
	if !v.AtLeast(MinVersion) {
		t.Fatalf("%s should be >= %s", v, MinVersion)
	}
	if (Version{Major: 0, Minor: 8, Patch: 2}).AtLeast(MinVersion) {
		t.Fatal("0.8.2 should not reach the minimum")
	}
	if _, ok := parseVersion([]byte("nope")); ok {
		t.Fatal("without a semver there should be no version")
	}
}

func TestParseMalformedResult(t *testing.T) {
	if _, err := parseWorktreeCreated([]byte("{")); err == nil {
		t.Fatal("an unreadable response should fail, not panic")
	}
	if _, err := parsePaneSplit([]byte(`{"id":"x"}`)); err == nil {
		t.Fatal("an envelope without a useful .result should fail")
	}
}

func TestParseServerErrorVariants(t *testing.T) {
	code, msg := parseServerError([]byte(`{"error":{"code":"worktree_create_failed","message":"no such branch"}}`))
	if code != "worktree_create_failed" || msg != "no such branch" {
		t.Fatalf("variant error = %q %q", code, msg)
	}
	code, msg = parseServerError([]byte(`{"code":"x","message":"y"}`))
	if code != "x" || msg != "y" {
		t.Fatalf("variant flat = %q %q", code, msg)
	}
	if code, msg := parseServerError([]byte("no json")); code != "" || msg != "" {
		t.Fatalf("text without json = %q %q", code, msg)
	}
}

func TestHerdrErrorThatIsNotJSONGetsNoCodeOrReasonAndInventsNone(t *testing.T) {
	for _, c := range []struct {
		name   string
		stderr string
	}{
		{"plain CLI text", "workspace_limit\n"},
		{"plain text with spaces", "pane w18:p1 not found"},
		{"usage help", "usage: herdr pane graphics set [--pane ID]"},
		// An `error` with no `message` is NOT here: the code is still read.
		{"error that is not an object", `{"error":"algo"}`},
		{"map without error or code", `{"other":"value"}`},
		// JSON-RPC's code is a NUMBER —{"code":-32601,...}.
		{"JSON-RPC code, which is numeric", `{"code":-32601,"message":"method not found"}`},
		{"loose numeric code", `{"code":42}`},
		{"array", `[1,2,3]`},
		{"json with trailing", `{"code":"x"} basura`},
	} {
		code, msg := parseServerError([]byte(c.stderr))
		if code != "" || msg != "" {
			t.Errorf("%s: it gave code=%q msg=%q, and a text that cannot be read has to come "+
				"back empty: an invented code classifies the operation as something it is not",
				c.name, code, msg)
		}
	}

	for _, c := range []struct {
		stderr   string
		wantCode string
		wantMsg  string
	}{
		{`{"error":{"code":"pane_not_found","message":"no such pane"}}`, "pane_not_found", "no such pane"},
		{`{"code":"rate_limited","message":"slow down"}`, "rate_limited", "slow down"},
		{`{"error":{"code":"workspace_limit"}}`, "workspace_limit", ""},
		{`{"other":"value"}`, "", ""},
	} {
		code, msg := parseServerError([]byte(c.stderr))
		if code != c.wantCode || msg != c.wantMsg {
			t.Errorf("%s: gave (%q, %q), want (%q, %q)", c.stderr, code, msg, c.wantCode, c.wantMsg)
		}
	}

	for _, empty := range []string{"", "   ", "\n\n"} {
		if code, msg := parseServerError([]byte(empty)); code != "" || msg != "" {
			t.Errorf("an empty stderr gave (%q, %q)", code, msg)
		}
	}
}
