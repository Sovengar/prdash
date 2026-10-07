package parse

import (
	"strings"
	"testing"

	"prdash/internal/forge/model"
	"prdash/internal/state"
)

func TestMergeableFromEveryPath(t *testing.T) {
	for _, tc := range []struct {
		name           string
		wantKnown      bool
		wantConflicted bool
		wantBlocked    bool
		got            func(t *testing.T) (model.Mergeability, state.Block)
	}{
		{
			name:           "github graphql: CONFLICTING",
			wantKnown:      true,
			wantConflicted: true,
			wantBlocked:    true,
			got: func(t *testing.T) (model.Mergeability, state.Block) {
				items := mustSearch(t, ghSearchConflictedFixture, 2)
				return items[1].Mergeable, state.MergeBlock(items[1])
			},
		},
		{
			name:      "github graphql: MERGEABLE gives no warning",
			wantKnown: true,
			got: func(t *testing.T) (model.Mergeability, state.Block) {
				items := mustSearch(t, ghSearchMergeableFixture, 1)
				return items[0].Mergeable, state.MergeBlock(items[0])
			},
		},
		{
			// UNKNOWN is "I do not know yet", not a "yes": with no Known there is no warning, because a false warning is worse.
			name: "github graphql: UNKNOWN gives no warning",
			got: func(t *testing.T) (model.Mergeability, state.Block) {
				items := mustSearch(t, ghSearchUnknownFixture, 1)
				return items[0].Mergeable, state.MergeBlock(items[0])
			},
		},
		{
			name: "github rest: the search does not bring the datum",
			got: func(t *testing.T) (model.Mergeability, state.Block) {
				items, err := ParseGHAuthored(ghAuthoredDraftFixture)
				if err != nil {
					t.Fatalf("parse: %v", err)
				}
				return items[0].Mergeable, state.MergeBlock(items[0])
			},
		},
		{
			name:           "gitlab graphql: CONFLICT",
			wantKnown:      true,
			wantConflicted: true,
			wantBlocked:    true,
			got: func(t *testing.T) (model.Mergeability, state.Block) {
				items, _, err := ParseGLGraphQL(glConflictFixture)
				if err != nil {
					t.Fatalf("parse: %v", err)
				}
				if len(items) != 1 {
					t.Fatalf("items = %d, want 1", len(items))
				}
				return items[0].Mergeable, state.MergeBlock(items[0])
			},
		},
		{
			name:           "gitlab rest: broken_status",
			wantKnown:      true,
			wantConflicted: true,
			wantBlocked:    true,
			got: func(t *testing.T) (model.Mergeability, state.Block) {
				items, err := ParseGLMRList(glBasicConflictFixture)
				if err != nil {
					t.Fatalf("parse: %v", err)
				}
				if len(items) != 1 {
					t.Fatalf("items = %d, want 1", len(items))
				}
				return items[0].Mergeable, state.MergeBlock(items[0])
			},
		},
		{
			// NEED_REBASE is not a conflict: the branch is behind but integrates without touching anything.
			name:      "gitlab graphql: NEED_REBASE is not a conflict",
			wantKnown: true,
			got: func(t *testing.T) (model.Mergeability, state.Block) {
				items, _, err := ParseGLGraphQL(glNeedRebaseFixture)
				if err != nil {
					t.Fatalf("parse: %v", err)
				}
				return items[0].Mergeable, state.MergeBlock(items[0])
			},
		},
		{
			name: "gitlab todos: the target does not bring the datum",
			got: func(t *testing.T) (model.Mergeability, state.Block) {
				items, _, err := ParseGLTodos(glTodosMRFixture)
				if err != nil {
					t.Fatalf("parse: %v", err)
				}
				if len(items) != 1 {
					t.Fatalf("items = %d, want 1", len(items))
				}
				return items[0].Mergeable, state.MergeBlock(items[0])
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m, block := tc.got(t)
			if m.Known != tc.wantKnown {
				t.Errorf("Known = %v, want %v", m.Known, tc.wantKnown)
			}
			if m.Conflicted != tc.wantConflicted {
				t.Errorf("Conflicted = %v, want %v", m.Conflicted, tc.wantConflicted)
			}
			if blocked := block.Reason != "" && strings.Contains(block.Reason, "conflicts"); blocked != tc.wantBlocked {
				t.Errorf("the gate warns about the conflict = %v (%q), want %v", blocked, block.Reason, tc.wantBlocked)
			}
		})
	}
}

func mustSearch(t *testing.T, raw string, n int) []model.Item {
	t.Helper()
	items, _, err := ParseGHGraphQLSearch(raw)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(items) < n {
		t.Fatalf("items = %d, want at least %d", len(items), n)
	}
	return items[:n]
}

