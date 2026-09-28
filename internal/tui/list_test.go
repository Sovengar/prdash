// Tests de la ventana de lista: reparto vertical, scroll que sigue al cursor y
// panel de detalle del ítem seleccionado.
package tui

import (
	"strconv"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/charmbracelet/x/ansi"

	"prdash/internal/forge/model"
	"prdash/internal/testutil"
)

// manyItems compone n ítems en la sección de creados, con título indexado para
// poder localizar una fila concreta en el texto pintado.
func manyItems(n int) []model.Item {
	items := make([]model.Item, 0, n)
	for i := 1; i <= n; i++ {
		items = append(items, mkItem("github", "github.com", "acme/widget", "Item "+strconv.Itoa(i), i, ""))
	}
	return items
}

func longModel(t *testing.T, n int) Model {
	t.Helper()
	m := newTestModel(t, ghAdapter())
	return send(t, m, page(1, "github", "github.com", model.SectionReview, model.ReviewRequested, manyItems(n), false))
}

// visibleLines cuenta las líneas de la vista ya sin códigos ANSI.
func visibleLines(view string) []string {
	return strings.Split(stripANSI(view), "\n")
}

// TestViewBoxesEverySection es el motivo del refactor: cada región de pantalla
// es una caja con borde redondeado y título embebido en su línea superior. La
// caja del inbox lleva la leyenda de conteos en vez del título "Inbox".
func TestViewBoxesEverySection(t *testing.T) {
	m := newTestModel(t, ghAdapter())
	m = send(t, m, page(1, "github", "github.com", model.SectionReview, model.ReviewRequested, []model.Item{
		mkItem("github", "github.com", "acme/widget", "Add widget", 1, ""),
	}, false))

	lines := visibleLines(m.View().Content)
	if len(lines) != m.height {
		t.Fatalf("la vista tiene %d líneas, want %d (la altura del terminal)", len(lines), m.height)
	}

	// Los títulos van embebidos en la línea superior de su caja, y cada caja
	// cierra con su borde inferior. La del inbox es la leyenda de conteos.
	for _, want := range []string{"PRDash", "Assigned (1)", "Keybinds"} {
		if !hasBoxTitle(lines, want) {
			t.Errorf("falta la caja %q:\n%s", want, strings.Join(lines, "\n"))
		}
	}
	if hasBoxTitle(lines, "Inbox") {
		t.Errorf("el título Inbox ya no debería existir: lo sustituye la leyenda:\n%s", strings.Join(lines, "\n"))
	}
	// La caja del detalle se titula con la referencia del ítem: es lo que dice de
	// un vistazo sobre qué ficha se trata.
	if !hasBoxTitle(lines, "acme/widget#1") {
		t.Errorf("la caja del detalle no se titula con la referencia del ítem:\n%s", strings.Join(lines, "\n"))
	}

	// Las esquinas de las cajas: cada ╭ abre y su ╯ correspondiente cierra.
	tops, bottoms := 0, 0
	for _, l := range lines {
		if strings.HasPrefix(l, "╭") {
			tops++
		}
		if strings.HasPrefix(l, "╰") {
			bottoms++
		}
	}
	if tops != bottoms || tops == 0 {
		t.Errorf("cajas descompensadas: %d aperturas ╭ y %d cierres ╰", tops, bottoms)
	}
}

// hasBoxTitle busca una caja cuyo borde superior embeba title.
func hasBoxTitle(lines []string, title string) bool {
	for _, l := range lines {
		if strings.HasPrefix(l, "╭") && strings.Contains(l, " "+title+" ") {
			return true
		}
	}
	return false
}

// TestViewFitsTerminalHeight es el motivo del scroll: con más ítems que líneas
// la vista ya no cabe en el terminal, así que hay que recortar en vez de
// desbordar (si no, la cabecera y la caja de atajos se van de pantalla).
func TestViewFitsTerminalHeight(t *testing.T) {
	m := longModel(t, 60)
	lines := visibleLines(m.View().Content)
	if len(lines) != m.height {
		t.Errorf("la vista tiene %d líneas, want %d (la altura del terminal)", len(lines), m.height)
	}
	// La caja de cabecera es la primera y lleva el nombre de la app embebido en
	// la línea de borde, como las demás.
	if !hasBoxTitle(lines, "PRDash") {
		t.Errorf("la caja de cabecera no titula la app: %q", lines[0])
	}
	if !strings.Contains(lines[len(lines)-2], "quit") {
		t.Errorf("los hints no están en la última línea de contenido: %q", lines[len(lines)-2])
	}
	if !strings.HasPrefix(lines[len(lines)-1], "╰") {
		t.Errorf("la vista no acaba en el borde inferior de la caja de atajos: %q", lines[len(lines)-1])
	}
}

