// Package testutil ofrece dobles en memoria de los contratos de prdash para
// tests (sin red, sin subproceso, sin disco) y una suite de conformidad
// compartida que todo adapter debe pasar.
package testutil

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"sync"

	"prdash/internal/forge"
	"prdash/internal/forge/model"
)

// FakeKey identifica una lista paginable del inbox.
type FakeKey struct {
	Section model.Section
	Kind    model.ReviewKind
}

// FakeAdapter es una implementación en memoria de forge.Adapter. Los tests
// configuran páginas, warnings y estados por lista; nada toca una CLI.
type FakeAdapter struct {
	ForgeName string
	HostName  string
	AuthState model.AuthState

	// Pages es la secuencia de páginas por lista; cada llamada a List avanza
	// una página y la última se repite.
	Pages map[FakeKey][]forge.Page
	// ListWarnings se emiten en cada llamada a List de la lista dada.
	ListWarnings map[FakeKey][]model.Warning

	// ItemStates / StateWarnings responden a ItemState por "proyecto#número".
	ItemStates    map[string]model.Item
	StateWarnings map[string][]model.Warning

	// Conversations / CommentWarnings responden a Comments por "proyecto#número".
	// Sin configurar, Comments devuelve una conversación vacía YA resuelta: así un
	// test que no se ocupa de los comentarios no se encuentra con un "cargando…"
	// que no termina nunca.
	Conversations   map[string]forge.CommentPage
	CommentWarnings map[string][]model.Warning

	// ActionWarnings responde a Approve/Merge/Retarget por
	// "<acción>:proyecto#número".
	ActionWarnings map[string][]model.Warning

	// BranchLists / BranchWarnings responden a Branches por proyecto. Sin
	// configurar, Branches devuelve una lista vacía YA resuelta, por el mismo
	// motivo que Comments: un test que no habla de ramas no debería tener que
	// inventarlas para que el buscador no se quede colgado.
	BranchLists    map[string][]string
	BranchWarnings map[string][]model.Warning

	mu          sync.Mutex
	calls       map[FakeKey]int
	listCalls   int
	actionCalls map[string]int
	// commentCalls cuenta cuántas veces se pidieron comentarios, para que un test
	// pueda afirmar que la ficha cachea y no repregunta en cada render.
	commentCalls int
	// MergeModes cuenta quantas veces se pidió cada estrategia de merge.
	MergeModes map[forge.MergeMode]int
	// Retargets registra, en orden, la base que se pidió en cada cambio de rama
	// destino. Es una lista y no un contador porque la pregunta de un test es
	// "¿a qué base lo movió?", y eso solo lo responde el orden.
	Retargets []string
	// branchCalls cuenta cuántas veces se pidieron las ramas de un repositorio,
	// para que un test pueda afirmar que el buscador cachea.
	branchCalls map[string]int
	// MergeDeletes registra, en orden, si cada merge pidió borrar la rama. Es una
	// lista y no un contador porque la pregunta de un test es "este merge, ¿con
	// borrado o sin él?", y con la lista se contesta sin depender del orden en
	// que el test firearms las llamadas.
	MergeDeletes []bool
	// OnMerge corre sobre el ítem en las lecturas de estado posteriores a un merge.
	// Sirve para lo que un fake de estado fijo no puede expresar: un merge que
	// sale y además cambia el ítem, que es el caso del borrado de la rama (el
	// merge queda hecho aunque el comando salga con error).
	OnMerge func(*model.Item)
	// merged marca que ya se pidió un merge, para que OnMerge no se aplique a la
	// relectura previa (la que decide si el merge sale).
	merged bool
}

// Cumple el contrato en compilación.
var _ forge.Adapter = (*FakeAdapter)(nil)

// Forge devuelve el nombre configurado.
func (f *FakeAdapter) Forge() string { return f.ForgeName }

// Host devuelve el host configurado.
func (f *FakeAdapter) Host() string { return f.HostName }

// Auth devuelve el estado configurado; por defecto, autenticado.
func (f *FakeAdapter) Auth(_ context.Context) model.AuthState {
	if f.AuthState.Forge == "" && !f.AuthState.OK && f.AuthState.Reason == "" {
		return model.AuthState{Forge: f.ForgeName, OK: true}
	}
	return f.AuthState
}

// List devuelve la siguiente página configurada para la lista.
func (f *FakeAdapter) List(_ context.Context, q forge.Query) (forge.Page, []model.Warning) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.calls == nil {
		f.calls = map[FakeKey]int{}
	}
	key := FakeKey{Section: q.Section, Kind: q.ReviewKind}
	f.listCalls++

	pages := f.Pages[key]
	idx := f.calls[key]
	f.calls[key]++
	if len(pages) == 0 {
		return forge.Page{}, f.ListWarnings[key]
	}
	if idx >= len(pages) {
		idx = len(pages) - 1
	}
	return pages[idx], f.ListWarnings[key]
}

