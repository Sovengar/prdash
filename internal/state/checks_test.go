package state

import (
	"strings"
	"testing"

	"prdash/internal/forge/model"
)

// TestChecksFailingReasonDistingueConteoDeSinConteo separa las dos frases de
// checks. Con recuento, el aviso dice cuántos de cuántos fallan, que es lo que
// distingue un CI roto de un check inestable. Sin datos de checks, no puede
// inventar un denominador: un "(0 of 0 checks)" affirmaría que se consultó y
// que no había ninguno, que es una afirmación distinta de "no se sabe".
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

// TestChecksFailingReasonLlegaAlAvisoDelGate comprueba que el conteo no se pierde
// por el camino: MergeBlock es lo que la TUI enseña y lo que el gate consulta, y
// si aquí el número se perdiera, el usuario vería un "no puedes" sin el dato que
// le dice si su CI tiene un test inestable o está entero roto.
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

	// Y sin datos de checks el bloque sigue existiendo pero sin inventar cifras.
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

// TestNormalizeComparaSinMayusculasNiSeparadores es el contrato de normalize:
// los enums de los forges llegan con mayúsculas y guiones ("CHANGES-REQUESTED",
// "in progress"), y la comparación tiene que ser insensible a las dos cosas.
//
// El caso de la 'Z' es el que importa de verdad: si el rango de mayúsculas
// dejara fuera la última letra del alfabeto, "BUZZ" no normalizaría a
// "buzz" y cualquier estado o decisión que lo contenga se compararía contra una
// cadena que nunca puede coincidir. La comparación se queda en silencio y el
// ítem cae en el caso por defecto, que es el peor sitio: ni error ni aviso.
func TestNormalizeComparaSinMayusculasNiSeparadores(t *testing.T) {
	cases := map[string]string{
		"OPEN":              "open",
		"open":              "open",
		"CHANGES_REQUESTED": "changes_requested",
		"changes-requested": "changes_requested",
		"changes requested": "changes_requested",
		"  OPEN\t":          "__open_",
		"":                  "",
		// La Z es la última mayúscula del rango: es la que un `<=` mal puesto
		// deja fuera.
		"BUZZ": "buzz",
		"Z":    "z",
		"AZ":   "az",
		"ZZ":   "zz",
		// Y lo que no es mayúscula se queda como estaba.
		"año2":   "año2",
		"9lives": "9lives",
	}
	for in, want := range cases {
		if got := normalize(in); got != want {
			t.Errorf("normalize(%q) = %q, want %q", in, got, want)
		}
	}
}

// TestDeriveConElZDelRango: el comportamiento observable de esa 'Z' es que un
// estado con Z se reconoce. Aunque hoy ningún enum de forge lleve Z, el
// predicado se usa sobre el texto que venga, y un valor no reconocido degenera en
// StatePending sin decir nada.
func TestDeriveNormalizaEstadosConZ(t *testing.T) {
	it := model.NewItem(model.RepoRef{Forge: "github", Host: "github.com", Project: "acme/widget"}, 1)
	it.State = "BUZZ"
	if got := Derive(it); got != StatePending {
		t.Errorf("Derive(estado=%q) = %v, want StatePending (un estado desconocido no bloquea)", it.State, got)
	}
	// Y el camino bueno: un estado en minúsculas y uno en mayúsculas dan lo mismo.
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
