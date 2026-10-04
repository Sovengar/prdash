package tui

import (
	"strings"
	"testing"
	"time"

	"prdash/internal/forge"
	"prdash/internal/testutil"
)

// Complementary: Valid says it cannot be done, Label returns the name as given.
func TestUnMergeModoDesconocidoNoSePresentaComoUnModoConocido(t *testing.T) {
	for _, c := range []struct {
		modo forge.MergeMode
		want string
	}{
		{forge.MergeCommit, "merge commit"},
		{forge.Rebase, "rebase"},
		{forge.Squash, "squash"},
		{forge.MergeMode("fast-forward"), "fast-forward"},
		{forge.MergeMode(""), ""},
	} {
		if got := c.modo.Label(); got != c.want {
			t.Errorf("%q.Label() = %q, want %q", string(c.modo), got, c.want)
		}
	}

	// Valid separates what is known from what is not, which is what decides whether to warn first.
	for _, c := range []struct {
		modo forge.MergeMode
		want bool
	}{
		{forge.MergeCommit, true},
		{forge.Rebase, true},
		{forge.Squash, true},
		{forge.MergeMode("fast-forward"), false},
		{forge.MergeMode(""), false},
		{forge.MergeMode("MERGE"), false}, // las mayúsculas son otro modo, no el mismo
	} {
		if got := c.modo.Valid(); got != c.want {
			t.Errorf("%q.Valid() = %v, want %v", string(c.modo), got, c.want)
		}
	}

	// And the confirmation's key, the third place an unknown mode has to be handled.
	for _, c := range []struct {
		modo forge.MergeMode
		want string
	}{
		{forge.MergeCommit, "m"},
		{forge.Rebase, "r"},
		{forge.Squash, "s"},
		{forge.MergeMode("otro"), "?"},
	} {
		if got := modeKey(c.modo); got != c.want {
			t.Errorf("tecla de %q = %q, want %q", string(c.modo), got, c.want)
		}
	}
}

// Composing on the current warning instead of stacking a second copy of the same text.
func TestReplaceSustituyeElAvisoVigenteYNoDuplicaElTexto(t *testing.T) {
	tm := &toastManager{now: func() time.Time { return time.Unix(0, 0) }}

	tm.show("consultando ramas", toastInfo)
	tm.replace("consultando ramas", "consultando ramas…", toastInfo)
	if len(tm.toasts) != 1 {
		t.Fatalf("quedan %d avisos tras sustituir, want 1: el texto nuevo se apiló en vez "+
			"de sustituir al viejo y se ven dos avisos del mismo origen", len(tm.toasts))
	}
	if tm.toasts[0].message != "consultando ramas…" {
		t.Errorf("el texto quedó en %q", tm.toasts[0].message)
	}

	// And with a `prev` that is not there: it stacks, it is not lost. A lost warning is the worst
	// outcome.
	tm.replace("un texto que ya no existe", "otro aviso", toastWarning)
	if len(tm.toasts) != 2 {
		t.Errorf("un `prev` inexistente dejó %d avisos, want 2: el nuevo se apila", len(tm.toasts))
	}

	// And an EMPTY message does not stack: an empty warning is indistinguishable from not warning.
	antes := len(tm.toasts)
	tm.replace("consultando ramas…", "", toastInfo)
	if len(tm.toasts) != antes {
		t.Errorf("un mensaje vacío cambió el número de avisos: %d -> %d", antes, len(tm.toasts))
	}
}

func TestLaCabeceraMuestraElSpinnerPrimeroYElEstadoDeLosForgesDetras(t *testing.T) {
	nuevo := func(t *testing.T) Model {
		t.Helper()
		m := newTestModel(t, &testutil.FakeAdapter{ForgeName: "github", HostName: "github.com"})
		m.width, m.height = 120, 40
		return m
	}

	cargando := nuevo(t)
	cargando.loading = true
	texto := cargando.headerSection().text
	if !strings.Contains(texto, "refreshing") {
		t.Errorf("cargando, la cabecera no dice que está refrescando:\n%s", texto)
	}
	if !strings.Contains(texto, "PRDash") {
		t.Errorf("la cabecera perdió el título:\n%s", texto)
	}
	dondeSpinner := strings.Index(texto, "refreshing")
	dondeEstado := len(texto)
	for _, marca := range []string{"github", "not authenticated", "no auth"} {
		if i := strings.Index(texto, marca); i >= 0 && i < dondeEstado {
			dondeEstado = i
		}
	}
	if dondeEstado < dondeSpinner {
		t.Errorf("el estado de los forges va antes del spinner: %q", texto)
	}

	quieto := nuevo(t)
	quieto.loading = false
	if strings.Contains(quieto.headerSection().text, "refreshing") {
		t.Errorf("sin carga la cabecera sigue diciendo que refresca:\n%s",
			quieto.headerSection().text)
	}

	// And the narrow terminal, which is the reason for the order: the spinner survives and the forge
	// statuses go.
	estrecho := nuevo(t)
	estrecho.loading = true
	estrecho.width = 40
	if !strings.Contains(estrecho.headerSection().text, "refreshing") {
		t.Errorf("con 40 columnas el spinner desaparece: %q", estrecho.headerSection().text)
	}
}

// The negative case of compactCount.
func TestUnRecuentoNegativoNoSePresentaComoUnoRedondeado(t *testing.T) {
	for _, c := range []struct {
		n    int
		want string
	}{
		{0, "0"},
		{1, "1"},
		{999, "999"},
		{1000, "1.0k"},
		{1050, "1.1k"},
		{1500, "1.5k"},
		// 9999 rounds UP to "10k", not down to "9.9k", and that comes out as five runes.
		{9999, "10k"},
		{10000, "10k"},
		{999999, "999k"},
		{1000000, "1000k"},
		{-1, "-1"},
		{-1500, "-1500"},
	} {
		if got := compactCount(c.n); got != c.want {
			t.Errorf("compactCount(%d) = %q, want %q", c.n, got, c.want)
		}
	}
}

// diffSpans only colours text that really is two signed digits.
func TestUnDiffstatQueNoEsDosCifrasConSignoNoSePinta(t *testing.T) {
	for _, c := range []struct {
		nombre string
		plain  string
		pinta  bool
	}{
		{"los dos con signo", "+12 -3", true},
		// The queue goes AFTER the deleted count, which is why "+12 files -3" does not read as a net.
		{"con unidades al final", "+12 -3 archivos", true},
		{"unidades en medio", "+12 archivos -3", false},
		{"sin el separado", "+12", false},
		{"sin signos", "12 3", false},
		{"vacío", "", false},
		{"texto que no es un recuento", "no changes", false},
		{"un solo signo", "+12 -", false},
		{"signo suelto", "- 3", false},
		{"cola vacía", "+12 -3   ", true},
	} {
		spans := diffSpans(c.plain)
		pinta := len(spans) > 0
		if pinta != c.pinta {
			t.Errorf("%s: pinta=%v, want %v (spans=%v)", c.nombre, pinta, c.pinta, spans)
		}
		for _, s := range spans {
			if s.text == "" {
				t.Errorf("%s: hay un span vacío en %v", c.nombre, spans)
			}
		}
	}
}
