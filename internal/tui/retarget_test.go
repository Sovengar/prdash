package tui

import (
	"strings"
	"testing"
	"time"

	"prdash/internal/forge"
	"prdash/internal/forge/model"
	"prdash/internal/testutil"
	"prdash/internal/worktree"
)

func retargetFixture(t *testing.T, branches ...string) (Model, *testutil.FakeAdapter) {
	t.Helper()
	m, a := openRetargetFixture(t, branches...)
	// The listing is awaited from the channel rather than injected, because the goroutine is the point.
	return send(t, m, waitMount(t, m).(branchesMsg)), a
}

// openRetargetFixture leaves the popup open with the listing not yet arrived.
func openRetargetFixture(t *testing.T, branches ...string) (Model, *testutil.FakeAdapter) {
	t.Helper()
	a := ghAdapter()
	a.BranchLists = map[string][]string{"acme/widget": branches}
	it := mkItem("github", "github.com", "acme/widget", "Add widget", 7, "REVIEW_REQUIRED")
	a.ItemStates = map[string]model.Item{stateKey(it): it}

	m := newTestModel(t, a)
	m = send(t, m, page(1, "github", "github.com", model.SectionReview, model.ReviewRequested,
		[]model.Item{it}, false))

	return press(t, m, "e"), a
}

// The filter is typed letter by letter, as the terminal would.
func pressFilter(t *testing.T, m Model, word string) Model {
	t.Helper()
	for _, r := range word {
		m = press(t, m, string(r))
	}
	return m
}

// `e` opens the popup and what appears are the repository's branches.
func TestEAbreElBuscadorYTraeLasRamasDelForge(t *testing.T) {
	m, a := retargetFixture(t, "main", "develop", "release/2.0")

	if m.retarget.state != retargetChoosing {
		t.Fatalf("estado del popup = %v, want el buscador", m.retarget.state)
	}
	if a.BranchCallCount("acme/widget") != 1 {
		t.Errorf("Branches = %d llamadas, want 1", a.BranchCallCount("acme/widget"))
	}
	if m.retarget.view[0] != "main" {
		t.Errorf("la primera fila es %q, want la base actual", m.retarget.view[0])
	}
	if got := strings.Join(m.retarget.view, ","); got != "main,develop,release/2.0" {
		t.Errorf("view = %q, want las tres ramas", got)
	}
}

// The filter runs on the whole name, not the last segment.
func TestElFiltroDejaLasRamasQueLoContienen(t *testing.T) {
	m, _ := retargetFixture(t, "main", "fix/hunk-pane-argv", "release/2.0")

	m = pressFilter(t, m, "hunk")
	if got := strings.Join(m.retarget.view, ","); got != "fix/hunk-pane-argv" {
		t.Errorf("view = %q, want solo la rama que contiene el filtro", got)
	}

	m = press(t, m, "backspace")
	if m.retarget.query != "hun" {
		t.Errorf("query = %q, want que backspace quite un carácter", m.retarget.query)
	}
	m = press(t, m, "ctrl+u")
	if m.retarget.query != "" {
		t.Errorf("query = %q, want que ctrl+u la borre entera", m.retarget.query)
	}

	m = pressFilter(t, m, "HUNK")
	if got := strings.Join(m.retarget.view, ","); got != "fix/hunk-pane-argv" {
		t.Errorf("view = %q, want el filtro sin distinguir mayúsculas", got)
	}
}

