package tui

import (
	"strings"
	"testing"
)

// The parts have to add up to the terminal height, because the view fills it exactly and any
// mismatch pushes the hints or the header off screen.
func TestComputeLayoutLaSumaDaLaAltura(t *testing.T) {
	for _, show := range []bool{true, false} {
		for _, hintAvailable := range []int{-1, 0, 1, 2, 3, 4, 10} {
			for height := -3; height <= 80; height++ {
				lay := computeLayout(height, hintAvailable, show)

				if !show {
					if lay != (layout{}) {
						t.Errorf("show=false dio %+v, want la vista vacía: antes del primer "+
							"WindowSizeMsg no se recorta nada", lay)
					}
					continue
				}
				if height <= 0 {
					if lay != (layout{}) {
						t.Errorf("con altura %d dio %+v, want la vista vacía", height, lay)
					}
					continue
				}

				if lay.bodyLines < 1 {
					t.Errorf("altura %d, atajos %d: el cuerpo central quedó en %d: "+
						"no puede desaparecer", height, hintAvailable, lay.bodyLines)
				}

				// THE FORMULA: everything reserved plus the body gives the exact height, except when the height
				//is below what is reserved.
				reservado := listChrome + detailChrome + lay.detailLines
				if lay.showHeader {
					reservado += headerLines
				}
				if lay.showKeybinds {
					reservado += keybindsChrome + lay.hintLines
				}
				quiere := max(1, height-reservado)
				if lay.bodyLines != quiere {
					t.Errorf("altura %d, atajos %d: cuerpo %d con %d reservados, want %d. "+
						"El descuadre empuja una caja fuera de pantalla, y el síntoma sale en otra caja",
						height, hintAvailable, lay.bodyLines, reservado, quiere)
				}
				// No height is NEGATIVE. A detail at -1 lines is not a small detail: the renderer sees an
				//invalid box.
				if lay.detailLines < 0 {
					t.Errorf("altura %d, atajos %d: el detalle quedó en %d líneas: un alto negativo no es un alto",
						height, hintAvailable, lay.detailLines)
				}
				if lay.hintLines < 0 {
					t.Errorf("altura %d, atajos %d: los atajos quedaron en %d líneas", height, hintAvailable, lay.hintLines)
				}
				if lay.showKeybinds && lay.hintLines < 1 {
					t.Errorf("altura %d, atajos %d: la caja de atajos se ve con %d líneas",
						height, hintAvailable, lay.hintLines)
				}
				if lay.bodyLines > minListRows && lay.detailLines < minDetailRows {
					t.Errorf("altura %d, atajos %d: el detalle bajó a %d con el cuerpo central en %d, "+
						"que todavía puede con su mínimo de %d",
						height, hintAvailable, lay.detailLines, lay.bodyLines, minDetailRows)
				}

				// When the height works out, the sum is exact.
				if height > reservado {
					if got := lay.bodyLines + reservado; got != height {
						t.Errorf("altura %d, atajos %d: cuerpo %d + reservado %d = %d, want %d",
							height, hintAvailable, lay.bodyLines, reservado, got, height)
					}
				}
			}
		}
	}
}

// The cascade degrades IN ORDER and each step has a floor respected until there is nothing left.
func TestComputeLayoutRespetaLosSuelosAntesDeQuitarlos(t *testing.T) {
	for _, height := range []int{5, 6, 7, 8, 9, 10, 12, 15, 18, 20, 24, 30, 40, 60, 80} {
		for _, hintAvailable := range []int{0, 1, 2, 3, 5} {
			lay := computeLayout(height, hintAvailable, true)
			if lay.showKeybinds && lay.hintLines < 1 {
				t.Errorf("altura %d, atajos %d: la caja de atajos se ve con %d líneas: "+
					"una caja visible sin contenido es un hueco", height, hintAvailable, lay.hintLines)
			}
			if lay.hintLines > maxHintLines {
				t.Errorf("altura %d, atajos %d: los atajos piden %d líneas, más que el tope %d: "+
					"la caja nunca pide más de las que se van a pintar",
					height, hintAvailable, lay.hintLines, maxHintLines)
			}
			if lay.showKeybinds && hintAvailable > 0 && lay.hintLines > hintAvailable {
				t.Errorf("altura %d, atajos %d: la caja pide %d líneas y solo hay %d",
					height, hintAvailable, lay.hintLines, hintAvailable)
			}
		}
	}
}

// One fewer terminal line yields exactly one more thing given up, in the cascade's order.
func TestComputeLayoutCedeMasAlBajarLaAltura(t *testing.T) {
	for _, hintAvailable := range []int{0, 1, 2, 3, 5} {
		anterior := computeLayout(5, hintAvailable, true)
		for height := 6; height <= 80; height++ {
			actual := computeLayout(height, hintAvailable, true)

			if actual.detailLines < anterior.detailLines {
				t.Errorf("altura %d: el detalle bajó a %d desde %d al subir de %d: "+
					"crecer no puede quitar", height, actual.detailLines, anterior.detailLines, height-1)
			}
			if anterior.showHeader && !actual.showHeader {
				t.Errorf("altura %d: la cabecera desapareció al subir de %d", height, height-1)
			}
			if anterior.showKeybinds && !actual.showKeybinds {
				t.Errorf("altura %d: los atajos desaparecieron al subir de %d", height, height-1)
			}
			if actual.hintLines < anterior.hintLines && !actual.showKeybinds {
			} else if actual.hintLines < anterior.hintLines {
				t.Errorf("altura %d: los atajos bajaron de %d a %d al subir: crecer no puede quitar",
					height, anterior.hintLines, actual.hintLines)
			}
			if actual.detailLines-anterior.detailLines > 1 {
				t.Errorf("altura %d: el detalle subió de golpe de %d a %d con una línea más de terminal",
					height, anterior.detailLines, actual.detailLines)
			}
			anterior = actual
		}
	}
}

