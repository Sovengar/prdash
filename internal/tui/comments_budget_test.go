package tui

import (
	"fmt"
	"strings"
	"testing"
	"unicode/utf8"

	"prdash/internal/forge"
	"prdash/internal/forge/model"
)

// Up and down have to be the SAME operation.
func TestLasFlechasSiguenLaListaEnLosDosSentidos(t *testing.T) {
	items := make([]model.Item, 4)
	for i := range items {
		items[i] = mkItem("github", "github.com", "acme/widget", "Uno", i+1, "")
	}
	n := len(items)
	nueva := func() Model {
		return send(t, newTestModel(t, ghAdapter()),
			page(1, "github", "github.com", model.SectionReview, model.ReviewRequested, items, false))
	}

	m := nueva()
	if m.cursor != 0 {
		t.Fatalf("el cursor inicial = %d, want 0", m.cursor)
	}
	for i := range items {
		m = press(t, m, "down")
		want := min(i+1, n-1)
		if m.cursor != want {
			t.Fatalf("tras %d pasos abajo el cursor = %d, want %d", i+1, m.cursor, want)
		}
	}
	m = press(t, m, "down")
	if m.cursor != n-1 {
		t.Errorf("abajo en el ultimo dio %d, want %d: la lista no es circular", m.cursor, n-1)
	}
	m = press(t, m, "j")
	if m.cursor != n-1 {
		t.Errorf("j en el ultimo dio %d, want %d", m.cursor, n-1)
	}

	m = nueva()
	m.cursor = n - 1
	m = press(t, m, "up")
	if m.cursor != n-2 {
		t.Fatalf("arriba desde el ultimo dio %d, want %d", m.cursor, n-2)
	}
	m = press(t, m, "k")
	if m.cursor != n-3 {
		t.Errorf("k dio el cursor %d, want %d", m.cursor, n-3)
	}
	m.cursor = 1
	m = press(t, m, "up")
	if m.cursor != 0 {
		t.Errorf("arriba desde la segunda fila dio %d, want 0", m.cursor)
	}
	m = press(t, m, "up")
	if m.cursor != 0 {
		t.Errorf("arriba en el primero dio %d, want 0: la lista no es circular", m.cursor)
	}

	m = nueva()
	m.cursor = n - 1
	for want := n - 2; want >= 0; want-- {
		m = press(t, m, "up")
		if m.cursor != want {
			t.Fatalf("vuelta hacia arriba: el cursor = %d, want %d", m.cursor, want)
		}
	}
}

// With no rows there is nowhere to paint, and that is the first cut.
func TestSinFilasNoSePintaNiUnAviso(t *testing.T) {
	m := modelWithComments(t, 45, []model.Comment{conv("alice", "hola")}, 1)
	it := mustSelected(t, m)
	it.ID()
	denied := it
	deniedID := denied.ID()
	m.denied[deniedID] = "no permission"
	inner := m.contentWidth()

	for _, avail := range []int{-5, -1, 0, 1} {
		if got := m.commentLines(it, avail, inner); len(got) != 0 {
			t.Errorf("con avail=%d devolvió %d líneas, want 0: sin sitio no se pinta nada, ni un aviso",
				avail, len(got))
		}
	}
	if got := m.commentLines(it, commentChrome-1, inner); len(got) != 0 {
		t.Errorf("con avail=%d devolvió %d líneas, want 0: menos que el marco no cabe la caja",
			commentChrome-1, len(got))
	}
}

// GitLab does not expose the count.
func TestElRecuentoNuncaInventaMasDeLosQueSeVen(t *testing.T) {
	casos := []struct {
		name  string
		total int
		want  int
	}{
		{"total mayor que la lista", 9, 9},
		{"total igual a la lista", 3, 3},
		{"total menor que la lista", 1, 3},
		{"total cero con lista", 0, 3},
		{"total negativo", -7, 3},
	}
	for _, c := range casos {
		t.Run(c.name, func(t *testing.T) {
			list := []model.Comment{conv("alice", "uno"), conv("bob", "dos"), conv("carol", "tres")}
			m := modelWithComments(t, 45, list, c.total)
			it := mustSelected(t, m)
			id := it.ID()
			st := m.comments[id]
			if st == nil {
				t.Fatal("no hay estado de comentarios")
			}
			if st.total != c.want {
				t.Errorf("el total se quedó en %d, want %d: el recuento no puede ser menor que lo que se ve", st.total, c.want)
			}
			// The border counts from the list, not from the inverted total.
			rows := m.commentLines(it, 30, m.contentWidth())
			if len(rows) == 0 {
				t.Fatal("no se pintó la caja")
			}
			leyenda := stripANSI(strings.Join(rows, "\n"))
			if !strings.Contains(leyenda, "3 of "+itoaSmall(c.want)) {
				t.Errorf("el borde no dice 3 de %d: %q", c.want, leyenda)
			}
		})
	}
}

