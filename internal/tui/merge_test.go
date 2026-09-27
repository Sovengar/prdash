// Tests de la doble confirmación de merge: la primera pulsación solo arma y la
// segunda es la que elige el modo y ejecuta. Un merge reescribe historia y no se
// deshace con un comando, así que lo que más importa es que no exista ningún
// camino que lo dispare con una estrategia que el usuario no ha nombrado.
package tui

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"prdash/internal/forge"
	"prdash/internal/forge/model"
	"prdash/internal/testutil"
	"prdash/internal/worktree"
)

// mergeFixture es un modelo con un ítem seleccionable, accionable y releíble por
// el adapter. Los tres hacen falta: sin selección no hay nada que armar, sin
// releer el forge RunAction aborta antes de llegar a Merge, y sin releer no se
// puede comprobar que el modo llegó a la CLI.
type mergeFixture struct {
	m     Model
	adp   *testutil.FakeAdapter
	items []model.Item
}

func mergeItems() []model.Item {
	return []model.Item{
		mkItem("github", "github.com", "acme/widget", "Add widget", 1, ""),
		mkItem("github", "github.com", "acme/widget", "Otro cambio", 2, ""),
	}
}

// newMergeFixture monta el modelo con los ítems dados ya presentes en la sección
// de review y registrados en el adapter para que ItemState los devuelva.
func newMergeFixture(t *testing.T, items ...model.Item) mergeFixture {
	t.Helper()
	adp := ghAdapter()
	adp.ItemStates = map[string]model.Item{}
	adp.StateWarnings = map[string][]model.Warning{}
	for _, it := range items {
		adp.ItemStates[stateKey(it)] = it
	}
	m := newTestModel(t, adp)
	m = send(t, m, page(1, "github", "github.com", model.SectionReview, model.ReviewRequested, items, false))
	return mergeFixture{m: m, adp: adp, items: items}
}

// stateKey es la clave con la que FakeAdapter indexa ItemState.
func stateKey(it model.Item) string {
	return it.Ref.Project + "#" + strconv.Itoa(it.Number)
}

// waitOutcome lee el resultado de la acción del canal de eventos.
func waitOutcome(t *testing.T, m Model) forge.Outcome {
	t.Helper()
	ev := waitMount(t, m)
	msg, ok := ev.(actionMsg)
	if !ok {
		t.Fatalf("evento %T, want actionMsg", ev)
	}
	return msg.outcome
}

// toasts aplana los avisos vivos a un string, para afirmar sobre el texto sin
// depender de cuántas vistas hay apiladas.
func toasts(m Model) string { return strings.Join(toastTexts(m), " | ") }

// TestMergeArmsOnFirstPress: la primera pulsación no ejecuta nada. Es el test que
// sostiene la doble verificación: si el merge saliera aquí, la segunda tecla no
// sería una confirmación sino un adorno.
func TestMergeArmsOnFirstPress(t *testing.T) {
	f := newMergeFixture(t, mergeItems()...)
	// El cursor no tiene por qué estar en el primer ítem de la página: las filas
	// se ordenan, así que se lee lo que hay seleccionado en vez de suponerlo.
	armed, ok := f.m.selected()
	if !ok {
		t.Fatal("el fixture necesita una selección")
	}

	m := press(t, f.m, "m")
	if !m.mergeArmed {
		t.Fatal("la primera pulsación debería armar el merge")
	}
	if m.actionBusy {
		t.Error("armar no debería lanzar la acción todavía")
	}
	if m.mergeArmedID != armed.ID() {
		t.Errorf("el armado fijó %+v, want %+v", m.mergeArmedID, armed.ID())
	}
}

// TestMergeArmedShowsConfirmInKeybinds: la confirmación sustituye a la barra de
// atajos. Es la caja siempre visible, no un toast, porque una confirmación a la
// que se contesta después de mirar a otro lado tiene que seguir ahí.
func TestMergeArmedShowsConfirmInKeybinds(t *testing.T) {
	f := newMergeFixture(t, mergeItems()...)
	m := press(t, f.m, "m")

	view := stripANSI(m.View().Content)
	for _, want := range []string{"press the mode", "m merge commit", "r rebase", "s squash", "esc cancel"} {
		if !strings.Contains(view, want) {
			t.Errorf("la confirmación no menciona %q\n%s", want, view)
		}
	}
	// La barra normal no debe seguir compitiendo con la confirmación.
	if strings.Contains(view, "pgup/dn page") {
		t.Errorf("con el merge armado la barra de atajos debería ceder\n%s", view)
	}
}

