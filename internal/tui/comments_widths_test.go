package tui

import (
	"strings"
	"testing"
	"unicode/utf8"

	"prdash/internal/forge/model"
)

// TestCommentWidths: los dos anchos de texto de un comentario.
//
// La primera fila lleva el autor delante, así que su cuerpo dispone de MENOS ancho
// que las siguientes. Ese es el invariante, y no se ve en el render: con un autor
// largo la primera fila se parte antes, y como la caja trunca igual, la diferencia
// se lee como "faltan caracteres" y no como "el ancho está mal".
//
// Por eso esto es una función aparte. Antes vivía dentro del bucle de pintado, y
// desde ahí no se podía comprobar: el marco tapaba el error.
func TestCommentWidths(t *testing.T) {
	for _, inner := range []int{0, 1, 5, 10, 20, 30, 40, 60, 80, 120, 200} {
		for _, author := range []string{"", "a", "alice", "someone-with-a-long-name"} {
			first, cont := commentWidths(inner, author)

			// La primera fila SIEMPRE va al menos tan estrecha como las demás: el
			// autor solo puede quitar sitio, nunca dar.
			if first > cont {
				t.Errorf("inner=%d author=%q: la primera fila (%d) es más ANCHA que las siguientes (%d)",
					inner, author, first, cont)
			}
			// Y el suelo de 8: por debajo, el texto no se lee. Con la caja más
			// estrecha que el nombre del autor, el cuerpo se quedaría sin nada.
			if first < 8 || cont < 8 {
				t.Errorf("inner=%d author=%q: (%d, %d), want >= 8 en los dos: por debajo no hay lectura",
					inner, author, first, cont)
			}
			// Y la aritmética exacta, que es lo que hace que el corte caiga donde
			// tiene que caer y no una palabra más allá.
			wantFirst := max(8, inner-utf8.RuneCountInString(commentIndent+author+commentSep))
			wantCont := max(8, inner-utf8.RuneCountInString(contIndent))
			if first != wantFirst || cont != wantCont {
				t.Errorf("inner=%d author=%q: dio (%d, %d), want (%d, %d)",
					inner, author, first, cont, wantFirst, wantCont)
			}
			// La cuenta es en RUNES: un autor con acentos o emoji ocupa una
			// columna por carácter, no dos.
			if author == "ñ" {
				if first != cont {
					t.Errorf("un autor de un rune ocupa una columna, pero la primera fila (%d) difiere de las siguientes (%d)",
						first, cont)
				}
			}
		}
	}
	// Y dos autores del MISMO número de caracteres dan el mismo ancho: lo que
	// cuenta es la longitud, no el nombre.
	aF, _ := commentWidths(60, "alice")
	bF, _ := commentWidths(60, "bobby")
	if aF != bF {
		t.Errorf("autores de la misma longitud dieron anchos distintos: %d vs %d", aF, bF)
	}
	// Y un autor más largo da la primera fila más ESTRECHA: el nombre se come
	// sitio del cuerpo, y el cuerpo es lo único que dice algo.
	cortoF, _ := commentWidths(60, "a")
	largoF, _ := commentWidths(60, strings.Repeat("x", 40))
	if largoF >= cortoF {
		t.Errorf("un autor de 40 caracteres dio la primera fila de %d y uno de 1 la dio de %d: el nombre debería empujar",
			largoF, cortoF)
	}
}

