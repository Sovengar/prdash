// GitHub comments and GitLab notes arrive with different field names over the same shape, so they
// normalise into one model type through one conversion function.
package parse

import (
	"encoding/json"
	"strings"

	"prdash/internal/forge/model"
)

// Inventing a name would be worse than saying we do not know who wrote it.
const unknownAuthor = "unknown"

type commentNode struct {
	Author struct {
		Login    string `json:"login"`
		Username string `json:"username"`
	} `json:"author"`
	Body      string `json:"body"`
	CreatedAt string `json:"createdAt"`
	System    bool   `json:"system"`
}

func (n commentNode) author() string {
	if n.Author.Login != "" {
		return n.Author.Login
	}
	if n.Author.Username != "" {
		return n.Author.Username
	}
	return unknownAuthor
}

// The system marker stays in the model and is not filtered here: how many nodes were requested is
// the adapter's business, not the response format's.
func (n commentNode) comment() model.Comment {
	return model.Comment{
		Author:    n.author(),
		Body:      n.Body,
		CreatedAt: parseTime(n.CreatedAt),
	}
}

type ghCommentsResp struct {
	Data struct {
		Repository *struct {
			PullRequest *struct {
				Comments *struct {
					TotalCount int           `json:"totalCount"`
					Nodes      []commentNode `json:"nodes"`
				} `json:"comments"`
			} `json:"pullRequest"`
		} `json:"repository"`
	} `json:"data"`
	Errors []struct {
		Message string `json:"message"`
	} `json:"errors"`
}

func ParseGHComments(raw string) ([]model.Comment, int, error) {
	var resp ghCommentsResp
	if err := json.Unmarshal([]byte(raw), &resp); err != nil {
		return nil, 0, &Error{Tool: "gh-comments", Msg: "invalid JSON", Err: err}
	}
	if len(resp.Errors) > 0 {
		return nil, 0, &Error{Tool: "gh-comments", Msg: resp.Errors[0].Message}
	}
	pr := resp.Data.Repository
	if pr == nil || pr.PullRequest == nil || pr.PullRequest.Comments == nil {
		return nil, 0, &Error{Tool: "gh-comments", Msg: "response without comments"}
	}
	c := pr.PullRequest.Comments
	return toComments(c.Nodes), c.TotalCount, nil
}

type glCommentsResp struct {
	Data struct {
		Project *struct {
			MergeRequest *struct {
				Notes *struct {
					Nodes []commentNode `json:"nodes"`
				} `json:"notes"`
			} `json:"mergeRequest"`
		} `json:"project"`
	} `json:"data"`
	Errors []struct {
		Message string `json:"message"`
	} `json:"errors"`
}

func ParseGLComments(raw string) ([]model.Comment, int, error) {
	var resp glCommentsResp
	if err := json.Unmarshal([]byte(raw), &resp); err != nil {
		return nil, 0, &Error{Tool: "gl-comments", Msg: "invalid JSON", Err: err}
	}
	if len(resp.Errors) > 0 {
		return nil, 0, &Error{Tool: "gl-comments", Msg: resp.Errors[0].Message}
	}
	mr := resp.Data.Project
	if mr == nil || mr.MergeRequest == nil || mr.MergeRequest.Notes == nil {
		return nil, 0, &Error{Tool: "gl-comments", Msg: "response without notes"}
	}
	notes := toComments(mr.MergeRequest.Notes.Nodes)
	return notes, len(notes), nil
}

// GitLab system notes are the MR's action history, not conversation, and mixed in they eat the pane's
// five rows with noise the detail already shows another way.
func toComments(nodes []commentNode) []model.Comment {
	out := make([]model.Comment, 0, len(nodes))
	for _, n := range nodes {
		if n.System {
			continue
		}
		out = append(out, n.comment())
	}
	return out
}

// Bots put them before their text, and they are the first line of a good part of GitHub
// conversations, so picking a representative line without skipping them reads invisible metadata.
func isHTMLComment(line string) bool {
	t := strings.TrimSpace(line)
	return strings.HasPrefix(t, "<!--") && strings.HasSuffix(t, "-->")
}

func CommentLines(body string) []string {
	out := make([]string, 0, 8)
	for _, raw := range strings.Split(body, "\n") {
		if isHTMLComment(raw) {
			continue
		}
		if flat := strings.Join(strings.Fields(raw), " "); flat != "" {
			out = append(out, flat)
		}
	}
	return out
}
