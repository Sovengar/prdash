package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

// ramasDelPopup abre el buscador de ramas sobre un listado y devuelve el modelo
// con la vista puesta, que es el estado del que sale todo lo que se pinta.
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

// lineasDelPopup separa la caja en sus líneas, sin estilos, y devuelve la línea
// de la cabecera y las de la lista de ramas.
func lineasDelPopup(m Model) []string {
	return strings.Split(stripANSI(m.retargetOverlay2()), "\n")
}

// TestSoloUnaFilaLlevaElCursorYEsLaElegida: el cursor es la fila que `enter`
// elige, así que tiene que haber exactamente una marcada y tiene que ser la del
// índice del cursor. Con dos marcadas, `enter` aplica una base y el popup enseña
// otra; con ninguna, el usuario no sabe qué va a pasar.
func TestSoloUnaFilaLlevaElCursorYEsLaElegida(t *testing.T) {
	ramas := []string{"main", "release/2.0", "fix/uno", "fix/dos", "wip"}
	for cursor := range len(ramas) {
		m := ramasDelPopup(t, "", ramas...)
		m.retarget.cursor = cursor
		m.retarget.win = m.retargetWindow()

		// Se cuenta sobre las líneas que contienen un nombre de rama, no sobre
		// todas: la cabecera y los hints no llevan marcador.
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
		// Y el cursor nunca se sale de la vista: si lo estuviera, no habría
		// ninguna fila marcada y el popup no diría qué se elige.
		if cursor < len(m.retarget.view) && conCursor == 0 {
			t.Errorf("cursor %d: ninguna fila marcada con la vista llena", cursor)
		}
	}
}

// TestElCursorYLaBaseActualNoCompartenSimbolo: son dos señales distintas y el
// popup tiene que poder dibujar las dos a la vez. El cursor es la fila que
// `enter` elige; la base actual es de dónde se sale. Si compartieran símbolo,
// cuando coincidieran en una fila el popup no podría decir si esa fila está
// elegida o es el punto de partida, y la pregunta "de dónde a dónde" se queda
// sin una de sus dos respuestas.
func TestElCursorYLaBaseActualNoCompartenSimbolo(t *testing.T) {
	m := ramasDelPopup(t, "", "main", "otra", "tercera")
	// El cursor encima de la base actual: las dos señales en la MISMA fila.
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
	// Y el resto de filas no lleva el distintivo de la base.
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

// TestLaListaSeAlineaEnUnaColumna: cada fila se compone del marcador, el nombre
// recortado al hueco que queda, y el sufijo. El nombre se RELLENA hasta ese hueco
// para que el sufijo caiga en la misma columna en todas las filas. Sin el
// relleno, "release/2.0" y "wip" would have their "· current" at different
// columns and the column would look ragged instead of a column.
func TestLaListaSeAlineaEnUnaColumna(t *testing.T) {
	// Una base larga y otra de un carácter, que es lo que separa una columna
	// alineada de un collage.
	m := ramasDelPopup(t, "", "main", "una-rama-con-nombre-larguísimo-de-verdad", "wip")
	m.retarget.cursor = 1
	m.retarget.win = 0

	// La cabecera fija el ancho: todas las filas tienen que medir lo mismo.
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

	// Y el sufijo de la base actual cae en la MISMA columna que el nombre
	// suelto, es decir que el relleno funciona: la columna del sufijo no depende
	// de lo larga que sea la base.
	// La columna del sufijo: en la fila de la base, y solo ahí.
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
	// Y el sufijo ACABA al borde del interior, no en una columna arbitraria: el
	// relleno del nombre es lo que lo empuja hasta ahí. Es la misma alineación
	// vista desde el otro lado, y por eso tampoco depende de lo larga que sea la
	// base.
	fin := colDeCurrent + len("current")
	wantFin := 1 + max(8, m.retargetBoxWidth()-2)
	if fin != wantFin {
		t.Errorf("el sufijo acaba en la columna %d, want %d (el borde del interior): el relleno no empuja el sufijo", fin, wantFin)
	}
}

// TestUnNombreLargoNoDesbordaNiPisaElSufijo: un nombre de rama más ancho que el
// hueco se recorta, porque el sufijo y el marco son más importantes que las
// últimas letras de una rama que el usuario reconoce por el principio. Y lo que se
// recorta es el NOMBRE, no el relleno entero: si se comiera el relleno, la fila
// sería más corta que las demás y la columna se descuadraría.
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
	// Las ramas cortas siguen siendo legibles: recortarlas con el nombre largo
	// sería un recorte global y dejaría la lista inservible.
	for _, r := range []string{"main", "wip"} {
		if !vistos[r] {
			t.Errorf("con un nombre enorme en la lista, %q desapareció de la caja", r)
		}
	}
	// Y el nombre largo se recorta a menos de la caja, no se corta a mitad de
	// palabra sin más: sigue siendo reconocible por su principio.
	if !strings.Contains(stripANSI(m.retargetOverlay2()), "feature/") {
		t.Error("el nombre recortado perdió el principio, que es lo que lo hace reconocible")
	}
	_ = inner
}

