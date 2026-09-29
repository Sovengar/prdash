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

// El contenido más ancho que el interior se recorta, no se re-envuelve: el wrap
// rompería el alto que el layout calculó.
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

// El recorte es ANSI-safe: la secuencia de color del contenido no se corrompe.
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

// TestBordeSinCaracterRellenaConEspacio: un borde con el caracter de relleno
// vacío (un lipgloss.Bound normalizado o construido a mano) pintaría una línea de
// bordes pegados, sin separación. Cae a un espacio, que es lo que hace el borde
// por defecto.
//
// Y un borde sin caracter lateral usa un espacio también: el interior no puede
// pegarse al marco.
func TestBordeSinCaracterRellenaConEspacio(t *testing.T) {
	// Borde con relleno vacío pero laterales presentes: el relleno tiene que ser
	// un espacio o la línea de arriba sería "┌┐" en vez de "┌──┐".
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

	// Sin laterales: también a espacio, o el interior se pegaría al marco.
	sinLaterales := lipgloss.Border{TopLeft: "+", Top: "-", TopRight: "+", BottomLeft: "+", Bottom: "-", BottomRight: "+"}
	out = RenderWithTitle(sinLaterales, nil, "", "hola", 8)
	lines = strings.Split(out, "\n")
	if got := ansi.Strip(lines[1]); got != " hola   " {
		t.Errorf("línea interior = %q, want \" hola   \" (laterales de espacio)", got)
	}
}

// TestElInteriorSeRellenaAlAnchoExacto: cada línea de contenido tiene que medir
// el interior completo. Si el relleno se calculara al revés (rellenando cuando NO
// sobra), una línea ancha se saldría de la caja y el borde derecho quedaría
// pisado por el texto; y si no se rellenara, la caja sería irregular y las
// columnas de dos cajas no casarían.
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
	// Y el interior es width-2, con los dos bordes: el contenido se recorta para
	// caber y el resto se rellena.
	out := RenderWithTitle(Rounded(), nil, "", "abcdefghij", 6)
	lines := strings.Split(out, "\n")
	if got := ansi.Strip(lines[1]); got != "│abcd│" {
		t.Errorf("contenido recortado = %q, want \"│abcd│\"", got)
	}
}

// TestTituloMasAnchoQueElInteriorSeRecorta: un título que no cabe deja de
// desbordar la caja. Es lo que pasa con un nombre de repo largo en un terminal
// estrecho: sin recorte, la línea de arriba sería más ancha que las de abajo y la
// caja quedaría torcida.
func TestTituloMasAnchoQueElInteriorSeRecorta(t *testing.T) {
	for _, width := range []int{4, 6, 8, 12} {
		out := RenderWithTitle(Rounded(), nil, "un titulo bastante largo de verdad", "", width)
		for i, l := range strings.Split(out, "\n") {
			if w := ansi.StringWidth(l); w != width {
				t.Errorf("width %d: línea %d mide %d, want %d: %q", width, i, w, width, ansi.Strip(l))
			}
		}
		// Y con el título justo en el límite, tampoco desborda.
		out = RenderWithTitle(Rounded(), nil, "abc", "", width)
		for i, l := range strings.Split(out, "\n") {
			if w := ansi.StringWidth(l); w != width {
				t.Errorf("width %d, título justo: línea %d mide %d, want %d", width, i, w, width)
			}
		}
	}
}

// TestElColorDelBordeSeAplica: el color del borde es parte del render, y un nil
// lo deja sin estilo mientras que un color lo aplica. Los tests existentes pasan
// nil, así que ninguno distingue "sin estilo" de "con estilo": un `nil` invertido
// seguiría pintando igual y nadie se enteraría.
//
// Se comprueba con un color real que el ANSI aparece en los bordes y en el
// relleno, y que el CONTENIDO conserva su propio estilo. Con nil, no hay ANSI del
// borde en ningún sitio.
func TestElColorDelBordeSeAplica(t *testing.T) {
	conColor := color.RGBA{R: 0x1b, G: 0x2b, B: 0x34, A: 0xff}
	out := RenderWithTitle(Rounded(), conColor, "t", "hola", 10)
	lines := strings.Split(out, "\n")
	if len(lines) != 3 {
		t.Fatalf("líneas = %d, want 3", len(lines))
	}
	// Con color hay secuencia ANSI: sin ella, el borde sería invisible en un
	// terminal de color y la caja no se distinguiría del fondo.
	for i, l := range lines {
		if !strings.Contains(l, "\x1b[") {
			t.Errorf("línea %d sin ANSI con color de borde: %q", i, ansi.Strip(l))
		}
		// Y el ancho NO cambia por el estilo: es la razón de medir en texto plano.
		if w := ansi.StringWidth(l); w != 10 {
			t.Errorf("línea %d ancho = %d, want 10: el estilo no debe alterar el ancho", i, w)
		}
	}
	// El contenido conserva su estilo propio: el del borde no lo pisa.
	conEstilo := lipgloss.NewStyle().Bold(true).Render("hola")
	out = RenderWithTitle(Rounded(), conColor, "t", conEstilo, 10)
	if !strings.Contains(out, "\x1b[1m") && !strings.Contains(out, "\x1b[") {
		t.Errorf("el contenido perdió su estilo: %q", out)
	}

	// Y con nil no hay estilo de borde: el texto plano es idéntico.
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

// El título más largo que el interior se recorta, no desborda la caja.
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

// El relleno del contenido lo hace el caller, no la caja: una línea de " " es
// indistinguible de una línea vacía para la caja, así que el alto exacto depende
// de quien compone.
func TestRenderWithTitleRellenaAltoDado(t *testing.T) {
	out := RenderWithTitle(Rounded(), nil, "", strings.Repeat("\n", 2), 8)
	if got := len(strings.Split(out, "\n")); got != 5 {
		t.Errorf("líneas = %d, want 5 (2 bordes + 3 de contenido)", got)
	}
}
