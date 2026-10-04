package tui

import (
	"slices"
	"strings"
	"testing"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"

	"prdash/internal/sim"
)

// The padding decides the box's HEIGHT.
func TestPadLinesRellenaHastaLaAlturaPedida(t *testing.T) {
	for _, tengo := range []int{0, 1, 3, 5, 12, 40} {
		lineas := make([]string, tengo)
		for i := range lineas {
			lineas[i] = "x"
		}
		for _, quiero := range []int{0, 1, 3, 5, 12, 40} {
			got := padLines(lineas, quiero)

			if len(got) < tengo {
				t.Errorf("padLines con %d líneas y objetivo %d devolvió %d: se perdió contenido",
					tengo, quiero, len(got))
			}
			if len(got) < quiero {
				t.Errorf("padLines con %d líneas y objetivo %d devolvió %d: no rellenó",
					tengo, quiero, len(got))
			}
			for i := tengo; i < len(got); i++ {
				if got[i] != "" {
					t.Errorf("la línea de relleno %d es %q, want una línea vacía", i, got[i])
				}
			}
			for i := range tengo {
				if got[i] != "x" {
					t.Errorf("la línea %d pasó a ser %q, want el contenido original", i, got[i])
				}
			}
		}
	}
	for _, quiero := range []int{-5, -1, 0} {
		if got := padLines([]string{"a", "b"}, quiero); len(got) != 2 {
			t.Errorf("padLines con objetivo %d devolvió %d líneas, want las 2 que había", quiero, len(got))
		}
		if got := padLines(nil, quiero); len(got) != 0 {
			t.Errorf("padLines(nil, %d) devolvió %d líneas, want ninguna", quiero, len(got))
		}
	}
	// The input slice is not touched: padding copies, it does not mutate.
	orig := []string{"a", "b"}
	padLines(orig, 10)
	if len(orig) != 2 || orig[0] != "a" || orig[1] != "b" {
		t.Errorf("padLines mutó el slice de entrada: %q", orig)
	}
}

// The sim popup's height is FIXED.
func TestLaCajaDeSimMideLoQueElRellenoManda(t *testing.T) {
	restore := simKinds
	simKinds = []sim.Kind{sim.KindMerge, sim.KindRebase, sim.Kind("otro")}
	t.Cleanup(func() { simKinds = restore })

	for _, h := range []int{20, 30, 45, 60} {
		m := showSim(t, solidSim(4, 3, negro))
		m = send(t, m, tea.WindowSizeMsg{Width: 100, Height: h})
		_, want := m.simBox()

		for _, celdas := range []int{0, 1, 5} {
			m.sim.cells = make([]string, celdas)
			for i := range m.sim.cells {
				m.sim.cells[i] = "celda"
			}
			lineas := strings.Split(stripANSI(m.simImageBox()), "\n")
			if len(lineas) != want {
				t.Errorf("terminal de %d filas con %d celdas: la caja mide %d, want %d: el relleno no llegó",
					h, celdas, len(lineas), want)
			}
		}

		m.sim.cells = make([]string, 40)
		for i := range m.sim.cells {
			m.sim.cells[i] = "celda"
		}
		lineas := strings.Split(stripANSI(m.simImageBox()), "\n")
		if quiere := 40 + simChrome; len(lineas) != quiere {
			t.Errorf("con 40 celdas en una caja de %d la caja mide %d, want %d: el relleno no es un recorte",
				want, len(lineas), quiere)
		}
	}
}

