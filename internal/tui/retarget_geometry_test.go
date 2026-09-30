package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

// sizedRetarget deja el popup de retarget abierto con un tamaño de terminal
// concreto, que es la palanca de la que dependen todas las medidas del popup.
func sizedRetarget(t *testing.T, w, h int, branches ...string) Model {
	t.Helper()
	m, _ := retargetFixture(t, branches...)
	return send(t, m, tea.WindowSizeMsg{Width: w, Height: h})
}

// TestElPopupDeRetargetRespetaLaTerminalEnLasDosDimensiones: el popup se dibuja
// ENCIMA de la vista, así que su geometría no puede depender del capricho de cada
// etiqueta: todas sus líneas miden lo mismo, y ese ancho es el del contenido
// acotado al ancho de diseño del popup.
//
// El suelo de 38 columnas del layout es parte del contrato (por debajo de eso la
// ficha no se lee), así que en una terminal más estrecha que el suelo la caja se
// sale a propósito. Lo que no puede pasar es que la caja sea más estrecha que su
// contenido, porque eso rompe el marco por dentro y parece un bug de dibujo.
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
			// El ancho nunca pasa del de diseño del popup, por ancha que sea la
			// terminal: 64 columnas es el ancho que el texto está escrito para.
			if ancho > retargetChooserWidth {
				t.Errorf("terminal %dx%d: caja de %d columnas, want <= %d", w, h, ancho, retargetChooserWidth)
			}
			// Y nunca por debajo del contenido disponible.
			if want := min(m.contentWidth(), retargetChooserWidth); ancho != want {
				t.Errorf("terminal %dx%d: caja de %d columnas, want %d (contenido acotado)", w, h, ancho, want)
			}
		}
	}
}

