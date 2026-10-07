package github

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"prdash/internal/forge"
	"prdash/internal/forge/model"
)

// A single segment is not a repo.
func TestSplitProjectSeparatesTwoPartsAndNothingMore(t *testing.T) {
	cases := []struct {
		project     string
		wantOwner   string
		wantName    string
		description string
	}{
		{"owner/repo", "owner", "repo", "the normal case"},
		{"acme/widget", "acme", "widget", "with Organization"},
		{"/owner/repo", "owner", "repo", "leading slash"},
		{"owner/repo/", "owner", "repo", "trailing slash"},
		{"/owner/repo/", "owner", "repo", "slashes on both sides"},
		{"repo", "", "repo", "a single segment"},
		{"", "", "", "empty"},
		{"/", "", "", "only slashes"},
		{"///", "", "", "only slashes, several"},
		{"a/b/c", "a", "b/c", "three segments"},
	}

	for _, c := range cases {
		owner, name := splitProject(c.project)
		if owner != c.wantOwner || name != c.wantName {
			t.Errorf("%s: splitProject(%q) = %q, %q; want %q, %q",
				c.description, c.project, owner, name, c.wantOwner, c.wantName)
		}
		if strings.Contains(name, "/") && c.description != "three segments" {
			t.Errorf("%s: the name %q comes out with slashes", c.description, name)
		}
	}

	// The rule that matters, stated as a rule: owner and name, or it is not a repo.
	for _, project := range []string{"", "repo", "/", "///"} {
		owner, name := splitProject(project)
		if owner == "" || name == "" {
			continue
		}
		t.Errorf("splitProject(%q) gave owner %q and name %q, and a single segment is not a repo",
			project, owner, name)
	}
}

// The warning is of type "notfound", not "network": nothing was queried.
func TestAnInvalidRefDoesNotGoOutToTheNetwork(t *testing.T) {
	dir := t.TempDir()
	argsFile := filepath.Join(dir, "args.log")
	script := writeScript(t, dir, "gh", "#!/bin/sh\necho \"$@\" >> \""+argsFile+"\"\necho '{}'\n")
	a := New("h.example", script)

	for _, project := range []string{"", "repo", "/", "///"} {
		_, warns := a.ItemState(context.Background(), model.RepoRef{Project: project}, 1)
		if len(warns) == 0 {
			t.Errorf("project %q: no warning came out, it should say there is nothing to ask", project)
			continue
		}
		if warns[0].Kind != "notfound" {
			t.Errorf("project %q: the warning is of kind %q, want notfound", project, warns[0].Kind)
		}
	}
	if _, err := os.Stat(argsFile); err == nil {
		raw, _ := os.ReadFile(argsFile)
		t.Errorf("an invalid ref went out to the network:\n%s", raw)
	}

	_, _ = a.ItemState(context.Background(), model.RepoRef{Project: "acme/widget"}, 1)
	raw, err := os.ReadFile(argsFile)
	if err != nil {
		t.Fatal("with a valid ref it did not go out to the network: the guard swallowed the good call")
	}
	if !strings.Contains(string(raw), "acme") {
		t.Errorf("the call does not carry the project: %s", raw)
	}
}

func TestFailureMsgPrefersTheServersReason(t *testing.T) {
	err := errors.New("gh api graphql -f query=... (exit 1)")

	cases := []struct {
		name string
		body string
		want string
	}{
		{"with message", `{"message":"Reference 'x' does not exist"}`, "Reference 'x' does not exist"},
		{"with errors", `{"errors":[{"message":"Bad credentials"}]}`, "Bad credentials"},
		{"empty body", "", err.Error()},
		{"blank body", "   \n", err.Error()},
		{"not json", "404 page not found", err.Error()},
		{"json without message", `{"documentation_url":"https://docs"}`, err.Error()},
		{"empty json", `{}`, err.Error()},
		{"empty list", `[]`, err.Error()},
		{"empty message", `{"message":""}`, err.Error()},
	}

	for _, c := range cases {
		if got := failureMsg(c.body, err); got != c.want {
			t.Errorf("%s: failureMsg(%q) = %q, want %q", c.name, c.body, got, c.want)
		}
	}

	// What matters: the message is NEVER empty, because a failure with no text cannot be acted on.
	for _, body := range []string{"", "  ", "nothing", "{}", `{"message":""}`, `{"message":null}`} {
		if strings.TrimSpace(failureMsg(body, err)) == "" {
			t.Errorf("with body %q the message ended up empty", body)
		}
	}
	// And with an empty error either: the error's own message, empty text or not, is all there is.
}