// TestCommentBodyWidthDescuentaSangrioYBordes: el ancho de texto es más estrecho
// que el de la caja por el sangrado de los dos lados más los dos bordes
// verticales, y tiene suelo.
//
// El suelo importa porque sin él el ancho se vuelve negativo en una caja estrecha y
// `truncate` con un ancho negativo hace un desastre. Con 8, una caja estrecha
// muestra un trozo y se ve que está cortada.
func TestCommentBodyWidthDescuentaSangrioYBordes(t *testing.T) {
	for outer := -20; outer <= 200; outer++ {
		got := commentBodyWidth(outer)
		want := max(8, commentBoxWidth(outer)-commentBoxBorder)
		if got != want {
			t.Errorf("commentBodyWidth(%d) = %d, want %d", outer, got, want)
		}
		if got < 8 {
			t.Errorf("commentBodyWidth(%d) = %d, want >= 8", outer, got)
		}
	}
	// Por encima del suelo, el descuento es exacto: caja menos sangrado menos
	// bordes. Una columna de más y el texto pisa el borde; una de menos y sobra
	// hueco a la derecha.
	for outer := 40; outer <= 120; outer++ {
		want := outer - 2*commentInset - commentBoxBorder
		if got := commentBodyWidth(outer); got != want {
			t.Errorf("commentBodyWidth(%d) = %d, want %d", outer, got, want)
		}
	}
	// Y el suelo cubre los anchos MUY pequeños: con 10 de caja el texto son 8,
	// porque 10 menos 2 de sangrado menos 2 de bordes daría 6.
	if got := commentBodyWidth(10); got != 8 {
		t.Errorf("commentBodyWidth(10) = %d, want 8 (el suelo)", got)
	}
	if got := commentBodyWidth(20); got != 20-2*commentInset-commentBoxBorder {
		t.Errorf("commentBodyWidth(20) = %d, want el descuento exacto: a 20 todavía no llega el suelo", got)
	}
}

