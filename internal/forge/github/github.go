package github

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"prdash/internal/forge"
	"prdash/internal/forge/model"
	"prdash/internal/forge/parse"
	"prdash/internal/forge/tool"
)

const ForgeName = "github"

const pageSize = 50

const commentFetch = 15

type Adapter struct {
	host   string
	runner *tool.Runner
}

var _ forge.Adapter = (*Adapter)(nil)

func New(host, bin string) *Adapter {
	if bin == "" {
		bin = "gh"
	}
	if host == "" {
		host = "github.com"
	}
	return &Adapter{host: host, runner: tool.New(bin, "GH_PROMPT_DISABLED=1")}
}

func (a *Adapter) Forge() string { return ForgeName }

func (a *Adapter) Host() string { return a.host }

func (a *Adapter) Auth(ctx context.Context) model.AuthState {
	out, err := a.runner.Run(ctx, "auth", "status", "--hostname", a.host)
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

var loginRe = regexp.MustCompile(`account ([^(\s]+)`)

func (a *Adapter) List(ctx context.Context, q forge.Query) (forge.Page, []model.Warning) {
	qualifier, ok := qualifierFor(q)
	if !ok {
		return forge.Page{}, []model.Warning{a.warn(q.Section, "unsupported", fmt.Errorf("unsupported list: %s", q.Section))}
	}

	raw, err := a.runner.Run(ctx, "api", "graphql", "-f", "query="+searchQuery(qualifier, q.Cursor))
	if err != nil {
		if q.Section == model.SectionAuthored && q.Cursor == "" {
			if p, ok := a.restAuthored(ctx); ok {
				return p, []model.Warning{a.degraded(q.Section, err)}
			}
		}
		return forge.Page{}, []model.Warning{a.warn(q.Section, tool.Kind(err), err)}
	}

	items, page, perr := parse.ParseGHGraphQLSearch(raw)
	if perr != nil {
		return forge.Page{}, []model.Warning{a.warn(q.Section, "parse", perr)}
	}
	a.stamp(items, q)
	return forge.Page{Items: items, Next: page.Next, More: page.More}, nil
}

func (a *Adapter) ItemState(ctx context.Context, ref model.RepoRef, number int) (model.Item, []model.Warning) {
	owner, name := splitProject(ref.Project)
	if owner == "" {
		return model.Item{}, []model.Warning{a.warn("", "notfound", fmt.Errorf("invalid repo reference: %q", ref.Project))}
	}

	raw, err := a.runner.Run(ctx, "api", "graphql", "-f", "query="+prQuery(owner, name, number))
	if err != nil {
		return model.Item{}, []model.Warning{a.warn("", tool.Kind(err), err)}
	}
	items, _, perr := parse.ParseGHGraphQLSearch(raw)
	if perr != nil {
		return model.Item{}, []model.Warning{a.warn("", "parse", perr)}
	}
	if len(items) == 0 {
		return model.Item{}, []model.Warning{a.warn("", "notfound", fmt.Errorf("PR #%d not found in %s", number, ref.Project))}
	}

	it := items[0]
	a.identity(&it)
	if checks, warns := a.checks(ctx, ref.Project, number); warns == nil {
		it.Checks = checks
	}
	return it, nil
}

func (a *Adapter) Comments(ctx context.Context, ref model.RepoRef, number int) (forge.CommentPage, []model.Warning) {
	owner, name := splitProject(ref.Project)
	if owner == "" || name == "" {
		return forge.CommentPage{}, []model.Warning{a.warn("", "notfound", fmt.Errorf("invalid repo reference: %q", ref.Project))}
	}

	raw, err := a.runner.Run(ctx, "api", "graphql", "-f", "query="+commentsQuery(owner, name, number, commentFetch))
	if err != nil {
		return forge.CommentPage{}, []model.Warning{a.warn("", tool.Kind(err), err)}
	}
	comments, total, perr := parse.ParseGHComments(raw)
	if perr != nil {
		return forge.CommentPage{}, []model.Warning{a.warn("", "parse", perr)}
	}
	return forge.CommentPage{Comments: comments, Total: total}.KeepLast(), nil
}

func (a *Adapter) Approve(ctx context.Context, ref model.RepoRef, number int) []model.Warning {
	return a.action(ctx, "pr", "review", strconv.Itoa(number), "--repo", ref.Project, "--approve")
}

func (a *Adapter) Merge(ctx context.Context, ref model.RepoRef, number int, req forge.MergeRequest) []model.Warning {
	flag, ok := ghMergeFlag(req.Mode)
	if !ok {
		return []model.Warning{a.warn("", "unsupported", forge.ErrUnknownMergeMode(req.Mode))}
	}
	if strings.TrimSpace(req.HeadSHA) == "" {
		return []model.Warning{a.warn("", "unsupported", forge.ErrMissingHeadSHA)}
	}
	args := []string{"pr", "merge", strconv.Itoa(number), "--repo", ref.Project,
		flag, "--match-head-commit", req.HeadSHA}
	if req.DeleteBranch {
		args = append(args, "--delete-branch")
	}
	return a.action(ctx, args...)
}

func ghMergeFlag(mode forge.MergeMode) (string, bool) {
	switch mode {
	case forge.MergeCommit:
		return "--merge", true
	case forge.Rebase:
		return "--rebase", true
	case forge.Squash:
		return "--squash", true
	default:
		return "", false
	}
}

func (a *Adapter) Retarget(ctx context.Context, ref model.RepoRef, number int, branch string) []model.Warning {
	if strings.TrimSpace(branch) == "" {
		return []model.Warning{a.warn("", "unsupported", forge.ErrMissingBaseBranch)}
	}
	out, err := a.runner.Run(ctx, "api", "-X", "PATCH", pullsEndpoint(ref.Project, number), "-f", "base="+branch)
	if err == nil {
		return nil
	}
	return []model.Warning{{Forge: ForgeName, Kind: tool.Kind(err), Msg: failureMsg(out, err)}}
}

func (a *Adapter) Branches(ctx context.Context, ref model.RepoRef) ([]string, []model.Warning) {
	if strings.TrimSpace(ref.Project) == "" {
		return nil, []model.Warning{a.warn("", "notfound", fmt.Errorf("empty repo reference"))}
	}
	raw, err := a.runner.Run(ctx, "api", "repos/"+ref.Project+"/branches?per_page=100",
		"--paginate", "--jq", ".[].name")
	if err != nil {
		return nil, []model.Warning{a.warn("", tool.Kind(err), err)}
	}
	return parse.ParseGHBranches(raw), nil
}

func pullsEndpoint(project string, number int) string {
	return "repos/" + project + "/pulls/" + strconv.Itoa(number)
}

func failureMsg(body string, err error) string {
	if msg := tool.APIMessage(body); msg != "" {
		return msg
	}
	return err.Error()
}

func (a *Adapter) action(ctx context.Context, args ...string) []model.Warning {
	if _, err := a.runner.Run(ctx, args...); err != nil {
		return []model.Warning{a.warn("", tool.Kind(err), err)}
	}
	return nil
}

func (a *Adapter) checks(ctx context.Context, project string, number int) (model.Checks, []model.Warning) {
	raw, err := a.runner.Run(ctx, "pr", "checks", strconv.Itoa(number),
		"--repo", project, "--json", "name,state,bucket")
	if c, perr := parse.ParseGHChecks(raw); perr == nil {
		return c, nil
	}
	if err != nil {
		return model.Checks{}, []model.Warning{a.warn("", tool.Kind(err), err)}
	}
	return model.Checks{}, []model.Warning{a.warn("", "parse", fmt.Errorf("unreadable checks output"))}
}

func (a *Adapter) restAuthored(ctx context.Context) (forge.Page, bool) {
	raw, err := a.runner.Run(ctx, "api", "-X", "GET", "search/issues",
		"-f", "q=is:pr is:open author:@me", "-f", "per_page="+strconv.Itoa(pageSize))
	if err != nil {
		return forge.Page{}, false
	}
	items, perr := parse.ParseGHAuthored(raw)
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
		Msg:     "GraphQL unavailable; partial data via REST (no branches or checks): " + tool.FirstLine(err.Error()),
	}
}

