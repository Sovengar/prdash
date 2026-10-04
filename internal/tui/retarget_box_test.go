package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func ramasDelPopup(t *testing.T, filtro string, ramas ...string) Model {
	t.Helper()
	m := sizedRetarget(t, 90, 60, ramas...)
	if filtro != "" {
		m = pressFilter(t, m, filtro)
	}
	if len(m.retarget.view) == 0 {
		t.Fatalf("el filtro %q dejó la vista vacía de %v", filtro, m.retarget.all)
	}
	return m
}

func lineasDelPopup(m Model) []string {
	return strings.Split(stripANSI(m.retargetOverlay2()), "\n")
}

// The cursor is the row enter picks.
func TestSoloUnaFilaLlevaElCursorYEsLaElegida(t *testing.T) {
	ramas := []string{"main", "release/2.0", "fix/uno", "fix/dos", "wip"}
	for cursor := range len(ramas) {
		m := ramasDelPopup(t, "", ramas...)
		m.retarget.cursor = cursor
		m.retarget.win = m.retargetWindow()

		esperado := ""
		for i, r := range m.retarget.view {
			if r == m.retarget.view[cursor] {
				esperado = r
				_ = i
			}
		}
		conCursor := 0
		for _, l := range lineasDelPopup(m) {
			if !strings.Contains(l, "▸") {
				continue
			}
			conCursor++
			if !strings.Contains(l, esperado) {
				t.Errorf("cursor %d: la fila marcada es %q, want la del cursor %q", cursor, strings.TrimSpace(l), esperado)
			}
		}
		if conCursor != 1 {
			t.Errorf("cursor %d: %d filas marcadas, want exactamente 1", cursor, conCursor)
		}
		// And the cursor never leaves the view: outside it there would be no row to draw it on.
		if cursor < len(m.retarget.view) && conCursor == 0 {
			t.Errorf("cursor %d: ninguna fila marcada con la vista llena", cursor)
		}
	}
}

// Two different signals and the popup has to distinguish them.
func TestElCursorYLaBaseActualNoCompartenSimbolo(t *testing.T) {
	m := ramasDelPopup(t, "", "main", "otra", "tercera")
	m.retarget.cursor = 0
	m.retarget.win = 0
	lineas := lineasDelPopup(m)
	var fila string
	for _, l := range lineas {
		if strings.Contains(l, "▸") {
			fila = l
		}
	}
	if fila == "" {
		t.Fatalf("no hay fila marcada: %q", lineas)
	}
	if !strings.Contains(fila, "▸") {
		t.Errorf("la fila del cursor no lleva el cursor: %q", fila)
	}
	if !strings.Contains(fila, "current") {
		t.Errorf("la base actual no se marca cuando el cursor está encima: %q", fila)
	}
	marked := 0
	for _, l := range lineas {
		if strings.Contains(l, "current") {
			marked++
		}
	}
	if marked != 1 {
		t.Errorf("%d filas marcadas como base actual, want 1: el distintivo dice de dónde se sale, no cuál está elegida", marked)
	}
}

func TestLaListaSeAlineaEnUnaColumna(t *testing.T) {
	// A long base and a one-character one, which is what separates an aligned column from a
	// coincidental one.
	m := ramasDelPopup(t, "", "main", "una-rama-con-nombre-larguísimo-de-verdad", "wip")
	m.retarget.cursor = 1
	m.retarget.win = 0

	// The header fixes the width: every row has to measure the same.
	var anchos []int
	for _, l := range lineasDelPopup(m) {
		anchos = append(anchos, ansi.StringWidth(l))
	}
	for i := 1; i < len(anchos); i++ {
		if anchos[i] != anchos[0] {
			t.Errorf("la línea %d mide %d columnas y la 0 mide %d: la caja se descentra",
				i, anchos[i], anchos[0])
		}
	}

	// The current base's suffix lands in the SAME column as the bare name.
	colDeCurrent := -1
	for _, l := range lineasDelPopup(m) {
		i := strings.Index(l, "current")
		if i < 0 {
			continue
		}
		col := ansi.StringWidth(l[:i])
		if colDeCurrent < 0 {
			colDeCurrent = col
		} else if col != colDeCurrent {
			t.Errorf("el sufijo cae en la columna %d y antes en la %d: la columna no está alineada", col, colDeCurrent)
		}
	}
	if colDeCurrent < 0 {
		t.Error("no se pintó el sufijo de la base actual")
	}
	// The suffix ENDS at the inner edge, not in an arbitrary column.
	fin := colDeCurrent + len("current")
	wantFin := 1 + max(8, m.retargetBoxWidth()-2)
	if fin != wantFin {
		t.Errorf("el sufijo acaba en la columna %d, want %d (el borde del interior): el relleno no empuja el sufijo", fin, wantFin)
	}
}

