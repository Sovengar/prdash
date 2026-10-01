package bordered

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// anchoDe da el ancho visible de una línea, que es lo que de verdad manda: el borde
// es ANSI y cuenta cero.
func anchoDe(t *testing.T, s string) int {
	t.Helper()
	return ansi.StringWidth(s)
}

// lineas parte el resultado en filas, que es la unidad en la que el layout razona.
func lineas(s string) []string { return strings.Split(s, "\n") }

// TestElAnchoExteriorEsElQueSePide: cada fila del resultado mide EXACTAMENTE el ancho que
// se pidió, bordes incluidos.
//
// Y este es el contrato entero del paquete: el layout calcula el alto y el ancho de una
// caja antes de que exista, y si el ancho real no es el pedido el texto se descuadra una
// columna por fila y la caja se ve rota. Así que el ancho es una igualdad, no una cota.
//
// Y va por filas y no por el bloque entero, porque el bloque se mide con la última fila y
// eso perdona un error en las de arriba: una fila que se pasa de ancho y las demás no se
// ven en el total.
func TestElAnchoExteriorEsElQueSePide(t *testing.T) {
	contenidos := []string{
		"",
		"una linea",
		"dos\nlineas",
		"tres\nlineas\ncortas",
		"una linea\n\ny un hueco en medio",
	}
	for _, ancho := range []int{2, 3, 4, 5, 10, 38, 80} {
		for _, contenido := range contenidos {
			got := RenderWithTitle(Rounded(), nil, "T", contenido, ancho)
			for i, l := range lineas(got) {
				if w := anchoDe(t, l); w != ancho {
					t.Errorf("ancho %d, contenido %q, fila %d: mide %d, want %d. "+
						"El ancho pedido es una igualdad, no una cota: el layout ya lo "+
						"ha reservado y una columna de mas descuadra la caja",
						ancho, contenido, i, w, ancho)
				}
			}
		}
	}
}

// TestElAnchoInteriorEsElExteriorMenosDos: el interior es lo que queda entre los bordes,
// y es a ese ancho al que se recorta el contenido.
//
// La cuenta se comprueba por diferencia, que es como la razona el layout: si el borde
// izquierdo y el derecho ocuparan distinto, el interior no sería el ancho pedido menos
// dos y el texto recortado quedaría descentrado.
func TestElAnchoInteriorEsElExteriorMenosDos(t *testing.T) {
	for _, ancho := range []int{2, 3, 4, 8, 40} {
		// Un contenido que no llega al interior va con relleno; se mide lo que ocupa la
		// fila menos los dos caracteres del borde.
		got := RenderWithTitle(Rounded(), nil, "", "x", ancho)
		filas := lineas(got)
		if len(filas) < 3 {
			t.Fatalf("ancho %d: el resultado tiene %d filas, y el test necesita la "+
				"superior, una de contenido y la inferior", ancho, len(filas))
		}
		// La fila de contenido es la del medio: una de borde arriba, el contenido, una
		// de borde abajo.
		contenido := filas[1]
		izq, der := Rounded().Left, Rounded().Right
		interior := strings.TrimSuffix(strings.TrimPrefix(contenido, izq), der)
		want := ancho - 2
		if w := anchoDe(t, interior); w != want {
			t.Errorf("ancho %d: el interior mide %d, want %d (el exterior menos los "+
				"dos bordes)", ancho, w, want)
		}
	}
}

// TestElAnchoMinimoEsDos: por debajo de dos no hay caja, así que el ancho se sube a dos.
//
// El motivo es que los dos bordes tienen que caber: con un ancho de uno no se puede
// dibujar `╭╮` ni `╰╯`, y una caja de ancho cero no es una caja. El suelo es lo que
// convierte "no hay sitio" en "la caja más pequeña que se puede dibujar", que es lo que
// permite al layout no tener que comprobar nada.
//
// Y el borde de esa condición es el ancho DOS, que es donde `width < 2` y `width <= 2`
// dan el mismo resultado: las dos dejan el ancho en dos. Por eso este test afirma el
// valor, no la condición —afirmar la condición sería comprobar el código contra sí mismo.
func TestElAnchoMinimoEsDos(t *testing.T) {
	for _, pedido := range []int{-40, -1, 0, 1} {
		got := RenderWithTitle(Rounded(), nil, "T", "c", pedido)
		for i, l := range lineas(got) {
			if w := anchoDe(t, l); w != 2 {
				t.Errorf("pedido %d, fila %d: mide %d, want 2. Por debajo de dos no "+
					"caban los bordes, y el suelo es lo que evita tener que comprobarlo "+
					"en el layout", pedido, i, w)
			}
		}
	}

	// Y el ancho dos de verdad lleva las tres filas: superior, contenido e inferior.
	if filas := len(lineas(RenderWithTitle(Rounded(), nil, "", "", 2))); filas != 3 {
		t.Errorf("con el ancho mínimo hay %d filas, want 3", filas)
	}
}