func qualifierFor(q forge.Query) (string, bool) {
	switch q.Section {
	case model.SectionAuthored:
		return "author:@me", true
	case model.SectionReview:
		if q.ReviewKind == model.ReviewAssigned {
			return "assignee:@me", true
		}
		return "review-requested:@me", true
	case model.SectionMentions:
		return "mentions:@me", true
	default:
		return "", false
	}
}

const ghPRFields = `number title url state isDraft isCrossRepository mergeable reviewDecision updatedAt headRefName baseRefName headRefOid additions deletions changedFiles author { login } repository { nameWithOwner name owner { login } mergeCommitAllowed rebaseMergeAllowed squashMergeAllowed } commits(last: 1) { nodes { commit { statusCheckRollup { state contexts(first: 50) { nodes { __typename ... on CheckRun { status conclusion } ... on StatusContext { state context } } } } } } }`

func searchQuery(qualifier, cursor string) string {
	after := ""
	if cursor != "" {
		after = fmt.Sprintf(`, after: "%s"`, escapeGraphQL(cursor))
	}
	return fmt.Sprintf(
		`query { search(query: "is:pr is:open %s", type: ISSUE, first: %d%s) { `+
			`pageInfo { hasNextPage endCursor } `+
			`nodes { ... on PullRequest { %s } } } }`,
		qualifier, pageSize, after, ghPRFields,
	)
}

func prQuery(owner, name string, number int) string {
	return fmt.Sprintf(
		`query { repository(owner: "%s", name: "%s") { pullRequest(number: %d) { %s } } }`,
		escapeGraphQL(owner), escapeGraphQL(name), number, ghPRFields,
	)
}

func commentsQuery(owner, name string, number, last int) string {
	return fmt.Sprintf(
		`query { repository(owner: "%s", name: "%s") { pullRequest(number: %d) { `+
			`comments(last: %d) { totalCount nodes { author { login } body createdAt } } } } }`,
		escapeGraphQL(owner), escapeGraphQL(name), number, last,
	)
}

func escapeGraphQL(s string) string { return forge.EscapeGraphQL(s) }

func splitProject(project string) (string, string) {
	project = strings.Trim(project, "/")
	if owner, name, found := strings.Cut(project, "/"); found {
		return owner, name
	}
	return "", project
}