// TestMergeSecondKeyPicksMode es el núcleo: cada segunda tecla ejecuta con SU modo,
// y no hay modo por defecto.
func TestMergeSecondKeyPicksMode(t *testing.T) {
	for _, tc := range []struct {
		key  string
		want forge.MergeMode
	}{
		{"m", forge.MergeCommit},
		{"r", forge.Rebase},
		{"s", forge.Squash},
	} {
		t.Run(string(tc.want), func(t *testing.T) {
			f := newMergeFixture(t, mergeItems()...)
			m := press(t, f.m, "m")
			m = press(t, m, tc.key)

			if m.mergeArmed {
				t.Error("tras la segunda pulsación no debería quedar armado")
			}
			out := waitOutcome(t, m)
			if !out.OK {
				t.Fatalf("merge %s no ok: %+v", tc.want, out)
			}
			if out.Mode != tc.want {
				t.Errorf("modo = %q, want %q", out.Mode, tc.want)
			}
			if got := f.adp.MergeModeCount(tc.want); got != 1 {
				t.Errorf("el forge recibió %d merge(s) como %q, want 1", got, tc.want)
			}
		})
	}
}

// TestMergeSecondKeyOnlyPicksItsOwnMode: la segunda tecla de un modo no dispara
// los otros dos. Sin esto, `ms` podría llegar a hacer un rebase.
func TestMergeSecondKeyOnlyPicksItsOwnMode(t *testing.T) {
	f := newMergeFixture(t, mergeItems()...)
	m := press(t, f.m, "m")
	m = press(t, m, "s")
	_ = waitOutcome(t, m)

	for _, other := range []forge.MergeMode{forge.MergeCommit, forge.Rebase} {
		if n := f.adp.MergeModeCount(other); n != 0 {
			t.Errorf("squash lanzó %d merge(s) como %q, want 0", n, other)
		}
	}
}

// TestMergeArmedConsumesOtherKey: unarmed y ADEMÁS se come la tecla.
//
// Antes se delegaba en handleKey, y eso convertía un merge mal armado en una
// acción distinta: `m` y luego `a` aprobaba el PR y `m` y luego `m` mergeaba con
// merge commit sin haber pasado por la confirmación. El motivo original del
// default —que un `m` a destiempo no dejara la vista esperando— se cumple igual
// sin re-despachar: la vista deja de esperar, y la tecla que no era un modo no
// hace nada. Consumirla es la única forma de que el gesto de confirmar no pueda
// terminar en una acción que el usuario no pidió.
func TestMergeArmedConsumesOtherKey(t *testing.T) {
	f := newMergeFixture(t, mergeItems()...)
	start := f.m.cursor
	m := press(t, f.m, "m")

	m = press(t, m, "down")
	if m.mergeArmed {
		t.Error("una tecla de navegación debería desarmar el merge")
	}
	if m.cursor != start {
		t.Errorf("cursor = %d, want %d: la tecla filtrada no debe re-despacharse", m.cursor, start)
	}
	if m.actionBusy {
		t.Error("desarmar no debería dejar una acción en curso")
	}
}

// TestMergeArmedApproveKeyDoesNotApprove blinda el agujero concreto: la tecla de
// approve es la más cercana a `m` en el teclado y con el fallback anterior
// aprobaba el PR. Aquí afirma que no hay ninguna acción registrada.
func TestMergeArmedApproveKeyDoesNotApprove(t *testing.T) {
	f := newMergeFixture(t, mergeItems()...)
	m := press(t, f.m, "m")

	m = press(t, m, "a")
	if m.actionBusy {
		t.Error("`a` con el merge armado no debería lanzar ninguna acción")
	}
	if n := f.adp.MergeModeCount(forge.Squash) + f.adp.MergeModeCount(forge.Rebase) +
		f.adp.MergeModeCount(forge.MergeCommit); n != 0 {
		t.Errorf("`a` con el merge armado lanzó %d merge(s), want 0", n)
	}
}

