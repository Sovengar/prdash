package tui

import "testing"

func altoDe(lay layout) int {
	n := lay.bodyLines + listChrome + detailChrome + lay.detailLines
	if lay.showHeader {
		n += headerLines
	}
	if lay.showKeybinds {
		n += keybindsChrome + lay.hintLines
	}
	return n
}

// The invariant the whole view rests on: each box gives up its height in turn.
func TestComputeLayoutLlenaLaAltura(t *testing.T) {
	for _, height := range []int{10, 14, 20, 24, 30, 40, 60, 120} {
		lay := computeLayout(height, 1, true)
		if got := altoDe(lay); got != height {
			t.Errorf("computeLayout(%d) suma %d líneas, want %d (%+v)", height, got, height, lay)
		}
		if lay.bodyLines < 1 {
			t.Errorf("computeLayout(%d) deja el cuerpo central en %d: sin él no hay nada que mirar", height, lay.bodyLines)
		}
	}
}

func TestComputeLayoutDetalleCuarentaPorCiento(t *testing.T) {
	for _, tc := range []struct{ height, detail int }{
		{40, 16}, // 16/40 = 40%
		{30, 12}, // 12/30 = 40%
		{24, 9},  // 9/24 = 37%, redondeo a la baja por entero
	} {
		lay := computeLayout(tc.height, 1, true)
		if lay.detailLines != tc.detail {
			t.Errorf("computeLayout(%d) detalle = %d, want %d", tc.height, lay.detailLines, tc.detail)
		}
		if !lay.showHeader || !lay.showKeybinds {
			t.Errorf("computeLayout(%d) = %+v; con %d líneas caben las cuatro cajas", tc.height, lay, tc.height)
		}
	}
}

func TestComputeLayoutDegradaSinPerderElCuerpo(t *testing.T) {
	lay := computeLayout(14, 1, true)
	if lay.showHeader {
		t.Errorf("computeLayout(14) = %+v; la caja de cabecera debería caer", lay)
	}
	if lay.bodyLines < minListRows {
		t.Errorf("cuerpo central = %d, want >= %d", lay.bodyLines, minListRows)
	}
	if lay.detailLines < minDetailRows {
		t.Errorf("detalle = %d, want >= %d", lay.detailLines, minDetailRows)
	}

	lay = computeLayout(10, 1, true)
	if lay.showHeader || lay.showKeybinds {
		t.Errorf("computeLayout(10) = %+v; en 10 líneas solo caben cuerpo y panel", lay)
	}
	if lay.bodyLines < 1 || lay.detailLines < 1 {
		t.Errorf("computeLayout(10) = %+v; cuerpo y panel no pueden quedarse a cero", lay)
	}
}

func TestComputeLayoutRecortaHintsAntesDeOcultarlos(t *testing.T) {
	lay := computeLayout(40, 3, true)
	if lay.showKeybinds && lay.hintLines != maxHintLines {
		t.Errorf("con sitio de sobra hints = %d, want %d (%+v)", lay.hintLines, maxHintLines, lay)
	}
	lay = computeLayout(40, 1, true)
	if lay.hintLines != 1 {
		t.Errorf("hints = %d, want 1 (%+v)", lay.hintLines, lay)
	}
	lay = computeLayout(9, 1, true)
	if lay.showKeybinds {
		t.Errorf("computeLayout(9) = %+v; la caja de atajos debería caer", lay)
	}
	if lay.hintLines != 0 {
		t.Errorf("hints = %d tras ocultar la caja, want 0", lay.hintLines)
	}
}

func TestComputeLayoutSinAlturaNoRecorta(t *testing.T) {
	for _, show := range []bool{false, true} {
		lay := computeLayout(0, 1, show)
		if lay.bodyLines != 0 || lay.detailLines != 0 {
			t.Errorf("computeLayout(0, show=%v) = %+v; want ceros", show, lay)
		}
		if lay.showHeader || lay.showKeybinds {
			t.Errorf("computeLayout(0, show=%v) = %+v; sin altura no se decide nada", show, lay)
		}
	}
}
