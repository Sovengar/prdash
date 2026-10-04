package parse

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"prdash/internal/forge/model"
)

const (
	ForgeGitHub = "github"
	ForgeGitLab = "gitlab"
)

type Error struct {
	Tool string // "gh-graphql" | "gh-search" | "gh-checks" | "gl-graphql"…
	Msg  string
	Err  error
}

type PageInfo struct {
	Next string
	More bool
}

func (e *Error) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("parse %s: %s: %v", e.Tool, e.Msg, e.Err)
	}
	return fmt.Sprintf("parse %s: %s", e.Tool, e.Msg)
}

func (e *Error) Unwrap() error { return e.Err }

// GraphQL exposes `ID!` values as strings and REST as numbers. A null, missing or non-numeric
// value reads as 0 and the consumer decides whether to drop the item: a number is never invented.
type flexInt int

func (f *flexInt) UnmarshalJSON(b []byte) error {
	s := strings.TrimSpace(string(b))
	if s == "" || s == "null" {
		*f = 0
		return nil
	}
	s = strings.Trim(s, `"`)
	if s == "" {
		*f = 0
		return nil
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		*f = 0
		return nil
	}
	*f = flexInt(n)
	return nil
}

// The bool pointers separate "the repo has it off" from "the response did not carry the field", which
// is the difference between offering three modes and offering none.
type ghPRNodeRepository struct {
	NameWithOwner string `json:"nameWithOwner"`
	Name          string `json:"name"`
	Owner         struct {
		Login string `json:"login"`
	} `json:"owner"`
	MergeCommitAllowed *bool `json:"mergeCommitAllowed"`
	RebaseMergeAllowed *bool `json:"rebaseMergeAllowed"`
	SquashMergeAllowed *bool `json:"squashMergeAllowed"`
}

type ghPRNode struct {
	Number         int    `json:"number"`
	Title          string `json:"title"`
	URL            string `json:"url"`
	State          string `json:"state"`
	IsDraft        bool   `json:"isDraft"`
	ReviewDecision string `json:"reviewDecision"`
	UpdatedAt      string `json:"updatedAt"`
	HeadRefName    string `json:"headRefName"`
	BaseRefName    string `json:"baseRefName"`
	// gh assumes the branch is in the target repo, so `--delete-branch` on a fork PR deletes nothing
	// while gh reports success. Without this field the "branch deleted" warning lies exactly on the
	// items most closely watched.
	IsCrossRepository bool   `json:"isCrossRepository"`
	Mergeable         string `json:"mergeable"`
	// This is what lets the merge be pinned: without it the branch may have moved since the last read.
	HeadRefOid string `json:"headRefOid"`
	// A pointer on purpose: `additions` is `Int!`, so if the field arrived the query asked for it. Absent
	// means the answer had none (REST fallback), leaving the diffstat unknown rather than zero.
	Additions    *int `json:"additions"`
	Deletions    int  `json:"deletions"`
	ChangedFiles int  `json:"changedFiles"`
	Author       struct {
		Login string `json:"login"`
	} `json:"author"`
	Repository ghPRNodeRepository `json:"repository"`
	Commits    struct {
		Nodes []struct {
			Commit struct {
				StatusCheckRollup *struct {
					State    string `json:"state"`
					Contexts struct {
						Nodes []struct {
							Typename   string `json:"__typename"`
							Status     string `json:"status"`     // CheckRun
							Conclusion string `json:"conclusion"` // CheckRun
							State      string `json:"state"`      // StatusContext
							Context    string `json:"context"`    // StatusContext
						} `json:"nodes"`
					} `json:"contexts"`
				} `json:"statusCheckRollup"`
			} `json:"commit"`
		} `json:"nodes"`
	} `json:"commits"`
}

type ghGraphQLResp struct {
	Data struct {
		Search *struct {
			PageInfo struct {
				HasNextPage bool   `json:"hasNextPage"`
				EndCursor   string `json:"endCursor"`
			} `json:"pageInfo"`
			Nodes []ghPRNode `json:"nodes"`
		} `json:"search"`
		Repository *struct {
			PullRequest *ghPRNode `json:"pullRequest"`
		} `json:"repository"`
	} `json:"data"`
	Errors []struct {
		Message string `json:"message"`
	} `json:"errors"`
}