// TestMergeEscCancels: esc cancela y lo dice, y q sigue cerrando la TUI. Son dos
// intenciones distintas —descartar y salir— y colapsarlas haría que q dejara de
// cerrar la aplicación.
func TestMergeEscCancels(t *testing.T) {
	f := newMergeFixture(t, mergeItems()...)
	m := press(t, f.m, "m")

	m = press(t, m, "esc")
	if m.mergeArmed {
		t.Error("esc debería cancelar el merge")
	}
	if m.actionBusy {
		t.Error("cancelar no debería dejar una acción en curso")
	}
	if !strings.Contains(toasts(m), "cancelled") {
		t.Errorf("cancelar debería decirlo, avisos = %q", toasts(m))
	}
}

func TestMergeQuittingStillQuits(t *testing.T) {
	f := newMergeFixture(t, mergeItems()...)
	m := press(t, f.m, "m")

	out, _ := m.Update(tea.KeyPressMsg{Code: 'q', Text: "q"})
	if out == nil {
		t.Error("q debería seguir cerrando la TUI con el merge armado")
	}
}

// TestMergeArmedGuardsBeforeArming: armar un merge que ya se sabe inválido solo
// genera una confirmación que no puede terminar bien, así que los guards corren
// antes de armar.
func TestMergeArmedGuardsBeforeArming(t *testing.T) {
	m := newTestModel(t, ghAdapter()) // sin selección

	m = press(t, m, "m")
	if m.mergeArmed {
		t.Error("sin selección no debería armar")
	}
	if !strings.Contains(toasts(m), "select an item") {
		t.Errorf("debería avisar de que falta selección, avisos = %q", toasts(m))
	}
}

// TestMergeArmedRefusesAlreadyDenied: un merge que ya volvió denegado por el
// forge no se arma. Armarlo y solo avisar al confirmar sería hacer esperar al
// usuario por un error que ya se conoce.
func TestMergeArmedRefusesAlreadyDenied(t *testing.T) {
	f := newMergeFixture(t, mergeItems()...)
	m := f.m
	sel, ok := m.selected()
	if !ok {
		t.Fatal("el fixture necesita una selección")
	}
	m.denied[sel.ID()] = "the branch is protected"

	m = press(t, m, "m")
	if m.mergeArmed {
		t.Error("un merge ya denegado no debería armar")
	}
	if !strings.Contains(toasts(m), "protected") {
		t.Errorf("debería explicar la denegación, avisos = %q", toasts(m))
	}
}

// TestMergeOnChangedItemAborts: un refresco puede recolocar el cursor entre el
// armado y la confirmación. El merge sale sobre lo que el usuario confirmó o no
// sale; nunca sobre lo que ahora esté debajo del cursor.
func TestMergeOnChangedItemAborts(t *testing.T) {
	f := newMergeFixture(t, mergeItems()...)
	m := press(t, f.m, "m")
	armed, _ := m.selected()

	// Lo que hace un ciclo de refresco: la selección se mueve. Se mueve el
	// cursor directamente porque lo que se prueba es el guard, no la cadena de
	// refresco que reposiciona.
	m.cursor = (m.cursor + 1) % len(m.rows())
	if moved, _ := m.selected(); moved.ID() == armed.ID() {
		t.Skip("el fixture no tiene un segundo ítem al que moverse")
	}

	m = press(t, m, "m")
	if m.actionBusy {
		t.Error("no debería mergear un ítem que ya no está donde se armó")
	}
	if m.mergeArmed {
		t.Error("debería desarmar tras detectar el cambio")
	}
	if !strings.Contains(toasts(m), "changed") {
		t.Errorf("debería explicar el cambio, avisos = %q", toasts(m))
	}
	total := f.adp.MergeModeCount(forge.MergeCommit) +
		f.adp.MergeModeCount(forge.Squash) + f.adp.MergeModeCount(forge.Rebase)
	if total != 0 {
		t.Errorf("no debería haber llegado ningún merge al forge, hay %d", total)
	}
}