// TestViewBoxesEveryLineAtTerminalWidth comprueba que la caja no se descuadra en
// horizontal: el borde se come 2 columnas del ancho de la terminal, y una línea
// más ancha que el interior se recortaría y rompería el borde.
func TestViewBoxesEveryLineAtTerminalWidth(t *testing.T) {
	m := longModel(t, 30)
	for i, l := range visibleLines(m.View().Content) {
		if w := ansi.StringWidth(l); w != m.width {
			t.Errorf("línea %d mide %d columnas, want %d:\n%q", i, w, m.width, l)
		}
	}
}

// TestScrollFollowsCursor comprueba el auto-scroll: al mover el cursor fuera de
// la ventana, la lista se desplaza lo justo para que la fila siga a la vista.
// El inbox ordena de más reciente a más antiguo, así que "Item 60" es la primera
// fila y "Item 1" la última.
func TestScrollFollowsCursor(t *testing.T) {
	m := longModel(t, 60)
	if m.scroll != 0 {
		t.Fatalf("scroll inicial = %d, want 0", m.scroll)
	}

	m = press(t, m, "end")
	if it, ok := m.selected(); !ok || it.Title != "Item 1" {
		t.Fatalf("con end el cursor debería estar en la última fila, no en %q", it.Title)
	}
	view := stripANSI(m.View().Content)
	if !strings.Contains(view, "Item 1") {
		t.Errorf("la fila del cursor no se ve:\n%s", view)
	}
	if strings.Contains(view, "Item 60") {
		t.Errorf("la lista se desplazó de más: la primera fila ya no debería verse:\n%s", view)
	}
	if m.scroll == 0 {
		t.Error("la lista no se desplazó con el cursor al final")
	}

	m = press(t, m, "home")
	view = stripANSI(m.View().Content)
	if !strings.Contains(view, "Item 60") {
		t.Errorf("con el cursor al principio debe verse la primera fila:\n%s", view)
	}
	if strings.Contains(view, "Item 1") {
		t.Errorf("la lista no volvió arriba:\n%s", view)
	}
	if m.scroll != 0 {
		t.Errorf("scroll tras home = %d, want 0", m.scroll)
	}
}

// TestPageKeysMoveOneWindow comprueba que pgup/pgdn mueven el cursor una
// ventana entera, que es lo que el usuario ve avanzar.
func TestPageKeysMoveOneWindow(t *testing.T) {
	m := longModel(t, 60)
	page := m.pageRows()
	if page < 2 {
		t.Fatalf("pageRows = %d, want >= 2", page)
	}

	m = press(t, m, "pgdown")
	if m.cursor != page {
		t.Errorf("cursor tras pgdown = %d, want %d", m.cursor, page)
	}
	if m.scroll != page {
		t.Errorf("la ventana tras pgdown = %d, want %d (debe avanzar una página entera)", m.scroll, page)
	}
	m = press(t, m, "pgup")
	if m.cursor != 0 {
		t.Errorf("cursor tras pgup = %d, want 0", m.cursor)
	}
	if m.scroll != 0 {
		t.Errorf("la ventana tras pgup = %d, want 0", m.scroll)
	}

	// Salir por encima o por debajo no rompe nada.
	m = press(t, m, "pgup")
	if m.cursor != 0 {
		t.Errorf("cursor tras pgup en el tope = %d, want 0", m.cursor)
	}
	for range 5 {
		m = press(t, m, "pgdown")
	}
	if m.cursor != len(m.rows())-1 {
		t.Errorf("cursor tras varios pgdown = %d, want %d", m.cursor, len(m.rows())-1)
	}
}

// TestScrollForAcomodaLaVentanaAlCursor cubre la aritmética del auto-scroll como
// función pura, donde los bordes sí importan y a través del modelo se pierden.
//
// Los tres casos son los que se rompen si la condición se invierte o el borde
// se mueve: la fila que cabe justo en la ventana no debe desplazar (si
// desplazara, la lista saltaría una línea antes de tiempo), la primera que se
// sale SÍ debe, y la última tiene que quedar pegada al borde inferior, no una
// más allá.
func TestScrollForAcomodaLaVentanaAlCursor(t *testing.T) {
	cases := []struct {
		name                         string
		current, target, total, view int
		want                         int
	}{
		{"el cursor ya se ve", 0, 3, 60, 10, 0},
		{"la última fila de la ventana cabe", 0, 9, 60, 10, 0},
		{"una por encima de la ventana sí desplaza", 0, 10, 60, 10, 1},
		{"el cursor está por encima", 5, 1, 60, 10, 1},
		{"el cursor es la primera línea", 5, 0, 60, 10, 0},
		// 60 líneas en una ventana de 10: la última fila es la 59, así que el
		// desplazamiento tiene que ser 59-10+1 = 50, y el recorte final lo
		// confirma (50+10 == 60, el contenido entero).
		{"la última fila se pega al borde", 0, 59, 60, 10, 50},
		{"el contenido cabe entero", 0, 5, 8, 10, 0},
		{"sin fila que seguir deja el scroll", 7, -1, 60, 10, 7},
		{"target negativo no arrastra el scroll a cero", 0, -1, 60, 10, 0},
		{"la ventana se acota al contenido", 40, 40, 12, 10, 2},
		{"una ventana mayor que el contenido deja arriba", 0, 0, 4, 10, 0},
		{"el scroll nunca queda negativo", -5, 0, 60, 10, 0},
		{"el scroll nunca se pasa del final", 99, 59, 60, 10, 50},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := scrollFor(c.current, c.target, c.total, c.view); got != c.want {
				t.Errorf("scrollFor(%d, %d, %d, %d) = %d, want %d",
					c.current, c.target, c.total, c.view, got, c.want)
			}
		})
	}
}