func ParseGHGraphQLSearch(raw string) ([]model.Item, PageInfo, error) {
	var resp ghGraphQLResp
	if err := json.Unmarshal([]byte(raw), &resp); err != nil {
		return nil, PageInfo{}, &Error{Tool: "gh-graphql", Msg: "invalid JSON", Err: err}
	}
	if len(resp.Errors) > 0 {
		return nil, PageInfo{}, &Error{Tool: "gh-graphql", Msg: resp.Errors[0].Message}
	}

	var (
		nodes []ghPRNode
		page  PageInfo
	)
	switch {
	case resp.Data.Search != nil:
		nodes = resp.Data.Search.Nodes
		page = PageInfo{
			Next: resp.Data.Search.PageInfo.EndCursor,
			More: resp.Data.Search.PageInfo.HasNextPage,
		}
	case resp.Data.Repository != nil && resp.Data.Repository.PullRequest != nil:
		nodes = []ghPRNode{*resp.Data.Repository.PullRequest}
	default:
		return nil, PageInfo{}, &Error{Tool: "gh-graphql", Msg: "response without pull request data"}
	}

	items := make([]model.Item, 0, len(nodes))
	for _, n := range nodes {
		if n.Number == 0 {
			continue // nodos de otro tipo en la unión SearchResultItem
		}
		items = append(items, itemFromGHNode(n))
	}
	return items, page, nil
}

func itemFromGHNode(n ghPRNode) model.Item {
	it := model.NewItem(model.RepoRef{
		Forge:   ForgeGitHub,
		Host:    "github.com",
		Project: n.Repository.NameWithOwner,
		Owner:   n.Repository.Owner.Login,
		Name:    n.Repository.Name,
	}, n.Number)
	it.Title = n.Title
	it.Author = n.Author.Login
	it.SourceBranch = n.HeadRefName
	it.TargetBranch = n.BaseRefName
	it.URL = n.URL
	it.State = n.State
	it.IsDraft = n.IsDraft
	it.IsFork = n.IsCrossRepository
	it.ReviewDecision = n.ReviewDecision
	it.Checks = checksFromRollup(n)
	it.Diff = diffFromGHNode(n)
	it.HeadSHA = n.HeadRefOid
	it.Merge = mergeRulesFromGHRepo(n.Repository)
	it.Mergeable = mergeableFromGH(n.Mergeable)
	it.UpdatedAt = parseTime(n.UpdatedAt)
	return it
}

// Known requires ALL THREE flags: with one missing, the answer came from a shape that does not carry
// them all, and assuming the absent ones are false would hide the one mode the repo may well allow.
func mergeRulesFromGHRepo(r ghPRNodeRepository) model.MergeRules {
	if r.MergeCommitAllowed == nil || r.RebaseMergeAllowed == nil || r.SquashMergeAllowed == nil {
		return model.MergeRules{}
	}
	return model.MergeRules{
		Known:       true,
		MergeCommit: *r.MergeCommitAllowed,
		Rebase:      *r.RebaseMergeAllowed,
		Squash:      *r.SquashMergeAllowed,
	}
}

// UNKNOWN is not a yes, it is "not computed yet", and GitHub returns it the first time every time.
// Translating it to mergeable would announce a conflict that does not exist.
func mergeableFromGH(v string) model.Mergeability {
	switch strings.ToUpper(strings.TrimSpace(v)) {
	case "CONFLICTING":
		return model.Mergeability{Known: true, Conflicted: true}
	case "MERGEABLE":
		return model.Mergeability{Known: true}
	default: // "", UNKNOWN y cualquier valor que no conocemos
		return model.Mergeability{}
	}
}

func diffFromGHNode(n ghPRNode) model.DiffStat {
	if n.Additions == nil {
		return model.DiffStat{}
	}
	return model.DiffStat{
		Additions: *n.Additions,
		Deletions: n.Deletions,
		Files:     n.ChangedFiles,
		Known:     true,
	}
}