// TestMergeNoticeNamesTheMode: el resultado dice con qué estrategia se integró.
// Sin eso, un "merge ok" no permite saber si se aplicó el rebase que el usuario
// no quería.
func TestMergeNoticeNamesTheMode(t *testing.T) {
	f := newMergeFixture(t, mergeItems()...)
	m := press(t, f.m, "m")
	m = press(t, m, "r")
	out := waitOutcome(t, m)
	m = send(t, m, actionMsg{cycle: m.cycle, outcome: out})

	if !strings.Contains(lastToast(m), "rebase") {
		t.Errorf("el aviso final debería nombrar el modo, aviso = %q", lastToast(m))
	}
}

// ═══════════════ B — auto-borrado del worktree tras mergear ═══════════════

// fakeRemover simula el auto-borrado del worktree del review. Registra el ítem
// con el que se le pidió y devuelve un resultado fijo.
type fakeRemover struct {
	removed bool
	reason  string
	err     error
	got     model.Item
	calls   int
}

func (f *fakeRemover) RemoveReview(_ context.Context, it model.Item) (bool, string, error) {
	f.got = it
	f.calls++
	return f.removed, f.reason, f.err
}

// mergeOutcome arma un merge rebase desde la TUI, lo dispara y devuelve el modelo
// (con el remover inyectado) junto al resultado del forge, sin aplicarlo.
func mergeOutcome(t *testing.T, remover ReviewRemover) (Model, forge.Outcome) {
	t.Helper()
	f := newMergeFixture(t, mergeItems()...)
	m := f.m
	if remover != nil {
		m.SetReviewRemover(remover)
	}
	m = press(t, m, "m")
	m = press(t, m, "r")
	return m, waitOutcome(t, m)
}

// mergeAndApply hace lo mismo y además vuelca el resultado como si llegara del
// canal de eventos.
func mergeAndApply(t *testing.T, remover ReviewRemover) Model {
	t.Helper()
	m, out := mergeOutcome(t, remover)
	return send(t, m, actionMsg{cycle: m.cycle, outcome: out})
}

// cleanupMsg lee el reviewCleanupMsg que dispara el merge OK y lo aplica.
func cleanupMsg(t *testing.T, m Model) reviewCleanupMsg {
	t.Helper()
	ev := waitMount(t, m)
	msg, ok := ev.(reviewCleanupMsg)
	if !ok {
		t.Fatalf("evento %T, want reviewCleanupMsg", ev)
	}
	return msg
}

// TestMergeOKRemovesCleanWorktree cubre el caso feliz: el merge OK borra el
// worktree del ítem seleccionado y el aviso dice las dos cosas.
func TestMergeOKRemovesCleanWorktree(t *testing.T) {
	remover := &fakeRemover{removed: true}
	m, out := mergeOutcome(t, remover)
	m = send(t, m, actionMsg{cycle: m.cycle, outcome: out})
	m = send(t, m, cleanupMsg(t, m))

	if remover.calls != 1 || remover.got.ID() != out.Item.ID() {
		t.Fatalf("remover calls=%d item=%v, quiero el ítem mergeado %v", remover.calls, remover.got.ID(), out.Item.ID())
	}
	toast := lastToast(m)
	if !strings.Contains(toast, "worktree removed") || !strings.Contains(toast, "ok") {
		t.Errorf("aviso = %q, want merge ok + worktree removed", toast)
	}
}

// TestMergeOKKeepsDirtyWorktree fija el texto exacto del caso sucio.
func TestMergeOKKeepsDirtyWorktree(t *testing.T) {
	remover := &fakeRemover{reason: worktree.KeptUncommitted}
	m := mergeAndApply(t, remover)
	m = send(t, m, cleanupMsg(t, m))

	if !strings.Contains(lastToast(m), "merged, but the worktree has uncommitted changes — kept") {
		t.Errorf("aviso = %q, want el texto exacto del caso sucio", lastToast(m))
	}
}

