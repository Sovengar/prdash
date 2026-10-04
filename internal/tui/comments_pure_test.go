package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"prdash/internal/sim"
)

// Text that does not fit has to SAY that something was lost.
func TestClipRunesMarcaLoQueSePerdio(t *testing.T) {
	corta := "abcdef"
	casos := []struct {
		n    int
		want string
	}{
		{6, "abcdef"},   // cabe justo: intacto
		{7, "abcdef"},   // sobra sitio: intacto
		{100, "abcdef"}, // mucho de sobra: intacto
		{1, "…"},        // solo cabe la marca
		{2, "a…"},       // un carácter y la marca
		{3, "ab…"},
		{5, "abcd…"},
		{0, ""},   // sin sitio, sin marca
		{-1, ""},  // negativo, sin marca
		{-50, ""}, // muy negativo, sin marca
	}
	for _, c := range casos {
		if got := clipRunes([]rune(corta), c.n); got != c.want {
			t.Errorf("clipRunes(%q, %d) = %q, want %q", corta, c.n, got, c.want)
		}
	}
	for _, n := range []int{-1, 0, 1, 5} {
		if got := clipRunes(nil, n); got != "" {
			t.Errorf("clipRunes(nil, %d) = %q, want %q", n, got, "")
		}
	}

	multibyte := []rune("ñáé")
	if got := clipRunes(multibyte, 3); got != "ñáé" {
		t.Errorf("clipRunes con 3 runes dio %q, want el texto entero: la cuenta es en runes, no en bytes", got)
	}
	if got := clipRunes(multibyte, 2); got != "ñ…" {
		t.Errorf("clipRunes con 2 runes dio %q, want %q", got, "ñ…")
	}
	// The result can always be printed and measured: a cut in the middle of a utf-8 sequence breaks
	// the width.
	for n := 1; n <= 8; n++ {
		got := clipRunes([]rune("ñáéíóú"), n)
		if !utf8Valido(got) {
			t.Errorf("clipRunes a %d dio %q, que no es utf-8 válido", n, got)
		}
		if w := ansi.StringWidth(got); w > max(n, 1) {
			t.Errorf("clipRunes a %d dio %q de %d columnas, que no caben", n, got, w)
		}
	}
}

func utf8Valido(s string) bool {
	for _, r := range s {
		if r == '�' {
			return false
		}
	}
	return true
}

func TestCommentBoxWidthRespetaElSueloYElSangrado(t *testing.T) {
	for outer := -10; outer <= 60; outer++ {
		got := commentBoxWidth(outer)
		want := max(8, outer-2*commentInset)
		if got != want {
			t.Errorf("commentBoxWidth(%d) = %d, want %d", outer, got, want)
		}
	}
	// The floor is exactly 8, not "almost 8": with 9 of outer width the interior is 7.
	for _, outer := range []int{0, 5, 9, 10} {
		if got := commentBoxWidth(outer); got != 8 {
			t.Errorf("commentBoxWidth(%d) = %d, want el suelo de 8", outer, got)
		}
	}
	if got := commentBoxWidth(11); got != 9 {
		t.Errorf("commentBoxWidth(11) = %d, want 9: a partir de aquí el sangrado manda sobre el suelo", got)
	}
	for outer := 10; outer <= 40; outer++ {
		if got := commentBoxWidth(outer); got != outer-2 {
			t.Errorf("commentBoxWidth(%d) = %d, want %d", outer, got, outer-2)
		}
	}
}

// The padding is measured in COLUMNS, not bytes.
func TestPadRightAlineaPorColumnasNoPorBytes(t *testing.T) {
	for _, s := range []string{"", "a", "ab", "uno", "a-label largo"} {
		for n := 0; n <= 20; n++ {
			got := padRight(s, n)
			if w := ansi.StringWidth(got); w != max(n, ansi.StringWidth(s)) {
				t.Errorf("padRight(%q, %d) mide %d columnas, want %d", s, n, w, max(n, ansi.StringWidth(s)))
			}
			ifTrimmed := strings.TrimRight(got, " ")
			if ifTrimmed != s {
				t.Errorf("padRight(%q, %d) = %q, want el mismo texto con relleno", s, n, got)
			}
		}
	}
	if got := padRight("ñ", 5); ansi.StringWidth(got) != 5 {
		t.Errorf("padRight(%q, 5) mide %d columnas, want 5: el relleno cuenta columnas, no bytes", "ñ", ansi.StringWidth(got))
	}
	conColor := "\x1b[31mrojo\x1b[0m"
	if got := padRight(conColor, 10); ansi.StringWidth(got) != 10 {
		t.Errorf("padRight con ANSI mide %d columnas, want 10: los códigos de color no son columnas", ansi.StringWidth(got))
	}
	if !strings.Contains(padRight(conColor, 10), "\x1b[31m") {
		t.Error("padRight se comió el color del texto")
	}
}