func checksFromRollup(n ghPRNode) model.Checks {
	if len(n.Commits.Nodes) == 0 {
		return model.Checks{}
	}
	rollup := n.Commits.Nodes[0].Commit.StatusCheckRollup
	if rollup == nil {
		return model.Checks{}
	}

	var c model.Checks
	for _, ctx := range rollup.Contexts.Nodes {
		c.Total++
		switch {
		case isGHFailure(ctx.Conclusion, ctx.State):
			c.Failing++
		case isGHPending(ctx.Status, ctx.State):
			c.Pending++
		}
	}
	switch {
	case c.Failing > 0:
		c.State = model.ChecksFailing
	case c.Pending > 0:
		c.State = model.ChecksPending
	case c.Total > 0:
		c.State = model.ChecksPassing
	default:
		c.State = checksStateFromRollup(rollup.State)
	}
	return c
}

func isGHFailure(conclusion, state string) bool {
	switch strings.ToUpper(conclusion) {
	case "FAILURE", "TIMED_OUT", "CANCELLED", "ACTION_REQUIRED", "STARTUP_FAILURE", "STALE":
		return true
	}
	return strings.EqualFold(state, "FAILURE") || strings.EqualFold(state, "ERROR")
}

func isGHPending(status, state string) bool {
	if status != "" && !strings.EqualFold(status, "COMPLETED") {
		return true
	}
	switch strings.ToUpper(state) {
	case "PENDING", "EXPECTED", "QUEUED", "IN_PROGRESS", "WAITING":
		return true
	}
	return false
}

func checksStateFromRollup(state string) model.CheckState {
	switch strings.ToUpper(state) {
	case "SUCCESS":
		return model.ChecksPassing
	case "FAILURE", "ERROR":
		return model.ChecksFailing
	case "PENDING", "EXPECTED":
		return model.ChecksPending
	default:
		return model.ChecksUnknown
	}
}

type ghSearchIssuesResp struct {
	TotalCount int `json:"total_count"`
	Items      []struct {
		Number    int    `json:"number"`
		Title     string `json:"title"`
		HTMLURL   string `json:"html_url"`
		State     string `json:"state"`
		Draft     bool   `json:"draft"`
		UpdatedAt string `json:"updated_at"`
		User      struct {
			Login string `json:"login"`
		} `json:"user"`
		PullRequest *struct {
			Head struct {
				Ref string `json:"ref"`
			} `json:"head"`
			Base struct {
				Ref string `json:"ref"`
			} `json:"base"`
		} `json:"pull_request"`
		RepositoryURL string `json:"repository_url"`
	} `json:"items"`
}

func ParseGHAuthored(raw string) ([]model.Item, error) {
	var resp ghSearchIssuesResp
	if err := json.Unmarshal([]byte(raw), &resp); err != nil {
		return nil, &Error{Tool: "gh-search", Msg: "invalid JSON", Err: err}
	}

	items := make([]model.Item, 0, len(resp.Items))
	for _, n := range resp.Items {
		owner, name := splitRepoURL(n.RepositoryURL)
		it := model.NewItem(model.RepoRef{
			Forge:   ForgeGitHub,
			Host:    "github.com",
			Project: joinProject(owner, name),
			Owner:   owner,
			Name:    name,
		}, n.Number)
		it.Title = n.Title
		it.Author = n.User.Login
		it.URL = n.HTMLURL
		it.State = n.State
		it.IsDraft = n.Draft
		if n.PullRequest != nil {
			it.SourceBranch = n.PullRequest.Head.Ref
			it.TargetBranch = n.PullRequest.Base.Ref
		}
		it.UpdatedAt = parseTime(n.UpdatedAt)
		items = append(items, it)
	}
	return items, nil
}

type ghCheck struct {
	Name   string `json:"name"`
	State  string `json:"state"`
	Bucket string `json:"bucket"`
}