// TestMergeOKKeepsUnreadableWorktree cubre el fail-safe: si no se pudo leer el
// estado, se conserva y el aviso lo dice.
func TestMergeOKKeepsUnreadableWorktree(t *testing.T) {
	remover := &fakeRemover{reason: worktree.KeptUnreadable}
	m := mergeAndApply(t, remover)
	m = send(t, m, cleanupMsg(t, m))

	if !strings.Contains(lastToast(m), "could not read the worktree status") {
		t.Errorf("aviso = %q, want que diga que no se pudo comprobar el estado", lastToast(m))
	}
}

// TestMergeOKWithoutReviewReportsNoError: sin review montado el merge termina
// igual y no aparece ningún aviso de limpieza.
func TestMergeOKWithoutReviewReportsNoError(t *testing.T) {
	remover := &fakeRemover{} // (false, "", nil): no hay review montado
	m := mergeAndApply(t, remover)
	m = send(t, m, cleanupMsg(t, m))

	toast := lastToast(m)
	if strings.Contains(toast, "worktree") || strings.Contains(toast, "could not") {
		t.Errorf("aviso = %q, sin review no debería haber aviso de limpieza", toast)
	}
	if !strings.Contains(toast, "ok") {
		t.Errorf("aviso = %q, el merge debería seguir diciendo que salió bien", toast)
	}
}

// TestMergeOKCleanupErrorIsWarn: un fallo de borrado no convierte el merge en
// error: el merge sí salió, así que se avisa como una advertencia.
func TestMergeOKCleanupErrorIsWarn(t *testing.T) {
	remover := &fakeRemover{err: errors.New("boom")}
	m := mergeAndApply(t, remover)
	m = send(t, m, cleanupMsg(t, m))

	if !strings.Contains(lastToast(m), "could not remove the worktree: boom") {
		t.Errorf("aviso = %q, want el motivo del fallo de borrado", lastToast(m))
	}
}

// TestMergeCleanupComposesWithBranchNotDeleted: el aviso del borrado no pisa el
// hecho de que la rama no se borró; los dos conviven.
func TestMergeCleanupComposesWithBranchNotDeleted(t *testing.T) {
	remover := &fakeRemover{reason: worktree.KeptUncommitted}
	m, out := mergeOutcome(t, remover)
	out.DeleteMsg = "the protected branch was kept"
	m = send(t, m, actionMsg{cycle: m.cycle, outcome: out})
	m = send(t, m, cleanupMsg(t, m))

	toast := lastToast(m)
	for _, want := range []string{"ok", "branch not deleted", "uncommitted changes — kept"} {
		if !strings.Contains(toast, want) {
			t.Errorf("aviso = %q, falta %q", toast, want)
		}
	}
}

// TestReviewCleanupNoticeLevels fija el nivel del aviso compuesto: cubrir el
// texto no basta, porque un borrado sobre un merge que ya avisaba (rama no
// borrada) debe conservar el warning en vez de rebajarlo a OK.
func TestReviewCleanupNoticeLevels(t *testing.T) {
	const base = "merge (squash) ok · branch not deleted: protected"
	cases := []struct {
		name      string
		baseLevel noticeLevel
		removed   bool
		reason    string
		err       error
		wantLevel noticeLevel
	}{
		{"borrado sobre base OK", levelOK, true, "", nil, levelOK},
		{"borrado sobre base warn", levelWarn, true, "", nil, levelWarn},
		{"conservado por sucio", levelOK, false, worktree.KeptUncommitted, nil, levelWarn},
		{"conservado por ilegible", levelOK, false, worktree.KeptUnreadable, nil, levelWarn},
		{"fallo de borrado", levelOK, false, "", errors.New("boom"), levelWarn},
		{"sin review montado", levelOK, false, "", nil, levelOK},
		{"sin review sobre base warn", levelWarn, false, "", nil, levelWarn},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, level := reviewCleanupNotice(base, tc.baseLevel, tc.removed, tc.reason, tc.err)
			if level != tc.wantLevel {
				t.Fatalf("nivel = %d, quiero %d", level, tc.wantLevel)
			}
		})
	}
}

