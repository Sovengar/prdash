package tui

import (
	"fmt"
	"strings"
	"testing"
	"unicode/utf8"

	"prdash/internal/forge"
	"prdash/internal/forge/model"
)

// TestLasFlechasSiguenLaListaEnLosDosSentidos: arriba y abajo tienen que ser la
// MISMA operacion con el signo cambiado. La de abajo estaba probada y la de arriba
// no, y esa asimetria se nota: un `+ 1` en la de arriba la convertiria en un
// segundo "abajo" que seguiria funcionando desde el medio de la lista y solo
// fallaria en el borde, y un `- 2` saltaria una fila.
//
// La lista NO es circular: se TOPA en los extremos, al contrario que el selector de
// ramas, que si es circular. Esa diferencia es intentional —una lista de trabajo
// no da la vuelta, y al dabajo de la ultima estarian avisos y no items— y es justo
// el borde que distingue un `- 1` bien puesto de un `+ 1`: en el medio de la lista
// los dos se comportan igual y solo en el extremo se separan.
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

	// Abajo: recorre los cuatro y se queda en el ultimo. No vuelve al primero.
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

	// Arriba: el camino entero desde arriba, y se queda en el primero. Este es el
	// caso que separa el `- 1` del `+ 1`: con el cursor en el primero, un `+ 1`
	// baja a la segunda fila y todo lo de despues va bien.
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

	// Y el recorrido completo hacia arriba desde abajo, que es donde una resta mal
	// puesta se acumula en vez de notarse en un paso.
	m = nueva()
	m.cursor = n - 1
	for want := n - 2; want >= 0; want-- {
		m = press(t, m, "up")
		if m.cursor != want {
			t.Fatalf("vuelta hacia arriba: el cursor = %d, want %d", m.cursor, want)
		}
	}
}

// TestSinFilasNoSePintaNiUnAviso: sin filas no hay dónde pintar nada, y el primer
// corte de commentLines es exactamente eso. Se afirma con el caso más difícil: con
// avisos de acción y sin comentarios, que es la única rama que devuelve contenido
// SIN mirar el presupuesto. Sin el corte, un terminal sin espacio pintaría un
// aviso y la caja se saldría por abajo.
//
// El cero es el caso de borde: `avail == 0` y `avail < 0` tienen que dar lo mismo
// que no pintar, porque las dos son "no hay sitio" y un `>` en vez de `>=` las
// dejaría pasar.
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
	// Y con UNA fila, que es más que cero y menos que un borde, tampoco: la caja
	// no cabe a medias.
	if got := m.commentLines(it, commentChrome-1, inner); len(got) != 0 {
		t.Errorf("con avail=%d devolvió %d líneas, want 0: menos que el marco no cabe la caja",
			commentChrome-1, len(got))
	}
}

// TestElRecuentoNuncaInventaMasDeLosQueSeVen: el total es lo que dice el forge, y
// GitLab no lo expone: ahí el total es lo leído. Cuando el total VENIDO sea menor
// que la lista —porque el forge va paginando y el recuento va por detrás, o
// porque un adapter no lo calculó bien— se corrige hacia arriba, nunca hacia abajo.
//
// Un total menor que la lista daría un "2 of 1" en el borde, que se lee como que
// se está enseñando más de lo que hay.
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
			// Y el borde cuenta de la lista, no del total invertido.
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