// ListCallCount devuelve cuántas veces se llamó a List (todas las listas).
func (f *FakeAdapter) ListCallCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.listCalls
}

// ItemState devuelve el ítem configurado para "proyecto#número".
//
// OnMerge, si está puesto, se aplica SOLO a las lecturas que vienen después de
// un merge: es lo que hace falta para el único caso en que el estado del ítem
// cambia por la acción, que es el borrado de la rama (el ítem queda mergeado
// aunque el comando salga con error). Que sea posterior y no siempre es lo que
// mantiene honesta la relectura previa, que es la que decide si el merge sale.
func (f *FakeAdapter) ItemState(_ context.Context, ref model.RepoRef, number int) (model.Item, []model.Warning) {
	key := ref.Project + "#" + strconv.Itoa(number)
	it := f.ItemStates[key]
	f.mu.Lock()
	after := f.merged
	f.mu.Unlock()
	if after && f.OnMerge != nil {
		f.OnMerge(&it)
	}
	return it, f.StateWarnings[key]
}

// Comments devuelve la conversación configurada para "proyecto#número". Sin
// configurar devuelve una página vacía resuelta, que es el caso bueno: un test
// que no habla de comentarios no debería tener que configurarlos.
func (f *FakeAdapter) Comments(_ context.Context, ref model.RepoRef, number int) (forge.CommentPage, []model.Warning) {
	key := ref.Project + "#" + strconv.Itoa(number)
	f.mu.Lock()
	f.commentCalls++
	f.mu.Unlock()
	if page, ok := f.Conversations[key]; ok {
		return page, f.CommentWarnings[key]
	}
	return forge.CommentPage{}, f.CommentWarnings[key]
}

// CommentCallCount devuelve cuántas veces se pidieron comentarios.
func (f *FakeAdapter) CommentCallCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.commentCalls
}

// Approve devuelve los warnings configurados para la acción.
func (f *FakeAdapter) Approve(_ context.Context, ref model.RepoRef, number int) []model.Warning {
	return f.record("approve", ref, number)
}

// Merge devuelve los warnings configurados para la acción.
func (f *FakeAdapter) Merge(_ context.Context, ref model.RepoRef, number int, req forge.MergeRequest) []model.Warning {
	// El fake se niega a mergear sin pin, igual que los dos adapters reales, y lo
	// hace ANTES de registrar nada: un merge que no sale no pidió ninguna
	// estrategia. Un fake que aceptara lo que producción rechaza haría que
	// todos los tests de merge de la TUI cubrieran un camino que no existe, y el
	// fallo real se manifestaría en el adapter, donde ningún test llega.
	if req.HeadSHA == "" {
		return []model.Warning{{Forge: f.ForgeName, Kind: "unsupported", Msg: forge.ErrMissingHeadSHA.Error()}}
	}
	// El modo y el borrado se registran aparte para que un test pueda afirmar con
	// qué estrategia y con qué housekeeping se pidió el merge, no solo que se
	// pidió.
	f.mu.Lock()
	if f.MergeModes == nil {
		f.MergeModes = map[forge.MergeMode]int{}
	}
	f.MergeModes[req.Mode]++
	f.MergeDeletes = append(f.MergeDeletes, req.DeleteBranch)
	f.merged = true
	f.mu.Unlock()
	return f.record("merge", ref, number)
}

// Retarget cambia la rama destino del ítem.
//
// Se niega a hacerlo sin rama, igual que se niega a mergear sin pin, y lo hace
// ANTES de registrar nada: un retarget que no sale no pidió ninguna base. Un fake
// que aceptara lo que producción rechaza haría que los tests de la TUI cubrieran
// un camino que no existe.
func (f *FakeAdapter) Retarget(_ context.Context, ref model.RepoRef, number int, branch string) []model.Warning {
	if strings.TrimSpace(branch) == "" {
		return []model.Warning{{Forge: f.ForgeName, Kind: "unsupported", Msg: forge.ErrMissingBaseBranch.Error()}}
	}
	f.mu.Lock()
	f.Retargets = append(f.Retargets, branch)
	f.mu.Unlock()
	return f.record("retarget", ref, number)
}

// RetargetCount cuenta los cambios de base que salieron.
func (f *FakeAdapter) RetargetCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.Retargets)
}

