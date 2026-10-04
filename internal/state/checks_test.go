package state

import (
	"strings"
	"testing"

	"prdash/internal/forge/model"
)

func TestChecksFailingReasonDistingueConteoDeSinConteo(t *testing.T) {
	cases := []struct {
		name   string
		checks model.Checks
		want   string
	}{
		{"con recuento", model.Checks{Total: 5, Failing: 2, State: model.ChecksFailing}, "CI is failing (2 of 5 checks)"},
		{"uno de uno", model.Checks{Total: 1, Failing: 1, State: model.ChecksFailing}, "CI is failing (1 of 1 checks)"},
		{"sin datos", model.Checks{}, "CI is failing"},
		{"total cero con fallo", model.Checks{Total: 0, Failing: 0, State: model.ChecksFailing}, "CI is failing"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := checksFailingReason(c.checks); got != c.want {
				t.Errorf("checksFailingReason(%+v) = %q, want %q", c.checks, got, c.want)
			}
		})
	}
}

// That the count survives the trip: MergeBlock carries it to the warning.
func TestChecksFailingReasonLlegaAlAvisoDelGate(t *testing.T) {
	it := model.NewItem(model.RepoRef{Forge: "github", Host: "github.com", Project: "acme/widget"}, 1)
	it.State = "OPEN"
	it.Checks = model.Checks{Total: 12, Failing: 3, State: model.ChecksFailing}

	block := MergeBlock(it)
	if block.Reason == "" {
		t.Fatal("un ítem con CI en rojo debería tener bloque")
	}
	if !strings.Contains(block.Reason, "3 of 12") {
		t.Errorf("el motivo = %q, want el recuento de checks", block.Reason)
	}

	sinChecks := it
	sinChecks.Checks = model.Checks{State: model.ChecksFailing}
	block = MergeBlock(sinChecks)
	if !strings.Contains(block.Reason, "CI is failing") {
		t.Errorf("el motivo = %q, want el aviso de CI", block.Reason)
	}
	if strings.Contains(block.Reason, "of 0 checks") {
		t.Errorf("sin datos de checks no hay recuento que enseñar: %q", block.Reason)
	}
}

// normalize's contract: the forges' enums arrive in different conventions.
func TestNormalizeComparaSinMayusculasNiSeparadores(t *testing.T) {
	cases := map[string]string{
		"OPEN":              "open",
		"open":              "open",
		"CHANGES_REQUESTED": "changes_requested",
		"changes-requested": "changes_requested",
		"changes requested": "changes_requested",
		"  OPEN\t":          "__open_",
		"":                  "",
		// The Z is the last uppercase of the range, the one a misplaced `<=` leaves out.
		"BUZZ":   "buzz",
		"Z":      "z",
		"AZ":     "az",
		"ZZ":     "zz",
		"año2":   "año2",
		"9lives": "9lives",
	}
	for in, want := range cases {
		if got := normalize(in); got != want {
			t.Errorf("normalize(%q) = %q, want %q", in, got, want)
		}
	}
}

// A state with Z is recognised even though it is out of range.
func TestDeriveNormalizaEstadosConZ(t *testing.T) {
	it := model.NewItem(model.RepoRef{Forge: "github", Host: "github.com", Project: "acme/widget"}, 1)
	it.State = "BUZZ"
	if got := Derive(it); got != StatePending {
		t.Errorf("Derive(estado=%q) = %v, want StatePending (un estado desconocido no bloquea)", it.State, got)
	}
	for _, s := range []string{"merged", "MERGED", "Merged"} {
		it.State = s
		if got := Derive(it); got != StateMerged {
			t.Errorf("Derive(%q) = %v, want StateMerged", s, got)
		}
	}
	for _, s := range []string{"closed", "CLOSED"} {
		it.State = s
		if got := MergeBlock(it).Reason; got != "item is already closed" {
			t.Errorf("MergeBlock(%q) = %q, want el bloqueo por cerrado", s, got)
		}
	}
}
