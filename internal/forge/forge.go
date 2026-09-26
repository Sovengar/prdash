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
	"fmt"
	"strings"
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

// CommentLimit es cuántos comentarios de la conversación se leen de un ítem. Son
// los que la ficha llega a pintar (ver tui.commentLines); más que eso sería
// pagar una consulta por filas que no se ven nunca.
const CommentLimit = 5

// CommentPage son los comentarios leídos de un ítem y cuántos hay en la
// conversación.
//
// El total va aparte porque no siempre coincide con lo leído: la ficha solo
// muestra CommentLimit, así que el total es lo que le permite decir "5 de 23". Sin
// él, cinco comentarios leídos se leerían como cinco comentarios escritos, que es
// justo el dato que decide si hace falta abrir el PR en el navegador.
//
// Total puede ser una cota inferior, nunca un número al alza. GitHub lo da
// exacto con `totalCount`, pero en GitLab no existe tal cosa y solo se sabe
// cuántos se leyeron: si vinieron menos de los que se pidieron, la conversación
// se agotó y el número es exacto, y si la conexión se llenó puede haber más
// detrás. La ficha lo trata como una pista de que hay más conversación, nunca
// como un recuento: quedarse corto no hace que nadie decida mal.
type CommentPage struct {
	Comments []model.Comment
	Total    int
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
	// Comments devuelve los primeros comentarios de la conversación de un ítem,
	// del más antiguo al más reciente. El tope es CommentLimit, no un parámetro:
	// lo consume la ficha, que tiene un alto fijo, y una consulta se paga por lo
	// que devuelve. Como todos los métodos, nunca falla duro: si el forge no
	// llega a responder devuelve warnings.
	Comments(ctx context.Context, ref model.RepoRef, number int) (CommentPage, []model.Warning)
	// Approve aprueba un ítem.
	Approve(ctx context.Context, ref model.RepoRef, number int) []model.Warning
	// Merge mergea un ítem con la estrategia indicada. El modo no es opcional:
	// un merge sin estrategia nombrada es un prompt interactivo colgado.
	Merge(ctx context.Context, ref model.RepoRef, number int, mode MergeMode) []model.Warning
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

// Stream consulta un forge de forma progresiva: lanza en paralelo la primera
// página de cada lista y, según van llegando, continúa con las siguientes
// hasta agotarlas. Cada página se emite por emit, que debe ser seguro para uso
// concurrente y devuelve false para detener esa lista (p. ej. si su cabecera no
// cambió). Bloquea hasta agotar o cancelarse por contexto.
func Stream(ctx context.Context, a Adapter, emit func(PageResult) bool) {
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

// streamQuery recorre las páginas de una lista hasta agotarla o hasta que emit
// pida parar. Ante un warning de rate limit detiene la lista para no insistir.
func streamQuery(ctx context.Context, a Adapter, q Query, emit func(PageResult) bool) {
	cursor := q.Cursor
	first := true
	for {
		page, warns := a.List(ctx, Query{Section: q.Section, ReviewKind: q.ReviewKind, Cursor: cursor})
		cont := emit(PageResult{
			Query:    q,
			Items:    page.Items,
			Next:     page.Next,
			More:     page.More,
			Warnings: warns,
			First:    first,
		})
		if !cont || !page.More || page.Next == "" || ctx.Err() != nil || hasKind(warns, "ratelimit") {
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
			streamQuery(ctx, a, q, func(p PageResult) bool {
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
				res.Warnings = append(res.Warnings, StampSection(p.Warnings, q.Section)...)
				return true
			})
		}(q)
	}
	wg.Wait()
	return res
}

// StampSection etiqueta con su sección los warnings que no la traigan.
func StampSection(warns []model.Warning, section model.Section) []model.Warning {
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

// MergeMode es cómo se integra el PR/MR en la rama destino.
//
// No hay modo por defecto a propósito: la TUI exige una doble pulsación y la
// segunda tecla ES la elección del modo, así que un merge nunca se dispara con
// una estrategia que el usuario no ha nombrado. Un rebase donde tocaba squash
// reescribe historia ya publicada y no se deshace con un comando.
type MergeMode string

const (
	// MergeCommit integra los commits con la rama destino.
	MergeCommit MergeMode = "merge"
	// Rebase reaplica los commits encima de la rama destino.
	Rebase MergeMode = "rebase"
	// Squash aplana los commits en uno solo.
	Squash MergeMode = "squash"
)

// Label es el nombre del modo para la UI y los avisos: es lo que el usuario
// lee antes de confirmar y lo que ve después.
func (m MergeMode) Label() string {
	switch m {
	case MergeCommit:
		return "merge commit"
	case Rebase:
		return "rebase"
	case Squash:
		return "squash"
	default:
		return string(m)
	}
}

// Valid informa si el modo es uno de los conocidos. Cada adapter lo consulta
// antes de construir su argv: un modo desconocido tiene que ser un warning
// explícito, no un flag vacío, porque `gh pr merge` sin flag de estrategia
// cae en un prompt interactivo y se quedaría colgado.
func (m MergeMode) Valid() bool {
	switch m {
	case MergeCommit, Rebase, Squash:
		return true
	default:
		return false
	}
}

// ErrUnknownMergeMode es el motivo que devuelve un adapter cuando le llega un
// modo que no reconoce. Vive aquí para que los dos forges que sí implementan
// merge hablen del mismo modo y no cada uno del suyo.
func ErrUnknownMergeMode(mode MergeMode) error {
	return fmt.Errorf("unknown merge mode %q: expected merge, rebase or squash", string(mode))
}

// Outcome es el resultado de ejecutar una acción rápida.
type Outcome struct {
	Kind     ActionKind
	ID       model.ID
	Mode     MergeMode // estrategia usada; solo tiene sentido en ActionMerge
	OK       bool      // la acción se aplicó
	Conflict bool      // el ítem cambió (cerrado/mergeado/ausente): hay que refrescar
	Perm     bool      // acción deshabilitada por permisos o por no soportado
	Msg      string    // motivo para la UI
	Item     model.Item
	HasItem  bool // Item trae el estado releído
}

// RunAction ejecuta una acción rápida de forma segura: relee el ítem, comprueba
// que sigue accionable, ejecuta la acción y vuelve a releer el estado para
// dejarlo consistente. Nunca devuelve error duro: clasifica el fallo en
// conflicto (el ítem cambió), permiso o error genérico.
//
// mode solo aplica a ActionMerge; approve lo ignora. Se pasa siempre para que la
// firma no dependa de la acción, que es lo que permite dispatchar con un switch.
func RunAction(ctx context.Context, a Adapter, kind ActionKind, ref model.RepoRef, number int, mode MergeMode) Outcome {
	out := Outcome{Kind: kind, ID: model.With(ref.Forge, ref.Host, ref.Project, number), Mode: mode}

	cur, warns := a.ItemState(ctx, ref, number)
	if stage := checkBeforeAction(cur, warns); stage != nil {
		stage.Kind, stage.ID = kind, out.ID
		return *stage
	}
	out.Item, out.HasItem = cur, true

	var actionWarns []model.Warning
	switch kind {
	case ActionApprove:
		actionWarns = a.Approve(ctx, ref, number)
	case ActionMerge:
		actionWarns = a.Merge(ctx, ref, number, mode)
	}
	out.OK, out.Conflict, out.Perm, out.Msg = classifyAction(actionWarns)

	// Relee el estado para dejar el ítem consistente tras la acción o el fallo.
	if it, w := a.ItemState(ctx, ref, number); len(w) == 0 && it.Number != 0 {
		out.Item, out.HasItem = it, true
	}
	return out
}

// checkBeforeAction decide si el ítem puede accionarse según su estado releído.
// Devuelve un Outcome de conflicto o permiso, o nil si se puede continuar.
func checkBeforeAction(cur model.Item, warns []model.Warning) *Outcome {
	switch {
	case hasKind(warns, "notfound"):
		return &Outcome{Conflict: true, Msg: "the item no longer exists in the forge"}
	case hasKind(warns, "permission"), hasKind(warns, "auth"):
		return &Outcome{Perm: true, Msg: firstMsg(warns)}
	case len(warns) > 0 || cur.Number == 0:
		msg := firstMsg(warns)
		if msg == "" {
			msg = "could not re-read the item"
		}
		out := &Outcome{Conflict: true, Msg: msg}
		if cur.Number != 0 {
			out.Item, out.HasItem = cur, true
		}
		return out
	}
	if ok, reason := state.Actionable(cur); !ok {
		return &Outcome{Conflict: true, Msg: reason, Item: cur, HasItem: true}
	}
	return nil
}

// classifyAction traduce los warnings de una acción a (ok, conflicto, permiso,
// motivo).
func classifyAction(warns []model.Warning) (ok, conflict, perm bool, msg string) {
	if len(warns) == 0 {
		return true, false, false, ""
	}
	msg = firstMsg(warns)
	switch {
	case hasKind(warns, "permission"), hasKind(warns, "auth"), hasKind(warns, "unsupported"):
		return false, false, true, msg
	case hasKind(warns, "selfreview"):
		// Es una denegación permanente de ese ítem, no un conflicto: se
		// clasifica como permiso para que la TUI la deje registrada y no
		//Repita la llamada. El motivo es el canónico, no el stderr de la CLI.
		return false, false, true, state.SelfReviewReason
	case hasKind(warns, "notfound"), hasKind(warns, "conflict"),
		hasKind(warns, "ratelimit"), hasKind(warns, "network"), hasKind(warns, "timeout"):
		return false, true, false, msg
	default:
		return false, false, false, msg
	}
}

func firstMsg(warns []model.Warning) string {
	if len(warns) == 0 {
		return ""
	}
	return warns[0].Msg
}

// EscapeGraphQL escapa un valor para incrustarlo como literal de GraphQL:
// barras, comillas y saltos de línea (que romperían la query en una línea).
func EscapeGraphQL(s string) string {
	return strings.NewReplacer(
		`\`, `\\`,
		`"`, `\"`,
		"\n", `\n`,
		"\r", `\r`,
		"\t", `\t`,
	).Replace(s)
}
