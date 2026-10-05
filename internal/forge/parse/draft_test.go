package parse

import (
	"testing"

	"prdash/internal/state"
)

// The test that was missing while the merge gate was dead.
func TestDraftFlagFromEveryPath(t *testing.T) {
	for _, tc := range []struct {
		name string
		got  func(t *testing.T) (draft bool, state string, block state.Block)
	}{
		{
			name: "github graphql",
			got: func(t *testing.T) (bool, string, state.Block) {
				items, _, err := ParseGHGraphQLSearch(ghSearchFixture)
				if err != nil {
					t.Fatalf("parse: %v", err)
				}
				if len(items) != 2 {
					t.Fatalf("items = %d, want 2", len(items))
				}
				return items[1].IsDraft, items[1].State, state.MergeBlock(items[1])
			},
		},
		{
			name: "github rest authored",
			got: func(t *testing.T) (bool, string, state.Block) {
				items, err := ParseGHAuthored(ghAuthoredDraftFixture)
				if err != nil {
					t.Fatalf("parse: %v", err)
				}
				return items[0].IsDraft, items[0].State, state.MergeBlock(items[0])
			},
		},
		{
			name: "gitlab graphql",
			got: func(t *testing.T) (bool, string, state.Block) {
				items, _, err := ParseGLGraphQL(glDraftFixture)
				if err != nil {
					t.Fatalf("parse: %v", err)
				}
				if len(items) != 1 {
					t.Fatalf("items = %d, want 1", len(items))
				}
				return items[0].IsDraft, items[0].State, state.MergeBlock(items[0])
			},
		},
		{
			name: "gitlab rest mr list",
			got: func(t *testing.T) (bool, string, state.Block) {
				items, err := ParseGLMRList(glBasicDraftFixture)
				if err != nil {
					t.Fatalf("parse: %v", err)
				}
				if len(items) != 1 {
					t.Fatalf("items = %d, want 1", len(items))
				}
				return items[0].IsDraft, items[0].State, state.MergeBlock(items[0])
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			draft, rawState, block := tc.got(t)
			if !draft {
				t.Error("IsDraft = false, want true: the forge marked it as a draft")
			}
			// The forge's enum stays the forge's, and normalisation is what decides the state.
			switch rawState {
			case "OPEN", "open", "opened":
			default:
				t.Errorf("State = %q, want the forge's raw enum (OPEN/open/opened)", rawState)
			}
			if !block.Hard || block.Reason == "" {
				t.Errorf("MergeBlock = %+v, want a hard block that mentions the draft", block)
			}
		})
	}
}

const ghAuthoredDraftFixture = `{
  "total_count": 1,
  "items": [
    {
      "number": 9,
      "title": "WIP",
      "html_url": "https://github.com/acme/lib/pull/9",
      "state": "open",
      "draft": true,
      "updated_at": "2026-09-20T08:00:00Z",
      "user": {"login": "me"},
      "repository_url": "https://api.github.com/repos/acme/lib",
      "pull_request": {"head": {"ref": "wip"}, "base": {"ref": "main"}}
    }
  ]
}`

const glDraftFixture = `{
  "data": {
    "currentUser": {
      "authoredMergeRequests": {"pageInfo": {"hasNextPage": false, "endCursor": null},
        "nodes": [
        {"iid":"11","title":"WIP","webUrl":"u11","state":"opened","draft":true,"sourceBranch":"wip","targetBranch":"main","approved":false,"updatedAt":"2026-09-23T09:00:00Z","author":{"username":"me"},"project":{"fullPath":"grp/proj","name":"proj","group":{"fullPath":"grp"}}}
      ]}
    }
  }
}`

const glBasicDraftFixture = `[
  {"iid":12,"title":"WIP","web_url":"u12","state":"opened","draft":true,
   "source_branch":"wip","target_branch":"main","updated_at":"2026-09-23T09:00:00Z",
   "author":{"username":"me"},"references":{"full":"grp/proj!12","short":"!12"}}
]`