// TestCursorLineLocalizaLaFilaDelCursor: cursorLine devuelve el índice de la
// línea que corresponde a la fila del cursor, no el número de la fila. Se
// diferencia porque por encima de las filas hay líneas fijas (prefijo, avisos,
// header) y porque los avisos y el indicador de paginación llevan row: -1 y no
// deben confundirse con la fila 0.
func TestCursorLineLocalizaLaFilaDelCursor(t *testing.T) {
	lines := []listLine{
		{text: "prefijo", row: -1},
		{text: "aviso", row: -1},
		{text: "header", row: -1},
		{text: "fila 0", row: 0},
		{text: "fila 1", row: 1},
		{text: "loading more", row: -1},
	}
	// cursor 0 tiene que dar la fila 0, no la línea de prefijo: es justo el caso
	// donde el row: -1 de las líneas fijas puede confundirse con la primera fila.
	cases := map[int]int{0: 3, 1: 4, 2: -1, 99: -1}
	for cursor, want := range cases {
		if got := cursorLine(lines, cursor); got != want {
			t.Errorf("cursorLine(cursor=%d) = %d, want %d", cursor, got, want)
		}
	}
	// Y una lista sin filas de ítem no puede dar ninguna: el cursor no está.
	// Una lista de cromo puro (todo row: -1) es el caso real de una sección sin
	// ítems pero con avisos.
	if got := cursorLine([]listLine{{text: "vacío", row: -1}}, 0); got != -1 {
		t.Errorf("cursorLine sobre una lista sin filas = %d, want -1", got)
	}
	if got := cursorLine(nil, 0); got != -1 {
		t.Errorf("cursorLine(nil, 0) = %d, want -1", got)
	}
}

// TestVisibleListAcotaLaVentana al contenido: sin esto, un scroll desfasado por
// un refresco podría cortaría en blanco o se saldría del final. Con view <= 0 o
// sin líneas no hay nada que ver y se devuelve nil en vez de un tramo vacío.
func TestVisibleListAcotaLaVentanaAlContenido(t *testing.T) {
	lines := make([]listLine, 10)
	for i := range lines {
		lines[i] = listLine{row: i}
	}

	// Recorte normal: view líneas desde scroll.
	got := visibleList(lines, 3, 4)
	if len(got) != 4 || got[0].row != 3 || got[3].row != 6 {
		t.Errorf("visibleList(3, 4) = %d líneas empezando en %d", len(got), got[0].row)
	}
	// Un scroll que se pasa del final se recorta hacia atrás para llenar la
	// ventana: con 10 líneas y scroll 99, la ventana son las 6 últimas, no un
	// tramo de 2 al final. Un recorte de 2 dejaría medio panel en negro.
	got = visibleList(lines, 99, 4)
	if len(got) != 4 || got[0].row != 6 {
		t.Errorf("visibleList(99, 4) = %d líneas empezando en %d, want 4 desde 6", len(got), got[0].row)
	}
	// El recorte de la ventana es el único que puede devolver menos de `view`
	// líneas, y ocurre cuando el contenido no da para una ventana entera desde
	// el principio: 6 líneas en una ventana de 8 son 6, no 8.
	seis := lines[:6]
	got = visibleList(seis, 0, 8)
	if len(got) != 6 || got[0].row != 0 {
		t.Errorf("visibleList(6 líneas, view 8) = %d líneas, want 6 desde 0", len(got))
	}
	// Y la ventana se acota sin comerse una línea de más del contenido: desde 3
	// con view 3 sobre 6 líneas salen las 3 últimas, no 4.
	got = visibleList(seis, 3, 3)
	if len(got) != 3 || got[0].row != 3 {
		t.Errorf("visibleList(6 líneas, 3, 3) = %d líneas desde %d, want 3 desde 3", len(got), got[0].row)
	}
	// Un scroll desfasado se acota por los dos lados en vez de romper.
	got = visibleList(lines, -5, 3)
	if len(got) != 3 || got[0].row != 0 {
		t.Errorf("visibleList(-5, 3) = %d líneas empezando en %d, want 3 desde 0", len(got), got[0].row)
	}
	got = visibleList(lines, 99, 3)
	if len(got) != 3 || got[0].row != 7 {
		t.Errorf("visibleList(99, 3) = %d líneas empezando en %d, want 3 desde 7", len(got), got[0].row)
	}
	// Sin ventana o sin contenido no hay vista.
	if visibleList(lines, 0, 0) != nil || visibleList(nil, 0, 5) != nil {
		t.Error("sin ventana o sin líneas visibleList debería devolver nil")
	}
}

