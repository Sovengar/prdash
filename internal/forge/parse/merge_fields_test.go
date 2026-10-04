package parse

import (
	"testing"

	"prdash/internal/forge/model"
)

// Without these two in the parsing, the pin can never be satisfied and the mode filter has no
// data.
func TestGHNodeCarriesThePinAndTheRules(t *testing.T) {
	raw := `{"data":{"search":{"pageInfo":{"hasNextPage":false,"endCursor":null},
		"nodes":[{"number":7,"title":"T","url":"https://github.com/acme/widget/7",
		"state":"OPEN","isDraft":false,"reviewDecision":"APPROVED",
		"headRefOid":"deadbeefcafe","headRefName":"feat/x","baseRefName":"main",
		"additions":1,"deletions":0,"changedFiles":1,
		"author":{"login":"me"},
		"repository":{"nameWithOwner":"acme/widget","name":"widget",
			"owner":{"login":"acme"},
			"mergeCommitAllowed":true,"rebaseMergeAllowed":true,"squashMergeAllowed":false}}]}}}`

	items, _, err := ParseGHGraphQLSearch(raw)
	if err != nil {
		t.Fatalf("ParseGHGraphQLSearch: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("items = %d, want 1", len(items))
	}
	if items[0].HeadSHA != "deadbeefcafe" {
		t.Errorf("HeadSHA = %q, want deadbeefcafe", items[0].HeadSHA)
	}
	if !items[0].Merge.Known {
		t.Error("Merge.Known = false, want true: los tres flags venían en la respuesta")
	}
	if !items[0].Merge.Rebase || !items[0].Merge.MergeCommit {
		t.Errorf("Merge = %+v, want rebase y merge commit permitidos", items[0].Merge)
	}
	// The case that matters: squash disabled in the repo has to read as disabled.
	if items[0].Merge.Squash {
		t.Errorf("Merge.Squash = true, want false: el repositorio lo tiene desactivado")
	}
}

// The REST fallback does not carry the flags; marking Known with all three false would be a
// lie.
func TestGHNodeWithoutRulesStaysUnknown(t *testing.T) {
	raw := `{"data":{"search":{"pageInfo":{"hasNextPage":false,"endCursor":null},
		"nodes":[{"number":7,"title":"T","url":"u","state":"OPEN","isDraft":false,
		"reviewDecision":"","headRefName":"f","baseRefName":"main",
		"additions":1,"deletions":0,"changedFiles":1,
		"author":{"login":"me"},
		"repository":{"nameWithOwner":"acme/widget","name":"widget","owner":{"login":"acme"}}}]}}}`

	items, _, err := ParseGHGraphQLSearch(raw)
	if err != nil {
		t.Fatalf("ParseGHGraphQLSearch: %v", err)
	}
	if items[0].Merge.Known {
		t.Errorf("Merge = %+v, want Known=false", items[0].Merge)
	}
	if got := len(allowed(items[0].Merge)); got != 3 {
		t.Errorf("con reglas desconocidas se ofrecen %d modos, want 3", got)
	}
}

func TestGHNodeWithPartialRulesStaysUnknown(t *testing.T) {
	raw := `{"data":{"search":{"pageInfo":{"hasNextPage":false,"endCursor":null},
		"nodes":[{"number":7,"title":"T","url":"u","state":"OPEN","isDraft":false,
		"reviewDecision":"","headRefName":"f","baseRefName":"main",
		"additions":1,"deletions":0,"changedFiles":1,
		"author":{"login":"me"},
		"repository":{"nameWithOwner":"acme/widget","name":"widget",
			"owner":{"login":"acme"},"rebaseMergeAllowed":true}}]}}}`

	items, _, err := ParseGHGraphQLSearch(raw)
	if err != nil {
		t.Fatalf("ParseGHGraphQLSearch: %v", err)
	}
	if items[0].Merge.Known {
		t.Errorf("Merge = %+v, want Known=false con reglas parciales", items[0].Merge)
	}
}

// diffHeadSha is a pointer because the schema declares it nullable.
func TestGLNodeCarriesThePin(t *testing.T) {
	raw := `{"data":{"currentUser":{"authoredMergeRequests":{
		"pageInfo":{"hasNextPage":false,"endCursor":null},
		"nodes":[{"iid":7,"title":"T","webUrl":"u","state":"opened",
		"sourceBranch":"f","targetBranch":"main","approved":true,
		"diffHeadSha":"beef1234","squash":false,
		"diffStats":[{"additions":1,"deletions":0}],
		"author":{"username":"me"},
		"project":{"fullPath":"grp/proj","name":"proj","group":{"fullPath":"grp"}}}]}}}}`

	items, _, err := ParseGLGraphQL(raw)
	if err != nil {
		t.Fatalf("ParseGLGraphQL: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("items = %d, want 1", len(items))
	}
	if items[0].HeadSHA != "beef1234" {
		t.Errorf("HeadSHA = %q, want beef1234", items[0].HeadSHA)
	}
	// GitLab does not expose the strategies over GraphQL, so they arrive unknown: no filtering
	// rather than filtering on nothing.
	if items[0].Merge.Known {
		t.Errorf("Merge = %+v, want Known=false en GitLab", items[0].Merge)
	}
}

func TestGLNodeWithNullPinStaysUnpinned(t *testing.T) {
	raw := `{"data":{"currentUser":{"authoredMergeRequests":{
		"pageInfo":{"hasNextPage":false,"endCursor":null},
		"nodes":[{"iid":7,"title":"T","webUrl":"u","state":"opened",
		"sourceBranch":"f","targetBranch":"main","approved":true,
		"diffHeadSha":null,"squash":false,
		"diffStats":[{"additions":1,"deletions":0}],
		"author":{"username":"me"},
		"project":{"fullPath":"grp/proj","name":"proj","group":{"fullPath":"grp"}}}]}}}}`

	items, _, err := ParseGLGraphQL(raw)
	if err != nil {
		t.Fatalf("ParseGLGraphQL: %v", err)
	}
	if items[0].HeadSHA != "" {
		t.Errorf("HeadSHA = %q, want vacío: un diff null no es un SHA", items[0].HeadSHA)
	}
}

func allowed(r model.MergeRules) []bool {
	if !r.Known {
		return []bool{true, true, true}
	}
	return []bool{r.MergeCommit, r.Rebase, r.Squash}
}