// What stops the two halves stepping on each other.
func TestJYKConFiltroEscribeYNavega(t *testing.T) {
	m, _ := retargetFixture(t, "main", "develop", "release/2.0", "fix/jj-one")

	m = press(t, m, "j")
	if m.retarget.cursor != 1 {
		t.Fatalf("con el filtro vacío, `j` no movió el cursor (cursor=%d)", m.retarget.cursor)
	}

	// As soon as the filter has something, `j` types: otherwise `fix/jj-one` is impossible to
	// write.
	m = pressFilter(t, m, "re")
	m = press(t, m, "j")
	if m.retarget.query != "rej" {
		t.Errorf("query = %q, want que `j` escriba con el filtro activo", m.retarget.query)
	}
	if m.retarget.cursor != 0 {
		t.Errorf("cursor = %d, want vuelta arriba en cuanto cambia el filtro", m.retarget.cursor)
	}

	m = press(t, m, "ctrl+u")
	m = press(t, m, "down")
	if m.retarget.cursor != 1 {
		t.Errorf("cursor = %d, want que `down` mueva con el filtro vacío", m.retarget.cursor)
	}
}

func TestElegirLaBaseQueYaTieneNoGastaLlamada(t *testing.T) {
	m, a := retargetFixture(t, "main", "develop")

	m = press(t, m, "enter") // main es la base actual y sale la primera
	if m.retarget.state != retargetClosed {
		t.Errorf("estado = %v, want el popup cerrado", m.retarget.state)
	}
	if a.RetargetCount() != 0 {
		t.Errorf("llamó al forge (%v) para no cambiar nada", a.Retargets)
	}
	assertToast(t, m, "already main")
}

// The second press is the confirmation.
func TestElCambioSeConfirmaAntesDeSalir(t *testing.T) {
	m, a := retargetFixture(t, "main", "release/2.0")

	m = press(t, m, "down")
	m = press(t, m, "enter")
	if m.retarget.state != retargetConfirm {
		t.Fatalf("estado = %v, want la confirmación", m.retarget.state)
	}
	if a.RetargetCount() != 0 {
		t.Fatalf("la confirmación ya llamó al forge (%v)", a.Retargets)
	}

	box := stripANSI(m.retargetOverlay2())
	for _, want := range []string{"main", "→", "release/2.0", "enter apply"} {
		if !strings.Contains(box, want) {
			t.Errorf("la confirmación no dice %q:\n%s", want, box)
		}
	}

	m = press(t, m, "enter")
	out := waitOutcome(t, m)
	if !out.OK {
		t.Errorf("la acción salió = %+v, want OK", out)
	}
	if out.Base != "release/2.0" {
		t.Errorf("Outcome.Base = %q, want la rama aplicada para el aviso", out.Base)
	}
	if a.RetargetCount() != 1 || a.Retargets[0] != "release/2.0" {
		t.Errorf("el adapter recibió %v, want release/2.0", a.Retargets)
	}
	if m.retarget.state != retargetClosed {
		t.Errorf("el popup sigue abierto tras aplicar")
	}
	if !m.actionBusy {
		t.Error("la acción no quedó marcada como en curso")
	}
}

func (m Model) retargetOverlay2() string {
	box, _ := m.retargetOverlay()
	return box
}

// The likeliest mistake when confirming is picking the wrong row.
func TestEscEnLaConfirmacionVuelveALaLista(t *testing.T) {
	m, a := retargetFixture(t, "main", "release/2.0")

	m = press(t, m, "down")
	m = press(t, m, "enter")
	m = press(t, m, "esc")

	if m.retarget.state != retargetChoosing {
		t.Errorf("estado = %v, want volver al buscador", m.retarget.state)
	}
	if a.BranchCallCount("acme/widget") != 1 {
		t.Errorf("volvió a pedir las ramas: %d llamadas", a.BranchCallCount("acme/widget"))
	}
}

// Open, it takes the whole keyboard: a key that leaked through would have `j` approve the
// PR.
func TestElPopupNoDejaPasarLasTeclasALaVista(t *testing.T) {
	m, a := retargetFixture(t, "main", "release/2.0")

	m = press(t, m, "a")
	if a.ActionCallCount("approve", mkRef(), 7) != 0 {
		t.Error("una tecla del popup disparó un approve en la vista")
	}
	if m.retarget.state != retargetChoosing {
		t.Errorf("estado = %v, want el popup abierto", m.retarget.state)
	}
}