// Branches devuelve la lista configurada para el proyecto, o una vacía.
func (f *FakeAdapter) Branches(_ context.Context, ref model.RepoRef) ([]string, []model.Warning) {
	f.mu.Lock()
	if f.branchCalls == nil {
		f.branchCalls = map[string]int{}
	}
	f.branchCalls[ref.Project]++
	f.mu.Unlock()
	if names, ok := f.BranchLists[ref.Project]; ok {
		return names, f.BranchWarnings[ref.Project]
	}
	return nil, f.BranchWarnings[ref.Project]
}

// BranchCallCount devuelve cuántas veces se pidieron las ramas de un proyecto.
func (f *FakeAdapter) BranchCallCount(project string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.branchCalls[project]
}

// MergeDeleteCount cuenta los merges que pidieron borrar la rama.
func (f *FakeAdapter) MergeDeleteCount(delete bool) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := 0
	for _, d := range f.MergeDeletes {
		if d == delete {
			n++
		}
	}
	return n
}

// MergeModeCount devuelve cuántas veces se pidió el merge con una estrategia.
func (f *FakeAdapter) MergeModeCount(mode forge.MergeMode) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.MergeModes[mode]
}

// record cuenta la acción y devuelve sus warnings: los tests pueden afirmar que
// una acción NO llegó a lanzarse.
func (f *FakeAdapter) record(kind string, ref model.RepoRef, number int) []model.Warning {
	key := kind + ":" + ref.Project + "#" + strconv.Itoa(number)
	f.mu.Lock()
	if f.actionCalls == nil {
		f.actionCalls = map[string]int{}
	}
	f.actionCalls[key]++
	f.mu.Unlock()
	return f.ActionWarnings[key]
}

// ActionCallCount devuelve cuántas veces se llamó a Approve/Merge sobre un ítem.
func (f *FakeAdapter) ActionCallCount(kind string, ref model.RepoRef, number int) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.actionCalls[kind+":"+ref.Project+"#"+strconv.Itoa(number)]
}

// ItemKey compone la clave de ItemState/acciones para un ítem.
func ItemKey(project string, number int) string {
	return project + "#" + strconv.Itoa(number)
}

// ConformanceOptions describe cómo debe comportarse un adapter en la suite.
type ConformanceOptions struct {
	Unsupported   bool // el adapter debe responder "unsupported" en todo
	MissingBinary bool // el CLI no existe: los listados deben avisar, no romper
}

// RunConformance ejecuta la suite de contrato compartida por todos los adapters y
// registra cada incumplimiento como fallo del test.
//
// Lo que hay aquí es solo elickness de imprimir: las comprobaciones viven en
// `ConformanceViolations`, que devuelve la lista y no toca el test. La razón es que una
// suite que solo se puede ejecutar con un `*testing.T` de verdad no se puede PROBAR —y una
// puerta que no se puede probar no distingue una puerta de un cartel—. Con la separación,
// los tests pueden darle un adapter roto y comprobar que la lista sale como tiene que
// salir, que es la única forma de saber que la puerta cierra.
func RunConformance(t testReport, a forge.Adapter, opts ConformanceOptions) {
	t.Helper()
	reportaIncumplimientos(a, opts, func(v string) { t.Error(v) })
}

// reportaIncumplimientos entrega cada incumplimiento a `informa`.
//
// Y la función de informe inyectada es lo que hace que este bucle sea comprobable en proceso:
// `t.Error` es un método de `*testing.T`, que no se puede doblear —tiene un método privado— y
// además no detiene la ejecución, así que probarlo "en verde" solo se puede haciendo que el test
// falle. Con el informe fuera, el bucle se comprueba contando lo que entrega y contando cuántas
// veces.
//
// Y `t.Error` y no `t.Fatalf` a propósito: la suite quiere REPORTAR todos los incumplimientos de
// una vez, porque un adapter roto suele romper más de una regla y ver solo la primera obliga a
// arreglar, correr, y volver a encontrar la siguiente.
func reportaIncumplimientos(a forge.Adapter, opts ConformanceOptions, informa func(string)) {
	for _, v := range ConformanceViolations(a, opts) {
		informa(v)
	}
}

