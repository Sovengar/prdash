package tui

import "testing"

// With just enough room the detail loses its rows first.
func TestLaCascadaDejaElDetalleAntesQueElCuerpo(t *testing.T) {
	casos := []struct {
		height    int
		wantBody  int
		wantDetal int
		nota      string
	}{
		{6, 1, 1, "el detalle se queda en su mínimo, no en cero"},
		{14, 4, 6, "todo oculto: solo lista y detalle, y el detalle en su mínimo"},
		{16, 3, 6, "vuelve la caja de atajos con una línea"},
		{18, 3, 6, "los atajos suben a su tope de tres"},
		{21, 3, 6, "vuelve la cabecera, y el cuerpo se queda en tres"},
		{22, 3, 7, "el detalle crece: es su 40% y hay sitio de sobra"},
		{23, 3, 8, "y sigue creciendo mientras el cuerpo aguanta tres"},
		{25, 3, 10, "el mínimo del cuerpo son tres filas: hasta ahí llega el detalle"},
		{28, 5, 11, "pasado ese punto el cuerpo se lleva el sobrante"},
	}

	for _, c := range casos {
		got := computeLayout(c.height, maxHintLines, true)
		if got.bodyLines != c.wantBody || got.detailLines != c.wantDetal {
			t.Errorf("altura %d dio cuerpo=%d detalle=%d, want cuerpo=%d detalle=%d. %s",
				c.height, got.bodyLines, got.detailLines, c.wantBody, c.wantDetal, c.nota)
		}
	}
}

func TestElDetalleRespetaSuMinimoMientrasHayaSitio(t *testing.T) {
	for h := 1; h <= 80; h++ {
		for _, ha := range []int{0, 1, maxHintLines, maxHintLines + 3} {
			got := computeLayout(h, ha, true)

			if got.detailLines < 0 {
				t.Errorf("altura %d con %d atajos dio detalle=%d: degradar es quitar, "+
					"no quedarse en negativo", h, ha, got.detailLines)
			}

			// The minimum is honoured AS SOON AS THE TERMINAL ALLOWS IT; the threshold is what decides.
			imposible := minDetailRows + detailChrome + listChrome + 1
			if h >= imposible && got.detailLines < minDetailRows {
				t.Errorf("altura %d con %d atajos dio detalle=%d, por debajo del mínimo %d, "+
					"y la terminal sí da para él (hacen falta %d). Con `>=` en vez de `>` "+
					"la frontera del bucle recorta una fila de más y la ficha se queda sin "+
					"la última",
					h, ha, got.detailLines, minDetailRows, imposible)
			}
			// Below the threshold the detail may drop, and that is correct. My first version said otherwise.
		}
	}
}

// The central body NEVER disappears: the list is what has to be scrollable.
func TestElCuerpoCentralNuncaDesaparece(t *testing.T) {
	for h := 1; h <= 60; h++ {
		for _, ha := range []int{0, 2, maxHintLines} {
			got := computeLayout(h, ha, true)
			if got.bodyLines < 1 {
				t.Errorf("altura %d con %d atajos dio cuerpo=%d: la lista es lo que hay "+
					"que poder desplazar, no puede quedarse sin filas", h, ha, got.bodyLines)
			}
			if got.detailLines < 0 || got.hintLines < 0 {
				t.Errorf("altura %d dio detalle=%d y atajos=%d, negativos: degradar es "+
					"quitar, no quedarse en negativo", h, got.detailLines, got.hintLines)
			}
		}
	}
}

func TestLaCascadaCedeEnElOrdenDeclarado(t *testing.T) {
	lay := computeLayout(14, maxHintLines, true)
	if !lay.showHeader && lay.detailLines > minDetailRows {
		t.Errorf("la cabecera se ocultó con el detalle todavía en %d filas de su mínimo "+
			"%d: el orden dice que el detalle cede primero", lay.detailLines, minDetailRows)
	}

	layChico := computeLayout(10, maxHintLines, true)
	if !layChico.showKeybinds && layChico.hintLines > 1 {
		t.Errorf("la caja de atajos desapareció con %d líneas dentro: los atajos bajan "+
			"de línea una a una antes de desaparecer enteros", layChico.hintLines)
	}
}

// Leaving the cascade, nothing is clipped that does not need to be.
func TestLaCascadaParaEnCuantoCabe(t *testing.T) {
	for h := 1; h <= 80; h++ {
		lay := computeLayout(h, maxHintLines, true)
		reservado := listChrome + detailChrome + lay.detailLines
		if lay.showHeader {
			reservado += headerLines
		}
		if lay.showKeybinds {
			reservado += keybindsChrome + lay.hintLines
		}
		// What is left over: if there were leftovers, the cascade could keep clipping.
		sobra := reservado + minListRows - h
		if sobra > 0 && (lay.detailLines > minDetailRows || lay.showHeader ||
			lay.hintLines > 1 || lay.showKeybinds) {
			t.Errorf("altura %d: sobran %d filas y aun así queda algo que ceder "+
				"(detalle=%d cabecera=%v atajos=%v con %d líneas): la cascada paró antes de tiempo",
				h, sobra, lay.detailLines, lay.showHeader, lay.showKeybinds, lay.hintLines)
		}
		// And the central body keeps the leftover, which is the reason it never disappears.
		if lay.bodyLines != max(1, h-reservado) {
			t.Errorf("altura %d: el cuerpo quedó en %d, y con %d reservadas debería ser %d",
				h, lay.bodyLines, reservado, max(1, h-reservado))
		}
	}
}

func TestAntesDelWindowSizeNoSeRecortaNada(t *testing.T) {
	vacio := computeLayout(0, maxHintLines, false)
	if vacio.bodyLines != 0 || vacio.detailLines != 0 || vacio.hintLines != 0 {
		t.Errorf("sin tamaño dio %+v, want un layout vacío: antes del primer WindowSizeMsg "+
			"no hay altura que repartir", vacio)
	}
	cero := computeLayout(0, maxHintLines, true)
	if cero == vacio {
		t.Log("con altura cero y show se devuelve el mismo layout vacío; se afirma solo " +
			"que no hay crash")
	}
	// And a negative terminal, which Terminal does not produce but a weird resize could.
	for _, h := range []int{-1, -40} {
		if got := computeLayout(h, maxHintLines, true); got != (layout{}) {
			t.Errorf("con altura %d dio %+v, want el layout vacío", h, got)
		}
	}
}
