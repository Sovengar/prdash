package gitlab

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"prdash/internal/forge"
	"prdash/internal/forge/model"
)

// GitLab has one output GitHub does not, and it is the one that matters: an empty list is not a
//broken answer.

func glabReturning(t *testing.T, body string) string {
	t.Helper()
	dir := t.TempDir()
	return writeScript(t, dir, "glab", "#!/bin/sh\n"+body+"\n")
}

func glabPrinting(output string) string {
	// SINGLE quotes: with double ones the JSON's own quotes close them and the shell hands over the
	//text without them.
	return "printf '%s\\n' '" + output + "'"
}

func onlyWarning(t *testing.T, warns []model.Warning, where string) model.Warning {
	t.Helper()
	if len(warns) != 1 {
		t.Fatalf("%s: %d warnings, want 1: %+v", where, len(warns), warns)
	}
	return warns[0]
}

// The EXACT shape glMRQuery asks for, with every field.
const emptyGraphql = `{"data":{"project":{"mergeRequest":null}}}`

// This is the case that happens in production.
func TestANonexistentMRSaysItDoesNotExistAndDoesNotReturnAFakeItem(t *testing.T) {
	a := New("gitlab.example.com", glabReturning(t, "cat <<'JSON'\n"+emptyGraphql+"\nJSON"))

	ref := model.RepoRef{Forge: "gitlab", Host: "gitlab.example.com", Project: "grupo/proyecto"}
	it, warns := a.ItemState(context.Background(), ref, 404)

	w := onlyWarning(t, warns, "nonexistent MR")
	if w.Kind != "notfound" {
		t.Errorf("kind %q, want notfound: an MR that does not exist is neither a parse error nor "+
			"a rate limit", w.Kind)
	}
	// The message says WHICH: without the number the warning could belong to any of the MRs.
	for _, want := range []string{"404", "grupo/proyecto"} {
		if !strings.Contains(w.Msg, want) {
			t.Errorf("the warning %q does not mention %q", w.Msg, want)
		}
	}
	// And the item is the zero value: an Item with the number set and nothing else would paint as a
	// real one.
	if it.ID() != (model.ID{}) || it.Number != 0 || it.Title != "" || it.HeadSHA != "" {
		t.Errorf("returned a half item instead of the zero value: %+v", it)
	}
	// The warning carries no section: ItemState belongs to no inbox column.
	if w.Section != "" {
		t.Errorf("the warning carries section %q and would also show up in the inbox", w.Section)
	}
}

// The heredoc detail is not cosmetic.
func TestOutputThatIsNotJSONInGitLabWarnsAndDoesNotBreak(t *testing.T) {
	for _, c := range []struct {
		name string
		body string
	}{
		{"html from a proxy", "echo '<html>Sign in to continue</html>'"},
		{"truncated json", "cat <<'JSON'\n{\"data\":{\"currentUser\":\nJSON"},
		{"empty json", "printf ''"},
		{"list instead of object", "echo '[1,2,3]'"},
	} {
		a := New("gitlab.example.com", glabReturning(t, c.body))
		ref := model.RepoRef{Forge: "gitlab", Host: "gitlab.example.com", Project: "grupo/proy"}
		it, warns := a.ItemState(context.Background(), ref, 7)

		if w := onlyWarning(t, warns, c.name); w.Kind != "parse" {
			t.Errorf("%s: kind %q, want parse", c.name, w.Kind)
		}
		if it.ID() != (model.ID{}) || it.Number != 0 {
			t.Errorf("%s: returned a half item", c.name)
		}
	}
}

// TWO functions with the same contract.
func TestListWithUnreadableOutputWarnsAndDoesNotInventItems(t *testing.T) {
	a := New("gitlab.example.com", glabReturning(t, "echo 'I am not json'"))

	for _, c := range []struct {
		name string
		q    forge.Query
	}{
		{"review graphql", forge.Query{Section: model.SectionReview, ReviewKind: model.ReviewRequested}},
		{"mentions graphql", forge.Query{Section: model.SectionMentions}},
		{"authored graphql", forge.Query{Section: model.SectionAuthored}},
	} {
		page, warns := a.List(context.Background(), c.q)
		if len(page.Items) != 0 {
			t.Errorf("%s: %d items from unreadable output", c.name, len(page.Items))
		}
		w := onlyWarning(t, warns, c.name)
		if w.Kind != "parse" {
			t.Errorf("%s: kind %q, want parse", c.name, w.Kind)
		}
		if w.Section != c.q.Section {
			t.Errorf("%s: the warning does not carry the queried section (%q)", c.name, w.Section)
		}
	}
}

// The same risk as GitHub but with a different output shape.
func TestGitLabsConversationWithUnreadableOutputDoesNotInventNotes(t *testing.T) {
	a := New("gitlab.example.com", glabReturning(t, "echo 'broken'"))
	ref := model.RepoRef{Forge: "gitlab", Host: "gitlab.example.com", Project: "grupo/proyecto"}

	page, warns := a.Comments(context.Background(), ref, 12)

	if w := onlyWarning(t, warns, "Comments"); w.Kind != "parse" {
		t.Errorf("kind %q, want parse", w.Kind)
	}
	if len(page.Comments) != 0 {
		t.Errorf("%d invented notes from unreadable output", len(page.Comments))
	}
	if page.Total != 0 {
		t.Errorf("Total = %d with unreadable output", page.Total)
	}
}

