// Package gitlab implements the self-managed GitLab forge over the `glab` CLI, with paginated
// GraphQL and the Todos API. glab resolves its own host, so the paths are relative.
package gitlab

import (
	"context"
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"prdash/internal/forge"
	"prdash/internal/forge/model"
	"prdash/internal/forge/parse"
	"prdash/internal/forge/tool"
)

const ForgeName = "gitlab"

const pageSize = 50

// Same reason as GitHub: the margin keeps an MR full of system notes from coming up short.
const commentFetch = 3 * forge.CommentLimit

type Adapter struct {
	host   string
	runner *tool.Runner
}

var _ forge.Adapter = (*Adapter)(nil)

func New(host, bin string) *Adapter {
	if bin == "" {
		bin = "glab"
	}
	if host == "" {
		host = "gitlab.example.com"
	}
	return &Adapter{
		host:   host,
		runner: tool.New(bin, "GLAB_NO_PROMPT=1", "GITLAB_HOST="+host),
	}
}

func (a *Adapter) Forge() string { return ForgeName }

func (a *Adapter) Host() string { return a.host }

func (a *Adapter) Auth(ctx context.Context) model.AuthState {
	out, err := a.runner.Run(ctx, a.authArgs()...)
	if err != nil {
		return model.AuthState{Forge: ForgeName, OK: false, Reason: err.Error()}
	}
	return model.AuthState{Forge: ForgeName, OK: true, Login: loginFromAuthStatus(out)}
}

func loginFromAuthStatus(out string) string {
	m := loginRe.FindStringSubmatch(out)
	if m == nil {
		return ""
	}
	return m[1]
}

var loginRe = regexp.MustCompile(`\bas ([^(\s]+)`)

func (a *Adapter) authArgs() []string {
	return []string{"auth", "status", "--hostname", a.host}
}

func (a *Adapter) graphqlArgs(query string) []string {
	return []string{"api", "--hostname", a.host, "graphql", "-f", "query=" + query}
}

// Without `-X GET`, passing fields turns the request into a POST.
func (a *Adapter) getArgs(endpoint string, fields ...string) []string {
	args := []string{"api", "--hostname", a.host, "-X", "GET", endpoint}
	for _, f := range fields {
		args = append(args, "-f", f)
	}
	return args
}

func (a *Adapter) mrArgs(sub string, number int, project string, extra ...string) []string {
	args := []string{"mr", sub, strconv.Itoa(number), "-R", project}
	return append(args, extra...)
}

func (a *Adapter) List(ctx context.Context, q forge.Query) (forge.Page, []model.Warning) {
	switch {
	case q.Section == model.SectionAuthored:
		return a.graphqlList(ctx, q, glAuthoredQuery(q.Cursor))
	case q.Section == model.SectionReview && q.ReviewKind == model.ReviewAssigned:
		return a.graphqlList(ctx, q, glAssignedQuery(q.Cursor))
	case q.Section == model.SectionReview:
		return a.graphqlList(ctx, q, glReviewQuery(q.Cursor))
	case q.Section == model.SectionMentions:
		return a.todosList(ctx, q)
	default:
		return forge.Page{}, []model.Warning{a.warn(q.Section, "unsupported", fmt.Errorf("unsupported list: %s", q.Section))}
	}
}

func (a *Adapter) ItemState(ctx context.Context, ref model.RepoRef, number int) (model.Item, []model.Warning) {
	if ref.Project == "" {
		return model.Item{}, []model.Warning{a.warn("", "notfound", fmt.Errorf("empty repo reference"))}
	}

	raw, err := a.runner.Run(ctx, a.graphqlArgs(glMRQuery(ref.Project, number))...)
	if err != nil {
		return model.Item{}, []model.Warning{a.warn("", tool.Kind(err), err)}
	}
	items, _, perr := parse.ParseGLGraphQL(raw)
	if perr != nil {
		return model.Item{}, []model.Warning{a.warn("", "parse", perr)}
	}
	if len(items) == 0 {
		return model.Item{}, []model.Warning{a.warn("", "notfound", fmt.Errorf("MR !%d not found in %s", number, ref.Project))}
	}
	it := items[0]
	a.identity(&it)
	return it, nil
}

func (a *Adapter) Comments(ctx context.Context, ref model.RepoRef, number int) (forge.CommentPage, []model.Warning) {
	if ref.Project == "" {
		return forge.CommentPage{}, []model.Warning{a.warn("", "notfound", fmt.Errorf("empty repo reference"))}
	}

	raw, err := a.runner.Run(ctx, a.graphqlArgs(glNotesQuery(ref.Project, number, commentFetch))...)
	if err != nil {
		return forge.CommentPage{}, []model.Warning{a.warn("", tool.Kind(err), err)}
	}
	comments, total, perr := parse.ParseGLComments(raw)
	if perr != nil {
		return forge.CommentPage{}, []model.Warning{a.warn("", "parse", perr)}
	}
	return forge.CommentPage{Comments: comments, Total: total}.KeepLast(), nil
}

