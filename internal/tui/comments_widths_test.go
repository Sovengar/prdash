package tui

import (
	"strings"
	"testing"
	"unicode/utf8"

	"prdash/internal/forge/model"
)

// The first row carries the author and the rest only the continuation indent.
func TestCommentWidths(t *testing.T) {
	for _, inner := range []int{0, 1, 5, 10, 20, 30, 40, 60, 80, 120, 200} {
		for _, author := range []string{"", "a", "alice", "someone-with-a-long-name"} {
			first, cont := commentWidths(inner, author)

			if first > cont {
				t.Errorf("inner=%d author=%q: la primera fila (%d) es más ANCHA que las siguientes (%d)",
					inner, author, first, cont)
			}
			// The floor of 8: below it the text is unreadable.
			if first < 8 || cont < 8 {
				t.Errorf("inner=%d author=%q: (%d, %d), want >= 8 en los dos: por debajo no hay lectura",
					inner, author, first, cont)
			}
			wantFirst := max(8, inner-utf8.RuneCountInString(commentIndent+author+commentSep))
			wantCont := max(8, inner-utf8.RuneCountInString(contIndent))
			if first != wantFirst || cont != wantCont {
				t.Errorf("inner=%d author=%q: dio (%d, %d), want (%d, %d)",
					inner, author, first, cont, wantFirst, wantCont)
			}
			// The count is in RUNES: an author with accents or emoji takes one column per character.
			if author == "ñ" {
				if first != cont {
					t.Errorf("un autor de un rune ocupa una columna, pero la primera fila (%d) difiere de las siguientes (%d)",
						first, cont)
				}
			}
		}
	}
	// Two authors of the SAME length give the same width: what counts is the length, not the text.
	aF, _ := commentWidths(60, "alice")
	bF, _ := commentWidths(60, "bobby")
	if aF != bF {
		t.Errorf("autores de la misma longitud dieron anchos distintos: %d vs %d", aF, bF)
	}
	// A longer author makes the first row NARROWER: the name eats the body's space.
	cortoF, _ := commentWidths(60, "a")
	largoF, _ := commentWidths(60, strings.Repeat("x", 40))
	if largoF >= cortoF {
		t.Errorf("un autor de 40 caracteres dio la primera fila de %d y uno de 1 la dio de %d: el nombre debería empujar",
			largoF, cortoF)
	}
}

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
	for outer := 40; outer <= 120; outer++ {
		want := outer - 2*commentInset - commentBoxBorder
		if got := commentBodyWidth(outer); got != want {
			t.Errorf("commentBodyWidth(%d) = %d, want %d", outer, got, want)
		}
	}
	if got := commentBodyWidth(10); got != 8 {
		t.Errorf("commentBodyWidth(10) = %d, want 8 (el suelo)", got)
	}
	if got := commentBodyWidth(20); got != 20-2*commentInset-commentBoxBorder {
		t.Errorf("commentBodyWidth(20) = %d, want el descuento exacto: a 20 todavía no llega el suelo", got)
	}
}

