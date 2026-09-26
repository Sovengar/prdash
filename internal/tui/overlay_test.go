package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

// viewOf es una vista de fondo reconocible: cada fila lleva su número al final y
// ocupa exactamente el ancho del terminal, como las cajas reales, para poder
// comprobar fila a fila qué sobrevivió al popup.
func viewOf(rows, width int) string {
	lines := make([]string, 0, rows)
	for i := range rows {
		label := "···" + string(rune('0'+i))
		lines = append(lines, label+strings.Repeat("·", max(0, width-ansi.StringWidth(label))))
	}
	return strings.Join(lines, "\n")
}

// plain deja solo el texto visible de una línea, para comparar sin que estorben
// los códigos de color.
func plain(s string) string { return ansi.Strip(s) }

// TestOverlayKeepsTheBackgroundAroundThePopup: el popup se superpone, no
// sustituye. Si las filas de arriba y de abajo desaparecieran, la TUI habría
// perdido el contexto en el momento justo en que se está leyendo.
func TestOverlayKeepsTheBackgroundAroundThePopup(t *testing.T) {
	content := viewOf(9, 20)
	box := "AAA\nBBB\nCCC"

	got := plain(overlayCentered(content, box, 20))
	lines := strings.Split(got, "\n")
	if len(lines) != 9 {
		t.Fatalf("la vista cambió de alto: %d líneas, want 9", len(lines))
	}
	for i, l := range lines {
		want := "···" + string(rune('0'+i)) + strings.Repeat("·", 16)
		if l == want {
			continue // fila intacta
		}
		if i >= 3 && i <= 5 {
			continue // las que tapa el popup
		}
		t.Errorf("fila %d = %q, want %q", i, l, want)
	}
	for i, want := range []string{"AAA", "BBB", "CCC"} {
		if row := strings.Split(got, "\n")[3+i]; !strings.Contains(row, want) {
			t.Errorf("fila %d = %q, no contiene %q", 3+i, row, want)
		}
	}
}

// TestOverlayCentersHorizontally: un popup descentrado se lee como pegado a un
// borde, que es donde no se mira.
func TestOverlayCentersHorizontally(t *testing.T) {
	got := plain(overlayCentered(strings.Repeat("x", 21), "MID", 21))
	row := strings.Split(got, "\n")[0]
	if !strings.HasPrefix(row, "xxxx") || !strings.HasSuffix(row, "xxxx") {
		t.Errorf("fila = %q, want el popup centrado", row)
	}
	if !strings.Contains(row, "MID") {
		t.Errorf("fila = %q, no contiene el popup", row)
	}
}

// TestOverlayPreservesLineWidth: si la línea resultara más ancha o más estrecha
// que antes, el popup se desplazaría un carácter en cada fila y la vista entera
// bailaría.
func TestOverlayPreservesLineWidth(t *testing.T) {
	content := viewOf(7, 20)
	for _, box := range []string{"A", "AAA\nBBB\nCCC", strings.Repeat("W", 40)} {
		got := overlayCentered(content, box, 20)
		for i, l := range strings.Split(got, "\n") {
			if w := ansi.StringWidth(l); w != 20 {
				t.Errorf("caja %q: fila %d mide %d, want 20", box, i, w)
			}
		}
	}
}

// TestOverlayCropsABoxTallerThanTheView: una ventana más alta que la pantalla
// tiene que recortarse, no empujar la vista hacia abajo ni desbordarla.
func TestOverlayCropsABoxTallerThanTheView(t *testing.T) {
	content := viewOf(3, 20)
	box := "1\n2\n3\n4\n5"

	got := plain(overlayCentered(content, box, 20))
	if lines := strings.Split(got, "\n"); len(lines) != 3 {
		t.Fatalf("líneas = %d, want 3", len(lines))
	}
	if !strings.Contains(got, "3") || strings.Contains(got, "4") {
		t.Errorf("no recortó por abajo:\n%s", got)
	}
}

// TestOverlayWithoutBoxIsIdentity: sin popup la vista tiene que volver idéntica,
// byte a byte, porque es la salida más frecuente.
func TestOverlayWithoutBoxIsIdentity(t *testing.T) {
	content := viewOf(5, 20)
	if got := overlayCentered(content, "", 20); got != content {
		t.Error("un popup vacío alteró la vista")
	}
}

// TestOverlayKeepsBaseColorsRightOfThePopup: la línea de base va recortada con
// ansi, no concatenada por índices: si el recorte no fuera consciente del color,
// lo que queda a la derecha saldría con el color por defecto del terminal y en
// scroll horizontal se vería el salto.
func TestOverlayKeepsBaseColorsRightOfThePopup(t *testing.T) {
	base := "\x1b[31m" + strings.Repeat("r", 20) + "\x1b[0m"
	got := overlayCentered(base, "PP", 20)

	if !strings.Contains(got, "\x1b[31m") {
		t.Error("la línea de base perdió su secuencia de color")
	}
	if !strings.HasSuffix(got, "\x1b[0m") {
		t.Errorf("la línea no conserva el cierre de estilo: %q", got)
	}
}
