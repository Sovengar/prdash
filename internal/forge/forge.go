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
	"errors"
	"fmt"
	"slices"
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

// KeepLast recorta la página a los CommentLimit últimos comentarios y deja el total
// intacto.
//
// Recorta por la COLA y no por la cabeza, que es lo que distingue "los últimos" de
// "los primeros": con la conexión pedida al revés, lo que sobra por delante es
// justo la parte que la ficha no iba a enseñar. El total no se toca, porque es lo
// que permite saber que lo que se ve no es todo lo que hay.
func (p CommentPage) KeepLast() CommentPage {
	if len(p.Comments) > CommentLimit {
		p.Comments = p.Comments[len(p.Comments)-CommentLimit:]
	}
	return p
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
	// Comments devuelve los Últimos comentarios de la conversación de un ítem, en
	// orden cronológico: los más recientes, del más antiguo de esos al más nuevo. El
	// tope es CommentLimit, no un parámetro: lo consume la ficha, que tiene un alto
	// fijo, y una consulta se paga por lo que devuelve. Como todos los métodos,
	// nunca falla duro: si el forge no llega a responder devuelve warnings.
	Comments(ctx context.Context, ref model.RepoRef, number int) (CommentPage, []model.Warning)
	// Approve aprueba un ítem.
	Approve(ctx context.Context, ref model.RepoRef, number int) []model.Warning
	// Merge mergea un ítem con lo que pide la MergeRequest dada.
	Merge(ctx context.Context, ref model.RepoRef, number int, req MergeRequest) []model.Warning
	// Retarget cambia la rama destino del ítem por la dada.
	//
	// No es approve/merge con otro nombre: cambia contra qué se integra el PR, así
	// que a partir de aquí el diff, la mergeabilidad y el CI son otros, y lo que
	// se ha mergeado o el forge ya no lo recuerda. No lleva pin de head SHA porque
	// mover la base no integra nada: el pin protege la INTEGRACIÓN, que es lo
	// irreversible, y aquí lo único que cambia es a qué se compara.
	//
	// Un branch vacío es un warning explícito y no un argv con el flag vacío, que
	// el forge lee como "deja el PR sin base".
	Retarget(ctx context.Context, ref model.RepoRef, number int, branch string) []model.Warning
	// Branches lista las ramas del repositorio, para el buscador de la base
	// destino. Es lo que hace que la acción no dependa de que nadie se acuerde del
	// nombre de la rama: con texto libre, una errata la acepta el forge y no se ve
	// hasta que el PR apunta a `main` en vez de a `main-2`, que es el peor sitio
	// para descubrirlo.
	//
	// Las ramas vienen del forge y de ningún otro sitio: preguntarle al clon local
	// daría solo las refs bajadas, que no son las que el forge puede integrar.
	Branches(ctx context.Context, ref model.RepoRef) ([]string, []model.Warning)
}