// TestMergeCleanupRemovedKeepsWarnLevel comprueba de punta a punta que el aviso
// final de un borrado sobre un merge ya advertido (rama no borrada) sigue siendo
// warning: el nivel forma parte del hecho, no solo el texto.
func TestMergeCleanupRemovedKeepsWarnLevel(t *testing.T) {
	remover := &fakeRemover{removed: true}
	m, out := mergeOutcome(t, remover)
	out.DeleteMsg = "the protected branch was kept"
	m = send(t, m, actionMsg{cycle: m.cycle, outcome: out})
	m = send(t, m, cleanupMsg(t, m))

	if !strings.Contains(lastToast(m), "worktree removed") {
		t.Fatalf("aviso = %q, quiero que diga que se borró", lastToast(m))
	}
	if got := lastToastLevel(m); got != toastWarning {
		t.Fatalf("nivel = %d, quiero warning: la rama no se borró", got)
	}
}

// TestCleanupDoesNotTriggerOnOtherOutcomes cubre los negativos: aprobar, cambiar
// la base y un merge que no sale bien no disparan ningún borrado.
func TestCleanupDoesNotTriggerOnOtherOutcomes(t *testing.T) {
	cases := []struct {
		name string
		out  forge.Outcome
	}{
		{"approve", forge.Outcome{Kind: forge.ActionApprove, OK: true}},
		{"retarget", forge.Outcome{Kind: forge.ActionRetarget, OK: true, Base: "main"}},
		{"merge falla", forge.Outcome{Kind: forge.ActionMerge, OK: false, Msg: "boom"}},
		{"conflicto", forge.Outcome{Kind: forge.ActionMerge, Conflict: true, Msg: "changed"}},
		{"no mergeable", forge.Outcome{Kind: forge.ActionMerge, Unmergeable: true, Msg: "rebase"}},
		{"permiso", forge.Outcome{Kind: forge.ActionMerge, Perm: true, Msg: "no"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newMergeFixture(t, mergeItems()...)
			remover := &fakeRemover{removed: true}
			m := f.m
			m.SetReviewRemover(remover)
			send(t, m, actionMsg{cycle: m.cycle, outcome: tc.out})
			if remover.calls != 0 {
				t.Errorf("remover llamado %d veces, want 0", remover.calls)
			}
		})
	}
}

// TestMergeOKWithoutRemoverStillWorks cubre la degradación: sin removedor
// inyectado el merge funciona igual y no auto-borra.
func TestMergeOKWithoutRemoverStillWorks(t *testing.T) {
	m := mergeAndApply(t, nil)
	if !strings.Contains(lastToast(m), "ok") {
		t.Fatalf("aviso = %q", lastToast(m))
	}
}

// TestQuitDoesNotRemoveWorktrees cubre el invariante duro: cerrar la app nunca
// ejecuta un borrado.
func TestQuitDoesNotRemoveWorktrees(t *testing.T) {
	f := newMergeFixture(t, mergeItems()...)
	remover := &fakeRemover{removed: true}
	m := f.m
	m.SetReviewRemover(remover)

	out, _ := m.Update(tea.KeyPressMsg{Code: 'q', Text: "q"})
	if out == nil {
		t.Fatal("q debería seguir cerrando la TUI")
	}
	if remover.calls != 0 {
		t.Errorf("cerrar la app no debe borrar nada (llamadas=%d)", remover.calls)
	}
}

// TestRefreshMergedItemDoesNotTriggerCleanup: un PR mergeado que se ve al
// refrescar (fuera de prdash) no dispara la limpieza.
func TestRefreshMergedItemDoesNotTriggerCleanup(t *testing.T) {
	f := newMergeFixture(t, mergeItems()...)
	remover := &fakeRemover{removed: true}
	m := f.m
	m.SetReviewRemover(remover)

	merged := f.items[0]
	merged.State = "MERGED"
	m = send(t, m, page(m.cycle, "github", "github.com", model.SectionReview, model.ReviewRequested, []model.Item{merged}, false))

	if remover.calls != 0 {
		t.Errorf("un refresco no debe disparar la limpieza (llamadas=%d)", remover.calls)
	}
}