func itoaSmall(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	if neg {
		return "-" + string(b)
	}
	return string(b)
}

func TestLaCajaTomaLoQueNecesitaYNoSePasa(t *testing.T) {
	uno := []model.Comment{conv("alice", "uno")}
	tres := []model.Comment{conv("alice", "uno"), conv("bob", "dos"), conv("carol", "tres")}

	m1 := modelWithComments(t, 45, uno, 1)
	it1 := mustSelected(t, m1)
	if rows := m1.commentLines(it1, commentChrome+1, m1.contentWidth()); len(rows) != commentChrome+1 {
		t.Errorf("presupuesto justo: la caja ocupa %d filas, want %d", len(rows), commentChrome+1)
	}
	for _, avail := range []int{commentChrome + 2, commentChrome + 4, 12, 30} {
		rows := m1.commentLines(it1, avail, m1.contentWidth())
		if len(rows) > avail {
			t.Errorf("con avail=%d la caja ocupa %d filas: se pasa del presupuesto", avail, len(rows))
		}
		if want := commentChrome + 1; len(rows) != want {
			t.Errorf("con avail=%d la caja ocupa %d filas, want %d: no rellena el hueco de más", avail, len(rows), want)
		}
	}

	m3 := modelWithComments(t, 45, tres, 3)
	it3 := mustSelected(t, m3)
	if rows := m3.commentLines(it3, commentChrome+3, m3.contentWidth()); len(rows) != commentChrome+3 {
		t.Errorf("presupuesto justo de 3 comentarios: %d filas, want %d", len(rows), commentChrome+3)
	}
	if rows := m3.commentLines(it3, commentChrome+2, m3.contentWidth()); len(rows) != 0 {
		t.Errorf("con 3 comentarios y una fila de menos la caja se pintó a medias (%d filas)", len(rows))
	}
	for _, avail := range []int{commentChrome + 3, commentChrome + 5, commentChrome + 9, 40} {
		if rows := m3.commentLines(it3, avail, m3.contentWidth()); len(rows) > avail {
			t.Errorf("con avail=%d y 3 comentarios la caja ocupa %d: se pasa", avail, len(rows))
		}
	}
}

// The body goes INSIDE the box.
func TestElCuerpoSeParteAlAnchoDeLaCajaYNoAlDeLaTerminal(t *testing.T) {
	frase := strings.Repeat("x", 400)
	for _, inner := range []int{20, 30, 40, 60, 80, 120, 200} {
		m := modelWithComments(t, 45, []model.Comment{conv("alice", frase)}, 1)
		it := mustSelected(t, m)
		rows := m.commentLines(it, 30, inner)
		if len(rows) == 0 {
			t.Errorf("inner %d: no se pintó la caja", inner)
			continue
		}
		// No row goes past the box's width. That is the invariant.
		for i, l := range rows {
			if w := utf8.RuneCountInString(stripANSI(l)); w > inner {
				t.Errorf("inner %d: la fila %d mide %d columnas, want <= %d: el cuerpo no cabe en su caja",
					inner, i, w, inner)
			}
		}
	}
	// The width really matters: the same body needs more rows in a narrow box.
	cuerpo := strings.Repeat("palabra ", 60)
	filasDe := func(inner int) int {
		m := modelWithComments(t, 45, []model.Comment{conv("alice", cuerpo)}, 1)
		it := mustSelected(t, m)
		return len(m.commentLines(it, 30, inner))
	}
	estrechas, anchas := filasDe(20), filasDe(200)
	if estrechas <= anchas {
		t.Errorf("con la caja estrecha salen %d filas y con la ancha %d: el ancho del cuerpo no está mandando",
			estrechas, anchas)
	}
	// The difference arrives even with maxCommentLines clipping both.
	t.Logf("estrecha %d filas, ancha %d (tope por comentario: %d)", estrechas, anchas, maxCommentLines)
}