// MergeRequest es lo que un merge necesita saber: con qué estrategia integrar,
// a qué commit se pinea y si la rama origen se borra al integrar.
//
// Va en struct y no como parámetros sueltos porque son tres datos que solo
// tienen sentido juntos —el pin sin modo no es un merge, y el borrado sin pin
// sería un merge a ciegas— y porque el forgetting de un bool entre dos strings
// es un fallo que compila.
type MergeRequest struct {
	// Mode es la estrategia de integración. No es opcional: un merge sin
	// estrategia nombrada es un prompt interactivo colgado.
	Mode MergeMode
	// HeadSHA es el commit al que la rama origen apuntaba en la lectura del
	// ítem, y el adapter DEBE pinear el merge a él. No es una cortesía: entre el
	// refresco del inbox y la pulsación la rama puede haber avanzado, y sin el
	// pin el merge integra commits que nadie revisó. Un headSHA vacío significa
	// "el forge no lo reportó" y se traduce en un warning, nunca en un merge
	// sin pin.
	HeadSHA string
	// DeleteBranch pide borrar la rama origen una vez integrado. Lo decide la
	// Confirmación de merge de la TUI y no la config, porque es la segunda de las
	// dos cosas que el usuario nombra en el gesto del merge.
	//
	// Es un efecto POSTERIOR a la integración y el adapter tiene que entenderlo
	// así: el flag que lo pide también nombra la rama en GitHub, donde borrarla
	// es parte del mismo comando. Si el borrado falla, el merge ya está hecho y
	// el resultado NO es un merge fallido (ver Outcome.DeleteMsg).
	DeleteBranch bool
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
	// ActionRetarget cambia la rama destino del ítem.
	ActionRetarget ActionKind = "retarget"
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

// ErrMissingHeadSHA es el motivo que devuelve un adapter cuando no puede pinear
// el merge al commit que se leyó del ítem.
//
// El pin no es decorativo: sin él, la rama origen puede haber avanzado entre la
// lectura y la pulsación y el merge integra commits que nadie miró. Un forge que
// no reporta el head SHA no es un forge al que se pueda mergear con seguridad
// desde aquí, así que la acción se niega en vez de dejarse sin pin.
var ErrMissingHeadSHA = errors.New(
	"the forge did not report the head commit, so the merge cannot be pinned to what was reviewed")

// ErrMissingBaseBranch es el motivo que devuelve un adapter cuando le piden
// cambiar la base sin decir cuál.
//
// El corte es aquí y no en la TUI porque el daño lo hace el argv, no la vista:
// un flag de base vacío no es una operación que no hace nada, es una que le dice
// al forge que deixe el ítem sin rama destino, que es justo el estado que el
// usuario quiere evitar.
var ErrMissingBaseBranch = errors.New("the new target branch is empty: there is nothing to retarget to")

// AllowedModes devuelve los modos que el repositorio admite, en el orden en que
// se ofrecen al usuario: rebase, merge commit, squash.
//
// Un repositorio que no publica sus reglas devuelve los tres, porque no saber
// no es lo mismo que no permitir y un filtro inventado dejaría fuera el único
// modo que el repositorio sí acepta. El coste de equivocarse es asimétrico: un
// modo de más lo rechaza el forge con un mensaje claro, y un modo de menos deja
// al usuario sin una salida legítima.
func AllowedModes(rules model.MergeRules) []MergeMode {
	if !rules.Known {
		return []MergeMode{Rebase, MergeCommit, Squash}
	}
	var out []MergeMode
	if rules.Rebase {
		out = append(out, Rebase)
	}
	if rules.MergeCommit {
		out = append(out, MergeCommit)
	}
	if rules.Squash {
		out = append(out, Squash)
	}
	return out
}

// AllowsMode informa si un modo concreto está permitido por las reglas dadas.
func AllowsMode(rules model.MergeRules, mode MergeMode) bool {
	return slices.Contains(AllowedModes(rules), mode)
}

// Outcome es el resultado de ejecutar una acción rápida.
type Outcome struct {
	Kind     ActionKind
	ID       model.ID
	Mode     MergeMode // estrategia usada; solo tiene sentido en ActionMerge
	OK       bool      // la acción se aplicó
	Conflict bool      // el ítem cambió (cerrado/mergeado/ausente): hay que refrescar
	Perm     bool      // acción deshabilitada por permisos o por no soportado
	// Unmergeable dice que el forge no pudo crear el merge con las ramas como
	// están. Viaja aparte de Conflict y de Perm porque no es lo mismo que
	// ninguna: un conflicto de estado se resuelve refrescando y un permiso no
	// tiene salida, mientras que aquí lo que hay que hacer es rebasar. La TUI lo
	// necesita para no prometer un refresco que no arregla nada.
	Unmergeable bool
	Msg         string // motivo para la UI
	Item        model.Item
	HasItem     bool // Item trae el estado releído

	// DeleteBranch es lo que se pidió en la Confirmación, para que el aviso
	// pueda decir qué pasó con la rama en vez de callarse.
	DeleteBranch bool
	// DeleteMsg es el motivo por el que la rama NO se borró en un merge que sí
	// salió. Vacío = nada que avisar.
	//
	// Vive aparte de Msg porque el fallo del borrado no es el fallo del merge: el
	// ítem está mergeado y volver a intentarlo no es lo que hay que hacer. Sin
	// esta distinción, un "merge falló" sobre un PR ya integrado hace que el
	// usuario busque un estado del forge que no existe.
	DeleteMsg string

	// Base es la rama destino que se pidió y FromBase la que tenía el ítem. Los
	// dos solo tienen sentido en ActionRetarget, y viajan juntos porque el aviso
	// tiene que poder decir "main → release/2.0": un "retarget ok" a secas no dice
	// qué ha pasado con el PR, que es justo lo que el usuario acaba de hacer.
	//
	// FromBase lo rellena quien tenía la lectura —la TUI, al confirmar— y no el
	// adapter: el forge no mira la base antigua, solo escribe la nueva.
	Base     string
	FromBase string
}

// runOn es el camino único de toda acción sobre un ítem: relee, comprueba que
// sigue accionable, ejecuta y relee otra vez para dejarlo consistente. Clasifica
// el fallo en conflicto, permiso o error genérico, y nunca devuelve error duro.
//
// El ejecutor recibe la RELECTURA, no la copia que la TUI tenía en memoria: es la
// lectura inmediatamente anterior a la acción y por tanto la que menos ventana
// deja entre lo observado y lo aplicado. RunAction no comprueba el pin del merge
// porque lo delega en el adapter, que es quien conoce el flag de cada CLI; aquí
// solo se le pasa el commit.
//
// seed son los campos que la acción ya sabe de sí misma (modo, borrado de rama) y
// que un guard temprano no debe perder: el Outcome de un "merge bloqueado" sigue
// siendo un Outcome de merge. Kind e ID los pone runOn, que son los mismos para
// todas.
//
// Devuelve también los warnings crudos de la acción porque un post-proceso los
// necesita para no perder el motivo original: el borrado de la rama del merge
// falla dentro del mismo comando que el merge, y su motivo no está en Msg.
func runOn(seed Outcome, ctx context.Context, a Adapter, kind ActionKind, ref model.RepoRef, number int, exec func(cur model.Item) []model.Warning) (Outcome, []model.Warning) {
	out := seed
	out.Kind, out.ID = kind, model.With(ref.Forge, ref.Host, ref.Project, number)

	cur, warns := a.ItemState(ctx, ref, number)
	if stage := checkBeforeAction(cur, warns); stage != nil {
		stage.Kind, stage.ID = kind, out.ID
		return *stage, nil
	}
	out.Item, out.HasItem = cur, true

	actionWarns := exec(cur)
	out.OK, out.Conflict, out.Perm, out.Msg = classifyAction(actionWarns)
	// classifyAction ya lo separó de Perm y de Conflict con el motivo canónico;
	// aquí solo se marca para que la TUI sepa con qué palabras presentarlo.
	out.Unmergeable = hasKind(actionWarns, "unmergeable")

	// Relee el estado para dejar el ítem consistente tras la acción o el fallo.
	if it, w := a.ItemState(ctx, ref, number); len(w) == 0 && it.Number != 0 {
		out.Item, out.HasItem = it, true
	}
	return out, actionWarns
}

// RunAction ejecuta approve o merge de forma segura.
//
// req solo aplica a ActionMerge; approve lo ignora. Se pasa siempre para que la
// firma no dependa de la acción, que es lo que permite dispatchar con un switch.
func RunAction(ctx context.Context, a Adapter, kind ActionKind, ref model.RepoRef, number int, req MergeRequest) Outcome {
	// Una acción que no existe NO se despacha, y el corte va ANTES de `runOn`.
	//
	// Antes, el `default` del switch de abajo devolvía nil, y nil es exactamente lo que
	// `classifyAction` lee como "no hubo ningún warning" —que es su forma de decir que la
	// acción salió bien—. El resultado era que `RunAction` con una acción desconocida
	// devolvía `OK: true` sin haber hecho nada, y el aviso de la cabecera —que se compone de
	// `OK`— decía lo contrario de lo que pasó.
	//
	// Hoy no se ve porque la TUI filtra por `canActionOn` y solo deja pasar approve y merge,
	// y el retarget tiene su propio camino. Pero una contención que informa de éxito no
	// contiene: el día que un Kind nuevo se enrute por aquí sin añadir su rama, el usuario
	// ve "hecho" sobre una acción que no ocurrió.
	//
	// Y el corte va antes de `runOn` y no dentro del `exec` por dos razones que se pagan en
	// cada llamada equivocada: `runOn` relee el ítem del forge, así que despachar una acción
	// imposible cuesta un viaje de ida y vuelta a GitHub para nada; y la respuesta no viene
	// de un forge, así que pasarla por `classifyAction` —que clasifica respuestas de forge—
	// sería meterla en una categoría que no le corresponde.
	//
	// El `Outcome` sale sin banderas a propósito: no es conflicto —el ítem no ha cambiado— ni
	// permiso —que en la TUI registra el ítem como denegado y no lo vuelve a intentar—. Es un
	// fallo de quién llamó, no del ítem ni de la sesión.
	// Ver `docs/adr/0008-dispatch-de-accion-desconocida.md`.
	if kind != ActionApprove && kind != ActionMerge {
		return Outcome{
			Kind:         kind,
			ID:           model.With(ref.Forge, ref.Host, ref.Project, number),
			Mode:         req.Mode,
			DeleteBranch: req.DeleteBranch,
			Msg:          "prdash does not implement the " + string(kind) + " action",
		}
	}

	out, actionWarns := runOn(
		Outcome{Mode: req.Mode, DeleteBranch: req.DeleteBranch},
		ctx, a, kind, ref, number,
		func(cur model.Item) []model.Warning {
			// El `default` que antes hacía de contención para un Kind desconocido ya no
			// está: `RunAction` corta antes de llegar aquí. Dejarlo sería un segundo sitio
			// donde un Kind nuevo se despacha en silencio, y dos sitios que dicen lo mismo
			// divergen.
			if kind == ActionApprove {
				return a.Approve(ctx, ref, number)
			}
			req.HeadSHA = cur.HeadSHA
			return a.Merge(ctx, ref, number, req)
		},
	)

	// El borrado de la rama va en el MISMO comando que el merge, así que su fallo
	// sale como fallo del comando entero aunque la integración ya esté hecha: sin
	// push, con la rama protegida, o en un repo con merge queue (que rechaza `-d`
	// antes de mergear). La relectura de runOn distingue los dos casos sin gastar
	// una llamada más: si el ítem vuelve mergeado, el merge salió y lo que falló
	// es el borrado.
	if kind == ActionMerge && req.DeleteBranch && out.HasItem {
		if !out.OK && state.Derive(out.Item) == state.StateMerged {
			out.OK, out.Conflict, out.Perm, out.Msg = true, false, false, ""
			out.DeleteMsg = firstMsg(actionWarns)
		}
		// Y al revés: un PR de fork NO tiene rama que borrar en el repo destino, y
		// el forge no protesta —lo da por hecho y sale con éxito—, así que sin esto
		// el aviso afirmaría un borrado que no ocurrió.
		if out.OK && out.Item.IsFork {
			out.DeleteMsg = "the branch lives in a fork"
		}
	}
	return out
}

// RunRetarget cambia la rama destino del ítem.
//
// Comparte el camino de runOn con approve/merge —mismos guards, misma
// clasificación, misma relectura— en vez de tener el suyo: son las tres acciones
// que se ejecutan sobre un ítem abierto y por eso tienen exactamente los mismos
// motivos para negarse. La única diferencia es qué se le pide al adapter, y eso
// lo dice el nombre de la rama.
//
// No lleva pin de head SHA y no es un descuido: mover la base no integra commits
// con la rama destino, solo cambia contra qué se comparan, así que lo que protege
// el pin (integrar lo que nadie revisó) no puede pasar aquí. Lo que sí se relee es
// el ítem, para que la vista enseñe la base nueva sin esperar al refresco.
func RunRetarget(ctx context.Context, a Adapter, ref model.RepoRef, number int, branch string) Outcome {
	if strings.TrimSpace(branch) == "" {
		// Se corta antes de releer: sin rama no hay nada que enviar, y una
		// relectura solo serviría para gastar una llamada y devolver un estado que
		// no se va a tocar.
		return Outcome{
			Kind: ActionRetarget,
			ID:   model.With(ref.Forge, ref.Host, ref.Project, number),
			Perm: true,
			Msg:  ErrMissingBaseBranch.Error(),
			Item: model.Item{},
		}
	}
	out, _ := runOn(
		Outcome{Base: branch},
		ctx, a, ActionRetarget, ref, number,
		func(model.Item) []model.Warning { return a.Retarget(ctx, ref, number, branch) },
	)
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
	case hasKind(warns, "unmergeable"):
		// No es un conflicto en el sentido de "el ítem cambió": eso se resuelve
		// solo y por eso avisa de refrescar. Aquí las ramas se pisan y no se
		// arreglan solas, así que el motivo es el canónico —que dice lo que hay
		// que hacer— en vez del inglés de la CLI. No es permiso tampoco: registrar
		// el ítem como denegado lo dejaría sin merge para siempre, y un rebase lo
		// arregla. Lo consume la TUI por su cuenta con Outcome.Unmergeable.
		return false, false, false, state.UnmergeableReason
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