// TestListLinesComponeElCuerpoEnOrden fija la composición exacta del cuerpo, que
// es un contrato y no un detalle: las filas se numeran con `row` para que el
// cursor y el scroll las encuentren, y el desplazamiento de la lista depende de
// que `row` coincida con la posición real. Si un append metiera una fila con el
// `row` equivocado, el cursor apuntaría a otra fila y el scroll llevaría la
// ventana a un sitio que no corresponde.
//
// Se afirma también qué líneas fijas hay por encima y por debajo de las filas
// (prefijo, avisos, header, "loading more…"), porque son las que llevan row: -1
// y las que se intercalan sin romper la numeración.
func TestListLinesComponeElCuerpoEnOrden(t *testing.T) {
	m := newTestModel(t, ghAdapter())
	m = send(t, m, page(1, "github", "github.com", model.SectionReview, model.ReviewRequested, []model.Item{
		mkItem("github", "github.com", "acme/widget", "Uno", 1, ""),
		mkItem("github", "github.com", "acme/widget", "Dos", 2, ""),
	}, false))

	lines := m.listLines(m.contentWidth())
	var rows []int
	for _, l := range lines {
		rows = append(rows, l.row)
	}
	// Los dos ítems comparten el proyecto, así que hay prefijo: prefijo + header
	// + las dos filas. Lo que importa es que las filas empiecen en row 0 y sigan
	// la numeración: el prefijo y el header son cromo con row -1.
	want := []int{-1, -1, 0, 1}
	if len(rows) != len(want) {
		t.Fatalf("líneas = %v, want %v", rows, want)
	}
	for i := range want {
		if rows[i] != want[i] {
			t.Fatalf("rows = %v, want %v (la fila %d debe ser row %d)", rows, want, i, want[i])
		}
	}

	// La fila del cursor se localiza por row, y solo esa lleva el marcador.
	i0 := cursorLine(lines, 0)
	if i0 < 0 || lines[i0].row != 0 {
		t.Fatalf("cursorLine(0) = %d", i0)
	}
	conCursor := stripANSI(lines[i0].text)
	sinCursor := stripANSI(lines[cursorLine(lines, 1)].text)
	if conCursor == sinCursor {
		t.Errorf("la fila del cursor no se distingue de la otra:\n%s", conCursor)
	}

	// Con "loading more…" la línea de paginación va al final, con row: -1 para
	// que ni el cursor ni el scroll la confundan con un ítem. Sale de la misma
	// página con more=true, no de una segunda: el indicador describe la lista que
	// se está pintando.
	m2 := newTestModel(t, ghAdapter())
	m2 = send(t, m2, page(1, "github", "github.com", model.SectionReview, model.ReviewRequested, []model.Item{
		mkItem("github", "github.com", "acme/widget", "Uno", 1, ""),
	}, true))
	lines = m2.listLines(m2.contentWidth())
	last := lines[len(lines)-1]
	if last.row != -1 || !strings.Contains(stripANSI(last.text), "loading more") {
		t.Errorf("la última línea = %+v, want el indicador de paginación con row -1", last)
	}
}

// TestListLinesVaciaPintaElEstadoVacio: una sección sin ítems y sin avisos no
// pinta filas ni header, sino el estado vacío. Si se quitara ese caso, la lista
// saldría en blanco sin explicación, que es indistinguible de un bug de pintado.
func TestListLinesVaciaPintaElEstadoVacio(t *testing.T) {
	m := newTestModel(t, ghAdapter())
	m = send(t, m, page(1, "github", "github.com", model.SectionReview, model.ReviewRequested, nil, false))

	lines := m.listLines(m.contentWidth())
	if len(lines) != 1 {
		t.Fatalf("líneas = %d (%v), want solo la del estado vacío", len(lines), lines)
	}
	if lines[0].row != -1 || !strings.Contains(stripANSI(lines[0].text), "(empty)") {
		t.Errorf("línea = %+v, want el estado vacío con row -1", lines[0])
	}
	// Y con avisos, el aviso sustituye al estado vacío: hay algo que explicar, y
	// un "(empty)" al lado de un "⚠" sería una contradicción en pantalla. El
	// texto sale de problemText, que para un warning de red da la etiqueta corta
	// y no el Msg crudo.
	m = send(t, m, pageMsg{
		cycle:    1,
		key:      streamKey{forge: "github", section: model.SectionReview, kind: model.ReviewRequested},
		warnings: []model.Warning{{Forge: "github", Section: model.SectionReview, Kind: "network", Msg: "dial tcp: timeout"}},
	})
	lines = m.listLines(m.contentWidth())
	joined := ""
	for _, l := range lines {
		joined += stripANSI(l.text)
	}
	if !strings.Contains(joined, "github: could not be queried") {
		t.Errorf("con un aviso debería pintarse el aviso del forge:\n%s", joined)
	}
	if strings.Contains(joined, "(empty)") {
		t.Errorf("con un aviso no debería pintarse también el estado vacío:\n%s", joined)
	}
	// El aviso va por encima de las filas, con row: -1.
	if lines[0].row != -1 {
		t.Errorf("el aviso debería ser la primera línea con row -1, es %+v", lines[0])
	}
}

