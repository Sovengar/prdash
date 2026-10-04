package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

func sizedRetarget(t *testing.T, w, h int, branches ...string) Model {
	t.Helper()
	m, _ := retargetFixture(t, branches...)
	return send(t, m, tea.WindowSizeMsg{Width: w, Height: h})
}

// The popup draws ON TOP, in both dimensions.
func TestElPopupDeRetargetRespetaLaTerminalEnLasDosDimensiones(t *testing.T) {
	for _, w := range []int{20, 40, 64, 80, 200} {
		for _, h := range []int{5, 8, 12, 20, 60} {
			m := sizedRetarget(t, w, h, "main", "release/2.0", "fix/uno")
			box := m.retargetOverlay2()
			if box == "" {
				continue
			}
			lineas := strings.Split(stripANSI(box), "\n")
			ancho := m.retargetBoxWidth()
			for i, l := range lineas {
				if got := ansi.StringWidth(l); got != ancho {
					t.Errorf("terminal %dx%d: la línea %d mide %d columnas y la caja %d: %q",
						w, h, i, got, ancho, l)
				}
			}
			// The width never exceeds the popup's design width however wide the terminal.
			if ancho > retargetChooserWidth {
				t.Errorf("terminal %dx%d: caja de %d columnas, want <= %d", w, h, ancho, retargetChooserWidth)
			}
			if want := min(m.contentWidth(), retargetChooserWidth); ancho != want {
				t.Errorf("terminal %dx%d: caja de %d columnas, want %d (contenido acotado)", w, h, ancho, want)
			}
		}
	}
}

// The number of branch rows is what decides the height.
func TestLasFilasDelPopupCabenEntreElMargenYElTecho(t *testing.T) {
	branches := []string{"main", "a/1", "a/2", "a/3", "a/4", "a/5", "a/6", "a/7", "a/8", "a/9", "a/10", "a/11", "a/12", "a/13"}
	for _, h := range []int{4, 6, 8, 10, 14, 20, 40, 100} {
		m := sizedRetarget(t, 80, h, branches...)
		got := m.retargetRows()

		if got < retargetMinRows {
			t.Errorf("altura %d: %d filas, want >= %d (sin filas el popup parece vacío)", h, got, retargetMinRows)
		}
		if got > retargetRows {
			t.Errorf("altura %d: %d filas, want <= %d", h, got, retargetRows)
		}
		want := h - 2*retargetMargin - retargetChrome
		if want < retargetMinRows {
			want = retargetMinRows
		}
		if want > retargetRows {
			want = retargetRows
		}
		if got != want {
			t.Errorf("altura %d: %d filas, want %d (h - 2*margen - marco, acotado a [%d, %d])",
				h, got, want, retargetMinRows, retargetRows)
		}
	}
}

// Filter and arrows share the same window calculation.
func TestMoverElCursorRecalculaLaVentana(t *testing.T) {
	m := sizedRetarget(t, 80, 60, "a", "b", "c", "d", "e")
	if win := m.retargetWindow(); win != 0 {
		t.Errorf("con la lista entera win = %d, want 0", win)
	}

	filas := m.retargetRows()
	m.retarget.view = seqBranches(filas)
	m.retarget.cursor, m.retarget.win = 0, 0
	if win := m.retargetWindow(); win != 0 {
		t.Errorf("lista de %d filas con ventana de %d y cursor arriba: win = %d, want 0: la primera rama se vería escondida",
			filas, filas, win)
	}

	m.retarget.view = seqBranches(filas + 4)
	for cursor := 0; cursor < filas+4; cursor++ {
		m.retarget.cursor, m.retarget.win = cursor, 0
		// This is what moveRetargetCursor does: the arithmetic lives in retargetWindow and the render
		//reads m.retarget.win.
		m.retarget.win = m.retargetWindow()
		visibles, start := m.retargetVisible()
		if cursor < start || cursor >= start+len(visibles) {
			t.Errorf("cursor %d: la ventana [%d, %d) no lo contiene (visibles %d)", cursor, start, start+len(visibles), len(visibles))
		}
		if start+len(visibles) > len(m.retarget.view) {
			t.Errorf("cursor %d: la ventana [%d, %d) se pasa de las %d ramas", cursor, start, start+len(visibles), len(m.retarget.view))
		}
	}

	m.retarget.cursor, m.retarget.win = filas+3, 0
	m.moveRetargetCursor(-(filas + 3))
	if m.retarget.cursor != 0 {
		t.Errorf("cursor = %d tras subir del todo, want 0", m.retarget.cursor)
	}
	if m.retarget.win != 0 {
		t.Errorf("win = %d tras subir del todo, want 0", m.retarget.win)
	}
	m.retarget.view = []string{"a", "b"}
	m.retarget.cursor, m.retarget.win = 1, 0
	m.moveRetargetCursor(5)
	if m.retarget.win != 0 {
		t.Errorf("lista de 2 filas con ventana de %d: win = %d, want 0", filas, m.retarget.win)
	}
}

