package bordered

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

func anchoDe(t *testing.T, s string) int {
	t.Helper()
	return ansi.StringWidth(s)
}

func lineas(s string) []string { return strings.Split(s, "\n") }

// Every row of the result measures EXACTLY the requested width.
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

func TestElAnchoInteriorEsElExteriorMenosDos(t *testing.T) {
	for _, ancho := range []int{2, 3, 4, 8, 40} {
		got := RenderWithTitle(Rounded(), nil, "", "x", ancho)
		filas := lineas(got)
		if len(filas) < 3 {
			t.Fatalf("ancho %d: el resultado tiene %d filas, y el test necesita la "+
				"superior, una de contenido y la inferior", ancho, len(filas))
		}
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

// Below two there is no box, so the width goes up to two.
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

	if filas := len(lineas(RenderWithTitle(Rounded(), nil, "", "", 2))); filas != 3 {
		t.Errorf("con el ancho mínimo hay %d filas, want 3", filas)
	}
}

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
			interior := ancho - 2
			pintado := ansi.StringWidth(stripBorde(filas[0]))
			if pintado > interior {
				t.Errorf("ancho %d, título %q: se pintaron %d caracteres de título en un "+
					"interior de %d", ancho, titulo, pintado, interior)
			}
		}
	}
}

func stripBorde(l string) string {
	b := Rounded()
	return strings.TrimSuffix(strings.TrimPrefix(l, b.TopLeft), b.TopRight)
}

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
				// The title's position is measured by counting the padding columns BEFORE it.
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

func TestElContenidoSeRecortaYSeRellenaAlInterior(t *testing.T) {
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

// Empty content does not disappear, it gives a row with the borders.
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
		for i := 1; i < len(filas)-1; i++ {
			interior := strings.TrimSuffix(strings.TrimPrefix(filas[i], Rounded().Left), Rounded().Right)
			if strings.TrimSpace(interior) != "" {
				t.Errorf("contenido %q, fila %d: el interior es %q y tiene algo dentro",
					contenido, i, interior)
			}
		}
	}
}

// A border with an empty fill paints spaces.
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

// strings.Repeat with a NEGATIVE count is a PANIC, and the clip above guarantees the difference
// is never negative, which is what replaced the guard.
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
			// The panic is caught to say WHICH case caused it, instead of the test dying.
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
