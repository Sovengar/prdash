// Package gitlab implementa el adapter del GitLab self-managed hablando con
// la CLI `glab`. El inbox usa GraphQL paginado por cursor y la API de Todos
// para las menciones.
//
// `glab` resuelve por sí solo el host y su subfolder REST (p. ej. `/git/`), así
// que las rutas que se le pasan son relativas (nunca construimos la URL
// absoluta). El host se fija con `--hostname` y con `GITLAB_HOST` para que
// ninguna llamada caiga por defecto en gitlab.com.
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
	host   string
	runner *tool.Runner
}

// Aseguramos en compilación que el adapter cumple el contrato.
var _ forge.Adapter = (*Adapter)(nil)

// New construye el adapter para un host y un binario de `glab`.
func New(host, bin string) *Adapter {
	if bin == "" {
		bin = "glab"
	}
	if host == "" {
		host = "gitlab.example.com"
	}
	return &Adapter{
		host: host,
		// GITLAB_HOST fija el host por defecto de todas las llamadas (incluidas
		// las de `glab mr`, que no aceptan `--hostname`).
		runner: tool.New(bin, "GLAB_NO_PROMPT=1", "GITLAB_HOST="+host),
	}
}

// Forge devuelve el nombre del forge.
func (a *Adapter) Forge() string { return ForgeName }

// Host devuelve el host configurado.
func (a *Adapter) Host() string { return a.host }

// Auth comprueba la sesión de `glab` para este host (no de todas las
// instancias configuradas).
func (a *Adapter) Auth(ctx context.Context) model.AuthState {
	if _, err := a.runner.Run(ctx, a.authArgs()...); err != nil {
		return model.AuthState{Forge: ForgeName, OK: false, Reason: err.Error()}
	}
	return model.AuthState{Forge: ForgeName, OK: true}
}

// authArgs compone `glab auth status` acotado al host.
func (a *Adapter) authArgs() []string {
	return []string{"auth", "status", "--hostname", a.host}
}

// graphqlArgs compone una llamada GraphQL con el host fijado.
func (a *Adapter) graphqlArgs(query string) []string {
	return []string{"api", "--hostname", a.host, "graphql", "-f", "query=" + query}
}

// getArgs compone una llamada REST por GET con el host fijado. Sin `-X GET`,
// pasar campos convertiría la petición en POST.
func (a *Adapter) getArgs(endpoint string, fields ...string) []string {
	args := []string{"api", "--hostname", a.host, "-X", "GET", endpoint}
	for _, f := range fields {
		args = append(args, "-f", f)
	}
	return args
}

// mrArgs compone una acción `glab mr` sobre un proyecto.
func (a *Adapter) mrArgs(sub string, number int, project string, extra ...string) []string {
	args := []string{"mr", sub, strconv.Itoa(number), "-R", project}
	return append(args, extra...)
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

	raw, err := a.runner.Run(ctx, a.graphqlArgs(glMRQuery(ref.Project, number))...)
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
	return a.action(ctx, a.mrArgs("approve", number, ref.Project)...)
}

// Merge mergea un MR con `glab mr merge`.
func (a *Adapter) Merge(ctx context.Context, ref model.RepoRef, number int) []model.Warning {
	return a.action(ctx, a.mrArgs("merge", number, ref.Project, "--yes")...)
}

func (a *Adapter) action(ctx context.Context, args ...string) []model.Warning {
	if _, err := a.runner.Run(ctx, args...); err != nil {
		return []model.Warning{a.warn("", tool.Kind(err), err)}
	}
	return nil
}

// graphqlList ejecuta una query GraphQL paginada y etiqueta los ítems.
func (a *Adapter) graphqlList(ctx context.Context, q forge.Query, query string) (forge.Page, []model.Warning) {
	raw, err := a.runner.Run(ctx, a.graphqlArgs(query)...)
	if err != nil {
		// Respaldo REST solo para la primera página de los MRs propios.
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

// todosList pagina la API de Todos del GitLab. El cursor es el número de
// página; se sigue mientras la página venga llena.
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

// restAuthored consulta el respaldo REST (una sola página) de los MRs propios.
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

// degraded avisa de datos parciales procedentes del respaldo REST.
func (a *Adapter) degraded(section model.Section, err error) model.Warning {
	return model.Warning{
		Forge:   ForgeName,
		Section: section,
		Kind:    "degraded",
		Msg:     "GraphQL no disponible; datos parciales vía REST: " + tool.FirstLine(err.Error()),
	}
}

// restEndpoint devuelve el recurso REST relativo. `glab api` ya resuelve el
// host y su subfolder (p. ej. `/git/api/v4/`) contra su base configurada: pasar
// la ruta absoluta da 404.
func restEndpoint(resource string) string {
	return strings.TrimLeft(resource, "/")
}

// mrFields son los campos GraphQL de un merge request que el inbox consume. La
// instancia CE no expone `approvalsLeft`, así que solo se pide `approved`.
const mrFields = `iid title webUrl state sourceBranch targetBranch approved updatedAt author { username } project { fullPath name group { fullPath } }`

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

// escapeGraphQL escapa un valor para incrustarlo como literal de GraphQL.
func escapeGraphQL(s string) string { return forge.EscapeGraphQL(s) }
