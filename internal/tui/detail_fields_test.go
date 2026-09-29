package tui

import (
	"strings"
	"testing"
	"time"

	"prdash/internal/forge/model"
)

// TestReviewLabelCaeAlEstadoCuandoNoHayDecision: la etiqueta del detalle dice
// cómo está el ítem, y sin decisión de review la única información real es el
// estado del forge. Sin ese cae, la etiqueta sale vacía y el detalle no dice nada
// del estado de un PR abierto sin reviews.
//
// Y el tipo de review se traduce: "review requested" y "assigned" son cosas
// distintas para el operador, aunque la decisión sea la misma.
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
			// Un valor de decisión que no es ninguno de los tres conocidos se
			// enseña tal cual: es dato del forge, no se inventa una traducción.
			"decisión desconocida tal cual",
			model.Item{State: "OPEN", ReviewDecision: "DISMISSED"},
			"DISMISSED",
		},
		{
			// Sin decisión NI estado: la etiqueta queda vacía, que es el dato
			// honesto cuando el forge no dijo nada.
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

// TestChecksDetailSeparaLoQueNoSeSabeDeLoQueEstaEnVerde: "no checks" y "passing
// (0)" son afirmaciones distintas. La primera dice que no se consultó, la segunda
// que se consultó y no había ninguno. Confundirlas hace que un repo sin CI (o con
// el dato sin traer) parezca un CI verde, que es la forma más caro de mentir que
// tiene la TUI.
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
			// Total == 0 con un estado que NO es unknown es un dato conocido con
			// cero checks, no la ausencia de datos.
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

// TestDiffDetailDistingueDesconocidoDeSinCambios: un diffstat que el forge no
// reportó y un MR que no toca ningún fichero son cosas distintas, y la segunda no
// se puede saber sin el dato. Aquí se afirma el texto entero porque es la
// distinción de la que depende el gate.
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

// TestAuthReasonNoConfundeLasDosCosasQueNoSeArreglanIgual: el motivo de un forge
// no autenticado decide qué hace el operador. "not authenticated" se arregla
// retomando el token; "not implemented" (un adapter que no soporta la acción) no
// se arregla de ninguna forma. Sin motivo, el texto por defecto es el que lleva a
// la sesión de depuración larga, que es justo lo que el comentario de la función
// dice evitar.
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

// TestDetailWarningsAvisaPorForgeYPorMotivoDeAccion: son dos filas distintas con
// dos potenciales órdenes distintos, y el motivo de la acción deshabilitada tiene
// prioridad sobre el del forge: el motivo de la acción es el que el usuario acaba
// de provocar, y mandarlo al aviso del forge lo lleva a un sitio donde no está el
// problema.
func TestDetailWarningsAvisaPorForgeYPorMotivoDeAccion(t *testing.T) {
	m := newTestModel(t, ghAdapter())
	it := mkItem("github", "github.com", "acme/widget", "Uno", 1, "")

	// Forge autenticado y sin motivo de acción: sin avisos.
	if got := m.detailWarnings(it); len(got) != 0 {
		t.Errorf("sin nada que avisar: %v", got)
	}

	// Forge no autenticado: avisa con el motivo. El slice lleva una línea vacía
	// delante, que es el separador que el detalle usa para no pegar el aviso al
	// campo de arriba.
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

// TestClipTopRecortaPorArribaYNoDejaHuecos: el recorte del detalle tiene
// que enseñar el final de una lista larga (donde está el dato que importa, el
// estado) y no la cabeza. Y con más líneas de las que caben, tiene que devolver
// justo las que caben.
func TestClipTopRecortaPorArribaYNoDejaHuecos(t *testing.T) {
	lineas := []string{"una", "dos", "tres", "cuatro"}

	// Cabe entero: no recorta.
	if got := clipTop(lineas, 4); len(got) != 4 {
		t.Errorf("con 4 filas para 4 líneas = %d, want 4", len(got))
	}
	if got := clipTop(lineas, 10); len(got) != 4 {
		t.Errorf("con hueco de sobra = %d, want 4 (no rellena)", len(got))
	}
	// No cabe: recorta, y devuelve el número exacto que se pidió.
	if got := clipTop(lineas, 2); len(got) != 2 {
		t.Fatalf("con 2 filas para 4 líneas = %d, want 2", len(got))
	}
	// Sin filas no hay recorte posible, así que la entrada vuelve intacta: es un
	// passthrough, no un recorte a cero. Quien llama ya sabe que no hay espacio, y
	// devolver la lista entera deja que sea el layout quien decida.
	for _, rows := range []int{0, -1} {
		if got := clipTop(lineas, rows); len(got) != len(lineas) {
			t.Errorf("rows=%d devolvió %d líneas, want las %d intactas (no hay recorte posible)", rows, len(got), len(lineas))
		}
	}
	if got := clipTop(nil, 3); len(got) != 0 {
		t.Errorf("sin líneas devolvió %d", len(got))
	}
	// Y una sola línea con hueco de sobra no se rellena con blancos: el detalle
	// no necesita un bloque de altura fija.
	if got := clipTop([]string{"x"}, 5); len(got) != 1 {
		t.Errorf("una línea con 5 filas = %d, want 1", len(got))
	}
}

// TestRelTimeNoInventaUnNumeroNegativo: la antigüedad decide si un ítem parece
// recién tocado o abandonado, así que los cortes tienen que estar donde dicen. El
// de días es el que más duele cuando se mueve: un corte movido por un día
// convierte un "hace 2 días" en "hace 3" y viceversa, y ambos son afirmaciones
// sobre urgency.
//
// Y el "ahora" de menos de un minuto no lleva unidades: un "-0m" se lee como un
// dato raro, y la información de que acaba de pasar no necesita más.
func TestRelTimeNoInventaUnNumeroNegativo(t *testing.T) {
	// Se mide contra ahora con tolerancias, porque relTime usa time.Now().
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
	// Una fecha en el futuro (un reloj desfasado o un fixture mal hecho) no puede
	// dar un número negativo: sería un dato que nadie sabe leer.
	if got := relativeTime(time.Now().Add(24 * time.Hour)); strings.HasPrefix(got, "-") {
		t.Errorf("una fecha futura dio %q, que se lee como un número negativo", got)
	}
	// Y la fecha cero es un guion, no un "-20000d": el forge no la trajo y eso
	// se dice con el mismo guion que el resto de campos sin dato.
	if got := relativeTime(time.Time{}); got != "-" {
		t.Errorf("una fecha sin dato dio %q, want \"-\"", got)
	}
}

// TestOrDashNoDejaUnCampoVacio: un campo sin dato se enseña como "-", no como un
// hueco. Un hueco en el detalle se lee como un campo que se olvidó de rellenar.
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