// TestSyncScrollNoSeRompeEnTerminalesDiminutos deja constancia del borde del
// auto-scroll: por muy enana que sea la terminal, el desplazamiento tiene que
// seguir a la fila del cursor.
//
// No es un test de la guarda `view <= 0` de syncScroll, y el motivo está aquí
// para que nadie lo busque después: computeLayout garantiza bodyLines >= 1
// (layout.go), así que esa guarda no se puede alcanzar desde el modelo. Es
// defensa ante un layout futuro que devuelva cero, no comportamiento actual. El
// caso que sí es real es el de abajo: una terminal tan pequeña que el cuerpo
// central se queda en la línea mínima.
func TestSyncScrollNoSeRompeEnTerminalesDiminutos(t *testing.T) {
	m := longModel(t, 60)
	m = press(t, m, "end")
	if it, ok := m.selected(); !ok || it.Title != "Item 1" {
		t.Fatalf("con end el cursor debería estar en la última fila, no en %q", it.Title)
	}

	// Se encoge la terminal y el desplazamiento tiene que seguir siendo válido:
	// acotado al contenido y con el cursor dentro de lo que se ve.
	for _, h := range []int{40, 12, 8, 5, 3, 1} {
		m.height = h
		m.syncScroll()
		lay := m.layout()
		if lay.bodyLines < 1 {
			t.Fatalf("height=%d: bodyLines = %d, pero el layout garantiza >= 1", h, lay.bodyLines)
		}
		lines := m.listLines(m.contentWidth())
		if m.scroll < 0 || m.scroll > max(0, len(lines)-1) {
			t.Errorf("height=%d: scroll = %d fuera de rango con %d líneas", h, m.scroll, len(lines))
		}
		// Y el cursor tiene que caer dentro de la ventana visible.
		if cl := cursorLine(lines, m.cursor); cl >= 0 {
			vis := visibleList(lines, m.scroll, lay.bodyLines)
			found := false
			for _, v := range vis {
				if v.row == m.cursor {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("height=%d: la fila del cursor no está en la ventana (línea %d de %d, %d visibles)",
					h, cl, len(lines), len(vis))
			}
		}
	}
}

// TestPadYTruncateMidenEnRunesYWEnBordes: las dos funciones que decide el ancho
// de la tabla. Los bordes importan y son los que se confunden:
//
//   - pad con una cadena que YA cabe no añade nada. Si añadiera, la celda se
//     saldría de su columna y la tabla bailaría al escribir encima.
//   - truncate con w=0 devuelve vacío, no un panic ni un "…" colgando: sin ancho
//     no hay nada que enseñar.
//   - truncate con w=1 devuelve solo elipsis, no el primer carácter más la
//     elipsis (que son dos columnas en una de ancho).
//   - truncate recorta a w runes exactos, contando el "…" como uno, porque es lo
//     que hace que la columna respete su ancho.
func TestPadYTruncateMidenEnRunesYWEnBordes(t *testing.T) {
	// pad: cuenta runes, no bytes, y no rellena si ya cabe.
	if got := pad("ab", 4); got != "ab  " {
		t.Errorf("pad(ab,4) = %q, want \"ab  \"", got)
	}
	if got := pad("ab", 2); got != "ab" {
		t.Errorf("pad con la cadena justa = %q, want %q (no rellena de más)", got, "ab")
	}
	if got := pad("abcd", 2); got != "abcd" {
		t.Errorf("pad de una cadena más larga = %q, want sin cambios (queda al truncado)", got)
	}
	// Con acentos y emoji: son runes, y un byte de más descuadraría la columna.
	if got := utf8.RuneCountInString(pad("áé", 4)); got != 4 {
		t.Errorf("pad con acentos = %d runes, want 4", got)
	}
	if got := pad("👍", 3); utf8.RuneCountInString(got) != 3 {
		t.Errorf("pad con un emoji = %d runes, want 3", utf8.RuneCountInString(got))
	}

	// truncate: el ancho es en runes y el "…" cuenta.
	if got := truncate("abcdef", 4); got != "abc…" {
		t.Errorf("truncate(abcdef,4) = %q, want \"abc…\"", got)
	}
	if got := utf8.RuneCountInString(truncate("abcdefgh", 5)); got != 5 {
		t.Errorf("truncate debe dar EXACTAMENTE 5 runes, dio %d", got)
	}
	// Sin recorte si ya cabe, o si cabe justo.
	if got := truncate("abc", 3); got != "abc" {
		t.Errorf("truncate de lo que cabe justo = %q, want sin cambios", got)
	}
	if got := truncate("abc", 10); got != "abc" {
		t.Errorf("truncate de lo que sobra sitio = %q, want sin cambios", got)
	}
	// Ancho 0 y 1: los bordes donde un "…" se colaría de más.
	if got := truncate("abc", 0); got != "" {
		t.Errorf("truncate(abc,0) = %q, want vacío", got)
	}
	if got := truncate("abc", 1); got != "…" {
		t.Errorf("truncate(abc,1) = %q, want solo la elipsis (dos columnas en una)", got)
	}
	if got := truncate("abc", -1); got != "" {
		t.Errorf("truncate con ancho negativo = %q, want vacío", got)
	}
	// Y con acentos: se recorta por runes, no por bytes, o partiría un carácter
	// por la mitad y la columna quedaría con un rune inválido.
	if got := truncate("áéíóú", 3); got != "áé…" {
		t.Errorf("truncate con acentos = %q, want \"áé…\"", got)
	}
}

// TestCenteredOriginColocaLaCaja: la esquina de un popup, y la función está
// compartida con la capa de gráficos a propósito (comentario en overlay.go): si
// el marco y la imagen calcularan su sitio por su cuenta, caerían en rectángulos
// distintos y solo se vería cuando coincidieran.
//
// El caso que importa es la caja más alta que el área: el centro saldría
// negativo y hay que pegarla al borde, no dejar el índice en negativo (que
// would panear la línea de arriba).
func TestCenteredOriginColocaLaCaja(t *testing.T) {
	cases := []struct {
		name                      string
		width, height, boxW, boxH int
		wantX, wantY              int
	}{
		{"caja centrada en un área mayor", 20, 10, 4, 4, 8, 3},
		{"caja que llena el área", 10, 10, 10, 10, 0, 0},
		{"más alta que el área", 20, 3, 4, 8, 8, 0},
		{"más ancha que el área", 3, 10, 10, 2, 0, 4},
		{"una fila de diferencia", 20, 11, 4, 4, 8, 3},
		{"impar por arriba", 21, 11, 4, 4, 8, 3},
		{"área de una línea", 20, 1, 4, 1, 8, 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			x, y := centeredOrigin(c.width, c.height, c.boxW, c.boxH)
			if x != c.wantX || y != c.wantY {
				t.Errorf("centeredOrigin(%d,%d,%d,%d) = %d,%d, want %d,%d",
					c.width, c.height, c.boxW, c.boxH, x, y, c.wantX, c.wantY)
			}
			// El origen nunca es negativo: un índice así parte la línea de arriba
			// al indexarla. Que la caja desborde por abajo SÍ es legitimo (una
			// ventana más alta que la pantalla), y lo recorta quien dibuja.
			if x < 0 || y < 0 {
				t.Errorf("origen negativo: x=%d y=%d", x, y)
			}
		})
	}
}