func TestElListadoSeCacheaPorRepositorio(t *testing.T) {
	m, a := retargetFixture(t, "main", "release/2.0")
	if a.BranchCallCount("acme/widget") != 1 {
		t.Fatalf("primera apertura = %d llamadas, want 1", a.BranchCallCount("acme/widget"))
	}

	m = press(t, m, "esc")
	m = press(t, m, "e")
	if m.retarget.state != retargetChoosing {
		t.Fatalf("estado = %v, want el buscador abierto al instante desde el caché", m.retarget.state)
	}
	if a.BranchCallCount("acme/widget") != 1 {
		t.Errorf("la segunda apertura vuelva a pedir las ramas: %d llamadas", a.BranchCallCount("acme/widget"))
	}
}

// A branch created a minute ago has to appear, or the picker lies.
func TestElCacheCaduca(t *testing.T) {
	m, a := retargetFixture(t, "main")
	key := keyOf(m.retargetItemForTest())
	m.branchCache[key] = branchCache{names: []string{"main"}, fetchedAt: time.Now().Add(-branchCacheTTL - time.Second)}

	m = press(t, m, "esc")
	m = press(t, m, "e")
	if m.retarget.state != retargetListing {
		t.Errorf("estado = %v, want que vuelva a preguntar por un caché caducado", m.retarget.state)
	}
	waitMount(t, m) // el segundo pedido sale solo, como el primero
	if a.BranchCallCount("acme/widget") != 2 {
		t.Errorf("llamadas = %d, want 2: un caché caducado se vuelve a pedir", a.BranchCallCount("acme/widget"))
	}
}

func TestUnListadoIlegibleSeExplica(t *testing.T) {
	a := ghAdapter()
	it := mkItem("github", "github.com", "acme/widget", "Add widget", 7, "REVIEW_REQUIRED")
	m := newTestModel(t, a)
	m = send(t, m, page(1, "github", "github.com", model.SectionReview, model.ReviewRequested,
		[]model.Item{it}, false))
	m = press(t, m, "e")

	m = send(t, m, branchesMsg{seq: m.branchSeq, errMsg: "gh: Not Found (HTTP 404)"})

	box := stripANSI(m.retargetOverlay2())
	if !strings.Contains(box, "404") {
		t.Errorf("el popup no explica el fallo del forge:\n%s", box)
	}
	if m.retarget.state != retargetListing {
		t.Errorf("estado = %v, want quedarse esperando con el motivo a la vista", m.retarget.state)
	}
}

func TestElPopupAplicaSobreElItemQueSeConfirmo(t *testing.T) {
	m, a := retargetFixture(t, "main", "release/2.0")
	other := mkItem("github", "github.com", "acme/otro", "Otro PR", 9, "REVIEW_REQUIRED")
	m = send(t, m, page(1, "github", "github.com", model.SectionReview, model.ReviewRequested,
		[]model.Item{other}, false))
	if !m.selectedIs(other) {
		t.Fatalf("el cursor no está sobre el otro ítem: el test no probaría nada")
	}

	m = press(t, m, "down") // el popup sigue con su teclado
	m = press(t, m, "enter")
	m = press(t, m, "enter")
	waitOutcome(t, m)

	if a.RetargetCount() != 1 || a.Retargets[0] != "release/2.0" {
		t.Errorf("el adapter recibió %v, want el cambio del ítem confirmado", a.Retargets)
	}
	if n := a.ActionCallCount("retarget", mkRef(), 7); n != 1 {
		t.Errorf("retarget sobre acme/widget = %d, want 1 (el ítem del popup, no el del cursor)", n)
	}
}

func TestUnListadoTardioNoSePinta(t *testing.T) {
	m, _ := retargetFixture(t, "main", "release/2.0")
	m = press(t, m, "esc") // cerrado: el pedido en vuelo queda obsoleto

	m = send(t, m, branchesMsg{seq: m.branchSeq, names: []string{"otra/cosa"}})

	if m.retarget.state != retargetClosed {
		t.Errorf("un listado tardío reabrió el popup: estado = %v", m.retarget.state)
	}
}