func TestUnNombreLargoNoDesborraNiPisaElSufijo(t *testing.T) {
	largo := "feature/" + strings.Repeat("nombre", 20)
	m := ramasDelPopup(t, "", "main", largo, "wip")
	m.retarget.cursor = 1
	m.retarget.win = 0
	inner := max(8, m.retargetBoxWidth()-2)

	var anchos []int
	vistos := map[string]bool{}
	for _, l := range lineasDelPopup(m) {
		anchos = append(anchos, ansi.StringWidth(l))
		for _, r := range []string{"main", "wip"} {
			if strings.Contains(l, r) {
				vistos[r] = true
			}
		}
	}
	for i := 1; i < len(anchos); i++ {
		if anchos[i] != anchos[0] {
			t.Errorf("la línea %d mide %d columnas y la 0 mide %d con un nombre enorme: se sale de la caja", i, anchos[i], anchos[0])
		}
	}
	for _, r := range []string{"main", "wip"} {
		if !vistos[r] {
			t.Errorf("con un nombre enorme en la lista, %q desapareció de la caja", r)
		}
	}
	if !strings.Contains(stripANSI(m.retargetOverlay2()), "feature/") {
		t.Error("el nombre recortado perdió el principio, que es lo que lo hace reconocible")
	}
	_ = inner
}

func TestCuantasFilasSePintanNiUnaMasNiUnaMenos(t *testing.T) {
	muchas := []string{"main"}
	for i := range 40 {
		muchas = append(muchas, "rama/"+string(rune('a'+i%26))+string(rune('a'+i/26)))
	}
	for _, h := range []int{10, 14, 20, 40} {
		m := sizedRetarget(t, 90, h, muchas...)
		want := min(m.retargetRows(), len(m.retarget.view))
		if got := cuentaFilasDeRama(m); got != want {
			t.Errorf("altura %d: %d filas de rama pintadas, want %d (las que la caja reservó: %d)",
				h, got, want, m.retargetRows())
		}
	}
	pocas := []string{"main", "otra", "wip"}
	for _, h := range []int{10, 20, 40} {
		m := sizedRetarget(t, 90, h, pocas...)
		if got := cuentaFilasDeRama(m); got != len(pocas) {
			t.Errorf("altura %d con %d ramas: %d pintadas, want %d", h, len(pocas), got, len(pocas))
		}
	}
}

func cuentaFilasDeRama(m Model) int {
	n := 0
	for _, l := range lineasDelPopup(m) {
		if strings.Contains(l, "▸") || strings.Contains(l, retargetCurrentSuffix) {
			n++
			continue
		}
		for _, r := range m.retarget.view {
			if r != "" && strings.Contains(l, r) && !strings.Contains(l, "from ") {
				n++
				break
			}
		}
	}
	return n
}

// The header names the base even when it does not know it.
func TestLaCabeceraNombraLaBaseAunqueNoLaConozca(t *testing.T) {
	m := ramasDelPopup(t, "", "main", "otra")
	if !strings.Contains(stripANSI(m.retargetOverlay2()), "from main") {
		t.Errorf("la cabecera debería decir de qué base se sale: %q", stripANSI(m.retargetOverlay2()))
	}

	m.retarget.item.TargetBranch = ""
	txt := stripANSI(m.retargetOverlay2())
	if !strings.Contains(txt, "from unknown") {
		t.Errorf("sin base conocida la cabecera debería decir unknown: %q", txt)
	}
	if strings.Contains(txt, "from  ") || strings.Contains(txt, "from ·") {
		t.Errorf("sin base conocida la cabecera dejó un hueco: %q", txt)
	}
	// And with no known base no row carries a "current" marker.
	if strings.Contains(txt, "current") {
		t.Errorf("sin base conocida no debería salir el distintivo de la base: %q", txt)
	}
}