// TestElTituloSeRecortaAlInteriorYNoLoDesborda: un título más ancho que el interior se
// recorta, y la fila sigue midiendo el ancho pedido.
//
// Sin recorte, un título largo revientaría la caja: la línea de borde passaría a medir más
// que el resto de filas, y como el layout apila las cajas por número de filas y no por
// ancho, el desborde se vería como una fila que invade la caja de al lado.
//
// Y el borde de la condición es el título que mide EXACTAMENTE el interior, que es donde
// recortar y no recortar dan lo mismo: truncar a N un texto que ya mide N no lo cambia.
func TestElTituloSeRecortaAlInteriorYNoLoDesborda(t *testing.T) {
	titulos := []string{
		"",
		"t",
		"titulo",
		"un titulo bastante mas largo que cualquier interior",
		"unicode: áéíóúñ",
	}
	for _, ancho := range []int{2, 3, 4, 6, 12, 30} {
		for _, titulo := range titulos {
			got := RenderWithTitle(Rounded(), nil, titulo, "c", ancho)
			filas := lineas(got)
			for i, l := range filas {
				if w := anchoDe(t, l); w != ancho {
					t.Errorf("ancho %d, título %q, fila %d: mide %d, want %d. Un título "+
						"que desborda el interior revienta la caja entera", ancho, titulo, i, w, ancho)
				}
			}
			// Y lo que se pinta del título cabe, y no aparece entero si no cabía.
			interior := ancho - 2
			pintado := ansi.StringWidth(stripBorde(filas[0]))
			if pintado > interior {
				t.Errorf("ancho %d, título %q: se pintaron %d caracteres de título en un "+
					"interior de %d", ancho, titulo, pintado, interior)
			}
		}
	}
}

// stripBorde quita el carácter de borde de cada extremo de una línea de borde.
func stripBorde(l string) string {
	b := Rounded()
	return strings.TrimSuffix(strings.TrimPrefix(l, b.TopLeft), b.TopRight)
}

// TestLaAlineacionDelTituloRespetaElAncho: el título se coloca donde dice la alineación
// y el relleno se reparte sin pasarse.
//
// Las tres alineaciones dan la misma fila de ancho, que es lo que se comprueba: la
// diferencia entre ellas es DÓNDE va el título, y el relleno que sobra tiene que ir al
// otro lado. Una alineación que se pasara de filas rompería la caja entera, así que la
// igualdad del ancho es el assert que de verdad importa y el que se hace.
//
// Y los tres casos de reparto que hay que mirar son el exacto —el título ocupa el interior
// entero y no queda relleno— y el impar —donde el reparto no puede ser exacto y alguien
// tiene que|roundar—. En el impar la fila tiene que llevar un carácter más de relleno, y
// ese carácter es el del lado derecho en la alineación a la derecha.
func TestLaAlineacionDelTituloRespetaElAncho(t *testing.T) {
	for _, ancho := range []int{6, 7, 8, 9, 10, 11, 20} {
		interior := ancho - 2
		for _, titulo := range []string{"ab", "abc", "abcd", "abcde"} {
			tw := ansi.StringWidth(titulo)
			if tw > interior {
				continue // aquí el título se recorta y la alineación deja de importar
			}
			for _, al := range []struct {
				nombre string
				align  int
			}{{"izquierda", AlignLeft}, {"centro", AlignCenter}, {"derecha", AlignRight}} {
				got := RenderWithTitles(Rounded(), nil, titulo, al.align, "", AlignLeft, "", ancho)
				fila := lineas(got)[0]
				if w := anchoDe(t, fila); w != ancho {
					t.Errorf("ancho %d, título %q, %s: la fila mide %d, want %d",
						ancho, titulo, al.nombre, w, ancho)
					continue
				}
				// Y el título está donde dice la alineación: se mide cuántas columnas
				// de relleno hay ANTES de él.
				//
				// Y se mide el ANCHO del prefijo y no el índice del título, porque el
				// índice es de BYTES y el relleno `─` son tres cada uno. La primera
				// versión comparaba el índice con una columna y daba posiciones como
				// "empieza en la columna 6" de un interior de cuatro: el número era
				// correcto en bytes y no tenía nada que ver con dónde se ve el título.
				interiorLinea := stripBorde(fila)
				pos := strings.Index(interiorLinea, titulo)
				if pos < 0 {
					t.Errorf("ancho %d, título %q, %s: el título no aparece en la línea %q",
						ancho, titulo, al.nombre, interiorLinea)
					continue
				}
				pos = ansi.StringWidth(interiorLinea[:pos])
				resto := interior - tw
				switch al.align {
				case AlignRight:
					if pos != resto {
						t.Errorf("ancho %d, título %q, derecha: empieza en la columna %d, "+
							"want %d (el relleno entero a la izquierda)", ancho, titulo, pos, resto)
					}
				case AlignCenter:
					if pos != resto/2 {
						t.Errorf("ancho %d, título %q, centro: empieza en la columna %d, "+
							"want %d (la mitad del relleno a la izquierda)", ancho, titulo, pos, resto/2)
					}
				default:
					if pos != 0 {
						t.Errorf("ancho %d, título %q, izquierda: empieza en la columna %d, "+
							"want 0", ancho, titulo, pos)
					}
				}
			}
		}
	}
}

