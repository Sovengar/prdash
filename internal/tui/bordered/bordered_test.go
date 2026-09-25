package bordered

import (
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
