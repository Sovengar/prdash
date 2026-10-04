package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

// If the text fits it comes out WHOLE, spaces and all.
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

	const texto = "a  b  c"
	const ancho = 7
	if got := wrapText(texto, ancho); len(got) != 1 || got[0] != texto {
		t.Errorf("un texto de %d columnas con ancho %d dio %q, want la línea intacta",
			ansi.StringWidth(texto), ancho, got)
	}
	// One column less and the wrapper starts working, and its work shows.
	if got := wrapText(texto, ancho-1); len(got) == 1 && got[0] == texto {
		t.Errorf("un texto de %d columnas con ancho %d volvió intacto: no debería, no cabe",
			ansi.StringWidth(texto), ancho-1)
	}
}

func TestWrapTextJuntaLaPalabraQueCierraElAncho(t *testing.T) {
	const max = 5

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

	if got := wrapText("ab cd ef", max-1); len(got) != 3 {
		t.Errorf("wrapText(%q, %d) devolvió %q, want 3 líneas: con una columna menos la palabra ya no cierra",
			"ab cd ef", max-1, got)
	}

	got = wrapText("ab cd ef gh", max)
	if len(got) != 2 || got[0] != "ab cd" || got[1] != "ef gh" {
		t.Errorf("wrapText(%q, %d) = %q, want [ab cd, ef gh]", "ab cd ef gh", max, got)
	}
}

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
	// Empty or spaces only gives ONE line, not none: an empty list would make the caller print
	// an extra blank line.
	for _, texto := range []string{"", "   ", "\t"} {
		if got := wrapText(texto, 10); len(got) != 1 {
			t.Errorf("wrapText(%q, 10) devolvió %q, want una línea", texto, got)
		}
	}
	if got := wrapText("    ", 2); len(got) != 1 || got[0] != "    " {
		t.Errorf("wrapText(%q, 2) = %q, want el texto intacto en una línea", "    ", got)
	}
}

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
					if len(strings.Fields(linea)) != 1 {
						t.Errorf("wrapText(%.20q, %d) dio la línea %q, que no cabe y no es una palabra suelta",
							texto, max, linea)
					}
				}
				if strings.TrimSpace(linea) == "" && strings.TrimSpace(texto) != "" {
					t.Errorf("wrapText(%.20q, %d) dio una línea de puros espacios: %q", texto, max, linea)
				}
			}
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
