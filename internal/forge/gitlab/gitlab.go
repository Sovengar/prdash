// Package gitlab implementa el adapter del GitLab self-managed hablando con
// la CLI `glab`. El inbox usa GraphQL paginado por cursor y la API de Todos
// para las menciones; el REST vive bajo el subfolder configurable
// (`/git/api/v4/`).
package gitlab

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
const ForgeName = "gitlab"

// pageSize es el número de resultados por página.
const pageSize = 50

// Adapter implementa forge.Adapter sobre la CLI `glab`.
type Adapter struct {
	host    string
	apiBase string
	runner  *tool.Runner
}

// Aseguramos en compilación que el adapter cumple el contrato.
var _ forge.Adapter = (*Adapter)(nil)

// New construye el adapter para un host, un binario de `glab` y el subfolder
// del REST (p. ej. "/git/api/v4/").
func New(host, bin, apiBase string) *Adapter {
	if bin == "" {
		bin = "glab"
	}
	if host == "" {
		host = "gitlab.example.com"
	}
	return &Adapter{host: host, apiBase: normalizeBase(apiBase), runner: tool.New(bin, "GLAB_NO_PROMPT=1")}
}

// Forge devuelve el nombre del forge.
func (a *Adapter) Forge() string { return ForgeName }

// Host devuelve el host configurado.
func (a *Adapter) Host() string { return a.host }

// Auth comprueba la sesión de `glab`.
func (a *Adapter) Auth(ctx context.Context) model.AuthState {
	if _, err := a.runner.Run(ctx, "auth", "status"); err != nil {
		return model.AuthState{Forge: ForgeName, OK: false, Reason: err.Error()}
	}
	return model.AuthState{Forge: ForgeName, OK: true}
}

// List devuelve una página de la lista pedida.
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
		return forge.Page{}, []model.Warning{a.warn(q.Section, "unsupported", fmt.Errorf("lista no soportada: %s", q.Section))}
	}
}

// ItemState relee el estado de aprobación de un MR concreto.
func (a *Adapter) ItemState(ctx context.Context, ref model.RepoRef, number int) (model.Item, []model.Warning) {
	if ref.Project == "" {
		return model.Item{}, []model.Warning{a.warn("", "notfound", fmt.Errorf("referencia de repo vacía"))}
	}

	raw, err := a.runner.Run(ctx, "api", "graphql", "-f", "query="+glMRQuery(ref.Project, number))
	if err != nil {
		return model.Item{}, []model.Warning{a.warn("", tool.Kind(err), err)}
	}
	items, _, perr := parse.ParseGLGraphQL(raw)
	if perr != nil {
		return model.Item{}, []model.Warning{a.warn("", "parse", perr)}
	}
	if len(items) == 0 {
		return model.Item{}, []model.Warning{a.warn("", "notfound", fmt.Errorf("MR !%d no encontrado en %s", number, ref.Project))}
	}
	it := items[0]
	a.identity(&it)
	return it, nil
}

// Approve aprueba un MR con `glab mr approve`.
func (a *Adapter) Approve(ctx context.Context, ref model.RepoRef, number int) []model.Warning {
	return a.action(ctx, "mr", "approve", strconv.Itoa(number), "-R", ref.Project)
}

// Merge mergea un MR con `glab mr merge`.
func (a *Adapter) Merge(ctx context.Context, ref model.RepoRef, number int) []model.Warning {
	return a.action(ctx, "mr", "merge", strconv.Itoa(number), "-R", ref.Project, "--yes")
}

func (a *Adapter) action(ctx context.Context, args ...string) []model.Warning {
	if _, err := a.runner.Run(ctx, args...); err != nil {
		return []model.Warning{a.warn("", tool.Kind(err), err)}
	}
	return nil
}

// graphqlList ejecuta una query GraphQL paginada y etiqueta los ítems.
func (a *Adapter) graphqlList(ctx context.Context, q forge.Query, query string) (forge.Page, []model.Warning) {
	raw, err := a.runner.Run(ctx, "api", "graphql", "-f", "query="+query)
	if err != nil {
		// Respaldo REST solo para la primera página de los MRs propios.
		if q.Section == model.SectionAuthored && q.Cursor == "" {
			if p, ok := a.restAuthored(ctx); ok {
				return p, []model.Warning{a.warn(q.Section, tool.Kind(err), err)}
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

// todosList pagina la API de Todos del GitLab. El cursor es el número de
// página; se sigue mientras la página venga llena.
func (a *Adapter) todosList(ctx context.Context, q forge.Query) (forge.Page, []model.Warning) {
	pageNum := 1
	if n, err := strconv.Atoi(q.Cursor); err == nil && n > 0 {
		pageNum = n
	}
	raw, err := a.runner.Run(ctx, "api", a.restEndpoint("todos"),
		"-f", "action=mentioned",
		"-f", "per_page="+strconv.Itoa(pageSize),
		"-f", "page="+strconv.Itoa(pageNum))
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

// restAuthored consulta el respaldo REST (una sola página) de los MRs propios.
func (a *Adapter) restAuthored(ctx context.Context) (forge.Page, bool) {
	raw, err := a.runner.Run(ctx, "api", a.restEndpoint("merge_requests"),
		"-f", "scope=created_by_me", "-f", "state=opened", "-f", "per_page="+strconv.Itoa(pageSize))
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

// restEndpoint compone la ruta REST bajo el subfolder configurado. El
// self-managed expone el API en `/git/api/v4/`, así que el path absoluto
// empieza por ese prefijo.
func (a *Adapter) restEndpoint(resource string) string {
	resource = strings.TrimLeft(resource, "/")
	if a.apiBase == "" {
		return resource
	}
	return "/" + a.apiBase + "/" + resource
}

// normalizeBase deja el subfolder sin barras sobrantes: "/git/api/v4/" →
// "git/api/v4".
func normalizeBase(base string) string {
	return strings.Trim(base, "/")
}

// mrFields son los campos GraphQL de un merge request que el inbox consume.
const mrFields = `iid title webUrl state sourceBranch targetBranch approved approvalsLeft updatedAt author { username } project { fullPath name group { fullPath } }`

// glConn cierra una conexión GraphQL con paginación.
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

// glMRQuery compone la query GraphQL de un MR concreto.
func glMRQuery(fullPath string, iid int) string {
	return fmt.Sprintf(
		`query { project(fullPath: "%s") { mergeRequest(iid: %d) { %s } } }`,
		escapeGraphQL(fullPath), iid, mrFields,
	)
}

func afterArg(cursor string) string {
	if cursor == "" {
		return ""
	}
	return fmt.Sprintf(`, after: "%s"`, escapeGraphQL(cursor))
}

func escapeGraphQL(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	return strings.ReplaceAll(s, `"`, `\"`)
}
