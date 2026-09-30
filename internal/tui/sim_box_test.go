package tui

import (
	"slices"
	"strings"
	"testing"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"

	"prdash/internal/sim"
)

// TestPadLinesRellenaHastaLaAlturaPedida: el relleno decide el ALTO de la caja del
// popup de sim, y por eso vive en una función propia. Dentro del pintado, una caja
// una fila más corta no se ve como una caja más corta: se ve como una caja con el
// pie pegado al borde de arriba, que es un defecto que nadie describe.
//
// Y el relleno es un SUELO, no un recorte. Con más líneas de las pedidas se
// devuelven todas: recortar contenido para forzar una altura sería tirar imagen, y
// la imagen es lo que el usuario está mirando.
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
			// Y el relleno son líneas VACÍAS, no espacios: una línea de espacios
			// mide columnas y ensucia el ancho; una vacía no.
			for i := tengo; i < len(got); i++ {
				if got[i] != "" {
					t.Errorf("la línea de relleno %d es %q, want una línea vacía", i, got[i])
				}
			}
			// Y las que ya estaban siguen siendo las mismas, en su sitio.
			for i := range tengo {
				if got[i] != "x" {
					t.Errorf("la línea %d pasó a ser %q, want el contenido original", i, got[i])
				}
			}
		}
	}
	// Un objetivo negativo o cero: no hay a qué rellenar, y no se inventa nada.
	for _, quiero := range []int{-5, -1, 0} {
		if got := padLines([]string{"a", "b"}, quiero); len(got) != 2 {
			t.Errorf("padLines con objetivo %d devolvió %d líneas, want las 2 que había", quiero, len(got))
		}
		if got := padLines(nil, quiero); len(got) != 0 {
			t.Errorf("padLines(nil, %d) devolvió %d líneas, want ninguna", quiero, len(got))
		}
	}
	// Y el slice de entrada no se toca: el relleno copia, no muta. Si mutara, la
	// segunda llamada de un resize arrancaría con líneas vacías en medio.
	orig := []string{"a", "b"}
	padLines(orig, 10)
	if len(orig) != 2 || orig[0] != "a" || orig[1] != "b" {
		t.Errorf("padLines mutó el slice de entrada: %q", orig)
	}
}

// TestLaCajaDeSimMideLoQueElRellenoManda: la caja del popup de sim tiene un alto
// FIJO, que es la razón de ser del relleno. Sin él, una imagen pequeña dejaría el
// pie pegado al borde de arriba y una grande se saldría.
//
// El alto de la caja es el del popup: `simBox` da las filas de la imagen más el
// marco, y el cuerpo tiene que llenar todo menos el marco y el pie.
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

		// Y con MÁS celdas de las que caben la caja CRECE en vez de recortar. Tirar
		// imagen para forzar un alto es peor que salirse: lo que el usuario está
		// mirando es la imagen, y un recorte se lee como un gráfico incompleto.
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

// TestSoloUnaFilaDelSelectorLlevaElCursor: el cursor del selector de estrategia es
// la fila que `enter` elige, así que tiene que haber exactamente una marcada y ser
// la del índice del cursor. Con dos marcadas, `enter` aplica una estrategia y el
// popup enseña otra; con ninguna, el usuario no sabe qué va a pasar.
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

// TestElFlujoDelSelectorEmpiezaEnUnaColumnaFija: la etiqueta de cada estrategia
// se rellena al mismo ancho, y lo que empieza DESPUES —la frase del flujo— cae
// siempre en la misma columna. Eso es lo que hace que dos popups distintos se lean
// de un vistazo: las estrategias forman una columna y los flujos otra.
//
// Y NO es lo mismo que alinear las flechas, que es la suposición fácil: la flecha
// va DENTRO de la frase del flujo, así que su columna depende de lo larga que sea
// la rama. Con la base en "main" la flecha del merge sale antes que la del rebase,
// y está bien: son frases, no una tabla.
//
// Para comprobar el ancho de la etiqueta hace falta un kind que la llene ENTERO
// (8 columnas). Con nombres más cortos, un ancho de etiqueta uno más estrecho
// daría la misma columna y el mutante pasaría.
func TestElFlujoDelSelectorEmpiezaEnUnaColumnaFija(t *testing.T) {
	restore := simKinds
	simKinds = []sim.Kind{sim.KindMerge, sim.KindRebase, sim.Kind("12345678")}
	t.Cleanup(func() { simKinds = restore })

	m := simModel(t, &fakeSimulator{available: true})
	m = press(t, m, "v")

	caja := stripANSI(m.simChooserBox())
	// El inicio del flujo se mide por el nombre de la rama, no por la flecha: la
	// flecha va DENTRO de la frase y su posición depende de lo larga que sea la
	// rama, así que medir por ella compara dos cosas distintas.
	//
	// El merge pone la base primero y el rebase la rama, así que cada fila se
	// localiza por el nombre que le toca. Y el índice de strings.Index es de
	// BYTES: la fila lleva "▸ " delante, que son 3 bytes y 2 columnas, y medir en
	// bytes daría columnas distintas entre la fila con cursor y las que no.
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
	// Y las tres filas dan la MISMA columna, con un kind que llena la etiqueta
	// entera y dos que no. Con la etiqueta un rune más estrecha, el kind de 8
	// columnas empujaría su flujo una columna y esto lo vería.
	if flujos[0] != flujos[1] || flujos[1] != flujos[2] {
		t.Errorf("los flujos empiezan en las columnas %v: la etiqueta no se rellena al mismo ancho", flujos)
	}
}

// TestLaFlechaDelMergeYLaDelRebaseApuntanEnSentidosOpuestos: el selector dice qué
// va a pasar antes de que pase, y las dos operaciones se dibujan con flechas
// OPUESTAS a propósito: un merge trae la rama del ítem a la base, y un rebase
// lleva la base a la rama. Dibujarlas con la misma flecha haría que las dos
// dijeran lo mismo, que es justo lo que el selector tiene que evitar.
//
// Es un detalle pequeño con consecuencias: una flecha invertida hace que el usuario
// lea "mi rama va a la base" justo antes de hacer lo contrario.
func TestLaFlechaDelMergeYLaDelRebaseApuntanEnSentidosOpuestos(t *testing.T) {
	restore := simKinds
	simKinds = []sim.Kind{sim.KindMerge, sim.KindRebase}
	t.Cleanup(func() { simKinds = restore })

	m := simModel(t, &fakeSimulator{available: true})
	m = press(t, m, "v")

	caja := stripANSI(m.simChooserBox())
	// El merge usa ←: la rama del ítem entra en la base.
	merge := lineaConKind(t, caja, sim.KindMerge)
	if !strings.Contains(merge, "←") {
		t.Errorf("la fila de merge debería usar la flecha que trae la rama a la base, dio %q", strings.TrimSpace(merge))
	}
	// El rebase usa →: la base va a la rama.
	rebase := lineaConKind(t, caja, sim.KindRebase)
	if !strings.Contains(rebase, "→") {
		t.Errorf("la fila de rebase debería usar la flecha que lleva la base a la rama, dio %q", strings.TrimSpace(rebase))
	}
	// Y son opuestas: si un día las dos se dibujan igual, el selector deja de
	// distinguir las dos operaciones y la comparación se pierde.
	if strings.Contains(merge, "→") || strings.Contains(rebase, "←") {
		t.Errorf("las dos operaciones comparten sentido: merge=%q rebase=%q", strings.TrimSpace(merge), strings.TrimSpace(rebase))
	}
	// Y las dos nombran las dos ramas, que es lo que convierte la flecha en una
	// frase y no en un adorno.
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