func ParseGHChecks(raw string) (model.Checks, error) {
	var checks []ghCheck
	if err := json.Unmarshal([]byte(raw), &checks); err != nil {
		return model.Checks{}, &Error{Tool: "gh-checks", Msg: "invalid JSON", Err: err}
	}

	var c model.Checks
	for _, ch := range checks {
		c.Total++
		switch strings.ToLower(ch.Bucket) {
		case "fail", "cancel":
			c.Failing++
		case "pending":
			c.Pending++
		case "pass", "skipping":
		default:
			switch {
			case isGHFailure("", ch.State):
				c.Failing++
			case isGHPending("", ch.State):
				c.Pending++
			}
		}
	}
	switch {
	case c.Failing > 0:
		c.State = model.ChecksFailing
	case c.Pending > 0:
		c.State = model.ChecksPending
	case c.Total > 0:
		c.State = model.ChecksPassing
	default:
		c.State = model.ChecksUnknown
	}
	return c, nil
}

type glMR struct {
	IID    flexInt `json:"iid"`
	Title  string  `json:"title"`
	WebURL string  `json:"webUrl"`
	State  string  `json:"state"`
	// Separate from State, which is the forge's enum ("opened") and does not cover it: without this a draft
	// MR was indistinguishable from an open one.
	Draft bool `json:"draft"`
	// The detailed one, not `mergeStatus`, because the simple one cannot tell "collides" from "pipeline
	// missing": with the simple one a red CI is announced as a branch conflict, which is a lie. GitLab
	// computes it per MR per request, so it is a calculation and not an extra call.
	DetailedMergeStatus string `json:"detailedMergeStatus"`
	SourceBranch        string `json:"sourceBranch"`
	TargetBranch        string `json:"targetBranch"`
	Approved            bool   `json:"approved"`
	UpdatedAt           string `json:"updatedAt"`
	// A pointer because GitLab declares it nullable and returns null while the diff is uncomputed;
	// absent and null both mean "cannot be pinned".
	DiffHeadSha *string `json:"diffHeadSha"`
	Squash      *bool   `json:"squash"`
	DiffStats   *[]struct {
		Additions flexInt `json:"additions"`
		Deletions flexInt `json:"deletions"`
	} `json:"diffStats"`
	Author struct {
		Username string `json:"username"`
	} `json:"author"`
	Project struct {
		FullPath string `json:"fullPath"`
		Name     string `json:"name"`
		Group    *struct {
			FullPath string `json:"fullPath"`
		} `json:"group"`
	} `json:"project"`
}

type glGraphQLResp struct {
	Data struct {
		CurrentUser *struct {
			AuthoredMergeRequests        *glConn `json:"authoredMergeRequests"`
			ReviewRequestedMergeRequests *glConn `json:"reviewRequestedMergeRequests"`
			AssignedMergeRequests        *glConn `json:"assignedMergeRequests"`
		} `json:"currentUser"`
		Project *struct {
			MergeRequest *glMR `json:"mergeRequest"`
		} `json:"project"`
	} `json:"data"`
	Errors []struct {
		Message string `json:"message"`
	} `json:"errors"`
}

type glConn struct {
	PageInfo struct {
		HasNextPage bool   `json:"hasNextPage"`
		EndCursor   string `json:"endCursor"`
	} `json:"pageInfo"`
	Nodes []glMR `json:"nodes"`
}

func ParseGLGraphQL(raw string) ([]model.Item, PageInfo, error) {
	var resp glGraphQLResp
	if err := json.Unmarshal([]byte(raw), &resp); err != nil {
		return nil, PageInfo{}, &Error{Tool: "gl-graphql", Msg: "invalid JSON", Err: err}
	}
	if len(resp.Errors) > 0 {
		return nil, PageInfo{}, &Error{Tool: "gl-graphql", Msg: resp.Errors[0].Message}
	}

	var (
		items []model.Item
		page  PageInfo
	)
	pageable := false
	if cu := resp.Data.CurrentUser; cu != nil {
		for _, conn := range []*glConn{cu.AuthoredMergeRequests, cu.ReviewRequestedMergeRequests, cu.AssignedMergeRequests} {
			if conn == nil {
				continue
			}
			if !pageable {
				page = PageInfo{Next: conn.PageInfo.EndCursor, More: conn.PageInfo.HasNextPage}
				pageable = true
			}
		}
		items = append(items, glItems(cu.AuthoredMergeRequests, model.SectionAuthored, "")...)
		items = append(items, glItems(cu.ReviewRequestedMergeRequests, model.SectionReview, model.ReviewRequested)...)
		items = append(items, glItems(cu.AssignedMergeRequests, model.SectionReview, model.ReviewAssigned)...)
	}
	if resp.Data.Project != nil && resp.Data.Project.MergeRequest != nil {
		items = append(items, itemFromGLMR(*resp.Data.Project.MergeRequest, "", ""))
	}
	return items, page, nil
}