// Same pattern as the binary: what is not set falls back.
func TestTheDefaultHostIsGithubAndTheRestAreNot(t *testing.T) {
	dir := t.TempDir()
	argsFile := filepath.Join(dir, "args.log")
	script := writeScript(t, dir, "gh", "#!/bin/sh\necho \"$@\" >> \""+argsFile+"\"\necho '{}'\n")

	for _, c := range []struct{ given, want string }{
		{"", "github.com"},
		{"github.com", "github.com"},
		{"h.example", "h.example"},
		{"gitlab.example.com", "gitlab.example.com"},
	} {
		a := New(c.given, script)
		if a.host != c.want {
			t.Errorf("New(%q) gave host %q, want %q", c.given, a.host, c.want)
		}
		// The host reaches the items, and NOT gh's argv: the binary is configured another way.
		items := []model.Item{{Number: 1}}
		a.stamp(items, forge.Query{Section: model.SectionReview})
		if items[0].Host != c.want {
			t.Errorf("with host %q the item ended up with host %q", c.want, items[0].Host)
		}
		if items[0].Ref.Host != c.want {
			t.Errorf("with host %q the item's ref ended up with host %q", c.want, items[0].Ref.Host)
		}
	}

	other := t.TempDir()
	logFile := filepath.Join(other, "args.log")
	fakeScript := writeScript(t, other, "fake", "#!/bin/sh\necho \"$0\" >> \""+logFile+"\"\necho '{}'\n")
	a := New("h.example", fakeScript)
	_, _ = a.ItemState(context.Background(), model.RepoRef{Project: "acme/widget"}, 1)
	if raw, _ := os.ReadFile(logFile); !strings.Contains(string(raw), fakeScript) {
		t.Errorf("the request did not go out through the given binary: %s", raw)
	}
}

// Not an interesting test on its own, but it is the existence condition of the rest.
func TestAFailingRunnerGivesAWarningAndNotAPanic(t *testing.T) {
	dir := t.TempDir()
	script := writeScript(t, dir, "gh", "#!/bin/sh\necho '{\"message\":\"boom\"}'\nexit 1\n")
	a := New("h.example", script)

	_, warns := a.ItemState(context.Background(), model.RepoRef{Project: "acme/widget"}, 1)
	if len(warns) == 0 {
		t.Fatal("a runner that fails gave no warning")
	}
	if strings.TrimSpace(warns[0].Msg) == "" {
		t.Errorf("the warning ended up empty: %+v", warns[0])
	}
	if warns[0].Forge != ForgeName {
		t.Errorf("the warning does not say which forge it is from: %q", warns[0].Forge)
	}
}

// The review kind belongs to the review section.
func TestStampPutsTheReviewKindOnlyInReview(t *testing.T) {
	for _, section := range []model.Section{model.SectionReview, model.SectionAuthored, model.SectionMentions} {
		a := New("h.example", "gh")
		items := []model.Item{{Number: 1, Ref: model.RepoRef{Forge: ForgeName, Host: "h.example", Project: "acme/widget", Owner: "acme", Name: "widget"}}}
		a.stamp(items, forge.Query{Section: section, ReviewKind: model.ReviewRequested})

		has := items[0].ReviewKind != ""
		want := section == model.SectionReview
		if has != want {
			t.Errorf("section %v: ReviewKind %q present=%v, want %v",
				section, items[0].ReviewKind, has, want)
		}
		// The section is stamped ALWAYS: without it the item does not know which column it belongs to.
		if items[0].Section != section {
			t.Errorf("section %v: ended up stamped as %v", section, items[0].Section)
		}
	}
}
