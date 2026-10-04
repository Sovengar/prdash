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

func stateKey(it model.Item) string {
	return it.Ref.Project + "#" + strconv.Itoa(it.Number)
}

func waitOutcome(t *testing.T, m Model) forge.Outcome {
	t.Helper()
	ev := waitMount(t, m)
	msg, ok := ev.(actionMsg)
	if !ok {
		t.Fatalf("evento %T, want actionMsg", ev)
	}
	return msg.outcome
}

func toasts(m Model) string { return strings.Join(toastTexts(m), " | ") }

func TestMergeArmsOnFirstPress(t *testing.T) {
	f := newMergeFixture(t, mergeItems()...)
	// The cursor need not be on the first row of the page: the rows are ordered.
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

func TestMergeArmedShowsConfirmInKeybinds(t *testing.T) {
	f := newMergeFixture(t, mergeItems()...)
	m := press(t, f.m, "m")

	view := stripANSI(m.View().Content)
	for _, want := range []string{"press the mode", "m merge commit", "r rebase", "s squash", "esc cancel"} {
		if !strings.Contains(view, want) {
			t.Errorf("la confirmación no menciona %q\n%s", want, view)
		}
	}
	if strings.Contains(view, "pgup/dn page") {
		t.Errorf("con el merge armado la barra de atajos debería ceder\n%s", view)
	}
}

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

// It ALSO eats the key. It used to delegate to handleKey, which turned a mis-armed merge into an
// approve: `m` then `a` approved the PR.
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

func TestMergeOnChangedItemAborts(t *testing.T) {
	f := newMergeFixture(t, mergeItems()...)
	m := press(t, f.m, "m")
	armed, _ := m.selected()

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

func applyMerge(t *testing.T, remover ReviewRemover) (Model, tea.Cmd) {
	t.Helper()
	m, out := mergeOutcome(t, remover)
	return m, m.applyAction(out, m.cycle)
}

func runCleanup(t *testing.T, m Model, cmd tea.Cmd) (Model, bool) {
	t.Helper()
	if cmd == nil {
		return m, false
	}
	msg, ok := cmd().(reviewCleanupMsg)
	if !ok {
		t.Fatalf("el comando de limpieza devolvió %T, want reviewCleanupMsg", cmd())
	}
	return send(t, m, msg), true
}

func TestMergeOKRemovesCleanWorktree(t *testing.T) {
	remover := &fakeRemover{removed: true}
	m, out := mergeOutcome(t, remover)
	cmd := m.applyAction(out, m.cycle)
	if cmd == nil {
		t.Fatal("un merge OK debería devolver el comando de limpieza")
	}
	m, _ = runCleanup(t, m, cmd)

	if remover.calls != 1 || remover.got.ID() != out.Item.ID() {
		t.Fatalf("remover calls=%d item=%v, quiero el ítem mergeado %v", remover.calls, remover.got.ID(), out.Item.ID())
	}
	toast := lastToast(m)
	if !strings.Contains(toast, "worktree removed") || !strings.Contains(toast, "ok") {
		t.Errorf("aviso = %q, want merge ok + worktree removed", toast)
	}
}

func TestMergeOKKeepsDirtyWorktree(t *testing.T) {
	remover := &fakeRemover{reason: worktree.KeptUncommitted}
	m, cmd := applyMerge(t, remover)
	m, _ = runCleanup(t, m, cmd)

	if !strings.Contains(lastToast(m), "merged, but the worktree has uncommitted changes — kept") {
		t.Errorf("aviso = %q, want el texto exacto del caso sucio", lastToast(m))
	}
}

func TestMergeOKKeepsUnreadableWorktree(t *testing.T) {
	remover := &fakeRemover{reason: worktree.KeptUnreadable}
	m, cmd := applyMerge(t, remover)
	m, _ = runCleanup(t, m, cmd)

	if !strings.Contains(lastToast(m), "could not read the worktree status") {
		t.Errorf("aviso = %q, want que diga que no se pudo comprobar el estado", lastToast(m))
	}
}

func TestMergeOKWithoutReviewReportsNoError(t *testing.T) {
	remover := &fakeRemover{} // (false, "", nil): no hay review montado
	m, cmd := applyMerge(t, remover)
	if cmd == nil {
		t.Fatal("con removedor inyectado un merge OK debería devolver comando")
	}
	m, _ = runCleanup(t, m, cmd)

	toast := lastToast(m)
	if strings.Contains(toast, "worktree") || strings.Contains(toast, "could not") {
		t.Errorf("aviso = %q, sin review no debería haber aviso de limpieza", toast)
	}
	if !strings.Contains(toast, "ok") {
		t.Errorf("aviso = %q, el merge debería seguir diciendo que salió bien", toast)
	}
}

func TestMergeCleanupNoopDoesNotDuplicateToast(t *testing.T) {
	remover := &fakeRemover{} // (false, "", nil)
	m, cmd := applyMerge(t, remover)
	before := len(toastTexts(m))
	m, _ = runCleanup(t, m, cmd)
	if got := len(toastTexts(m)); got != before {
		t.Fatalf("avisos vivos = %d tras el no-op, quiero %d (sin duplicar): %q", got, before, toastTexts(m))
	}
}

// reviewCleanupMsg comes from a Cmd, not the events channel.
func TestReviewCleanupMsgDoesNotArmChannelReader(t *testing.T) {
	remover := &fakeRemover{removed: true}
	m, cmd := applyMerge(t, remover)
	msg, ok := cmd().(reviewCleanupMsg)
	if !ok {
		t.Fatalf("el comando de limpieza devolvió %T, want reviewCleanupMsg", cmd())
	}

	updated, follow := m.Update(msg)
	if follow != nil {
		t.Fatalf("un mensaje Cmd-delivered no debe devolver un cmd (withPump armaría un lector de más): %T", follow)
	}
	if got := updated.(Model).readers; got != 1 {
		t.Fatalf("readers = %d, quiero 1", got)
	}
}

func TestMergeCleanupRemovedDoesNotDuplicateToast(t *testing.T) {
	remover := &fakeRemover{removed: true}
	m, out := mergeOutcome(t, remover)
	cmd := m.applyAction(out, m.cycle)
	m, _ = runCleanup(t, m, cmd)

	base := actionDoneNotice(out)
	texts := toastTexts(m)
	stale, removed := 0, 0
	for _, txt := range texts {
		if txt == base {
			stale++
		}
		if strings.Contains(txt, "worktree removed") {
			removed++
		}
	}
	if stale != 0 {
		t.Fatalf("quedó una copia sin actualizar del aviso del merge: %q", texts)
	}
	if removed != 1 {
		t.Fatalf("quiero un único aviso con 'worktree removed': %q", texts)
	}
}

func TestMergeOKCleanupErrorIsWarn(t *testing.T) {
	remover := &fakeRemover{err: errors.New("boom")}
	m, cmd := applyMerge(t, remover)
	m, _ = runCleanup(t, m, cmd)

	if !strings.Contains(lastToast(m), "could not remove the worktree: boom") {
		t.Errorf("aviso = %q, want el motivo del fallo de borrado", lastToast(m))
	}
}

func TestMergeCleanupComposesWithBranchNotDeleted(t *testing.T) {
	remover := &fakeRemover{reason: worktree.KeptUncommitted}
	m, out := mergeOutcome(t, remover)
	out.DeleteMsg = "the protected branch was kept"
	cmd := m.applyAction(out, m.cycle)
	m, _ = runCleanup(t, m, cmd)

	toast := lastToast(m)
	for _, want := range []string{"ok", "branch not deleted", "uncommitted changes — kept"} {
		if !strings.Contains(toast, want) {
			t.Errorf("aviso = %q, falta %q", toast, want)
		}
	}
}

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

func TestMergeCleanupRemovedKeepsWarnLevel(t *testing.T) {
	remover := &fakeRemover{removed: true}
	m, out := mergeOutcome(t, remover)
	out.DeleteMsg = "the protected branch was kept"
	cmd := m.applyAction(out, m.cycle)
	m, _ = runCleanup(t, m, cmd)

	if !strings.Contains(lastToast(m), "worktree removed") {
		t.Fatalf("aviso = %q, quiero que diga que se borró", lastToast(m))
	}
	if got := lastToastLevel(m); got != toastWarning {
		t.Fatalf("nivel = %d, quiero warning: la rama no se borró", got)
	}
}

func TestTriggersReviewCleanup(t *testing.T) {
	yes := []forge.Outcome{
		{Kind: forge.ActionMerge, OK: true},
	}
	no := []forge.Outcome{
		{Kind: forge.ActionMerge, OK: false},
		{Kind: forge.ActionApprove, OK: true},
		{Kind: forge.ActionRetarget, OK: true},
		{Kind: forge.ActionMerge, Conflict: true},
		{Kind: forge.ActionMerge, Unmergeable: true},
		{Kind: forge.ActionMerge, Perm: true},
	}
	for _, out := range yes {
		if !triggersReviewCleanup(out) {
			t.Errorf("un merge OK debería disparar la limpieza: %+v", out)
		}
	}
	for _, out := range no {
		if triggersReviewCleanup(out) {
			t.Errorf("no debería disparar la limpieza: %+v", out)
		}
	}
}

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
			if cmd := m.applyAction(tc.out, m.cycle); cmd != nil {
				t.Fatalf("no debería devolver comando de limpieza para %s", tc.name)
			}
			if remover.calls != 0 {
				t.Errorf("remover llamado %d veces, want 0", remover.calls)
			}
		})
	}
}

func TestMergeOKWithoutRemoverStillWorks(t *testing.T) {
	m, cmd := applyMerge(t, nil)
	if cmd != nil {
		t.Fatal("sin removedor no debería haber comando de limpieza")
	}
	if !strings.Contains(lastToast(m), "ok") {
		t.Fatalf("aviso = %q", lastToast(m))
	}
}

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