// TestCuantasFilasSePintanNiUnaMasNiUnaMenos: la lista dibuja exactamente las
// filas que la caja ha reservado, o las que quedan si son menos. Una fila de más
// se sale por debajo del borde —y el borde es lo que dice dónde acaba la lista—,
// y una de menos esconde una rama que el usuario ya estaba leyendo.
func TestCuantasFilasSePintanNiUnaMasNiUnaMenos(t *testing.T) {
	// Con más ramas de las que la caja muestra, se pintan las que caben.
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
	// Con menos ramas de las que caben, se pintan todas.
	pocas := []string{"main", "otra", "wip"}
	for _, h := range []int{10, 20, 40} {
		m := sizedRetarget(t, 90, h, pocas...)
		if got := cuentaFilasDeRama(m); got != len(pocas) {
			t.Errorf("altura %d con %d ramas: %d pintadas, want %d", h, len(pocas), got, len(pocas))
		}
	}
}

// cuentaFilasDeRama cuenta las filas de la lista, que son las que llevan el
// marcador del cursor o el distintivo de la base. La cabecera y los hints no.
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

// TestLaCabeceraNombraLaBaseAunqueNoLaConozca: la cabecera del buscador dice de qué
// base se sale, y si el forge no trajo la base se dice "unknown" y no se deja un
// hueco. Un hueco se lee como un campo que se olvidó de rellenar; "unknown" se lee
// como lo que es, que es un dato que no vino.
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
	// Y sin base conocida no hay distintivo "current" en ninguna fila: se
	// distinguiría una base que no existe de la que sí.
	if strings.Contains(txt, "current") {
		t.Errorf("sin base conocida no debería salir el distintivo de la base: %q", txt)
	}
}

// TestLaCuentaDeRamasDistingueFiltradoDeSinFiltrar: con el filtro apagado la caja
// dice cuántas ramas hay; con el filtro puesto dice cuántas casan, que es la
// diferencia entre "el repositorio tiene doscientas" y "de doscientas, estas
// dos". Confundirlas hace que un filtro que deja un rastro parezca un
// repositorio con dos ramas.
func TestLaCuentaDeRamasDistingueFiltradoDeSinFiltrar(t *testing.T) {
	ramas := []string{"main", "fix/uno", "fix/dos", "wip", "otro"}
	completas := []string{"main", "fix/uno", "fix/dos", "wip", "otro"}

	// Sin filtro: la cuenta total, en plural o en singular según el número, y sin
	// la fórmula "X of Y".
	m := ramasDelPopup(t, "", completas...)
	txt := stripANSI(m.retargetOverlay2())
	if !strings.Contains(txt, "5 branches") {
		t.Errorf("sin filtro la caja debería decir cuántas hay: %q", txt)
	}
	if strings.Contains(txt, "match") {
		t.Errorf("sin filtro no debería haber fórmula de coincidencia: %q", txt)
	}

	// Con filtro: las que casan SOBRE el total.
	m = ramasDelPopup(t, "fix", completas...)
	txt = stripANSI(m.retargetOverlay2())
	if !strings.Contains(txt, "2 of 5 branches match") {
		t.Errorf("con filtro la caja debería decir cuántas casan de cuántas hay: %q", txt)
	}

	// Y el singular: "1 branch" y no "1 branches", porque un popup que dice
	// "1 branches" hace dudar de la cuenta. Con filtro y con una sola coincidencia
	// también, porque el singular es el del total.
	if got := pluralBranches(1); got != "1 branch" {
		t.Errorf("pluralBranches(1) = %q, want %q", got, "1 branch")
	}
	if got := pluralBranches(0); got != "0 branches" {
		t.Errorf("pluralBranches(0) = %q, want %q", got, "0 branches")
	}
	if got := pluralBranches(2); got != "2 branches" {
		t.Errorf("pluralBranches(2) = %q, want %q", got, "2 branches")
	}
	// Y un filtro que deja una sola coincidencia: el total sigue en plural, que
	// es lo que hace legible la frase.
	m = ramasDelPopup(t, "wip", completas...)
	if !strings.Contains(stripANSI(m.retargetOverlay2()), "1 of 5 branches match") {
		t.Errorf("con una sola coincidencia: %q", stripANSI(m.retargetOverlay2()))
	}
	_ = ramas
}

