// Package github implementa el adapter del forge GitHub hablando con la CLI
// `gh` por subproceso. El inbox rico (reviewDecision + checks) se consulta por
// GraphQL paginando por cursor; la búsqueda REST queda como respaldo.
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

// ForgeName es el identificador del forge.
const ForgeName = "github"

// pageSize es el número de resultados por página que se pide a GraphQL.
const pageSize = 50

// commentFetch es cuántos nodos de comentarios se piden por consulta. Se pide
// más de los que la ficha muestra (forge.CommentLimit) para que un PR con
// boilerplate de bots no se quede corto: lo que se devuelve ya viene filtrado y
// recortado al tope, así que el margen no cuesta nada al usuario.
const commentFetch = 3 * forge.CommentLimit

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
	out, err := a.runner.Run(ctx, "auth", "status", "--hostname", a.host)
	if err != nil {
		return model.AuthState{Forge: ForgeName, OK: false, Reason: err.Error()}
	}
	return model.AuthState{Forge: ForgeName, OK: true, Login: loginFromAuthStatus(out)}
}

// loginFromAuthStatus extrae el login de la sesión de la salida de
// `gh auth status`, que ya se pedía para comprobar la sesión y se descartaba.
// Así el viewer se conoce sin ninguna llamada extra.
func loginFromAuthStatus(out string) string {
	m := loginRe.FindStringSubmatch(out)
	if m == nil {
		return ""
	}
	return m[1]
}

// loginRe captura el login de "Logged in to <host> account <login> (<path>)".
// El grupo exige un espacio tras "account" para no confundirlo con la línea
// "Active account: true", que también contiene la palabra.
var loginRe = regexp.MustCompile(`account ([^(\s]+)`)