func TestAllocateReparteLasFilasSinQueDependaDelOrden(t *testing.T) {
	casos := []struct {
		name   string
		need   []int
		budget int
		want   []int
	}{
		{
			"sobra para todos: cada uno pide lo suyo",
			[]int{1, 2, 3}, 10,
			[]int{1, 2, 3},
		},
		{
			"exacto: nadie recibe de más",
			[]int{2, 2}, 4,
			[]int{2, 2},
		},
		{
			// The one with least does NOT get it: it goes to whoever has least AND STILL WANTS more.
			"el que menos tiene no recibe si ya tiene todo su texto",
			[]int{3, 1}, 4,
			[]int{3, 1},
		},
		{
			// The levelling is real: one asking a lot and two asking little all end up the same.
			"el grande no se come el presupuesto",
			[]int{4, 2, 2}, 8,
			[]int{4, 2, 2},
		},
		{
			"nadie absorbe más: sobra presupuesto y no se reparte",
			[]int{1, 1}, 10,
			[]int{1, 1},
		},
		{
			"uno solo, con presupuesto de sobra",
			[]int{4}, 10,
			[]int{4},
		},
		{
			"uno solo, con presupuesto corto",
			[]int{4}, 3,
			[]int{3},
		},
		{
			// The budget can be SMALLER than the number of comments, and then everyone gets one row.
			"el presupuesto es menor que los comentarios: todos a una fila, y se pasa",
			[]int{5, 5, 5}, 2,
			[]int{1, 1, 1},
		},
		{
			"presupuesto cero: todos a una fila igualmente",
			[]int{5, 5}, 0,
			[]int{1, 1},
		},
		{
			"nada que repartir",
			nil, 10,
			nil,
		},
	}
	for _, c := range casos {
		t.Run(c.name, func(t *testing.T) {
			got := allocate(c.need, c.budget)
			if !mismoInt(got, c.want) {
				t.Errorf("allocate(%v, %d) = %v, want %v", c.need, c.budget, got, c.want)
			}
			// The sum never goes over the budget, UNLESS the budget is smaller than the number of comments.
			suma := 0
			for _, r := range got {
				suma += r
			}
			if techo := max(c.budget, len(c.need)); suma > techo {
				t.Errorf("allocate(%v, %d) repartió %d filas, más que el techo de %d",
					c.need, c.budget, suma, techo)
			}
			// And never less than one row per comment that asked for something.
			for i, r := range got {
				if c.need[i] > 0 && r < 1 {
					t.Errorf("allocate(%v, %d)[%d] = %d: un comentario necesita al menos una fila para verse",
						c.need, c.budget, i, r)
				}
			}
			for i, r := range got {
				if r > c.need[i] {
					t.Errorf("allocate(%v, %d)[%d] = %d, más de lo que pidió", c.need, c.budget, i, r)
				}
			}
		})
	}

	// The result does not depend on the input ORDER: the same numbers in another order allocate the
	// same.
	base := allocate([]int{6, 1, 1, 6, 1}, 12)
	for _, perm := range [][]int{
		{1, 6, 1, 6, 1},
		{1, 1, 6, 1, 6},
		{6, 6, 1, 1, 1},
	} {
		got := allocate(perm, 12)
		if !mismoMultiset(base, got) {
			t.Errorf("allocate con las mismas necesidades en otro orden dio un reparto distinto: %v vs %v", base, got)
		}
	}
	if base[1] == 1 && base[2] == 1 && base[0] == 6 {
		t.Error("el reparto concentrations el presupuesto en los primeros: eso solo depende del orden de llegada")
	}
}

func mismoInt(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func mismoMultiset(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	copia := append([]int(nil), a...)
	for _, v := range b {
		en := -1
		for i, c := range copia {
			if c == v {
				en = i
				break
			}
		}
		if en < 0 {
			return false
		}
		copia = append(copia[:en], copia[en+1:]...)
	}
	return true
}

// The kind selector is circular.
func TestElCursorDelSelectorDaLaVueltaPorLosDosLados(t *testing.T) {
	restore := simKinds
	simKinds = []sim.Kind{sim.KindMerge, sim.KindRebase, sim.Kind("otro")}
	t.Cleanup(func() { simKinds = restore })

	// THREE kinds on purpose, the third being synthetic: with the two real ones, cursor-1 and
	//cursor+1 would both land on the only one and the wrap would be untestable.
	n := len(simKinds)
	if n < 3 {
		t.Fatalf("hacen falta 3 kinds para que +1 y -1 se distinguan, hay %d", n)
	}
	m := simModel(t, &fakeSimulator{available: true})
	m = press(t, m, "v")

	m.sim.cursor = 0
	for i := range n {
		m = press(t, m, "j")
		if want := (i + 1) % n; m.sim.cursor != want {
			t.Fatalf("tras %d pasos abajo el cursor = %d, want %d", i+1, m.sim.cursor, want)
		}
	}

	m.sim.cursor = 0
	m = press(t, m, "k")
	if m.sim.cursor != n-1 {
		t.Errorf("arriba desde el primero dio %d, want %d (el último): un índice negativo aquí es un panic", m.sim.cursor, n-1)
	}
	m.sim.cursor = 0
	for i := range n {
		m = press(t, m, "k")
		if want := (n - 1 - i) % n; m.sim.cursor != want {
			t.Fatalf("tras %d pasos arriba el cursor = %d, want %d", i+1, m.sim.cursor, want)
		}
	}

	for _, alias := range []string{"j", "right", "tab"} {
		m.sim.cursor = 0
		m = press(t, m, alias)
		if m.sim.cursor != 1 {
			t.Errorf("la tecla %q dio el cursor %d, want 1 (es alias de abajo)", alias, m.sim.cursor)
		}
	}
	m.sim.cursor = 0
	m = press(t, m, "k")
	if m.sim.cursor != n-1 {
		t.Errorf("la tecla k dio el cursor %d, want %d (es alias de arriba)", m.sim.cursor, n-1)
	}
}