func TestCommentRowReservaElHuecoDelEllipsis(t *testing.T) {
	const w = 20
	// With clipping, a text of exactly (w-1) characters fits JUST with the mark.
	justo := strings.Repeat("x", w-1)
	got := stripANSI(commentRow(0, "alice", ": ", justo, w, true))
	if !strings.HasSuffix(got, justo+"…") {
		t.Errorf("un texto de %d columnas con marca debería entrar entero más la marca, dio %q", w-1, got)
	}
	if ancho := utf8.RuneCountInString(got) - utf8.RuneCountInString(commentIndent) -
		utf8.RuneCountInString("alice: "); ancho > w {
		t.Errorf("la fila mide %d columnas de texto, más que el ancho %d: la marca se salió de la caja", ancho, w)
	}
	unoMas := strings.Repeat("x", w)
	got = stripANSI(commentRow(0, "alice", ": ", unoMas, w, true))
	if utf8.RuneCountInString(got) > utf8.RuneCountInString(commentIndent)+utf8.RuneCountInString("alice: ")+w {
		t.Errorf("un texto de %d columnas se pasó del ancho %d: %q", w, w, got)
	}
	if !strings.HasSuffix(got, "…") {
		t.Errorf("un texto recortado debería llevar la marca, dio %q", got)
	}
	// The clipping SATURATES: w columns and w+5 give the same row.
	dosMas := strings.Repeat("x", w+5)
	if a, b := stripANSI(commentRow(0, "alice", ": ", unoMas, w, true)),
		stripANSI(commentRow(0, "alice", ": ", dosMas, w, true)); a != b {
		t.Errorf("un texto de %d y otro de %d dieron filas distintas %q y %q: los dos se cortan al hueco de la marca",
			w, w+5, a, b)
	}

	// Without clipping a text that fits comes whole and WITHOUT a mark.
	sinCorte := strings.Repeat("x", w-3)
	if got := stripANSI(commentRow(0, "alice", ": ", sinCorte, w, false)); strings.Contains(got, "…") {
		t.Errorf("un texto que cabe entero no debería llevar marca: %q", got)
	}
	// The EDGE: a text of EXACTLY w columns goes in whole with no mark. A `>=` would add one.
	justoSinCorte := strings.Repeat("x", w)
	got = stripANSI(commentRow(0, "alice", ": ", justoSinCorte, w, false))
	if strings.Contains(got, "…") {
		t.Errorf("un texto de exactamente %d columnas sin corte debería entrar entero, dio %q", w, got)
	}
	if n := utf8.RuneCountInString(got) - utf8.RuneCountInString(commentIndent) -
		utf8.RuneCountInString("alice: "); n != w {
		t.Errorf("un texto de %d columnas dio una fila de %d: la fila exacta no debe cambiar de largo", w, n)
	}
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

	primera := stripANSI(commentRow(0, "alice", ": ", "texto", w, false))
	siguiente := stripANSI(commentRow(1, "alice", ": ", "texto", w, false))
	if !strings.HasPrefix(primera, commentIndent+"alice: ") {
		t.Errorf("la primera fila debería llevar el autor delante, dio %q", primera)
	}
	if strings.Contains(siguiente, "alice") {
		t.Errorf("la segunda fila no debería llevar el autor: %q", siguiente)
	}
	// The continuation carries a FIXED indent whatever the author, which is what makes the body read
	//as a block.
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

// When the comment does not fit in the rows it gets.
func TestElCorteMarcaLaUltimaFilaYNoAnadeNinguna(t *testing.T) {
	const inner = 40
	first, _ := commentWidths(inner, "alice")

	palabra := strings.Repeat("x", first)
	for _, trozos := range []int{1, 2, 3, 4, 5, 6} {
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

		todas := commentBody(c, trozos+3, inner)
		if len(todas) != trozos {
			t.Errorf("con %d trozos y presupuesto de sobrta salieron %d filas, want %d", trozos, len(todas), trozos)
		}
		for i, l := range todas {
			if strings.Contains(stripANSI(l), "…") {
				t.Errorf("con presupuesto de sobra la fila %d lleva marca de corte: %q", i, stripANSI(l))
			}
		}

		exactas := commentBody(c, trozos, inner)
		if len(exactas) != trozos {
			t.Errorf("con %d trozos y presupuesto exacto salieron %d filas, want %d", trozos, len(exactas), trozos)
		}
		for i, l := range exactas {
			if strings.Contains(stripANSI(l), "…") {
				t.Errorf("con presupuesto EXACTO la fila %d lleva marca: no se perdió nada, dio %q", i, stripANSI(l))
			}
		}

		// One piece less: one row less, the last one marked, and only the last.
		pide := max(1, trozos-1)
		corta := commentBody(c, pide, inner)
		if len(corta) != pide {
			t.Errorf("con %d trozos y presupuesto de %d salieron %d filas, want %d: ni una más de las pedidas",
				trozos, pide, len(corta), pide)
		}
		if len(corta) == 0 {
			continue
		}
		// The mark only appears when something was really lost: with one chunk and enough budget there
		// is nothing to say.
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
		for i, l := range corta {
			if ancho := utf8.RuneCountInString(stripANSI(l)) -
				utf8.RuneCountInString(commentIndent) - utf8.RuneCountInString("alice: "); ancho > first && i == 0 {
				t.Errorf("la primera fila mide %d columnas de texto y el ancho es %d", ancho, first)
			}
		}
	}

	// A second paragraph: the cut has to land on the last chunk written.
	cuerpo := palabra + "\n\n" + palabra + " " + palabra + " " + palabra
	c := model.Comment{Author: "alice", Body: cuerpo}
	primerParrafo := len(wrapText(palabra, first))
	segundo := len(wrapText(strings.TrimSpace(strings.TrimPrefix(strings.SplitN(cuerpo, "\n\n", 2)[1], " ")), first))
	conDos := commentBody(c, primerParrafo+segundo, inner)
	if len(conDos) != primerParrafo+segundo {
		t.Errorf("con dos párrafos (%d+%d) salieron %d filas, want %d",
			primerParrafo, segundo, len(conDos), primerParrafo+segundo)
	}

	// And the empty body: one row saying so, instead of an empty list that would look broken.
	for _, vacio := range []string{"", "   ", "\n\n"} {
		got := commentBody(model.Comment{Author: "alice", Body: vacio}, 5, inner)
		if len(got) != 1 {
			t.Errorf("un cuerpo vacío dio %d filas, want 1 (la que lo dice)", len(got))
		} else if !strings.Contains(stripANSI(got[0]), "no text") {
			t.Errorf("un cuerpo vacío dio %q, want la fila que lo dice", stripANSI(got[0]))
		}
	}

	// With zero rows of budget there is one, not zero: nobody loses their row to a rounding.
	for _, n := range []int{-5, 0, 1} {
		if got := commentBody(model.Comment{Author: "alice", Body: "hola"}, n, inner); len(got) == 0 {
			t.Errorf("con presupuesto %d no salió ninguna fila: una fila vacía es mejor que un índice fuera de rango", n)
		}
	}
}
