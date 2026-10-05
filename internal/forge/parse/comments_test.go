package parse

import (
	"strings"
	"testing"
)

func TestParseGHComments(t *testing.T) {
	raw := `{"data":{"repository":{"pullRequest":{"comments":{
		"totalCount":23,
		"nodes":[
			{"author":{"login":"alice"},"body":"please add a test","createdAt":"2026-09-20T10:00:00Z"},
			{"author":{"login":"bob"},"body":"nit: typo","createdAt":"2026-09-21T11:30:00Z"}
		]}}}}}`

	comments, total, err := ParseGHComments(raw)
	if err != nil {
		t.Fatalf("ParseGHComments: %v", err)
	}
	if total != 23 {
		t.Errorf("total = %d, want 23", total)
	}
	if len(comments) != 2 {
		t.Fatalf("len = %d, want 2", len(comments))
	}
	if comments[0].Author != "alice" || comments[1].Author != "bob" {
		t.Errorf("authors = %q/%q, want alice/bob", comments[0].Author, comments[1].Author)
	}
	if comments[0].Body != "please add a test" {
		t.Errorf("body = %q", comments[0].Body)
	}
	if got := comments[1].CreatedAt.Format("2006-01-02"); got != "2026-09-21" {
		t.Errorf("date = %q, want 2026-09-21", got)
	}
}

func TestParseGHCommentsDeletedAuthor(t *testing.T) {
	raw := `{"data":{"repository":{"pullRequest":{"comments":{
		"totalCount":1,
		"nodes":[{"author":null,"body":"gone","createdAt":"2026-09-20T10:00:00Z"}]
	}}}}}`

	comments, _, err := ParseGHComments(raw)
	if err != nil {
		t.Fatalf("ParseGHComments: %v", err)
	}
	if comments[0].Author != "unknown" {
		t.Errorf("author = %q, want %q", comments[0].Author, unknownAuthor)
	}
}

func TestParseGHCommentsErrors(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		want string
	}{
		{"invalid json", `{"data":`, "invalid JSON"},
		{"forge error", `{"errors":[{"message":"Field 'comments' doesn't exist"}]}`, "doesn't exist"},
		{"no pull request", `{"data":{"repository":{}}}`, "without comments"},
		{"no comments", `{"data":{"repository":{"pullRequest":{}}}}`, "without comments"},
		{"empty response", `{}`, "without comments"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			comments, total, err := ParseGHComments(tc.raw)
			if err == nil {
				t.Fatalf("it should fail, it returned %d comments", len(comments))
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error = %q, want it to contain %q", err, tc.want)
			}
			if total != 0 {
				t.Errorf("a failure must not invent a total: %d", total)
			}
		})
	}
}

func TestParseGLComments(t *testing.T) {
	raw := `{"data":{"project":{"mergeRequest":{"notes":{"nodes":[
		{"author":{"username":"alice"},"body":"ok for me","createdAt":"2026-09-20T10:00:00Z","system":false},
		{"author":{"username":null},"body":"assigned to @bob","createdAt":"2026-09-20T10:01:00Z","system":true}
	]}}}}}`

	comments, total, err := ParseGLComments(raw)
	if err != nil {
		t.Fatalf("ParseGLComments: %v", err)
	}
	if len(comments) != 1 || comments[0].Author != "alice" {
		t.Fatalf("notes = %+v, want only alice's", comments)
	}
	if total != 1 {
		t.Errorf("total = %d, want 1 (the ones read, GitLab exposes no count)", total)
	}
}

func TestParseGLCommentsDropsSystemNotes(t *testing.T) {
	raw := `{"data":{"project":{"mergeRequest":{"notes":{"nodes":[
		{"author":{"username":null},"body":"mentioned in commit abc","createdAt":"2026-09-20T10:00:00Z","system":true},
		{"author":{"username":"alice"},"body":"first","createdAt":"2026-09-20T10:01:00Z","system":false},
		{"author":{"username":null},"body":"added 56 commits","createdAt":"2026-09-20T10:02:00Z","system":true},
		{"author":{"username":"bob"},"body":"second","createdAt":"2026-09-20T10:03:00Z","system":false},
		{"author":{"username":null},"body":"changed the description","createdAt":"2026-09-20T10:04:00Z","system":true}
	]}}}}}`

	comments, _, err := ParseGLComments(raw)
	if err != nil {
		t.Fatalf("ParseGLComments: %v", err)
	}
	if len(comments) != 2 {
		t.Fatalf("len = %d, want 2 (the two written by people)", len(comments))
	}
	if comments[0].Body != "first" || comments[1].Body != "second" {
		t.Errorf("system notes slipped in: %+v", comments)
	}
}

func TestParseGLCommentsKeepsEmptySystemFlag(t *testing.T) {
	raw := `{"data":{"project":{"mergeRequest":{"notes":{"nodes":[
		{"author":{"username":"alice"},"body":"without the system field","createdAt":"2026-09-20T10:00:00Z"}
	]}}}}}`

	comments, _, err := ParseGLComments(raw)
	if err != nil {
		t.Fatalf("ParseGLComments: %v", err)
	}
	if len(comments) != 1 {
		t.Fatalf("len = %d, want 1: an absent `system` is false, not system", len(comments))
	}
}

func TestParseGLCommentsErrors(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		want string
	}{
		{"invalid json", `nope`, "invalid JSON"},
		{"forge error", `{"errors":[{"message":"Field 'notes' doesn't exist on type"}]}`, "doesn't exist"},
		{"no merge request", `{"data":{"project":{}}}`, "without notes"},
		{"no notes", `{"data":{"project":{"mergeRequest":{}}}}`, "without notes"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, _, err := ParseGLComments(tc.raw); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error = %v, want it to contain %q", err, tc.want)
			}
		})
	}
}

// GitHub's bots open with an HTML comment.
func TestCommentLines(t *testing.T) {
	cases := []struct {
		name string
		body string
		want []string
	}{
		{"one line", "please add a test", []string{"please add a test"}},
		{
			"with an HTML comment in front",
			"<!-- ssf: origin=o/r#553 -->\n\nssf attaching agent",
			[]string{"ssf attaching agent"},
		},
		{"with blanks", "\n\n   \nfirst\n\nsecond\n", []string{"first", "second"}},
		{
			"paragraphs separated",
			"line one\n\nline two",
			[]string{"line one", "line two"},
		},
		{
			"list keeps the dashes",
			"- fix the timeout\n- fix the backoff",
			[]string{"- fix the timeout", "- fix the backoff"},
		},
		{
			"the heading keeps its markup",
			"### Applied fixes\n\n**HIGH** double prefix",
			[]string{"### Applied fixes", "**HIGH** double prefix"},
		},
		{"spaces and tabs inside the line", "a\t\tb   c", []string{"a b c"}},
		{"only boilerplate", "<!-- x -->\n\n<!-- y -->", nil},
		{"empty", "", nil},
		{"only spaces", "   \n\t", nil},
		{"unicode", "the timeout is 30× higher 🚀", []string{"the timeout is 30× higher 🚀"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := CommentLines(tc.body)
			if len(got) != len(tc.want) {
				t.Fatalf("CommentLines(%q) = %q, want %q", tc.body, got, tc.want)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Errorf("line %d = %q, want %q", i, got[i], tc.want[i])
				}
			}
		})
	}
}