// With the filter off the box says how many branches the repo has.
func TestLaCuentaDeRamasDistingueFiltradoDeSinFiltrar(t *testing.T) {
	ramas := []string{"main", "fix/uno", "fix/dos", "wip", "otro"}
	completas := []string{"main", "fix/uno", "fix/dos", "wip", "otro"}

	m := ramasDelPopup(t, "", completas...)
	txt := stripANSI(m.retargetOverlay2())
	if !strings.Contains(txt, "5 branches") {
		t.Errorf("sin filtro la caja debería decir cuántas hay: %q", txt)
	}
	if strings.Contains(txt, "match") {
		t.Errorf("sin filtro no debería haber fórmula de coincidencia: %q", txt)
	}

	m = ramasDelPopup(t, "fix", completas...)
	txt = stripANSI(m.retargetOverlay2())
	if !strings.Contains(txt, "2 of 5 branches match") {
		t.Errorf("con filtro la caja debería decir cuántas casan de cuántas hay: %q", txt)
	}

	// And the singular: "1 branch", because a popup that says "1 branches" looks broken.
	if got := pluralBranches(1); got != "1 branch" {
		t.Errorf("pluralBranches(1) = %q, want %q", got, "1 branch")
	}
	if got := pluralBranches(0); got != "0 branches" {
		t.Errorf("pluralBranches(0) = %q, want %q", got, "0 branches")
	}
	if got := pluralBranches(2); got != "2 branches" {
		t.Errorf("pluralBranches(2) = %q, want %q", got, "2 branches")
	}
	m = ramasDelPopup(t, "wip", completas...)
	if !strings.Contains(stripANSI(m.retargetOverlay2()), "1 of 5 branches match") {
		t.Errorf("con una sola coincidencia: %q", stripANSI(m.retargetOverlay2()))
	}
	_ = ramas
}

// Without a filter the field shows a dim placeholder.
func TestElCampoDeFiltroDistingueElPlaceholderDelTexto(t *testing.T) {
	m := ramasDelPopup(t, "", "main", "otra")
	txt := stripANSI(m.retargetOverlay2())
	if !strings.Contains(txt, "type to search") {
		t.Errorf("sin filtro el campo debería ofrecer el placeholder: %q", txt)
	}

	m = ramasDelPopup(t, "ot", "main", "otra")
	txt = stripANSI(m.retargetOverlay2())
	if strings.Contains(txt, "type to search") {
		t.Errorf("con filtro puesto no debería quedar el placeholder: %q", txt)
	}
	if !strings.Contains(txt, "ot") {
		t.Errorf("con filtro el campo debería enseñar lo escrito: %q", txt)
	}

	// A filter of only spaces trims to empty, so the view is the WHOLE list.
	m.retarget.query = "   "
	m.applyQuery()
	if len(m.retarget.view) != len(m.retarget.all) {
		t.Errorf("un filtro de solo espacios dio %d de %d ramas: el filtro se recorta y no deja la vista vacía",
			len(m.retarget.view), len(m.retarget.all))
	}
	if !strings.Contains(stripANSI(m.retargetOverlay2()), "2 of 2 branches match") {
		t.Errorf("un filtro de solo espacios no está filtrando nada: %q", stripANSI(m.retargetOverlay2()))
	}
	// And a one-character query, which is where a `query == ""` written by mistake would break.
	for _, q := range []string{"x", "f", "1"} {
		m.retarget.query = q
		txt := stripANSI(m.retargetOverlay2())
		if strings.Contains(txt, "type to search") || !strings.Contains(txt, q) {
			t.Errorf("con el filtro %q la caja no enseña lo escrito: %q", q, txt)
		}
	}
}

// A filter that matches nothing leaves the list empty and the box says so.
func TestElPopupDiceCuandoNoEncuentraNada(t *testing.T) {
	m := ramasDelPopup(t, "", "main", "otra")
	m.retarget.query = "noexiste"
	m.applyQuery()
	txt := stripANSI(m.retargetOverlay2())
	if !strings.Contains(txt, "no branch matches the filter") {
		t.Errorf("un filtro sin coincidencias debería explicarlo: %q", txt)
	}
	// The forge's error has its own place: it is another cause with another repair.
	m.retarget.query = ""
	m.retarget.errMsg = "gh: Not Found (HTTP 404)"
	txt = stripANSI(m.retargetOverlay2())
	if !strings.Contains(txt, "404") {
		t.Errorf("el error del forge debería verse: %q", txt)
	}
	if strings.Contains(txt, "no branch matches") {
		t.Errorf("con un error del forge no debería salir el mensaje de filtro: %q", txt)
	}
	// The error WINS over the list: a repo that errored has no list to show.
	m.retarget.view = []string{"lo que hubiera quedado de antes"}
	if txt = stripANSI(m.retargetOverlay2()); !strings.Contains(txt, "404") {
		t.Errorf("con error y lista, la caja debería enseñar el error: %q", txt)
	}
}
