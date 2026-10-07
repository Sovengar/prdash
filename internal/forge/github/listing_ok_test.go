package github

import (
	"context"
	"testing"

	"prdash/internal/forge"
	"prdash/internal/forge/model"
)

const searchWithPRs = `{"data":{"search":{"issueCount":2,"pageInfo":` +
	`{"hasNextPage":true,"endCursor":"CUR2"},"nodes":[` +
	`{"__typename":"PullRequest","number":7,"title":"one","state":"OPEN",` +
	`"updatedAt":"2026-03-17T10:00:00Z","url":"https://github.com/acme/widget/pull/7",` +
	`"author":{"login":"alice"},"headRefName":"feat/x","baseRefName":"main",` +
	`"mergeable":"MERGEABLE","isDraft":false,` +
	`"repository":{"nameWithOwner":"acme/widget","name":"widget",` +
	`"owner":{"login":"acme"},"mergeCommitAllowed":true,"rebaseMergeAllowed":true,` +
	`"squashMergeAllowed":true}},` +
	`{"__typename":"PullRequest","number":8,"title":"dos","state":"OPEN",` +
	`"updatedAt":"2026-03-18T10:00:00Z","url":"https://github.com/acme/widget/pull/8",` +
	`"author":{"login":"bob"},"headRefName":"fix/y","baseRefName":"main",` +
	`"mergeable":"MERGEABLE","isDraft":false,` +
	`"repository":{"nameWithOwner":"acme/widget","name":"widget",` +
	`"owner":{"login":"acme"},"mergeCommitAllowed":true,"rebaseMergeAllowed":true,` +
	`"squashMergeAllowed":true}}]}}}`

// It goes through printf with SINGLE quotes and not a heredoc.
func ghPrinting(s string) string {
	return `printf '%s\n' '` + s + `'`
}

func TestListWithAValidResponseReturnsTheItemsAndPagination(t *testing.T) {
	a := New("github.com", ghReturning(t, ghPrinting(searchWithPRs)))
	q := forge.Query{Section: model.SectionReview, ReviewKind: model.ReviewRequested}

	page, warns := a.List(context.Background(), q)
	if len(warns) != 0 {
		t.Fatalf("a valid response gave %d warnings: %+v", len(warns), warns)
	}
	if len(page.Items) != 2 {
		t.Fatalf("%d items came out of a response with 2, want 2", len(page.Items))
	}

	first := page.Items[0]
	if first.Number != 7 || first.Title != "one" {
		t.Errorf("the first item is %+v", first)
	}
	if first.Forge != "github" || first.Host != "github.com" {
		t.Errorf("the item identity is %s/%s", first.Forge, first.Host)
	}
	if first.Ref.Project != "acme/widget" || first.Ref.Owner != "acme" ||
		first.Ref.Name != "widget" {
		t.Errorf("the item repo is %q (owner %q, name %q): without it nothing can be cloned",
			first.Ref.Project, first.Ref.Owner, first.Ref.Name)
	}
	if first.SourceBranch != "feat/x" || first.TargetBranch != "main" {
		t.Errorf("the item branches are %q -> %q", first.SourceBranch, first.TargetBranch)
	}
	if id := first.ID(); id.Forge != "github" || id.Host != "github.com" ||
		id.Project != "acme/widget" {
		t.Errorf("the item ID is %+v", id)
	}

	if !page.More {
		t.Error("More = false with hasNextPage=true: the inbox gets truncated without warning and " +
			"the user sees five PRs out of forty with no signal at all")
	}
	if page.Next != "CUR2" {
		t.Errorf("Next = %q, want CUR2: without the cursor pagination cannot continue", page.Next)
	}
	// Both together: More with no cursor, or a cursor with no More, are states that do not exist.
	if page.More && page.Next == "" {
		t.Error("More = true with an empty Next: the next page cannot be requested")
	}
}