// ConformanceViolations devuelve una línea por cada regla del contrato que el adapter
// incumple. Vacía significa conforme.
//
// Y el detalle de por qué una lista y no `error` es que hay varios incumplimientos
// independientes, y reportar solo el primero obliga a arreglar y volver a ejecutar para
// descubrir el siguiente. Con un adapter roto son cinco o seis.
func ConformanceViolations(a forge.Adapter, opts ConformanceOptions) []string {
	var fuera []string
	// Forge y host vacíos no son un incumplimiento menor: sin ellos el resto de los
	// mensajes de la app salen sin nombre, y los avisos de degradación no dicen de qué
	// forge hablan.
	if a.Forge() == "" {
		fuera = append(fuera, "Forge() vacío")
	}
	if a.Host() == "" {
		fuera = append(fuera, "Host() vacío")
	}
	ctx := context.Background()

	for _, q := range forge.Streams {
		page, warns := a.List(ctx, q)
		// `More` sin `Next` es la incoherencia que más caro sale: la TUI pide la página
		// siguiente con un cursor vacío, recibe la primera otra vez, y el inbox entra en
		// un bucle que consume red sin mostrar nada nuevo.
		if page.More && page.Next == "" {
			fuera = append(fuera, fmt.Sprintf("%s: List(%v) More=true sin Next", a.Forge(), q))
		}
		if opts.Unsupported {
			if !hasKind(warns, "unsupported") {
				fuera = append(fuera, fmt.Sprintf("%s: List(%v) debería reportar unsupported", a.Forge(), q))
			}
			if len(page.Items) != 0 {
				fuera = append(fuera, fmt.Sprintf("%s: List(%v) no debería devolver ítems", a.Forge(), q))
			}
		}
		if opts.MissingBinary {
			if len(page.Items) != 0 {
				fuera = append(fuera, fmt.Sprintf("%s: List(%v) sin binario no debería devolver ítems", a.Forge(), q))
			}
			// Sin aviso, el inbox aparece vacío sin explicación. Es peor que un error
			// visible: el usuario ve que no tiene trabajo y no tiene por qué.
			if len(warns) == 0 {
				fuera = append(fuera, fmt.Sprintf("%s: List(%v) sin binario debería devolver un warning", a.Forge(), q))
			}
		}
	}

	// Las cinco acciones, con el mismo criterio: un adapter inerte lo dice en todo. Y son
	// las que más se olvidan en una suite —una que solo mirara los listados dejaría sin
	// comprobar approve, merge, retarget y branches, que son las que modifican algo—.
	ref := model.RepoRef{Forge: a.Forge(), Host: a.Host(), Project: "o/r", Owner: "o", Name: "r"}
	if _, warns := a.ItemState(ctx, ref, 1); opts.Unsupported && !hasKind(warns, "unsupported") {
		fuera = append(fuera, fmt.Sprintf("%s: ItemState debería reportar unsupported", a.Forge()))
	}
	if _, warns := a.Comments(ctx, ref, 1); opts.Unsupported && !hasKind(warns, "unsupported") {
		fuera = append(fuera, fmt.Sprintf("%s: Comments debería reportar unsupported", a.Forge()))
	}
	if warns := a.Approve(ctx, ref, 1); opts.Unsupported && !hasKind(warns, "unsupported") {
		fuera = append(fuera, fmt.Sprintf("%s: Approve debería reportar unsupported", a.Forge()))
	}
	if warns := a.Merge(ctx, ref, 1, forge.MergeRequest{Mode: forge.Squash, HeadSHA: "deadbeef"}); opts.Unsupported && !hasKind(warns, "unsupported") {
		fuera = append(fuera, fmt.Sprintf("%s: Merge debería reportar unsupported", a.Forge()))
	}
	if warns := a.Retarget(ctx, ref, 1, "release/2.0"); opts.Unsupported && !hasKind(warns, "unsupported") {
		fuera = append(fuera, fmt.Sprintf("%s: Retarget debería reportar unsupported", a.Forge()))
	}
	// El buscador se abre aunque el listado venga vacío: por eso lo que se comprueba es
	// el aviso y que la lista no tenga ramas. Un adapter que devolviera la lista vacía
	// sin decir nada haría que el buscador pareciera un repo sin ramas, que es un
	// diagnóstico distinto del correcto.
	//
	// Y la asimetría con el listado es deliberada: para un listado, "no puedo" quiere
	// decir lista vacía; para el buscador, quiere decir "avisa y abre".
	if names, warns := a.Branches(ctx, ref); opts.Unsupported && !hasKind(warns, "unsupported") {
		fuera = append(fuera, fmt.Sprintf("%s: Branches debería reportar unsupported", a.Forge()))
	} else if len(names) != 0 {
		fuera = append(fuera, fmt.Sprintf("%s: Branches no debería devolver ramas (%v)", a.Forge(), names))
	}
	return fuera
}

func hasKind(warns []model.Warning, kind string) bool {
	for _, w := range warns {
		if w.Kind == kind {
			return true
		}
	}
	return false
}
