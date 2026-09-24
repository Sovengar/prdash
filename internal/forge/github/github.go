// Package github implementa el adapter del forge GitHub hablando con la CLI
// `gh` por subproceso. El inbox rico (reviewDecision + checks) se consulta por
// GraphQL paginando por cursor; la búsqueda REST queda como respaldo.
package github

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"prdash/internal/forge"
	"prdash/internal/forge/model"
	"prdash/internal/forge/parse"
	"prdash/internal/forge/tool"
)

// ForgeName es el identificador del forge.
const ForgeName = "github"

// pageSize es el número de resultados por página que se pide a GraphQL.
const pageSize = 50

// Adapter implementa forge.Adapter sobre la CLI `gh`.
type Adapter struct {
	host   string
	runner *tool.Runner
}

// Aseguramos en compilación que el adapter cumple el contrato.
var _ forge.Adapter = (*Adapter)(nil)

// New construye el adapter para un host y un binario de `gh`.
func New(host, bin string) *Adapter {
	if bin == "" {
		bin = "gh"
	}
	if host == "" {
		host = "github.com"
	}
	return &Adapter{host: host, runner: tool.New(bin, "GH_PROMPT_DISABLED=1")}
}

// Forge devuelve el nombre del forge.
func (a *Adapter) Forge() string { return ForgeName }

// Host devuelve el host configurado.
func (a *Adapter) Host() string { return a.host }

// Auth comprueba la sesión de `gh` contra el host.
func (a *Adapter) Auth(ctx context.Context) model.AuthState {
	if _, err := a.runner.Run(ctx, "auth", "status", "--hostname", a.host); err != nil {
		return model.AuthState{Forge: ForgeName, OK: false, Reason: err.Error()}
	}
	return model.AuthState{Forge: ForgeName, OK: true}
}

// List devuelve una página de la lista pedida.
func (a *Adapter) List(ctx context.Context, q forge.Query) (forge.Page, []model.Warning) {
	qualifier, ok := qualifierFor(q)
	if !ok {
		return forge.Page{}, []model.Warning{a.warn(q.Section, "unsupported", fmt.Errorf("lista no soportada: %s", q.Section))}
	}

	raw, err := a.runner.Run(ctx, "api", "graphql", "-f", "query="+searchQuery(qualifier, q.Cursor))
	if err != nil {
		// Respaldo REST solo para la primera página de los PRs propios.
		if q.Section == model.SectionAuthored && q.Cursor == "" {
			if p, ok := a.restAuthored(ctx); ok {
				return p, []model.Warning{a.warn(q.Section, tool.Kind(err), err)}
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

// ItemState relee el estado rich de un PR concreto (decisión de review y
// checks) para refrescarlo tras una acción o un cambio en el forge.
func (a *Adapter) ItemState(ctx context.Context, ref model.RepoRef, number int) (model.Item, []model.Warning) {
	owner, name := splitProject(ref.Project)
	if owner == "" || name == "" {
		return model.Item{}, []model.Warning{a.warn("", "notfound", fmt.Errorf("referencia de repo inválida: %q", ref.Project))}
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
		return model.Item{}, []model.Warning{a.warn("", "notfound", fmt.Errorf("PR #%d no encontrado en %s", number, ref.Project))}
	}

	it := items[0]
	a.identity(&it)
	if checks, warns := a.checks(ctx, ref.Project, number); warns == nil {
		it.Checks = checks
	}
	return it, nil
}

// Approve aprueba un PR con `gh pr review --approve`.
func (a *Adapter) Approve(ctx context.Context, ref model.RepoRef, number int) []model.Warning {
	return a.action(ctx, "pr", "review", strconv.Itoa(number), "--repo", ref.Project, "--approve")
}

// Merge mergea un PR con `gh pr merge --squash`.
func (a *Adapter) Merge(ctx context.Context, ref model.RepoRef, number int) []model.Warning {
	return a.action(ctx, "pr", "merge", strconv.Itoa(number), "--repo", ref.Project, "--squash")
}

func (a *Adapter) action(ctx context.Context, args ...string) []model.Warning {
	if _, err := a.runner.Run(ctx, args...); err != nil {
		return []model.Warning{a.warn("", tool.Kind(err), err)}
	}
	return nil
}

// checks consulta el estado de los checks del PR. Un PR sin checks hace que
// `gh` falle: se trata como "sin checks", no como error fatal.
func (a *Adapter) checks(ctx context.Context, project string, number int) (model.Checks, []model.Warning) {
	raw, err := a.runner.Run(ctx, "pr", "checks", strconv.Itoa(number),
		"--repo", project, "--json", "name,state,bucket")
	if err != nil {
		return model.Checks{}, []model.Warning{a.warn("", tool.Kind(err), err)}
	}
	c, perr := parse.ParseGHChecks(raw)
	if perr != nil {
		return model.Checks{}, []model.Warning{a.warn("", "parse", perr)}
	}
	return c, nil
}

// restAuthored consulta el respaldo REST (una sola página) de los PRs propios.
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

// stamp fija la sección, el tipo de review y la identidad de forge/host.
func (a *Adapter) stamp(items []model.Item, q forge.Query) {
	for i := range items {
		items[i].Section = q.Section
		if q.Section == model.SectionReview {
			items[i].ReviewKind = q.ReviewKind
		}
		a.identity(&items[i])
	}
}

// identity normaliza forge y host del ítem.
func (a *Adapter) identity(it *model.Item) {
	it.Forge = ForgeName
	it.Host = a.host
	it.Ref.Forge = ForgeName
	it.Ref.Host = a.host
}

func (a *Adapter) warn(section model.Section, kind string, err error) model.Warning {
	return model.Warning{Forge: ForgeName, Section: section, Kind: kind, Msg: err.Error()}
}

// qualifierFor traduce una lista del inbox al qualifier de búsqueda de GitHub.
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

// searchQuery compone la query GraphQL de búsqueda, con paginación por cursor y
// los campos ricos que el inbox necesita.
func searchQuery(qualifier, cursor string) string {
	after := ""
	if cursor != "" {
		after = fmt.Sprintf(`, after: "%s"`, escapeGraphQL(cursor))
	}
	return fmt.Sprintf(
		`query { search(query: "is:pr is:open %s", type: ISSUE, first: %d%s) { `+
			`pageInfo { hasNextPage endCursor } `+
			`nodes { number title url state isDraft reviewDecision updatedAt headRefName baseRefName `+
			`author { login } repository { nameWithOwner name owner { login } } `+
			`commits(last: 1) { nodes { commit { statusCheckRollup { state contexts(first: 50) { nodes { __typename status conclusion state } } } } } } `+
			`} } }`,
		qualifier, pageSize, after,
	)
}

// prQuery compone la query GraphQL de un PR concreto.
func prQuery(owner, name string, number int) string {
	return fmt.Sprintf(
		`query { repository(owner: "%s", name: "%s") { pullRequest(number: %d) { `+
			`number title url state isDraft reviewDecision updatedAt headRefName baseRefName `+
			`author { login } repository { nameWithOwner name owner { login } } } } }`,
		escapeGraphQL(owner), escapeGraphQL(name), number,
	)
}

// escapeGraphQL escapa comillas y barras para incrustar un valor como literal
// de GraphQL.
func escapeGraphQL(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	return strings.ReplaceAll(s, `"`, `\"`)
}

// splitProject separa "owner/repo" en sus dos partes.
func splitProject(project string) (string, string) {
	project = strings.Trim(project, "/")
	if i := strings.Index(project, "/"); i >= 0 {
		return project[:i], project[i+1:]
	}
	return "", project
}