func seqBranches(n int) []string {
	out := make([]string, 0, n)
	for i := range n {
		out = append(out, "rama/"+string(rune('a'+i%26))+string(rune('a'+i/26)))
	}
	return out
}

func TestLaFlechaDeLaConfirmacionCaeEnUnaColumnaFija(t *testing.T) {
	columna := func(base string) int {
		t.Helper()
		m, _ := retargetFixture(t, base, "destino")
		m.retarget.item.TargetBranch = base
		m.retarget.cursor = 1
		m.retarget.chosen = "destino"
		m.retarget.state = retargetConfirm

		box := stripANSI(m.retargetConfirmBox())
		wantCol := 2 + (m.retargetBoxWidth()-6)/2
		visto := -1
		for _, l := range strings.Split(box, "\n") {
			i := strings.Index(l, "→")
			if i < 0 {
				continue
			}
			col := ansi.StringWidth(l[:i])
			if ansi.StringWidth(base) <= (m.retargetBoxWidth()-6)/2 && col != wantCol {
				t.Errorf("base %q: la flecha cae en la columna %d, want %d (media caja, el relleno de la base): %q",
					base, col, wantCol, l)
			}
			if !strings.Contains(l, "→ "+m.retarget.chosen) {
				t.Errorf("base %q: la flecha no está pegada al destino: %q", base, l)
			}
			visto = col
		}
		if visto < 0 {
			t.Fatalf("base %q: la caja no tiene flecha: %q", base, box)
		}
		return visto
	}

	want := columna("main")
	for _, base := range []string{"main", "release/2.0", "feature/x", "x", "fix/hunk"} {
		if got := columna(base); got != want {
			t.Errorf("la columna de la flecha depende de la base: %q dio %d y %q dio %d", "main", want, base, got)
		}
	}
	larga := "feature/" + strings.Repeat("x", 40)
	if columna(larga) <= want {
		t.Errorf("una base de %d columnas debería empujar la flecha más allá de %d", len(larga), want)
	}
}

func TestLaCajaDeConfirmacionNombraLasDosRamas(t *testing.T) {
	m, _ := retargetFixture(t, "main", "release/2.0")
	m.retarget.cursor = 1
	m.retarget.chosen = "release/2.0"
	m.retarget.state = retargetConfirm

	box := stripANSI(m.retargetConfirmBox())
	for _, want := range []string{"main", "release/2.0", "enter apply", "esc back"} {
		if !strings.Contains(box, want) {
			t.Errorf("la caja debería contener %q: %q", want, box)
		}
	}
	// Without it the box only lists the keys and the user cannot tell what stopped being true.
	if !strings.Contains(box, "recomputed") {
		t.Errorf("la caja debería decir que se recalcula: %q", box)
	}

	sinBase := m
	sinBase.retarget.item.TargetBranch = ""
	box = stripANSI(sinBase.retargetConfirmBox())
	if !strings.Contains(box, "unknown") {
		t.Errorf("sin base conocida la caja debería decir unknown: %q", box)
	}
	if strings.Contains(box, " →  ") || strings.Contains(box, "(→") {
		t.Errorf("sin base conocida la caja dejó un hueco en la flecha: %q", box)
	}
}

// The in-progress warning is the only thing that distinguishes one retarget from another.
func TestRetargetProgressNoticeDistingueLasDosRamas(t *testing.T) {
	conBase := stripANSI(retargetProgressNotice("main", "release/2.0"))
	if !strings.Contains(conBase, "main") || !strings.Contains(conBase, "release/2.0") {
		t.Errorf("el aviso debería nombrar las dos ramas: %q", conBase)
	}
	sinBase := stripANSI(retargetProgressNotice("", "release/2.0"))
	if !strings.Contains(sinBase, "release/2.0") {
		t.Errorf("sin base de origen el aviso debería nombrar el destino: %q", sinBase)
	}
	if strings.Contains(sinBase, "( →") || strings.Contains(sinBase, "(→  ") {
		t.Errorf("sin base de origen el aviso dejó un hueco: %q", sinBase)
	}
	if conBase == sinBase {
		t.Errorf("con y sin base de origen el aviso es el mismo: %q", conBase)
	}
}
