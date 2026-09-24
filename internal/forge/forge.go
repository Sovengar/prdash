// Package forge define el contrato que implementa cada forge y el registro de
// adapters habilitados.
//
// Los métodos devuelven ítems más warnings tipados: nunca un error duro. Un
// forge caído o sin autenticar no puede vaciar el inbox. La consulta del inbox
// es paginable, de modo que la UI puede pintar la primera página y seguir
// trayendo el resto en segundo plano.
package forge

import (
	"context"
	"sort"
	"sync"

	"prdash/internal/forge/model"
	"prdash/internal/inbox"
	"prdash/internal/state"
)

// Query describe una lista paginable del inbox. Cursor vacío = primera página.
type Query struct {
	Section    model.Section
	ReviewKind model.ReviewKind // requested/assigned (solo review)
	Cursor     string
}

// Page es una página de resultados de una Query.
type Page struct {
	Items []model.Item
	Next  string // cursor de la página siguiente
	More  bool   // quedan páginas
}

// Streams son las listas que se consultan, en orden de pintado.
var Streams = []Query{
	{Section: model.SectionAuthored},
	{Section: model.SectionReview, ReviewKind: model.ReviewRequested},
	{Section: model.SectionReview, ReviewKind: model.ReviewAssigned},
	{Section: model.SectionMentions},
}

// Adapter es el contrato de un forge. Las implementaciones hablan con su CLI
// (`gh`, `glab`, …) por subproceso.
type Adapter interface {
	// Forge devuelve el nombre del forge ("github", "gitlab", …).
	Forge() string
	// Host devuelve el host configurado ("github.com", …).
	Host() string
	// Auth informa si el forge está autenticado y operativo.
	Auth(ctx context.Context) model.AuthState
	// List devuelve una página de resultados de la lista pedida.
	List(ctx context.Context, q Query) (Page, []model.Warning)
	// ItemState relee el estado de un ítem concreto.
	ItemState(ctx context.Context, ref model.RepoRef, number int) (model.Item, []model.Warning)
	// Approve aprueba un ítem.
	Approve(ctx context.Context, ref model.RepoRef, number int) []model.Warning
	// Merge mergea un ítem.
	Merge(ctx context.Context, ref model.RepoRef, number int) []model.Warning
}

// PageResult es una página emitida por Stream.
type PageResult struct {
	Query    Query
	Items    []model.Item
	Next     string
	More     bool
	Warnings []model.Warning
	First    bool // primera página de la lista
}

// Registry mantiene los adapters habilitados por nombre de forge.
type Registry struct {
	adapters map[string]Adapter
}

// NewRegistry construye un registro vacío.
func NewRegistry() *Registry {
	return &Registry{adapters: map[string]Adapter{}}
}

// Register añade un adapter al registro, reemplazando el anterior del mismo
// forge si lo hubiera.
func (r *Registry) Register(a Adapter) {
	if a == nil {
		return
	}
	r.adapters[a.Forge()] = a
}

// Get devuelve el adapter de un forge.
func (r *Registry) Get(forge string) (Adapter, bool) {
	a, ok := r.adapters[forge]
	return a, ok
}

// All devuelve los adapters ordenados por nombre de forge, para un arranque
// determinista.
func (r *Registry) All() []Adapter {
	out := make([]Adapter, 0, len(r.adapters))
	for _, a := range r.adapters {
		out = append(out, a)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Forge() < out[j].Forge() })
	return out
}

// Stream consulta un forge de forma progresiva: lanza en paralelo la primera
// página de cada lista y, según van llegando, continúa con las siguientes
// hasta agotarlas. Cada página se emite por emit, que debe ser seguro para uso
// concurrente. Bloquea hasta agotar o cancelarse por contexto.
func Stream(ctx context.Context, a Adapter, emit func(PageResult)) {
	var wg sync.WaitGroup
	for _, q := range Streams {
		wg.Add(1)
		go func(q Query) {
			defer wg.Done()
			streamQuery(ctx, a, q, emit)
		}(q)
	}
	wg.Wait()
}

// streamQuery recorre las páginas de una lista hasta agotarla. Ante un warning
// de rate limit detiene la lista para no insistir.
func streamQuery(ctx context.Context, a Adapter, q Query, emit func(PageResult)) {
	cursor := q.Cursor
	first := true
	for {
		page, warns := a.List(ctx, Query{Section: q.Section, ReviewKind: q.ReviewKind, Cursor: cursor})
		emit(PageResult{
			Query:    q,
			Items:    page.Items,
			Next:     page.Next,
			More:     page.More,
			Warnings: warns,
			First:    first,
		})
		if !page.More || page.Next == "" || ctx.Err() != nil || hasKind(warns, "ratelimit") {
			return
		}
		cursor = page.Next
		first = false
	}
}