// TestLaCajaTomaLoQueNecesitaYNoSePasa: la caja NUNCA se pasa del presupuesto, y
// toma solo lo que necesita. No rellena: un bloque de tres filas en un hueco de
// veinte no estira la conversación para llenar el espacio, porque estirarla es
// inventar altura que la conversación no tiene.
//
// El caso que importa es el EXACTO: si el presupuesto da justo para el marco más
// las filas de los comentarios, la caja lo llena entero. Ese es el que separa un
// `>` bien puesto de un `>=` en la red de seguridad, que se comería la caja por un
// pelo y dejaría al usuario sin comentarios aunque tuvieran sitio de sobra.
func TestLaCajaTomaLoQueNecesitaYNoSePasa(t *testing.T) {
	uno := []model.Comment{conv("alice", "uno")}
	tres := []model.Comment{conv("alice", "uno"), conv("bob", "dos"), conv("carol", "tres")}

	// El presupuesto justo: marco + una fila por comentario. Se llena entero.
	m1 := modelWithComments(t, 45, uno, 1)
	it1 := mustSelected(t, m1)
	if rows := m1.commentLines(it1, commentChrome+1, m1.contentWidth()); len(rows) != commentChrome+1 {
		t.Errorf("presupuesto justo: la caja ocupa %d filas, want %d", len(rows), commentChrome+1)
	}
	// Y con hueco de sobra toma lo que necesita, y no estira.
	for _, avail := range []int{commentChrome + 2, commentChrome + 4, 12, 30} {
		rows := m1.commentLines(it1, avail, m1.contentWidth())
		if len(rows) > avail {
			t.Errorf("con avail=%d la caja ocupa %d filas: se pasa del presupuesto", avail, len(rows))
		}
		if want := commentChrome + 1; len(rows) != want {
			t.Errorf("con avail=%d la caja ocupa %d filas, want %d: no rellena el hueco de más", avail, len(rows), want)
		}
	}

	// Tres comentarios: el presupuesto justo los llena, una fila menos y la caja
	// se cae ENTERA, no a medias. Es el otro lado del mismo borde.
	m3 := modelWithComments(t, 45, tres, 3)
	it3 := mustSelected(t, m3)
	if rows := m3.commentLines(it3, commentChrome+3, m3.contentWidth()); len(rows) != commentChrome+3 {
		t.Errorf("presupuesto justo de 3 comentarios: %d filas, want %d", len(rows), commentChrome+3)
	}
	if rows := m3.commentLines(it3, commentChrome+2, m3.contentWidth()); len(rows) != 0 {
		t.Errorf("con 3 comentarios y una fila de menos la caja se pintó a medias (%d filas)", len(rows))
	}
	// Y nunca se pasa, con hueco de sobra ni sin él.
	for _, avail := range []int{commentChrome + 3, commentChrome + 5, commentChrome + 9, 40} {
		if rows := m3.commentLines(it3, avail, m3.contentWidth()); len(rows) > avail {
			t.Errorf("con avail=%d y 3 comentarios la caja ocupa %d: se pasa", avail, len(rows))
		}
	}
}

// TestElCuerpoSeParteAlAnchoDeLaCajaYNoAlDeLaTerminal: el cuerpo va DENTRO de la
// caja, que además está sangrada. El ancho de texto es el de la caja menos sus dos
// bordes, y no el de la terminal: si se partiera al ancho de la terminal, el texto
// se saldría del marco horizontalmente y el borde derecho quedaría pisado.
func TestElCuerpoSeParteAlAnchoDeLaCajaYNoAlDeLaTerminal(t *testing.T) {
	// Una frase larga sin espacios de corte: obliga a partirse por ANCHO, y el
	// número de filas que sale dice con qué ancho se ha partido.
	frase := strings.Repeat("x", 400)
	for _, inner := range []int{20, 30, 40, 60, 80, 120, 200} {
		m := modelWithComments(t, 45, []model.Comment{conv("alice", frase)}, 1)
		it := mustSelected(t, m)
		rows := m.commentLines(it, 30, inner)
		if len(rows) == 0 {
			t.Errorf("inner %d: no se pintó la caja", inner)
			continue
		}
		// Ninguna fila se pasa del ancho de la caja. Es el invariante: si el
		// ancho de texto fuera el de la terminal, estas filas se pasarían del
		// marco y el borde quedaría pisado.
		for i, l := range rows {
			if w := utf8.RuneCountInString(stripANSI(l)); w > inner {
				t.Errorf("inner %d: la fila %d mide %d columnas, want <= %d: el cuerpo no cabe en su caja",
					inner, i, w, inner)
			}
		}
	}
	// Y el ancho MANDA de verdad: el mismo cuerpo necesita más filas en una caja
	// estrecha que en una ancha. Sin esto, un ancho mal calculado daría el mismo
	// número de filas siempre y la prueba de arriba no miraría nada.
	//
	// El cuerpo son palabras cortas, para que se parta por ANCHO y no se tope con
	// el tope de filas por comentario: un cuerpo de 400 caracteres sin espacios
	// llega al tope con cualquier anchura y no distingue nada.
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
	// Y la diferencia llega aunque el tope de filas por comentario (maxCommentLines)
	// recorte las dos: la caja estrecha se apoya en el tope y la ancha no. Ese
	// tope es justamente lo que hace la prueba poco sensible, así que la
	// afirmación fuerte es "la estrecha necesita MÁS", no "muchas más".
	t.Logf("estrecha %d filas, ancha %d (tope por comentario: %d)", estrechas, anchas, maxCommentLines)
}