func (a *Adapter) Approve(ctx context.Context, ref model.RepoRef, number int) []model.Warning {
	return a.action(ctx, a.mrArgs("approve", number, ref.Project)...)
}

// `--auto-merge=false` is not optional: glab defaults it to true, so with a pipeline running the
// command queued the MR and exited 0. `--sha` is the same trap the other way: an empty headSHA refuses.
func (a *Adapter) Merge(ctx context.Context, ref model.RepoRef, number int, req forge.MergeRequest) []model.Warning {
	extra := []string{"--yes", "--auto-merge=false"}
	if flag, ok := glabMergeFlag(req.Mode); ok {
		extra = append(extra, flag)
	} else {
		return []model.Warning{a.warn("", "unsupported", forge.ErrUnknownMergeMode(req.Mode))}
	}
	if strings.TrimSpace(req.HeadSHA) == "" {
		return []model.Warning{a.warn("", "unsupported", forge.ErrMissingHeadSHA)}
	}
	extra = append(extra, "--sha", req.HeadSHA)
	// The flag is `-d`/`--remove-source-branch`, not gh's `--delete-branch`, and it forces the
	// behaviour on projects that leave "delete source branch" to their own default.
	if req.DeleteBranch {
		extra = append(extra, "--remove-source-branch")
	}
	return a.action(ctx, a.mrArgs("merge", number, ref.Project, extra...)...)
}

func glabMergeFlag(mode forge.MergeMode) (string, bool) {
	switch mode {
	case forge.MergeCommit:
		return "", true
	case forge.Rebase:
		return "--rebase", true
	case forge.Squash:
		return "--squash", true
	default:
		return "", false
	}
}

// A PUT, not `glab mr update --target-branch`: an EDIT command that opens an editor, and in a
// subprocess it fails rather than hangs. Explicit because `-f` makes glab POST, which updates nothing.
func (a *Adapter) Retarget(ctx context.Context, ref model.RepoRef, number int, branch string) []model.Warning {
	if strings.TrimSpace(branch) == "" {
		return []model.Warning{a.warn("", "unsupported", forge.ErrMissingBaseBranch)}
	}
	args := a.mrAPIArgs("PUT", mrEndpoint(ref.Project, number), "target_branch="+branch)
	out, err := a.runner.Run(ctx, args...)
	if err == nil {
		return nil
	}
	msg := err.Error()
	if api := tool.APIMessage(out); api != "" {
		msg = api
	}
	return []model.Warning{{Forge: ForgeName, Kind: tool.Kind(err), Msg: msg}}
}

func (a *Adapter) Branches(ctx context.Context, ref model.RepoRef) ([]string, []model.Warning) {
	if strings.TrimSpace(ref.Project) == "" {
		return nil, []model.Warning{a.warn("", "notfound", fmt.Errorf("empty repo reference"))}
	}
	endpoint := "projects/" + url.QueryEscape(ref.Project) + "/repository/branches?per_page=100"
	raw, err := a.runner.Run(ctx, "api", "--hostname", a.host, "-X", "GET", endpoint,
		"--paginate", "--output", "ndjson")
	if err != nil {
		return nil, []model.Warning{a.warn("", tool.Kind(err), err)}
	}
	names, perr := parse.ParseGLBranches(raw)
	if perr != nil {
		return nil, []model.Warning{a.warn("", "parse", perr)}
	}
	return names, nil
}

func mrEndpoint(project string, number int) string {
	return "projects/" + url.QueryEscape(project) + "/merge_requests/" + strconv.Itoa(number)
}

func (a *Adapter) mrAPIArgs(method, endpoint string, fields ...string) []string {
	args := []string{"api", "--hostname", a.host, "-X", method, endpoint}
	for _, f := range fields {
		args = append(args, "-f", f)
	}
	return args
}

func (a *Adapter) action(ctx context.Context, args ...string) []model.Warning {
	if _, err := a.runner.Run(ctx, args...); err != nil {
		return []model.Warning{a.warn("", tool.Kind(err), err)}
	}
	return nil
}

func (a *Adapter) graphqlList(ctx context.Context, q forge.Query, query string) (forge.Page, []model.Warning) {
	raw, err := a.runner.Run(ctx, a.graphqlArgs(query)...)
	if err != nil {
		if q.Section == model.SectionAuthored && q.Cursor == "" {
			if p, ok := a.restAuthored(ctx); ok {
				return p, []model.Warning{a.degraded(q.Section, err)}
			}
		}
		return forge.Page{}, []model.Warning{a.warn(q.Section, tool.Kind(err), err)}
	}
	items, page, perr := parse.ParseGLGraphQL(raw)
	if perr != nil {
		return forge.Page{}, []model.Warning{a.warn(q.Section, "parse", perr)}
	}
	a.stamp(items, q)
	return forge.Page{Items: items, Next: page.Next, More: page.More}, nil
}