// The count lives on the border, not in the body.
func TestLaLeyendaDiceCuantosSeVenDeCuantosHay(t *testing.T) {
	list := make([]model.Comment, 3)
	for i := range list {
		list[i] = conv("alice", "uno")
	}
	m := modelWithComments(t, 45, list, 3)
	it := mustSelected(t, m)
	rows := m.commentLines(it, 30, m.contentWidth())
	if len(rows) == 0 {
		t.Fatal("no se pintó la caja")
	}
	plano := stripANSI(strings.Join(rows, "\n"))
	if !strings.Contains(plano, "3 of 3") {
		t.Errorf("con todo visto el borde debería decir 3 de 3: %q", plano)
	}

	m2 := modelWithComments(t, 45, list, 12)
	it2 := mustSelected(t, m2)
	plano = stripANSI(strings.Join(m2.commentLines(it2, 30, m2.contentWidth()), "\n"))
	if !strings.Contains(plano, "3 of 12") {
		t.Errorf("con 12 en el total el borde debería decir 3 de 12: %q", plano)
	}
	// And the hint that there is more appears when there is more.
	if !strings.Contains(plano, commentHint) {
		t.Errorf("con 12 en el total debería salir la pista de que hay más: %q", plano)
	}

	// With a narrow gap the hint is dropped but the count stays, because the number is the datum.
	m3 := modelWithComments(t, 45, list, 12)
	it3 := mustSelected(t, m3)
	plano = stripANSI(strings.Join(m3.commentLines(it3, 30, 12), "\n"))
	if !strings.Contains(plano, "3 of 12") {
		t.Errorf("con la caja estrecha el recuento no puede caerse: %q", plano)
	}
	// The pure legend, unpainted, for the arithmetic that decides whether the hint fits.
	const (
		espacioJusto = 45
		espacioCorto = 44
	)
	anchoLeyenda := len([]rune(fmt.Sprintf("%d of %d", 3, 12) + commentHint))
	for _, c := range []struct {
		shown, total, outer int
		wantPista           bool
	}{
		{3, 3, espacioJusto, false},  // todo visto: nada que anunciar
		{3, 12, espacioJusto, true},  // hay más y cabe JUSTO
		{3, 12, espacioCorto, false}, // un rune menos y ya no cabe
		{3, 12, 60, true},            // y con hueco de sobra, también
		{1, 1, espacioJusto, false},  // uno de uno
		{5, 5, espacioJusto, false},  // cinco de cinco
		{3, 12, 8, false},            // ni el suelo da
	} {
		got := commentLegend(c.shown, c.total, c.outer)
		if !strings.Contains(stripANSI(got), itoaSmall(c.shown)+" of "+itoaSmall(c.total)) {
			t.Errorf("commentLegend(%d, %d, %d) = %q, no dice el recuento",
				c.shown, c.total, c.outer, stripANSI(got))
		}
		if tienePista := strings.Contains(stripANSI(got), commentHint); tienePista != c.wantPista {
			t.Errorf("commentLegend(%d, %d, %d) %s la pista, want que %s (la leyenda y la pista miden %d runes y el hueco es %d)",
				c.shown, c.total, c.outer,
				map[bool]string{true: "trae", false: "no trae"}[tienePista],
				map[bool]string{true: "la traiga", false: "no la traiga"}[c.wantPista],
				anchoLeyenda,
				commentBoxWidth(c.outer)-commentBoxBorder-legendGap)
		}
	}
}

// At most five comments are shown, even when there are twenty.
func TestElTopeDeCincoSeAplicaYSeDice(t *testing.T) {
	for _, n := range []int{1, 4, 5, 6, 20} {
		list := make([]model.Comment, n)
		for i := range list {
			list[i] = conv("alice", "uno")
		}
		m := modelWithComments(t, 80, list, n)
		it := mustSelected(t, m)
		rows := m.commentLines(it, 60, m.contentWidth())
		if len(rows) == 0 {
			t.Errorf("con %d comentarios no se pintó la caja", n)
			continue
		}
		plano := stripANSI(strings.Join(rows, "\n"))
		want := n
		if want > forge.CommentLimit {
			want = forge.CommentLimit
		}
		if !strings.Contains(plano, itoaSmall(want)+" of "+itoaSmall(n)) {
			t.Errorf("con %d comentarios el borde debería decir %d de %d, no más: %q", n, want, n, plano)
		}
		// And the ones not shown do not appear: it is a cap, not an arbitrary trim.
		for i := forge.CommentLimit; i < n; i++ {
			if strings.Contains(plano, "comment-"+itoaSmall(i)) {
				t.Errorf("con %d comentarios se enseñó el %d, que está por encima del tope de %d", n, i, forge.CommentLimit)
			}
		}
	}
}
