package bordered

import (
	"image/color"
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

func TestRenderWithTitleAnchoExactoYGrifos(t *testing.T) {
	out := RenderWithTitle(Rounded(), lipgloss.Color("238"), " prdash ", "hola", 20)
	lines := strings.Split(out, "\n")
	if len(lines) != 3 {
		t.Fatalf("líneas = %d, want 3 (borde + contenido + borde)", len(lines))
	}
	for i, l := range lines {
		if w := ansi.StringWidth(l); w != 20 {
			t.Errorf("línea %d ancho = %d, want 20: %q", i, w, l)
		}
	}
	top := ansi.Strip(lines[0])
	if !strings.HasPrefix(top, "╭") || !strings.HasSuffix(top, "╮") {
		t.Errorf("esquinas superiores incorrectas: %q", top)
	}
	if !strings.Contains(top, " prdash ") {
		t.Errorf("título no embebido en la línea superior: %q", top)
	}
	bot := ansi.Strip(lines[2])
	if !strings.HasPrefix(bot, "╰") || !strings.HasSuffix(bot, "╯") {
		t.Errorf("esquinas inferiores incorrectas: %q", bot)
	}
}

// Content wider than the interior is CLIPPED, not re-wrapped: wrapping would break the
// geometry.
func TestRenderWithTitleRecortaSinWrap(t *testing.T) {
	largo := strings.Repeat("x", 100)
	out := RenderWithTitle(Rounded(), nil, "", largo, 12)
	lines := strings.Split(out, "\n")
	if len(lines) != 3 {
		t.Fatalf("líneas = %d, want 3 (el recorte no añade líneas)", len(lines))
	}
	for i, l := range lines {
		if w := ansi.StringWidth(l); w != 12 {
			t.Errorf("línea %d ancho = %d, want 12", i, w)
		}
	}
	if got := ansi.StringWidth(ansi.Strip(lines[1])); got != 12 {
		t.Errorf("contenido recortado ancho = %d, want 12", got)
	}
}

// The clip is ANSI-safe: the content's colour sequence is not corrupted.
func TestRenderWithTitleRecorteANSI(t *testing.T) {
	contenido := "\x1b[31m" + strings.Repeat("ab", 40) + "\x1b[0m"
	out := RenderWithTitle(Rounded(), nil, "", contenido, 10)
	lines := strings.Split(out, "\n")
	if len(lines) != 3 {
		t.Fatalf("líneas = %d, want 3", len(lines))
	}
	if w := ansi.StringWidth(lines[1]); w != 10 {
		t.Errorf("ancho ANSI = %d, want 10: %q", w, lines[1])
	}
	if !strings.Contains(lines[1], "\x1b[") {
		t.Errorf("se perdió el ANSI del contenido: %q", lines[1])
	}
}

// A border with an empty fill character (a lipgloss.Border with none set).
func TestBordeSinCaracterRellenaConEspacio(t *testing.T) {
	vacio := lipgloss.Border{TopLeft: "┌", Top: "", TopRight: "┐", Left: "│", Right: "│", BottomLeft: "└", Bottom: "", BottomRight: "┘"}
	out := RenderWithTitle(vacio, nil, "", "hola", 8)
	lines := strings.Split(out, "\n")
	if len(lines) != 3 {
		t.Fatalf("líneas = %d, want 3", len(lines))
	}
	if got := ansi.Strip(lines[0]); got != "┌      ┐" {
		t.Errorf("línea superior = %q, want \"┌      ┐\" (relleno de espacio)", got)
	}
	if got := ansi.Strip(lines[2]); got != "└      ┘" {
		t.Errorf("línea inferior = %q, want \"└      ┘\"", got)
	}
	for i, l := range lines {
		if w := ansi.StringWidth(l); w != 8 {
			t.Errorf("línea %d ancho = %d, want 8: %q", i, w, l)
		}
	}

	// Without side borders it is a space too, or the content would stick to the frame.
	sinLaterales := lipgloss.Border{TopLeft: "+", Top: "-", TopRight: "+", BottomLeft: "+", Bottom: "-", BottomRight: "+"}
	out = RenderWithTitle(sinLaterales, nil, "", "hola", 8)
	lines = strings.Split(out, "\n")
	if got := ansi.Strip(lines[1]); got != " hola   " {
		t.Errorf("línea interior = %q, want \" hola   \" (laterales de espacio)", got)
	}
}

func TestElInteriorSeRellenaAlAnchoExacto(t *testing.T) {
	for _, inner := range []string{"x", "corta", "exacta!!", "se pasa de largo y de sobra"} {
		for _, width := range []int{6, 8, 12, 20} {
			out := RenderWithTitle(Rounded(), nil, "", inner, width)
			for i, l := range strings.Split(out, "\n") {
				if w := ansi.StringWidth(l); w != width {
					t.Errorf("contenido %q y width %d: línea %d mide %d, want %d: %q",
						inner, width, i, w, width, ansi.Strip(l))
				}
			}
		}
	}
	out := RenderWithTitle(Rounded(), nil, "", "abcdefghij", 6)
	lines := strings.Split(out, "\n")
	if got := ansi.Strip(lines[1]); got != "│abcd│" {
		t.Errorf("contenido recortado = %q, want \"│abcd│\"", got)
	}
}

func TestTituloMasAnchoQueElInteriorSeRecorta(t *testing.T) {
	for _, width := range []int{4, 6, 8, 12} {
		out := RenderWithTitle(Rounded(), nil, "un titulo bastante largo de verdad", "", width)
		for i, l := range strings.Split(out, "\n") {
			if w := ansi.StringWidth(l); w != width {
				t.Errorf("width %d: línea %d mide %d, want %d: %q", width, i, w, width, ansi.Strip(l))
			}
		}
		out = RenderWithTitle(Rounded(), nil, "abc", "", width)
		for i, l := range strings.Split(out, "\n") {
			if w := ansi.StringWidth(l); w != width {
				t.Errorf("width %d, título justo: línea %d mide %d, want %d", width, i, w, width)
			}
		}
	}
}

// A nil border colour leaves it unstyled.
func TestElColorDelBordeSeAplica(t *testing.T) {
	conColor := color.RGBA{R: 0x1b, G: 0x2b, B: 0x34, A: 0xff}
	out := RenderWithTitle(Rounded(), conColor, "t", "hola", 10)
	lines := strings.Split(out, "\n")
	if len(lines) != 3 {
		t.Fatalf("líneas = %d, want 3", len(lines))
	}
	// With a colour there is an ANSI sequence: without one the border would be invisible on a dark
	// terminal.
	for i, l := range lines {
		if !strings.Contains(l, "\x1b[") {
			t.Errorf("línea %d sin ANSI con color de borde: %q", i, ansi.Strip(l))
		}
		// And the width does NOT change with the style: that is why the measurement is in plain text.
		if w := ansi.StringWidth(l); w != 10 {
			t.Errorf("línea %d ancho = %d, want 10: el estilo no debe alterar el ancho", i, w)
		}
	}
	conEstilo := lipgloss.NewStyle().Bold(true).Render("hola")
	out = RenderWithTitle(Rounded(), conColor, "t", conEstilo, 10)
	if !strings.Contains(out, "\x1b[1m") && !strings.Contains(out, "\x1b[") {
		t.Errorf("el contenido perdió su estilo: %q", out)
	}

	plano := ansi.Strip(RenderWithTitle(Rounded(), nil, "t", "hola", 10))
	if plano != ansi.Strip(out) {
		t.Errorf("con y sin color el texto plano debería ser el mismo:\n%q\n%q", plano, ansi.Strip(out))
	}
}

func TestRenderWithTitleWidthMinimo(t *testing.T) {
	out := RenderWithTitle(Rounded(), nil, "titulo largo", "x", 1)
	for i, l := range strings.Split(out, "\n") {
		if w := ansi.StringWidth(l); w != 2 {
			t.Errorf("línea %d ancho = %d, want 2 (clamp)", i, w)
		}
	}
}

func TestRenderWithTitleContenidoVacio(t *testing.T) {
	out := RenderWithTitle(Rounded(), nil, "", "", 8)
	lines := strings.Split(out, "\n")
	if len(lines) != 3 {
		t.Fatalf("líneas = %d, want 3", len(lines))
	}
	if got := ansi.Strip(lines[1]); got != "│      │" { // interior = 8-2
		t.Errorf("línea interior vacía = %q, want borde + relleno interior", got)
	}
}

func TestRenderWithTitleTituloLargo(t *testing.T) {
	out := RenderWithTitle(Rounded(), nil, strings.Repeat("t", 40), "x", 10)
	for i, l := range strings.Split(out, "\n") {
		if w := ansi.StringWidth(l); w != 10 {
			t.Errorf("línea %d ancho = %d, want 10: %q", i, w, l)
		}
	}
}

// La leyenda inferior se embebe en la línea de borde inferior.
func TestRenderWithTitlesLeyendaInferior(t *testing.T) {
	out := RenderWithTitles(Rounded(), nil, " arriba ", AlignLeft, " abajo ", AlignRight, "c", 24)
	lines := strings.Split(out, "\n")
	bot := ansi.Strip(lines[len(lines)-1])
	if !strings.Contains(bot, " abajo ") {
		t.Errorf("leyenda inferior ausente: %q", bot)
	}
	if !strings.HasSuffix(bot, "╯") {
		t.Errorf("esquina inferior derecha ausente: %q", bot)
	}
	for i, l := range lines {
		if w := ansi.StringWidth(l); w != 24 {
			t.Errorf("línea %d ancho = %d, want 24", i, w)
		}
	}
}

// The content's padding is the caller's, not the box's: a line of spaces is indistinguishable
// from an empty one.
func TestRenderWithTitleRellenaAltoDado(t *testing.T) {
	out := RenderWithTitle(Rounded(), nil, "", strings.Repeat("\n", 2), 8)
	if got := len(strings.Split(out, "\n")); got != 5 {
		t.Errorf("líneas = %d, want 5 (2 bordes + 3 de contenido)", got)
	}
}
