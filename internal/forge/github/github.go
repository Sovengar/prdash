// Package github implementa el adapter del forge GitHub hablando con la CLI
// `gh` por subproceso. El inbox rico (reviewDecision + checks) se consulta por
// GraphQL; la búsqueda REST queda como respaldo.
package github

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
const ForgeName = "github"

// defaultTimeout es el límite por invocación de `gh`.
const defaultTimeout = 30 * time.Second

// Adapter implementa forge.Adapter sobre la CLI `gh`.
type Adapter struct {
	host    string
	bin     string
	timeout time.Duration
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
	return &Adapter{host: host, bin: bin, timeout: defaultTimeout}
}

// Forge devuelve el nombre del forge.
func (a *Adapter) Forge() string { return ForgeName }

// Host devuelve el host configurado.
func (a *Adapter) Host() string { return a.host }

// Auth comprueba la sesión de `gh` contra el host.
func (a *Adapter) Auth(ctx context.Context) model.AuthState {
	_, err := a.run(ctx, "auth", "status", "--hostname", a.host)
	if err != nil {
		return model.AuthState{Forge: ForgeName, OK: false, Reason: err.Error()}
	}
	return model.AuthState{Forge: ForgeName, OK: true}
}

// Authored lista los PRs creados por el usuario.
func (a *Adapter) Authored(ctx context.Context) ([]model.Item, []model.Warning) {
	items, warnings := a.search(ctx, "author:@me", model.SectionAuthored)
	if len(items) > 0 || len(warnings) == 0 {
		return items, warnings
	}
	// Respaldo REST si GraphQL no devolvió nada.
	if items, ok := a.restAuthored(ctx); ok {
		return items, warnings
	}
	return items, warnings
}

// ReviewRequested lista los PRs con review pedido al usuario.
func (a *Adapter) ReviewRequested(ctx context.Context) ([]model.Item, []model.Warning) {
	return a.search(ctx, "review-requested:@me", model.SectionReview)
}

// Mentions lista los PRs donde mencionan al usuario.
func (a *Adapter) Mentions(ctx context.Context) ([]model.Item, []model.Warning) {
	return a.search(ctx, "mentions:@me", model.SectionMentions)
}

// ItemState relee el estado de un PR concreto. Se completa en la etapa de
// detalle y acciones.
func (a *Adapter) ItemState(_ context.Context, _ model.RepoRef, _ int) (model.Item, []model.Warning) {
	return model.Item{}, unsupportedWarnings()
}

// Approve aprueba un PR. Se completa en la etapa de acciones.
func (a *Adapter) Approve(_ context.Context, _ model.RepoRef, _ int) []model.Warning {
	return unsupportedWarnings()
}

// Merge mergea un PR. Se completa en la etapa de acciones.
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

// search ejecuta la query GraphQL de búsqueda con el qualifier dado y etiqueta
// los ítems con su sección.
func (a *Adapter) search(ctx context.Context, qualifier string, section model.Section) ([]model.Item, []model.Warning) {
	raw, err := a.run(ctx, "api", "graphql", "-f", "query="+searchQuery(qualifier))
	if err != nil {
		return nil, []model.Warning{a.warn(section, classify(err), err)}
	}
	items, perr := parse.ParseGHGraphQLSearch(raw)
	if perr != nil {
		return nil, []model.Warning{a.warn(section, "parse", perr)}
	}
	a.stamp(items, section)
	return items, nil
}

// restAuthored consulta el respaldo REST de la búsqueda de PRs propios.
func (a *Adapter) restAuthored(ctx context.Context) ([]model.Item, bool) {
	raw, err := a.run(ctx, "api", "-X", "GET", "search/issues",
		"-f", "q=is:pr is:open author:@me", "-f", "per_page=50")
	if err != nil {
		return nil, false
	}
	items, perr := parse.ParseGHAuthored(raw)
	if perr != nil {
		return nil, false
	}
	a.stamp(items, model.SectionAuthored)
	return items, true
}

// stamp fija la sección y la identidad de forge/host en los ítems parseados.
func (a *Adapter) stamp(items []model.Item, section model.Section) {
	for i := range items {
		items[i].Section = section
		items[i].Forge = ForgeName
		items[i].Host = a.host
		items[i].Ref.Forge = ForgeName
		items[i].Ref.Host = a.host
	}
}

func (a *Adapter) warn(section model.Section, kind string, err error) model.Warning {
	return model.Warning{Forge: ForgeName, Section: section, Kind: kind, Msg: err.Error()}
}

// run ejecuta `gh` con timeout y entorno no interactivo, devolviendo stdout.
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

// searchQuery compone la query GraphQL de búsqueda de PRs con los campos ricos
// que el inbox necesita (reviewDecision y checks del último commit).
func searchQuery(qualifier string) string {
	return fmt.Sprintf(
		`query { search(query: "is:pr is:open %s", type: ISSUE, first: 50) { nodes { `+
			`number title url state isDraft reviewDecision updatedAt headRefName baseRefName `+
			`author { login } repository { nameWithOwner name owner { login } } `+
			`commits(last: 1) { nodes { commit { statusCheckRollup { state contexts(first: 50) { nodes { __typename status conclusion state } } } } } } `+
			`} } }`,
		qualifier,
	)
}

// toolEnv devuelve el entorno de los subprocesos: locale inglés para poder
// reconocer los mensajes de error y modo no interactivo.
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
		"GH_PROMPT_DISABLED=1",
		"NO_COLOR=1",
	)
}

// classify etiqueta el warning según el error: timeout, red o auth.
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