// TestElContenidoSeRecortaYSeRellenaAlInterior: cada línea de contenido se recorta al
// interior y se rellena con espacios hasta él.
//
// Son las dos mitades de lo mismo y las dos hacen falta: si solo se rellena, un contenido
// largo revienta la caja; si solo se recorta, una línea corta deja un hueco por dentro que
// la caja de al lado se ve a través. El relleno es lo que hace que la caja sea opaca.
//
// Y el borde de "recortar" es la línea que mide EXACTAMENTE el interior, donde truncar y
// no truncar dan lo mismo. Y el borde del relleno es el cero de relleno, que es donde el
// `pad > 0` decide: con relleno cero no hay nada que añadir y añadirlo sería añadir un
// espacio de más.
func TestElContenidoSeRecortaYSeRellenaAlInterior(t *testing.T) {
	// Contenido de anchos controlados, uno por debajo del interior, uno exacto y otro
	// por encima.
	for _, ancho := range []int{4, 6, 10, 20} {
		interior := ancho - 2
		for _, largo := range []int{0, 1, interior - 1, interior, interior + 1, interior * 2} {
			if largo < 0 {
				continue
			}
			contenido := strings.Repeat("x", largo)
			got := RenderWithTitle(Rounded(), nil, "", contenido, ancho)
			filas := lineas(got)
			if len(filas) != 3 {
				t.Fatalf("ancho %d, contenido de %d: %d filas, want 3", ancho, largo, len(filas))
			}
			fila := filas[1]
			if w := anchoDe(t, fila); w != ancho {
				t.Errorf("ancho %d, contenido de %d: la fila mide %d, want %d. El "+
					"relleno es lo que hace que la caja sea opaca, y el recorte lo que "+
					"impide que se salga", ancho, largo, w, ancho)
				continue
			}
			// Y lo que queda dentro del borde son exactamente `interior` caracteres, con
			// los de contenido delante y el relleno detrás.
			interiorLinea := fila
			b := Rounded()
			interiorLinea = strings.TrimPrefix(interiorLinea, b.Left)
			interiorLinea = strings.TrimSuffix(interiorLinea, b.Right)
			want := strings.Repeat("x", min(largo, interior)) +
				strings.Repeat(" ", max(0, interior-largo))
			if interiorLinea != want {
				t.Errorf("ancho %d, contenido de %d: el interior es %q, want %q",
					ancho, largo, interiorLinea, want)
			}
		}
	}
}

// TestElContenidoVacioDaUnaFilaEnBlancoInterior: contenido vacío no desaparece, da una
// fila con el interior en blanco.
//
// Es lo que se ve cuando una caja se pinta antes de que tenga nada dentro, y es
// preferible a no pintar la fila: una caja cuya altura cambia según tenga contenido hace
// que el layout, que calcula el alto antes de que exista el contenido, no cuadre.
func TestElContenidoVacioDaUnaFilaEnBlancoInterior(t *testing.T) {
	for _, contenido := range []string{"", "\n", "\n\n"} {
		esperadas := strings.Count(contenido, "\n") + 1
		got := RenderWithTitle(Rounded(), nil, "", contenido, 12)
		filas := lineas(got)
		if len(filas) != esperadas+2 {
			t.Errorf("contenido %q: %d filas, want %d (las de contenido más los dos "+
				"bordes)", contenido, len(filas), esperadas+2)
		}
		for i, l := range filas {
			if anchoDe(t, l) != 12 {
				t.Errorf("contenido %q, fila %d: mide %d, want 12", contenido, i, anchoDe(t, l))
			}
		}
		// Y las de contenido están en blanco por dentro: sin huecos de más.
		for i := 1; i < len(filas)-1; i++ {
			interior := strings.TrimSuffix(strings.TrimPrefix(filas[i], Rounded().Left), Rounded().Right)
			if strings.TrimSpace(interior) != "" {
				t.Errorf("contenido %q, fila %d: el interior es %q y tiene algo dentro",
					contenido, i, interior)
			}
		}
	}
}

