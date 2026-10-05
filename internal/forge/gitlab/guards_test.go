package gitlab

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"prdash/internal/forge"
	"prdash/internal/forge/model"
)

func glabThatLogs(t *testing.T, body string) (script, argsFile string) {
	t.Helper()
	dir := t.TempDir()
	argsFile = filepath.Join(dir, "args.log")
	script = writeScript(t, dir, "glab", "#!/bin/sh\necho \"$@\" >> \""+argsFile+"\"\n"+body)
	return script, argsFile
}

func loggedArgs(t *testing.T, path string) string {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("no call was logged: %v", err)
	}
	return string(raw)
}

// The Todos API's cursor is the page NUMBER, and it is only used if it really is a number.
func TestTheAPIsCursorIsThePageNumberAndOnlyIfItIsANumber(t *testing.T) {
	cases := []struct {
		cursor string
		want   string
	}{
		{"", "page=1"},
		{"1", "page=1"},
		{"2", "page=2"},
		{"7", "page=7"},
		{"999", "page=999"},
		{"0", "page=1"},
		{"-1", "page=1"},
		{"-99", "page=1"},
		{"abc", "page=1"},
		{"1abc", "page=1"},
		{"1.5", "page=1"},
		{" 1", "page=1"},
		{"1 ", "page=1"},
		{"0x10", "page=1"},
		{"+1", "page=1"},
	}

	for _, c := range cases {
		t.Run("cursor="+c.cursor, func(t *testing.T) {
			script, argsFile := glabThatLogs(t, `echo '[]'
`)
			a := New("h.example", script)
			a.List(context.Background(), forge.Query{Section: model.SectionMentions, Cursor: c.cursor})

			log := loggedArgs(t, argsFile)
			if !strings.Contains(log, c.want) {
				t.Errorf("with cursor %q it asked for %q and %q was expected:\n%s", c.cursor, c.want, c.want, log)
			}
			// ONE page was asked for, not two. Careful counting: the args also carry "per_page=50".
			if n := strings.Count(log, " -f page="); n != 1 {
				t.Errorf("with cursor %q it asked for %d pages:\n%s", c.cursor, n, log)
			}
		})
	}
}

// When the server sends a reason, the warning teaches it.
func TestA400FromGitLabSaysWhatTheServerSays(t *testing.T) {
	script, _ := glabThatLogs(t, `echo "{\"message\":\"Reference does not exist on the remote\"}"
exit 1
`)
	a := New("h.example", script)
	warns := a.Retarget(context.Background(), model.RepoRef{Project: "grupo/proy"}, 3, "main")
	if len(warns) == 0 {
		t.Fatal("a runner that fails gave no warning")
	}
	if !strings.Contains(warns[0].Msg, "does not exist on the remote") {
		t.Errorf("the warning does not mention what the server said: %q", warns[0].Msg)
	}
	// And it does not say only the exit code, which is exactly what has to be avoided.
	if strings.TrimSpace(warns[0].Msg) == "exit status 1" {
		t.Errorf("the warning is only the exit code: %q", warns[0].Msg)
	}

	script, _ = glabThatLogs(t, `exit 1
`)
	a = New("h.example", script)
	warns = a.Retarget(context.Background(), model.RepoRef{Project: "grupo/proy"}, 3, "main")
	if len(warns) == 0 {
		t.Fatal("a runner that fails without a body gave no warning")
	}
	if strings.TrimSpace(warns[0].Msg) == "" {
		t.Error("the warning ended up empty: a failure with no text informs of nothing")
	}

	script, _ = glabThatLogs(t, `echo 'I am not json'
exit 1
`)
	a = New("h.example", script)
	warns = a.Retarget(context.Background(), model.RepoRef{Project: "grupo/proy"}, 3, "main")
	if len(warns) == 0 || strings.TrimSpace(warns[0].Msg) == "" {
		t.Errorf("with an unreadable body the warning ended up empty: %+v", warns)
	}
}

// The warning is "unsupported", not a network one: nothing was sent.
func TestRetargetWithoutABaseBranchDoesNotGoOutToTheNetwork(t *testing.T) {
	for _, branch := range []string{"", "   ", "\t\n"} {
		script, argsFile := glabThatLogs(t, `echo '{}'
`)
		a := New("h.example", script)
		warns := a.Retarget(context.Background(), model.RepoRef{Project: "grupo/proy"}, 3, branch)
		if len(warns) == 0 {
			t.Errorf("branch %q: no warning came out", branch)
			continue
		}
		if warns[0].Kind != "unsupported" {
			t.Errorf("branch %q: the warning is of kind %q, want unsupported: the request makes no sense, it is not a network failure",
				branch, warns[0].Kind)
		}
		if _, err := os.Stat(argsFile); err == nil {
			t.Errorf("branch %q: it went out to the network:\n%s", branch, loggedArgs(t, argsFile))
		}
	}
	script, argsFile := glabThatLogs(t, `echo '{}'
`)
	a := New("h.example", script)
	_ = a.Retarget(context.Background(), model.RepoRef{Project: "grupo/proy"}, 3, "main")
	if _, err := os.Stat(argsFile); err != nil {
		t.Error("with a branch it did not go out to the network: the guard swallowed the good call")
	}
}

// A guard before the call, so it has to be a "not found", not a failure.
func TestAnEmptyRefDoesNotGoOutToTheNetwork(t *testing.T) {
	script, argsFile := glabThatLogs(t, `echo '{}'
`)
	a := New("h.example", script)

	_, warns := a.ItemState(context.Background(), model.RepoRef{Project: ""}, 1)
	if len(warns) == 0 {
		t.Fatal("an empty ref gave no warning: it should say there is nothing to ask")
	}
	if warns[0].Kind != "notfound" {
		t.Errorf("the warning is of kind %q, want notfound: it is not that the MR is missing, it is that there is nothing to ask",
			warns[0].Kind)
	}
	// And it did not reach the network, which is the point of the guard.
	if _, err := os.Stat(argsFile); err == nil {
		t.Errorf("an empty ref went out to the network:\n%s", loggedArgs(t, argsFile))
	}

	_, _ = a.ItemState(context.Background(), model.RepoRef{Project: "grupo/proy"}, 1)
	if _, err := os.Stat(argsFile); err != nil {
		t.Error("with a ref it did not go out to the network: the guard swallowed the good call")
	}
}

func TestGitLabsStampPutsTheReviewKindOnlyInReview(t *testing.T) {
	for _, section := range []model.Section{model.SectionReview, model.SectionAuthored, model.SectionMentions} {
		a := New("h.example", "glab")
		items := []model.Item{{Number: 1, Ref: model.RepoRef{
			Forge: ForgeName, Host: "h.example", Project: "grupo/proy", Owner: "grupo", Name: "proy"}}}
		a.stamp(items, forge.Query{Section: section, ReviewKind: model.ReviewRequested})

		has := items[0].ReviewKind != ""
		want := section == model.SectionReview
		if has != want {
			t.Errorf("section %v: ReviewKind %q present=%v, want %v",
				section, items[0].ReviewKind, has, want)
		}
		// The section is always stamped: without it the item does not know which list it belongs to.
		if items[0].Section != section {
			t.Errorf("section %v: ended up stamped as %v", section, items[0].Section)
		}
	}
}
