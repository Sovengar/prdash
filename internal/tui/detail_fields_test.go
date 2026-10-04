package tui

import (
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/charmbracelet/x/ansi"

	"prdash/internal/forge/model"
)

func TestReviewLabelCaeAlEstadoCuandoNoHayDecision(t *testing.T) {
	cases := []struct {
		name string
		it   model.Item
		want string
	}{
		{
			"sin decisión cae al estado",
			model.Item{State: "OPEN", ReviewDecision: ""},
			"OPEN",
		},
		{
			"aprobada sin tipo de review",
			model.Item{State: "OPEN", ReviewDecision: "APPROVED"},
			"approved",
		},
		{
			"cambios pedidos",
			model.Item{State: "OPEN", ReviewDecision: "CHANGES_REQUESTED"},
			"changes requested",
		},
		{
			"review pendiente",
			model.Item{State: "OPEN", ReviewDecision: "REVIEW_REQUIRED"},
			"review required",
		},
		{
			"review solicitado",
			model.Item{State: "OPEN", ReviewDecision: "APPROVED", ReviewKind: model.ReviewRequested},
			"approved · review requested",
		},
		{
			"asignado",
			model.Item{State: "OPEN", ReviewDecision: "APPROVED", ReviewKind: model.ReviewAssigned},
			"approved · assigned",
		},
		{
			"decisión desconocida tal cual",
			model.Item{State: "OPEN", ReviewDecision: "DISMISSED"},
			"DISMISSED",
		},
		{
			// With neither decision nor state the label is empty, which is the honest datum.
			"sin nada que decir",
			model.Item{},
			"",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := reviewLabel(c.it); got != c.want {
				t.Errorf("reviewLabel() = %q, want %q", got, c.want)
			}
		})
	}
}

// "no checks" and "passing (0)" are different claims.
func TestChecksDetailSeparaLoQueNoSeSabeDeLoQueEstaEnVerde(t *testing.T) {
	cases := []struct {
		name   string
		checks model.Checks
		want   string
	}{
		{
			"sin datos: no se sabe",
			model.Checks{State: model.ChecksUnknown},
			"no checks",
		},
		{
			"total cero pero con estado conocido: hay recuento",
			model.Checks{State: model.ChecksPassing},
			"passing (0)",
		},
		{
			"todo verde",
			model.Checks{Total: 3, State: model.ChecksPassing},
			"passing (3)",
		},
		{
			"con fallidos",
			model.Checks{Total: 5, Failing: 2, State: model.ChecksFailing},
			"failing (2/5 failing, 0 pending)",
		},
		{
			"con pendientes",
			model.Checks{Total: 4, Pending: 1, State: model.ChecksPending},
			"pending (0/4 failing, 1 pending)",
		},
		{
			"con los dos",
			model.Checks{Total: 6, Failing: 1, Pending: 2, State: model.ChecksFailing},
			"failing (1/6 failing, 2 pending)",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := checksDetail(c.checks); got != c.want {
				t.Errorf("checksDetail(%+v) = %q, want %q", c.checks, got, c.want)
			}
		})
	}
}

func TestDiffDetailDistingueDesconocidoDeSinCambios(t *testing.T) {
	cases := []struct {
		name string
		d    model.DiffStat
		want string
	}{
		{"desconocido", model.DiffStat{}, "unknown (forge did not report it)"},
		{"conocido sin cambios", model.DiffStat{Known: true}, "no changes"},
		{"un fichero", model.DiffStat{Known: true, Additions: 1, Deletions: 0, Files: 1}, "+1 -0 (1 file)"},
		{"varios", model.DiffStat{Known: true, Additions: 40, Deletions: 2, Files: 3}, "+40 -2 (3 files)"},
		{"solo borrados", model.DiffStat{Known: true, Deletions: 5, Files: 1}, "+0 -5 (1 file)"},
		{"sin ficheros pero con cambios", model.DiffStat{Known: true, Additions: 3, Deletions: 1}, "+3 -1 (0 files)"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := diffDetail(c.d); got != c.want {
				t.Errorf("diffDetail(%+v) = %q, want %q", c.d, got, c.want)
			}
		})
	}
}