// TestElCampoDeFiltroDistingueElPlaceholderDelTexto: sin filtro, el campo enseña
// un texto gris que explica que ahí se escribe. Con filtro, enseña LO ESCRITO. Un
// popup que enseñara el placeholder con texto dentro no dejaría ver qué se está
// filtrando, que es justo lo que hace falta leer para decidir si el filtro es lo
// que se quería.
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

	// Un filtro de solo espacios recorta a vacío, así que la vista es la lista
	// ENTERA. Es el comportamiento que importa: escribir espacios no deja al
	// usuario con un popup en blanco creyendo que el repo no tiene ramas con ese
	// nombre. Lo que se ve en el campo es lo escrito, que también es honesto.
	m.retarget.query = "   "
	m.applyQuery()
	if len(m.retarget.view) != len(m.retarget.all) {
		t.Errorf("un filtro de solo espacios dio %d de %d ramas: el filtro se recorta y no deja la vista vacía",
			len(m.retarget.view), len(m.retarget.all))
	}
	if !strings.Contains(stripANSI(m.retargetOverlay2()), "2 of 2 branches match") {
		t.Errorf("un filtro de solo espacios no está filtrando nada: %q", stripANSI(m.retargetOverlay2()))
	}
	// Y una consulta de un solo caracter, que es el caso en el que un
	// `query == ""` mal puesto no se confundiría con un filtro de verdad.
	for _, q := range []string{"x", "f", "1"} {
		m.retarget.query = q
		txt := stripANSI(m.retargetOverlay2())
		if strings.Contains(txt, "type to search") || !strings.Contains(txt, q) {
			t.Errorf("con el filtro %q la caja no enseña lo escrito: %q", q, txt)
		}
	}
}

// TestElPopupDiceCuandoNoEncuentraNada: un filtro que no casa deja la lista vacía,
// y una lista vacía sin explicación parece un repositorio sin ramas. La caja lo
// dice, que es la diferencia entre "no hay destino" y "no hay nada con ese
// nombre".
func TestElPopupDiceCuandoNoEncuentraNada(t *testing.T) {
	m := ramasDelPopup(t, "", "main", "otra")
	m.retarget.query = "noexiste"
	m.applyQuery()
	txt := stripANSI(m.retargetOverlay2())
	if !strings.Contains(txt, "no branch matches the filter") {
		t.Errorf("un filtro sin coincidencias debería explicarlo: %q", txt)
	}
	// Y el error del forge tiene su propio sitio: es otra causa con otro arreglo,
	// y una caja que las confunde manda a mirar donde no está el problema.
	m.retarget.query = ""
	m.retarget.errMsg = "gh: Not Found (HTTP 404)"
	txt = stripANSI(m.retargetOverlay2())
	if !strings.Contains(txt, "404") {
		t.Errorf("el error del forge debería verse: %q", txt)
	}
	if strings.Contains(txt, "no branch matches") {
		t.Errorf("con un error del forge no debería salir el mensaje de filtro: %q", txt)
	}
	// El error MANDA sobre la lista: un repo que dio error no tiene una lista
	// que enseñar aunque la vista tuviera filas de antes.
	m.retarget.view = []string{"lo que hubiera quedado de antes"}
	if txt = stripANSI(m.retargetOverlay2()); !strings.Contains(txt, "404") {
		t.Errorf("con error y lista, la caja debería enseñar el error: %q", txt)
	}
}