// TestOverlayCenteredRecortaLaCajaQueNoCabe: un popup más alto que la pantalla se
// recorta por abajo en vez de desbordar. Y una caja vacía no toca la vista, que es
// lo que evita que un popup sin contenido borre la pantalla.
func TestOverlayCenteredRecortaLaCajaQueNoCabe(t *testing.T) {
	// Fondo de líneas anchas para que la caja caiga DENTRO: el overlay recorta
	// la línea donde hace falta el texto de la caja, no borra la línea entera. Con
	// un fondo más corto que el desplazamiento, el texto de la caja se pegaría al
	// final de la línea en vez de en su sitio, que es el comportamiento de ansi.
	fondo := "aaaa\naaaa\naaaa\naaaa"
	got := overlayCentered(fondo, "1\n2\n3\n4\n5\n6", 10)
	lines := strings.Split(got, "\n")
	if len(lines) != 4 {
		t.Fatalf("líneas = %d, want 4 (el fondo no crece):\n%s", len(lines), got)
	}
	// x = (10-1)/2 = 4, así que la caja pisa la quinta columna de cada línea.
	for i, want := range []string{"aaaa1", "aaaa2", "aaaa3", "aaaa4"} {
		if lines[i] != want {
			t.Errorf("línea %d = %q, want %q (la caja se pinta encima del fondo)", i, lines[i], want)
		}
	}
	// Una caja vacía deja la vista como estaba.
	if got := overlayCentered(fondo, "", 10); got != fondo {
		t.Errorf("caja vacía = %q, want la vista intacta", got)
	}
	// Y la caja se recorta al ANCHO del viewport: una caja más ancha se pinta
	// recortada sobre el fondo, que conserva su longitud. Con x=0 porque la caja
	// es más ancha que el área, el resultado es la caja recortada + el resto del
	// fondo intacto.
	got = overlayCentered("aaaaaaaaaa", "0123456789", 6)
	linea := strings.Split(got, "\n")[0]
	if linea != "012345aaaa" {
		t.Errorf("línea = %q, want \"012345aaaa\" (caja recortada al ancho, fondo detrás)", linea)
	}
}

