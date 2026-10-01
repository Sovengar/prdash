package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

// TestWrapTextNoTocaLoQueYaCabe: si el texto entra entero, sale ENTERO, con sus
// espacios tal cual.
//
// Esto no es un detalle: el envoltorio normaliza los espacios al partir por
// palabras (usa strings.Fields, que colapsa cualquier racha). Si el texto cabe, no
// hay por qué pasar por ahí, y un texto que cabe no debería cambiar ni en un
// espacio.
//
// Ese es justo el borde que separa el `<=` del `<`: con el texto midiendo
// exactamente el ancho, uno dice "no hace falta partir" y el otro se mete al
// envoltorio y devuelve el texto normalizado. Con un texto de espacios simples los
// dos coinciden, que es por lo que el mutante se camuflaba; con un espacio doble se
// ven.
func TestWrapTextNoTocaLoQueYaCabe(t *testing.T) {
	for _, tc := range []struct{ texto, quiere string }{
		{"ab cd", "ab cd"},
		{"a  b", "a  b"},
		{"a   b   c", "a   b   c"},
		{"a\tb", "a\tb"},
		{"ab  cd", "ab  cd"}, // dos espacios: 6 columnas, y con max 6 cabe
		{"  con  LEADING  y  final  ", "  con  LEADING  y  final  "},
	} {
		for _, max := range []int{1, 4, 5, 6, 10, 100} {
			if ansi.StringWidth(tc.texto) > max {
				continue // este max no cabe, no es el caso que se afirma
			}
			got := wrapText(tc.texto, max)
			if len(got) != 1 {
				t.Errorf("wrapText(%q, %d) devolvió %d líneas, want 1: el texto cabe", tc.texto, max, len(got))
				continue
			}
			if got[0] != tc.quiere {
				t.Errorf("wrapText(%q, %d) devolvió %q, want %q: un texto que cabe no se toca, ni en un espacio",
					tc.texto, max, got[0], tc.quiere)
			}
		}
	}

	// Y el caso del borde, dicho explícito: el texto mide 7 columnas y entra en un
	// ancho de 7, con dos espacios de más por dentro, que es donde el `<` se delata.
	const texto = "a  b  c"
	const ancho = 7
	if got := wrapText(texto, ancho); len(got) != 1 || got[0] != texto {
		t.Errorf("un texto de %d columnas con ancho %d dio %q, want la línea intacta",
			ansi.StringWidth(texto), ancho, got)
	}
	// Una columna menos de ancho y el envoltorio se pone a trabajar, y su trabajo se
	// nota aunque el texto quepa en UNA línea al final: los espacios se normalizan.
	//
	// Ese es el punto. "a  b  c" con ancho 6 son tres palabras de una columna que sí
	// caben juntas, así que sale una sola línea, pero con UN espacio entre ellas en
	// vez de dos. El texto cambió, y por eso se ve que entró al envoltorio: con el
	// `<` en vez del `<=`, el texto de ancho exacto también entraría, y con ancho
	// exacto lo que sale sería también normalizado. La diferencia entre "no ha entrado
	// al envoltorio" y "ha entrado" está en si los espacios sobreviven.
	if got := wrapText(texto, ancho-1); len(got) == 1 && got[0] == texto {
		t.Errorf("un texto de %d columnas con ancho %d volvió intacto: no debería, no cabe",
			ansi.StringWidth(texto), ancho-1)
	}
}

// TestWrapTextJuntaLaPalabraQueCierraElAncho: al envolver, una palabra que
// CIERRA el ancho se junta a la línea, y una palabra que se pasa de una columna a
// otra empieza línea nueva.
//
// El borde es un `<=` y no un `<`, y la diferencia se ve en el ancho justo: "ab cd"
// con max 5 son 2+1+2 = 5 columnas, o sea CIERRA. Con un `<` no se juntaría y
// saldrían dos líneas de dos columnas en una caja que cabe de sobra.
//
// Y aquí está el detalle que hace que el borde sea alcanzable: si el texto entero
// midiera lo mismo que el ancho, el envoltorio ni siquiera entraría al bucle. Hace
// falta un texto MÁS ANCHO que max en el que un TROZO sí cierre el ancho, y por eso
// el caso lleva tres palabras.
func TestWrapTextJuntaLaPalabraQueCierraElAncho(t *testing.T) {
	const max = 5

	// "ab" + " " + "cd" = 5 exacto: se juntan. Y luego "ef" ya no cabe.
	got := wrapText("ab cd ef", max)
	if len(got) != 2 {
		t.Fatalf("wrapText(%q, %d) devolvió %q, want 2 líneas: la segunda cierra el ancho justo",
			"ab cd ef", max, got)
	}
	if got[0] != "ab cd" {
		t.Errorf("la primera línea es %q, want %q: la palabra que cierra el ancho tiene que juntarse", got[0], "ab cd")
	}
	if got[1] != "ef" {
		t.Errorf("la segunda línea es %q, want %q", got[1], "ef")
	}

	// Y con un ancho UNO más pequeño, la misma palabra ya no cierra y la línea se
	// parte antes. Ese es el borde, en las dos direcciones.
	if got := wrapText("ab cd ef", max-1); len(got) != 3 {
		t.Errorf("wrapText(%q, %d) devolvió %q, want 3 líneas: con una columna menos la palabra ya no cierra",
			"ab cd ef", max-1, got)
	}

	// Y una palabra que cierra el ancho en la última posición, con texto de más
	// detrás: la última línea completa se queda tal cual.
	got = wrapText("ab cd ef gh", max)
	if len(got) != 2 || got[0] != "ab cd" || got[1] != "ef gh" {
		t.Errorf("wrapText(%q, %d) = %q, want [ab cd, ef gh]", "ab cd ef gh", max, got)
	}
}