// TestUnBordeSinCaracteresDeRellenoUsaEspacios: un borde con el relleno vacío pinta
// espacios, porque un `Repeat` de cero no pintaría nada.
//
// Es el caso de un borde degenerado —el de un borde sin línea de relleno—, y el suelo a
// espacio es lo que hace que la fila siga midiendo el ancho pedido en vez de encogerse.
func TestUnBordeSinCaracteresDeRellenoUsaEspacios(t *testing.T) {
	b := lipgloss.Border{
		TopLeft: "+", Top: "", TopRight: "+",
		Left: "|", BottomLeft: "+", Bottom: "", BottomRight: "+", Right: "|",
	}
	got := RenderWithTitle(b, nil, "t", "c", 10)
	for i, l := range lineas(got) {
		if w := anchoDe(t, l); w != 10 {
			t.Errorf("fila %d con un borde sin relleno mide %d, want 10: el relleno "+
				"vacío tiene que caer a espacio", i, w)
		}
	}
}

// TestElRellenoNoPuedePedirEspaciosDeMenos: `strings.Repeat` con un número NEGATIVO es un
// PANIC, y la guarda que lo impedía se quitó al recortar sin condición.
//
// El recorte es lo que garantiza el suelo: `ansi.Truncate(line, w, "")` devuelve siempre
// un texto de ancho visible ≤ w, así que `w - StringWidth(line)` nunca baja de cero. Eso
// es una garantía de la librería, no del código, y una garantía que viene de fuera se
// comprueba: este test barre contenido con códigos ANSI —que es donde truncar y medir
// pueden desincronizarse— y afirma dos cosas a la vez, el ancho exacto de cada fila y
// que nada revienta.
//
// Y el caso que de verdad lo haría reventar es el contenido vacío de texto con ANSI: una
// secuencia de escape sin ningún carácter visible mide cero, se trunca a cero y el relleno
// que se pide es el interior entero. Si el truncado devolviera algo de ancho negativo o
// mayor que el interior, aquí se vería.
func TestElRellenoNoPuedePedirEspaciosDeMenos(t *testing.T) {
	contenidos := []string{
		"",
		"\x1b[31m\x1b[0m",           // escapes sin texto visible
		"\x1b[31m\x1b[0m\x1b[32m",   // escapes encadenados
		"\x1b[31mtexto\x1b[0m",      // escapes normales
		"\x1b[31m\x1b[0m\n\x1b[32m", // dos filas, la segunda solo con escapes
		"\x1b[31m" + strings.Repeat("x", 100) + "\x1b[0m",
		"\x1b[1m\x1b[4m\x1b[31mábc\x1b[0m",
		"áéíóú" + "\x1b[0m",
	}
	for _, ancho := range []int{2, 3, 4, 5, 8, 20, 40} {
		for _, contenido := range contenidos {
			var (
				got string
				pan bool
			)
			// Se atrapa el panic para poder decir EN QUÉ caso fue, en vez de que el test
			// se muera con una traza que no dice cuál de los hundreds era.
			func() {
				defer func() {
					if r := recover(); r != nil {
						pan = true
					}
				}()
				got = RenderWithTitle(Rounded(), nil, "t", contenido, ancho)
			}()
			if pan {
				t.Errorf("ancho %d, contenido %q: el relleno pidió espacios de menos y "+
					"reventó. El recorte es lo que garantiza que no pase, y eso hay "+
					"que comprobarlo y no suponerlo", ancho, contenido)
				continue
			}
			for i, l := range lineas(got) {
				if w := anchoDe(t, l); w != ancho {
					t.Errorf("ancho %d, contenido %q, fila %d: mide %d, want %d",
						ancho, contenido, i, w, ancho)
				}
			}
		}
	}
}