// TestCommentRowReservaElHuecoDelEllipsis: cuando queda texto detrás, la fila
// lleva "…" y el hueco de esa marca se DESCUENTA del ancho del texto. Si no se
// descontara, la fila se iría una columna más allá del marco y el "…" caería
// fuera.
//
// El borde es donde se separa un `>` de un `>=`: un texto que mide justo lo que
// queda DESPUÉS de reservar la marca entra entero con la marca al lado, y uno que
// mide una columna más ya no cabe y se recorta.
//
// Y sin corte no se reserva nada: un texto que cabe entero se devuelve entero, sin
// marca. Poner la marca ahí sería decir "se perdió algo" de algo que no se perdió.
func TestCommentRowReservaElHuecoDelEllipsis(t *testing.T) {
	const w = 20
	// Con corte, un texto de exactamente (w-1) caracteres: cabe JUSTO con la
	// marca. Este es el caso que separa el `<=` del `<`.
	justo := strings.Repeat("x", w-1)
	got := stripANSI(commentRow(0, "alice", ": ", justo, w, true))
	if !strings.HasSuffix(got, justo+"…") {
		t.Errorf("un texto de %d columnas con marca debería entrar entero más la marca, dio %q", w-1, got)
	}
	if ancho := utf8.RuneCountInString(got) - utf8.RuneCountInString(commentIndent) -
		utf8.RuneCountInString("alice: "); ancho > w {
		t.Errorf("la fila mide %d columnas de texto, más que el ancho %d: la marca se salió de la caja", ancho, w)
	}
	// Una columna más: ya no cabe con la marca, así que se recorta a (w-1) con la
	// marca dentro.
	unoMas := strings.Repeat("x", w)
	got = stripANSI(commentRow(0, "alice", ": ", unoMas, w, true))
	if utf8.RuneCountInString(got) > utf8.RuneCountInString(commentIndent)+utf8.RuneCountInString("alice: ")+w {
		t.Errorf("un texto de %d columnas se pasó del ancho %d: %q", w, w, got)
	}
	if !strings.HasSuffix(got, "…") {
		t.Errorf("un texto recortado debería llevar la marca, dio %q", got)
	}
	// Y que el recorte SATURA: un texto de w columnas y otro de w+5 dan la misma
	// fila, porque los dos se cortan al hueco de la marca. Es lo que hace que el
	// texto se lea siempre como "esto y más", y no como un fragmento de longitud
	// arbitraria.
	dosMas := strings.Repeat("x", w+5)
	if a, b := stripANSI(commentRow(0, "alice", ": ", unoMas, w, true)),
		stripANSI(commentRow(0, "alice", ": ", dosMas, w, true)); a != b {
		t.Errorf("un texto de %d y otro de %d dieron filas distintas %q y %q: los dos se cortan al hueco de la marca",
			w, w+5, a, b)
	}

	// Sin corte: un texto que cabe entra entero y SIN marca. Y uno que no cabe
	// (una palabra suelta más ancha que la caja) se recorta a w, sin marca.
	sinCorte := strings.Repeat("x", w-3)
	if got := stripANSI(commentRow(0, "alice", ": ", sinCorte, w, false)); strings.Contains(got, "…") {
		t.Errorf("un texto que cabe entero no debería llevar marca: %q", got)
	}
	// El BORDE: un texto de EXACTAMENTE w columnas, sin corte, entra entero y sin
	// marca. Con un `>=` en vez de `>` searía a la rama de recorte y saldría con
	// "…" un texto que cabía justo, que es decir que se perdió algo cuando no se
	// perdió nada.
	justoSinCorte := strings.Repeat("x", w)
	got = stripANSI(commentRow(0, "alice", ": ", justoSinCorte, w, false))
	if strings.Contains(got, "…") {
		t.Errorf("un texto de exactamente %d columnas sin corte debería entrar entero, dio %q", w, got)
	}
	if n := utf8.RuneCountInString(got) - utf8.RuneCountInString(commentIndent) -
		utf8.RuneCountInString("alice: "); n != w {
		t.Errorf("un texto de %d columnas dio una fila de %d: la fila exacta no debe cambiar de largo", w, n)
	}
	// Y una columna más: ya no cabe y sí se recorta.
	got = stripANSI(commentRow(0, "alice", ": ", strings.Repeat("x", w+1), w, false))
	if !strings.Contains(got, "…") {
		t.Errorf("un texto de %d columnas en una fila de %d debería recortarse, dio %q", w+1, w, got)
	}

	ancho := strings.Repeat("x", w+10)
	got = stripANSI(commentRow(0, "alice", ": ", ancho, w, false))
	if !strings.Contains(got, "…") {
		t.Errorf("una palabra demasiado ancha debería recortarse marcando, dio %q", got)
	}
	if utf8.RuneCountInString(got) > utf8.RuneCountInString(commentIndent)+utf8.RuneCountInString("alice: ")+w {
		t.Errorf("la palabra recortada se pasó del ancho: %q", got)
	}

	// La sangría: la primera fila lleva el autor y las siguientes solo el sangrado
	// de continuación. Es lo que hace que el cuerpo se lea como un bloque.
	primera := stripANSI(commentRow(0, "alice", ": ", "texto", w, false))
	siguiente := stripANSI(commentRow(1, "alice", ": ", "texto", w, false))
	if !strings.HasPrefix(primera, commentIndent+"alice: ") {
		t.Errorf("la primera fila debería llevar el autor delante, dio %q", primera)
	}
	if strings.Contains(siguiente, "alice") {
		t.Errorf("la segunda fila no debería llevar el autor: %q", siguiente)
	}
	// Y la continuación lleva un sangrado FIJO, el mismo con cualquier autor. Eso
	// es lo que hace que el cuerpo de un comentario se lea como un bloque y no
	// como trozos sueltos.
	//
	// OJO: el sangrado de continuación NO se alinea con el texto de la primera
	// fila, que empieza en 2 + nombre + 2. El comentario de commentRow dice que
	// "se alinean", y no es lo que hace el código: el de continuación es
	// constante y el de la primera depende de lo largo que sea el nombre. Aquí se
	// afirma lo que el código hace, que es lo comprobable.
	if !strings.HasPrefix(siguiente, contIndent) {
		t.Errorf("la segunda fila debería ir con el sangrado de continuación, dio %q", siguiente)
	}
	for _, autor := range []string{"a", "alice", "someone-with-a-long-name"} {
		primera := stripANSI(commentRow(0, autor, ": ", "texto", w, false))
		siguiente := stripANSI(commentRow(1, autor, ": ", "texto", w, false))
		colTexto := func(s string) int {
			return utf8.RuneCountInString(s[:strings.Index(s, "texto")])
		}
		want := utf8.RuneCountInString(commentIndent + autor + commentSep)
		if colTexto(primera) != want {
			t.Errorf("con el autor %q el texto de la primera fila empieza en la columna %d, want %d",
				autor, colTexto(primera), want)
		}
		if colTexto(siguiente) != utf8.RuneCountInString(contIndent) {
			t.Errorf("con el autor %q la continuación empieza en la columna %d, want %d (fija, no sigue al autor)",
				autor, colTexto(siguiente), utf8.RuneCountInString(contIndent))
		}
	}
}