func (a *Adapter) todosList(ctx context.Context, q forge.Query) (forge.Page, []model.Warning) {
	pageNum := 1
	if n, err := strconv.Atoi(q.Cursor); err == nil && n > 0 {
		pageNum = n
	}
	raw, err := a.runner.Run(ctx, a.getArgs("todos",
		"action=mentioned",
		"per_page="+strconv.Itoa(pageSize),
		"page="+strconv.Itoa(pageNum))...)
	if err != nil {
		return forge.Page{}, []model.Warning{a.warn(q.Section, tool.Kind(err), err)}
	}
	items, total, perr := parse.ParseGLTodos(raw)
	if perr != nil {
		return forge.Page{}, []model.Warning{a.warn(q.Section, "parse", perr)}
	}
	a.stamp(items, q)

	page := forge.Page{Items: items}
	if total >= pageSize {
		page.More = true
		page.Next = strconv.Itoa(pageNum + 1)
	}
	return page, nil
}

func (a *Adapter) restAuthored(ctx context.Context) (forge.Page, bool) {
	raw, err := a.runner.Run(ctx, a.getArgs("merge_requests",
		"scope=created_by_me", "state=opened", "per_page="+strconv.Itoa(pageSize))...)
	if err != nil {
		return forge.Page{}, false
	}
	items, perr := parse.ParseGLMRList(raw)
	if perr != nil {
		return forge.Page{}, false
	}
	a.stamp(items, forge.Query{Section: model.SectionAuthored})
	return forge.Page{Items: items}, true
}

func (a *Adapter) stamp(items []model.Item, q forge.Query) {
	for i := range items {
		items[i].Section = q.Section
		if q.Section == model.SectionReview {
			items[i].ReviewKind = q.ReviewKind
		}
		a.identity(&items[i])
	}
}

func (a *Adapter) identity(it *model.Item) {
	it.Forge = ForgeName
	it.Host = a.host
	it.Ref.Forge = ForgeName
	it.Ref.Host = a.host
}

func (a *Adapter) warn(section model.Section, kind string, err error) model.Warning {
	return model.Warning{Forge: ForgeName, Section: section, Kind: kind, Msg: err.Error()}
}

func (a *Adapter) degraded(section model.Section, err error) model.Warning {
	return model.Warning{
		Forge:   ForgeName,
		Section: section,
		Kind:    "degraded",
		Msg:     "GraphQL unavailable; partial data via REST: " + tool.FirstLine(err.Error()),
	}
}

func restEndpoint(resource string) string {
	return strings.TrimLeft(resource, "/")
}

// `diffStats` is one entry PER CHANGED FILE and the schema exposes no `changedFiles`, so the count
// is the list's length. The merge strategies are deliberately not asked: unknown does not restrict.
const mrFields = `iid title webUrl state draft sourceBranch targetBranch approved updatedAt ` +
	`diffHeadSha squash detailedMergeStatus diffStats { additions deletions } ` +
	`author { username } project { fullPath name group { fullPath } }`

const glConn = `pageInfo { hasNextPage endCursor } nodes { %s }`

func glAuthoredQuery(cursor string) string {
	return fmt.Sprintf(
		`query { currentUser { authoredMergeRequests(state: opened, first: %d%s) { %s } } }`,
		pageSize, afterArg(cursor), fmt.Sprintf(glConn, mrFields),
	)
}

func glReviewQuery(cursor string) string {
	return fmt.Sprintf(
		`query { currentUser { reviewRequestedMergeRequests(state: opened, first: %d%s) { %s } } }`,
		pageSize, afterArg(cursor), fmt.Sprintf(glConn, mrFields),
	)
}

func glAssignedQuery(cursor string) string {
	return fmt.Sprintf(
		`query { currentUser { assignedMergeRequests(state: opened, first: %d%s) { %s } } }`,
		pageSize, afterArg(cursor), fmt.Sprintf(glConn, mrFields),
	)
}

// The iid goes as a string literal because the schema declares it `String!`: GraphQL does not
// coerce an Int literal into a String. `last` rather than `first`, as on GitHub.
func glNotesQuery(fullPath string, iid, last int) string {
	return fmt.Sprintf(
		`query { project(fullPath: "%s") { mergeRequest(iid: "%d") { `+
			`notes(last: %d) { nodes { author { username } body createdAt system } } } } }`,
		escapeGraphQL(fullPath), iid, last,
	)
}

// An Int literal for a `String!` field is rejected with argumentLiteralsIncompatible and takes
// the whole query down; flexInt only handles the iid arriving as a string in the RESPONSE.
func glMRQuery(fullPath string, iid int) string {
	return fmt.Sprintf(
		`query { project(fullPath: "%s") { mergeRequest(iid: "%d") { %s } } }`,
		escapeGraphQL(fullPath), iid, mrFields,
	)
}

func afterArg(cursor string) string {
	if cursor == "" {
		return ""
	}
	return fmt.Sprintf(`, after: "%s"`, escapeGraphQL(cursor))
}

func escapeGraphQL(s string) string { return forge.EscapeGraphQL(s) }