// It is what lets the same PR appear in two columns with the right review kind in each.
func TestListStampsEachItemWithTheSectionBeingQueried(t *testing.T) {
	a := New("github.com", ghReturning(t, ghPrinting(searchWithPRs)))

	for _, c := range []struct {
		name string
		q    forge.Query
	}{
		{"requested review", forge.Query{
			Section: model.SectionReview, ReviewKind: model.ReviewRequested}},
		{"assigned review", forge.Query{
			Section: model.SectionReview, ReviewKind: model.ReviewAssigned}},
		{"authored", forge.Query{Section: model.SectionAuthored}},
	} {
		page, warns := a.List(context.Background(), c.q)
		if len(warns) != 0 {
			t.Errorf("%s: %d warnings", c.name, len(warns))
			continue
		}
		for _, it := range page.Items {
			if it.Section != c.q.Section {
				t.Errorf("%s: item %d came out with section %q, want %q",
					c.name, it.Number, it.Section, c.q.Section)
			}
		}
	}
}

// A different case from broken JSON: GraphQL answers 200 with an errors array.
func TestListWithGraphQLErrorsGivesAWarningAndNoItems(t *testing.T) {
	const withErrors = `{"errors":[{"message":"Field 'reviewDecision' doesn't exist on ` +
		`type 'PullRequest'","type":"INTERNAL"}]}`

	a := New("github.com", ghReturning(t, ghPrinting(withErrors)))
	page, warns := a.List(context.Background(), forge.Query{
		Section: model.SectionReview, ReviewKind: model.ReviewRequested,
	})

	if len(page.Items) != 0 {
		t.Errorf("%d items came out of a response that only carries errors", len(page.Items))
	}
	if len(warns) != 1 {
		t.Fatalf("%d warnings, want 1: %+v", len(warns), warns)
	}
	if !contains(warns[0].Msg, "reviewDecision") {
		t.Errorf("the warning %q does not carry the server's reason", warns[0].Msg)
	}
}

// A deliberate limit: when GraphQL fails, one's own PRs are asked over REST and the rest are not.
func TestTheRESTFallbackIsUsedOnlyOnTheFirstPageOfAuthored(t *testing.T) {
	down := New("github.com", ghReturning(t, "echo 'gh: could not resolve host' >&2\nexit 1"))

	first, warns := down.List(context.Background(), forge.Query{Section: model.SectionAuthored})
	if len(first.Items) != 0 {
		t.Errorf("with `gh` down, %d items came out of the first page", len(first.Items))
	}
	if len(warns) == 0 {
		t.Error("with `gh` down there is no warning: the inbox would look empty without an explanation")
	}
	second, warns2 := down.List(context.Background(),
		forge.Query{Section: model.SectionAuthored, Cursor: "CUR2"})
	if len(second.Items) != 0 {
		t.Errorf("with `gh` down the second page brought %d items", len(second.Items))
	}
	if len(warns2) == 0 {
		t.Error("the second page without a cursor gave no warning")
	}
}

// A section with no search does not query the forge on purpose.
func TestAnUnsupportedSectionDoesNotEvenQueryTheForge(t *testing.T) {
	a := New("github.com", ghReturning(t, "echo 'should not have been called' >&2\nexit 1"))

	page, warns := a.List(context.Background(), forge.Query{Section: "an-invented-section"})
	if len(page.Items) != 0 {
		t.Errorf("an unsupported section brought %d items", len(page.Items))
	}
	if len(warns) != 1 {
		t.Fatalf("%d warnings, want 1: %+v", len(warns), warns)
	}
	if warns[0].Kind != "unsupported" {
		t.Errorf("kind %q, want unsupported: it is neither a network nor a parsing failure, it is "+
			"that this combination does not exist on this forge", warns[0].Kind)
	}
	if !contains(warns[0].Msg, "an-invented-section") {
		t.Errorf("the warning %q does not name the section that is not supported", warns[0].Msg)
	}
	if warns[0].Section != "an-invented-section" {
		t.Errorf("the warning does not carry the section (carries %q)", warns[0].Section)
	}
}

func contains(haystack, needle string) bool {
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return true
		}
	}
	return len(needle) == 0
}