// Collect consulta un forge completo (todas las páginas) y compone su resultado
// del inbox. Es el modo de una sola pasada que usa la impresión por texto.
func Collect(ctx context.Context, a Adapter) inbox.ForgeResult {
	res := inbox.ForgeResult{Forge: a.Forge(), Host: a.Host()}
	if auth := a.Auth(ctx); !auth.OK {
		res.Warnings = append(res.Warnings, model.Warning{
			Forge: a.Forge(),
			Kind:  "auth",
			Msg:   auth.Reason,
		})
	}

	var (
		mu sync.Mutex
		wg sync.WaitGroup
	)
	for _, q := range Streams {
		wg.Add(1)
		go func(q Query) {
			defer wg.Done()
			streamQuery(ctx, a, q, func(p PageResult) {
				mu.Lock()
				defer mu.Unlock()
				switch q.Section {
				case model.SectionAuthored:
					res.Authored = append(res.Authored, p.Items...)
				case model.SectionReview:
					res.Review = append(res.Review, p.Items...)
				case model.SectionMentions:
					res.Mentions = append(res.Mentions, p.Items...)
				}
				res.Warnings = append(res.Warnings, stampSection(p.Warnings, q.Section)...)
			})
		}(q)
	}
	wg.Wait()
	return res
}

// stampSection etiqueta con su sección los warnings que no la traigan.
func stampSection(warns []model.Warning, section model.Section) []model.Warning {
	for i := range warns {
		if warns[i].Section == "" {
			warns[i].Section = section
		}
	}
	return warns
}

func hasKind(warns []model.Warning, kind string) bool {
	for _, w := range warns {
		if w.Kind == kind {
			return true
		}
	}
	return false
}

// ActionKind es el tipo de acción rápida del inbox.
type ActionKind string

const (
	// ActionApprove aprueba el ítem.
	ActionApprove ActionKind = "approve"
	// ActionMerge mergea el ítem.
	ActionMerge ActionKind = "merge"
)

// Outcome es el resultado de ejecutar una acción rápida.
type Outcome struct {
	Kind     ActionKind
	ID       model.ID
	OK       bool   // la acción se aplicó
	Conflict bool   // el ítem cambió (cerrado/mergeado/ausente): hay que refrescar
	Perm     bool   // acción deshabilitada por permisos o por no soportado
	Msg      string // motivo para la UI
	Item     model.Item
	HasItem  bool // Item trae el estado releído
}

// RunAction ejecuta una acción rápida de forma segura: relee el ítem, comprueba
// que sigue accionable, ejecuta la acción y vuelve a releer el estado para
// dejarlo consistente. Nunca devuelve error duro: clasifica el fallo en
// conflicto (el ítem cambió), permiso o error genérico.
func RunAction(ctx context.Context, a Adapter, kind ActionKind, ref model.RepoRef, number int) Outcome {
	out := Outcome{Kind: kind, ID: model.With(ref.Forge, ref.Host, ref.Project, number)}

	cur, warns := a.ItemState(ctx, ref, number)
	switch {
	case hasKind(warns, "notfound"):
		out.Conflict = true
		out.Msg = "el ítem ya no existe en el forge"
		return out
	case hasKind(warns, "permission"), hasKind(warns, "auth"):
		out.Perm = true
		out.Msg = firstMsg(warns)
		return out
	case len(warns) > 0 || cur.Number == 0:
		out.Conflict = true
		out.Msg = firstMsg(warns)
		if out.Msg == "" {
			out.Msg = "no se pudo releer el ítem"
		}
		return out
	}

	out.Item, out.HasItem = cur, true
	if ok, reason := state.Actionable(cur); !ok {
		out.Conflict = true
		out.Msg = reason
		return out
	}

	var actionWarns []model.Warning
	switch kind {
	case ActionApprove:
		actionWarns = a.Approve(ctx, ref, number)
	case ActionMerge:
		actionWarns = a.Merge(ctx, ref, number)
	}
	if hasKind(actionWarns, "permission") || hasKind(actionWarns, "auth") || hasKind(actionWarns, "unsupported") {
		out.Perm = true
		out.Msg = firstMsg(actionWarns)
		return out
	}

	out.OK = len(actionWarns) == 0
	if !out.OK {
		out.Msg = firstMsg(actionWarns)
		for _, k := range []string{"notfound", "conflict", "ratelimit", "network", "timeout"} {
			if hasKind(actionWarns, k) {
				out.Conflict = true
			}
		}
	}

	// Relee el estado para dejar el ítem consistente tras la acción o el fallo.
	if it, w := a.ItemState(ctx, ref, number); len(w) == 0 && it.Number != 0 {
		out.Item, out.HasItem = it, true
	}
	return out
}

func firstMsg(warns []model.Warning) string {
	if len(warns) == 0 {
		return ""
	}
	return warns[0].Msg
}
