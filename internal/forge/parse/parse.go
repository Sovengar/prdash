// Package parse traduce la salida JSON de `gh`/`glab` (GraphQL, REST y la API
// de Todos) a los tipos normalizados de model.
//
// Es una capa pura: no ejecuta subprocesos ni toca red o disco. Cada forma de
// salida tiene su propia función y ninguna lanza panic: ante una entrada
// inesperada devuelven los ítems que se pudieron leer más un error tipado.
package parse

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"prdash/internal/forge/model"
)

// ForgeGitHub y ForgeGitLab son los nombres de forge que el parseo estampa en
// los ítems cuando la salida no trae el dato (el adapter los ajusta al host
// real configurado).
const (
	ForgeGitHub = "github"
	ForgeGitLab = "gitlab"
)

// Error describe un fallo de parseo de la salida de una herramienta.
type Error struct {
	Tool string // "gh-graphql" | "gh-search" | "gh-checks" | "gl-graphql"…
	Msg  string
	Err  error
}

// PageInfo describe la paginación de una respuesta: Next es el cursor (GraphQL)
// o el número de página siguiente (REST) y More indica si quedan páginas.
type PageInfo struct {
	Next string
	More bool
}

// Error implementa el contrato de error.
func (e *Error) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("parse %s: %s: %v", e.Tool, e.Msg, e.Err)
	}
	return fmt.Sprintf("parse %s: %s", e.Tool, e.Msg)
}

// Unwrap permite inspeccionar la causa subyacente.
func (e *Error) Unwrap() error { return e.Err }

// flexInt acepta un entero serializado como número JSON o como string: GraphQL
// expone los `ID!` (p. ej. `iid`) como string y REST los devuelve como número.
// Un valor nulo, ausente o no numérico se lee como 0; quien lo consuma decide
// si descarta el ítem (no se inventa un número).
type flexInt int

// UnmarshalJSON implementa la tolerancia de tipo sin fallar el parseo entero.
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

// ---- GitHub: búsqueda GraphQL ----

// ghPRNode es el nodo de pull request que devuelven las queries GraphQL de
// búsqueda y de pullRequest individual. Comparten forma a propósito, de modo
// que una sola función de parseo sirve para ambos.
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
	// Additions va como puntero a propósito: `additions` es `Int!` en el schema,
	// así que si el campo viene es que la query lo pidió. Ausente = la respuesta
	// no lo trajo (respaldo REST u otra forma de salida) y el diffstat queda
	// como desconocido en vez de como un cambio de cero líneas.
	Additions    *int `json:"additions"`
	Deletions    int  `json:"deletions"`
	ChangedFiles int  `json:"changedFiles"`
	Author       struct {
		Login string `json:"login"`
	} `json:"author"`
	Repository struct {
		NameWithOwner string `json:"nameWithOwner"`
		Name          string `json:"name"`
		Owner         struct {
			Login string `json:"login"`
		} `json:"owner"`
	} `json:"repository"`
	Commits struct {
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

// ParseGHGraphQLSearch interpreta la respuesta de una query GraphQL de PRs
// (búsqueda con `search.nodes` o un `repository.pullRequest` individual).
// Devuelve los ítems leídos, la paginación de la búsqueda y un error tipado si
// la entrada no es válida o la API reporta errores.
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
	it.ReviewDecision = n.ReviewDecision
	it.Checks = checksFromRollup(n)
	it.Diff = diffFromGHNode(n)
	it.UpdatedAt = parseTime(n.UpdatedAt)
	return it
}

// diffFromGHNode lee el diffstat de un PR. GitHub lo da ya agregado en tres
// escalares, así que no hay nada que sumar.
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

// checksFromRollup agrega el statusCheckRollup del último commit en un
// resumen de checks.
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

// ---- GitHub: REST search/issues (fallback de authored) ----

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

// ParseGHAuthored interpreta la salida de la búsqueda REST de issues/PRs
// (`GET search/issues`), usada como respaldo cuando GraphQL no está
// disponible. No trae reviewDecision, checks ni diffstat: eso queda como
// desconocido.
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
		if n.PullRequest != nil {
			it.SourceBranch = n.PullRequest.Head.Ref
			it.TargetBranch = n.PullRequest.Base.Ref
		}
		it.UpdatedAt = parseTime(n.UpdatedAt)
		items = append(items, it)
	}
	return items, nil
}

// ---- GitHub: `gh pr checks --json` ----

type ghCheck struct {
	Name   string `json:"name"`
	State  string `json:"state"`
	Bucket string `json:"bucket"`
}

// ParseGHChecks interpreta la salida de `gh pr checks --json name,state,bucket`
// y resume el estado de los checks. El bucket que normaliza `gh` manda; si
// falta, se cae al estado crudo.
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
			// cuenta como correcto
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

