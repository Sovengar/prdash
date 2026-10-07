package github

import (
	"context"
	"strings"
	"testing"

	"prdash/internal/forge/model"
)

func TestAResponseThatParsesGivesTheItemAndNotAWarning(t *testing.T) {
	dir := t.TempDir()

	withPR := `{"data":{"search":{"issueCount":1,"nodes":[{"__typename":"PullRequest","number":7,"title":"one","state":"OPEN","updatedAt":"2026-03-17T10:00:00Z","url":"https://github.com/acme/widget/pull/7","author":{"login":"alice"},"headRefName":"feat/x","baseRefName":"main","mergeable":"MERGEABLE"}]}}}`
	script := writeScript(t, dir, "gh", "#!/bin/sh\necho '"+withPR+"'\n")
	a := New("github.com", script)

	it, warns := a.ItemState(context.Background(),
		model.RepoRef{Forge: "github", Host: "github.com", Project: "acme/widget",
			Owner: "acme", Name: "widget"}, 7)

	if len(warns) != 0 {
		t.Errorf("a response that parses gave %d warnings: %+v. With the condition "+
			"inverted the good path returns a parse warning, and the PR comes out empty "+
			"without it being noticed why", len(warns), warns)
	}
	if it.Number != 7 {
		t.Errorf("the item came out with number %d, want 7: the response carries PR 7", it.Number)
	}
	if it.Title != "one" {
		t.Errorf("the item came out with title %q, want %q", it.Title, "one")
	}

	// The other half: a response that does NOT parse MUST give a warning, or the assertion above passes with the condition reversed.
	empty := `{"data":{"search":{"issueCount":0,"nodes":[]}}}`
	emptyScript := writeScript(t, dir, "gh", "#!/bin/sh\necho '"+empty+"'\n")
	emptyAdapter := New("github.com", emptyScript)
	_, warns = emptyAdapter.ItemState(context.Background(),
		model.RepoRef{Forge: "github", Host: "github.com", Project: "acme/widget",
			Owner: "acme", Name: "widget"}, 7)
	if len(warns) == 0 {
		t.Error("a response with no PRs gave no warning, and it should say that " +
			"that number was not found")
	}
	if warns[0].Kind != "notfound" && warns[0].Kind != "parse" {
		t.Errorf("the warning came out of kind %q, and what is expected here is notfound or parse", warns[0].Kind)
	}

	garbage := writeScript(t, dir, "gh", "#!/bin/sh\necho 'this is not json'\n")
	garbageAdapter := New("github.com", garbage)
	it, warns = garbageAdapter.ItemState(context.Background(),
		model.RepoRef{Forge: "github", Host: "github.com", Project: "acme/widget",
			Owner: "acme", Name: "widget"}, 7)
	if len(warns) == 0 {
		t.Fatal("a response that is not JSON gave no warning: the parse warning " +
			"is the only thing that says the API changed format")
	}
	if warns[0].Kind != "parse" {
		t.Errorf("an unreadable response gave a warning of kind %q, want parse", warns[0].Kind)
	}
	if !strings.Contains(strings.ToLower(warns[0].Msg), "json") {
		t.Errorf("the parse warning says %q and does not mention JSON: the message is what "+
			"tells the user whether to look at their network or the API", warns[0].Msg)
	}
	if it.Number != 0 {
		t.Errorf("an unreadable response returned item %d: without parsing there is no item", it.Number)
	}
}

func TestTheDefaultHostIsNotOverwrittenEither(t *testing.T) {
	for _, c := range []struct{ in, want string }{
		{"", "github.com"},
		{"ghe.example.com", "ghe.example.com"},
	} {
		if got := New(c.in, "").Host(); got != c.want {
			t.Errorf("New(%q).Host() = %q, want %q", c.in, got, c.want)
		}
	}
}