// An unauthenticated forge and an unimplemented one are not the same repair.
func TestAuthReasonNoConfundeLasDosCosasQueNoSeArreglanIgual(t *testing.T) {
	if got := authReason(model.AuthState{}); got != "not authenticated" {
		t.Errorf("sin motivo = %q, want \"not authenticated\"", got)
	}
	for _, reason := range []string{
		"GH_TOKEN is not set",
		"not implemented: Bitbucket has no approvals",
		"token expirado",
	} {
		if got := authReason(model.AuthState{Reason: reason}); got != reason {
			t.Errorf("authReason(%q) = %q, want el motivo del adapter", reason, got)
		}
	}
}

func TestDetailWarningsAvisaPorForgeYPorMotivoDeAccion(t *testing.T) {
	m := newTestModel(t, ghAdapter())
	it := mkItem("github", "github.com", "acme/widget", "Uno", 1, "")

	// Forge autenticado y sin motivo de acción: sin avisos.
	if got := m.detailWarnings(it); len(got) != 0 {
		t.Errorf("sin nada que avisar: %v", got)
	}

	m.statuses["github"].auth = model.AuthState{Forge: "github", OK: false, Reason: "GH_TOKEN no vale"}
	got := m.detailWarnings(it)
	if len(got) != 2 {
		t.Fatalf("aviso = %q, want 2 (separador + aviso)", got)
	}
	if got[0] != "" {
		t.Errorf("la primera línea del aviso debería ser un separador vacío, dio %q", got[0])
	}
	if !strings.Contains(stripANSI(got[1]), "GH_TOKEN no vale") || !strings.Contains(stripANSI(got[1]), "github") {
		t.Errorf("el aviso debería nombrar el forge y el motivo: %q", stripANSI(got[1]))
	}

	// Forge no autenticado Y motivo de acción: manda el de la acción.
	m.denied[it.ID()] = "the branch conflicts with main"
	got = m.detailWarnings(it)
	if len(got) != 2 {
		t.Fatalf("aviso = %q, want 2 (la acción manda)", got)
	}
	if s := stripANSI(got[1]); !strings.Contains(s, "the branch conflicts with main") {
		t.Errorf("el aviso debería ser el motivo de la acción: %q", s)
	}
	if s := stripANSI(got[1]); strings.Contains(s, "GH_TOKEN no vale") {
		t.Errorf("con motivo de acción no debería salir el del forge: %q", s)
	}
}

// The cut has to show the END.
func TestClipTopRecortaPorArribaYNoDejaHuecos(t *testing.T) {
	lineas := []string{"una", "dos", "tres", "cuatro"}

	if got := clipTop(lineas, 4); len(got) != 4 {
		t.Errorf("con 4 filas para 4 líneas = %d, want 4", len(got))
	}
	if got := clipTop(lineas, 10); len(got) != 4 {
		t.Errorf("con hueco de sobra = %d, want 4 (no rellena)", len(got))
	}
	if got := clipTop(lineas, 2); len(got) != 2 {
		t.Fatalf("con 2 filas para 4 líneas = %d, want 2", len(got))
	}
	// With no rows there is no clipping possible, so the entry comes back whole: it is a
	// passthrough.
	for _, rows := range []int{0, -1} {
		if got := clipTop(lineas, rows); len(got) != len(lineas) {
			t.Errorf("rows=%d devolvió %d líneas, want las %d intactas (no hay recorte posible)", rows, len(got), len(lineas))
		}
	}
	if got := clipTop(nil, 3); len(got) != 0 {
		t.Errorf("sin líneas devolvió %d", len(got))
	}
	if got := clipTop([]string{"x"}, 5); len(got) != 1 {
		t.Errorf("una línea con 5 filas = %d, want 1", len(got))
	}
}