// TestDetailPaneShowsSelectedItem es el panel inferior: describe el ítem bajo el
// cursor y cambia al moverlo, con la ruta completa del forge. Los dos ítems van
// a la sección activa (Assigned) con el mismo UpdatedAt, para que el orden entre
// ellos sea determinista por número.
func TestDetailPaneShowsSelectedItem(t *testing.T) {
	m := newTestModel(t, ghAdapter(), &testutil.FakeAdapter{ForgeName: "gitlab", HostName: "gitlab.example.com"})
	first := mkItem("github", "github.com", "acme/widget", "PR de github", 1, "")
	second := mkItem("gitlab", "gitlab.example.com", "grp/proj", "MR de gitlab", 2, "")
	first.UpdatedAt = time.Unix(1000, 0)
	second.UpdatedAt = time.Unix(1000, 0)
	m = send(t, m, page(1, "github", "github.com", model.SectionReview, model.ReviewRequested, []model.Item{first}, false))
	m = send(t, m, page(1, "gitlab", "gitlab.example.com", model.SectionReview, model.ReviewRequested, []model.Item{second}, false))

	view := stripANSI(m.View().Content)
	for _, want := range []string{"PR de github", "github@github.com", "Item", "Author", "State"} {
		if !strings.Contains(view, want) {
			t.Errorf("el panel no contiene %q:\n%s", want, view)
		}
	}

	m = press(t, m, "down")
	view = stripANSI(m.View().Content)
	for _, want := range []string{"MR de gitlab", "gitlab@gitlab.example.com"} {
		if !strings.Contains(view, want) {
			t.Errorf("tras mover el cursor el panel no contiene %q:\n%s", want, view)
		}
	}
	if strings.Contains(view, "github@github.com") {
		t.Errorf("el panel sigue mostrando el forge del ítem anterior:\n%s", view)
	}
}

// TestDetailPaneEmptyWithoutSelection evita inventar datos cuando el inbox está
// vacío: el panel lo dice, y su caja se titula "Detail" porque no hay ninguna
// referencia que poner.
func TestDetailPaneEmptyWithoutSelection(t *testing.T) {
	m := newTestModel(t, ghAdapter())
	view := stripANSI(m.View().Content)
	if !strings.Contains(view, "no selection") {
		t.Errorf("con el inbox vacío el panel debería avisar:\n%s", view)
	}
	if !hasBoxTitle(visibleLines(m.View().Content), "Detail") {
		t.Errorf("la caja del panel debería titularse Detail sin selección:\n%s", view)
	}
}

// TestDetailPaneFitsShortScreenInTwoColumns comprueba el fallback: en una
// pantalla de 24 filas el 40% son 9 líneas y el detalle en una columna no cabe,
// así que se reparte en dos columnas antes que recortar campos. Forge y Autor
// solo existen en el panel, así que perderlos sería la peor opción.
func TestDetailPaneFitsShortScreenInTwoColumns(t *testing.T) {
	m := newTestModel(t, ghAdapter())
	m.width, m.height = 120, 24
	m = send(t, m, page(1, "github", "github.com", model.SectionReview, model.ReviewRequested, []model.Item{
		mkItem("github", "github.com", "acme/widget", "Add widget", 1, ""),
	}, false))

	detail := m.layout().detailLines
	if detail != 9 {
		t.Fatalf("panel de %d líneas, want 9 (el 40%% de 24)", detail)
	}
	it, ok := m.selected()
	lines := m.detailLines(it, ok, detail)
	if len(lines) != detail {
		t.Fatalf("el panel devolvió %d líneas, want %d", len(lines), detail)
	}
	joined := stripANSI(strings.Join(lines, "\n"))
	for _, want := range []string{"Add widget", "Item:", "Forge:", "github@github.com", "Author:", "State:", "Role:"} {
		if !strings.Contains(joined, want) {
			t.Errorf("el panel de dos columnas perdió %q:\n%s", want, joined)
		}
	}
}