// The cursor is the row enter picks.
func TestSoloUnaFilaDelSelectorLlevaElCursor(t *testing.T) {
	restore := simKinds
	simKinds = []sim.Kind{sim.KindMerge, sim.KindRebase, sim.Kind("otro")}
	t.Cleanup(func() { simKinds = restore })

	m := simModel(t, &fakeSimulator{available: true})
	m = press(t, m, "v")

	for cursor := range len(simKinds) {
		m.sim.cursor = cursor
		marcadas := 0
		var conMarca string
		for _, l := range strings.Split(stripANSI(m.simChooserBox()), "\n") {
			if !strings.Contains(l, "▸") {
				continue
			}
			marcadas++
			conMarca = l
		}
		if marcadas != 1 {
			t.Errorf("cursor %d: %d filas marcadas, want exactamente 1", cursor, marcadas)
		}
		if marcadas == 1 && !strings.Contains(conMarca, string(simKinds[cursor])) {
			t.Errorf("cursor %d: la fila marcada es %q, want la de %q", cursor, strings.TrimSpace(conMarca), simKinds[cursor])
		}
	}
}

// Each strategy's label is padded.
func TestElFlujoDelSelectorEmpiezaEnUnaColumnaFija(t *testing.T) {
	restore := simKinds
	simKinds = []sim.Kind{sim.KindMerge, sim.KindRebase, sim.Kind("12345678")}
	t.Cleanup(func() { simKinds = restore })

	m := simModel(t, &fakeSimulator{available: true})
	m = press(t, m, "v")

	caja := stripANSI(m.simChooserBox())
	// The flow's start is measured by the branch NAME, not the arrow.
	flujos := []int{}
	for kind, rama := range map[sim.Kind]string{
		sim.KindMerge:        "main",
		sim.KindRebase:       "feat/x",
		sim.Kind("12345678"): "main",
	} {
		l := lineaConKind(t, caja, kind)
		i := strings.Index(l, rama)
		if i < 0 {
			t.Fatalf("la fila de %q no nombra %q: %q", kind, rama, strings.TrimSpace(l))
		}
		flujos = append(flujos, utf8.RuneCountInString(l[:i]))
	}
	slices.Sort(flujos)
	for i := 1; i < len(flujos); i++ {
		if flujos[i] != flujos[0] {
			t.Errorf("el flujo de %q empieza en la columna %d y el de %q en %d: la etiqueta no está rellenada igual",
				simKinds[i], flujos[i], simKinds[0], flujos[0])
		}
	}
	// All three rows land in the SAME column.
	if flujos[0] != flujos[1] || flujos[1] != flujos[2] {
		t.Errorf("los flujos empiezan en las columnas %v: la etiqueta no se rellena al mismo ancho", flujos)
	}
}

// The merge and rebase arrows point in opposite directions.
func TestLaFlechaDelMergeYLaDelRebaseApuntanEnSentidosOpuestos(t *testing.T) {
	restore := simKinds
	simKinds = []sim.Kind{sim.KindMerge, sim.KindRebase}
	t.Cleanup(func() { simKinds = restore })

	m := simModel(t, &fakeSimulator{available: true})
	m = press(t, m, "v")

	caja := stripANSI(m.simChooserBox())
	merge := lineaConKind(t, caja, sim.KindMerge)
	if !strings.Contains(merge, "←") {
		t.Errorf("la fila de merge debería usar la flecha que trae la rama a la base, dio %q", strings.TrimSpace(merge))
	}
	rebase := lineaConKind(t, caja, sim.KindRebase)
	if !strings.Contains(rebase, "→") {
		t.Errorf("la fila de rebase debería usar la flecha que lleva la base a la rama, dio %q", strings.TrimSpace(rebase))
	}
	if strings.Contains(merge, "→") || strings.Contains(rebase, "←") {
		t.Errorf("las dos operaciones comparten sentido: merge=%q rebase=%q", strings.TrimSpace(merge), strings.TrimSpace(rebase))
	}
	for _, l := range []string{merge, rebase} {
		if !strings.Contains(l, "main") {
			t.Errorf("la fila %q no nombra la base: sin las dos ramas la flecha no dice nada", strings.TrimSpace(l))
		}
	}
}

func lineaConKind(t *testing.T, caja string, kind sim.Kind) string {
	t.Helper()
	for _, l := range strings.Split(caja, "\n") {
		if strings.Contains(l, string(kind)) {
			return l
		}
	}
	t.Fatalf("no hay fila para %q en:\n%s", kind, caja)
	return ""
}
