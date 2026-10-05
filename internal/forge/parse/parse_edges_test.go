package parse

import (
	"fmt"
	"strings"
	"testing"

	"prdash/internal/forge/model"
)

// The parsing branches that only show up with data the forge NEVER sends, which is why they are the
//ones most likely to rot.

// The hierarchy of the parse.
func TestACheckWithAnUnknownBucketFallsToTheRawStateAndDoesNotInventACount(t *testing.T) {
	for _, c := range []struct {
		name        string
		check       string
		wantFail    int
		wantPending int
		wantTotal   int
	}{
		{"bucket pass", `{"name":"ci","bucket":"pass","state":""}`, 0, 0, 1},
		{"bucket skip", `{"name":"ci","bucket":"skipping","state":""}`, 0, 0, 1},
		{"bucket fail", `{"name":"ci","bucket":"fail","state":""}`, 1, 0, 1},
		{"bucket cancel counts as a failure", `{"name":"ci","bucket":"cancel","state":""}`, 1, 0, 1},
		{"bucket pending", `{"name":"ci","bucket":"pending","state":""}`, 0, 1, 1},

		// The `default`: with no known bucket the raw state is read, and the counting branch follows it.
		{"state FAILURE without bucket", `{"name":"ci","state":"FAILURE"}`, 1, 0, 1},
		{"state error without bucket", `{"name":"ci","state":"ERROR"}`, 1, 0, 1},
		{"state FAILURE lowercased", `{"name":"ci","state":"failure"}`, 1, 0, 1},
		{"state in_progress without bucket", `{"name":"ci","state":"IN_PROGRESS"}`, 0, 1, 1},
		{"state queued without bucket", `{"name":"ci","state":"QUEUED"}`, 0, 1, 1},

		// And the case the default must leave at zero: neither failure nor pending.
		{"invented state", `{"name":"ci","state":"timed_out_wtf"}`, 0, 0, 1},
		{"invented bucket and empty state", `{"name":"ci","bucket":"raro","state":""}`, 0, 0, 1},
		{"strange bucket and strange state", `{"name":"ci","bucket":"raro","state":"raro"}`, 0, 0, 1},

		// The asymmetry to watch: the bucket wins over the state even when they contradict each other.
		{"bucket pass with state FAILURE", `{"name":"ci","bucket":"pass","state":"FAILURE"}`, 0, 0, 1},
	} {
		raw := "[" + c.check + "]"
		st, err := ParseGHChecks(raw)
		if err != nil {
			t.Errorf("%s: %v", c.name, err)
			continue
		}
		if st.Total != c.wantTotal {
			t.Errorf("%s: Total = %d, want %d", c.name, st.Total, c.wantTotal)
		}
		if st.Failing != c.wantFail {
			t.Errorf("%s: Failing = %d, want %d", c.name, st.Failing, c.wantFail)
		}
		if st.Pending != c.wantPending {
			t.Errorf("%s: Pending = %d, want %d", c.name, st.Pending, c.wantPending)
		}
	}

	mix := `[{"name":"a","bucket":"pass"},{"name":"b","bucket":"fail"},` +
		`{"name":"c","bucket":"pass"}]`
	st, err := ParseGHChecks(mix)
	if err != nil {
		t.Fatal(err)
	}
	if st.State != model.ChecksFailing {
		t.Errorf("with a failing check the global state is %v, want failing", st.State)
	}
	if st.Total != 3 || st.Failing != 1 {
		t.Errorf("the summary is %+v: 3 checks and 1 failing", st)
	}
}

// The commonest "no checks" case.
func TestGHChecksWithEmptyOutputInventsNoChecksAndDoesNotFail(t *testing.T) {
	for _, raw := range []string{"[]", "null"} {
		st, err := ParseGHChecks(raw)
		if err != nil {
			t.Errorf("%q gave an error: %v", raw, err)
			continue
		}
		if st.Failing != 0 || st.Pending != 0 || st.Total != 0 {
			t.Errorf("%q invented checks: %+v", raw, st)
		}
	}

	st, err := ParseGHChecks(`[{"name":"a","bucket":"pass"},{"name":"b","bucket":"pass"}]`)
	if err != nil {
		t.Fatal(err)
	}
	if st.State != model.ChecksPassing {
		t.Errorf("only green checks gave %v, want passing", st.State)
	}
}