// TestDetailPaneClipsOnlyWhenTooSmall comprueba el último recurso: si ni
// siquiera en dos columnas cabe, se recorta por arriba lo que falta. El final
// (estado, review y rol) es lo que dice si la acción procede.
func TestDetailPaneClipsOnlyWhenTooSmall(t *testing.T) {
	m := longModel(t, 1)
	it, ok := m.selected()
	clipped := m.detailLines(it, ok, 4)
	if len(clipped) != 4 {
		t.Fatalf("el recorte devolvió %d líneas, want 4", len(clipped))
	}
	joined := stripANSI(strings.Join(clipped, "\n"))
	for _, want := range []string{"Review:", "Role:"} {
		if !strings.Contains(joined, want) {
			t.Errorf("el recorte perdió %q:\n%s", want, joined)
		}
	}
	if strings.Contains(joined, "Forja:") || strings.Contains(joined, "Item:") {
		t.Errorf("el recorte debería haber dejado caer la cabecera del detalle:\n%s", joined)
	}
}

// TestDetailShowsDiffStat: el detalle es donde el diffstat se ve siempre, con
// las cifras sin compactar y el recuento de ficheros, porque la columna de la
// lista abrevia y puede no estar.
func TestDetailShowsDiffStat(t *testing.T) {
	it := mkItem("gitlab", "gitlab.example.com", "grp/proj", "MR", 1015, "")
	it.Diff = model.DiffStat{Additions: 42, Deletions: 1, Files: 2, Known: true}
	m := newTestModel(t, ghAdapter(), &testutil.FakeAdapter{ForgeName: "gitlab", HostName: "gitlab.example.com"})
	m = send(t, m, page(1, "gitlab", "gitlab.example.com", model.SectionReview, model.ReviewRequested, []model.Item{it}, false))

	view := stripANSI(m.View().Content)
	if !strings.Contains(view, "+42 -1 (2 files)") {
		t.Errorf("el detalle debería mostrar el diffstat agregado:\n%s", view)
	}
}

// TestDetailDiffUnknownAndEmpty: los dos ceros significan cosas distintas y el
// detalle las distingue. Desconocido lo dice, para que un "-" no se lea como un
// MR que no toca nada.
func TestDetailDiffUnknownAndEmpty(t *testing.T) {
	cases := []struct {
		name string
		d    model.DiffStat
		want string
	}{
		{"sin datos", model.DiffStat{}, "unknown (forge did not report it)"},
		{"sin cambios", model.DiffStat{Known: true}, "no changes"},
		{"un fichero", model.DiffStat{Additions: 3, Deletions: 0, Files: 1, Known: true}, "+3 -0 (1 file)"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			it := mkItem("github", "github.com", "acme/widget", "PR", 1, "")
			it.Diff = c.d
			m := newTestModel(t, ghAdapter())
			m = send(t, m, page(1, "github", "github.com", model.SectionReview, model.ReviewRequested, []model.Item{it}, false))
			view := stripANSI(m.View().Content)
			if !strings.Contains(view, c.want) {
				t.Errorf("el detalle debería mostrar %q:\n%s", c.want, view)
			}
		})
	}
}

// TestDetailDropsDiffBeforeLosingTheTitle: al añadir el diffstat la rejilla pasó
// de 6 a 7 filas. Cuando el panel no cabe, el diffstat es lo primero que se cae
// —nunca se vio, no se pierde— y el título se conserva.
func TestDetailDropsDiffBeforeLosingTheTitle(t *testing.T) {
	detail := func(rows int) string {
		m := newTestModel(t, ghAdapter())
		m.width, m.height = 120, 24
		it := mkItem("github", "github.com", "acme/widget", "Add widget", 1, "")
		it.Diff = model.DiffStat{Additions: 42, Deletions: 1, Files: 2, Known: true}
		m = send(t, m, page(1, "github", "github.com", model.SectionReview, model.ReviewRequested, []model.Item{it}, false))
		// El aviso de acción deshabilitada es el del forge (`denied`), el único que la
		// ficha sigue pintando: ocupa dos filas (blanco + texto) y es lo que obliga a
		// la rejilla a ceder.
		m.denied[it.ID()] = "the forge refused the action"
		return stripANSI(strings.Join(m.detailPane(rows), "\n"))
	}

	// 9 filas es el 40% de un terminal de 24: no cabe la rejilla de 7 con el
	// título y el aviso de acción, así que el diffstat se va.
	tight := detail(9)
	if strings.Contains(tight, "Diff:") {
		t.Errorf("a 9 filas el diffstat debería caerse antes que el título:\n%s", tight)
	}
	if !strings.Contains(tight, "Add widget") {
		t.Errorf("el título no debería caerse por culpa del diffstat:\n%s", tight)
	}
	if !strings.Contains(tight, "Role:") {
		t.Errorf("el rol dice si la acción procede y no puede caerse:\n%s", tight)
	}

	// Con sitio, el diffstat sale: el detalle es la vista que no pierde campos.
	if roomy := detail(11); !strings.Contains(roomy, "+42 -1 (2 files)") {
		t.Errorf("a 11 filas el diffstat debería estar:\n%s", roomy)
	}
}