func glItems(conn *glConn, section model.Section, kind model.ReviewKind) []model.Item {
	if conn == nil {
		return nil
	}
	out := make([]model.Item, 0, len(conn.Nodes))
	for _, mr := range conn.Nodes {
		if mr.IID == 0 {
			continue // nodo sin iid utilizable
		}
		out = append(out, itemFromGLMR(mr, section, kind))
	}
	return out
}

func itemFromGLMR(mr glMR, section model.Section, kind model.ReviewKind) model.Item {
	owner := mr.Project.FullPath
	if i := strings.LastIndex(owner, "/"); i >= 0 {
		owner = owner[:i]
	} else {
		owner = ""
	}
	it := model.NewItem(model.RepoRef{
		Forge:   ForgeGitLab,
		Host:    "",
		Project: mr.Project.FullPath,
		Owner:   owner,
		Name:    mr.Project.Name,
	}, int(mr.IID))
	it.Section = section
	it.ReviewKind = kind
	it.Title = mr.Title
	it.Author = mr.Author.Username
	it.SourceBranch = mr.SourceBranch
	it.TargetBranch = mr.TargetBranch
	it.URL = mr.WebURL
	it.State = mr.State
	it.IsDraft = mr.Draft
	it.Mergeable = mergeableFromGL(mr.DetailedMergeStatus)
	it.ReviewDecision = glReviewDecision(mr)
	it.Diff = diffFromGLMR(mr)
	if mr.DiffHeadSha != nil {
		it.HeadSHA = *mr.DiffHeadSha
	}
	// GitLab does not expose the merge strategies in GraphQL and reading them over REST would cost a
	// call per repository, so the rules stay unknown, which restricts nothing.
	it.Merge = model.MergeRules{}
	it.UpdatedAt = parseTime(mr.UpdatedAt)
	return it
}

// The enum is accepted in both spellings because REST lowercases it and GraphQL does not: a
// conflict that does not exist is a false warning, and a false warning that repeats trains the user to
// ignore the whole box. `need_rebase` is NOT a conflict: the branch integrates without touching anything.
func mergeableFromGL(v string) model.Mergeability {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "conflict", "broken_status":
		return model.Mergeability{Known: true, Conflicted: true}
	case "":
		return model.Mergeability{}
	default:
		return model.Mergeability{Known: true}
	}
}

// The file count comes from the length of the list and can therefore be short: GitLab collapses
// diffs past its size and row limits, so on a huge MR the numbers are a minimum, not an exact figure.
func diffFromGLMR(mr glMR) model.DiffStat {
	if mr.DiffStats == nil {
		return model.DiffStat{}
	}
	var d model.DiffStat
	for _, s := range *mr.DiffStats {
		d.Additions += int(s.Additions)
		d.Deletions += int(s.Deletions)
	}
	d.Files = len(*mr.DiffStats)
	d.Known = true
	return d
}

func glReviewDecision(mr glMR) string {
	if mr.Approved {
		return "APPROVED"
	}
	return ""
}

type glBasicMR struct {
	IID    flexInt `json:"iid"`
	Title  string  `json:"title"`
	WebURL string  `json:"web_url"`
	State  string  `json:"state"`
	Draft  bool    `json:"draft"`
	// It arrives in the REST list response, so reading it costs nothing. The deprecated `merge_status` is
	// ignored on purpose: it cannot tell a conflict from a red pipeline.
	DetailedMergeStatus string `json:"detailed_merge_status"`
	SourceBranch        string `json:"source_branch"`
	TargetBranch        string `json:"target_branch"`
	UpdatedAt           string `json:"updated_at"`
	Author              struct {
		Username string `json:"username"`
	} `json:"author"`
	References struct {
		Full  string `json:"full"`  // "grupo/proy!12"
		Short string `json:"short"` // "!12"
	} `json:"references"`
}

