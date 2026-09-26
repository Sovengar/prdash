// Tests de la doble confirmación de merge: la primera pulsación solo arma y la
// segunda es la que elige el modo y ejecuta. Un merge reescribe historia y no se
// deshace con un comando, así que lo que más importa es que no exista ningún
// camino que lo dispare con una estrategia que el usuario no ha nombrado.
package tui

import (
	"strconv"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"prdash/internal/forge"
	"prdash/internal/forge/model"
	"prdash/internal/testutil"
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
// depender de cuántas可视 hay apilados.
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
	for _, want := range []string{"Are you sure", "m merge commit", "r rebase", "s squash", "esc cancel"} {
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

// TestMergeArmedCancelsOnOtherKey: unarmed y re-despacha. Así un `m` a destiempo
// no deja la vista esperando una segunda pulsación, y moverse por la lista
// cancela el merge implícitamente.
func TestMergeArmedCancelsOnOtherKey(t *testing.T) {
	f := newMergeFixture(t, mergeItems()...)
	start := f.m.cursor
	m := press(t, f.m, "m")

	m = press(t, m, "down")
	if m.mergeArmed {
		t.Error("una tecla de navegación debería desarmar el merge")
	}
	if m.cursor == start {
		t.Error("la navegación debería ejecutarse tras desarmar")
	}
	if m.actionBusy {
		t.Error("desarmar no debería dejar una acción en curso")
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
