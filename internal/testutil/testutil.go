// Package testutil ofrece dobles en memoria de los contratos de prdash para
// tests (sin red, sin subproceso, sin disco) y una suite de conformidad
// compartida que todo adapter debe pasar.
package testutil

import (
	"context"
	"strconv"
	"sync"
	"testing"

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

	// ActionWarnings responde a Approve/Merge por "approve:proyecto#número".
	ActionWarnings map[string][]model.Warning

	mu          sync.Mutex
	calls       map[FakeKey]int
	listCalls   int
	actionCalls map[string]int
	// commentCalls cuenta cuántas veces se pidieron comentarios, para que un test
	// pueda afirmar que la ficha cachea y no repregunta en cada render.
	commentCalls int
	// MergeModes cuenta quantas veces se pidió cada estrategia de merge.
	MergeModes map[forge.MergeMode]int
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

// RunConformance ejecuta la suite de contrato compartida por todos los
// adapters. Con MissingBinary usa un binario inexistente, de modo que no hay
// ninguna llamada de red; con Unsupported comprueba el adapter inerte.
func RunConformance(t *testing.T, a forge.Adapter, opts ConformanceOptions) {
	t.Helper()
	if a.Forge() == "" {
		t.Error("Forge() vacío")
	}
	if a.Host() == "" {
		t.Error("Host() vacío")
	}
	ctx := context.Background()

	for _, q := range forge.Streams {
		page, warns := a.List(ctx, q)
		if page.More && page.Next == "" {
			t.Errorf("%s: List(%v) More=true sin Next", a.Forge(), q)
		}
		if opts.Unsupported {
			if !hasKind(warns, "unsupported") {
				t.Errorf("%s: List(%v) debería reportar unsupported", a.Forge(), q)
			}
			if len(page.Items) != 0 {
				t.Errorf("%s: List(%v) no debería devolver ítems", a.Forge(), q)
			}
		}
		if opts.MissingBinary {
			if len(page.Items) != 0 {
				t.Errorf("%s: List(%v) sin binario no debería devolver ítems", a.Forge(), q)
			}
			if len(warns) == 0 {
				t.Errorf("%s: List(%v) sin binario debería devolver un warning", a.Forge(), q)
			}
		}
	}

	ref := model.RepoRef{Forge: a.Forge(), Host: a.Host(), Project: "o/r", Owner: "o", Name: "r"}
	if _, warns := a.ItemState(ctx, ref, 1); opts.Unsupported && !hasKind(warns, "unsupported") {
		t.Errorf("%s: ItemState debería reportar unsupported", a.Forge())
	}
	if _, warns := a.Comments(ctx, ref, 1); opts.Unsupported && !hasKind(warns, "unsupported") {
		t.Errorf("%s: Comments debería reportar unsupported", a.Forge())
	}
	if warns := a.Approve(ctx, ref, 1); opts.Unsupported && !hasKind(warns, "unsupported") {
		t.Errorf("%s: Approve debería reportar unsupported", a.Forge())
	}
	if warns := a.Merge(ctx, ref, 1, forge.MergeRequest{Mode: forge.Squash, HeadSHA: "deadbeef"}); opts.Unsupported && !hasKind(warns, "unsupported") {
		t.Errorf("%s: Merge debería reportar unsupported", a.Forge())
	}
}

func hasKind(warns []model.Warning, kind string) bool {
	for _, w := range warns {
		if w.Kind == kind {
			return true
		}
	}
	return false
}