func ParseGLMRList(raw string) ([]model.Item, error) {
	var list []glBasicMR
	if err := json.Unmarshal([]byte(raw), &list); err != nil {
		return nil, &Error{Tool: "gl-mr-list", Msg: "invalid JSON", Err: err}
	}

	items := make([]model.Item, 0, len(list))
	for _, mr := range list {
		project := projectFromRef(mr.References.Full)
		owner, name := splitProject(project)
		it := model.NewItem(model.RepoRef{
			Forge:   ForgeGitLab,
			Host:    "",
			Project: project,
			Owner:   owner,
			Name:    name,
		}, int(mr.IID))
		it.Title = mr.Title
		it.Author = mr.Author.Username
		it.SourceBranch = mr.SourceBranch
		it.TargetBranch = mr.TargetBranch
		it.URL = mr.WebURL
		it.State = mr.State
		it.IsDraft = mr.Draft
		it.Mergeable = mergeableFromGL(mr.DetailedMergeStatus)
		it.UpdatedAt = parseTime(mr.UpdatedAt)
		items = append(items, it)
	}
	return items, nil
}

type glTodo struct {
	ID         flexInt `json:"id"`
	ActionName string  `json:"action_name"`
	TargetType string  `json:"target_type"`
	TargetURL  string  `json:"target_url"`
	UpdatedAt  string  `json:"updated_at"`
	Target     *struct {
		IID          flexInt `json:"iid"`
		Title        string  `json:"title"`
		WebURL       string  `json:"web_url"`
		State        string  `json:"state"`
		SourceBranch string  `json:"source_branch"`
		TargetBranch string  `json:"target_branch"`
		Author       struct {
			Username string `json:"username"`
		} `json:"author"`
		References struct {
			Full string `json:"full"`
		} `json:"references"`
	} `json:"target"`
}

func ParseGLTodos(raw string) ([]model.Item, int, error) {
	var todos []glTodo
	if err := json.Unmarshal([]byte(raw), &todos); err != nil {
		return nil, 0, &Error{Tool: "gl-todos", Msg: "invalid JSON", Err: err}
	}

	items := []model.Item{}
	for _, td := range todos {
		if td.Target == nil || td.TargetType != "MergeRequest" {
			continue
		}
		if td.ActionName != "mentioned" && td.ActionName != "directly_addressed" {
			continue
		}
		if td.Target.IID == 0 {
			continue // sin iid utilizable
		}
		project := projectFromRef(td.Target.References.Full)
		owner, name := splitProject(project)
		it := model.NewItem(model.RepoRef{
			Forge:   ForgeGitLab,
			Host:    "",
			Project: project,
			Owner:   owner,
			Name:    name,
		}, int(td.Target.IID))
		it.Section = model.SectionMentions
		it.Title = td.Target.Title
		it.Author = td.Target.Author.Username
		it.SourceBranch = td.Target.SourceBranch
		it.TargetBranch = td.Target.TargetBranch
		it.URL = td.Target.WebURL
		it.State = td.Target.State
		it.UpdatedAt = parseTime(td.UpdatedAt)
		items = append(items, it)
	}
	return items, len(todos), nil
}

func parseTime(s string) time.Time {
	if s == "" {
		return time.Time{}
	}
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t
	}
	return time.Time{}
}

func splitRepoURL(u string) (string, string) {
	const marker = "/repos/"
	if i := strings.Index(u, marker); i >= 0 {
		return splitProject(u[i+len(marker):])
	}
	return "", ""
}

func joinProject(owner, name string) string {
	switch {
	case owner == "":
		return name
	case name == "":
		return owner
	default:
		return owner + "/" + name
	}
}

func splitProject(path string) (string, string) {
	path = strings.Trim(path, "/")
	if i := strings.LastIndex(path, "/"); i >= 0 {
		return path[:i], path[i+1:]
	}
	return "", path
}

func projectFromRef(ref string) string {
	if i := strings.LastIndex(ref, "!"); i >= 0 {
		return ref[:i]
	}
	return ref
}