// The comments' budget is what is left after the header, and what is not comments is discounted.
func TestCommentBudgetDescuentaLoQueNoSonComentarios(t *testing.T) {
	for rows := -5; rows <= 60; rows++ {
		for grid := 0; grid <= 20; grid++ {
			for url := 0; url <= 4; url++ {
				for avisos := 0; avisos <= 5; avisos++ {
					got := commentBudget(rows, grid, url, avisos)
					want := rows - grid - url - 2 - avisos
					if got != want {
						t.Fatalf("commentBudget(%d,%d,%d,%d) = %d, want %d",
							rows, grid, url, avisos, got, want)
					}
				}
			}
		}
	}

	if got := commentBudget(20, 6, 1, 0); got != 11 {
		t.Errorf("commentBudget(20, 6, 1, 0) = %d, want 11 (20 - 6 - 1 - 2 - 0)", got)
	}
	if commentBudget(20, 6, 1, 0) <= commentBudget(19, 6, 1, 0) {
		t.Error("más filas no dan más presupuesto: el presupuesto tiene que crecer con las filas")
	}
	if got := commentBudget(3, 20, 4, 5); got >= 0 {
		t.Errorf("con la cabecera más grande que el panel dio %d de presupuesto, want negativo", got)
	}
	soloCabecera := commentBudget(18, 0, 0, 0)
	if soloCabecera != 16 {
		t.Errorf("con 18 filas, 0 campos, 0 url y 0 avisos dio %d, want 16: el título y el hueco son 2", soloCabecera)
	}
	base := commentBudget(20, 5, 1, 2)
	for _, c := range []struct {
		nombre string
		got    int
		want   int
	}{
		{"más campos", commentBudget(20, 6, 1, 2), base - 1},
		{"más avisos", commentBudget(20, 5, 1, 3), base - 1},
		{"más url", commentBudget(20, 5, 2, 2), base - 1},
		{"menos filas", commentBudget(19, 5, 1, 2), base - 1},
	} {
		if c.got != c.want {
			t.Errorf("%s: dio %d, want %d (una parte más resta una fila)", c.nombre, c.got, c.want)
		}
	}
}

// Clipped from the TOP, so the END of the detail —state, review and role — stays visible.
func TestClipTopRecortaPorArribaYNoPorAbajo(t *testing.T) {
	lines := []string{"a", "b", "c", "d", "e"}

	for _, rows := range []int{5, 6, 100} {
		if got := clipTop(lines, rows); len(got) != len(lines) || &got[0] != &lines[0] {
			t.Errorf("clipTop con %d filas dio %q, want las cinco intactas", rows, got)
		}
	}
	if got := clipTop(lines, 3); !equalStrings(got, []string{"c", "d", "e"}) {
		t.Errorf("clipTop con 3 filas dio %q, want [c d e]: se recorta por arriba", got)
	}
	if got := clipTop(lines, 1); !equalStrings(got, []string{"e"}) {
		t.Errorf("clipTop con 1 fila dio %q, want [e]", got)
	}
	if got := clipTop(lines, 5); len(got) != 5 {
		t.Errorf("clipTop con exactamente 5 filas dio %d, want 5", len(got))
	}
	for _, rows := range []int{0, -1, -100} {
		if got := clipTop(lines, rows); len(got) != len(lines) {
			t.Errorf("clipTop con %d filas dio %d líneas, want las %d intactas",
				rows, len(got), len(lines))
		}
	}
	for _, rows := range []int{-5, 0, 1, 5} {
		if got := clipTop(nil, rows); len(got) != 0 {
			t.Errorf("clipTop(nil, %d) dio %d líneas, want ninguna", rows, len(got))
		}
	}
}

func equalStrings(a, b []string) bool {
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

func TestDetailGridReparteEnDosColumnas(t *testing.T) {
	para := func(campos ...string) []detailField {
		fs := make([]detailField, len(campos))
		for i, c := range campos {
			fs[i] = detailField{key: c, value: "v"}
		}
		return fs
	}

	for _, c := range []struct {
		nombre string
		campos []detailField
		filas  int
	}{
		{"ninguno", nil, 0},
		{"uno", para("a"), 1},
		{"dos", para("a", "b"), 1},
		{"tres", para("a", "b", "c"), 2},
		{"cuatro", para("a", "b", "c", "d"), 2},
		{"cinco", para("a", "b", "c", "d", "e"), 3},
		{"seis", para("a", "b", "c", "d", "e", "f"), 3},
	} {
		got := detailGrid(c.campos, 80)
		if len(got) != c.filas {
			t.Errorf("%s: %d campos dieron %d filas, want %d", c.nombre, len(c.campos), len(got), c.filas)
		}
		for i, l := range got {
			if strings.Contains(l, "\n") {
				t.Errorf("%s: la fila %d tiene un salto dentro: %q", c.nombre, i, l)
			}
		}
	}
}