const ghSearchConflictedFixture = `{
  "data": {
    "search": {
      "pageInfo": {"hasNextPage": false, "endCursor": null},
      "nodes": [
        {"number": 3, "title": "ok", "url": "u3", "state": "OPEN", "isDraft": false,
         "mergeable": "MERGEABLE", "reviewDecision": "APPROVED", "updatedAt": "2026-09-20T08:00:00Z",
         "headRefName": "feat/ok", "baseRefName": "main", "headRefOid": "aaa111",
         "repository": {"nameWithOwner": "acme/lib", "name": "lib", "owner": {"login": "acme"},
                        "mergeCommitAllowed": true, "rebaseMergeAllowed": true, "squashMergeAllowed": true},
         "author": {"login": "me"}},
        {"number": 4, "title": "choca", "url": "u4", "state": "OPEN", "isDraft": false,
         "mergeable": "CONFLICTING", "reviewDecision": "APPROVED", "updatedAt": "2026-09-20T08:00:00Z",
         "headRefName": "feat/choca", "baseRefName": "main", "headRefOid": "bbb222",
         "repository": {"nameWithOwner": "acme/lib", "name": "lib", "owner": {"login": "acme"},
                        "mergeCommitAllowed": true, "rebaseMergeAllowed": true, "squashMergeAllowed": true},
         "author": {"login": "me"}}
      ]
    }
  }
}`

const ghSearchMergeableFixture = `{
  "data": {
    "search": {
      "pageInfo": {"hasNextPage": false, "endCursor": null},
      "nodes": [
        {"number": 5, "title": "ok", "url": "u5", "state": "OPEN", "isDraft": false,
         "mergeable": "MERGEABLE", "reviewDecision": "", "updatedAt": "2026-09-20T08:00:00Z",
         "headRefName": "feat/ok", "baseRefName": "main", "headRefOid": "ccc333",
         "repository": {"nameWithOwner": "acme/lib", "name": "lib", "owner": {"login": "acme"},
                        "mergeCommitAllowed": true, "rebaseMergeAllowed": true, "squashMergeAllowed": true},
         "author": {"login": "me"}}
      ]
    }
  }
}`

const ghSearchUnknownFixture = `{
  "data": {
    "search": {
      "pageInfo": {"hasNextPage": false, "endCursor": null},
      "nodes": [
        {"number": 6, "title": "calculando", "url": "u6", "state": "OPEN", "isDraft": false,
         "mergeable": "UNKNOWN", "reviewDecision": "", "updatedAt": "2026-09-20T08:00:00Z",
         "headRefName": "feat/new", "baseRefName": "main", "headRefOid": "ddd444",
         "repository": {"nameWithOwner": "acme/lib", "name": "lib", "owner": {"login": "acme"},
                        "mergeCommitAllowed": true, "rebaseMergeAllowed": true, "squashMergeAllowed": true},
         "author": {"login": "me"}}
      ]
    }
  }
}`

const glConflictFixture = `{
  "data": {
    "currentUser": {
      "authoredMergeRequests": {"pageInfo": {"hasNextPage": false, "endCursor": null},
        "nodes": [
        {"iid":"21","title":"choca","webUrl":"u21","state":"opened","draft":false,
         "detailedMergeStatus":"CONFLICT","sourceBranch":"feat/choca","targetBranch":"main",
         "approved":true,"updatedAt":"2026-09-23T09:00:00Z","author":{"username":"me"},
         "project":{"fullPath":"grp/proj","name":"proj","group":{"fullPath":"grp"}}}
      ]}
    }
  }
}`

const glNeedRebaseFixture = `{
  "data": {
    "currentUser": {
      "authoredMergeRequests": {"pageInfo": {"hasNextPage": false, "endCursor": null},
        "nodes": [
        {"iid":"22","title":"detras","webUrl":"u22","state":"opened","draft":false,
         "detailedMergeStatus":"NEED_REBASE","sourceBranch":"feat/detras","targetBranch":"main",
         "approved":true,"updatedAt":"2026-09-23T09:00:00Z","author":{"username":"me"},
         "project":{"fullPath":"grp/proj","name":"proj","group":{"fullPath":"grp"}}}
      ]}
    }
  }
}`

const glBasicConflictFixture = `[
  {"iid":23,"title":"choca","web_url":"u23","state":"opened","draft":false,
   "detailed_merge_status":"broken_status",
   "source_branch":"feat/choca","target_branch":"main","updated_at":"2026-09-23T09:00:00Z",
   "author":{"username":"me"},"references":{"full":"grp/proj!23","short":"!23"}}
]`

const glTodosMRFixture = `[
  {"id":1,"action_name":"mentioned","target_type":"MergeRequest",
   "target_url":"https://gitlab.example.com/grp/proj/-/merge_requests/24",
   "updated_at":"2026-09-23T09:00:00Z",
   "target":{"iid":24,"title":"mencionado","web_url":"u24","state":"opened",
             "source_branch":"feat/x","target_branch":"main","author":{"username":"other"},
             "references":{"full":"grp/proj!24"}}}
]`