// TestLasFilasDelPopupCabenEntreElMargenYElTecho: el número de filas de rama es
// lo que decide cuántas se ven a la vez. Sale de lo que queda tras el margen
// (arriba y abajo) y el marco, con un suelo y un techo.
//
// El suelo importa en una terminal diminuta: sin él, `free` negativo deja el
// popup en cero filas y el buscador no muestra ni una rama, que es un popup que
// parece vacío. El techo importa en una terminal enorme: sin él, el popup
// reserva doce filas para tres ramas y la caja queda con un hueco enorme debajo.
func TestLasFilasDelPopupCabenEntreElMargenYElTecho(t *testing.T) {
	branches := []string{"main", "a/1", "a/2", "a/3", "a/4", "a/5", "a/6", "a/7", "a/8", "a/9", "a/10", "a/11", "a/12", "a/13"}
	for _, h := range []int{4, 6, 8, 10, 14, 20, 40, 100} {
		m := sizedRetarget(t, 80, h, branches...)
		got := m.retargetRows()

		// El suelo: por debajo de tres filas el buscador no sirve de nada, así
		// que se recorta a la terminal pero nunca por debajo del suelo.
		if got < retargetMinRows {
			t.Errorf("altura %d: %d filas, want >= %d (sin filas el popup parece vacío)", h, got, retargetMinRows)
		}
		// El techo: nunca más de las que el diseño quiere.
		if got > retargetRows {
			t.Errorf("altura %d: %d filas, want <= %d", h, got, retargetRows)
		}
		// Y la relación con la terminal: lo que se ve más el marco cabe, con el
		// margen. Con la terminal justa, las filas se recortan al hueco real.
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

// TestMoverElCursorRecalculaLaVentana: filtro y flechas comparten la misma cuenta
// de ventana (retargetWindow), y esa unicidad es lo que evita que el render tenga
// su propia aritmética y las dos digan cosas distintas sobre qué se ve.
//
// El caso que separa los cortes es la lista que mide JUSTO lo que la ventana
// muestra: con una fila menos, arrancar en la posición 1 escondería la primera
// rama sin motivo, y eso es un popup que pierde una opción sin avisar.
func TestMoverElCursorRecalculaLaVentana(t *testing.T) {
	m := sizedRetarget(t, 80, 60, "a", "b", "c", "d", "e")
	// Con la ventana generosa, la lista cabe entera y `win` es cero siempre: la
	// última fila tiene que ser visible desde arriba.
	if win := m.retargetWindow(); win != 0 {
		t.Errorf("con la lista entera win = %d, want 0", win)
	}

	// Con la lista midiendo justo lo que la ventana muestra, y el cursor arriba,
	// la ventana no avanza: la primera fila se ve.
	filas := m.retargetRows()
	m.retarget.view = seqBranches(filas)
	m.retarget.cursor, m.retarget.win = 0, 0
	if win := m.retargetWindow(); win != 0 {
		t.Errorf("lista de %d filas con ventana de %d y cursor arriba: win = %d, want 0: la primera rama se vería escondida",
			filas, filas, win)
	}

	// Y bajando hasta el final, la ventana se mueve lo justo para que la última
	// fila entre, sin pasarse de la lista.
	m.retarget.view = seqBranches(filas + 4)
	for cursor := 0; cursor < filas+4; cursor++ {
		m.retarget.cursor, m.retarget.win = cursor, 0
		// Es lo que hace moveRetargetCursor: la cuenta va en retargetWindow y el
		// render lee m.retarget.win, así que el test tiene que pasar por la misma
		// asignación que la production.
		m.retarget.win = m.retargetWindow()
		visibles, start := m.retargetVisible()
		if cursor < start || cursor >= start+len(visibles) {
			t.Errorf("cursor %d: la ventana [%d, %d) no lo contiene (visibles %d)", cursor, start, start+len(visibles), len(visibles))
		}
		if start+len(visibles) > len(m.retarget.view) {
			t.Errorf("cursor %d: la ventana [%d, %d) se pasa de las %d ramas", cursor, start, start+len(visibles), len(m.retarget.view))
		}
	}

	// Y mover hacia arriba desde abajo devuelve la ventana a cero, en vez de
	// dejarla a medio camino: el cursor acaba arriba y la lista también.
	m.retarget.cursor, m.retarget.win = filas+3, 0
	m.moveRetargetCursor(-(filas + 3))
	if m.retarget.cursor != 0 {
		t.Errorf("cursor = %d tras subir del todo, want 0", m.retarget.cursor)
	}
	if m.retarget.win != 0 {
		t.Errorf("win = %d tras subir del todo, want 0", m.retarget.win)
	}
	// Con la lista más corta que la ventana, la ventana no puede arrancar más
	// allá del principio: no hay nada que recorrer.
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

// TestLaFlechaDeLaConfirmacionCaeEnUnaColumnaFija: la línea del medio de la caja
// de confirmación pone la base de la que se sale a la izquierda y el destino a la
// derecha, unidos por una flecha. La base se rellena a media caja, así que la
// flecha cae SIEMPRE en la misma columna mientras la base quepa: eso es lo que
// hace que dos cajas de retarget distintos se lean de un vistazo. Si la columna
// dependiera del texto, dos cajas distintas tendrían las flechas en sitios
// distintos y la comparación se iría al ojo.
//
// El relleno es un SUELO, no un recorte: una base más ancha que media caja no se
// corta, empuja la flecha y se lleva el espacio del destino por delante. Se
// afirma a propósito, para que el comportamiento sea el que está escrito y no el
// que salga por casualidad.
func TestLaFlechaDeLaConfirmacionCaeEnUnaColumnaFija(t *testing.T) {
	columna := func(base string) int {
		t.Helper()
		m, _ := retargetFixture(t, base, "destino")
		m.retarget.item.TargetBranch = base
		m.retarget.cursor = 1
		m.retarget.chosen = "destino"
		m.retarget.state = retargetConfirm

		box := stripANSI(m.retargetConfirmBox())
		// El relleno va desde dentro del borde, y la flecha tras un espacio: la
		// columna es el borde + el relleno + ese espacio.
		wantCol := 2 + (m.retargetBoxWidth()-6)/2
		visto := -1
		for _, l := range strings.Split(box, "\n") {
			i := strings.Index(l, "→")
			if i < 0 {
				continue
			}
			col := ansi.StringWidth(l[:i])
			// Una base que cabe en media caja: la flecha cae en la columna fija.
			if ansi.StringWidth(base) <= (m.retargetBoxWidth()-6)/2 && col != wantCol {
				t.Errorf("base %q: la flecha cae en la columna %d, want %d (media caja, el relleno de la base): %q",
					base, col, wantCol, l)
			}
			// Y en cualquier caso la flecha va pegada al destino, con su espacio.
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

	// Bases de longitudes muy distintas: la columna no se mueve.
	want := columna("main")
	for _, base := range []string{"main", "release/2.0", "feature/x", "x", "fix/hunk"} {
		if got := columna(base); got != want {
			t.Errorf("la columna de la flecha depende de la base: %q dio %d y %q dio %d", "main", want, base, got)
		}
	}
	// Y una base que NO cabe empuja la flecha en vez de recortarse: el nombre
	// completo es el dato, y recortarlo a media caja dejaría sin saber a qué base
	// se vuelve. Se anota para que el empuje sea conocido y no un accidente.
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
	// El aviso de lo que deja de ser verdad: sin él, la caja solo dice qué teclas
	// hay y el usuario no sabe que está a punto de invalidar una aprobación.
	if !strings.Contains(box, "recomputed") {
		t.Errorf("la caja debería decir que se recalcula: %q", box)
	}

	// Sin base conocida: "unknown", no un hueco.
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

// TestRetargetProgressNoticeDistingueLasDosRamas: el aviso de retarget en curso
// es lo único que separa un retarget de otro en una lista de avisos iguales, y lo
// que el usuario tiene que poder leer mientras espera. Sin la base de origen, se
// dice igual que no se dice nada, y con el par de ramas se lee qué pasó.
func TestRetargetProgressNoticeDistingueLasDosRamas(t *testing.T) {
	conBase := stripANSI(retargetProgressNotice("main", "release/2.0"))
	if !strings.Contains(conBase, "main") || !strings.Contains(conBase, "release/2.0") {
		t.Errorf("el aviso debería nombrar las dos ramas: %q", conBase)
	}
	// Sin base de origen, el aviso se acorta pero sigue siendo un aviso.
	sinBase := stripANSI(retargetProgressNotice("", "release/2.0"))
	if !strings.Contains(sinBase, "release/2.0") {
		t.Errorf("sin base de origen el aviso debería nombrar el destino: %q", sinBase)
	}
	if strings.Contains(sinBase, "( →") || strings.Contains(sinBase, "(→  ") {
		t.Errorf("sin base de origen el aviso dejó un hueco: %q", sinBase)
	}
	// Y los dos avisos se distinguen a simple vista: si fueran iguales, la lista
	// de avisos no diría nada.
	if conBase == sinBase {
		t.Errorf("con y sin base de origen el aviso es el mismo: %q", conBase)
	}
}