func TestElAvisoDiceDeQueBaseAQue(t *testing.T) {
	m, a := retargetFixture(t, "main", "release/2.0")
	m, out := applyBranchAndMove(t, m, a)
	it, _ := m.selected()
	it.TargetBranch = out.Base

	m = applyActionResult(t, m, forge.Outcome{
		Kind: forge.ActionRetarget, OK: true, HasItem: true, Item: it,
		Base: out.Base, FromBase: "main",
	})
	if !strings.Contains(lastToast(m), "main") || !strings.Contains(lastToast(m), "release/2.0") {
		t.Errorf("el aviso = %q, want las dos ramas", lastToast(m))
	}
}

// Changing the base does not touch the worktree.
func TestElAvisoAvisaDelWorktreeDesfasado(t *testing.T) {
	m, a := retargetFixture(t, "main", "release/2.0")
	m.SetReviewLookup(fakeLookup{path: "/tmp/wt/prdash-pr-7"})

	m, out := applyBranchAndMove(t, m, a)
	it, _ := m.selected()
	m = applyActionResult(t, m, forge.Outcome{
		Kind: forge.ActionRetarget, OK: true, HasItem: true, Item: it,
		Base: out.Base, FromBase: "main",
	})

	if !strings.Contains(lastToast(m), "mounted review") {
		t.Errorf("el aviso = %q, want que nombre el review desfasado", lastToast(m))
	}
}

// The warning is for the real case, not decoration on every item.
func TestSinReviewMontadoNoHayQueAvisar(t *testing.T) {
	m, a := retargetFixture(t, "main", "release/2.0")
	m.SetReviewLookup(fakeLookup{}) // hay registro, pero no review montado

	m, out := applyBranchAndMove(t, m, a)
	it, _ := m.selected()
	m = applyActionResult(t, m, forge.Outcome{
		Kind: forge.ActionRetarget, OK: true, HasItem: true, Item: it,
		Base: out.Base, FromBase: "main",
	})

	if strings.Contains(lastToast(m), "mounted review") {
		t.Errorf("el aviso = %q, want que no hable de un review que no existe", lastToast(m))
	}
}

// Without the port injected the action works and only the warning is lost.
func TestSinRegistroDeReviewsNoHayQueAvisar(t *testing.T) {
	m, a := retargetFixture(t, "main", "release/2.0")
	if m.reviewLookup != nil {
		t.Fatal("el test necesita el modelo sin registro: New no lo inyecta")
	}
	m, out := applyBranchAndMove(t, m, a)
	it, _ := m.selected()
	m = applyActionResult(t, m, forge.Outcome{
		Kind: forge.ActionRetarget, OK: true, HasItem: true, Item: it,
		Base: out.Base, FromBase: "main",
	})

	if !strings.Contains(lastToast(m), "release/2.0") {
		t.Errorf("el aviso = %q, want el resultado del cambio", lastToast(m))
	}
}

// The base only changes on an item that is still something to integrate.
func TestUnItemNoAccionableNoAbreElPopup(t *testing.T) {
	a := ghAdapter()
	it := mkItem("github", "github.com", "acme/widget", "Add widget", 7, "APPROVED")
	it.State = "MERGED"
	m := newTestModel(t, a)
	m = send(t, m, page(1, "github", "github.com", model.SectionReview, model.ReviewRequested,
		[]model.Item{it}, false))

	m = press(t, m, "e")

	if m.retarget.state != retargetClosed {
		t.Errorf("estado = %v, want que no abra sobre un ítem mergeado", m.retarget.state)
	}
	if a.BranchCallCount("acme/widget") != 0 {
		t.Error("pidió las ramas de un ítem que no es accionable")
	}
	assertToast(t, m, "already merged")
}

