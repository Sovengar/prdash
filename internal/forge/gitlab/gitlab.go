// Package gitlab implementa el adapter del GitLab self-managed hablando con
// la CLI `glab`. El inbox usa GraphQL (currentUser) y la API de Todos para las
// menciones; el REST vive bajo el subfolder configurable (`/git/api/v4/`).
package gitlab

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"prdash/internal/forge"
	"prdash/internal/forge/model"
	"prdash/internal/forge/parse"
)

// ForgeName es el identificador del forge.
const ForgeName = "gitlab"

// defaultTimeout es el límite por invocación de `glab`.
const defaultTimeout = 30 * time.Second

// Adapter implementa forge.Adapter sobre la CLI `glab`.
type Adapter struct {
	host    string
	bin     string
	apiBase string
	timeout time.Duration
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
	return &Adapter{host: host, bin: bin, apiBase: normalizeBase(apiBase), timeout: defaultTimeout}
}

// Forge devuelve el nombre del forge.
func (a *Adapter) Forge() string { return ForgeName }

// Host devuelve el host configurado.
func (a *Adapter) Host() string { return a.host }

// Auth comprueba la sesión de `glab`.
func (a *Adapter) Auth(ctx context.Context) model.AuthState {
	_, err := a.run(ctx, "auth", "status")
	if err != nil {
		return model.AuthState{Forge: ForgeName, OK: false, Reason: err.Error()}
	}
	return model.AuthState{Forge: ForgeName, OK: true}
}

// Authored lista los MRs creados por el usuario.
func (a *Adapter) Authored(ctx context.Context) ([]model.Item, []model.Warning) {
	items, warnings := a.graphql(ctx, authoredQuery(), model.SectionAuthored)
	if len(items) > 0 || len(warnings) == 0 {
		return items, warnings
	}
	// Respaldo REST si GraphQL no devolvió nada.
	if items, ok := a.restAuthored(ctx); ok {
		return items, warnings
	}
	return items, warnings
}

// ReviewRequested lista los MRs con review pedido o asignados al usuario.
func (a *Adapter) ReviewRequested(ctx context.Context) ([]model.Item, []model.Warning) {
	return a.graphql(ctx, reviewQuery(), model.SectionReview)
}

// Mentions lista las menciones del usuario vía la API de Todos.
func (a *Adapter) Mentions(ctx context.Context) ([]model.Item, []model.Warning) {
	raw, err := a.run(ctx, "api", a.restEndpoint("todos"), "-f", "action=mentioned", "-f", "per_page=50")
	if err != nil {
		return nil, []model.Warning{a.warn(model.SectionMentions, classify(err), err)}
	}
	items, perr := parse.ParseGLTodos(raw)
	if perr != nil {
		return nil, []model.Warning{a.warn(model.SectionMentions, "parse", perr)}
	}
	a.stamp(items)
	return items, nil
}

// ItemState relee el estado de un MR concreto. Se completa en la etapa de
// detalle y acciones.
func (a *Adapter) ItemState(_ context.Context, _ model.RepoRef, _ int) (model.Item, []model.Warning) {
	return model.Item{}, unsupportedWarnings()
}

// Approve aprueba un MR. Se completa en la etapa de acciones.
func (a *Adapter) Approve(_ context.Context, _ model.RepoRef, _ int) []model.Warning {
	return unsupportedWarnings()
}

// Merge mergea un MR. Se completa en la etapa de acciones.
func (a *Adapter) Merge(_ context.Context, _ model.RepoRef, _ int) []model.Warning {
	return unsupportedWarnings()
}

func unsupportedWarnings() []model.Warning {
	return []model.Warning{{
		Forge: ForgeName,
		Kind:  "unsupported",
		Msg:   "acción disponible en una etapa posterior",
	}}
}

// graphql ejecuta una query GraphQL y etiqueta los ítems con su sección.
func (a *Adapter) graphql(ctx context.Context, query string, section model.Section) ([]model.Item, []model.Warning) {
	raw, err := a.run(ctx, "api", "graphql", "-f", "query="+query)
	if err != nil {
		return nil, []model.Warning{a.warn(section, classify(err), err)}
	}
	items, perr := parse.ParseGLGraphQL(raw)
	if perr != nil {
		return nil, []model.Warning{a.warn(section, "parse", perr)}
	}
	a.stamp(items)
	return items, nil
}

// restAuthored consulta el respaldo REST de los MRs propios (scope
// created_by_me).
func (a *Adapter) restAuthored(ctx context.Context) ([]model.Item, bool) {
	raw, err := a.run(ctx, "api", a.restEndpoint("merge_requests"),
		"-f", "scope=created_by_me", "-f", "state=opened", "-f", "per_page=50")
	if err != nil {
		return nil, false
	}
	items, perr := parse.ParseGLMRList(raw)
	if perr != nil {
		return nil, false
	}
	a.stamp(items)
	return items, true
}

// stamp fija la identidad de forge/host en los ítems parseados.
func (a *Adapter) stamp(items []model.Item) {
	for i := range items {
		items[i].Forge = ForgeName
		items[i].Host = a.host
		items[i].Ref.Forge = ForgeName
		items[i].Ref.Host = a.host
	}
}

func (a *Adapter) warn(section model.Section, kind string, err error) model.Warning {
	return model.Warning{Forge: ForgeName, Section: section, Kind: kind, Msg: err.Error()}
}

// run ejecuta `glab` con timeout y entorno no interactivo, devolviendo stdout.
func (a *Adapter) run(ctx context.Context, args ...string) (string, error) {
	cctx, cancel := context.WithTimeout(ctx, a.timeout)
	defer cancel()

	cmd := exec.CommandContext(cctx, a.bin, args...)
	cmd.Env = toolEnv()
	var out, errb bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errb
	if err := cmd.Run(); err != nil {
		msg := firstLine(strings.TrimSpace(errb.String()))
		if msg == "" {
			msg = err.Error()
		}
		return out.String(), fmt.Errorf("%s %s: %s", a.bin, strings.Join(args, " "), msg)
	}
	return out.String(), nil
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

func authoredQuery() string {
	return fmt.Sprintf(
		`query { currentUser { authoredMergeRequests(state: opened, first: 50) { nodes { %s } } } }`,
		mrFields,
	)
}

func reviewQuery() string {
	return fmt.Sprintf(
		`query { currentUser { reviewRequestedMergeRequests(state: opened, first: 50) { nodes { %s } } assignedMergeRequests(state: opened, first: 50) { nodes { %s } } } }`,
		mrFields, mrFields,
	)
}

// toolEnv devuelve el entorno de los subprocesos: locale inglés y modo no
// interactivo.
func toolEnv() []string {
	env := os.Environ()
	out := env[:0]
	for _, kv := range env {
		switch {
		case strings.HasPrefix(kv, "LC_ALL="),
			strings.HasPrefix(kv, "LANG="),
			strings.HasPrefix(kv, "LANGUAGE="),
			strings.HasPrefix(kv, "LC_MESSAGES="):
			continue
		}
		out = append(out, kv)
	}
	return append(out,
		"LC_ALL=C",
		"GIT_TERMINAL_PROMPT=0",
		"GLAB_NO_PROMPT=1",
		"NO_COLOR=1",
	)
}

// classify etiqueta el warning según el error: timeout, auth (401) o red.
func classify(err error) string {
	switch {
	case strings.Contains(err.Error(), "context deadline exceeded"):
		return "timeout"
	case strings.Contains(err.Error(), "401"), strings.Contains(err.Error(), "auth"):
		return "auth"
	default:
		return "network"
	}
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}