// The detail is a two-column card and its geometry is the whole point.
func TestLosCamposDelDetalleNoSePisanNiSeComen(t *testing.T) {
	largo := detailField{key: "checks", value: strings.Repeat("x", 200)}

	for _, inner := range []int{30, 40, 60, 80, 120} {
		cell := max(24, (inner-detailGap)/2)
		linea, w := detailCell(largo, cell)
		if w > cell {
			t.Errorf("inner %d (celda %d): el campo mide %d columnas, want <= %d: %q",
				inner, cell, w, cell, ansi.Strip(linea))
		}
		// Y el ancho devuelto es el real, en texto plano: la etiqueta más el
		// valor recortado.
		want := labelWidth + utf8.RuneCountInString(stripANSI(linea)) - labelWidth
		if w != want {
			t.Errorf("inner %d: el ancho devuelto es %d pero el valor mide %d: el padding se calcularía mal",
				inner, w, want)
		}
	}

	// The cell never drops below 24 however narrow the interior, because below that the label does
	// not fit.
	for _, inner := range []int{10, 20, 28, 30} {
		if cell := max(24, (inner-detailGap)/2); cell != 24 {
			t.Errorf("inner %d dio celda de %d columnas, want el mínimo de 24", inner, cell)
		}
	}

	// A grid row never exceeds the interior, which is what keeps the two columns apart.
	for _, inner := range []int{10, 28, 30, 40} {
		filas := detailGrid([]detailField{largo, largo}, inner)
		if len(filas) != 1 {
			t.Fatalf("inner %d: %d filas, want 1", inner, len(filas))
		}
		want := 2*24 + detailGap
		if w := ansi.StringWidth(filas[0]); w != want {
			t.Errorf("inner %d: la fila mide %d columnas, want %d (dos celdas de 24 más el hueco de %d): %q",
				inner, w, want, detailGap, ansi.Strip(filas[0]))
		}
	}
	for _, inner := range []int{60, 80, 120, 200} {
		for _, line := range detailGrid([]detailField{largo, largo, largo}, inner) {
			if w := ansi.StringWidth(line); w > inner {
				t.Errorf("inner %d: la fila mide %d columnas, want <= %d: %q", inner, w, inner, ansi.Strip(line))
			}
		}
	}

	for _, inner := range []int{20, 38, 60, 160} {
		linea := fullWidthField(detailField{key: "url", value: "https://gitlab.example.com/grp/proj/-/merge_requests/1"}, inner)
		if w := ansi.StringWidth(linea); w > inner {
			t.Errorf("inner %d: el campo de ancho completo mide %d columnas, want <= %d: %q",
				inner, w, inner, ansi.Strip(linea))
		}
	}
	// And with a 1-column interior it does not panic: the max(1, ...) prevents a negative.
	for _, inner := range []int{0, 1, 2} {
		linea := fullWidthField(detailField{key: "u", value: "valor"}, inner)
		if w := ansi.StringWidth(linea); w != labelWidth+1 {
			t.Errorf("inner %d: el campo mínimo mide %d columnas, want %d (etiqueta + 1)",
				inner, w, labelWidth+1)
		}
	}
}

// The age decides whether an item looks freshly touched or abandoned.
func TestRelTimeNoInventaUnNumeroNegativo(t *testing.T) {
	for _, tc := range []struct {
		atras time.Duration
		want  string
	}{
		{0, "now"},
		{30 * time.Second, "now"},
		{59 * time.Second, "now"},
		{61 * time.Second, "1m"},
		{59 * time.Minute, "59m"},
		{61 * time.Minute, "1h"},
		{23 * time.Hour, "23h"},
		{25 * time.Hour, "1d"},
		{6 * 24 * time.Hour, "6d"},
		{30 * 24 * time.Hour, "30d"},
	} {
		got := relativeTime(time.Now().Add(-tc.atras))
		if got != tc.want {
			t.Errorf("relativeTime(hace %v) = %q, want %q", tc.atras, got, tc.want)
		}
	}
	// A date in the future (a skewed clock or a bad fixture) cannot give a negative age.
	if got := relativeTime(time.Now().Add(24 * time.Hour)); strings.HasPrefix(got, "-") {
		t.Errorf("una fecha futura dio %q, que se lee como un número negativo", got)
	}
	// And the zero date is a dash, not a "-20000d": the forge did not send it.
	if got := relativeTime(time.Time{}); got != "-" {
		t.Errorf("una fecha sin dato dio %q, want \"-\"", got)
	}
}

func TestOrDashNoDejaUnCampoVacio(t *testing.T) {
	if got := orDash(""); got != "-" {
		t.Errorf("orDash(\"\") = %q, want \"-\"", got)
	}
	if got := orDash("valor"); got != "valor" {
		t.Errorf("orDash(\"valor\") = %q, want el valor intacto", got)
	}
	// Un valor de un solo espacio ES dato: no se descarta.
	if got := orDash(" "); got != " " {
		t.Errorf("orDash(\" \") = %q, want el espacio intacto", got)
	}
}