// List devuelve una página de la lista pedida.
func (a *Adapter) List(ctx context.Context, q forge.Query) (forge.Page, []model.Warning) {
	qualifier, ok := qualifierFor(q)
	if !ok {
		return forge.Page{}, []model.Warning{a.warn(q.Section, "unsupported", fmt.Errorf("unsupported list: %s", q.Section))}
	}

	raw, err := a.runner.Run(ctx, "api", "graphql", "-f", "query="+searchQuery(qualifier, q.Cursor))
	if err != nil {
		// Respaldo REST solo para la primera página de los PRs propios.
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

// ItemState relee el estado rich de un PR concreto (decisión de review y
// checks) para refrescarlo tras una acción o un cambio en el forge.
func (a *Adapter) ItemState(ctx context.Context, ref model.RepoRef, number int) (model.Item, []model.Warning) {
	owner, name := splitProject(ref.Project)
	if owner == "" || name == "" {
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
		return model.Item{}, []model.Warning{a.warn("", "notfound", fmt.Errorf("PR #%d no encontrado en %s", number, ref.Project))}
	}

	it := items[0]
	a.identity(&it)
	if checks, warns := a.checks(ctx, ref.Project, number); warns == nil {
		it.Checks = checks
	}
	return it, nil
}

// Comments devuelve los últimos comentarios de la conversación del PR, del más
// antiguo de esos al más reciente. La conversación se pide aparte de la búsqueda
// porque los comentarios son un detalle del ítem seleccionado y no un dato del
// inbox: pedirlos con cada lista multiplicaría las consultas por el número de PRs
// que nadie está mirando.
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
	// Se recorta a los últimos CommentLimit y se conserva el total que dice el
	// forge: el recorte es de lo que se pinta, no de lo que existe.
	return forge.CommentPage{Comments: comments, Total: total}.KeepLast(), nil
}

// Approve aprueba un PR con `gh pr review --approve`.
func (a *Adapter) Approve(ctx context.Context, ref model.RepoRef, number int) []model.Warning {
	return a.action(ctx, "pr", "review", strconv.Itoa(number), "--repo", ref.Project, "--approve")
}

// Merge mergea un PR con `gh pr merge` y el flag de estrategia que toque. Los
// tres modos tienen flag propio en gh, así que siempre se pasa uno: sin
// estrategia, gh abre un prompt interactivo que en un subproceso no
// interactivo se queda colgado.
func (a *Adapter) Merge(ctx context.Context, ref model.RepoRef, number int, mode forge.MergeMode) []model.Warning {
	flag, ok := ghMergeFlag(mode)
	if !ok {
		return []model.Warning{a.warn("", "unsupported", forge.ErrUnknownMergeMode(mode))}
	}
	return a.action(ctx, "pr", "merge", strconv.Itoa(number), "--repo", ref.Project, flag)
}

// ghMergeFlag traduce el modo al flag de `gh pr merge`.
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

func (a *Adapter) action(ctx context.Context, args ...string) []model.Warning {
	if _, err := a.runner.Run(ctx, args...); err != nil {
		return []model.Warning{a.warn("", tool.Kind(err), err)}
	}
	return nil
}

// checks consulta el estado de los checks del PR. `gh pr checks` sale con
// exit 8 (pendiente) o 1 (fallo) trayendo el JSON igualmente: si la salida es
// parseable cuenta como estado válido, no como error.
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

// degraded avisa de datos parciales procedentes del respaldo REST: no trae
// ramas ni decisión de review.
func (a *Adapter) degraded(section model.Section, err error) model.Warning {
	return model.Warning{
		Forge:   ForgeName,
		Section: section,
		Kind:    "degraded",
		Msg:     "GraphQL unavailable; partial data via REST (no branches or checks): " + tool.FirstLine(err.Error()),
	}
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

// ghPRFields son los campos GraphQL de un pull request que el inbox consume.
// `statusCheckRollup.contexts.nodes` es la unión StatusCheckRollupContext
// (CheckRun | StatusContext): cada rama pide sus campos reales.
//
// `additions`/`deletions`/`changedFiles` son escalares que la búsqueda ya
// pagina, así que el diffstat no cuesta ninguna llamada extra.
const ghPRFields = `number title url state isDraft reviewDecision updatedAt headRefName baseRefName ` +
	`additions deletions changedFiles ` +
	`author { login } repository { nameWithOwner name owner { login } } ` +
	`commits(last: 1) { nodes { commit { statusCheckRollup { state contexts(first: 50) { nodes { __typename ... on CheckRun { status conclusion } ... on StatusContext { state context } } } } } } }`

// searchQuery compone la query GraphQL de búsqueda, con paginación por cursor y
// los campos ricos que el inbox necesita. `search.nodes` es la unión
// SearchResultItem, así que los campos del PR van en un fragmento PullRequest.
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

// prQuery compone la query GraphQL de un PR concreto.
func prQuery(owner, name string, number int) string {
	return fmt.Sprintf(
		`query { repository(owner: "%s", name: "%s") { pullRequest(number: %d) { %s } } }`,
		escapeGraphQL(owner), escapeGraphQL(name), number, ghPRFields,
	)
}

// commentsQuery compone la query de la conversación de un PR concreto.
//
// Se pide `last` y no `first`: la ficha enseña el final de la conversación, que es
// donde está lo último que se dijo del PR y dónde está el estado actual de la
// discusión. `totalCount` viene en la misma conexión para no gastar una segunda
// consulta en saber que hay más.
//
// `last` devuelve los nodos en orden cronológico, no invertido: el más antiguo de
// los últimos va primero. Así se leen hacia abajo como se escribieron, que es como
// se sigue una discusión.
func commentsQuery(owner, name string, number, last int) string {
	return fmt.Sprintf(
		`query { repository(owner: "%s", name: "%s") { pullRequest(number: %d) { `+
			`comments(last: %d) { totalCount nodes { author { login } body createdAt } } } } }`,
		escapeGraphQL(owner), escapeGraphQL(name), number, last,
	)
}

// escapeGraphQL escapa un valor para incrustarlo como literal de GraphQL.
func escapeGraphQL(s string) string { return forge.EscapeGraphQL(s) }

// splitProject separa "owner/repo" en sus dos partes.
func splitProject(project string) (string, string) {
	project = strings.Trim(project, "/")
	if i := strings.Index(project, "/"); i >= 0 {
		return project[:i], project[i+1:]
	}
	return "", project
}