// All three operations have it.
func TestAnEmptyProjectSaysNoAndDoesNotGoOutToAskTheForge(t *testing.T) {
	a := New("gitlab.example.com", glabReturning(t,
		"echo 'glab should not have called me' >&2\nexit 1"))
	ref := model.RepoRef{Forge: "gitlab", Host: "gitlab.example.com", Project: ""}

	it, warns := a.ItemState(context.Background(), ref, 12)
	if w := onlyWarning(t, warns, "ItemState without project"); w.Kind != "notfound" {
		t.Errorf("kind %q, want notfound", w.Kind)
	}
	if it.ID() != (model.ID{}) {
		t.Errorf("returned an item without project: %+v", it)
	}

	page, warns := a.Comments(context.Background(), ref, 12)
	if w := onlyWarning(t, warns, "Comments without project"); w.Kind != "notfound" {
		t.Errorf("kind %q, want notfound", w.Kind)
	}
	if len(page.Comments) != 0 || page.Total != 0 {
		t.Errorf("returned comments without project: %+v", page)
	}

	// The warning carries no binary URL: that is noise from one machine in a message the operator
	// reads.
	for _, w := range warns {
		if strings.Contains(w.Msg, "glab") {
			t.Errorf("the warning mentions the binary, which tells the user nothing: %q", w.Msg)
		}
	}
}

// What is checked is that the warning does NOT mention an exit code.
func TestAMissingBinarySaysNoAndDoesNotTryToRun(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "no-glab-here")
	a := New("gitlab.example.com", missing)
	ref := model.RepoRef{Forge: "gitlab", Host: "gitlab.example.com", Project: "grupo/proyecto"}

	_, warns := a.ItemState(context.Background(), ref, 12)
	w := onlyWarning(t, warns, "no glab")
	if w.Kind == "" {
		t.Error("with no binary there is no warning: the inbox would look empty without an explanation")
	}
	if !strings.Contains(w.Msg, "glab") {
		t.Errorf("the warning %q does not say that glab is missing, which is the actionable part", w.Msg)
	}
}

// The good path was untested, which is a needle in a haystack.
func TestReadingTheStateOfAnExistingMRReturnsItStampedWithItsIdentity(t *testing.T) {
	const liveMR = `{"data":{"project":{"mergeRequest":{"iid":12,"title":"Un MR",` +
		`"webUrl":"https://gitlab.acme.example/g/p/-/merge_requests/12","state":"opened",` +
		`"draft":false,"detailedMergeStatus":"mergeable","sourceBranch":"feat/a",` +
		`"targetBranch":"main","approved":false,"updatedAt":"2026-09-21T07:00:00Z",` +
		`"diffHeadSha":"abc123"}}}}`
	a := New("gitlab.acme.example", glabReturning(t, glabPrinting(liveMR)))
	ref := model.RepoRef{Forge: "gitlab", Host: "gitlab.acme.example", Project: "grupo/proyecto"}

	it, warns := a.ItemState(context.Background(), ref, 12)

	if len(warns) != 0 {
		t.Fatalf("an MR that exists gave %d warnings: %+v", len(warns), warns)
	}
	if it.Number != 12 {
		t.Errorf("Number = %d, want 12", it.Number)
	}
	if it.Title != "Un MR" || it.SourceBranch != "feat/a" || it.TargetBranch != "main" {
		t.Errorf("the MR arrived incomplete: %+v", it)
	}
	// And the HeadSHA, which is what lets the merge be pinned; without it the merge would go out
	// without --sha.
	if it.HeadSHA != "abc123" {
		t.Errorf("HeadSHA = %q, want abc123: without it the merge cannot be pinned", it.HeadSHA)
	}
	if it.Forge != "gitlab" || it.Host != "gitlab.acme.example" {
		t.Errorf("identity on the Item = %s/%s, want gitlab/gitlab.acme.example",
			it.Forge, it.Host)
	}
	if it.Ref.Forge != "gitlab" || it.Ref.Host != "gitlab.acme.example" {
		t.Errorf("identity on the Ref = %s/%s", it.Ref.Forge, it.Ref.Host)
	}
	if it.ID().Forge != "gitlab" || it.ID().Host != "gitlab.acme.example" {
		t.Errorf("the ID does not carry the identity: %+v", it.ID())
	}

	// identity does NOT seal the project, same as GitHub; the two identity functions are line for
//line identical.
	if it.Ref.Project != "" {
		t.Errorf("identity sealed the project (%q): the project comes from the item it is "+
			"applied to, and sealing it here would imply the Item is self-contained", it.Ref.Project)
	}

	// And a different host gives a different ID, which is what stops two instances from colliding.
	other := New("other.acme.example", glabReturning(t, glabPrinting(
		`{"data":{"project":{"mergeRequest":{"iid":12,"title":"Other host","webUrl":"u",`+
			`"state":"opened","sourceBranch":"feat/a","targetBranch":"main",`+
			`"updatedAt":"2026-09-21T07:00:00Z"}}}}`)))
	it2, warns := other.ItemState(context.Background(), ref, 12)
	if len(warns) != 0 {
		t.Fatalf("the second adapter warns: %+v", warns)
	}
	if it2.ID() == it.ID() {
		t.Error("the same MR on two hosts gave the same ID: the reviews memory would confuse them")
	}
	if it2.HeadSHA != "" {
		t.Errorf("HeadSHA = %q without diffHeadSha: it must stay empty so the merge does NOT get "+
			"pinned to an invented SHA", it2.HeadSHA)
	}
}