// TestWrapTextAnchoNoPositivoNoParte: un ancho de cero o menos significa "no partas".
//
// Con un ancho de cero, partir por palabras daría una palabra por línea, que para un
// texto de tres palabras son tres líneas de dos columnas. No informa de nada, y peor:
// un texto partido en trozos de una palabra es un texto al que le falta la mitad.
//
// El ancho llega aquí desde toastGeometry, que nunca da menos de 6, y desde
// commentWidths, que nunca da menos de 8. Así que el suelo no se ve en el producto:
// se ve en el contrato de la función, que es pura y tiene que ser correcta por sí
// misma, no solo para los que llaman desde dentro.
func TestWrapTextAnchoNoPositivoNoParte(t *testing.T) {
	for _, max := range []int{-20, -1, 0} {
		for _, texto := range []string{"ab cd ef", "una palabra", "a  b  c", ""} {
			got := wrapText(texto, max)
			if len(got) != 1 {
				t.Errorf("wrapText(%q, %d) devolvió %q, want una sola línea: con un ancho no positivo no se parte",
					texto, max, got)
				continue
			}
			if got[0] != texto {
				t.Errorf("wrapText(%q, %d) devolvió %q: sin partir, el texto tiene que volver tal cual",
					texto, max, got[0])
			}
		}
	}
	// Y el texto vacío o de puros espacios: una línea, no ninguna. Una lista vacía
	// haría que el que llama escribiera una línea en blanco de más.
	for _, texto := range []string{"", "   ", "\t"} {
		if got := wrapText(texto, 10); len(got) != 1 {
			t.Errorf("wrapText(%q, 10) devolvió %q, want una línea", texto, got)
		}
	}
	// Y con puros espacios el ancho SÍ importa: no hay palabras que juntar, así que
	// sale el texto tal cual en vez de una línea vacía inventada.
	if got := wrapText("    ", 2); len(got) != 1 || got[0] != "    " {
		t.Errorf("wrapText(%q, 2) = %q, want el texto intacto en una línea", "    ", got)
	}
}

// TestWrapTextCadaLineaCabeYNoSeParteUnaPalabra: las dos propiedades que hacen que
// el texto siga leyéndose.
//
// La primera: toda línea cabe. La segunda: una palabra no se parte, porque partirla
// por la mitad la convierte en otra palabra, y un aviso que dice otra cosa no informa
// de nada. Es la razón de que el envoltorio sea por palabras y no por columnas.
func TestWrapTextCadaLineaCabeYNoSeParteUnaPalabra(t *testing.T) {
	for max := 1; max <= 40; max++ {
		for _, texto := range []string{
			"ab cd ef gh ij",
			"palabra muy larga que no cabe de ninguna manera en nada",
			strings.Repeat("x ", 30) + "y",
			"corto",
			"  espacios   al   principio  y   al   final  ",
		} {
			for _, linea := range wrapText(texto, max) {
				if ansi.StringWidth(linea) > max {
					// Solo vale si es UNA palabra sola más ancha que max, que es el
					// caso que no se puede partir.
					if len(strings.Fields(linea)) != 1 {
						t.Errorf("wrapText(%.20q, %d) dio la línea %q, que no cabe y no es una palabra suelta",
							texto, max, linea)
					}
				}
				// Y ninguna línea se parte un espacio por la mitad: o el texto
				// entero, o palabras completas.
				if strings.TrimSpace(linea) == "" && strings.TrimSpace(texto) != "" {
					t.Errorf("wrapText(%.20q, %d) dio una línea de puros espacios: %q", texto, max, linea)
				}
			}
			// Y las palabras del texto salen enteras en alguna línea: no se pierde
			// ni se parte ninguna.
			juntas := wrapText(texto, max)
			for _, palabra := range strings.Fields(texto) {
				found := false
				for _, linea := range juntas {
					if palabra == linea || strings.Contains(linea, " "+palabra+" ") ||
						strings.HasPrefix(linea, palabra+" ") || strings.HasSuffix(linea, " "+palabra) {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("wrapText(%.20q, %d) perdió o partió la palabra %q", texto, max, palabra)
				}
			}
		}
	}
}
