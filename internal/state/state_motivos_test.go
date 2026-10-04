package state

import (
	"strings"
	"testing"

	"prdash/internal/forge/model"
)

// Score orders the inbox and the order is a product decision, not an accident.
func TestElOrdenDeAtencionEsElDocumentado(t *testing.T) {
	orden := []State{StateError, StateChangesRequested, StateReviewRequired, StateApproved,
		StatePending, StateMerged, StateDraft}
	for i := 0; i < len(orden)-1; i++ {
		a, b := orden[i], orden[i+1]
		if orden[i].Score() <= orden[i+1].Score() {
			t.Errorf("%q (score %d) no ordena por encima de %q (score %d)",
				a, a.Score(), b, b.Score())
		}
	}

	// Merged and closed tie. Deliberately: both mean "this is no longer pending".
	if StateMerged.Score() != StateClosed.Score() {
		t.Errorf("merged puntua %d y closed %d: el codigo los pone juntos a proposito, "+
			"asi que si divergen es una decision que hay que escribir",
			StateMerged.Score(), StateClosed.Score())
	}

	// An unknown state scores zero, below draft: unknown is not "urgent".
	if got := State(99).Score(); got != 0 {
		t.Errorf("un estado desconocido puntua %d, want 0", got)
	}
	if got := State(99).String(); got == "" {
		t.Error("un estado desconocido dio etiqueta vacia")
	}
}

// String goes in the state column, one of the narrowest.
func TestLaEtiquetaDelEstadoEsLaDeLaColumnaEstrecha(t *testing.T) {
	casos := []struct {
		estado State
		want   string
	}{
		{StateDraft, "draft"},
		{StatePending, "pending"},
		{StateApproved, "approved"},
		{StateReviewRequired, "review required"},
		{StateChangesRequested, "changes requested"},
		{StateMerged, "merged"},
		{StateClosed, "closed"},
		{StateError, "error"},
	}
	for _, c := range casos {
		if got := c.estado.String(); got != c.want {
			t.Errorf("State(%q).String() dio %q, want %q", c.estado, got, c.want)
		}
		// An empty label is worse than a long one: the column goes blank and there is nothing to read.
		if strings.TrimSpace(c.estado.String()) == "" {
			t.Errorf("State(%q) tiene etiqueta vacia", c.estado)
		}
		if c.estado.String() != strings.TrimSpace(c.estado.String()) {
			t.Errorf("la etiqueta de %q tiene espacios en los bordes", c.estado)
		}
	}
	// An unknown state invents no label: it prints what can be extracted from an int.
	if got := State(99).String(); strings.TrimSpace(got) == "" {
		t.Error("un estado desconocido dio etiqueta vacia")
	}
}

// The conflict warning has to name the branch you have to rebase against.
func TestElMotivoDelConflictoNombraLaRama(t *testing.T) {
	casos := []struct {
		target string
		want   string
	}{
		{"main", "the branch conflicts with main"},
		{"release/2026-04", "the branch conflicts with release/2026-04"},
		{"", "the branch conflicts with the target branch"},
		{"   ", "the branch conflicts with the target branch"},
		{"\t\n", "the branch conflicts with the target branch"},
	}
	for _, c := range casos {
		got := conflictedReason(c.target)
		if got != c.want {
			t.Errorf("conflictedReason(%q) dio %q, want %q", c.target, got, c.want)
		}
		if strings.TrimSpace(got) == "" {
			t.Errorf("conflictedReason(%q) devolvio texto vacio", c.target)
		}
	}
	// And the generic one carries NO invented branch name.
	if strings.HasSuffix(conflictedReason(""), "with ") {
		t.Error("el aviso generico acaba en \"with \": parece una rama que se perdio")
	}
}

// The failing-check count is what tells a broken CI from a pending one.
func TestElMotivoDelCIEmpiezaPorElRecuento(t *testing.T) {
	casos := []struct {
		checks model.Checks
		want   string
	}{
		{model.Checks{Total: 10, Failing: 1}, "CI is failing (1 of 10 checks)"},
		{model.Checks{Total: 3, Failing: 3}, "CI is failing (3 of 3 checks)"},
		{model.Checks{Total: 1, Failing: 1}, "CI is failing (1 of 1 checks)"},
		{model.Checks{}, "CI is failing"},
	}
	for _, c := range casos {
		if got := checksFailingReason(c.checks); got != c.want {
			t.Errorf("checksFailingReason(%+v) dio %q, want %q", c.checks, got, c.want)
		}
	}
	// A known total never prints without counts: if Failing is zero and the total is not, the text
	// has to say so.
	if got := checksFailingReason(model.Checks{Total: 5, Failing: 0}); got == "CI is failing" {
		t.Error("con 5 checks conocidos el aviso salio sin cifras: se pierde el dato")
	}
}

func TestLosMotivosNoEstanVacios(t *testing.T) {
	motivos := []string{UnmergeableReason}
	for _, m := range motivos {
		if strings.TrimSpace(m) == "" {
			t.Errorf("un motivo esta vacio: %q", m)
		}
		if m != strings.TrimSpace(m) {
			t.Errorf("el motivo %q tiene espacios en los bordes", m)
		}
	}
	// The merge reason reads as something to DO, not as a "no".
	if !strings.Contains(UnmergeableReason, "push") {
		t.Errorf("el motivo de merge no dice que hacer: %q", UnmergeableReason)
	}
}