// TestElCorteMarcaLaUltimaFilaYNoAnadeNinguna: cuando el comentario no cabe en las
// filas que le tocan, la última fila se VUELVE A COMPONER marcando el corte. No se
// añade una fila nueva, y no se recorta la primera.
//
// Es el contrato de `lines`: da exactamente las filas que se pidieron, ni una más,
// con la última marcada. El borde que importa es un comentario que produce un
// trozo MÁS de los que caben: ahí el corte tiene que caer en la fila pedida, y un
// `>` en vez de `>=` lo dejaría caer una fila más abajo.
//
// El número de trozos NO se escribe a mano: sale de `wrapText`, que es la misma
// función pura con la que compone el código. Si un día esa cambia, el test avisa
// de que el número esperado cambió, en vez de quedarse verde con una cuenta vieja.
func TestElCorteMarcaLaUltimaFilaYNoAnadeNinguna(t *testing.T) {
	const inner = 40
	first, _ := commentWidths(inner, "alice")

	// Cada palabra ocupa una fila entera, así que el número de trozos es el número
	// de palabras y el presupuesto se puede pedir sobre un corte exacto.
	palabra := strings.Repeat("x", first)
	for _, trozos := range []int{1, 2, 3, 4, 5, 6} {
		// Un cuerpo cuyo primer párrafo da EXACTAMENTE `trozos` líneas.
		cuerpo := palabra
		for i := 1; i < trozos; i++ {
			cuerpo += " " + palabra
		}
		c := model.Comment{Author: "alice", Body: cuerpo}

		wantTrozos := len(wrapText(cuerpo, first))
		if wantTrozos != trozos {
			t.Fatalf("el cuerpo de %d palabras da %d trozos, no %d: el ancho cambió y el "+
				"test ya no mide lo que cree medir", trozos, wantTrozos, trozos)
		}

		// Con presupuesto de sobra: sale entero, sin marcas.
		todas := commentBody(c, trozos+3, inner)
		if len(todas) != trozos {
			t.Errorf("con %d trozos y presupuesto de sobrta salieron %d filas, want %d", trozos, len(todas), trozos)
		}
		for i, l := range todas {
			if strings.Contains(stripANSI(l), "…") {
				t.Errorf("con presupuesto de sobra la fila %d lleva marca de corte: %q", i, stripANSI(l))
			}
		}

		// Con presupuesto EXACTO: sale entero y sin corte. Al valer justo, no se
		// perdió nada, y marcar sería mentir.
		exactas := commentBody(c, trozos, inner)
		if len(exactas) != trozos {
			t.Errorf("con %d trozos y presupuesto exacto salieron %d filas, want %d", trozos, len(exactas), trozos)
		}
		for i, l := range exactas {
			if strings.Contains(stripANSI(l), "…") {
				t.Errorf("con presupuesto EXACTO la fila %d lleva marca: no se perdió nada, dio %q", i, stripANSI(l))
			}
		}

		// Con un trozo menos: sale con una fila menos, la última marcada, y solo
		// la última.
		//
		// El suelo de una fila: con presupuesto cero o negativo sale UNA fila, no
		// cero. Nadie se queda sin fila por un descuadre de reparto, y sin ella el
		// `out[len(out)-1]` del corte reventaría.
		pide := max(1, trozos-1)
		corta := commentBody(c, pide, inner)
		if len(corta) != pide {
			t.Errorf("con %d trozos y presupuesto de %d salieron %d filas, want %d: ni una más de las pedidas",
				trozos, pide, len(corta), pide)
		}
		if len(corta) == 0 {
			continue
		}
		// La marca solo se espera si de verdad se perdió algo: con un solo trozo y
		// presupuesto de una fila, la fila lo tiene todo y no hay "después" que
		// señalar. Marcarlo ahí sería mentir sobre un comentario completo.
		if pide < trozos {
			if ultima := stripANSI(corta[len(corta)-1]); !strings.Contains(ultima, "…") {
				t.Errorf("con %d trozos y presupuesto de %d la última fila no lleva marca: %q",
					trozos, pide, ultima)
			}
		}
		for i, l := range corta[:len(corta)-1] {
			if strings.Contains(stripANSI(l), "…") {
				t.Errorf("con presupuesto corto la fila %d lleva marca: solo la última puede cortarse", i)
			}
		}
		// Y el texto que se perdió se marca sin pasarse del ancho de la fila: la
		// marca seMétió dentro, no encima del marco.
		for i, l := range corta {
			if ancho := utf8.RuneCountInString(stripANSI(l)) -
				utf8.RuneCountInString(commentIndent) - utf8.RuneCountInString("alice: "); ancho > first && i == 0 {
				t.Errorf("la primera fila mide %d columnas de texto y el ancho es %d", ancho, first)
			}
		}
	}

	// Un segundo párrafo: el corte tiene que caer en el último trozo escrito, no
	// en el primero del párrafo, y el total de filas sigue siendo el pedido.
	cuerpo := palabra + "\n\n" + palabra + " " + palabra + " " + palabra
	c := model.Comment{Author: "alice", Body: cuerpo}
	primerParrafo := len(wrapText(palabra, first))
	segundo := len(wrapText(strings.TrimSpace(strings.TrimPrefix(strings.SplitN(cuerpo, "\n\n", 2)[1], " ")), first))
	conDos := commentBody(c, primerParrafo+segundo, inner)
	if len(conDos) != primerParrafo+segundo {
		t.Errorf("con dos párrafos (%d+%d) salieron %d filas, want %d",
			primerParrafo, segundo, len(conDos), primerParrafo+segundo)
	}

	// Y el cuerpo vacío: una fila que lo dice, en vez de una lista vacía que
	// parecería un comentario que no se ha pintado.
	for _, vacio := range []string{"", "   ", "\n\n"} {
		got := commentBody(model.Comment{Author: "alice", Body: vacio}, 5, inner)
		if len(got) != 1 {
			t.Errorf("un cuerpo vacío dio %d filas, want 1 (la que lo dice)", len(got))
		} else if !strings.Contains(stripANSI(got[0]), "no text") {
			t.Errorf("un cuerpo vacío dio %q, want la fila que lo dice", stripANSI(got[0]))
		}
	}

	// Y con cero filas de presupuesto sale una, no cero: nadie se queda sin fila
	// por un descuadre de reparto, y sin ella el `out[len(out)-1]` del corte
	// reventaría.
	for _, n := range []int{-5, 0, 1} {
		if got := commentBody(model.Comment{Author: "alice", Body: "hola"}, n, inner); len(got) == 0 {
			t.Errorf("con presupuesto %d no salió ninguna fila: una fila vacía es mejor que un índice fuera de rango", n)
		}
	}
}