func TestElPopupNoSeAbreConElMergeArmado(t *testing.T) {
	m, a := retargetFixture(t, "main", "release/2.0")
	m = press(t, m, "esc")

	m = press(t, m, "m") // arma el merge
	if !m.mergeArmed {
		t.Fatal("el merge no se armó: el test no probaría nada")
	}
	m = press(t, m, "e")

	if m.retarget.state != retargetClosed {
		t.Errorf("estado = %v, want que el popup no se abra con el merge armado", m.retarget.state)
	}
	if a.BranchCallCount("acme/widget") != 1 {
		t.Errorf("la `e` llegó a pedir ramas: %d llamadas", a.BranchCallCount("acme/widget"))
	}
}

func TestLaTeclaSePuedeRebind(t *testing.T) {
	a := ghAdapter()
	a.BranchLists = map[string][]string{"acme/widget": {"main", "develop"}}
	m := newTestModel(t, a)
	m.cfg.Keybindings["retarget"] = "T"
	m = send(t, m, page(1, "github", "github.com", model.SectionReview, model.ReviewRequested,
		[]model.Item{mkItem("github", "github.com", "acme/widget", "Add widget", 7, "REVIEW_REQUIRED")}, false))

	m = press(t, m, "T")
	m = send(t, m, waitMount(t, m).(branchesMsg))

	if m.retarget.state != retargetChoosing {
		t.Errorf("estado = %v, want que el rebind abra el buscador", m.retarget.state)
	}
}

// The reason has to be the forge's, not the CLI's line.
func TestUnRechazoDelForgeSeEnsenaSinElArgv(t *testing.T) {
	m, a := retargetFixture(t, "main", "release/2.0")
	a.ActionWarnings = map[string][]model.Warning{
		"retarget:" + stateKey(m.retarget.item): {{
			Forge: "github", Kind: "validation",
			Msg: "Proposed base branch 'release/2.0' was not found",
		}},
	}

	m, out := applyBranchAndMove(t, m, a)
	m = applyActionResult(t, m, out)

	if out.OK {
		t.Error("un rechazo del forge no puede salir como OK")
	}
	if out.Conflict {
		t.Errorf("salió como conflicto: un refresco no arregla el nombre de la rama")
	}
	if strings.Contains(lastToast(m), "-X PATCH") {
		t.Errorf("el aviso = %q, want el motivo del forge y no el argv", lastToast(m))
	}
	if !strings.Contains(lastToast(m), "was not found") {
		t.Errorf("el aviso = %q, want el motivo del forge", lastToast(m))
	}
}

type fakeLookup struct{ path string }

func (f fakeLookup) ActiveReview(model.Item) (worktree.Worktree, bool) {
	if f.path == "" {
		return worktree.Worktree{}, false
	}
	return worktree.Worktree{Path: f.path}, true
}

func mkRef() model.RepoRef {
	return model.RepoRef{Forge: "github", Host: "github.com", Project: "acme/widget", Owner: "acme", Name: "widget"}
}

func applyBranchAndMove(t *testing.T, m Model, a *testutil.FakeAdapter) (Model, forge.Outcome) {
	t.Helper()
	m = press(t, m, "down")
	m = press(t, m, "enter")
	m = press(t, m, "enter")
	out := waitOutcome(t, m)
	if a.RetargetCount() != 1 {
		t.Fatalf("el popup no llegó a aplicar: %v", a.Retargets)
	}
	return m, out
}

func applyActionResult(t *testing.T, m Model, out forge.Outcome) Model {
	t.Helper()
	return send(t, m, actionMsg{cycle: m.cycle, outcome: out})
}

func (m Model) selectedIs(it model.Item) bool {
	got, ok := m.selected()
	return ok && got.ID() == it.ID()
}

func (m Model) retargetItemForTest() model.Item {
	it, _ := m.selected()
	return it
}