// ---- GitLab: GraphQL (currentUser / project) ----

type glMR struct {
	IID          flexInt `json:"iid"`
	Title        string  `json:"title"`
	WebURL       string  `json:"webUrl"`
	State        string  `json:"state"`
	SourceBranch string  `json:"sourceBranch"`
	TargetBranch string  `json:"targetBranch"`
	Approved     bool    `json:"approved"`
	UpdatedAt    string  `json:"updatedAt"`
	// DiffStats es una entrada por fichero cambiado, no un agregado, y va como
	// puntero a slice para poder distinguir las dos cosas que un `[]` vacío
	// significaría: ausente (la query no lo pidió, p. ej. la API de Todos) y
	// presente-pero-vacío (el MR no toca ningún fichero).
	DiffStats *[]struct {
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

// ParseGLGraphQL interpreta la respuesta GraphQL del GitLab: las listas del
// `currentUser` (authored, reviewRequested, assigned) y, si está, un
// `project.mergeRequest` individual. Cada ítem sale con su sección y su
// ReviewKind ya resueltos; la paginación corresponde a la primera conexión
// presente (cada consulta pide una sola).
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
	it.ReviewDecision = glReviewDecision(mr)
	it.Diff = diffFromGLMR(mr)
	it.UpdatedAt = parseTime(mr.UpdatedAt)
	return it
}

// diffFromGLMR suma el diffstat de un MR. GitLab entrega `diffStats` como una
// lista con una entrada por fichero cambiado, no como un agregado: hay que
// sumarla entera. Cog solo la primera o la última entrada daría el total de un
// fichero cualquiera en lugar del MR.
//
// El recuento de ficheros sale de la longitud de la lista, y por eso puede
// quedarse corto: GitLab colapsa los diffs que superan su límite de tamaño y de
// filas, así que en un MR enorme las cifras son un mínimo, no un exacto.
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

// glReviewDecision traduce la aprobación del MR a una decisión homóloga a la de
// GitHub. La instancia CE no expone el recuento de aprobaciones, así que un MR
// no aprobado se reporta como desconocido en vez de inventar "review required".
func glReviewDecision(mr glMR) string {
	if mr.Approved {
		return "APPROVED"
	}
	return ""
}

// ---- GitLab: REST merge_requests (BasicMergeRequest) ----

type glBasicMR struct {
	IID          flexInt `json:"iid"`
	Title        string  `json:"title"`
	WebURL       string  `json:"web_url"`
	State        string  `json:"state"`
	SourceBranch string  `json:"source_branch"`
	TargetBranch string  `json:"target_branch"`
	UpdatedAt    string  `json:"updated_at"`
	Author       struct {
		Username string `json:"username"`
	} `json:"author"`
	References struct {
		Full  string `json:"full"`  // "grupo/proy!12"
		Short string `json:"short"` // "!12"
	} `json:"references"`
}

// ParseGLMRList interpreta una lista de merge requests del GitLab en su forma
// REST (`glab mr list -F json` o `GET /merge_requests`). El endpoint REST de
// merge requests no expone additions ni deletions (gitlab-org/gitlab#464260), así
// que el diffstat queda como desconocido.
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
		it.UpdatedAt = parseTime(mr.UpdatedAt)
		items = append(items, it)
	}
	return items, nil
}

// ---- GitLab: API de Todos ----

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

// ParseGLTodos interpreta la API de Todos del GitLab y devuelve como ítems
// solo las menciones sobre merge requests, que son las que alimentan la
// sección de menciones. El segundo valor es el número de todos de la página
// (antes de filtrar), que el adapter usa para saber si quedan páginas.
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

// ---- utilidades ----

func parseTime(s string) time.Time {
	if s == "" {
		return time.Time{}
	}
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t
	}
	return time.Time{}
}

// splitRepoURL extrae (owner, repo) de "https://api.github.com/repos/owner/repo".
func splitRepoURL(u string) (string, string) {
	const marker = "/repos/"
	if i := strings.Index(u, marker); i >= 0 {
		return splitProject(u[i+len(marker):])
	}
	return "", ""
}

// joinProject compone "owner/repo" omitiendo el separador si falta una parte.
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

// splitProject separa "grupo/sub/proy" en ("grupo/sub", "proy").
func splitProject(path string) (string, string) {
	path = strings.Trim(path, "/")
	if i := strings.LastIndex(path, "/"); i >= 0 {
		return path[:i], path[i+1:]
	}
	return "", path
}

// projectFromRef extrae "grupo/proy" de una referencia "grupo/proy!12".
func projectFromRef(ref string) string {
	if i := strings.LastIndex(ref, "!"); i >= 0 {
		return ref[:i]
	}
	return ref
}