// TestLaLeyendaDiceCuantosSeVenDeCuantosHay: el recuento vive en el borde, a la
// derecha, y no en el cuerpo: ahí cada fila es una fila de lo que dijo la gente, y
// una de recuento es una que no es de nadie.
//
// Y el texto tiene una condición para no aparecer: solo si hay más de los que se
// ven Y cabe en el hueco. Con todo visto, "3 of 3" no dice nada que no diga el
// hecho de que están los tres, y no tiene hueco.
func TestLaLeyendaDiceCuantosSeVenDeCuantosHay(t *testing.T) {
	list := make([]model.Comment, 3)
	for i := range list {
		list[i] = conv("alice", "uno")
	}
	// Todo visto: el recuento coincide y no hay nada que anunciarle al usuario.
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

	// Más de los que se ven: el total del forge manda y la leyenda lo dice.
	m2 := modelWithComments(t, 45, list, 12)
	it2 := mustSelected(t, m2)
	plano = stripANSI(strings.Join(m2.commentLines(it2, 30, m2.contentWidth()), "\n"))
	if !strings.Contains(plano, "3 of 12") {
		t.Errorf("con 12 en el total el borde debería decir 3 de 12: %q", plano)
	}
	// Y la pista de que hay más sale cuando hay más: es lo que evita que el
	// usuario recorte el filtro y se pregunte dónde están los otros nueve.
	if !strings.Contains(plano, commentHint) {
		t.Errorf("con 12 en el total debería salir la pista de que hay más: %q", plano)
	}

	// Y con un hueco estrecho la pista se cae, pero el recuento se queda: el
	// número es el dato, la pista es la ayuda.
	m3 := modelWithComments(t, 45, list, 12)
	it3 := mustSelected(t, m3)
	plano = stripANSI(strings.Join(m3.commentLines(it3, 30, 12), "\n"))
	if !strings.Contains(plano, "3 of 12") {
		t.Errorf("con la caja estrecha el recuento no puede caerse: %q", plano)
	}
	// Y la leyenda pura, sin pintar, para la aritmética que decide si la pista
	// cabe.
	// Y la leyenda pura, sin pintar, para la aritmética que decide si la pista
	// cabe. `outer` está calculado para que la leyenda y la pista midan EXACTO lo
	// que hay: en 45 caben justas, en 44 ya no. Ese par es el que separa un `<=`
	// bien puesto de un `<` que perdería la pista justo cuando el hueco da, y del
	// `+ legendGap` que la metería cuando no cabe y la partiría por el borde. Un
	// `outer` de 60 deja el hueco sobrado y no mira nada.
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

// TestElTopeDeCincoSeAplicaYSeDice: se enseñan como mucho cinco comentarios,
// aunque haya veinte. El tope es una decisión de la ficha —una ficha que se salta
// su propio alto porque confió en quien la llenó enseñaría comentarios que no
// caben— y se mide sobre lo que se va a enseñar, no sobre lo que vino.
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
		// Y los que no se enseñan no aparecen: es el tope, no un recorte al azar.
		for i := forge.CommentLimit; i < n; i++ {
			if strings.Contains(plano, "comment-"+itoaSmall(i)) {
				t.Errorf("con %d comentarios se enseñó el %d, que está por encima del tope de %d", n, i, forge.CommentLimit)
			}
		}
	}
}
