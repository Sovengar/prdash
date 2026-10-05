package gitlab

import (
	"context"
	"strconv"
	"strings"
	"testing"

	"prdash/internal/forge"
	"prdash/internal/forge/model"
)

// If the API returns exactly a page's worth, there is more.
func TestWithExactlyAFullPageThereIsStillMore(t *testing.T) {
	dir := t.TempDir()

	todos := func(n int, action string) string {
		rows := make([]string, 0, n)
		for i := range n {
			rows = append(rows, `{"id":`+strconv.Itoa(i+1)+`,"action_name":"`+action+
				`","target_type":"MergeRequest","updated_at":"2026-03-17T10:00:00Z",`+
				`"target":{"iid":`+strconv.Itoa(i+1)+`,"title":"mr `+strconv.Itoa(i+1)+
				`","web_url":"https://gitlab.com/g/p!`+strconv.Itoa(i+1)+
				`","state":"opened","source_branch":"feat/x","target_branch":"main",`+
				`"author":{"username":"alice"},`+
				`"references":{"full":"g/p!`+strconv.Itoa(i+1)+`","name":"p","namespace":"g"}}}`)
		}
		return "[" + strings.Join(rows, ",") + "]"
	}

	cases := []struct {
		n        int
		action   string
		wantMore bool
		wantNext string
		note     string
	}{
		{0, "mentioned", false, "", "empty page"},
		{1, "mentioned", false, "", "one row of a page of fifty"},
		{pageSize - 1, "mentioned", false, "",
			"one row LESS than the page: the next one comes out empty and is not requested"},
		{pageSize, "mentioned", true, "2",
			"EXACTLY a full page: there is at least one more to find out about, and without a " +
				"button the user sees 50 of 50 with no clue that any are missing"},
		{pageSize + 1, "mentioned", true, "2", "full with leftovers"},
		{pageSize * 3, "mentioned", true, "2", "three pages at once"},
		{pageSize, "labeled", true, "2",
			"a full page of rows that are NOT mentions: pagination looks at how many " +
				"rows came, not at how many are MRs, because the filter is client-side"},
	}

	for _, c := range cases {
		script := writeScript(t, dir, "glab", "#!/bin/sh\necho '"+todos(c.n, c.action)+"'\n")
		a := New("gitlab.com", script)

		page, warns := a.todosList(context.Background(),
			forge.Query{Section: model.SectionMentions})
		if len(warns) != 0 {
			t.Errorf("%s: %d warnings: %+v", c.note, len(warns), warns)
			continue
		}
		if page.More != c.wantMore {
			t.Errorf("%s: More=%v, want %v", c.note, page.More, c.wantMore)
		}
		if page.Next != c.wantNext {
			t.Errorf("%s: Next=%q, want %q", c.note, page.Next, c.wantNext)
		}
		if c.wantMore && page.Next != strconv.Itoa(2) {
			t.Errorf("%s: the next cursor is %q and it has to be page 2",
				c.note, page.Next)
		}
	}

	// With the cursor on page 3 the next is 4: the cursor is the page NUMBER.
	script := writeScript(t, dir, "glab", "#!/bin/sh\necho '"+todos(pageSize, "mentioned")+"'\n")
	a := New("gitlab.com", script)
	page, warns := a.todosList(context.Background(),
		forge.Query{Section: model.SectionMentions, Cursor: "3"})
	if len(warns) != 0 {
		t.Fatalf("with the cursor on page 3: %d warnings: %+v", len(warns), warns)
	}
	if page.Next != "4" {
		t.Errorf("with the cursor on page 3 the next is %q, want 4", page.Next)
	}
}