// A different case from an empty page.
func TestGLGraphQLErrorsFromTheServerAreNotConfusedWithAnEmptyPage(t *testing.T) {
	// The server's message comes through verbatim, and it is the only thing that tells "my query
	// already ran" from a real failure.
	raw := `{"errors":[{"message":"Field 'mergeRequest' doesn't exist on type 'Project'",` +
		`"type":"undefinedField","path":["project"]}]}`
	items, page, err := ParseGLGraphQL(raw)
	if err == nil {
		t.Fatal("some GraphQL errors gave nil: the adapter would treat them as an empty inbox")
	}
	if len(items) != 0 {
		t.Errorf("%d items came out of a response that only carries errors", len(items))
	}
	if page.More || page.Next != "" {
		t.Errorf("the pagination came out with data: %+v", page)
	}
	if !strings.Contains(err.Error(), "mergeRequest") {
		t.Errorf("the error %q does not bring the server's message", err)
	}

	// The control: broken JSON IS a parse error, and both are errors but of different classes.
	if _, _, err := ParseGLGraphQL("I am not json"); err == nil ||
		!strings.Contains(err.Error(), "invalid JSON") {
		t.Errorf("broken JSON gave %v, want an invalid JSON error", err)
	}
}

// The response carries neither data nor errors.
func TestGLGraphQLWithAMissingConnectionDoesNotFailAndDoesNotLoseTheRest(t *testing.T) {
	const mr = `{"iid":12,"title":"MR","webUrl":"u","state":"opened",` +
		`"sourceBranch":"feat/a","targetBranch":"main",` +
		`"updatedAt":"2026-09-21T07:00:00Z","author":{"username":"me"}}`
	const conn = `{"pageInfo":{"hasNextPage":false,"endCursor":null},"nodes":[%s]}`

	raw := fmt.Sprintf(
		`{"data":{"currentUser":{"authoredMergeRequests":null,`+
			`"reviewRequestedMergeRequests":%s,`+
			`"assignedMergeRequests":null}}}`,
		fmt.Sprintf(conn, mr))
	items, _, err := ParseGLGraphQL(raw)
	if err != nil {
		t.Fatalf("a missing connection gave an error: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("%d items came out with a connection at null, want 1", len(items))
	}
	if items[0].Number != 12 {
		t.Errorf("the item is %+v", items[0])
	}

	empty := `{"data":{"currentUser":{"authoredMergeRequests":null,` +
		`"reviewRequestedMergeRequests":null,"assignedMergeRequests":null}}}`
	if items, _, err = ParseGLGraphQL(empty); err != nil || len(items) != 0 {
		t.Errorf("three connections at null gave %d items and %v", len(items), err)
	}

	noData := `{"data":{}}`
	if items, _, err = ParseGLGraphQL(noData); err != nil || len(items) != 0 {
		t.Errorf("with no `data` it gave %d items and %v", len(items), err)
	}

	// Pagination is taken from the FIRST connection there is, which stops a second one from
	// overwriting it.
	two := fmt.Sprintf(
		`{"data":{"currentUser":{"authoredMergeRequests":null,`+
			`"reviewRequestedMergeRequests":%s,`+
			`"assignedMergeRequests":%s}}}`,
		fmt.Sprintf(`{"pageInfo":{"hasNextPage":true,"endCursor":"CUR2"},"nodes":[%s]}`, mr),
		`{"pageInfo":{"hasNextPage":false,"endCursor":null},"nodes":[]}`)
	_, page, err := ParseGLGraphQL(two)
	if err != nil {
		t.Fatal(err)
	}
	if !page.More || page.Next != "CUR2" {
		t.Errorf("with a null connection before the one carrying items, the pagination is %+v: "+
			"the cursor was lost and the inbox gets truncated", page)
	}
}

// The Todos API brings EVERY event a user has, so the action filter is what makes it a
// mentions list.
func TestGLTodosIgnoresWhatIsNotAMention(t *testing.T) {
	for _, c := range []struct {
		name   string
		action string
		kind   string
		iid    int
		want   int
	}{
		{"mention of an MR", "mentioned", "MergeRequest", 12, 1},
		{"direct mention of an MR", "directly_addressed", "MergeRequest", 13, 1},
		{"own push", "pushed", "MergeRequest", 14, 0},
		{"label change", "update", "MergeRequest", 15, 0},
		{"assigned to an MR", "assigned", "MergeRequest", 19, 0},
		{"mention of an ISSUE", "mentioned", "Issue", 16, 0},
		{"direct mention of an ISSUE", "directly_addressed", "Issue", 17, 0},
		{"empty action", "", "MergeRequest", 18, 0},
		{"empty kind", "mentioned", "", 20, 0},
		{"iid at zero", "mentioned", "MergeRequest", 0, 0},
	} {
		// The iid is inside the target: the parser requires it to build the item.
		raw := fmt.Sprintf(
			`[{"action_name":%q,"target_type":%q,`+
				`"target":{"iid":%d,"references":{"full":"grp/sub/proy!%d"}}}]`,
			c.action, c.kind, c.iid, c.iid)

		items, total, err := ParseGLTodos(raw)
		if err != nil {
			t.Errorf("%s: %v", c.name, err)
			continue
		}
		if len(items) != c.want {
			t.Errorf("%s: %d items, want %d", c.name, len(items), c.want)
		}
		// The total the forge reports is the RESPONSE's, not the filtered mentions': a count that
		// contradicts the column is not a bug.
		if total != 1 {
			t.Errorf("%s: total = %d, want 1 (the response's events)", c.name, total)
		}
	}

	items, total, err := ParseGLTodos("[]")
	if err != nil || len(items) != 0 || total != 0 {
		t.Errorf("an empty list gave %d items, total %d and %v", len(items), total, err)
	}
}

// A real case, not invented data.
func TestAGitLabNodeWithoutIidIsDiscardedAndTheRestSurvive(t *testing.T) {
	noIID := `{"title":"deleted MR","webUrl":"u","state":"opened",` +
		`"sourceBranch":"feat/a","targetBranch":"main","author":{"username":"someone"}}`
	withZero := `{"iid":0,"title":"another deleted","webUrl":"u","state":"opened",` +
		`"sourceBranch":"feat/b","targetBranch":"main","author":{"username":"other"}}`
	good := `{"iid":12,"title":"MR","webUrl":"u","state":"opened",` +
		`"sourceBranch":"feat/c","targetBranch":"main",` +
		`"updatedAt":"2026-09-21T07:00:00Z","author":{"username":"me"}}`

	for _, c := range []struct {
		name  string
		nodes string
		want  int
	}{
		{"all three, one valid", fmt.Sprintf(`[%s,%s,%s]`, noIID, withZero, good), 1},
		{"only the valid one", "[" + good + "]", 1},
		{"all invalid", fmt.Sprintf(`[%s,%s]`, noIID, withZero), 0},
		{"empty list", "[]", 0},
	} {
		raw := `{"data":{"currentUser":{"reviewRequestedMergeRequests":` +
			`{"pageInfo":{"hasNextPage":false,"endCursor":null},"nodes":` +
			c.nodes + `}}}}`
		items, _, err := ParseGLGraphQL(raw)
		if err != nil {
			t.Errorf("%s: %v", c.name, err)
			continue
		}
		if len(items) != c.want {
			t.Errorf("%s: %d items, want %d", c.name, len(items), c.want)
		}
		// None of the survivors carries number zero, which is what would collide with the others.
		for _, it := range items {
			if it.Number == 0 {
				t.Errorf("%s: an item with number 0 came out: %+v", c.name, it)
			}
		}
		// The survivor is the good one, not the first: an invalid node at the front would win otherwise.
		if c.want == 1 && items[0].Number != 12 {
			t.Errorf("%s: the surviving item is %d, want 12", c.name, items[0].Number)
		}
	}
}
